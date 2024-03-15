package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/math"
	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	dymensiontypes "github.com/decentrio/rollup-e2e-testing/dymension"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type ExtractedInfo struct {
	CreationHeight string   `json:"creationHeight"` // hub height
	StartHeight    string   `json:"startHeight"`    // rollap start height finalized
	NumBlocks      string   `json:"numBlocks"`      // num of rollapp blocks finalized in a batch
	Heights        []string `json:"heights"`        // rollapp heights finalized
}

// This test verifies the system's behavior for batch finalization with different dispute periods
// Dispute period is updated with a gov proposal during the test
func TestBatchFinalization(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["gas_prices"] = "0adym"

	const BLOCK_FINALITY_PERIOD = 50
	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
	)

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1
	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "rollappevm_1234-1",
				Images:              []ibc.DockerImage{rollappImage},
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
			Name: "dymension-hub",
			ChainConfig: ibc.ChainConfig{
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
				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
				ConfigFileOverrides: nil,
			},
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
		SkipPathCreation: false,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	})
	require.NoError(t, err)

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	// rollapp height should be finalized in a batch processed > 50 hub blocks (dispute period)
	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	IsAnyRollappStateFinalized(ctx, dymension, rollapp1.GetChainID(), 300)
	require.NoError(t, err)

	lastFinalizedRollappHeight, err := dymension.FinalizedRollappStateHeight(ctx, rollapp1.GetChainID())
	require.NoError(t, err)
	fmt.Println(lastFinalizedRollappHeight)

	currentFinalizedRollappDymHeight, err := dymension.FinalizedRollappDymHeight(ctx, rollapp1.GetChainID())
	require.NoError(t, err)
	fmt.Println(currentFinalizedRollappDymHeight)

	// verify that the last creation height for finalized states on the hub is greater than dispute period
	// also, last finalized rollapp state height needs to be greater than the rollapp height verified
	require.True(t, (currentFinalizedRollappDymHeight > BLOCK_FINALITY_PERIOD) && (lastFinalizedRollappHeight > rollappHeight),
		fmt.Sprintf("Mismatch in batch finalization check. Current finalization hub height: %d. Dispute period: %d. Last finalized rollapp height: %d. Rollapp height asserted: %d",
			currentFinalizedRollappDymHeight, BLOCK_FINALITY_PERIOD, lastFinalizedRollappHeight, rollappHeight))

	// rollappState, err := dymension.QueryRollappState(ctx, rollapp1.GetChainID(), true)
	// require.NoError(t, err)

	// extractedInfo, err := ValidateAndExtract(*rollappState)
	// require.NoError(t, err)
	// fmt.Println(extractedInfo)
}

func IsAnyRollappStateFinalized(ctx context.Context, dymension *dym_hub.DymHub, rollappChainID string, timeoutSecs int) (bool, error) {
	var err error
	startTime := time.Now()
	timeout := time.Duration(timeoutSecs) * time.Second

	for {
		select {
		case <-time.After(timeout):
			return false, fmt.Errorf("timeout reached without rollap state finalization")
		default:
			var rollappState *dymensiontypes.RollappState
			rollappState, err = dymension.QueryRollappState(ctx, rollappChainID, true)
			if err != nil {
				if time.Since(startTime) < timeout {
					time.Sleep(2 * time.Second) // 2sec interval
					continue
				}
			}

			if rollappState != nil {
				return true, nil
			}
		}
	}
}

func ValidateAndExtract(state dymensiontypes.RollappState) (*ExtractedInfo, error) {
	if state.StateInfo.Status != "FINALIZED" {
		return nil, fmt.Errorf("No finalized status in the rollapp state info. The status was %s", state.StateInfo.Status)
	}

	var extracted ExtractedInfo
	extracted.CreationHeight = state.StateInfo.CreationHeight
	extracted.StartHeight = state.StateInfo.StartHeight
	extracted.NumBlocks = state.StateInfo.NumBlocks
	for _, bd := range state.StateInfo.BlockDescriptors.BD {
		extracted.Heights = append(extracted.Heights, bd.Height)
	}

	return &extracted, nil
}
