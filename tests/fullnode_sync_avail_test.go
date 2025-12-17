package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

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

const (
	// AvailTuringRPCEndpoint is the public RPC endpoint for Avail Turing testnet
	AvailTuringRPCEndpoint = "https://avail-turing-rpc.publicnode.com"

	// AvailAppID is the application ID for blob submission on Avail
	AvailAppID = 1

	// Number of batches to submit before checking sync
	AvailTestBatchCount = 5
)

// TestFullnodeSync_Avail_EVM tests the synchronization of a fullnode using Avail as DA.
// This test submits batches to Avail and verifies the fullnode can sync from DA.
func TestFullnodeSync_Avail_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Check Avail balance before starting the test
	t.Logf("Checking Avail balance for address: %s", AvailAddress)
	CheckAvailBalance(t, AvailAddress)

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "30s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	// Avail DA configuration (uses AvailMnemonic from setup.go)
	da_config := []string{fmt.Sprintf(`{"endpoint": "%s", "app_id": %d, "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
		AvailTuringRPCEndpoint, AvailAppID, AvailMnemonic)}

	dymintTomlOverrides["da_layer"] = []string{"avail"}
	dymintTomlOverrides["da_config"] = da_config

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "avail",
		},
	)

	// Create chain factory
	numHubVals := 1
	numHubFullNodes := 0
	numRollAppFn := 1
	numRollAppVals := 1

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
				ModifyGenesis:       modifyRollappEVMGenesis(modifyEVMGenesisKV),
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	// Docker setup
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
	}, nil, "", nil, false, 1179360, true)
	require.NoError(t, err)

	t.Log("Chains started, waiting for rollapp to produce blocks and submit batches to Avail...")

	// Wait for enough blocks to ensure multiple batches are submitted
	// batch_submit_time is 30s, so wait for sufficient time
	targetBlocks := 50
	err = testutil.WaitForBlocks(ctx, targetBlocks, rollapp1)
	require.NoError(t, err)

	// Get the current validator height (this is the target for fullnode sync)
	targetHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)
	t.Logf("Validator reached height: %d", targetHeight)

	// Stop the sequencer to prevent more batches from being submitted
	t.Log("Stopping sequencer...")
	err = rollapp1.Validators[0].StopContainer(ctx)
	require.NoError(t, err)
	t.Log("Sequencer stopped, waiting for fullnode to sync...")

	// Poll until fullnode syncs to the target height
	err = testutil.WaitForCondition(
		time.Minute*10,
		time.Second*5,
		func() (bool, error) {
			fullnodeHeight, err := rollapp1.FullNodes[0].Height(ctx)
			if err != nil {
				t.Logf("Error getting fullnode height: %v", err)
				return false, nil
			}

			t.Logf("Fullnode height: %d / Target: %d", fullnodeHeight, targetHeight)

			// Fullnode should catch up to at least the target height
			if fullnodeHeight >= targetHeight {
				return true, nil
			}

			return false, nil
		},
	)
	require.NoError(t, err)

	finalHeight, err := rollapp1.FullNodes[0].Height(ctx)
	require.NoError(t, err)
	t.Logf("Fullnode successfully synced to height %d using Avail DA", finalHeight)
}
