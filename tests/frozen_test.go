package tests

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"strconv"
// 	"strings"
// 	"testing"

// 	"cosmossdk.io/math"
// 	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
// 	"github.com/stretchr/testify/require"
// 	"go.uber.org/zap/zaptest"

// 	test "github.com/decentrio/rollup-e2e-testing"
// 	"github.com/decentrio/rollup-e2e-testing/cosmos"
// 	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
// 	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
// 	"github.com/decentrio/rollup-e2e-testing/ibc"
// 	"github.com/decentrio/rollup-e2e-testing/relayer"
// 	"github.com/decentrio/rollup-e2e-testing/testreporter"
// 	"github.com/decentrio/rollup-e2e-testing/testutil"
// )

// var extraFlags = map[string]interface{}{"genesis-accounts-path": true}

// // TestRollAppFreeze ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing.
// func TestRollAppFreeze_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappevm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "decentrio_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "3s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1

// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappevm_1234-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp2",
// 				ChainID:             "decentrio_12345-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	// relayer for rollapp 1
// 	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)
// 	// relayer for rollapp 2
// 	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r1, "relayer1").
// 		AddRelayer(r2, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r1,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: r2,
// 			Path:    anotherIbcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet1, found := r1.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	wallet2, found := r2.GetWallet(rollapp2.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
// 	require.NoError(t, err)
// 	keyPath2 := keyDir2 + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 2, dymension)
// 	require.NoError(t, err)

// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	// Start both relayers
// 	err = r1.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r1.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 			err = r2.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollappUser := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollappUserAddr := rollappUser.FormattedAddress()

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)

// 	// IBC channel for rollapps
// 	channsDym1, err := r1.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 2)

// 	channsDym2, err := r2.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym2, 2)

// 	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp2, 1)

// 	channDymRollApp2 := channsRollApp2[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp2.ChannelID)

// 	channsRollApp2Dym := channsRollApp2[0]
// 	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// Assert balance was updated on the hub
// 	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	uintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := uintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := oldLatestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)

// 		require.NoError(t, err)
// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	rollappHeight, err = rollapp1.Height(ctx)
// 	require.NoError(t, err)

// 	rollapp1Clients, err := r1.GetClients(ctx, eRep, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, 1, len(rollapp1Clients))

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	// Check if rollapp has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp state index not increment
// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// IBC Transfer not working
// 	// Compose an IBC transfer and send from dymension -> rollapp
// 	transferData = ibc.WalletData{
// 		Address: rollappUserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
// 	require.Error(t, err)
// 	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

// 	// Compose an IBC transfer and send from rollapp -> dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	// Get the IBC denom
// 	rollapp1Denom := transfertypes.GetPrefixedDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

// 	// Get origin dym hub ibc denom balance
// 	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 30, dymension)
// 	require.NoError(t, err)

// 	// Get updated dym hub ibc denom balance
// 	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	// IBC balance should not change
// 	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")

// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// // TestRollAppFreeze ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing.
// func TestRollAppFreeze_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappwasm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "decentrio_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "3s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappwasm_1234-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp2",
// 				ChainID:             "decentrio_12345-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	// relayer for rollapp 1
// 	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)
// 	// relayer for rollapp 2
// 	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r1, "relayer1").
// 		AddRelayer(r2, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r1,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: r2,
// 			Path:    anotherIbcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet1, found := r1.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	wallet2, found := r2.GetWallet(rollapp2.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
// 	require.NoError(t, err)
// 	keyPath2 := keyDir2 + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 2, dymension)
// 	require.NoError(t, err)

// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	// Start both relayers
// 	err = r1.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r1.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer : %s", err)
// 			}
// 			err = r2.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollappUser := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollappUserAddr := rollappUser.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	// IBC channel for rollapps
// 	channsDym1, err := r1.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 2)

// 	channsDym2, err := r2.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym2, 2)

// 	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp2, 1)

// 	channDymRollApp2 := channsRollApp2[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp2.ChannelID)

// 	channsRollApp2Dym := channsRollApp2[0]
// 	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// Assert balance was updated on the hub
// 	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	intIndex, err := strconv.ParseInt(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := intIndex + 1

// 	currentHeight, err := rollapp1.Height(ctx)
// 	require.NoError(t, err)

// 	_, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.Config().ChainID, currentHeight, 300)
// 	require.NoError(t, err)

// 	// oldLatestRollapp1, err := dymension.FinalizedRollappStateIndex(ctx, rollapp1.Config().ChainID)
// 	// require.NoError(t, err)

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	// Check if rollapp has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp state index not increment
// 	latestFinalizedIndex, err := dymension.FinalizedRollappStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, targetIndex, latestFinalizedIndex, "rollapp state index still increment")

// 	// IBC Transfer not working
// 	// Compose an IBC transfer and send from dymension -> rollapp
// 	transferData = ibc.WalletData{
// 		Address: rollappUserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
// 	require.Error(t, err)
// 	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

