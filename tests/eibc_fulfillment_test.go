package tests

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
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

// TestEIBCFulfillOnOneRollApp tests the case where we have 2 rollapps running, we have pending eIBC packets from both rollapps. one of the packets is fulfilled by the market maker, the other packet is not fulfilled. The test verifies that the funds are transferred correctly and the market maker's balance is updated correctly (after grace period).
func TestEIBCFulfillOnOneRollApp_EVM(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

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

	dymChannel_ra1, err := r1.GetChannels(ctx, eRep, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra1))
	channDymRollApp1 := dymChannel_ra1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	dymChannel_ra2, err := r2.GetChannels(ctx, eRep, rollapp2.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra2))
	channDymRollApp2 := dymChannel_ra2[0].Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp2.SendIBCTransfer(ctx, dymChannel_ra2[0].ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	transferDataRollapp1 := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  bigTransferAmount,
	}

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(dymChannel_ra1[0].Counterparty.PortID, dymChannel_ra1[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Get the IBC denom for Rollapp 2's token on Hub
	rollapp2TokenDenom := transfertypes.GetPrefixedDenom(dymChannel_ra2[0].Counterparty.PortID, dymChannel_ra2[0].Counterparty.ChannelID, rollapp2.Config().Denom)
	rollapp2IBCDenom := transfertypes.ParseDenomTrace(rollapp2TokenDenom).IBCDenom()

	var options ibc.TransferOptions
	// market maker needs to have funds on the hub first to be able to fulfill incoming demand order
	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferDataRollapp1, options)
	require.NoError(t, err)
	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized on Rollapps
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := (bigTransferAmount.Sub(bigBridgingFee))
	balance, err := dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after preconditions:", balance)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// end of preconditions

	transferDataRollapp1 = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	transferDataRollapp2 := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp2.SendIBCTransfer(ctx, dymChannel_ra2[0].ChannelID, rollapp2UserAddr, transferDataRollapp2, options)
	require.NoError(t, err)
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer from rollapp 2 to dym hub:", balance)
	require.True(t, balance.Equal(transferAmount.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmount.Sub(bridgingFee), balance))

	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferDataRollapp1, options)
	require.NoError(t, err)
	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer from rollapp 1 to dym hub:", balance)
	require.True(t, balance.Equal(transferAmount.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmount.Sub(bridgingFee), balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	var fulfill_demand_order = false
	// fulfill demand orders from rollapp 1
	for _, eibcEvent := range eibcEvents {
		re := regexp.MustCompile(`^\d+`)
		if re.ReplaceAllString(eibcEvent.Price, "") == rollappIBCDenom && eibcEvent.PacketStatus == "PENDING" {
			fmt.Println("EIBC Event:", eibcEvent)
			txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvent.OrderId, marketMakerAddr, eibcFee)
			require.NoError(t, err)
			fmt.Println(txhash)
			// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
			// if eibcEvent != nil {
			// 	fmt.Println("After order fulfillment:", eibcEvent)
			// }
			fulfill_demand_order = true
		}
	}

	require.Equal(t, true, fulfill_demand_order)

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Add(transferAmount.Sub(bridgingFee)).Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Add(transferAmount.Sub(bridgingFee)).Sub(bridgingFee), balance))

	// verify correct funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub((transferAmount)).Add(eibcFee).Add(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 200)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Add(transferDataRollapp1.Amount)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	// user should have received funds upon grace period of IBC packet from rollapp 2
	err = testutil.WaitForBlocks(ctx, 30, rollapp2)
	require.NoError(t, err)
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr for rollapp 2 ibc denom after grace period:", balance)
	require.True(t, balance.Equal((transferDataRollapp2.Amount.Sub(bridgingFee)).MulRaw(2)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", (transferDataRollapp2.Amount.Sub(bridgingFee)).MulRaw(2), balance))

	// No packet commitments left
	res, err := rollapp1.GetNode().QueryPacketCommitments(ctx, "transfer", dymChannel_ra1[0].ChannelID)
	require.NoError(t, err)
	require.Equal(t, len(res.Commitments) > 0, false, "packet commitments exist")

	res, err = rollapp2.GetNode().QueryPacketCommitments(ctx, "transfer", dymChannel_ra2[0].ChannelID)
	require.NoError(t, err)
	require.Equal(t, len(res.Commitments) > 0, false, "packet commitments exist")

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

// TestEIBCFulfillOnOneRollApp tests the case where we have 2 rollapps running, we have pending eIBC packets from both rollapps. one of the packets is fulfilled by the market maker, the other packet is not fulfilled. The test verifies that the funds are transferred correctly and the market maker's balance is updated correctly (after grace period).
func TestEIBCFulfillOnOneRollApp_Wasm(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

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

	dymChannel_ra1, err := r1.GetChannels(ctx, eRep, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra1))
	channDymRollApp1 := dymChannel_ra1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	dymChannel_ra2, err := r2.GetChannels(ctx, eRep, rollapp2.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra2))
	channDymRollApp2 := dymChannel_ra2[0].Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp2.SendIBCTransfer(ctx, dymChannel_ra2[0].ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	transferDataRollapp1 := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  bigTransferAmount,
	}

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(dymChannel_ra1[0].Counterparty.PortID, dymChannel_ra1[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Get the IBC denom for Rollapp 2's token on Hub
	rollapp2TokenDenom := transfertypes.GetPrefixedDenom(dymChannel_ra2[0].Counterparty.PortID, dymChannel_ra2[0].Counterparty.ChannelID, rollapp2.Config().Denom)
	rollapp2IBCDenom := transfertypes.ParseDenomTrace(rollapp2TokenDenom).IBCDenom()

	var options ibc.TransferOptions
	// market maker needs to have funds on the hub first to be able to fulfill incoming demand order
	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferDataRollapp1, options)
	require.NoError(t, err)
	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized on Rollapps
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := (bigTransferAmount.Sub(bigBridgingFee))
	balance, err := dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after preconditions:", balance)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// end of preconditions

	transferDataRollapp1 = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	transferDataRollapp2 := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp2.SendIBCTransfer(ctx, dymChannel_ra2[0].ChannelID, rollapp2UserAddr, transferDataRollapp2, options)
	require.NoError(t, err)
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer from rollapp 2 to dym hub:", balance)
	require.True(t, balance.Equal(transferAmount.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmount.Sub(bridgingFee), balance))

	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferDataRollapp1, options)
	require.NoError(t, err)
	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(transferAmount.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmount.Sub(bridgingFee), balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	var fulfill_demand_order = false
	// fulfill demand orders from rollapp 1
	for _, eibcEvent := range eibcEvents {
		re := regexp.MustCompile(`^\d+`)
		if re.ReplaceAllString(eibcEvent.Price, "") == rollappIBCDenom && eibcEvent.PacketStatus == "PENDING" {
			_, err := dymension.FullfillDemandOrder(ctx, eibcEvent.OrderId, marketMakerAddr, eibcFee)
			require.NoError(t, err)
			// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
			// if eibcEvent != nil {
			// 	fmt.Println("After order fulfillment:", eibcEvent)
			// }
			fulfill_demand_order = true
		}
	}

	require.Equal(t, true, fulfill_demand_order)

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Add(transferAmount.Sub(bridgingFee)).Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Add(transferAmount.Sub(bridgingFee)).Sub(bridgingFee), balance))

	// verify correct funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub((transferAmount)).Add(eibcFee).Add(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 200)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Add(transferDataRollapp1.Amount)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	// user should have received funds upon grace period of IBC packet from rollapp 2
	err = testutil.WaitForBlocks(ctx, 30, rollapp2)
	require.NoError(t, err)
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr for rollapp 2 ibc denom after grace period:", balance)
	require.True(t, balance.Equal((transferDataRollapp2.Amount.Sub(bridgingFee)).MulRaw(2)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", (transferDataRollapp2.Amount.Sub(bridgingFee)).MulRaw(2), balance))

	// No packet commitments left
	res, err := rollapp1.GetNode().QueryPacketCommitments(ctx, "transfer", dymChannel_ra1[0].ChannelID)
	require.NoError(t, err)
	require.Equal(t, len(res.Commitments) > 0, false, "packet commitments exist")

	res, err = rollapp2.GetNode().QueryPacketCommitments(ctx, "transfer", dymChannel_ra2[0].ChannelID)
	require.NoError(t, err)
	require.Equal(t, len(res.Commitments) > 0, false, "packet commitments exist")

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

// This test case verifies the system's behavior when an eIBC packet sent from the rollapp to the hub
// that is fulfilled by the market maker
func TestEIBCFulfillment_EVM(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	transferAmountWithoutFee := transferAmount.Sub(eibcFee)

	dymChannel, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 2, len(dymChannel))

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel[0].Counterparty.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 30, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(dymChannel[0].Counterparty.PortID, dymChannel[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	var options ibc.TransferOptions
	// Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := transferData.Amount.Sub(transferAmount.Quo(math.NewInt(1000)))
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

	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel[0].ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)

	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents)

	// fulfill demand order
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[0].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	fmt.Println(txhash)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Sub(bridgingFee), balance))
	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub((transferAmountWithoutFee)).Add(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Add(transferData.Amount)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

func TestEIBCFulfillment_Wasm(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	transferAmountWithoutFee := transferAmount.Sub(eibcFee)

	dymChannels, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 2, len(dymChannels))

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp2, 1)

	require.Len(t, dymChannels, 2)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub

	transferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].PortID, channsRollApp1[0].ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	var options ibc.TransferOptions

	// Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := transferAmount.Sub(transferAmount.Quo(math.NewInt(1000)))
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

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)

	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents[0])

	// fulfill demand order
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[0].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	fmt.Println(txhash)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Sub(bridgingFee), balance))
	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub((transferAmountWithoutFee)).Add(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Add(transferData.Amount)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

func TestEIBCFulfillment_two_rollapps_EVM(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

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
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1, rollapp2)
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

	dymChannel_ra1, err := r1.GetChannels(ctx, eRep, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra1))
	channDymRollApp1 := dymChannel_ra1[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	dymChannel_ra2, err := r2.GetChannels(ctx, eRep, rollapp2.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra2))
	channDymRollApp2 := dymChannel_ra2[0].Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	transferData = ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp2.SendIBCTransfer(ctx, dymChannel_ra2[0].ChannelID, rollapp2UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	rollapp2Height, err := rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollapp2Height, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(dymChannel_ra1[0].Counterparty.PortID, dymChannel_ra1[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Get the IBC denom for Rollapp 2's token on Hub
	rollapp2TokenDenom := transfertypes.GetPrefixedDenom(dymChannel_ra2[0].Counterparty.PortID, dymChannel_ra2[0].Counterparty.ChannelID, rollapp2.Config().Denom)
	rollapp2IBCDenom := transfertypes.ParseDenomTrace(rollapp2TokenDenom).IBCDenom()

	var options ibc.TransferOptions
	// market maker needs to have funds on the hub first to be able to fulfill incoming demand order
	// Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := transferAmount.Sub(transferAmount.Quo(math.NewInt(1000)))
	balance, err := dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after preconditions:", balance)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	// Precondition funds check on Hub for Rollapp 2
	balanceRollapp2, err := dymension.GetBalance(ctx, marketMakerAddr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after preconditions for", rollapp2IBCDenom, ":", balance)
	require.True(t, balanceRollapp2.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balanceRollapp2))
	// end of preconditions

	transferDataRollapp1 := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	// Send funds from Rollapp 2 to Hub and set eIBC specific memo
	transferDataRollapp2 := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp2.SendIBCTransfer(ctx, dymChannel_ra2[0].ChannelID, rollapp2UserAddr, transferDataRollapp2, options)
	require.NoError(t, err)
	// rollapp2Height, err = rollapp2.GetNode().Height(ctx)
	// require.NoError(t, err)
	zeroBalance := math.NewInt(0)
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer from rollapp 2 to dym hub:", balance)
	require.True(t, balance.Equal(zeroBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBalance, balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	require.Equal(t, eibcEvents[len(eibcEvents)-1].PacketStatus, "PENDING")

	// fulfill demand order 1
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[len(eibcEvents)-1].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	fmt.Println(txhash)
	eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	if eibcEvent != nil {
		fmt.Println("After order fulfillment:", eibcEvent)
	}

	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferDataRollapp1, options)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer from rollapp 1 to dym hub:", balance)
	require.True(t, balance.Equal(zeroBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBalance, balance))

	// get eIbc event
	eibcEvents, err = getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	require.Equal(t, eibcEvents[len(eibcEvents)-1].PacketStatus, "PENDING")

	// fulfill demand order 2
	txhash, err = dymension.FullfillDemandOrder(ctx, eibcEvents[len(eibcEvents)-1].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	fmt.Println(txhash)
	eibcEvent = getEibcEventFromTx(t, dymension, txhash)
	if eibcEvent != nil {
		fmt.Println("After order fulfillment:", eibcEvent)
	}

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Sub(bridgingFee), balance))

	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance for ibc denom from rollapp 2 of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Sub(bridgingFee), balance))

	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Sub(transferAmountWithoutFee).Add(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, rollapp2IBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Add(transferAmount).Sub(bridgingFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

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

// This test case verifies the system's behavior when an eIBC packet sent from the rollapp to the hub
// with third party ibc token that is fulfilled by the market maker
func TestEIBCFulfillment_ThirdParty_EVM(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

	// Disable erc20
	modifyRollappGeneisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.erc20.params.enable_erc20",
			Value: false,
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1
	numVals := 1
	numFullNodes := 0
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
				ModifyGenesis:       modifyRollappEVMGenesis(modifyRollappGeneisKV),
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
				ModifyGenesis:       modifyRollappEVMGenesis(modifyRollappGeneisKV),
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
				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
				ConfigFileOverrides: nil,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
		{
			Name:          "gaia",
			Version:       "v14.2.0",
			ChainConfig:   gaiaConfig,
			NumValidators: &numVals,
			NumFullNodes:  &numFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)
	gaia := chains[3].(*cosmos.CosmosChain)

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

	r3 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer3", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddChain(gaia).
		AddRelayer(r1, "relayer1").
		AddRelayer(r2, "relayer2").
		AddRelayer(r3, "relayer3").
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
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  gaia,
			Relayer: r3,
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

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r3, eRep, dymension.CosmosChain, gaia, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1, gaia)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, rollapp1User, gaiaUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, gaia.Config().Denom, walletAmount)

	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	transferAmountWithoutFee := transferAmount.Sub(eibcFee)

	dymChannels, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 3, len(dymChannels))

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp2, 1)

	gaiaChannels, err := r3.GetChannels(ctx, eRep, gaia.GetChainID())
	require.NoError(t, err)

	require.Len(t, dymChannels, 3)
	require.Len(t, gaiaChannels, 1)

	channDymGaia := gaiaChannels[0].Counterparty
	require.NotEmpty(t, channDymGaia.ChannelID)

	channGaiaDym := gaiaChannels[0]
	require.NotEmpty(t, channGaiaDym.ChannelID)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r3.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   gaia.Config().Denom,
		Amount:  bigTransferAmount,
	}

	// Get the IBC denom
	gaiaTokenDenom := transfertypes.GetPrefixedDenom(channDymGaia.PortID, channDymGaia.ChannelID, gaia.Config().Denom)
	gaiaIBCDenom := transfertypes.ParseDenomTrace(gaiaTokenDenom).IBCDenom()

	secondHopDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].PortID, channsRollApp1[0].ChannelID, gaiaTokenDenom)
	secondHopIBCDenom := transfertypes.ParseDenomTrace(secondHopDenom).IBCDenom()

	// First hop
	var options ibc.TransferOptions
	_, err = gaia.SendIBCTransfer(ctx, channGaiaDym.ChannelID, gaiaUserAddr, transferData, options)
	require.NoError(t, err)

	t.Log("gaiaIBCDenom:", gaiaIBCDenom)

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	balance, err := dymension.GetBalance(ctx, dymensionUserAddr, gaiaIBCDenom)
	require.NoError(t, err)
	require.True(t, balance.Equal(bigTransferAmount), fmt.Sprintf("Value mismatch. Expected %s, actual %s", bigTransferAmount, balance))

	transferData = ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   gaiaIBCDenom,
		Amount:  bigTransferAmount,
	}

	// Second hop
	_, err = dymension.SendIBCTransfer(ctx, channsRollApp1[0].Counterparty.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)

	balance, err = rollapp1.GetBalance(ctx, rollapp1UserAddr, secondHopIBCDenom)
	require.NoError(t, err)
	require.True(t, balance.Equal(bigTransferAmount), fmt.Sprintf("Value mismatch. Expected %s, actual %s", bigTransferAmount, balance))

	transferData = ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   secondHopIBCDenom,
		Amount:  transferAmount,
	}

	// market maker needs to have funds on the hub first to be able to fulfill upcoming demand order
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollapp1UserAddr, transferData, options)
	require.NoError(t, err)

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	expBalance := transferData.Amount.Sub(transferData.Amount.Quo(math.NewInt(1000)))
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	require.True(t, balance.Equal(expBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalance, balance))
	// done preparation

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   secondHopIBCDenom,
		Amount:  transferAmount,
	}

	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollapp1UserAddr, transferData, options)
	require.NoError(t, err)

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents[0])

	// fulfill demand order
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[0].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	fmt.Println(txhash)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Sub(bridgingFee), balance))

	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalance := transferAmount.Sub((transferAmountWithoutFee))
	require.True(t, balance.Equal(expMmBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalance, balance))

	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expMmBalance = expMmBalance.Add(transferData.Amount).Sub(bridgingFee)
	require.True(t, balance.Equal(expMmBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalance, balance))

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

			err = r3.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

