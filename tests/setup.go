package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"cosmossdk.io/math"
	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/cosmos/cosmos-sdk/x/params/client/utils"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/icza/dyno"
	"github.com/stretchr/testify/require"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	hubgenesis "github.com/dymensionxyz/dymension-rdk/x/hub-genesis/types"
	denommetadatatypes "github.com/dymensionxyz/dymension/v3/x/denommetadata/types"
	eibc "github.com/dymensionxyz/dymension/v3/x/eibc/types"
	rollapp "github.com/dymensionxyz/dymension/v3/x/rollapp/types"
	ethermintcrypto "github.com/evmos/ethermint/crypto/codec"
	ethermint "github.com/evmos/ethermint/types"
)

var rollappDenomMetadata = banktypes.Metadata{
	Description: "Denom of the rollapp",
	Base:        "urax",
	Display:     "RAX",
	Name:        "RAX",
	Symbol:      "urax",
	DenomUnits: []*banktypes.DenomUnit{
		{
			Denom:    "urax",
			Exponent: 0,
		}, {
			Denom:    "RAX",
			Exponent: 18,
		},
	},
}

type memoData struct {
	denommetadatatypes.MemoData
	User *userData `json:"user,omitempty"`
}

type userData struct {
	Data string `json:"data"`
}

func MustMarshalJSON(v any) string {
	bz, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(bz)
}

type PacketMetadata struct {
	Forward *ForwardMetadata `json:"forward"`
}

type ForwardMetadata struct {
	Receiver       string        `json:"receiver"`
	Port           string        `json:"port"`
	Channel        string        `json:"channel"`
	Timeout        time.Duration `json:"timeout"`
	Retries        *uint8        `json:"retries,omitempty"`
	Next           *string       `json:"next,omitempty"`
	RefundSequence *uint64       `json:"refund_sequence,omitempty"`
}

const (
	ibcPath               = "dymension-demo"
	anotherIbcPath        = "dymension-demo2"
	BLOCK_FINALITY_PERIOD = 30
)

