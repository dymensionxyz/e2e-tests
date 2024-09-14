package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	haltHeightDelta    = int64(20)
	blocksAfterUpgrade = int64(10)
)

var (
	// baseChain is the current version of the chain that will be upgraded from
	baseChain = ibc.DockerImage{
		Repository: "ghcr.io/dymensionxyz/dymension",
		Version:    "latest",
		UidGid:     "1025:1025",
	}
)

func TestHubUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "10s"
	maxProofTime := "500ms"
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime)

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime)
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 0
	numRollAppFn := 0
	numRollAppVals := 1

	// Create chain factory with dymension

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
				ConfigFileOverrides: configFileOverrides1,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp2",
				ChainID:             "decentrio_12345-1",
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
				ConfigFileOverrides: configFileOverrides2,
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

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	// relayer for rollapp 1
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r1, "relayer1").
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r1,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: r2,
			Path:    anotherIbcPath,
		})

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: false,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil)
	require.NoError(t, err)

	// err = dymension.StopAllNodes(ctx)
	// require.NoError(t, err)

	// path := "/home/ubuntu/genesis.json"
	// state, err := os.ReadFile(path)
	// require.NoError(t, err)
	// for _, node := range dymension.Nodes() {
	// 	err := node.OverwriteGenesisFile(ctx, state)
	// 	require.NoError(t, err)
	// }

	// for _, node := range dymension.Nodes() {
	// 	_, _, err = node.ExecBin(ctx, "tendermint", "unsafe-reset-all")
	// 	require.NoError(t, err)
	// }

	// _ = dymension.StartAllNodes(ctx)

	//Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension, dymension, rollapp1, rollapp2)

	// Get our Bech32 encoded user addresses
	dymensionUser1, dymensionUser2, marketmaker1, marketmaker2, rollappUser1, rollappUser2 := users[0], users[1], users[2], users[3], users[4], users[5]

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	dymensionUser1Addr := dymensionUser1.FormattedAddress()
	dymensionUser2Addr := dymensionUser2.FormattedAddress()
	marketMaker1Addr := marketmaker1.FormattedAddress()
	marketMaker2Addr := marketmaker2.FormattedAddress()
	rollappUser1Addr := rollappUser1.FormattedAddress()
	rollappUser2Addr := rollappUser2.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUser1Addr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, dymensionUser2Addr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMaker1Addr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMaker2Addr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUser1Addr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp2, rollappUser2Addr, rollapp2.Config().Denom, walletAmount)

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

	upgradeTx, err := dymension.UpgradeLegacyProposal(ctx, dymensionUser1.KeyName(), proposal)
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
	// dymension.UpgradeVersion(ctx, client, DymensionMainRepo, "v4.0.0")
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

	// Get dymension -> rollapp1 channel
	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channRollApp1Dym := channsRollApp1[0]
	require.NotEmpty(t, channRollApp1Dym.ChannelID)
	channDymRollApp1 := channRollApp1Dym.Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)
	println("port dym rollapp1: ", channDymRollApp1.PortID)
	println("channel dym rollapp1: ", channDymRollApp1.ChannelID)
	// Get dymension -> rollapp2 channel
	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp2, 1)
	channRollApp2Dym := channsRollApp2[0]
	require.NotEmpty(t, channRollApp2Dym.ChannelID)
	channDymRollApp2 := channRollApp2Dym.Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	println("port dym rollapp2: ", channDymRollApp2.PortID)
	println("channel dym rollapp2: ", channDymRollApp2.ChannelID)
	// Trigger genesis event for rollapp1
	// rollapp1Param := rollappParam{
	// 	rollappID: rollapp1.Config().ChainID,
	// 	channelID: channDymRollApp1.ChannelID,
	// 	userKey:   dymensionUser1.KeyName(),
	// }

	// rollapp2Param := rollappParam{
	// 	rollappID: rollapp2.Config().ChainID,
	// 	channelID: channDymRollApp2.ChannelID,
	// 	userKey:   dymensionUser2.KeyName(),
	// }
	// triggerHubGenesisEvent(t, dymension, rollapp1Param, rollapp2Param)

	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	//SET UP TEST FOR ROLLAPP 1
	// ibc transfer from hub to rollapp1
	transferData := ibc.WalletData{
		Address: rollappUser1Addr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}
	rollapp1Height, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Compose an IBC transfer and send from hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUser1Addr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUser1Addr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 400)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom
	dymensionTokenDenom1 := transfertypes.GetPrefixedDenom(channDymRollApp1.PortID, channDymRollApp1.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom1 := transfertypes.ParseDenomTrace(dymensionTokenDenom1).IBCDenom()

	// check assets balance
	testutil.AssertBalance(t, ctx, rollapp1, rollappUser1Addr, dymensionIBCDenom1, transferData.Amount)
	testutil.AssertBalance(t, ctx, dymension, dymensionUser1Addr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	// MAKE eIBC TRANSFER
	// Get the IBC denom for urax on Hub
	rollapp1TokenDenom := transfertypes.GetPrefixedDenom(channRollApp1Dym.PortID, channRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollapp1IBCDenom := transfertypes.ParseDenomTrace(rollapp1TokenDenom).IBCDenom()

	var options ibc.TransferOptions

	multiplier := math.NewInt(10)
	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	transferAmountWithoutFee := transferAmount.Sub(eibcFee)
	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	// Send packet from rollapp1 -> hub market maker
	transferData = ibc.WalletData{
		Address: marketMaker1Addr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channRollApp1Dym.ChannelID, rollappUser1Addr, transferData, options)
	require.NoError(t, err)
	rollapp1Height, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized on Rollapp 1
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 400)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	expMmBalanceRollapp1Denom := transferData.Amount
	balance, err := dymension.GetBalance(ctx, marketMaker1Addr, rollapp1IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMaker1Addr after preconditions:", balance)
	require.True(t, balance.Equal(expMmBalanceRollapp1Denom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollapp1Denom, balance))

	transferData = ibc.WalletData{
		Address: dymensionUser1Addr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	// eibc transfer from hub to rollapp1
	_, err = rollapp1.SendIBCTransfer(ctx, channRollApp1Dym.ChannelID, rollappUser1Addr, transferData, options)
	require.NoError(t, err)

	rollapp1Height, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)
	zeroBalance := math.NewInt(0)
	balance, err = dymension.GetBalance(ctx, dymensionUser1Addr, rollapp1IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUser1Addr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(zeroBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBalance, balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	fmt.Println("Events:", eibcEvents)

	_, err = dymension.FullfillDemandOrder(ctx, eibcEvents[len(eibcEvents)-1].OrderId, marketMaker1Addr, eibcFee)
	require.NoError(t, err)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUser1Addr, rollapp1IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUser1Addr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmount.Sub(eibcFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee, balance))

	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMaker1Addr, rollapp1IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMaker1Addr after fulfilling the order:", balance)
	expMmBalanceRollapp1Denom = expMmBalanceRollapp1Denom.Sub(transferAmountWithoutFee)
	require.True(t, balance.Equal(expMmBalanceRollapp1Denom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollapp1Denom, balance))

	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMaker1Addr, rollapp1IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMaker1Addr after packet finalization:", balance)
	expMmBalanceRollapp1Denom = expMmBalanceRollapp1Denom.Add(transferData.Amount)
	require.True(t, balance.Equal(expMmBalanceRollapp1Denom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollapp1Denom, balance))

	//SET UP TEST FOR ROLLAPP 2
	// ibc transfer from hub to rollapp2
	transferData = ibc.WalletData{
		Address: rollappUser2Addr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}
	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// Compose an IBC transfer and send from hub -> rollapp 2
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp2.ChannelID, dymensionUser2Addr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUser2Addr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))
	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom
	dymensionTokenDenom2 := transfertypes.GetPrefixedDenom(channRollApp2Dym.PortID, channRollApp2Dym.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom2 := transfertypes.ParseDenomTrace(dymensionTokenDenom2).IBCDenom()
	println("check ibc denom 2: ", dymensionIBCDenom2)
	println("check addres rollapp2 user: ", rollappUser2Addr)
	// check assets balance
	testutil.AssertBalance(t, ctx, dymension, dymensionUser2Addr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, rollapp2, rollappUser2Addr, dymensionIBCDenom2, transferData.Amount)

	// MAKE eIBC TRANSFER
	// Get the IBC denom for urax on Hub
	rollapp2TokenDenom := transfertypes.GetPrefixedDenom(channDymRollApp2.PortID, channDymRollApp2.ChannelID, rollapp2.Config().Denom)
	rollapp2IBCDenom := transfertypes.ParseDenomTrace(rollapp2TokenDenom).IBCDenom()

	// Send packet from rollapp2 -> hub market maker
	transferData = ibc.WalletData{
		Address: marketMaker2Addr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp2.SendIBCTransfer(ctx, channRollApp2Dym.ChannelID, rollappUser2Addr, transferData, options)
	require.NoError(t, err)
	rollapp2Height, err = rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized on Rollapp 2
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	expMmBalanceRollapp2Denom := transferData.Amount
	balance, err = dymension.GetBalance(ctx, marketMaker2Addr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMaker2Addr after preconditions:", balance)
	require.True(t, balance.Equal(expMmBalanceRollapp2Denom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollapp2Denom, balance))

	transferData = ibc.WalletData{
		Address: dymensionUser2Addr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}
	// eibc transfer from hub to rollapp2
	_, err = rollapp2.SendIBCTransfer(ctx, channRollApp2Dym.ChannelID, rollappUser2Addr, transferData, options)
	require.NoError(t, err)

	rollapp2Height, err = rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)
	balance, err = dymension.GetBalance(ctx, dymensionUser2Addr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUser2Addr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// get eIbc event
	eibcEvents, err = getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	fmt.Println("Events:", eibcEvents)

	_, err = dymension.FullfillDemandOrder(ctx, eibcEvents[len(eibcEvents)-1].OrderId, marketMaker2Addr, eibcFee)
	require.NoError(t, err)
	// eibcEvent = getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUser2Addr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUser2Addr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmount.Sub(eibcFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee, balance))

	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMaker2Addr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMaker2Addr after fulfilling the order:", balance)
	expMmBalanceRollapp2Denom = expMmBalanceRollapp2Denom.Sub(transferAmountWithoutFee)
	require.True(t, balance.Equal(expMmBalanceRollapp2Denom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollapp2Denom, balance))

	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMaker2Addr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMaker2Addr after packet finalization:", balance)
	expMmBalanceRollapp2Denom = expMmBalanceRollapp2Denom.Add(transferData.Amount)
	require.True(t, balance.Equal(expMmBalanceRollapp2Denom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollapp2Denom, balance))
	// channel, err := ibc.GetTransferChannel(ctx, r1, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> rollapp
	// err = dymension.IBCTransfer(ctx,
	// 	dymension, rollapp1, transferAmount, dymensionUserAddr,
	// 	rollappUserAddr, r1, ibcPath, channel,
	// 	eRep, ibc.TransferOptions{})
	// require.NoError(t, err)
	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
			err = r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}