// This test case verifies the system's behavior when an eIBC packet sent from the rollapp to the hub
// with third party ibc token that is fulfilled by the market maker
func TestEIBCFulfillment_ThirdParty_Wasm(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1
	numVals := 1
	numFullNodes := 0
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
				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
				ConfigFileOverrides: nil,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
		{
			Name:          "gaia",
			Version:       "v14.2.0",
			ChainConfig:   gaiaConfig,
			NumValidators: &numVals,
			NumFullNodes:  &numFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)
	gaia := chains[3].(*cosmos.CosmosChain)

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
	// Relayer for gaia
	r3 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer3", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddChain(gaia).
		AddRelayer(r1, "relayer1").
		AddRelayer(r2, "relayer2").
		AddRelayer(r3, "relayer3").
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
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  gaia,
			Relayer: r3,
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

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r3, eRep, dymension.CosmosChain, gaia, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1, gaia)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, rollapp1User, gaiaUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, gaia.Config().Denom, walletAmount)

	bigTransferAmount := math.NewInt(1_000_000_000)
	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	transferAmountWithoutFee := transferAmount.Sub(eibcFee)

	dymChannels, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 3, len(dymChannels))

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp2, 1)

	gaiaChannels, err := r3.GetChannels(ctx, eRep, gaia.GetChainID())
	require.NoError(t, err)

	require.Len(t, dymChannels, 3)
	require.Len(t, gaiaChannels, 1)

	channDymGaia := gaiaChannels[0].Counterparty
	require.NotEmpty(t, channDymGaia.ChannelID)

	channGaiaDym := gaiaChannels[0]
	require.NotEmpty(t, channGaiaDym.ChannelID)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r3.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   gaia.Config().Denom,
		Amount:  bigTransferAmount,
	}

	// Get the IBC denom
	gaiaTokenDenom := transfertypes.GetPrefixedDenom(channDymGaia.PortID, channDymGaia.ChannelID, gaia.Config().Denom)
	gaiaIBCDenom := transfertypes.ParseDenomTrace(gaiaTokenDenom).IBCDenom()

	secondHopDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].PortID, channsRollApp1[0].ChannelID, gaiaTokenDenom)
	secondHopIBCDenom := transfertypes.ParseDenomTrace(secondHopDenom).IBCDenom()

	// First hop
	var options ibc.TransferOptions
	_, err = gaia.SendIBCTransfer(ctx, channGaiaDym.ChannelID, gaiaUserAddr, transferData, options)
	require.NoError(t, err)

	t.Log("gaiaIBCDenom:", gaiaIBCDenom)

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	balance, err := dymension.GetBalance(ctx, dymensionUserAddr, gaiaIBCDenom)
	require.NoError(t, err)
	require.True(t, balance.Equal(bigTransferAmount), fmt.Sprintf("Value mismatch. Expected %s, actual %s", bigTransferAmount, balance))

	transferData = ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   gaiaIBCDenom,
		Amount:  bigTransferAmount,
	}

	// Second hop
	_, err = dymension.SendIBCTransfer(ctx, channsRollApp1[0].Counterparty.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)

	balance, err = rollapp1.GetBalance(ctx, rollapp1UserAddr, secondHopIBCDenom)
	require.NoError(t, err)
	require.True(t, balance.Equal(bigTransferAmount), fmt.Sprintf("Value mismatch. Expected %s, actual %s", bigTransferAmount, balance))

	transferData = ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   secondHopIBCDenom,
		Amount:  transferAmount,
	}

	// market maker needs to have funds on the hub first to be able to fulfill upcoming demand order
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollapp1UserAddr, transferData, options)
	require.NoError(t, err)

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	require.True(t, balance.Equal(transferAmount.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmount.Sub(bridgingFee), balance))

	// done preparation

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   secondHopIBCDenom,
		Amount:  transferAmount,
	}

	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollapp1UserAddr, transferData, options)
	require.NoError(t, err)

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents[0])

	// fulfill demand order
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[0].OrderId, marketMakerAddr, eibcFee)
	require.NoError(t, err)
	fmt.Println(txhash)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmountWithoutFee.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Sub(bridgingFee), balance))

	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalance := transferAmount.Sub((transferAmountWithoutFee))
	require.True(t, balance.Equal(expMmBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalance, balance))

	// wait until packet finalization and verify funds + fee were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expMmBalance = expMmBalance.Add(transferData.Amount).Sub(bridgingFee)
	require.True(t, balance.Equal(expMmBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalance, balance))

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

			err = r3.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