var (
	walletAmount = math.NewInt(1_000_000_000_000)

	transferAmount = math.NewInt(1_000_000)

	bigTransferAmount = math.NewInt(1_000_000_000)

	zeroBal = math.ZeroInt()

	bridgingFee = math.NewInt(1_000)

	bigBridgingFee = math.NewInt(1_000_000)

	DymensionMainRepo = "ghcr.io/dymensionxyz/dymension"

	RollappEVMMainRepo = "ghcr.io/dymensionxyz/rollapp-evm"

	RollappWasmMainRepo = "ghcr.io/dymensionxyz/rollapp-wasm"

	RelayerMainRepo = "ghcr.io/dymensionxyz/go-relayer"

	dymensionVersion, rollappEVMVersion, rollappWasmVersion, relayerVersion = GetDockerImageVersion()

	upgradeName, upgradeEVMName, upgradeWasmName = GetUpgradeName()

	pullRelayerImage = GetPullRelayerImage()

	dymensionImage = ibc.DockerImage{
		Repository: DymensionMainRepo,
		Version:    dymensionVersion,
		UidGid:     "1025:1025",
	}

	rollappEVMImage = ibc.DockerImage{
		Repository: RollappEVMMainRepo,
		Version:    rollappEVMVersion,
		UidGid:     "1025:1025",
	}

	rollappWasmImage = ibc.DockerImage{
		Repository: RollappWasmMainRepo,
		Version:    rollappWasmVersion,
		UidGid:     "1025:1025",
	}

	preUpgradeRollappEVMImage = ibc.DockerImage{
		Repository: RollappEVMMainRepo,
		Version:    "latest",
		UidGid:     "1025:1025",
	}

	preUpgradeRollappWasmImage = ibc.DockerImage{
		Repository: RollappWasmMainRepo,
		Version:    "a00a8c6d",
		UidGid:     "1025:1025",
	}

	dymensionConfig = ibc.ChainConfig{
		Type:                "hub-dym",
		Name:                "dymension",
		ChainID:             "dymension_100-1",
		Images:              []ibc.DockerImage{dymensionImage},
		Bin:                 "dymd",
		Bech32Prefix:        "dym",
		Denom:               "adym",
		CoinType:            "60",
		GasPrices:           "0.0adym",
		EncodingConfig:      encodingConfig(),
		GasAdjustment:       1.1,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
		ConfigFileOverrides: nil,
	}

	// Setup for gaia
	gaiaImageRepo = "ghcr.io/strangelove-ventures/heighliner/gaia"

	gaiaImage = ibc.DockerImage{
		Repository: gaiaImageRepo,
		UidGid:     "1025:1025",
	}

	gaiaConfig = ibc.ChainConfig{
		Type:                "cosmos",
		Name:                "gaia",
		ChainID:             "gaia_1",
		Images:              []ibc.DockerImage{gaiaImage},
		Bin:                 "gaiad",
		Bech32Prefix:        "cosmos",
		Denom:               "uatom",
		CoinType:            "118",
		GasPrices:           "0uatom",
		EncodingConfig:      defaultConfig(),
		GasAdjustment:       2,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       nil,
		ConfigFileOverrides: nil,
	}

	rollappEVMGenesisKV = []cosmos.GenesisKV{
		{
			Key:   "app_state.mint.params.mint_denom",
			Value: "urax",
		},
		{
			Key:   "app_state.staking.params.bond_denom",
			Value: "urax",
		},
		{
			Key:   "app_state.evm.params.evm_denom",
			Value: "urax",
		},
		{
			Key:   "consensus_params.block.max_gas",
			Value: "40000000",
		},
		{
			Key:   "app_state.feemarket.params.no_base_fee",
			Value: true,
		},
		{
			Key:   "app_state.feemarket.params.min_gas_price",
			Value: "0",
		},
		{
			Key:   "app_state.gov.voting_params.voting_period",
			Value: "30s",
		},
		{
			Key:   "app_state.gov.deposit_params.max_deposit_period",
			Value: "30s",
		},
		{
			Key:   "app_state.erc20.params.enable_erc20",
			Value: true,
		},
		{
			Key:   "app_state.erc20.params.enable_evm_hook",
			Value: false,
		},
		// Bank denom metadata
		{
			Key: "app_state.bank.denom_metadata",
			Value: []interface{}{
				map[string]interface{}{
					"base": "urax",
					"denom_units": []interface{}{
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "urax",
							"exponent": "0",
						},
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "rax",
							"exponent": "18",
						},
					},
					"description": "Denom metadata for Rollapp EVM",
					"display":     "rax",
					"name":        "rax",
					"symbol":      "rax",
				},
			},
		},
	}

	rollappWasmGenesisKV = []cosmos.GenesisKV{
		// Bank denom metadata
		{
			Key: "app_state.bank.denom_metadata",
			Value: []interface{}{
				map[string]interface{}{
					"base": "urax",
					"denom_units": []interface{}{
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "urax",
							"exponent": "0",
						},
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "rax",
							"exponent": "18",
						},
					},
					"description": "Denom metadata for Rollapp Wasm",
					"display":     "rax",
					"name":        "rax",
					"symbol":      "rax",
				},
			},
		},
	}

	dymensionGenesisKV = []cosmos.GenesisKV{
		// gov params
		{
			Key:   "app_state.gov.voting_params.voting_period",
			Value: "20s",
		},
		// staking params
		{
			Key:   "app_state.staking.params.bond_denom",
			Value: "adym",
		},
		{
			Key:   "app_state.mint.params.mint_denom",
			Value: "adym",
		},
		// increase the tx size cost per byte from 10 to 100
		{
			Key:   "app_state.auth.params.tx_size_cost_per_byte",
			Value: "100",
		},
		// jail validators faster, and shorten recovery time, no slash for downtime
		{
			Key:   "app_state.slashing.params.signed_blocks_window",
			Value: "10000",
		},
		{
			Key:   "app_state.slashing.params.min_signed_per_window",
			Value: "0.800000000000000000",
		},
		{
			Key:   "app_state.slashing.params.downtime_jail_duration",
			Value: "120s",
		},
		{
			Key:   "app_state.slashing.params.slash_fraction_downtime",
			Value: "0.0",
		},
		// cometbft's updated values
		// MaxBytes: 4194304 - four megabytes
		// MaxGas:   10000000
		{
			Key:   "consensus_params.block.max_bytes",
			Value: "4194304",
		},
		{
			Key:   "consensus_params.block.max_gas",
			Value: "10000000",
		},
		// EVM params
		{
			Key:   "app_state.feemarket.params.no_base_fee",
			Value: true,
		},
		{
			Key:   "app_state.feemarket.params.min_gas_price",
			Value: "0",
		},
		{
			Key:   "app_state.evm.params.evm_denom",
			Value: "adym",
		},
		{
			Key:   "app_state.evm.params.enable_create",
			Value: false,
		},
		// Incentives params should be set to days on live net and lockable duration to 2 weeks
		{
			Key:   "app_state.incentives.params.distr_epoch_identifier",
			Value: "minute",
		},
		{
			Key:   "app_state.incentives.lockable_durations",
			Value: []string{"60s"},
		},
		// Misc params
		{
			Key:   "app_state.crisis.constant_fee.denom",
			Value: "adym",
		},
		{
			Key:   "app_state.txfees.basedenom",
			Value: "adym",
		},
		{
			Key:   "app_state.txfees.params.epoch_identifier",
			Value: "minute",
		},
		{
			Key:   "app_state.gamm.params.enable_global_pool_fees",
			Value: true,
		},
		// Bank denom metadata
		{
			Key: "app_state.bank.denom_metadata",
			Value: []interface{}{
				map[string]interface{}{
					"base": "adym",
					"denom_units": []interface{}{
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "adym",
							"exponent": "0",
						},
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "DYM",
							"exponent": "18",
						},
					},
					"description": "Denom metadata for DYM (adym)",
					"display":     "DYM",
					"name":        "DYM",
					"symbol":      "DYM",
				},
			},
		},
	}
)

