package tests

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
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

var dymModifyGenesisKV = append(
	dymensionGenesisKV,
	cosmos.GenesisKV{
		Key:   "app_state.rollapp.params.dispute_period_in_blocks",
		Value: "20",
	},
)

var extraFlags = map[string]interface{}{"genesis-accounts-path": true}

// TestRollAppFreeze ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing.
func TestRollAppFreeze_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := overridesDymintToml("dymension", fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name()), "rollappevm_1234-1", "0adym", "3s")
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
				CoinType:            "60",
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
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
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

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Start both relayers
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

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

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

	triggerHubGenesisEvent(t, dymension,
		rollappParam{
			rollappID: rollapp1.Config().ChainID,
			channelID: channDymRollApp1.ChannelID,
			userKey:   dymensionUser.KeyName(),
		}, rollappParam{
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

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

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
	transferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.Error(t, err)
	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

	// Compose an IBC transfer and send from rollapp -> dymension
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	// Get the IBC denom
	rollapp1Denom := transfertypes.GetPrefixedDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

	// Get origin dym hub ibc denom balance
	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Wait a few blocks
	err = testutil.WaitForBlocks(ctx, 20, dymension)

	// Get updated dym hub ibc denom balance
	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	// IBC balance should not change
	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")
}

// TestRollAppFreeze ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing.
func TestRollAppFreeze_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := overridesDymintToml("dymension", fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name()), "rollappwasm_1234-1", "0adym", "3s")
	configFileOverrides2 := overridesDymintToml("dymension", fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name()), "rollappwasm_12345-1", "0adym", "3s")

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
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       nil,
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp2",
				ChainID:             "rollappwasm_12345-1",
				Images:              []ibc.DockerImage{rollappWasmImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "rol",
				Denom:               "urax",
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       nil,
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
				CoinType:            "60",
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
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
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

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Start both relayers
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err = r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer : %s", err)
			}
			err = r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

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

	triggerHubGenesisEvent(t, dymension,
		rollappParam{
			rollappID: rollapp1.Config().ChainID,
			channelID: channDymRollApp1.ChannelID,
			userKey:   dymensionUser.KeyName(),
		}, rollappParam{
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

	currentHeight, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	_, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.Config().ChainID, currentHeight, 300)
	require.NoError(t, err)

	oldLatestRollapp1, err := dymension.FinalizedRollappStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)

	var fraudHeight string
	var fraudProposer string

	// Loop until the next pending state index appears
	for {
		res, err := dymension.QueryRollappState(ctx, rollapp1.Config().ChainID, false)
		require.NoError(t, err)

		latestIndex := res.StateInfo.StateInfoIndex.Index
		parsedIndex, err := strconv.ParseUint(latestIndex, 10, 64)
		require.NoError(t, err)

		if parsedIndex > oldLatestRollapp1 && res.StateInfo.Status == "PENDING" {
			fraudHeight = res.StateInfo.BlockDescriptors.BD[len(res.StateInfo.BlockDescriptors.BD)-1].Height
			fraudProposer = res.StateInfo.Sequencer
			break
		}
	}

	submitFraudStr := "fraud"
	deposit := "500000000000" + dymension.Config().Denom

	var rollapp1ClientOnDym string

	dymClients, err := r1.GetClients(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(dymClients), 2)

	for _, client := range dymClients {
		if client.ClientState.ChainID == rollapp1.Config().ChainID {
			rollapp1ClientOnDym = client.ClientID
		}
	}

	propTx, err := dymension.SubmitFraudProposal(
		ctx, dymensionUser.KeyName(),
		rollapp1.Config().ChainID,
		fraudHeight,
		fraudProposer,
		rollapp1ClientOnDym,
		submitFraudStr,
		submitFraudStr,
		deposit,
	)
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	// Check if rollapp has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

	// Check rollapp state index not increment
	latestFinalizedIndex, err := dymension.FinalizedRollappStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, targetIndex, latestFinalizedIndex, "rollapp state index still increment")

	// IBC Transfer not working
	// Compose an IBC transfer and send from dymension -> rollapp
	transferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.Error(t, err)
	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

	// Compose an IBC transfer and send from rollapp -> dymension
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	// Get the IBC denom
	rollapp1Denom := transfertypes.GetPrefixedDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

	// Get origin dym hub ibc denom balance
	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Wait a few blocks
	err = testutil.WaitForBlocks(ctx, 20, dymension)

	// Get updated dym hub ibc denom balance
	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	// IBC balance should not change
	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")
}

