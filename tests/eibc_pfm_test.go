package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

func TestEIBCPFM_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlementLayer := "dymension"
	nodeAddress := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1Id := "rollappevm_1-1"
	gasPrice := "0adym"
	emptyBlocksMaxTimeRollapp1 := "3s"
	configFileOverrides1 := overridesDymintToml(settlementLayer, nodeAddress, rollapp1Id, gasPrice, emptyBlocksMaxTimeRollapp1)

	// setup config for rollapp 2
	rollapp2Id := "rollappevm_2-1"
	emptyBlocksMaxTimeRollapp2 := "30s" // make sure rollapp 1 will have finalize height > rollapp 2
	configFileOverrides2 := overridesDymintToml(settlementLayer, nodeAddress, rollapp2Id, gasPrice, emptyBlocksMaxTimeRollapp2)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1

	const BLOCK_FINALITY_PERIOD = 80
	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
	)

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "rollappevm_1-1",
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
				ChainID:             "rollappevm_2-1",
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

	// Start both relayers
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	// t.Cleanup(
	// 	func() {
	// 		err = r1.StopRelayer(ctx, eRep)
	// 		if err != nil {
	// 			t.Logf("an error occurred while stopping the relayer: %s", err)
	// 		}
	// 		err = r2.StopRelayer(ctx, eRep)
	// 		if err != nil {
	// 			t.Logf("an error occurred while stopping the relayer: %s", err)
	// 		}
	// 	},
	// )

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()
	rollapp2UserAddr := rollapp2User.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp2, rollapp2UserAddr, rollapp2.Config().Denom, walletAmount)

	// Get dymension -> rollapp1 channel
	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channRollApp1Dym := channsRollApp1[0]
	require.NotEmpty(t, channRollApp1Dym.ChannelID)
	channDymRollApp1 := channRollApp1Dym.Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	// Get dymension -> rollapp2 channel
	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channRollApp2Dym := channsRollApp2[0]
	require.NotEmpty(t, channRollApp2Dym.ChannelID)
	channDymRollApp2 := channRollApp2Dym.Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	// Trigger genesis event for rollapp1
	rollapp1Param := rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: channDymRollApp1.ChannelID,
		userKey:   dymensionUser.KeyName(),
	}

	triggerHubGenesisEvent(t, dymension, rollapp1Param)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the rollapp1 is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	rollapp2Param := rollappParam{
		rollappID: rollapp2.Config().ChainID,
		channelID: channDymRollApp2.ChannelID,
		userKey:   dymensionUser.KeyName(),
	}
	triggerHubGenesisEvent(t, dymension, rollapp2Param)

	zeroBal := math.ZeroInt()
	transferAmount := math.NewInt(1_000_000)
	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// Send packet from rollapp1 -> dym -> rollapp2
	transfer := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	packetMetadata := &PacketMetadata{
		Forward: &ForwardMetadata{
			Receiver: rollapp2UserAddr,
			Channel:  channDymRollApp2.ChannelID,
			Port:     channDymRollApp2.PortID,
			Timeout:  5 * time.Minute,
		},
	}

	forwardMetadata, err := json.Marshal(packetMetadata)
	require.NoError(t, err)
	memo := fmt.Sprintf(`{"eibc": {"fee": "%s"}, %s}`, eibcFee.String(), string(forwardMetadata))

	_, err = rollapp1.SendIBCTransfer(ctx, channRollApp1Dym.ChannelID, rollapp1User.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 110, rollapp1, dymension)
	require.NoError(t, err)

	firstHopDenom := transfertypes.GetPrefixedDenom(channDymRollApp1.PortID, channDymRollApp1.ChannelID, rollapp1.Config().Denom)
	secondHopDenom := transfertypes.GetPrefixedDenom(channRollApp2Dym.PortID, channRollApp2Dym.ChannelID, firstHopDenom)

	firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
	secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

	firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
	secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferAmount))
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, transferAmount)
	testutil.AssertBalance(t, ctx, rollapp2, rollapp2UserAddr, secondHopIBCDenom, zeroBal)

}