func GetDockerImageVersion() (dymensionVersion, rollappEVMVersion, rollappWasmVersion, relayerVersion string) {
	dymensionVersion, found := os.LookupEnv("DYMENSION_CI")
	if !found {
		dymensionVersion = "latest"
	}

	rollappEVMVersion, found = os.LookupEnv("ROLLAPP_EVM_CI")
	if !found {
		rollappEVMVersion = "latest"
	}

	rollappWasmVersion, found = os.LookupEnv("ROLLAPP_WASM_CI")
	if !found {
		rollappWasmVersion = "latest"
	}
	relayerVersion, found = os.LookupEnv("RELAYER_CI")
	if !found {
		relayerVersion = "main-dym"
	}
	return dymensionVersion, rollappEVMVersion, rollappWasmVersion, relayerVersion
}

func GetUpgradeName() (upgradeName, upgradeEVMName, upgradeWasmName string) {
	upgradeName, found := os.LookupEnv("UPGRADE_NAME")
	if !found {
		upgradeName = ""
	}
	upgradeEVMName, found = os.LookupEnv("UPGRADE_ROLAPP_EVM_NAME")
	if !found {
		upgradeEVMName = ""
	}
	upgradeWasmName, found = os.LookupEnv("UPGRADE_ROLLAPP_WASM_NAME")
	if !found {
		upgradeWasmName = ""
	}
	return upgradeName, upgradeEVMName, upgradeWasmName
}

func GetPullRelayerImage() (pullRelayerImage bool) {
	pull, found := os.LookupEnv("RELAYER_CI")
	if !found {
		pullRelayerImage = true
	}
	if pull == "e2e" {
		pullRelayerImage = false
	}
	return pullRelayerImage
}

