package tests

import (
	"bytes"
	"context"
	"fmt"
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

func TestEIBC_AckError_Dym_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1-1"
	gas_price_rollapp1 := "0adym"
	emptyBlocksMaxTimeRollapp1 := "10s"
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, node_address, rollapp1_id, gas_price_rollapp1, emptyBlocksMaxTimeRollapp1)

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "rollappevm_2-1"
	gas_price_rollapp2 := "0adym"
	emptyBlocksMaxTimeRollapp2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, node_address, rollapp2_id, gas_price_rollapp2, emptyBlocksMaxTimeRollapp2)

	// Custom dymension epoch for faster disconnection
	const BLOCK_FINALITY_PERIOD = 50
	modifyGenesisKV := append(
		dymModifyGenesisKV,
		[]cosmos.GenesisKV{
			{
				Key:   "app_state.rollapp.params.dispute_period_in_blocks",
				Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
			},
			{
				Key:   "app_state.transfer.params.receive_enabled",
				Value: false,
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
				Name:                "rollapp-temp",
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
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "2.5.2", "100:1000"),
	).Build(t, client, "relayer1", network)
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

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	transferAmount := math.NewInt(1_000_000)
	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	transferAmountWithoutFee := transferAmount.Sub(eibcFee)

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

	// Trigger genesis event for both rollapps
	rollapps := []rollappParam{
		{
			rollappID: rollapp1.Config().ChainID,
			channelID: channDymRollApp1.ChannelID,
			userKey:   dymensionUser.KeyName(),
		},
		{
			rollappID: rollapp2.Config().ChainID,
			channelID: channDymRollApp2.ChannelID,
			userKey:   dymensionUser.KeyName(),
		},
	}

	triggerHubGenesisEvent(t, dymension, rollapps...)

	rollapp1Height, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the rollapps is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Transfer dymension from hub to rollapp
	transferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Get the IBC denom for adym on rollapp
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channDymRollApp1.PortID, channDymRollApp1.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	var options ibc.TransferOptions

	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 3, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferAmount)

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   dymensionIBCDenom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	ibcTx, err := rollapp1.SendIBCTransfer(ctx, channRollApp1Dym.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	zeroBalance := math.NewInt(0)
	balance, err := rollapp1.GetBalance(ctx, rollappUserAddr, dymensionIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of rollappUserAddr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(zeroBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBalance, balance))

	// catch ACK errors
	rollappHeight, err = rollapp1.Height(ctx)
	require.NoError(t, err)

	ack, err := testutil.PollForAck(ctx, rollapp1, rollappHeight, rollappHeight+80, ibcTx.Packet)
	require.NoError(t, err)

	// Make sure that the ack contains error
	require.True(t, bytes.Contains(ack.Acknowledgement, []byte("error")))

	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferAmount)

	// At the moment, the ack returned and the demand order status became "finalized"
	// We will execute the ibc transfer again and try to fulfill the demand order
	_, err = rollapp1.SendIBCTransfer(ctx, channRollApp1Dym.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	require.Len(t, eibcEvents, 2)
	fmt.Println("Event:", eibcEvents[1])
	require.Equal(t, eibcEvents[1].PacketStatus, "PENDING")

	// Get the balance of dymensionUserAddr and marketMakerAddr before fulfill the demand order
	dymensionUserBalance, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	marketMakerBalance, err := dymension.GetBalance(ctx, marketMakerAddr, dymension.Config().Denom)
	require.NoError(t, err)

	// fulfill demand order
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[1].ID, marketMakerAddr)
	require.NoError(t, err)
	fmt.Println(txhash)
	eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	if eibcEvent != nil {
		fmt.Println("After order fulfillment:", eibcEvent)
	}

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	fmt.Println("Balance changed of dymensionUserAddr after fulfilling the order:", balance.Sub(dymensionUserBalance))
	require.True(t, balance.Sub(dymensionUserBalance).Equal(transferAmountWithoutFee), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee, balance.Sub(dymensionUserBalance)))
	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, dymension.Config().Denom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceDymDenom := marketMakerBalance.Sub((transferAmountWithoutFee))
	require.True(t, balance.Equal(expMmBalanceDymDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceDymDenom, balance))
	// wait until packet finalization, mm balance should be the same due to the ack error
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, dymension.Config().Denom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	require.True(t, balance.Equal(expMmBalanceDymDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceDymDenom, balance))

	// check if the fund returns to rollapp
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferAmount)
	t.Cleanup(
		func() {
			err := r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}