func TestEIBCFulfillment_ignore_hub_to_RA_EVM(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

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
	}, nil, "", nil)
	require.NoError(t, err)

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1, rollapp2)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1, rollapp2)
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

	dymChannel_ra1, err := r1.GetChannels(ctx, eRep, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra1))
	channDymRollApp1 := dymChannel_ra1[0]
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	dymChannel_ra2, err := r2.GetChannels(ctx, eRep, rollapp2.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra2))
	channDymRollApp2 := dymChannel_ra2[0].Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	transferDataRollapp1 := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	options := ibc.TransferOptions{}
	options.Memo = BuildEIbcMemo(eibcFee)

	// send fund from dymension to rollapp1
	_, err = dymension.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, dymensionUserAddr, transferDataRollapp1, options)
	require.NoError(t, err)

	// get eIbc event
	_, err = getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	// expect no eibc events created as ibc transfer from hub to rollapp is ignored
	require.Error(t, err, "There wasn't a single 'eibc' event registered within the specified block range on the hub")

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

func TestEIBCFulfillment_ignore_hub_to_RA_Wasm(t *testing.T) {
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
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "decentrio_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "50s")

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
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1, rollapp2)
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

	dymChannel_ra1, err := r1.GetChannels(ctx, eRep, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra1))
	channDymRollApp1 := dymChannel_ra1[0]
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	dymChannel_ra2, err := r2.GetChannels(ctx, eRep, rollapp2.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 1, len(dymChannel_ra2))
	channDymRollApp2 := dymChannel_ra2[0].Counterparty
	require.NotEmpty(t, channDymRollApp2.ChannelID)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)
	// end of preconditions

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	transferDataRollapp1 := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	options := ibc.TransferOptions{}
	options.Memo = BuildEIbcMemo(eibcFee)

	// send fund from dymension to rollapp1
	_, err = dymension.SendIBCTransfer(ctx, dymChannel_ra1[0].ChannelID, dymensionUserAddr, transferDataRollapp1, options)
	require.NoError(t, err)

	// get eIbc event
	_, err = getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	// expect no eibc events created as ibc transfer from hub to rollapp is ignored
	require.Error(t, err, "There wasn't a single 'eibc' event registered within the specified block range on the hub")

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
