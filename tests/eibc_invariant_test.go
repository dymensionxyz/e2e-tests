package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"

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

func TestEIBCInvariant_EVM(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "100s")

	const EPOCH_IDENTIFIER string = "minute"
	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
		cosmos.GenesisKV{
			Key:   "app_state.delayedack.params.epoch_identifier",
			Value: EPOCH_IDENTIFIER,
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
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
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
	}, nil, "", nil)
	require.NoError(t, err)

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1, rollapp2)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, rollappUser, rollapp2User := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	rollapp2UserAddr := rollapp2User.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp2, rollapp2UserAddr, rollapp2.Config().Denom, walletAmount)

	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	transferAmountWithoutFee := transferAmount.Sub(eibcFee)

	dymChannels, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)

	dymRA2Channels, err := r2.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)

	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)

	require.Len(t, channsRollApp1, 1)
	require.Len(t, channsRollApp2, 1)
	require.Len(t, dymChannels, 2)
	require.Len(t, dymRA2Channels, 2)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	newParams, err := dymension.QueryParam(ctx, "delayedack", "EpochIdentifier")
	require.NoError(t, err)
	require.Equal(t, EPOCH_IDENTIFIER, strings.ReplaceAll(newParams.Value, `"`, ""))
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	transferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount.MulRaw(10),
	}

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].Counterparty.PortID, channsRollApp1[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	var options ibc.TransferOptions
	// market maker needs to have funds on the hub first to be able to fulfill upcoming demand order
	// this `normal`` transfer also enable ibc transfer
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)
	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := transferData.Amount.Sub(transferData.Amount.Quo(math.NewInt(1000)))
	balance, err := dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after preconditions:", balance)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// end of preconditions

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	// Send 5 ibc transfer
	for i := 0; i < 5; i++ {
		_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
		require.NoError(t, err)
	}

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)

	// fulfill 2 demand order
	_, err = dymension.FullfillDemandOrder(ctx, eibcEvents[1].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	_, err = dymension.FullfillDemandOrder(ctx, eibcEvents[2].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	// eibcEvent = getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	for i, e := range eibcEvents {
		fmt.Println("Event", i, ":", e)
	}

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Sub(bridgingFee).Mul(math.NewInt(2))), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Sub(bridgingFee).Mul(math.NewInt(2)), balance))
	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(transferAmountWithoutFee.MulRaw(2)).Add(bridgingFee.MulRaw(2))
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Add(transferData.Amount.MulRaw(2))
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(bridgingFee.MulRaw(2))
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	// Send 2 ibc transfer
	for i := 0; i < 2; i++ {
		_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
		require.NoError(t, err)
	}

	// Wait 1 epochs
	ok, err := dymension.WaitUntilEpochEnds(ctx, "minute", 100)
	require.NoError(t, err)
	require.True(t, ok)

	// Run eibc variants
	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "eibc", "demand-order-count")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "eibc", "underlying-packet-exist")
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

func TestEIBCInvariant_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappwasm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "10s"
	maxProofTime := "500ms"
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "100s")

	const EPOCH_IDENTIFIER string = "minute"
	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
		cosmos.GenesisKV{
			Key:   "app_state.delayedack.params.epoch_identifier",
			Value: EPOCH_IDENTIFIER,
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
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
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
	}, nil, "", nil)
	require.NoError(t, err)

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1, rollapp2)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, rollappUser, rollapp2User := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	rollapp2UserAddr := rollapp2User.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp2, rollapp2UserAddr, rollapp2.Config().Denom, walletAmount)

	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	transferAmountWithoutFee := transferAmount.Sub(eibcFee)

	dymChannels, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)

	dymRA2Channels, err := r2.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)

	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)

	require.Len(t, channsRollApp1, 1)
	require.Len(t, channsRollApp2, 1)
	require.Len(t, dymChannels, 2)
	require.Len(t, dymRA2Channels, 2)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	newParams, err := dymension.QueryParam(ctx, "delayedack", "EpochIdentifier")
	require.NoError(t, err)
	require.Equal(t, EPOCH_IDENTIFIER, strings.ReplaceAll(newParams.Value, `"`, ""))

	transferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount.MulRaw(10),
	}

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].Counterparty.PortID, channsRollApp1[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	var options ibc.TransferOptions
	// market maker needs to have funds on the hub first to be able to fulfill upcoming demand order
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)
	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := transferData.Amount.Sub(bridgingFee.MulRaw(10))
	balance, err := dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after preconditions:", balance)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// end of preconditions

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	// Send 5 ibc transfer
	for i := 0; i < 5; i++ {
		_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
		require.NoError(t, err)
	}

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)

	// fulfill 2 demand order
	_, err = dymension.FullfillDemandOrder(ctx, eibcEvents[1].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	_, err = dymension.FullfillDemandOrder(ctx, eibcEvents[2].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	// eibcEvent = getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Sub(bridgingFee).Mul(math.NewInt(2))), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Sub(bridgingFee).Mul(math.NewInt(2)), balance))
	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(transferAmountWithoutFee.MulRaw(2)).Add(bridgingFee.MulRaw(2))
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Add(transferData.Amount.MulRaw(2))
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(bridgingFee.MulRaw(2))
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	// Send 2 ibc transfer
	for i := 0; i < 2; i++ {
		_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
		require.NoError(t, err)
	}

	// Wait 1 epochs
	ok, err := dymension.WaitUntilEpochEnds(ctx, "minute", 100)
	require.NoError(t, err)
	require.True(t, ok)

	// Run eibc variants
	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "eibc", "demand-order-count")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "eibc", "underlying-packet-exist")
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}