// 	// Compose an IBC transfer and send from rollapp -> dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	// Get the IBC denom
// 	rollapp1Denom := transfertypes.GetPrefixedDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

// 	// Get origin dym hub ibc denom balance
// 	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Wait a few blocks
// 	err = testutil.WaitForBlocks(ctx, 20, dymension)

// 	// Get updated dym hub ibc denom balance
// 	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	// IBC balance should not change
// 	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")
// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// // TestOtherRollappNotAffected_EVM ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing and other rollapp works fine.
// func TestOtherRollappNotAffected_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappevm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "10s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "decentrio_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "1s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappevm_1234-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp1",
// 				ChainID:             "decentrio_12345-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "118",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r, "relayer1").
// 		AddRelayer(s, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: s,
// 			Path:    anotherIbcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet1, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	wallet2, found := s.GetWallet(rollapp2.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
// 	require.NoError(t, err)
// 	keyPath2 := keyDir2 + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 2, dymension)
// 	require.NoError(t, err)

// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, s, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}

// 			err = s.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()
// 	rollapp2UserAddr := rollapp2User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollapp2OrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 2)

// 	channsDym2, err := s.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym2, 2)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	channsRollApp2, err := s.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp2, 1)

// 	channDymRollApp2 := channsRollApp2[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp2.ChannelID)

// 	channsRollApp2Dym := channsRollApp2[0]
// 	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp2.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}
// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	rollapp2Index, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp2.Config().ChainID)
// 	require.NoError(t, err)

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// Compose an IBC transfer and send from dymension -> rollapp
// 	transferData = ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	// Confirm IBC Transfer not working between Dymension <-> rollapp1
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
// 	require.Error(t, err)
// 	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	// Get the IBC denom
// 	rollapp1Denom := transfertypes.GetPrefixedDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

// 	// Get origin dym hub ibc denom balance
// 	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	// Wait a few blocks
// 	err = testutil.WaitForBlocks(ctx, 30, dymension)

// 	// Get updated dym hub ibc denom balance
// 	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	// IBC balance should not change
// 	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")

// 	// Check other rollapp state index still increase
// 	rollapp2IndexLater, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp2.Config().ChainID)
// 	fmt.Println(rollapp2IndexLater.StateIndex.Index)
// 	fmt.Println(rollapp2Index.StateIndex.Index)
// 	require.NoError(t, err)
// 	require.True(t, rollapp2IndexLater.StateIndex.Index > rollapp2Index.StateIndex.Index, "Another rollapp got freeze")

// 	// Get the IBC denom
// 	rollapp2IbcDenom := GetIBCDenom(channsRollApp2Dym.Counterparty.PortID, channsRollApp2Dym.Counterparty.ChannelID, rollapp2.Config().Denom)
// 	dymToRollapp2IbcDenom := GetIBCDenom(channsRollApp2Dym.PortID, channsRollApp2Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin dym hub ibc denom balance
// 	dymUserOriginBal2, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IbcDenom)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp2
// 	transferData = ibc.WalletData{
// 		Address: rollapp2UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp2.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp2)

// 	erc20MAcc, err := rollapp2.Validators[0].QueryModuleAccount(ctx, "erc20")
// 	require.NoError(t, err)
// 	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
// 	testutil.AssertBalance(t, ctx, rollapp2, erc20MAccAddr, dymToRollapp2IbcDenom, transferData.Amount)

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp2.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	tx, err := rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)
// 	require.NotEmpty(t, tx.TxHash, "tx is nil")

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp2)

// 	rollappHeight, err = rollapp2.Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp2)

// 	// Get updated dym hub ibc denom balance
// 	dymUserUpdateBal2, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserUpdateBal2.Equal(dymUserOriginBal2.Add(transferAmount).Sub(bridgingFee)), "dym hub balance did not change")
// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// // TestOtherRollappNotAffected_Wasm ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing and other rollapp works fine.
// func TestOtherRollappNotAffected_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	configFileOverrides := make(map[string]any)
// 	dymintTomlOverrides := make(testutil.Toml)
// 	dymintTomlOverrides["settlement_layer"] = "dymension"
// 	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
// 	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
// 	dymintTomlOverrides["max_idle_time"] = "3s"
// 	dymintTomlOverrides["max_proof_time"] = "500ms"
// 	dymintTomlOverrides["batch_submit_time"] = "50s"
// 	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

// 	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

// 	configFileOverrides2 := make(map[string]any)
// 	dymintTomlOverrides2 := make(testutil.Toml)
// 	dymintTomlOverrides2["settlement_layer"] = "dymension"
// 	dymintTomlOverrides2["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	dymintTomlOverrides2["rollapp_id"] = "decentrio_12345-1"
// 	dymintTomlOverrides2["settlement_gas_prices"] = "0adym"
// 	dymintTomlOverrides2["max_proof_time"] = "500ms"
// 	dymintTomlOverrides2["max_idle_time"] = "3s"
// 	dymintTomlOverrides2["batch_submit_time"] = "50s"
// 	dymintTomlOverrides2["p2p_blocksync_enabled"] = "false"

