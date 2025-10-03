package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"crypto/rand"
	"encoding/hex"

	"cosmossdk.io/math"
	util "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/decentrio/rollup-e2e-testing/blockdb"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	dymensiontesting "github.com/decentrio/rollup-e2e-testing/dymension"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/icza/dyno"
	"github.com/stretchr/testify/require"

	denommetadatatypes "github.com/dymensionxyz/dymension/v3/x/denommetadata/types"
	eibc "github.com/dymensionxyz/dymension/v3/x/eibc/types"
	rollapp "github.com/dymensionxyz/dymension/v3/x/rollapp/types"
	ethermintcrypto "github.com/evmos/ethermint/crypto/codec"
	ethermint "github.com/evmos/ethermint/types"
)

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
		fmt.Println("Err:", err)
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
	ibcPath                             = "dymension-demo"
	anotherIbcPath                      = "dymension-demo2"
	BLOCK_FINALITY_PERIOD               = 50
	EventDemandOrderCreated             = "dymensionxyz.dymension.eibc.EventDemandOrderCreated"
	EventDemandOrderFulfilled           = "dymensionxyz.dymension.eibc.EventDemandOrderFulfilled"
	EventDemandOrderFeeUpdated          = "dymensionxyz.dymension.eibc.EventDemandOrderFeeUpdated"
	EventDemandOrderPacketStatusUpdated = "dymensionxyz.dymension.eibc.EventDemandOrderPacketStatusUpdated"
)