// TestOtherRollappNotAffected_EVM ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing and other rollapp works fine.
func TestOtherRollappNotAffected_EVM(t *testing.T) {
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
	dymintTomlOverrides["empty_blocks_max_time"] = "3s"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	configFileOverrides2 := make(map[string]any)
	dymintTomlOverrides2 := make(testutil.Toml)
	dymintTomlOverrides2["settlement_layer"] = "dymension"
	dymintTomlOverrides2["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides2["rollapp_id"] = "rollappevm_12345-1"
	dymintTomlOverrides2["gas_prices"] = "0adym"
	dymintTomlOverrides2["empty_blocks_max_time"] = "3s"

	configFileOverrides2["config/dymint.toml"] = dymintTomlOverrides2
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
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp1",
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
				CoinType:            "60",
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

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	).Build(t, client, "relayer1", network)

	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r, "relayer1").
		AddRelayer(s, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: s,
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

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, s, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Start both relayers
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err = r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}

			err = s.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()
	rollapp2UserAddr := rollapp2User.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollapp2OrigBal)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)

	// IBC channel for rollapps
	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym1, 2)

	channsDym2, err := s.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym2, 2)

	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channDymRollApp1 := channsRollApp1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	channsRollApp1Dym := channsRollApp1[0]
	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

	channsRollApp2, err := s.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp2, 1)

	channDymRollApp2 := channsRollApp2[0].Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	channsRollApp2Dym := channsRollApp2[0]
	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

	triggerHubGenesisEvent(t, dymension,
		rollappParam{
			rollappID: rollapp1.Config().ChainID,
			channelID: channDymRollApp1.ChannelID,
			userKey:   dymensionUser.KeyName(),
		}, rollappParam{
			rollappID: rollapp2.Config().ChainID,
			channelID: channDymRollApp2.ChannelID,
			userKey:   dymensionUser.KeyName(),
		})

	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)

	// Access the index value
	index := oldLatestIndex.StateIndex.Index
	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)

	targetIndex := oldUintIndex + 1

	rollapp2Index, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp2.Config().ChainID)
	require.NoError(t, err)

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

	rollappHeight, err := rollapp1.Height(ctx)
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

	propTx, err := dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), rollapp1.Config().ChainID, fraudHeight, sequencerAddr, rollapp1ClientOnDym, submitFraudStr, submitFraudStr, deposit)
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	// Check if rollapp1 has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

	// Check rollapp1 state index not increment
	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

	// Compose an IBC transfer and send from dymension -> rollapp
	transferData := ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Confirm IBC Transfer not working between Dymension <-> rollapp1
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.Error(t, err)
	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	// Get the IBC denom
	rollapp1Denom := transfertypes.GetPrefixedDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

	// Get origin dym hub ibc denom balance
	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Wait a few blocks
	err = testutil.WaitForBlocks(ctx, 20, dymension)

	// Get updated dym hub ibc denom balance
	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	// IBC balance should not change
	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")

	// Check other rollapp state index still increase
	rollapp2IndexLater, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp2.Config().ChainID)
	require.NoError(t, err)
	require.True(t, rollapp2IndexLater.StateIndex.Index > rollapp2Index.StateIndex.Index, "Another rollapp got freeze")

	// Get the IBC denom
	rollapp2IbcDenom := GetIBCDenom(channsRollApp2Dym.Counterparty.PortID, channsRollApp2Dym.Counterparty.ChannelID, rollapp2.Config().Denom)
	dymToRollapp2IbcDenom := GetIBCDenom(channsRollApp2Dym.PortID, channsRollApp2Dym.ChannelID, dymension.Config().Denom)

	// Get origin dym hub ibc denom balance
	dymUserOriginBal2, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IbcDenom)
	require.NoError(t, err)

	rollapp2UserOriginBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, dymToRollapp2IbcDenom)
	require.NoError(t, err)

	// IBC Transfer working between Dymension <-> rollapp2
	transferData = ibc.WalletData{
		Address: rollapp2UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp2.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, dymension, rollapp2)

	rollapp2UserUpdateBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, dymToRollapp2IbcDenom)
	require.NoError(t, err)

	require.Equal(t, true, rollapp2UserUpdateBal.Sub(transferAmount).Equal(rollapp2UserOriginBal), "rollapp balance did not change")

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}

	tx, err := rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, tx.TxHash, "tx is nil")

	rollappHeight, err = rollapp2.Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Get updated dym hub ibc denom balance
	dymUserUpdateBal2, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IbcDenom)
	require.NoError(t, err)

	require.Equal(t, true, dymUserUpdateBal2.Equal(dymUserOriginBal2.Add(transferAmount)), "dym hub balance did not change")
}