// 	configFileOverrides2["config/dymint.toml"] = dymintTomlOverrides2
// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappwasm_1234-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp1",
// 				ChainID:             "decentrio_12345-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r, "relayer1").
// 		AddRelayer(s, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: s,
// 			Path:    anotherIbcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet1, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	wallet2, found := s.GetWallet(rollapp2.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
// 	require.NoError(t, err)
// 	keyPath2 := keyDir2 + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 2, dymension)
// 	require.NoError(t, err)

// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, s, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}

// 			err = s.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()
// 	rollapp2UserAddr := rollapp2User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollapp2OrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 2)

// 	channsDym2, err := s.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym2, 2)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	channsRollApp2, err := s.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp2, 1)

// 	channDymRollApp2 := channsRollApp2[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp2.ChannelID)

// 	channsRollApp2Dym := channsRollApp2[0]
// 	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp2.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	rollapp2Index, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp2.Config().ChainID)
// 	require.NoError(t, err)

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// Compose an IBC transfer and send from dymension -> rollapp
// 	transferData = ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	// Confirm IBC Transfer not working between Dymension <-> rollapp1
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
// 	require.Error(t, err)
// 	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	// Get the IBC denom
// 	rollapp1Denom := transfertypes.GetPrefixedDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

// 	// Get origin dym hub ibc denom balance
// 	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	// Wait a few blocks
// 	err = testutil.WaitForBlocks(ctx, 20, dymension)

// 	// Get updated dym hub ibc denom balance
// 	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	// IBC balance should not change
// 	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")

// 	// Check other rollapp state index still increase
// 	rollapp2IndexLater, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp2.Config().ChainID)
// 	require.NoError(t, err)
// 	require.True(t, rollapp2IndexLater.StateIndex.Index > rollapp2Index.StateIndex.Index, "Another rollapp got freeze")

// 	// Get the IBC denom
// 	rollapp2IbcDenom := GetIBCDenom(channsRollApp2Dym.Counterparty.PortID, channsRollApp2Dym.Counterparty.ChannelID, rollapp2.Config().Denom)
// 	dymToRollapp2IbcDenom := GetIBCDenom(channsRollApp2Dym.PortID, channsRollApp2Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin dym hub ibc denom balance
// 	dymUserOriginBal2, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IbcDenom)
// 	require.NoError(t, err)

// 	rollapp2UserOriginBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, dymToRollapp2IbcDenom)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp2
// 	transferData = ibc.WalletData{
// 		Address: rollapp2UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp2.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	testutil.WaitForBlocks(ctx, 10, dymension)

// 	rollapp2UserUpdateBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, dymToRollapp2IbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, rollapp2UserUpdateBal.Sub(transferAmount).Equal(rollapp2UserOriginBal), "rollapp balance did not change")

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp2.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	tx, err := rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)
// 	require.NotEmpty(t, tx.TxHash, "tx is nil")

// 	testutil.WaitForBlocks(ctx, 10, dymension)

// 	rollapp2Height, err = rollapp2.Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	// Get updated dym hub ibc denom balance
// 	dymUserUpdateBal2, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IbcDenom)
// 	require.NoError(t, err)

// 	// Minus 0.1% for bridge fee
// 	require.Equal(t, true, dymUserUpdateBal2.Equal(dymUserOriginBal2.Add(transferAmount.Sub(transferAmount.Quo(math.NewInt(1000))))), "dym hub balance did not change")
// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// // TestPacketRollbacked_EVM ensure upon freeze gov proposal passed, ibc-transfer that not yet finalized's packets will be reverted.
// func TestPacketRollbacked_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappevm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappevm_1234-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "118",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddRelayer(r, "relayer1").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
// 	// Get the IBC denom
// 	rollapp1IbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	dymToRollapp1IbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin rollapp1 denom balance
// 	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollapp1IbcDenom)
// 	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp1
// 	transferDataFromDym := ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount.Quo(math.NewInt(4)),
// 	}

// 	var options ibc.TransferOptions
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	rollappHeight, err = rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
// 	require.NoError(t, err)
// 	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
// 	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymToRollapp1IbcDenom, transferDataFromDym.Amount)
// 	// verified ibc transfers worked

// 	// Create some pending eIBC packet
// 	multiplier := math.NewInt(10)

// 	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

// 	// set eIBC specific memo
// 	options.Memo = BuildEIbcMemo(eibcFee)

// 	// IBC Transfer working between rollapp1 <-> Dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
// 	require.NoError(t, err)
// 	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserRollapp1bal.Equal(transferAmount.Sub(bridgingFee)), "dym hub balance changed")

// 	// get eIbc event
// 	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
// 	require.NoError(t, err)