func encodingConfig() *simappparams.EncodingConfig {
	cfg := cosmos.DefaultEncoding()

	ethermint.RegisterInterfaces(cfg.InterfaceRegistry)
	ethermintcrypto.RegisterInterfaces(cfg.InterfaceRegistry)
	eibc.RegisterInterfaces(cfg.InterfaceRegistry)
	rollapp.RegisterInterfaces(cfg.InterfaceRegistry)
	hubgenesis.RegisterInterfaces(cfg.InterfaceRegistry)
	return &cfg
}

func defaultConfig() *simappparams.EncodingConfig {
	cfg := cosmos.DefaultEncoding()

	return &cfg
}

func modifyRollappEVMGenesis(genesisKV []cosmos.GenesisKV) func(ibc.ChainConfig, []byte) ([]byte, error) {
	return func(chainConfig ibc.ChainConfig, inputGenBz []byte) ([]byte, error) {
		g := make(map[string]interface{})
		if err := json.Unmarshal(inputGenBz, &g); err != nil {
			return nil, fmt.Errorf("failed to unmarshal genesis file: %w", err)
		}

		if err := dyno.Set(g, "10000000000", "app_state", "gov", "deposit_params", "min_deposit", 0, "amount"); err != nil {
			return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
		}

		outputGenBz, err := json.Marshal(g)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal genesis bytes to json: %w", err)
		}

		return cosmos.ModifyGenesis(genesisKV)(chainConfig, outputGenBz)
	}
}

func modifyRollappWasmGenesis(genesisKV []cosmos.GenesisKV) func(ibc.ChainConfig, []byte) ([]byte, error) {
	return func(chainConfig ibc.ChainConfig, inputGenBz []byte) ([]byte, error) {
		g := make(map[string]interface{})
		if err := json.Unmarshal(inputGenBz, &g); err != nil {
			return nil, fmt.Errorf("failed to unmarshal genesis file: %w", err)
		}

		if err := dyno.Set(g, "10000000000", "app_state", "gov", "deposit_params", "min_deposit", 0, "amount"); err != nil {
			return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
		}

		outputGenBz, err := json.Marshal(g)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal genesis bytes to json: %w", err)
		}

		return cosmos.ModifyGenesis(genesisKV)(chainConfig, outputGenBz)
	}
}

func modifyDymensionGenesis(genesisKV []cosmos.GenesisKV) func(ibc.ChainConfig, []byte) ([]byte, error) {
	return func(chainConfig ibc.ChainConfig, inputGenBz []byte) ([]byte, error) {
		g := make(map[string]interface{})
		if err := json.Unmarshal(inputGenBz, &g); err != nil {
			return nil, fmt.Errorf("failed to unmarshal genesis file: %w", err)
		}

		epochData, err := dyno.Get(g, "app_state", "epochs", "epochs")
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve epochs: %w", err)
		}
		epochs := epochData.([]interface{})
		exist := false
		// Check if the "minute" identifier already exists
		for _, epoch := range epochs {
			if epochMap, ok := epoch.(map[string]interface{}); ok {
				if epochMap["identifier"] == "minute" {
					exist = true
				}
			}
		}
		if !exist {
			// Define the new epoch type to be added
			newEpochType := map[string]interface{}{
				"identifier":                 "minute",
				"start_time":                 "0001-01-01T00:00:00Z",
				"duration":                   "60s",
				"current_epoch":              "0",
				"current_epoch_start_time":   "0001-01-01T00:00:00Z",
				"epoch_counting_started":     false,
				"current_epoch_start_height": "0",
			}

			// Add the new epoch to the epochs array
			updatedEpochs := append(epochs, newEpochType)
			if err := dyno.Set(g, updatedEpochs, "app_state", "epochs", "epochs"); err != nil {
				return nil, fmt.Errorf("failed to set epochs in genesis json: %w", err)
			}
		}
		if err := dyno.Set(g, "adym", "app_state", "gov", "deposit_params", "min_deposit", 0, "denom"); err != nil {
			return nil, fmt.Errorf("failed to set denom on gov min_deposit in genesis json: %w", err)
		}
		if err := dyno.Set(g, "10000000000", "app_state", "gov", "deposit_params", "min_deposit", 0, "amount"); err != nil {
			return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
		}
		if err := dyno.Set(g, "adym", "app_state", "gamm", "params", "pool_creation_fee", 0, "denom"); err != nil {
			return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
		}
		outputGenBz, err := json.Marshal(g)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal genesis bytes to json: %w", err)
		}

		return cosmos.ModifyGenesis(genesisKV)(chainConfig, outputGenBz)
	}
}

