package tests

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"cosmossdk.io/math"
	// transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
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

func Test1(t *testing.T) {
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
				CoinType:            "118",
				GasPrices:           "0.0adym",
				EncodingConfig:      encodingConfig(),
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				NoHostMount:         false,
				ModifyGenesis:       modifyDymensionGenesis(dymModifyGenesisKV),
				ConfigFileOverrides: nil,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
			ExtraFlags:    extraFlags,
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
		relayer.CustomDockerImage("ghcr.io/dymensionxyz/go-relayer", "main-dym", "100:1000"),
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
	})
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Start both relayers
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

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, newSequencer, rollapp1User := users[0], users[1], users[2]

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

	err = dymension.GetNode().TriggerGenesisEvent(ctx, "sequencer", rollapp1.Config().ChainID, channDymRollApp1.ChannelID, dymension.GetRollApps()[0].GetSequencerKeyDir())
	require.NoError(t, err)

	// Confirm previous ibc transfers were successful (dymension -> rollapp1)
	// Get the IBC denom
	rollappIbcDenom := GetIBCDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
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
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, options)
	require.NoError(t, err)
	dymUserRollapp1bal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollappIbcDenom)
	require.NoError(t, err)

	require.Equal(t, true, dymUserRollapp1bal.Equal(zeroBal), "dym hub balance changed")

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
	rollappHeight, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	fraudHeight := fmt.Sprint(rollappHeight - 5)

	dymClients, err := r.GetClients(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymClients))

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

	// get last height of rollapp and export genesis at that height
	lastHeightOfRollapp, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	stateOfOldRollApp, err := rollapp1.ExportState(ctx, int64(lastHeightOfRollapp))

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, anotherIbcPath)

	// stop all nodes and override genesis with new state
	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)
	stateOfOldRollApp = strings.Split(stateOfOldRollApp, "\n")[0]

	new_rollapp_chainId := "rollappevm_1234-2"
	newState := strings.Replace(stateOfOldRollApp, "\"rollappevm_1234-1\"", "\"rollappevm_1234-2\"", 10)

	err = dymension.SetUpNewRollAppToHub(ctx, new_rollapp_chainId, newSequencer.KeyName(), "5", keyDir, nil)
	// wait a few blocks and verify sender received funds on the hub
	// err = testutil.WaitForBlocks(ctx, 5, dymension)
	// require.NoError(t, err)
	response, err := dymension.QueryRollappParams(ctx, new_rollapp_chainId)
	require.NoError(t, err)
	require.NotNil(t, response)
	// err = dymension.RegisterRollAppToHub(ctx, newSequencer.KeyName(), new_rollapp_chainId, "5", keyDir, metadataFileDir, flags)
	for _, node := range rollapp1.Nodes() {
		err := node.OverwriteGenesisFile(ctx, []byte(newState))
		require.NoError(t, err)
	}

	// override dymint.toml
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["rollapp_id"] = new_rollapp_chainId

	for _, node := range rollapp1.Nodes() {
		err = testutil.ModifyTomlConfigFile(
			ctx,
			node.Logger(),
			node.DockerClient,
			node.TestName,
			node.VolumeName,
			node.Chain.Config().Name,
			"config/dymint.toml",
			dymintTomlOverrides,
		)
		require.NoError(t, err)
	}

	// create new sequencer
	_, _, err = rollapp1.GetNode().ExecInit(ctx, "newsequencer", rollapp1.HomeDir() + "/newsequencer")
	require.NoError(t, err)

	command := append([]string{rollapp1.GetNode().Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.HomeDir() + "/newsequencer")
	pub1, _, err := rollapp1.GetNode().Exec(ctx, command, nil)
	require.NoError(t, err)

	command = []string{}
	command = append(command, "sequencer", "create-sequencer", string(pub1), new_rollapp_chainId, "{\"Moniker\":\"myrollapp-sequencer\",\"Identity\":\"\",\"Website\":\"\",\"SecurityContact\":\"\",\"Details\":\"\"}", "1000000000adym",
		"--broadcast-mode", "block")

	_, err = dymension.GetNode().ExecTx(ctx, newSequencer.KeyName(), command...)
	require.NoError(t, err)


	// nid, err := rollapp1.Validators[0].NodeID(ctx)
	// require.NoError(t, err)

	// anotherDymintTomlOverrides := make(testutil.Toml)
	// anotherDymintTomlOverrides["persistent_peers"] = fmt.Sprintf("%s@newsequencer:26656", nid)

	// for _, node := range rollapp1.Nodes() {
	// 	nid, err := node.NodeID(ctx)
	// 	require.NoError(t, err)

	// 	anotherDymintTomlOverrides := make(testutil.Toml)
	// 	anotherDymintTomlOverrides["persistent_peers"] = fmt.Sprintf("%s@newsequencer:26656", nid)

	// 	err = testutil.ModifyTomlConfigFile(
	// 		ctx,
	// 		node.Logger(),
	// 		node.DockerClient,
	// 		node.TestName,
	// 		node.VolumeName,
	// 		node.Chain.Config().Name,
	// 		"newsequencer/config/config.toml",
	// 		anotherDymintTomlOverrides,
	// 	)
	// 	require.NoError(t, err)
	// }

	// rerun nodes with new chain id
	err = rollapp1.Start(t.Name(), ctx, ibc.WalletData{})
	require.NoError(t, err, "error starting node(s)")

	balanceOfRollAppUser, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)
	// dymUserRollapp1bal, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIbcDenom)
	// require.NoError(t, err)
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, balanceOfRollAppUser.Sub(transferData.Amount))


	// err = r.StartRelayer(ctx, eRep, anotherIbcPath)
	// require.NoError(t, err)

	// // create new sequencer
	// _, _, err = rollapp1.GetNode().ExecInit(ctx, "newsequencer", "/var/cosmos-chain/newsequencer")
	// require.NoError(t, err)

	// command := append([]string{rollapp1.GetNode().Chain.Config().Bin}, "dymint", "show-sequencer", "--home", "/var/cosmos-chain/newsequencer")
	// pub1, _, err := rollapp1.GetNode().Exec(ctx, command, nil)
	// require.NoError(t, err)

	// // rollapp1
	// command = []string{}
	// command = append(command, "sequencer", "create-sequencer", string(pub1), new_rollapp_id, "{\"Moniker\":\"myrollapp-sequencer\",\"Identity\":\"\",\"Website\":\"\",\"SecurityContact\":\"\",\"Details\":\"\"}", "1000000000adym",
	// 	"--broadcast-mode", "block")

	// _, err = dymension.GetNode().ExecTx(ctx, newSequencer.KeyName(), command...)
	// require.NoError(t, err)
}