// 	for i, eibcEvent := range eibcEvents {
// 		fmt.Println(i, "EIBC Event:", eibcEvent)
// 	}

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	// SEND IBC transfer right before vote
// 	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension)
// 	require.NoError(t, err)

// 	rollappHeight, err = rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 600)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	err = testutil.WaitForBlocks(ctx, 10, dymension)
// 	require.NoError(t, err)

// 	// after Grace period, the latest index should be the same
// 	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// After rollapp frozen, fund should return
// 	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "funds aren't sent back to sender")
// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// // TestPacketRollbacked_Wasm ensure upon freeze gov proposal passed, ibc-transfer that not yet finalized's packets will be reverted.
// func TestPacketRollbacked_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappwasm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappwasm_1234-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddRelayer(r, "relayer1").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 1)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
// 	// Get the IBC denom
// 	rollapp1IbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	dymToRollapp1IbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin rollapp1 denom balance
// 	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollapp1IbcDenom)
// 	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp1
// 	transferDataFromDym := ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount.Quo(math.NewInt(4)),
// 	}

// 	var options ibc.TransferOptions
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	rollappHeight, err = rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	rollapp1UserUpdateBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollapp1IbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, rollapp1OriginBal1.Add(transferAmount.Quo(math.NewInt(4))).Equal(rollapp1UserUpdateBal), "rollapp balance did not change")
// 	// verified ibc transfers worked

// 	// Create some pending eIBC packet
// 	multiplier := math.NewInt(10)

// 	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

// 	// set eIBC specific memo
// 	options.Memo = BuildEIbcMemo(eibcFee)

// 	// IBC Transfer working between rollapp1 <-> Dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
// 	require.NoError(t, err)
// 	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserRollapp1bal.Equal(transferAmount.Sub(bridgingFee)), "dym hub balance changed")

// 	// get eIbc event
// 	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
// 	require.NoError(t, err)

// 	for i, eibcEvent := range eibcEvents {
// 		fmt.Println(i, "EIBC Event:", eibcEvent)
// 	}

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	// SEND IBC transfer right before vote
// 	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension)
// 	require.NoError(t, err)

// 	rollappHeight, err = rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	err = testutil.WaitForBlocks(ctx, 10, dymension)
// 	require.NoError(t, err)

// 	// after Grace period, the latest index should be the same
// 	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// After rollapp frozen, fund should return
// 	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "funds aren't sent back to sender")

// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// // TestRollAppFreezeNoBrokenInvariants_EVM ensure upon freeze gov proposal passed, no updates can be made to the rollapp evm. No invariants broken.
// func TestRollAppFreezeNoBrokenInvariants_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappevm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "decentrio_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "3s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappevm_1234-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp1",
// 				ChainID:             "decentrio_12345-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "118",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r, "relayer1").
// 		AddRelayer(s, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: s,
// 			Path:    anotherIbcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet1, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	wallet2, found := s.GetWallet(rollapp2.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
// 	require.NoError(t, err)
// 	keyPath2 := keyDir2 + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 2, dymension)
// 	require.NoError(t, err)

// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, s, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}

// 			err = s.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()
// 	rollapp2UserAddr := rollapp2User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollapp2OrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 2)

// 	channsDym2, err := s.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym2, 2)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	channsRollApp2, err := s.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp2, 1)

// 	channDymRollApp2 := channsRollApp2[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp2.ChannelID)

// 	channsRollApp2Dym := channsRollApp2[0]
// 	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp2.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// Run invariant check
// 	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "rollapp", "block-height-to-finalization-queue")
// 	require.NoError(t, err)

// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// // TestRollAppFreezeNoBrokenInvariants_Wasm ensure upon freeze gov proposal passed, no updates can be made to the rollapp wasm. No invariants broken.
// func TestRollAppFreezeNoBrokenInvariants_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappwasm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "decentrio_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "3s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappwasm_1234-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp2",
// 				ChainID:             "decentrio_12345-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "118",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r, "relayer1").
// 		AddRelayer(s, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: s,
// 			Path:    anotherIbcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet1, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	wallet2, found := s.GetWallet(rollapp2.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
// 	require.NoError(t, err)
// 	keyPath2 := keyDir2 + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 2, dymension)
// 	require.NoError(t, err)

// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, s, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}

// 			err = s.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()
// 	rollapp2UserAddr := rollapp2User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollapp2OrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 2)

// 	channsDym2, err := s.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym2, 2)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	channsRollApp2, err := s.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp2, 1)

// 	channDymRollApp2 := channsRollApp2[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp2.ChannelID)

// 	channsRollApp2Dym := channsRollApp2[0]
// 	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp2.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// Run invariant check
// 	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "rollapp", "block-height-to-finalization-queue")
// 	require.NoError(t, err)

// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// // TestRollAppSqcSlashedJailed_EVM ensure upon freeze gov proposal passed, Sequencer should be slashed with all the bond and jailed. Inability to register a new sequencer
// func TestRollAppSqcSlashedJailed_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappevm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "decentrio_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "3s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappevm_1234-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp1",
// 				ChainID:             "decentrio_12345-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r, "relayer1").
// 		AddRelayer(s, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: s,
// 			Path:    anotherIbcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet1, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	wallet2, found := s.GetWallet(rollapp2.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
// 	require.NoError(t, err)
// 	keyPath2 := keyDir2 + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 2, dymension)
// 	require.NoError(t, err)

// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, s, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}

// 			err = s.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()
// 	rollapp2UserAddr := rollapp2User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollapp2OrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)	SequencerOrigBal, err := dymension.GetBalance(ctx, rollapp2UserAddr, dymension.Config().Denom)
// 	// fmt.Println("SequencerOrigBal", SequencerOrigBal)

// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 2)

// 	channsDym2, err := s.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym2, 2)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	channsRollApp2, err := s.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp2, 1)

// 	channDymRollApp2 := channsRollApp2[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp2.ChannelID)

// 	channsRollApp2Dym := channsRollApp2[0]
// 	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp2.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
// 	// Get the IBC denom
// 	rollapp1IbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	dymToRollapp1IbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin rollapp1 denom balance
// 	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollapp1IbcDenom)
// 	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp1
// 	transferDataFromDym := ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount.Quo(math.NewInt(4)),
// 	}

// 	var options ibc.TransferOptions
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

// 	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
// 	require.NoError(t, err)
// 	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
// 	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymToRollapp1IbcDenom, transferDataFromDym.Amount)
// 	// verified ibc transfers worked

// 	// Create some pending eIBC packet
// 	multiplier := math.NewInt(10)

// 	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

// 	// set eIBC specific memo
// 	options.Memo = BuildEIbcMemo(eibcFee)

// 	// IBC Transfer working between rollapp1 <-> Dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
// 	require.NoError(t, err)
// 	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserRollapp1bal.Equal(transferAmount.Sub(bridgingFee)), "dym hub balance changed")

// 	// get eIbc event
// 	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
// 	require.NoError(t, err)
// 	// Make sure we have pending eIbc packet
// 	require.Greater(t, len(eibcEvents), 0, "no eIbc packet found in pending queue")

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	// Check Sequencer status before submitting fraud
// 	// sequencerStatus, err := dymension.GetNode().QuerySequencerStatus(ctx, sequencerAddr)
// 	// require.NotEmpty(t, sequencerStatus, "sequencers status is empty")
// 	// // Because we only have one sequencer in this test
// 	// fmt.Println("Sequencer jail status before submitting fraud proposal: ", sequencerStatus.Sequencers[0].Jailed)
// 	// fmt.Println("Sequencer bond amount before submitting fraud proposal: ", sequencerStatus.Sequencers[0].Tokens)
// 	// require.Equal(t, false, sequencerStatus.Sequencers[0].Jailed, "sequencer should not be jailed")
// 	// require.Equal(t, true, sequencerStatus.Sequencers[0].Tokens.AmountOf("adym").IsPositive(), "sequencer should have bond")

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// // Make sure sequencer slashed with all bond and jailed after fraud proposal passed
// 	// sequencerStatus, err = dymension.GetNode().QuerySequencerStatus(ctx, sequencerAddr)
// 	// require.NoError(t, err)
// 	// require.NotEmpty(t, sequencerStatus)
// 	// fmt.Println("Sequencer jail status after submitting fraud proposal: ", sequencerStatus.Sequencers[0].Jailed)
// 	// fmt.Println("Sequencer bond amount after submitting fraud proposal: ", sequencerStatus.Sequencers[0].Tokens)
// 	// require.Equal(t, true, sequencerStatus.Sequencers[0].Jailed, "sequencer should have been jailed")
// 	// require.Equal(t, false, sequencerStatus.Sequencers[0].Tokens.AmountOf("adym").IsPositive(), "sequencer should have been slashed and have zero bond")

// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// 	// TODO: Make sure we can not register a new sequencer

// }

// // TestRollAppSqcSlashedJailed_Wasm ensure upon freeze gov proposal passed, Sequencer should be slashed with all the bond and jailed. Inability to register a new sequencer
// func TestRollAppSqcSlashedJailed_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappwasm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "decentrio_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "3s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappwasm_1234-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp2",
// 				ChainID:             "decentrio_12345-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r, "relayer1").
// 		AddRelayer(s, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: s,
// 			Path:    anotherIbcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet1, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	wallet2, found := s.GetWallet(rollapp2.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
// 	require.NoError(t, err)
// 	keyPath2 := keyDir2 + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 2, dymension)
// 	require.NoError(t, err)

// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, s, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}

// 			err = s.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()
// 	rollapp2UserAddr := rollapp2User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollapp2OrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)	SequencerOrigBal, err := dymension.GetBalance(ctx, rollapp2UserAddr, dymension.Config().Denom)
// 	// fmt.Println("SequencerOrigBal", SequencerOrigBal)

// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 2)