type rollappParam struct {
	rollappID, channelID, userKey string
}

// func triggerHubGenesisEvent(t *testing.T, dymension *dym_hub.DymHub, rollapps ...rollappParam) {
// 	ctx := context.Background()
// 	for i, r := range rollapps {
// 		keyDir := dymension.GetRollApps()[i].GetSequencerKeyDir()
// 		sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 		require.NoError(t, err)
// 		registerGenesisEventTriggerer(t, dymension.CosmosChain, r.userKey, sequencerAddr, "rollapp", "DeployerWhitelist")
// 		err = testutil.WaitForBlocks(ctx, 20, dymension)
// 		require.NoError(t, err)
// 		err = dymension.GetNode().TriggerGenesisEvent(ctx, "sequencer", r.rollappID, r.channelID, keyDir)
// 		require.NoError(t, err)
// 	}
// }

func registerGenesisEventTriggerer(t *testing.T, targetChain *cosmos.CosmosChain, userKey, address, module, param string) {
	ctx := context.Background()
	deployerWhitelistParams := json.RawMessage(fmt.Sprintf(`[{"address":"%s"}]`, address))
	propTx, err := targetChain.ParamChangeProposal(ctx, userKey, &utils.ParamChangeProposalJSON{
		Title:       "Add new deployer to whitelist",
		Description: "Add current user addr to the deployer whitelist",
		Changes: utils.ParamChangesJSON{
			utils.NewParamChangeJSON(module, param, deployerWhitelistParams),
		},
		Deposit: "500000000000" + targetChain.Config().Denom, // greater than min deposit
	})
	require.NoError(t, err)

	err = targetChain.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := targetChain.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, targetChain, height, height+30, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	newParams, err := targetChain.QueryParam(ctx, module, param)
	require.NoError(t, err)
	require.Equal(t, string(deployerWhitelistParams), newParams.Value)
}

func overridesDymintToml(settlemenLayer, nodeAddress, rollappId, gasPrices, maxIdleTime, maxProofTime, batchSubmitMaxTime string) map[string]any {
	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)

	dymintTomlOverrides["settlement_layer"] = settlemenLayer
	dymintTomlOverrides["settlement_node_address"] = nodeAddress
	dymintTomlOverrides["rollapp_id"] = rollappId
	dymintTomlOverrides["settlement_gas_prices"] = gasPrices
	dymintTomlOverrides["max_idle_time"] = maxIdleTime
	dymintTomlOverrides["max_proof_time"] = maxProofTime
	dymintTomlOverrides["batch_submit_max_time"] = batchSubmitMaxTime

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	return configFileOverrides
}

func CreateChannel(ctx context.Context, t *testing.T, r ibc.Relayer, eRep *testreporter.RelayerExecReporter, chainA, chainB *cosmos.CosmosChain, ibcPath string) {
	err := r.GeneratePath(ctx, eRep, chainA.Config().ChainID, chainB.Config().ChainID, ibcPath)
	require.NoError(t, err)

	err = r.CreateClients(ctx, eRep, ibcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 20, chainA, chainB)
	require.NoError(t, err)

	err = r.CreateConnections(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, chainA, chainB)
	require.NoError(t, err)

	err = r.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)
}
