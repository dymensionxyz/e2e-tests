package tests

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"cosmossdk.io/math"
	// transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
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

func TestHardFork_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
	)

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
			Name: "dymension-hub",
			ChainConfig: ibc.ChainConfig{
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
				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
				ConfigFileOverrides: nil,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
			ExtraFlags:    nil,
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
	).Build(t, client, "relayer1", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r, "relayer1").
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
	}, nil, "", nil)
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)

	// IBC channel for rollapps
	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym1, 1)

	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channDymRollApp1 := channsRollApp1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	channsRollApp1Dym := channsRollApp1[0]
	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

	// Start relayer
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err = r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Get the IBC denom for urax on Hub
	rollappIbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIbcDenom, transferAmount.Sub(bridgingFee))

	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
	// Get the IBC denom
	dymToRollappIbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

	// Get origin rollapp1 denom balance
	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
	require.NoError(t, err)

	// IBC Transfer working between Dymension <-> rollapp1
	transferDataFromDym := ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	var options ibc.TransferOptions
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymToRollappIbcDenom, transferData.Amount)
	// verified ibc transfers worked

	// Create some pending eIBC packet
	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	// IBC Transfer working between rollapp1 <-> Dymension
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIbcDenom, transferAmount.Sub(bridgingFee))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)

	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.DemandOrders))

	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)

	// Access the index value
	index := oldLatestIndex.StateIndex.Index
	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)

	targetIndex := oldUintIndex + 1

	// Loop until the latest index updates
	for {
		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
		require.NoError(t, err)

		index := latestIndex.StateIndex.Index
		uintIndex, err := strconv.ParseUint(index, 10, 64)
		require.NoError(t, err)

		if uintIndex >= targetIndex {
			break
		}
	}

	submitFraudStr := "fraud"
	deposit := "500000000000" + dymension.Config().Denom

	// Get new height after frozen
	rollappHeight, err = rollapp1.Height(ctx)
	require.NoError(t, err)

	fraudHeight := fmt.Sprint(rollappHeight - 5)

	dymClients, err := r.GetClients(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 2, len(dymClients))

	var rollapp1ClientOnDym string

	for _, client := range dymClients {
		if client.ClientState.ChainID == rollapp1.Config().ChainID {
			rollapp1ClientOnDym = client.ClientID
		}
	}

	// Submit fraud proposal
	propTx, err := dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), rollapp1.Config().ChainID, fraudHeight, sequencerAddr, rollapp1ClientOnDym, submitFraudStr, submitFraudStr, deposit)
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	testutil.WaitForBlocks(ctx, 30, dymension, rollapp1)
	// after Grace period, the latest index should be the same
	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

	// Check if rollapp1 has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

	// Check rollapp1 state index not increment
	require.NoError(t, err)
	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")
	// stop all nodes and override genesis with new state
	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)
	// get the latest hight was finalized
	rollappState, err := dymension.QueryRollappState(ctx, rollapp1.Config().ChainID, true)
	require.NoError(t, err)

	lastHeightFinalized := rollappState.StateInfo.BlockDescriptors.BD[len(rollappState.StateInfo.BlockDescriptors.BD)-1].Height
	height, err = strconv.ParseInt(lastHeightFinalized, 10, 64)
	require.NoError(t, err)

	// export genesis
	stateOfOldRollApp, err := rollapp1.ExportState(ctx, int64(height))
	require.NoError(t, err)

	stateOfOldRollApp = strings.Split(stateOfOldRollApp, "\n")[0]
	genesis := strings.Replace(stateOfOldRollApp, "rollappevm_1234-1", "rollappevm_1234-2", 10)

	// setup new rollapp with the same rollapp and increase revision number
	// setup config for rollapp 1

	new_rollapp_id := "rollappevm_1234-2"
	configFileOverrides = overridesDymintToml(settlement_layer_rollapp, settlement_node_address, new_rollapp_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	cf = test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "new_rollapp",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp2",
				ChainID:             "rollappevm_1234-2",
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
	})

	chains, err = cf.Chains(t.Name())
	require.NoError(t, err)

	dymension.RemoveRollApp(rollapp1)
	// check that we removed old rollapp
	for _, rollapp := range dymension.GetRollApps() {
		require.True(t, rollapp.(ibc.Chain).Config().ChainID == rollapp1.Config().ChainID)
	}

	newRollApp := chains[0].(*dym_rollapp.DymRollApp)

	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer2", network)

	ic = test.NewSetup().
		AddRollUp(dymension, newRollApp).
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  newRollApp,
			Relayer: r2,
			Path:    anotherIbcPath,
		})

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, dymension, new_rollapp_id, []byte(genesis))
	require.NoError(t, err)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, newRollApp.CosmosChain, anotherIbcPath)

	channsNewRollApp, err := r2.GetChannels(ctx, eRep, newRollApp.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsNewRollApp, 2)

	channDymNewRollApp := channsNewRollApp[1].Counterparty
	require.NotEmpty(t, channDymNewRollApp.ChannelID)

	channsNewRollAppDym := channsNewRollApp[1]
	require.NotEmpty(t, channsNewRollAppDym.ChannelID)

	// Create user account on new roll app
	users = test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, newRollApp)

	// Get our Bech32 encoded user addresses
	newRollAppUser := users[0]
	newRollAppUserAddr := newRollAppUser.FormattedAddress()
	// Get original account balance
	newRollAppOrigBal, err := newRollApp.GetBalance(ctx, newRollAppUserAddr, newRollApp.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, newRollAppOrigBal)

	// Start relayer
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	err = testutil.WaitForBlocks(ctx, 10, dymension, newRollApp)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   newRollApp.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = newRollApp.SendIBCTransfer(ctx, channsNewRollAppDym.ChannelID, newRollAppUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	newRollAppHeight, err := newRollApp.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, newRollApp, newRollAppUserAddr, newRollApp.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, newRollApp.GetChainID(), newRollAppHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Get the IBC denom for urax on Hub
	newRollAppIbcDenom := GetIBCDenom(channsNewRollAppDym.Counterparty.PortID, channsNewRollAppDym.Counterparty.ChannelID, newRollApp.Config().Denom)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, newRollAppIbcDenom, transferAmount.Sub(bridgingFee))

	// Get the IBC denom
	dymToNewRollappIbcDenom := GetIBCDenom(channsNewRollAppDym.PortID, channsNewRollAppDym.ChannelID, dymension.Config().Denom)

	// IBC Transfer working between Dymension <-> new roll app
	transferDataFromDym = ibc.WalletData{
		Address: newRollAppUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}
	// hub -> new roll app
	dymBalanceBefore, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)

	_, err = dymension.SendIBCTransfer(ctx, channDymNewRollApp.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, dymension, newRollApp)
	// check assets balance
	erc20MAcc, err = newRollApp.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, newRollApp, erc20MAccAddr, dymToNewRollappIbcDenom, transferDataFromDym.Amount)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymBalanceBefore.Sub(transferDataFromDym.Amount))

	// new roll app to hub
	transferDataFromNewRollApp := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   newRollApp.Config().Denom,
		Amount:  transferAmount,
	}

	newRollAppBalanceBefore, err := newRollApp.GetBalance(ctx, newRollAppUserAddr, newRollApp.Config().Denom)
	require.NoError(t, err)

	_, err = newRollApp.SendIBCTransfer(ctx, channsNewRollAppDym.ChannelID, newRollAppUserAddr, transferDataFromNewRollApp, ibc.TransferOptions{})
	require.NoError(t, err)

	newRollAppHeight, err = newRollApp.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, newRollApp.GetChainID(), newRollAppHeight, 400)
	require.NoError(t, err)
	require.True(t, isFinalized)

	multiplier = math.NewInt(1000)
	bridgeFee := transferAmount.Quo(multiplier)
	// check assets balance
	testutil.AssertBalance(t, ctx, newRollApp, newRollAppUserAddr, newRollApp.Config().Denom, newRollAppBalanceBefore.Sub(transferDataFromNewRollApp.Amount))
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, newRollAppIbcDenom, transferAmount.Add(transferAmount).Sub(bridgeFee).Sub(bridgeFee))
}