// TestOtherRollappNotAffected_Wasm ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing and other rollapp works fine.
func TestOtherRollappNotAffected_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["gas_prices"] = "0adym"
	dymintTomlOverrides["empty_blocks_max_time"] = "3s"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	configFileOverrides2 := make(map[string]any)
	dymintTomlOverrides2 := make(testutil.Toml)
	dymintTomlOverrides2["settlement_layer"] = "dymension"
	dymintTomlOverrides2["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides2["rollapp_id"] = "rollappwasm_12345-1"
	dymintTomlOverrides2["gas_prices"] = "0adym"
	dymintTomlOverrides2["empty_blocks_max_time"] = "3s"

	configFileOverrides2["config/dymint.toml"] = dymintTomlOverrides2
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
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       nil,
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp1",
				ChainID:             "rollappwasm_12345-1",
				Images:              []ibc.DockerImage{rollappWasmImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "rol",
				Denom:               "urax",
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       nil,
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
				CoinType:            "60",
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

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	).Build(t, client, "relayer1", network)

	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r, "relayer1").
		AddRelayer(s, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: s,
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

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, s, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Start both relayers
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err = r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}

			err = s.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()
	rollapp2UserAddr := rollapp2User.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollapp2OrigBal)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)

	// IBC channel for rollapps
	channsDym1, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym1, 2)

	channsDym2, err := s.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym2, 2)

	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channDymRollApp1 := channsRollApp1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	channsRollApp1Dym := channsRollApp1[0]
	require.NotEmpty(t, channsRollApp1Dym.ChannelID)

	channsRollApp2, err := s.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp2, 1)

	channDymRollApp2 := channsRollApp2[0].Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	channsRollApp2Dym := channsRollApp2[0]
	require.NotEmpty(t, channsRollApp2Dym.ChannelID)

	triggerHubGenesisEvent(t, dymension,
		rollappParam{
			rollappID: rollapp1.Config().ChainID,
			channelID: channDymRollApp1.ChannelID,
			userKey:   dymensionUser.KeyName(),
		}, rollappParam{
			rollappID: rollapp2.Config().ChainID,
			channelID: channDymRollApp2.ChannelID,
			userKey:   dymensionUser.KeyName(),
		})

	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)

	// Access the index value
	index := oldLatestIndex.StateIndex.Index
	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)

	targetIndex := oldUintIndex + 1

	rollapp2Index, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp2.Config().ChainID)
	require.NoError(t, err)

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

	rollappHeight, err := rollapp1.Height(ctx)
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

	propTx, err := dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), rollapp1.Config().ChainID, fraudHeight, sequencerAddr, rollapp1ClientOnDym, submitFraudStr, submitFraudStr, deposit)
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+20, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	// Check if rollapp1 has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, true, rollappParams.Rollapp.Frozen, "rollapp does not frozen")

	// Check rollapp1 state index not increment
	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprint(targetIndex), latestIndex.StateIndex.Index, "rollapp state index still increment")

	// Compose an IBC transfer and send from dymension -> rollapp
	transferData := ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Confirm IBC Transfer not working between Dymension <-> rollapp1
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.Error(t, err)
	require.Equal(t, true, strings.Contains(err.Error(), "status Frozen"))

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	// Get the IBC denom
	rollapp1Denom := transfertypes.GetPrefixedDenom(channsRollApp1Dym.Counterparty.PortID, channsRollApp1Dym.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollapp1IbcDenom := transfertypes.ParseDenomTrace(rollapp1Denom).IBCDenom()

	// Get origin dym hub ibc denom balance
	dymUserOriginBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1Dym.ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Wait a few blocks
	err = testutil.WaitForBlocks(ctx, 20, dymension)

	// Get updated dym hub ibc denom balance
	dymUserUpdateBal, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp1IbcDenom)
	require.NoError(t, err)

	// IBC balance should not change
	require.Equal(t, dymUserOriginBal, dymUserUpdateBal, "dym hub still get transfer from frozen rollapp")

	// Check other rollapp state index still increase
	rollapp2IndexLater, err := dymension.GetNode().QueryLatestStateIndex(ctx, rollapp2.Config().ChainID)
	require.NoError(t, err)
	require.True(t, rollapp2IndexLater.StateIndex.Index > rollapp2Index.StateIndex.Index, "Another rollapp got freeze")

	// Get the IBC denom
	rollapp2IbcDenom := GetIBCDenom(channsRollApp2Dym.Counterparty.PortID, channsRollApp2Dym.Counterparty.ChannelID, rollapp2.Config().Denom)
	dymToRollapp2IbcDenom := GetIBCDenom(channsRollApp2Dym.PortID, channsRollApp2Dym.ChannelID, dymension.Config().Denom)

	// Get origin dym hub ibc denom balance
	dymUserOriginBal2, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IbcDenom)
	require.NoError(t, err)

	rollapp2UserOriginBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, dymToRollapp2IbcDenom)
	require.NoError(t, err)

	// IBC Transfer working between Dymension <-> rollapp2
	transferData = ibc.WalletData{
		Address: rollapp2UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp2.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, dymension)

	rollapp2UserUpdateBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, dymToRollapp2IbcDenom)
	require.NoError(t, err)

	require.Equal(t, true, rollapp2UserUpdateBal.Sub(transferAmount).Equal(rollapp2UserOriginBal), "rollapp balance did not change")

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}

	tx, err := rollapp2.SendIBCTransfer(ctx, channsRollApp2Dym.ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, tx.TxHash, "tx is nil")

	rollapp2Height, err := rollapp2.Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Get updated dym hub ibc denom balance
	dymUserUpdateBal2, err := dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IbcDenom)
	require.NoError(t, err)

	require.Equal(t, true, dymUserUpdateBal2.Equal(dymUserOriginBal2.Add(transferAmount)), "dym hub balance did not change")
}

func GetIBCDenom(counterPartyPort, counterPartyChannel, denom string) string {
	prefixDenom := transfertypes.GetPrefixedDenom(counterPartyPort, counterPartyChannel, denom)
	ibcDenom := transfertypes.ParseDenomTrace(prefixDenom).IBCDenom()
	return ibcDenom
}
