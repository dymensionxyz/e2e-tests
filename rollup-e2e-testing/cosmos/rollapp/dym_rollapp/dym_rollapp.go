package dym_rollapp

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	valKey = "validator"
)

type DymRollApp struct {
	*cosmos.CosmosChain
	sequencerKeyDir string
	sequencerKey    string
	extraFlags      map[string]interface{}
}

var _ ibc.Chain = (*DymRollApp)(nil)
var _ ibc.RollApp = (*DymRollApp)(nil)

func NewDymRollApp(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger, extraFlags map[string]interface{}) *DymRollApp {
	cosmosChain := cosmos.NewCosmosChain(testName, chainConfig, numValidators, numFullNodes, log)

	c := &DymRollApp{
		CosmosChain: cosmosChain,
		extraFlags:  extraFlags,
	}

	return c
}

func (c *DymRollApp) Start(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) error {
	nodes := c.Nodes()

	if err := nodes.LogGenesisHashes(ctx); err != nil {
		return err
	}

	eg, egCtx := errgroup.WithContext(ctx)
	for _, n := range nodes {
		n := n
		eg.Go(func() error {
			return n.CreateNodeContainer(egCtx)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	peers := nodes.PeerString(ctx)

	eg, egCtx = errgroup.WithContext(ctx)
	for _, n := range nodes {
		n := n
		c.Logger().Info("Starting container", zap.String("container", n.Name()))
		eg.Go(func() error {
			if err := n.SetPeers(egCtx, peers); err != nil {
				return err
			}
			return n.StartContainer(egCtx)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// Wait for 5 blocks before considering the chains "started"
	return testutil.WaitForBlocks(ctx, 5, c.GetNode())
}

func (c *DymRollApp) Configuration(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) error {
	chainCfg := c.Config()

	decimalPow := int64(math.Pow10(int(*chainCfg.CoinDecimals)))

	genesisAmount := sdk.Coin{
		Amount: sdkmath.NewInt(100_000_000_000_000).MulRaw(decimalPow),
		Denom:  chainCfg.Denom,
	}

	genesisSelfDelegation := sdk.Coin{
		Amount: sdkmath.NewInt(50_000_000_000_000).MulRaw(decimalPow),
		Denom:  chainCfg.Denom,
	}

	if chainCfg.ModifyGenesisAmounts != nil {
		genesisAmount, genesisSelfDelegation = chainCfg.ModifyGenesisAmounts()
	}

	genesisAmounts := []sdk.Coin{genesisAmount}

	configFileOverrides := chainCfg.ConfigFileOverrides

	eg := new(errgroup.Group)
	// Initialize config and sign gentx for each validator.
	for _, v := range c.Validators {
		v := v
		c.sequencerKeyDir = v.HomeDir()
		v.Chain = c
		v.Validator = true
		eg.Go(func() error {
			if err := v.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("Provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := testutil.ModifyTomlConfigFile(
					ctx,
					v.Logger(),
					v.DockerClient,
					v.TestName,
					v.VolumeName,
					v.Chain.Config().Name,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			if !c.Config().SkipGenTx {
				return v.InitValidatorGenTx(ctx, &chainCfg, genesisAmounts, genesisSelfDelegation)
			}
			return nil
		})
	}

	// Initialize config for each full node.
	for _, n := range c.FullNodes {
		n := n
		n.Validator = false
		n.Chain = c
		eg.Go(func() error {
			if err := n.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("Provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := testutil.ModifyTomlConfigFile(
					ctx,
					n.Logger(),
					n.DockerClient,
					n.TestName,
					n.VolumeName,
					n.Chain.Config().Name,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// wait for this to finish
	if err := eg.Wait(); err != nil {
		return err
	}

	if c.Config().PreGenesis != nil {
		err := c.Config().PreGenesis(chainCfg)
		if err != nil {
			return err
		}
	}

	// for the validators we need to collect the gentxs and the accounts
	// to the first node's genesis file
	validator0 := c.Validators[0]
	for i := 1; i < len(c.Validators); i++ {
		validatorN := c.Validators[i]

		bech32, err := validatorN.AccountKeyBech32(ctx, valKey)
		if err != nil {
			return err
		}
		if err := validator0.AddGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
			return err
		}

		if !c.Config().SkipGenTx {
			if err := validatorN.CopyGentx(ctx, validator0); err != nil {
				return err
			}
		}
	}

	for _, wallet := range additionalGenesisWallets {

		if err := validator0.AddGenesisAccount(ctx, wallet.Address, []sdk.Coin{{Denom: wallet.Denom, Amount: wallet.Amount}}); err != nil {
			return err
		}
	}

	if !c.Config().SkipGenTx {
		if err := validator0.CollectGentxs(ctx); err != nil {
			return err
		}
	}

	genbz, err := validator0.GenesisFileContent(ctx)
	if err != nil {
		return err
	}

	genbz = bytes.ReplaceAll(genbz, []byte(`"stake"`), []byte(fmt.Sprintf(`"%s"`, chainCfg.Denom)))

	if c.Config().ModifyGenesis != nil {
		genbz, err = c.Config().ModifyGenesis(chainCfg, genbz)
		if err != nil {
			return err
		}
	}

	// Provide EXPORT_GENESIS_FILE_PATH and EXPORT_GENESIS_CHAIN to help debug genesis file
	exportGenesis := os.Getenv("EXPORT_GENESIS_FILE_PATH")
	exportGenesisChain := os.Getenv("EXPORT_GENESIS_CHAIN")
	if exportGenesis != "" && exportGenesisChain == c.Config().Name {
		c.Logger().Debug("Exporting genesis file",
			zap.String("chain", exportGenesisChain),
			zap.String("path", exportGenesis),
		)
		_ = os.WriteFile(exportGenesis, genbz, 0600)
	}
	nodes := c.Nodes()

	for _, node := range nodes {
		if err := node.OverwriteGenesisFile(ctx, genbz); err != nil {
			return err
		}
	}
	c.sequencerKey, err = c.ShowSequencer(ctx)
	if err != nil {
		return fmt.Errorf("failed to show seq %s: %w", c.Config().Name, err)
	}
	return nil
}

func (c *DymRollApp) ShowSequencer(ctx context.Context) (string, error) {
	var command []string
	command = append(command, "dymint", "show-sequencer")

	seq, _, err := c.GetNode().ExecBin(ctx, command...)
	return string(bytes.TrimSuffix(seq, []byte("\n"))), err
}

func (c *DymRollApp) GetSequencer() string {
	return c.sequencerKey
}

func (c *DymRollApp) GetSequencerKeyDir() string {
	return c.sequencerKeyDir
}