func TestHardFork_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappwasm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"

	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
	)

	configFileOverrides := overridesDymintToml(settlement_layer_rollapp, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

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
			Name: "dymension-hub",
			ChainConfig: ibc.ChainConfig{
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
				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
				ConfigFileOverrides: nil,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
			ExtraFlags:    nil,
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
	).Build(t, client, "relayer1", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r, "relayer1").
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
	}, nil, "", nil)
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)

	// IBC channel for rollapps
	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym1, 1)

	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channDymRollApp1 := channsRollApp1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	channsRollApp1Dym := channsRollApp1[0]
	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

	// Start relayer
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err = r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Get the IBC denom for urax on Hub
	rollappIbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIbcDenom, transferAmount.Sub(bridgingFee))

	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
	// Get the IBC denom
	dymToRollappIbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

	// Get origin rollapp1 denom balance
	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
	require.NoError(t, err)

	// IBC Transfer working between Dymension <-> rollapp1
	transferDataFromDym := ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	var options ibc.TransferOptions
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

	rollapp1UserUpdateBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
	require.NoError(t, err)

	require.Equal(t, true, rollapp1OriginBal1.Add(transferAmount).Equal(rollapp1UserUpdateBal), "rollapp balance did not change")
	// verified ibc transfers worked

	// Create some pending eIBC packet
	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	// IBC Transfer working between rollapp1 <-> Dymension
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIbcDenom, transferAmount.Sub(bridgingFee))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)

	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.DemandOrders))

	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)

	// Access the index value
	index := oldLatestIndex.StateIndex.Index
	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)

	targetIndex := oldUintIndex + 1

	// Loop until the latest index updates
	for {
		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
		require.NoError(t, err)

		index := latestIndex.StateIndex.Index
		uintIndex, err := strconv.ParseUint(index, 10, 64)
		require.NoError(t, err)

		if uintIndex >= targetIndex {
			break
		}
	}

	submitFraudStr := "fraud"
	deposit := "500000000000" + dymension.Config().Denom

	// Get new height after frozen
	rollappHeight, err = rollapp1.Height(ctx)
	require.NoError(t, err)

	fraudHeight := fmt.Sprint(rollappHeight - 5)

	dymClients, err := r.GetClients(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 2, len(dymClients))

	var rollapp1ClientOnDym string

	for _, client := range dymClients {
		if client.ClientState.ChainID == rollapp1.Config().ChainID {
			rollapp1ClientOnDym = client.ClientID
		}
	}

	// Submit fraud proposal
	propTx, err := dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), rollapp1.Config().ChainID, fraudHeight, sequencerAddr, rollapp1ClientOnDym, submitFraudStr, submitFraudStr, deposit)
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	testutil.WaitForBlocks(ctx, 30, dymension, rollapp1)
	// after Grace period, the latest index should be the same
	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

	// Check if rollapp1 has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

	// Check rollapp1 state index not increment
	require.NoError(t, err)
	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")
	// stop all nodes and override genesis with new state
	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)
	// get the latest hight was finalized
	rollappState, err := dymension.QueryRollappState(ctx, rollapp1.Config().ChainID, true)
	require.NoError(t, err)

	lastHeightFinalized := rollappState.StateInfo.BlockDescriptors.BD[len(rollappState.StateInfo.BlockDescriptors.BD)-1].Height
	height, err = strconv.ParseInt(lastHeightFinalized, 10, 64)
	require.NoError(t, err)

	// export genesis
	stateOfOldRollApp, err := rollapp1.ExportState(ctx, int64(height))
	require.NoError(t, err)

	stateOfOldRollApp = strings.Split(stateOfOldRollApp, "\n")[0]
	genesis := strings.Replace(stateOfOldRollApp, "rollappwasm_1234-1", "rollappwasm_1234-2", 10)

	// setup new rollapp with the same rollapp and increase revision number
	// setup config for rollapp 1

	new_rollapp_id := "rollappwasm_1234-2"
	configFileOverrides = overridesDymintToml(settlement_layer_rollapp, settlement_node_address, new_rollapp_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	cf = test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "new_rollapp",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "rollappwasm_1234-2",
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
	})

	chains, err = cf.Chains(t.Name())
	require.NoError(t, err)

	dymension.RemoveRollApp(rollapp1)
	// check that we removed old rollapp
	for _, rollapp := range dymension.GetRollApps() {
		require.True(t, rollapp.(ibc.Chain).Config().ChainID == rollapp1.Config().ChainID)
	}

	newRollApp := chains[0].(*dym_rollapp.DymRollApp)

	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/dymensionxyz/go-relayer", "main-dym", "100:1000"),
	).Build(t, client, "relayer2", network)

	ic = test.NewSetup().
		AddRollUp(dymension, newRollApp).
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  newRollApp,
			Relayer: r2,
			Path:    anotherIbcPath,
		})

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, dymension, new_rollapp_id, []byte(genesis))
	require.NoError(t, err)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, newRollApp.CosmosChain, anotherIbcPath)

	channsNewRollApp, err := r2.GetChannels(ctx, eRep, newRollApp.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsNewRollApp, 2)

	channDymNewRollApp := channsNewRollApp[1].Counterparty
	require.NotEmpty(t, channDymNewRollApp.ChannelID)

	channsNewRollAppDym := channsNewRollApp[1]
	require.NotEmpty(t, channsNewRollAppDym.ChannelID)

	// Create user account on new roll app
	users = test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, newRollApp)

	// Get our Bech32 encoded user addresses
	newRollAppUser := users[0]
	newRollAppUserAddr := newRollAppUser.FormattedAddress()
	// Get original account balance
	newRollAppOrigBal, err := newRollApp.GetBalance(ctx, newRollAppUserAddr, newRollApp.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, newRollAppOrigBal)

	// Start relayer
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	err = testutil.WaitForBlocks(ctx, 10, dymension, newRollApp)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   newRollApp.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = newRollApp.SendIBCTransfer(ctx, channsNewRollAppDym.ChannelID, newRollAppUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	newRollAppHeight, err := newRollApp.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, newRollApp, newRollAppUserAddr, newRollApp.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, newRollApp.GetChainID(), newRollAppHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Get the IBC denom
	newRollAppIbcDenom := GetIBCDenom(channsNewRollAppDym.Counterparty.PortID, channsNewRollAppDym.Counterparty.ChannelID, newRollApp.Config().Denom)
	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, newRollAppIbcDenom, transferAmount.Sub(bridgingFee))

	// Get the IBC denom
	dymToNewRollappIbcDenom := GetIBCDenom(channsNewRollAppDym.PortID, channsNewRollAppDym.ChannelID, dymension.Config().Denom)

	// IBC Transfer working between Dymension <-> new roll app
	transferDataFromDym = ibc.WalletData{
		Address: newRollAppUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}
	// hub -> new roll app
	dymBalanceBefore, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)

	_, err = dymension.SendIBCTransfer(ctx, channDymNewRollApp.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, dymension, newRollApp)
	// check assets balance
	testutil.AssertBalance(t, ctx, newRollApp, newRollAppUserAddr, dymToNewRollappIbcDenom, transferDataFromDym.Amount)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymBalanceBefore.Sub(transferDataFromDym.Amount))

	// new roll app to hub
	transferDataFromNewRollApp := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   newRollApp.Config().Denom,
		Amount:  transferAmount,
	}

	newRollAppBalanceBefore, err := newRollApp.GetBalance(ctx, newRollAppUserAddr, newRollApp.Config().Denom)
	require.NoError(t, err)

	_, err = newRollApp.SendIBCTransfer(ctx, channsNewRollAppDym.ChannelID, newRollAppUserAddr, transferDataFromNewRollApp, ibc.TransferOptions{})
	require.NoError(t, err)

	newRollAppHeight, err = newRollApp.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, newRollApp.GetChainID(), newRollAppHeight, 400)
	require.NoError(t, err)
	require.True(t, isFinalized)

	multiplier = math.NewInt(1000)
	bridgeFee := transferAmount.Quo(multiplier)
	// check assets balance
	testutil.AssertBalance(t, ctx, newRollApp, newRollAppUserAddr, newRollApp.Config().Denom, newRollAppBalanceBefore.Sub(transferDataFromNewRollApp.Amount))
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, newRollAppIbcDenom, transferAmount.Add(transferAmount).Sub(bridgeFee).Sub(bridgeFee))
}