// 	channsDym2, err := s.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym2, 2)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	channsRollApp2, err := s.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp2, 1)

// 	channDymRollApp2 := channsRollApp2[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp2.ChannelID)

// 	channsRollApp2Dym := channsRollApp2[0]
// 	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp2.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}
// 	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
// 	// Get the IBC denom
// 	rollapp1IbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	dymToRollapp1IbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin rollapp1 denom balance
// 	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollapp1IbcDenom)
// 	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp1
// 	transferDataFromDym := ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount.Quo(math.NewInt(4)),
// 	}

// 	var options ibc.TransferOptions
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

// 	rollapp1UserUpdateBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollapp1IbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, rollapp1OriginBal1.Add(transferAmount.Quo(math.NewInt(4))).Equal(rollapp1UserUpdateBal), "rollapp balance did not change")
// 	// verified ibc transfers worked

// 	// Create some pending eIBC packet
// 	multiplier := math.NewInt(10)

// 	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

// 	// set eIBC specific memo
// 	options.Memo = BuildEIbcMemo(eibcFee)

// 	// IBC Transfer working between rollapp1 <-> Dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
// 	require.NoError(t, err)
// 	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserRollapp1bal.Equal(transferAmount.Sub(bridgingFee)), "dym hub balance changed")

// 	// get eIbc event
// 	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
// 	require.NoError(t, err)
// 	// Make sure we have pending eIbc packet
// 	require.Greater(t, len(eibcEvents), 0, "no eIbc packet found in pending queue")

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	// Check Sequencer status before submitting fraud
// 	// sequencerStatus, err := dymension.GetNode().QuerySequencerStatus(ctx, sequencerAddr)
// 	// require.NotEmpty(t, sequencerStatus, "sequencers status is empty")
// 	// // Because we only have one sequencer in this test
// 	// fmt.Println("Sequencer jail status before submitting fraud proposal: ", sequencerStatus.Sequencers[0].Jailed)
// 	// fmt.Println("Sequencer bond amount before submitting fraud proposal: ", sequencerStatus.Sequencers[0].Tokens)
// 	// require.Equal(t, false, sequencerStatus.Sequencers[0].Jailed, "sequencer should not be jailed")
// 	// require.Equal(t, true, sequencerStatus.Sequencers[0].Tokens.AmountOf("adym").IsPositive(), "sequencer should have bond")

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// // Make sure sequencer slashed with all bond and jailed after fraud proposal passed
// 	// sequencerStatus, err = dymension.GetNode().QuerySequencerStatus(ctx, sequencerAddr)
// 	// require.NoError(t, err)
// 	// require.NotEmpty(t, sequencerStatus)
// 	// fmt.Println("Sequencer jail status after submitting fraud proposal: ", sequencerStatus.Sequencers[0].Jailed)
// 	// fmt.Println("Sequencer bond amount after submitting fraud proposal: ", sequencerStatus.Sequencers[0].Tokens)
// 	// require.Equal(t, true, sequencerStatus.Sequencers[0].Jailed, "sequencer should have been jailed")
// 	// require.Equal(t, false, sequencerStatus.Sequencers[0].Tokens.AmountOf("adym").IsPositive(), "sequencer should have been slashed and have zero bond")

// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
// 	// TODO: Make sure we can not register a new sequencer

// }

// func GetIBCDenom(counterPartyPort, counterPartyChannel, denom string) string {
// 	prefixDenom := transfertypes.GetPrefixedDenom(counterPartyPort, counterPartyChannel, denom)
// 	ibcDenom := transfertypes.ParseDenomTrace(prefixDenom).IBCDenom()
// 	return ibcDenom
// }

// func TestRollAppFreezeStateNotProgressing_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappevm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides := overridesDymintToml(settlement_layer_rollapp, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappevm_1234-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddRelayer(r, "relayer1").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 1)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// Assert balance was updated on the hub
// 	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
// 	// Get the IBC denom
// 	rollappIbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	dymToRollappIbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin rollapp1 denom balance
// 	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
// 	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp1
// 	transferDataFromDym := ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	var options ibc.TransferOptions
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

// 	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
// 	require.NoError(t, err)
// 	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
// 	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymToRollappIbcDenom, transferDataFromDym.Amount)
// 	// verified ibc transfers worked

// 	// Create some pending eIBC packet
// 	multiplier := math.NewInt(10)

// 	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

// 	// set eIBC specific memo
// 	options.Memo = BuildEIbcMemo(eibcFee)

// 	// IBC Transfer working between rollapp1 <-> Dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
// 	require.NoError(t, err)
// 	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollappIbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserRollapp1bal.Equal(transferAmount.Sub(bridgingFee)), "dym hub balance changed")

// 	// get eIbc event
// 	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
// 	require.NoError(t, err)

// 	for i, eibcEvent := range eibcEvents {
// 		fmt.Println(i, "EIBC Event:", eibcEvent)
// 	}

