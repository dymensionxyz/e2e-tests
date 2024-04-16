package tests

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
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

func TestHardFork_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()
	// setup config for rollapps
	configFileOverrides1 := overridesDymintToml("dymension", fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name()), "rollappevm_1234-1", "0adym", "3s")
	configFileOverrides2 := overridesDymintToml("dymension", fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name()), "rollappevm_12345-1", "0adym", "3s")

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
				Name:                "rollapp-temp1",
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
				ChainID:             "rollappevm_12345-1",
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
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	// relayer for rollapp 1
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
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
		SkipPathCreation: true,
		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	})
	require.NoError(t, err)
	err = r1.GeneratePath(ctx, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID, ibcPath)
	require.NoError(t, err)
	err = r2.GeneratePath(ctx, eRep, dymension.Config().ChainID, rollapp2.Config().ChainID, anotherIbcPath)
	require.NoError(t, err)

	err = r1.CreateClients(ctx, eRep, ibcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)
	err = r2.CreateClients(ctx, eRep, anotherIbcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, dymension)
	require.NoError(t, err)

	r1.UpdateClients(ctx, eRep, ibcPath)
	require.NoError(t, err)
	r2.UpdateClients(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = r1.CreateConnections(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.CreateConnections(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	err = r1.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)
	err = r2.CreateChannel(ctx, eRep, anotherIbcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1, rollapp2)
	require.NoError(t, err)
	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err = r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
			err = r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
	walletAmount := math.NewInt(1_000_000_000_000)
	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)
	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)
	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User := users[0], users[1]
	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()
	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 3, dymension, rollapp1)
	require.NoError(t, err)
	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)

	// IBC channel for rollapps
	channsDym1, err := r1.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym1, 2)

	channsDym2, err := r2.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym2, 2)

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channDymRollApp1 := channsRollApp1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	channsRollApp1Dym := channsRollApp1[0]
	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp2, 1)

	channDymRollApp2 := channsRollApp2[0].Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	channsRollApp2Dym := channsRollApp2[0]
	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

	triggerHubGenesisEvent(t, dymension, rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: channDymRollApp1.ChannelID,
		userKey:   dymensionUser.KeyName(),
	})

	triggerHubGenesisEvent(t, dymension, rollappParam{
		rollappID: rollapp2.Config().ChainID,
		channelID: channDymRollApp2.ChannelID,
		userKey:   dymensionUser.KeyName(),
	})

	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	// Access the index value
	index := oldLatestIndex.StateIndex.Index
	uintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)
	targetIndex := uintIndex + 1
	// Loop until the latest index updates
	for {
		oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
		require.NoError(t, err)
		index := oldLatestIndex.StateIndex.Index
		uintIndex, err := strconv.ParseUint(index, 10, 64)
		require.NoError(t, err)
		if uintIndex >= targetIndex {
			break
		}
	}
	rollappHeight, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	rollapp1Clients, err := r1.GetClients(ctx, eRep, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(rollapp1Clients))
	propTx, err := dymension.SubmitFraudProposal(
		ctx, dymensionUser.KeyName(),
		rollapp1.Config().ChainID,
		fmt.Sprint(rollappHeight-2),
		sequencerAddr,
		rollapp1Clients[0].ClientID,
		"fraud",
		"fraud",
		"500000000000"+dymension.Config().Denom,
	)
	require.NoError(t, err)
	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")
	// Wait a few blocks for the gov to pass and to verify if the state index increment
	err = testutil.WaitForBlocks(ctx, 30, dymension, rollapp1)
	require.NoError(t, err)
	// Check if rollapp has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")
	// Check rollapp state index not increment
	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

	// IBC Transfer not working
	// Compose an IBC transfer and send from dymension -> rollapp
	transferAmount := math.NewInt(1_000_000)

	transferDataDym2RollApp := ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataDym2RollApp, ibc.TransferOptions{})
	require.Error(t, err)
	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

	// Get the IBC denom
	rollapp1Denom := transfertypes.GetPrefixedDenom(channDymRollApp1.PortID, channDymRollApp1.ChannelID, rollapp1.Config().Denom)
	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

	// Get origin dym hub ibc denom balance
	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	// Compose an IBC transfer and send from rollapp -> dymension
	transferDataRollApp2Dym := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferDataRollApp2Dym, ibc.TransferOptions{})
	require.NoError(t, err)
	err = testutil.WaitForBlocks(ctx, 20, dymension)
	require.NoError(t, err)

	// Get updated dym hub ibc denom balance
	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	// IBC balance should not change
	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")

	curHeight, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_, err = rollapp1.ExportState(ctx, int64(curHeight))
	require.NoError(t, err)

}
