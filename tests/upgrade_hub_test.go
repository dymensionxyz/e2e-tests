package tests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	upgradeName = "v3"

	haltHeightDelta    = uint64(20)
	blocksAfterUpgrade = uint64(10)
)

var (
	// baseChain is the current version of the chain that will be upgraded from
	baseChain = ibc.DockerImage{
		Repository: "ghcr.io/dymensionxyz/dymension",
		Version:    "3.1.0",
		UidGid:     "1025:1025",
	}
)

func TestHubUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// // setup config for rollapp 1
	// settlement_layer_rollapp1 := "dymension"
	// node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	// rollapp1_id := "rollappevm_1234-1"
	// gas_price_rollapp1 := "0adym"
	// emptyBlocksMaxTime := "3s"
	// configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, node_address, rollapp1_id, gas_price_rollapp1, emptyBlocksMaxTime)

	// // setup config for rollapp 2
	// settlement_layer_rollapp2 := "dymension"
	// rollapp2_id := "rollappwasm_12345-1"
	// gas_price_rollapp2 := "0adym"
	// configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, node_address, rollapp2_id, gas_price_rollapp2, emptyBlocksMaxTime)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 0
	// numRollAppFn := 0
	// numRollAppVals := 1

	// Create chain factory with dymension

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		// {
		// 	Name: "rollapp1",
		// 	ChainConfig: ibc.ChainConfig{
		// 		Type:                "rollapp-dym",
		// 		Name:                "rollapp-temp",
		// 		ChainID:             "rollappevm_1234-1",
		// 		Images:              []ibc.DockerImage{rollappEVMImage},
		// 		Bin:                 "rollappd",
		// 		Bech32Prefix:        "ethm",
		// 		Denom:               "urax",
		// 		CoinType:            "60",
		// 		GasPrices:           "0.0urax",
		// 		GasAdjustment:       1.1,
		// 		TrustingPeriod:      "112h",
		// 		EncodingConfig:      encodingConfig(),
		// 		NoHostMount:         false,
		// 		ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
		// 		ConfigFileOverrides: configFileOverrides1,
		// 	},
		// 	NumValidators: &numRollAppVals,
		// 	NumFullNodes:  &numRollAppFn,
		// },
		// {
		// 	Name: "rollapp2",
		// 	ChainConfig: ibc.ChainConfig{
		// 		Type:                "rollapp-dym",
		// 		Name:                "rollapp-temp2",
		// 		ChainID:             "rollappwasm_12345-1",
		// 		Images:              []ibc.DockerImage{rollappWasmImage},
		// 		Bin:                 "rollappd",
		// 		Bech32Prefix:        "rol",
		// 		Denom:               "urax",
		// 		CoinType:            "118",
		// 		GasPrices:           "0.0urax",
		// 		GasAdjustment:       1.1,
		// 		TrustingPeriod:      "112h",
		// 		EncodingConfig:      encodingConfig(),
		// 		NoHostMount:         false,
		// 		ModifyGenesis:       nil,
		// 		ConfigFileOverrides: configFileOverrides2,
		// 	},
		// 	NumValidators: &numRollAppVals,
		// 	NumFullNodes:  &numRollAppFn,
		// },
		{
			Name: "dymension-hub",
			ChainConfig: ibc.ChainConfig{
				Type:                "hub-dym",
				Name:                "dymension",
				ChainID:             "dymension_100-1",
				Images:              []ibc.DockerImage{baseChain},
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
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})
	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	// rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	// rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[0].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	// // relayer for rollapp 1
	// r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
	// 	relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	// ).Build(t, client, "relayer1", network)
	// // relayer for rollapp 2
	// r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
	// 	relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	// ).Build(t, client, "relayer2", network)

	ic := test.NewSetup().AddChain(dymension)
	// AddRollUp(dymension, rollapp1, rollapp2).
	// AddRelayer(r1, "relayer1").
	// AddRelayer(r2, "relayer2").
	// AddLink(test.InterchainLink{
	// 	Chain1:  dymension,
	// 	Chain2:  rollapp1,
	// 	Relayer: r1,
	// 	Path:    ibcPath,
	// }).
	// AddLink(test.InterchainLink{
	// 	Chain1:  dymension,
	// 	Chain2:  rollapp2,
	// 	Relayer: r2,
	// 	Path:    anotherIbcPath,
	// })

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

	err = dymension.StopAllNodes(ctx)
	require.NoError(t, err)

	path := "/home/ubuntu/dym.json"
	state, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, node := range dymension.Nodes() {
		err := node.OverwriteGenesisFile(ctx, state)
		require.NoError(t, err)
	}

	for _, node := range dymension.Nodes() {
		_, _, err = node.ExecBin(ctx, "tendermint", "unsafe-reset-all")
		require.NoError(t, err)
	}

	_ = dymension.StartAllNodes(ctx)

	// Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()
	// Make sure gov params has changed
	dymNode := dymension.FullNodes[0]
	votingParams, err := dymNode.QueryParam(ctx, "gov", "voting_params")
	require.NoError(t, err)
	fmt.Println("gov voting params: ", votingParams.Value)

	// Create some user accounts on both chains
	user := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension)[0]

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height before submit upgrade proposal")

	haltHeight := height + haltHeightDelta

	proposal := cosmos.SoftwareUpgradeProposal{
		Deposit:     "500000000000" + dymension.Config().Denom, // greater than min deposit
		Title:       "Chain Upgrade 1",
		Name:        upgradeName,
		Description: "First chain software upgrade",
		Height:      haltHeight,
		Info:        "Info",
	}

	upgradeTx, err := dymension.UpgradeLegacyProposal(ctx, user.KeyName(), proposal)
	require.NoError(t, err, "error submitting software upgrade proposal tx")
	fmt.Println("upgradeTx", upgradeTx)

	err = dymension.VoteOnProposalAllValidators(ctx, upgradeTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, upgradeTx.ProposalID, cosmos.ProposalStatusPassed)
	prop, _ := dymension.QueryProposal(ctx, upgradeTx.ProposalID)
	fmt.Println("prop: ", prop)
	require.Equal(t, prop.Status, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed in expected number of blocks")

	timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, time.Second*45)
	defer timeoutCtxCancel()

	height, err = dymension.Height(ctx)
	require.NoError(t, err, "error fetching height before upgrade")

	// this should timeout due to chain halt at upgrade height.
	_ = testutil.WaitForBlocks(timeoutCtx, int(haltHeight-height)+1, dymension)

	height, err = dymension.Height(ctx)
	require.NoError(t, err, "error fetching height after chain should have halted")

	// make sure that chain is halted
	require.Equal(t, haltHeight, height, "height is not equal to halt height")

	// bring down nodes to prepare for upgrade
	err = dymension.StopAllNodes(ctx)
	require.NoError(t, err, "error stopping node(s)")

	// upgrade version on all nodes
	dymension.UpgradeVersion(ctx, client, DymensionMainRepo, dymensionVersion)

	// start all nodes back up.
	// validators reach consensus on first block after upgrade height
	// and chain block production resumes.
	err = dymension.StartAllNodes(ctx)
	require.NoError(t, err, "error starting upgraded node(s)")

	timeoutCtx, timeoutCtxCancel = context.WithTimeout(ctx, time.Second*45)
	defer timeoutCtxCancel()

	err = testutil.WaitForBlocks(timeoutCtx, int(blocksAfterUpgrade), dymension)
	require.NoError(t, err, "chain did not produce blocks after upgrade")

	height, err = dymension.Height(ctx)
	require.NoError(t, err, "error fetching height after upgrade")

	require.GreaterOrEqual(t, height, haltHeight+blocksAfterUpgrade, "height did not increment enough after upgrade")

	// channel, err := ibc.GetTransferChannel(ctx, r1, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> rollapp
	// err = dymension.IBCTransfer(ctx,
	// 	dymension, rollapp1, transferAmount, dymensionUserAddr,
	// 	rollappUserAddr, r1, ibcPath, channel,
	// 	eRep, ibc.TransferOptions{})
	// require.NoError(t, err)
}