// 	resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
// 	require.NoError(t, err)
// 	require.Equal(t, 1, len(resp.DemandOrders))

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	testutil.WaitForBlocks(ctx, 30, dymension, rollapp1)
// 	// after Grace period, the latest index should be the same
// 	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")
// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// func TestRollAppFreezeStateNotProgressing_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappwasm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappwasm_1234-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddRelayer(r, "relayer1").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 1)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// Assert balance was updated on the hub
// 	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
// 	// Get the IBC denom
// 	rollappIbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	dymToRollappIbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin rollapp1 denom balance
// 	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
// 	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp1
// 	transferDataFromDym := ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	var options ibc.TransferOptions
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

// 	rollapp1UserUpdateBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, rollapp1OriginBal1.Add(transferAmount).Equal(rollapp1UserUpdateBal), "rollapp balance did not change")
// 	// verified ibc transfers worked

// 	// Create some pending eIBC packet
// 	multiplier := math.NewInt(10)

// 	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

// 	// set eIBC specific memo
// 	options.Memo = BuildEIbcMemo(eibcFee)

// 	// IBC Transfer working between rollapp1 <-> Dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
// 	require.NoError(t, err)
// 	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollappIbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserRollapp1bal.Equal(transferAmount.Sub(bridgingFee)), "dym hub balance changed")

// 	// get eIbc event
// 	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
// 	require.NoError(t, err)

// 	for i, eibcEvent := range eibcEvents {
// 		fmt.Println(i, "EIBC Event:", eibcEvent)
// 	}

// 	resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
// 	require.NoError(t, err)
// 	require.Equal(t, 1, len(resp.DemandOrders))

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	testutil.WaitForBlocks(ctx, 30, dymension, rollapp1)
// 	// after Grace period, the latest index should be the same
// 	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")
// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// func TestRollAppFreezeEibcPending_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappevm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides := overridesDymintToml(settlement_layer_rollapp, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappevm_1234-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddRelayer(r, "relayer1").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 1)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// Assert balance was updated on the hub
// 	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
// 	// Get the IBC denom
// 	rollappIbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	dymToRollappIbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin rollapp1 denom balance
// 	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
// 	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp1
// 	transferDataFromDym := ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	var options ibc.TransferOptions
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

// 	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
// 	require.NoError(t, err)
// 	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
// 	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymToRollappIbcDenom, transferDataFromDym.Amount)
// 	// verified ibc transfers worked

// 	// Create some pending eIBC packet
// 	multiplier := math.NewInt(10)

// 	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

// 	// set eIBC specific memo
// 	options.Memo = BuildEIbcMemo(eibcFee)

// 	// IBC Transfer working between rollapp1 <-> Dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
// 	require.NoError(t, err)
// 	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollappIbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserRollapp1bal.Equal(transferAmount.Sub(bridgingFee)), "dym hub balance changed")

// 	// get eIbc event
// 	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
// 	require.NoError(t, err)

// 	for i, eibcEvent := range eibcEvents {
// 		fmt.Println(i, "EIBC Event:", eibcEvent)
// 	}

// 	resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
// 	require.NoError(t, err)
// 	require.Equal(t, 1, len(resp.DemandOrders))

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	// after Grace period, the latest index should be the same
// 	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// eibc demand order reverted
// 	resp, err = dymension.QueryEIBCDemandOrders(ctx, "REVERTED")
// 	require.NoError(t, err)
// 	require.Equal(t, 2, len(resp.DemandOrders))

// 	// After rollapp frozen, inability to fulfill eIBC transfer
// 	// rollappUserUpdateBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	// require.Equal(t, rollappUserOriginBal.Sub(transferAmount), rollappUserUpdateBal)

// 	// check balances of dymensionUserAddr
// 	balanceOfDymUserAddr, err := dymension.GetBalance(ctx, dymensionUserAddr, rollappIbcDenom)
// 	require.NoError(t, err)
// 	require.Equal(t, (transferAmount.Sub(bridgingFee)), balanceOfDymUserAddr)
// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }

// func TestRollAppFreezeEibcPending_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappwasm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappwasm_1234-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddRelayer(r, "relayer1").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
// 	require.True(t, found)

// 	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	keyPath := keyDir + "/sequencer_keys"

// 	err = testutil.WaitForBlocks(ctx, 5, dymension)
// 	require.NoError(t, err)

// 	//Update white listed relayers
// 	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

// 	// Start both relayers
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err = r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	// keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
// 	// sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
// 	// require.NoError(t, err)
// 	// IBC channel for rollapps
// 	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym1, 1)

// 	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp1, 1)

// 	channDymRollApp1 := channsRollApp1[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp1.ChannelID)

// 	channsRollApp1Dym := channsRollApp1[0]
// 	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// Assert balance was updated on the hub
// 	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
// 	fmt.Println(res)
// 	require.NoError(t, err)

// 	for _, packet := range res.RollappPackets {

// 		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
// 		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)
// 		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
// 		require.NoError(t, err)

// 		fmt.Println(txhash)
// 	}

