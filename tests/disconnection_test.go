package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/icza/dyno"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

func customConfig() ibc.ChainConfig {
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
					"duration":                   "5s",
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
			// if err := dyno.Set(g, "1000000000000", "app_state", "rollapp", "params", "registration_fee", "amount"); err != nil {
			// 	return nil, fmt.Errorf("failed to set registration_fee in genesis json: %w", err)
			// }
			if err := dyno.Set(g, "adym", "app_state", "gamm", "params", "pool_creation_fee", 0, "denom"); err != nil {
				return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
			}
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

func TestDisconnection_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["batch_submit_bytes"] = "1000"
	dymintTomlOverrides["block_batch_max_size_bytes"] = "1000"
	dymintTomlOverrides["max_batch_skew"] = "1"
	dymintTomlOverrides["batch_acceptance_attempts"] = "1"
	dymintTomlOverrides["batch_acceptance_timeout"] = "5s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 0

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "rollappevm_1234-1",
				Images:              []ibc.DockerImage{rollappEVMImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "ethm",
				Denom:               "urax",
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name:          "dymension-hub",
			ChainConfig:   customConfig(),
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1)

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil)
	require.NoError(t, err)

	// Wait for rollapp finalized
	rollapp1Height, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 300)

	t.Run("hub disconnect", func(t *testing.T) {
		err = dymension.StopAllNodes(ctx)
		require.NoError(t, err)

		// Wait until rollapp stops produce block
		rollappHeight, err := rollapp1.Height(ctx)
		require.NoError(t, err)

		err = testutil.WaitForCondition(
			time.Minute*10,
			time.Second*5, // each epoch is 5 seconds
			func() (bool, error) {
				newRollappHeight, err := rollapp1.Height(ctx)
				require.NoError(t, err)

				if newRollappHeight > rollappHeight {
					rollappHeight = newRollappHeight
					return false, nil
				}

				return true, nil
			},
		)
		require.NoError(t, err)

		err = dymension.StartAllNodes(ctx)
		require.NoError(t, err)

		// Make sure rollapp start pro
		err = testutil.WaitForBlocks(ctx, 1, rollapp1)
		require.NoError(t, err)
	})
}

func TestDisconnection_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "5s"
	dymintTomlOverrides["batch_submit_bytes"] = "1000"
	dymintTomlOverrides["block_batch_max_size_bytes"] = "1000"
	dymintTomlOverrides["max_batch_skew"] = "1"
	dymintTomlOverrides["batch_acceptance_attempts"] = "1"
	dymintTomlOverrides["batch_acceptance_timeout"] = "5s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 0

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "rollappwasm_1234-1",
				Images:              []ibc.DockerImage{rollappWasmImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "rol",
				Denom:               "urax",
				CoinType:            "118",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name:          "dymension-hub",
			ChainConfig:   customConfig(),
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1)

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil)
	require.NoError(t, err)

	// Wait for rollapp finalized
	rollapp1Height, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 300)

	t.Run("hub disconnect", func(t *testing.T) {
		err = dymension.StopAllNodes(ctx)
		require.NoError(t, err)

		// Wait until rollapp stops produce block
		rollappHeight, err := rollapp1.Height(ctx)
		require.NoError(t, err)

		err = testutil.WaitForCondition(
			time.Minute*10,
			time.Second*5, // each epoch is 5 seconds
			func() (bool, error) {
				newRollappHeight, err := rollapp1.Height(ctx)
				require.NoError(t, err)

				if newRollappHeight > rollappHeight {
					rollappHeight = newRollappHeight
					return false, nil
				}

				return true, nil
			},
		)
		require.NoError(t, err)

		err = dymension.StartAllNodes(ctx)
		require.NoError(t, err)

		// Make sure rollapp start produce blocks
		err = testutil.WaitForBlocks(ctx, 1, rollapp1)
		require.NoError(t, err)
	})
}
