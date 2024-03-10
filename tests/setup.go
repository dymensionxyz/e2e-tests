package tests

import (
	"encoding/json"
	"fmt"
	"os"

	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/icza/dyno"

	eibc "github.com/dymensionxyz/dymension/v3/x/eibc/types"
	rollapp "github.com/dymensionxyz/dymension/v3/x/rollapp/types"
	ethermintcrypto "github.com/evmos/ethermint/crypto/codec"
	ethermint "github.com/evmos/ethermint/types"
)

var (
	DymensionMainRepo = "ghcr.io/dymensionxyz/dymension"

	RollappMainRepo = "ghcr.io/dymensionxyz/rollapp"

	dymensionVersion, rollappVersion = GetDockerImageVersion()

	dymensionImage = ibc.DockerImage{
		Repository: DymensionMainRepo,
		Version:    dymensionVersion,
		UidGid:     "1025:1025",
	}

	rollappImage = ibc.DockerImage{
		Repository: "ghcr.io/decentrio/rollapp-evm",
		Version:    "e2e-amd",
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

	// Setup for Osmosis
	osmosisImageRepo = "ghcr.io/strangelove-ventures/heighliner/osmosis" //

	osmosisImage = ibc.DockerImage{
		Repository: osmosisImageRepo,
		UidGid:     "1025:1025",
	}

	osmosisConfig = ibc.ChainConfig{
		Type:                "cosmos",
		Name:                "osmosis",
		ChainID:             "osmosis-1",
		Images:              []ibc.DockerImage{osmosisImage},
		Bin:                 "osmosisd",
		Bech32Prefix:        "osmo",
		Denom:               "uosmo",
		CoinType:            "118",
		GasPrices:           "0.5uosmo",
		EncodingConfig:      defaultConfig(),
		GasAdjustment:       2,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       nil,
		ConfigFileOverrides: nil,
	}

	// IBC Path
	pathHubToRollApp = "hub-path"
	pathDymToOsmos   = "dym-osmo"

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
	}

	dymensionGenesisKV = []cosmos.GenesisKV{
		// gov params
		{
			Key:   "app_state.gov.voting_params.voting_period",
			Value: "1m",
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

func GetDockerImageVersion() (dymensionVersion, rollappVersion string) {
	dymensionVersion, found := os.LookupEnv("DYMENSION_CI")
	if !found {
		dymensionVersion = "latest"
	}

	rollappVersion, found = os.LookupEnv("ROLLAPP_CI")
	if !found {
		rollappVersion = "latest"
	}
	return dymensionVersion, rollappVersion
}

func encodingConfig() *simappparams.EncodingConfig {
	cfg := cosmos.DefaultEncoding()

	ethermint.RegisterInterfaces(cfg.InterfaceRegistry)
	ethermintcrypto.RegisterInterfaces(cfg.InterfaceRegistry)
	eibc.RegisterInterfaces(cfg.InterfaceRegistry)
	rollapp.RegisterInterfaces(cfg.InterfaceRegistry)
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