// 	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
// 	// Get the IBC denom
// 	rollappIbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	dymToRollappIbcDenom := GetIBCDenom(channsRollApp1Dym.PortID, channsRollApp1Dym.ChannelID, dymension.Config().Denom)

// 	// Get origin rollapp1 denom balance
// 	rollapp1OriginBal1, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
// 	fmt.Println("rollapp1OriginBal1", rollapp1OriginBal1)
// 	require.NoError(t, err)

// 	// IBC Transfer working between Dymension <-> rollapp1
// 	transferDataFromDym := ibc.WalletData{
// 		Address: rollapp1UserAddr,
// 		Denom:   dymension.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	var options ibc.TransferOptions
// 	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataFromDym, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)

// 	rollapp1UserUpdateBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, dymToRollappIbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, rollapp1OriginBal1.Add(transferAmount).Equal(rollapp1UserUpdateBal), "rollapp balance did not change")
// 	// verified ibc transfers worked

// 	// Create some pending eIBC packet
// 	multiplier := math.NewInt(10)

// 	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

// 	// set eIBC specific memo
// 	options.Memo = BuildEIbcMemo(eibcFee)

// 	// IBC Transfer working between rollapp1 <-> Dymension
// 	transferData = ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount,
// 	}

// 	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
// 	require.NoError(t, err)
// 	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollappIbcDenom)
// 	require.NoError(t, err)

// 	require.Equal(t, true, dymUserRollapp1bal.Equal(transferAmount.Sub(bridgingFee)), "dym hub balance changed")

// 	// get eIbc event
// 	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
// 	require.NoError(t, err)

// 	for i, eibcEvent := range eibcEvents {
// 		fmt.Println(i, "EIBC Event:", eibcEvent)
// 	}

// 	resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
// 	require.NoError(t, err)
// 	require.Equal(t, 1, len(resp.DemandOrders))

// 	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	// Access the index value
// 	index := oldLatestIndex.StateIndex.Index
// 	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
// 	require.NoError(t, err)

// 	targetIndex := oldUintIndex + 1

// 	// Loop until the latest index updates
// 	for {
// 		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 		require.NoError(t, err)

// 		index := latestIndex.StateIndex.Index
// 		uintIndex, err := strconv.ParseUint(index, 10, 64)
// 		require.NoError(t, err)

// 		if uintIndex >= targetIndex {
// 			break
// 		}
// 	}

// 	fraud_height, err := rollapp1.Height(ctx)
// 	require.NoError(t, err, "error fetching height")

// 	// Create fraud proposal message
// 	msg := map[string]interface{}{
// 		"@type":                    "/dymensionxyz.dymension.rollapp.MsgRollappFraudProposal",
// 		"authority":                "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
// 		"rollapp_id":               "rollappevm_1234-1",
// 		"fraud_revision":           "0",
// 		"fraud_height":             fmt.Sprint(fraud_height),
// 		"punish_sequencer_address": "",
// 	}

// 	rawMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		fmt.Println("Err:", err)
// 	}

// 	proposal := cosmos.TxFraudProposal{
// 		Deposit:  "500000000000" + dymension.Config().Denom,
// 		Title:    "rollapp Upgrade 1",
// 		Summary:  "test",
// 		Messages: []json.RawMessage{rawMsg},
// 		Metadata: "ipfs://CID",
// 	}

// 	// Submit fraud proposal
// 	_, _ = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), proposal)

// 	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 	require.NoError(t, err, "failed to submit votes")

// 	height, err := dymension.Height(ctx)
// 	require.NoError(t, err, "error fetching height")
// 	haltHeight := height + haltHeightDelta

// 	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
// 	require.NoError(t, err, "proposal status did not change to passed")

// 	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	// after Grace period, the latest index should be the same
// 	lalatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, latestIndex, lalatestIndex, "rollapp state index still increment after grace period. Rerun")

// 	// Check if rollapp1 has frozen or not
// 	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
// 	require.NoError(t, err)
// 	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

// 	// Check rollapp1 state index not increment
// 	require.NoError(t, err)
// 	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

// 	// eibc demand order reverted
// 	resp, err = dymension.QueryEIBCDemandOrders(ctx, "REVERTED")
// 	require.NoError(t, err)
// 	require.Equal(t, 2, len(resp.DemandOrders))

// 	// After rollapp frozen, inability to fulfill eIBC transfer
// 	// rollappUserUpdateBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 	// require.NoError(t, err)
// 	// require.Equal(t, rollappUserOriginBal.Sub(transferAmount), rollappUserUpdateBal)

// 	// check balances of dymensionUserAddr (just receive the fund for the fisrt transfer)
// 	balanceOfDymUserAddr, err := dymension.GetBalance(ctx, dymensionUserAddr, rollappIbcDenom)
// 	require.NoError(t, err)
// 	require.Equal(t, (transferAmount.Sub(bridgingFee)), balanceOfDymUserAddr)

// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())

// }