var (
	walletAmount   = math.NewInt(100_000_000_000_000_000).MulRaw(100_000)
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

	eibcClientImage = ibc.DockerImage{
		Repository: "ghcr.io/decentrio/eibc-client",
		Version:    "latest",
		UidGid:     "1025:1025",
	}

	anvilImage = ibc.DockerImage{
		Repository: "ghcr.io/decentrio/anvil",
		Version:    "latest",
		UidGid:     "1025:1025",
	}

	hyperlaneImage = ibc.DockerImage{
		Repository: "ghcr.io/decentrio/hyperlane",
		// Version:    "arm",
		Version: "latest",
		UidGid:  "1025:1025",
	}

	hyperlaneAgentImage = ibc.DockerImage{
		Repository: "gcr.io/abacus-labs-dev/hyperlane-agent",
		Version:    "f009a0e-20250524-021447",
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
			Key:   "app_state.rollappparams.params.drs_version",
			Value: 7,
		},
		{
			Key:   "consensus_params.block.max_gas",
			Value: "400000000",
		},
		{
			Key:   "app_state.feemarket.params.no_base_fee",
			Value: true,
		},
		{
			Key:   "app_state.feemarket.params.min_gas_price",
			Value: "0.0",
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
			Key:   "app_state.evm.params.gas_denom",
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
		{
			Key:   "app_state.rollappparams.params.drs_version",
			Value: 9,
		},
		{
			Key:   "app_state.gov.voting_params.voting_period",
			Value: "30s",
		},
		{
			Key:   "consensus_params.block.max_gas",
			Value: "400000000",
		},
		// {
		// 	Key:   "app_state.feemarket.params.no_base_fee",
		// 	Value: true,
		// },
		// {
		// 	Key:   "app_state.feemarket.params.min_gas_price",
		// 	Value: "0.0",
		// },
		// {
		{
			Key:   "app_state.mint.params.mint_denom",
			Value: "urax",
		},
		{
			Key:   "app_state.staking.params.bond_denom",
			Value: "urax",
		},
		// {
		// 	Key:   "app_state.evm.params.evm_denom",
		// 	Value: "urax",
		// },
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
		{
			Key:   "app_state.mint.params.mint_denom",
			Value: "adym",
		},
		{
			Key:   "app_state.staking.params.bond_denom",
			Value: "adym",
		},
		{
			Key:   "app_state.evm.params.evm_denom",
			Value: "adym",
		},
		{
			Key:   "app_state.sequencer.params.notice_period",
			Value: "60s",
		},
		{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
		// gov params
		{
			Key:   "app_state.gov.params.voting_period",
			Value: "20s",
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
			Key:   "consensus.params.block.max_bytes",
			Value: "4194304",
		},
		{
			Key:   "consensus.params.block.max_gas",
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

func encodingConfig() *util.TestEncodingConfig {
	cfg := cosmos.DefaultEncoding()

	ethermint.RegisterInterfaces(cfg.InterfaceRegistry)
	ethermintcrypto.RegisterInterfaces(cfg.InterfaceRegistry)
	eibc.RegisterInterfaces(cfg.InterfaceRegistry)
	rollapp.RegisterInterfaces(cfg.InterfaceRegistry)
	// hubgenesis.RegisterInterfaces(cfg.InterfaceRegistry)
	return &cfg
}

func defaultConfig() *util.TestEncodingConfig {
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
		if err := dyno.Set(g, "adym", "app_state", "gov", "params", "min_deposit", 0, "denom"); err != nil {
			return nil, fmt.Errorf("failed to set denom on gov min_deposit in genesis json: %w", err)
		}
		// if err := dyno.Set(g, "1000000000000", "app_state", "rollapp", "params", "registration_fee", "amount"); err != nil {
		// 	return nil, fmt.Errorf("failed to set registration_fee in genesis json: %w", err)
		// }
		if err := dyno.Set(g, "10000000000", "app_state", "gov", "params", "min_deposit", 0, "amount"); err != nil {
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

// func registerGenesisEventTriggerer(t *testing.T, targetChain *cosmos.CosmosChain, userKey, address, module, param string) {
// 	ctx := context.Background()
// 	deployerWhitelistParams := json.RawMessage(fmt.Sprintf(`[{"address":"%s"}]`, address))
// 	propTx, err := targetChain.ParamChangeProposal(ctx, userKey, &utils.ParamChangeProposalJSON{
// 		Title:       "Add new deployer to whitelist",
// 		Description: "Add current user addr to the deployer whitelist",
// 		Changes: utils.ParamChangesJSON{
// 			utils.NewParamChangeJSON(module, param, deployerWhitelistParams),
// 		},
// 		Deposit: "500000000000" + targetChain.Config().Denom, // greater than min deposit
// 	})
// 	require.NoError(t, err)

// 	err = targetChain.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := targetChain.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	_, err = cosmos.PollForProposalStatus(ctx, targetChain, height, height+30, propTx.ProposalID, cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	newParams, err := targetChain.QueryParam(ctx, module, param)
// 	require.NoError(t, err)
// 	require.Equal(t, string(deployerWhitelistParams), newParams.Value)
// }

func overridesDymintToml(settlemenLayer, nodeAddress, rollappId, gasPrices, maxIdleTime, maxProofTime, batch_submit_time string, optionalConfigs ...bool) map[string]any {
	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)

	// Default values for optional fields
	includeDaGrpcLayer := false

	// Check if any options were passed and update the optional fields
	if len(optionalConfigs) > 0 {
		includeDaGrpcLayer = optionalConfigs[0]
	}

	if includeDaGrpcLayer {
		dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
		dymintTomlOverrides["da_layer"] = []string{"grpc"}
	} else {
		dymintTomlOverrides["da_config"] = []string{""}
		dymintTomlOverrides["da_layer"] = []string{"mock"}
	}

	dymintTomlOverrides["settlement_layer"] = settlemenLayer
	dymintTomlOverrides["settlement_node_address"] = nodeAddress
	dymintTomlOverrides["rollapp_id"] = rollappId
	dymintTomlOverrides["settlement_gas_prices"] = gasPrices
	dymintTomlOverrides["max_idle_time"] = maxIdleTime
	dymintTomlOverrides["max_proof_time"] = maxProofTime
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["batch_submit_time"] = batch_submit_time

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	return configFileOverrides
}

func CreateChannel(ctx context.Context, t *testing.T, r ibc.Relayer, eRep *testreporter.RelayerExecReporter, chainA, chainB *cosmos.CosmosChain, ibcPath string) {
	err := r.GeneratePath(ctx, eRep, chainA.Config().ChainID, chainB.Config().ChainID, ibcPath)
	require.NoError(t, err)

	err = r.CreateClients(ctx, eRep, ibcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, chainA, chainB)
	require.NoError(t, err)

	err = r.CreateConnections(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, chainA, chainB)
	require.NoError(t, err)

	err = r.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

	err = r.GenesisBridge(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, chainA, chainB)
	require.NoError(t, err)
}

func customEpochConfig(epochDuration string) ibc.ChainConfig {
	// Custom dymension epoch for faster disconnection
	modifyGenesisKV := dymensionGenesisKV
	for i, kv := range modifyGenesisKV {
		if kv.Key == "app_state.incentives.params.distr_epoch_identifier" || kv.Key == "app_state.txfees.params.epoch_identifier" {
			modifyGenesisKV[i].Value = "custom"
		}
	}

	customDymensionConfig := ibc.ChainConfig{
		Type:           "hub-dym",
		Name:           "dymension",
		ChainID:        "dymension_100-1",
		Images:         []ibc.DockerImage{dymensionImage},
		Bin:            "dymd",
		Bech32Prefix:   "dym",
		Denom:          "adym",
		CoinType:       "60",
		GasPrices:      "0.0adym",
		EncodingConfig: encodingConfig(),
		GasAdjustment:  1.1,
		TrustingPeriod: "112h",
		NoHostMount:    false,
		ModifyGenesis: func(chainConfig ibc.ChainConfig, inputGenBz []byte) ([]byte, error) {
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
			// Check if the "custom" identifier already exists
			for _, epoch := range epochs {
				if epochMap, ok := epoch.(map[string]interface{}); ok {
					if epochMap["identifier"] == "custom" {
						exist = true
					}
				}
			}
			if !exist {
				// Define the new epoch type to be added
				newEpochType := map[string]interface{}{
					"identifier":                 "custom",
					"start_time":                 "0001-01-01T00:00:00Z",
					"duration":                   epochDuration,
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
			if err := dyno.Set(g, "adym", "app_state", "gov", "params", "min_deposit", 0, "denom"); err != nil {
				return nil, fmt.Errorf("failed to set denom on gov min_deposit in genesis json: %w", err)
			}
			if err := dyno.Set(g, "10000000000", "app_state", "gov", "params", "min_deposit", 0, "amount"); err != nil {
				return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
			}
			if err := dyno.Set(g, "adym", "app_state", "gamm", "params", "pool_creation_fee", 0, "denom"); err != nil {
				return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
			}
			// if err := dyno.Set(g, "1000000000000", "app_state", "rollapp", "params", "registration_fee", "amount"); err != nil {
			// 	return nil, fmt.Errorf("failed to set registration_fee in genesis json: %w", err)
			// }
			outputGenBz, err := json.Marshal(g)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal genesis bytes to json: %w", err)
			}

			return cosmos.ModifyGenesis(modifyGenesisKV)(chainConfig, outputGenBz)
		},
		ConfigFileOverrides: nil,
	}

	return customDymensionConfig
}

func RandomHex(numberOfBytes int) (string, error) {
	bytes := make([]byte, numberOfBytes)

	// Read random bytes from crypto/rand
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}

	// Encode the bytes as a hex string
	hexString := hex.EncodeToString(bytes)

	return hexString, nil
}

func GetFaucet(api, address string) {
	// Data to send in the POST request
	data := map[string]string{
		"address": address,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	// Create a new POST request
	req, err := http.NewRequest("POST", api, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// Set the request header to indicate that we're sending JSON data
	req.Header.Set("Content-Type", "application/json")

	// Create an HTTP client and send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}

	fmt.Println("Response Status:", resp.Status)
	fmt.Println("Response Body:", string(body))

	if resp.Status != "200 OK" {
		time.Sleep(15 * time.Second)
		GetFaucet(api, address)
	}
}

func GetLatestBlockHeight(url, headerKey, headerValue string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add(headerKey, headerValue)
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func getEibcEventFromTx(t *testing.T, dymension *dym_hub.DymHub, txhash string) *dymensiontesting.EibcEvent {
	txResp, err := dymension.GetTransaction(txhash)
	if err != nil {
		require.NoError(t, err)
		return nil
	}

	events := txResp.Events

	var (
		id, _           = cosmos.AttributeValue(events, EventDemandOrderFulfilled, "order_id")
		price, _        = cosmos.AttributeValue(events, EventDemandOrderFulfilled, "price")
		fee, _          = cosmos.AttributeValue(events, EventDemandOrderFulfilled, "fee")
		isFulfilled, _  = cosmos.AttributeValue(events, EventDemandOrderFulfilled, "is_fulfilled")
		packetStatus, _ = cosmos.AttributeValue(events, EventDemandOrderFulfilled, "packet_status")
	)

	eibcEvent := new(dymensiontesting.EibcEvent)
	eibcEvent.OrderId = id
	eibcEvent.Price = price
	eibcEvent.Fee = fee
	eibcEvent.IsFulfilled, err = strconv.ParseBool(isFulfilled)
	if err != nil {
		require.NoError(t, err)
		return nil
	}
	eibcEvent.PacketStatus = packetStatus

	return eibcEvent
}

func getEIbcEventsWithinBlockRange(
	ctx context.Context,
	dymension *dym_hub.DymHub,
	blockRange int64,
	breakOnFirstOccurence bool,
) ([]dymensiontesting.EibcEvent, error) {
	var eibcEventsArray []dymensiontesting.EibcEvent

	height, err := dymension.Height(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Dymension height: %w", err)
	}
	fmt.Printf("Dymension height: %d\n", height)

	eibcEvents, err := getEibcEventsOfType(dymension.CosmosChain, height-10, height+blockRange, breakOnFirstOccurence)
	if err != nil {
		return nil, fmt.Errorf("error getting events of type 'eibc': %w", err)
	}

	if len(eibcEvents) == 0 {
		return nil, fmt.Errorf("There wasn't a single 'eibc' event registered within the specified block range on the hub")
	}

	for _, event := range eibcEvents {
		eibcEvent, err := dymensiontesting.MapToEibcEvent(event)
		if err != nil {
			println("go to here man")
			return nil, fmt.Errorf("error mapping to EibcEvent: %w", err)
		}
		eibcEventsArray = append(eibcEventsArray, eibcEvent)
	}

	return eibcEventsArray, nil
}

func areSlicesEqual(slice1, slice2 []blockdb.EventAttribute) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	for i := range slice1 {
		if slice1[i] != slice2[i] {
			return false
		}
	}

	return true
}

func contains(slice []blockdb.Event, item blockdb.Event) bool {
	for _, v := range slice {
		if areSlicesEqual(v.Attributes, item.Attributes) {
			return true
		}
	}
	return false
}

func getEibcEventsOfType(chain *cosmos.CosmosChain, startHeight int64, endHeight int64, breakOnFirstOccurence bool) ([]blockdb.Event, error) {
	var eventTypeArray []blockdb.Event
	shouldReturn := false

	for height := startHeight; height <= endHeight && !shouldReturn; height++ {
		err := testutil.WaitForBlocks(context.Background(), 1, chain)
		if err != nil {
			return nil, fmt.Errorf("error waiting for blocks: %w", err)
		}

		txs, err := chain.FindTxs(context.Background(), height)
		if err != nil {
			return nil, fmt.Errorf("error fetching transactions at height %d: %w", height, err)
		}

		for _, tx := range txs {
			for _, event := range tx.Events {
				if event.Type == EventDemandOrderCreated {
					if !contains(eventTypeArray, event) {
						eventTypeArray = append(eventTypeArray, event)
					}
					if breakOnFirstOccurence {
						shouldReturn = true
						fmt.Printf("%s event found on block height: %d", event.Type, height)
						break
					}
				}
			}
			if shouldReturn {
				break
			}
		}
	}

	return eventTypeArray, nil
}

func BuildEIbcMemo(eibcFee math.Int) string {
	return fmt.Sprintf(`{"eibc": {"fee": "%s"}}`, eibcFee.String())
}
func CheckInvariant(t *testing.T, ctx context.Context, dymension *dym_hub.DymHub, keyName string) {
	_, err := dymension.GetNode().CrisisInvariant(ctx, keyName, "eibc", "demand-order-count")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, keyName, "eibc", "underlying-packet-exist")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, keyName, "rollapp", "rollapp-count")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, keyName, "rollapp", "block-height-to-finalization-queue")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, keyName, "rollapp", "rollapp-by-eip155-key")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, keyName, "rollapp", "rollapp-finalized-state")
	require.NoError(t, err)
}
