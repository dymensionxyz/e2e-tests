package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/cosmos/cosmos-sdk/x/params/client/utils"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/icza/dyno"
	"github.com/stretchr/testify/require"

	hubgenesis "github.com/dymensionxyz/dymension-rdk/x/hub-genesis/types"
	eibc "github.com/dymensionxyz/dymension/v3/x/eibc/types"
	rollapp "github.com/dymensionxyz/dymension/v3/x/rollapp/types"
	ethermintcrypto "github.com/evmos/ethermint/crypto/codec"
	ethermint "github.com/evmos/ethermint/types"
)

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

var (
	DymensionMainRepo = "ghcr.io/dymensionxyz/dymension"

	RollappEVMMainRepo = "ghcr.io/dymensionxyz/rollapp-evm"

	RollappWasmMainRepo = "ghcr.io/dymensionxyz/rollapp-wasm"

	IBCRelayerImage   = "ghcr.io/decentrio/relayer"
	IBCRelayerVersion = "main"

	dymensionVersion, rollappEVMVersion, rollappWasmVersion = GetDockerImageVersion()

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

	dymensionConfig = ibc.ChainConfig{
		Type:                "hub-dym",
		Name:                "dymension",
		ChainID:             "dymension_100-1",
		Images:              []ibc.DockerImage{dymensionImage},
		Bin:                 "dymd",
		Bech32Prefix:        "dym",
		Denom:               "adym",
		CoinType:            "118",
		GasPrices:           "0.0adym",
		EncodingConfig:      encodingConfig(),
		GasAdjustment:       1.1,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
		ConfigFileOverrides: nil,
	}

	// Setup for gaia
	gaiaImageRepo = "ghcr.io/strangelove-ventures/heighliner/gaia" //

	gaiaImage = ibc.DockerImage{
		Repository: gaiaImageRepo,
		UidGid:     "1025:1025",
	}

	gaiaConfig = ibc.ChainConfig{
		Type:                "cosmos",
		Name:                "gaia",
		ChainID:             "gaia-1",
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

	// IBC Path
	pathHubToRollApp = "hub-path"
	pathDymToGaia    = "dym-gaia"

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
			Key:   "app_state.claims.params.claims_denom",
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
			Value: false,
		},
		{
			Key:   "app_state.erc20.params.enable_evm_hook",
			Value: false,
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

func GetDockerImageVersion() (dymensionVersion, rollappEVMVersion, rollappWasmVersion string) {
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
	return dymensionVersion, rollappEVMVersion, rollappWasmVersion
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

func registerGenesisEventTriggerer(t *testing.T, dymension *dym_hub.DymHub, user ibc.Wallet, module, param string) {
	ctx := context.Background()
	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)
	deployerWhitelistParams := json.RawMessage(fmt.Sprintf(`[{"address":"%s"}]`, sequencerAddr))
	propTx, err := dymension.ParamChangeProposal(ctx, user.KeyName(), &utils.ParamChangeProposalJSON{
		Title:       "Add new deployer to whitelist",
		Description: "Add current user addr to the deployer whitelist",
		Changes: utils.ParamChangesJSON{
			utils.NewParamChangeJSON(module, param, deployerWhitelistParams),
		},
		Deposit: "500000000000" + dymension.Config().Denom, // greater than min deposit
	})
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	new_params, err := dymension.QueryParam(ctx, "rollapp", "DeployerWhitelist")
	require.NoError(t, err)
	require.Equal(t, new_params.Value, string(deployerWhitelistParams))
}

func triggerGenesisEvent(t *testing.T, dymension *dym_hub.DymHub, rollappID, channelID string, user ibc.Wallet) {
	registerGenesisEventTriggerer(t, dymension, user, "rollapp", "DeployerWhitelist")
	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	err := dymension.GetNode().TriggerGenesisEvent(context.Background(), "sequencer", rollappID, channelID, keyDir)
	require.NoError(t, err)
}