func TestHardForkRecoverIbcClient_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	// Custom dymension epoch for faster disconnection
	modifyGenesisKV := append(
		dymModifyGenesisKV,
		[]cosmos.GenesisKV{
			{
				Key:   "app_state.rollapp.params.dispute_period_in_blocks",
				Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
			},
		}...,
	)

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
			Name: "dymension-hub",
			ChainConfig: ibc.ChainConfig{
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
				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
				ConfigFileOverrides: nil,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
			ExtraFlags:    nil,
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
	).Build(t, client, "relayer1", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r, "relayer1").
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
	}, nil, "", nil)
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)

	// IBC channel for rollapps
	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym1, 1)

	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channDymRollApp1 := channsRollApp1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	channsRollApp1Dym := channsRollApp1[0]
	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

	// Start relayer
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err = r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Get the IBC denom for urax on Hub
	rollappIbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIbcDenom, transferAmount.Sub(bridgingFee))

	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
	// Get the IBC denom
	dymToRollappIbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

	// Get origin rollapp1 denom balance
	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
	require.NoError(t, err)

	// IBC Transfer working between Dymension <-> rollapp1
	transferDataFromDym := ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	var options ibc.TransferOptions
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymToRollappIbcDenom, transferData.Amount)

	// verified ibc transfers worked

	// Create some pending eIBC packet
	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	// IBC Transfer working between rollapp1 <-> Dymension
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIbcDenom, transferAmount.Sub(bridgingFee))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)

	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.DemandOrders))

	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)

	// Access the index value
	index := oldLatestIndex.StateIndex.Index
	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)

	targetIndex := oldUintIndex + 1

	// Loop until the latest index updates
	for {
		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
		require.NoError(t, err)

		index := latestIndex.StateIndex.Index
		uintIndex, err := strconv.ParseUint(index, 10, 64)
		require.NoError(t, err)

		if uintIndex >= targetIndex {
			break
		}
	}

	submitFraudStr := "fraud"
	deposit := "500000000000" + dymension.Config().Denom

	// Get new height after frozen
	rollappHeight, err = rollapp1.Height(ctx)
	require.NoError(t, err)

	fraudHeight := fmt.Sprint(rollappHeight - 5)

	dymClients, err := r.GetClients(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 2, len(dymClients))

	var rollapp1ClientOnDym string

	for _, client := range dymClients {
		if client.ClientState.ChainID == rollapp1.Config().ChainID {
			rollapp1ClientOnDym = client.ClientID
		}
	}

	// Submit fraud proposal
	propTx, err := dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), rollapp1.Config().ChainID, fraudHeight, sequencerAddr, rollapp1ClientOnDym, submitFraudStr, submitFraudStr, deposit)
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	testutil.WaitForBlocks(ctx, 30, dymension, rollapp1)
	// after Grace period, the latest index should be the same
	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

	// Check if rollapp1 has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

	// Check rollapp1 state index not increment
	require.NoError(t, err)
	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")
	// stop all nodes and override genesis with new state
	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)
	// get the latest hight was finalized
	rollappState, err := dymension.QueryRollappState(ctx, rollapp1.Config().ChainID, true)
	require.NoError(t, err)

	lastHeightFinalized := rollappState.StateInfo.BlockDescriptors.BD[len(rollappState.StateInfo.BlockDescriptors.BD)-1].Height
	height, err = strconv.ParseInt(lastHeightFinalized, 10, 64)
	require.NoError(t, err)

	// export genesis
	stateOfOldRollApp, err := rollapp1.ExportState(ctx, int64(height))
	require.NoError(t, err)

	stateOfOldRollApp = strings.Split(stateOfOldRollApp, "\n")[0]
	genesis := strings.Replace(stateOfOldRollApp, "rollappevm_1234-1", "rollappevm_1234-2", 10)

	// setup new rollapp with the same rollapp and increase revision number
	// setup config for rollapp 1

	new_rollapp_id := "rollappevm_1234-2"
	configFileOverrides = overridesDymintToml(settlement_layer_rollapp, settlement_node_address, new_rollapp_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	cf = test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "new_rollapp",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp2",
				ChainID:             "rollappevm_1234-2",
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
	})

	chains, err = cf.Chains(t.Name())
	require.NoError(t, err)

	dymension.RemoveRollApp(rollapp1)
	// check that we removed old rollapp
	for _, rollapp := range dymension.GetRollApps() {
		require.True(t, rollapp.(ibc.Chain).Config().ChainID == rollapp1.Config().ChainID)
	}

	newRollApp := chains[0].(*dym_rollapp.DymRollApp)

	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer2", network)

	ic = test.NewSetup().
		AddRollUp(dymension, newRollApp).
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  newRollApp,
			Relayer: r2,
			Path:    anotherIbcPath,
		})

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, dymension, new_rollapp_id, []byte(genesis))
	require.NoError(t, err)

	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, newRollApp.CosmosChain, anotherIbcPath)

	channsNewRollApp, err := r2.GetChannels(ctx, eRep, newRollApp.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsNewRollApp, 2)

	channDymNewRollApp := channsNewRollApp[1].Counterparty
	require.NotEmpty(t, channDymNewRollApp.ChannelID)

	channsNewRollAppDym := channsNewRollApp[1]
	require.NotEmpty(t, channsNewRollAppDym.ChannelID)

	// Create user account on new roll app
	users = test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, newRollApp)

	// Get our Bech32 encoded user addresses
	newRollAppUser := users[0]
	newRollAppUserAddr := newRollAppUser.FormattedAddress()
	// Get original account balance
	newRollAppOrigBal, err := newRollApp.GetBalance(ctx, newRollAppUserAddr, newRollApp.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, newRollAppOrigBal)

	// Start relayer
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	err = testutil.WaitForBlocks(ctx, 10, dymension, newRollApp)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   newRollApp.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = newRollApp.SendIBCTransfer(ctx, channsNewRollAppDym.ChannelID, newRollAppUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	newRollAppHeight, err := newRollApp.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, newRollApp, newRollAppUserAddr, newRollApp.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, newRollApp.GetChainID(), newRollAppHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Get the IBC denom for urax on Hub
	newRollAppIbcDenom := GetIBCDenom(channsNewRollAppDym.Counterparty.PortID, channsNewRollAppDym.Counterparty.ChannelID, newRollApp.Config().Denom)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, newRollAppIbcDenom, transferAmount.Sub(bridgingFee))

	// check client before submit update client proposal
	clientStatus, err := dymension.GetNode().QueryClientStatus(ctx, "07-tendermint-0")
	require.NoError(t, err)
	require.Equal(t, "Frozen", clientStatus.Status)

	propTx, err = dymension.SubmitUpdateClientProposal(ctx, dymensionUser.KeyName(), "07-tendermint-0", "07-tendermint-1", deposit)
	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err = dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	// check client after submit update client proposal
	clientStatus, err = dymension.GetNode().QueryClientStatus(ctx, "07-tendermint-0")
	require.NoError(t, err)
	require.Equal(t, "Active", clientStatus.Status)

	// Get the IBC denom
	dymToNewRollappIbcDenom := GetIBCDenom(channsNewRollAppDym.PortID, channsNewRollAppDym.ChannelID, dymension.Config().Denom)

	// IBC Transfer working between Dymension <-> new roll app
	transferDataFromDym = ibc.WalletData{
		Address: newRollAppUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}
	// hub -> new roll app
	dymBalanceBefore, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)

	_, err = dymension.SendIBCTransfer(ctx, channDymNewRollApp.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, dymension, newRollApp)
	// check assets balance
	erc20MAcc, err = newRollApp.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, newRollApp, erc20MAccAddr, dymToNewRollappIbcDenom, transferDataFromDym.Amount)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymBalanceBefore.Sub(transferDataFromDym.Amount))

	// new roll app to hub
	transferDataFromNewRollApp := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   newRollApp.Config().Denom,
		Amount:  transferAmount,
	}

	newRollAppBalanceBefore, err := newRollApp.GetBalance(ctx, newRollAppUserAddr, newRollApp.Config().Denom)
	require.NoError(t, err)

	_, err = newRollApp.SendIBCTransfer(ctx, channsNewRollAppDym.ChannelID, newRollAppUserAddr, transferDataFromNewRollApp, ibc.TransferOptions{})
	require.NoError(t, err)

	newRollAppHeight, err = newRollApp.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, newRollApp.GetChainID(), newRollAppHeight, 400)
	require.NoError(t, err)
	require.True(t, isFinalized)

	multiplier = math.NewInt(1000)
	bridgeFee := transferAmount.Quo(multiplier)
	// check assets balance
	testutil.AssertBalance(t, ctx, newRollApp, newRollAppUserAddr, newRollApp.Config().Denom, newRollAppBalanceBefore.Sub(transferDataFromNewRollApp.Amount))
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, newRollAppIbcDenom, transferAmount.Add(transferAmount).Sub(bridgeFee).Sub(bridgeFee))
}
