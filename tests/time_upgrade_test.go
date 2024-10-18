package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/x/params/client/utils"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

func Test_TimeBaseUpgrade_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["block_time"] = "10s"

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

	// Relayer Factory
	client, network := test.DockerSetup(t)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		})

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil, false, 780)
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 1, "should have 1 sequences")

	// Assuming block height is located in status["SyncInfo"]["latest_block_height"]
	rollappHeightBeforeUpgrade, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err, "Failed to query Rollapp1 height before upgrade")
	fmt.Printf("Rollapp1 current block height before upgrade version: %d\n", rollappHeightBeforeUpgrade)

	// Change gov upgrade version
	newBridgeFeeParam := json.RawMessage(`"0"`)
	_, err = dymension.GetNode().ParamChangeProposal(ctx, dymensionUser.KeyName(),
		&utils.ParamChangeProposalJSON{
			Title:       "Change bridge fee params",
			Description: "Change bridge fee params",
			Changes: utils.ParamChangesJSON{
				utils.NewParamChangeJSON("delayedack", "BridgeFee", newBridgeFeeParam),
			},
			Deposit: "500000000000" + dymension.Config().Denom, // greater than min deposit
		})
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	// Wait for chain halt within 10 second
	startTime := time.Now()
	chainIsBackOnline := false

	for time.Since(startTime) < 10*time.Second {
		_, err := rollapp1.GetNode().Height(ctx)
		if err == nil {
			chainIsBackOnline = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.True(t, chainIsBackOnline, "Chain did not come back online within 10 seconds after upgrade")

	rollappHeightAfterUpgrade, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err, "Failed to query Rollapp1 height after upgrade")
	fmt.Printf("Rollapp1 current block height after upgrade version: %d\n", rollappHeightAfterUpgrade)
	require.Greater(t, rollappHeightAfterUpgrade, rollappHeightBeforeUpgrade, "Block height after upgrade should be greater than before upgrade")

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))
}
