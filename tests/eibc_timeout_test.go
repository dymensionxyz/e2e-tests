package tests

import (
	"context"
	"fmt"
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

// This test case verifies the system's behavior when an eIBC packet is from dym to rollapp and it times out.
// It verifies that new demand order is automatically created when that happens

func TestEIBCTimeoutDymToRollapp_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_max_time"] = "100s"
	dymintTomlOverrides["batch_submit_time"] = "20s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

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
	dymension := chains[1].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)
	const ibcPath = "ibc-path"
	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r, "relayer").
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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMakerUser, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMakerUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Compose an IBC transfer and send from hub to rollapp
	// global eibc fee in case of auto created orders is 0.0015
	numerator := math.NewInt(15)
	denominator := math.NewInt(10000)
	globalEIbcFee := transferAmount.Mul(numerator).Quo(denominator)
	transferAmountWithoutFee := transferAmount.Sub(globalEIbcFee)

	// Set a short timeout for IBC transfer
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 1000000, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from dym -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)
	// Get the IBC denom for dymension on roll app
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, zeroBal)

	// According to delayedack module, we need the rollapp to have finalizedHeight > ibcClientLatestHeight
	// in order to trigger ibc timeout or else it will trigger callback
	err = testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
	require.NoError(t, err)

	// get eibc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 40, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents[0])
	require.Equal(t, eibcEvents[0].Price, fmt.Sprintf("%s%s", transferAmountWithoutFee, dymension.Config().Denom))
	require.Equal(t, eibcEvents[0].Fee, fmt.Sprintf("%s%s", globalEIbcFee, dymension.Config().Denom))

	// fulfill demand order
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[0].OrderId, marketMakerAddr, globalEIbcFee)
	require.NoError(t, err)
	fmt.Println(txhash)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }
	// require.True(t, eibcEvent.IsFulfilled)

	// wait a few blocks and verify sender received funds on the dymension
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	expBalance := walletAmount.Sub(transferData.Amount).Add(transferAmountWithoutFee)
	require.True(t, balance.Equal(expBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalance, balance))

	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, dymension.Config().Denom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expBalanceMarketMaker := walletAmount.Add(transferAmountWithoutFee).Add(globalEIbcFee)
	require.True(t, balance.Equal(expBalanceMarketMaker), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalanceMarketMaker, balance))

	// wait until packet finalization and verify funds (incl. fee) were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, dymension.Config().Denom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expBalanceMarketMaker = expBalanceMarketMaker.Add(transferData.Amount)
	require.True(t, balance.Equal(expBalanceMarketMaker), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalanceMarketMaker, balance))

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

// TestEIBCTimeoutFulFillDymToRollapp test send 3rd party IBC denom from dymension to rollapp with timeout
// and full filled
func TestEIBCTimeoutFulFillDymToRollapp_EVM(t *testing.T) {
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
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)
	// relayer for rollapp gaia
	r3 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer3", network)

	const ibcPath = "ibc-path"
	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddChain(gaia).
		AddRelayer(r, "relayer").
		AddRelayer(r3, "relayer3").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
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

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r3, eRep, dymension.CosmosChain, gaia, ibcPath)

	channsDym, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym, 2)

	rollAppChan, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, rollAppChan, 1)

	dymRollAppChan := rollAppChan[0].Counterparty
	require.NotEmpty(t, dymRollAppChan.ChannelID)

	rollappDymChan := rollAppChan[0]
	require.NotEmpty(t, rollappDymChan.ChannelID)

	gaiaChan, err := r3.GetChannels(ctx, eRep, gaia.GetChainID())
	require.NoError(t, err)
	require.Len(t, gaiaChan, 1)

	dymGaiaChan := gaiaChan[0].Counterparty
	require.NotEmpty(t, dymGaiaChan.ChannelID)

	gaiaDymChan := gaiaChan[0]
	require.NotEmpty(t, gaiaDymChan.ChannelID)

	// Start the relayer and set the cleanup function.
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r3.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
			err = r3.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer2: %s", err)
			}
		},
	)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, gaia, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, gaiaUser, marketMakerUser, rollappUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	gaianUserAddr := gaiaUser.FormattedAddress()
	marketMakerAddr := marketMakerUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, gaia, gaianUserAddr, gaia.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	// global eibc fee in case of auto created orders is 0.0015
	numerator := math.NewInt(15)
	denominator := math.NewInt(10000)
	globalEIbcFee := transferAmount.Mul(numerator).Quo(denominator)
	transferAmountWithoutFee := transferAmount.Sub(globalEIbcFee)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp1.SendIBCTransfer(ctx, rollappDymChan.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Set a short timeout for IBC transfer
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: testutil.ImmediatelyTimeout().NanoSeconds, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	gaiaToDymTransferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   gaia.Config().Denom,
		Amount:  transferAmount,
	}

	gaiaToMMTransferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   gaia.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from gaiai -> dym and market maker
	_, err = gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaianUserAddr, gaiaToDymTransferData, ibc.TransferOptions{})
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, gaia, gaianUserAddr, gaia.Config().Denom, walletAmount.Sub(gaiaToDymTransferData.Amount))

	_, err = gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaianUserAddr, gaiaToMMTransferData, ibc.TransferOptions{})
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, gaia, gaianUserAddr, gaia.Config().Denom, walletAmount.Sub(gaiaToDymTransferData.Amount).Sub(gaiaToMMTransferData.Amount))

	// Get the IBC denom for gaia on dym
	gaiaTokenDenom := transfertypes.GetPrefixedDenom(dymGaiaChan.PortID, dymGaiaChan.ChannelID, gaia.Config().Denom)
	gaiaIBCDenom := transfertypes.ParseDenomTrace(gaiaTokenDenom).IBCDenom()

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1, gaia)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, gaiaIBCDenom, gaiaToDymTransferData.Amount)

	dymToRollAppTransferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   gaiaIBCDenom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from dym -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, dymRollAppChan.ChannelID, dymensionUserAddr, dymToRollAppTransferData, options)
	require.NoError(t, err)
	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)
	// Assert balance was updated on the dym
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, gaiaIBCDenom, zeroBal)
	// Get the IBC denom of 3rd party token on roll app
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(rollappDymChan.PortID, rollappDymChan.ChannelID, gaiaIBCDenom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, zeroBal)

	// According to delayedack module, we need the rollapp to have finalizedHeight > ibcClientLatestHeight
	// in order to trigger ibc timeout or else it will trigger callback
	err = testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
	require.NoError(t, err)

	// get eibc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents[0])
	require.Equal(t, eibcEvents[0].Price, fmt.Sprintf("%s%s", transferAmountWithoutFee, gaiaIBCDenom))
	require.Equal(t, eibcEvents[0].Fee, fmt.Sprintf("%s%s", globalEIbcFee, gaiaIBCDenom))

	// fulfill demand order
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[0].OrderId, marketMakerAddr, globalEIbcFee)
	require.NoError(t, err)
	fmt.Println(txhash)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }
	// require.True(t, eibcEvent.IsFulfilled)

	// wait a few blocks and verify sender received funds on the dymension
	err = testutil.WaitForBlocks(ctx, 3, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err := dymension.GetBalance(ctx, dymensionUserAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	expBalance := gaiaToDymTransferData.Amount.Sub(dymToRollAppTransferData.Amount).Add(transferAmountWithoutFee)
	require.True(t, balance.Equal(expBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalance, balance))
	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expBalanceMarketMaker := gaiaToMMTransferData.Amount.Sub(transferAmountWithoutFee)
	require.True(t, balance.Equal(expBalanceMarketMaker), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalanceMarketMaker, balance))
	// wait until packet finalization and verify funds (incl. fee) were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expBalanceMarketMaker = expBalanceMarketMaker.Add(dymToRollAppTransferData.Amount)
	require.True(t, balance.Equal(expBalanceMarketMaker), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalanceMarketMaker, balance))

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
	// Check the commitment was deleted
	resp, err := dymension.GetNode().QueryPacketCommitments(ctx, "transfer", dymRollAppChan.ChannelID)
	require.NoError(t, err)
	require.Equal(t, len(resp.Commitments) == 0, true, "packet commitments still exist")
}

func TestEIBCTimeoutFulFillDymToRollapp_Wasm(t *testing.T) {
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
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
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
		AddRelayer(r, "relayer1").
		AddRelayer(r2, "relayer2").
		AddRelayer(r3, "relayer3").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
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
	}, nil, "", nil)
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)
	CreateChannel(ctx, t, r3, eRep, dymension.CosmosChain, gaia, ibcPath)

	channsDym, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym, 3)

	rollAppChan, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, rollAppChan, 1)

	dymRollAppChan := rollAppChan[0].Counterparty
	require.NotEmpty(t, dymRollAppChan.ChannelID)

	rollappDymChan := rollAppChan[0]
	require.NotEmpty(t, rollappDymChan.ChannelID)

	gaiaChan, err := r3.GetChannels(ctx, eRep, gaia.GetChainID())
	require.NoError(t, err)
	require.Len(t, gaiaChan, 1)

	dymGaiaChan := gaiaChan[0].Counterparty
	require.NotEmpty(t, dymGaiaChan.ChannelID)

	gaiaDymChan := gaiaChan[0]
	require.NotEmpty(t, gaiaDymChan.ChannelID)

	// Start the relayer and set the cleanup function.
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = r3.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
			err = r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
			err = r3.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer2: %s", err)
			}
		},
	)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, gaia, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, gaiaUser, marketMakerUser, rollappUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	gaianUserAddr := gaiaUser.FormattedAddress()
	marketMakerAddr := marketMakerUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, gaia, gaianUserAddr, gaia.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	// global eibc fee in case of auto created orders is 0.0015
	numerator := math.NewInt(15)
	denominator := math.NewInt(10000)
	globalEIbcFee := transferAmount.Mul(numerator).Quo(denominator)
	transferAmountWithoutFee := transferAmount.Sub(globalEIbcFee)

	// Set a short timeout for IBC transfer
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: testutil.ImmediatelyTimeout().NanoSeconds, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp1.SendIBCTransfer(ctx, rollappDymChan.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	gaiaToDymTransferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   gaia.Config().Denom,
		Amount:  transferAmount,
	}

	gaiaToMMTransferData := ibc.WalletData{
		Address: marketMakerAddr,
		Denom:   gaia.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from gaiai -> dym and market maker
	_, err = gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaianUserAddr, gaiaToDymTransferData, ibc.TransferOptions{})
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, gaia, gaianUserAddr, gaia.Config().Denom, walletAmount.Sub(gaiaToDymTransferData.Amount))

	_, err = gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaianUserAddr, gaiaToMMTransferData, ibc.TransferOptions{})
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, gaia, gaianUserAddr, gaia.Config().Denom, walletAmount.Sub(gaiaToDymTransferData.Amount).Sub(gaiaToMMTransferData.Amount))

	// Get the IBC denom for gaia on dym
	gaiaTokenDenom := transfertypes.GetPrefixedDenom(dymGaiaChan.PortID, dymGaiaChan.ChannelID, gaia.Config().Denom)
	gaiaIBCDenom := transfertypes.ParseDenomTrace(gaiaTokenDenom).IBCDenom()

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1, gaia)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, gaiaIBCDenom, gaiaToDymTransferData.Amount)

	dymToRollAppTransferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   gaiaIBCDenom,
		Amount:  transferAmount,
	}

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)
	// Get the IBC denom of 3rd party token on roll app before sending the transaction to make sure demand order is fulfilled
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(rollappDymChan.PortID, rollappDymChan.ChannelID, gaiaIBCDenom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()
	// Compose an IBC transfer and send from dym -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, dymRollAppChan.ChannelID, dymensionUserAddr, dymToRollAppTransferData, options)
	require.NoError(t, err)

	// Assert balance was updated on the dym
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, gaiaIBCDenom, zeroBal)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, zeroBal)

	// get eibc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)

	// fulfill demand order
	txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvents[0].OrderId, marketMakerAddr, globalEIbcFee)
	// Pushing event assertion later in the test so that fulfill demand order can always be fulfilled
	fmt.Println("Event:", eibcEvents[0])
	require.Equal(t, eibcEvents[0].Price, fmt.Sprintf("%s%s", transferAmountWithoutFee, gaiaIBCDenom))
	require.Equal(t, eibcEvents[0].Fee, fmt.Sprintf("%s%s", globalEIbcFee, gaiaIBCDenom))

	require.NoError(t, err)
	fmt.Println(txhash)
	// eibcEvent := getEibcEventFromTx(t, dymension, txhash)
	// if eibcEvent != nil {
	// 	fmt.Println("After order fulfillment:", eibcEvent)
	// }
	// require.True(t, eibcEvent.IsFulfilled)

	// wait a few blocks and verify sender received funds on the dymension
	err = testutil.WaitForBlocks(ctx, 3, dymension)
	require.NoError(t, err)

	// verify funds minus fee were added to receiver's address
	balance, err := dymension.GetBalance(ctx, dymensionUserAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	expBalance := gaiaToDymTransferData.Amount.Sub(dymToRollAppTransferData.Amount).Add(transferAmountWithoutFee)
	require.True(t, balance.Equal(expBalance), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalance, balance))
	// verify funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expBalanceMarketMaker := gaiaToMMTransferData.Amount.Sub(transferAmountWithoutFee)
	require.True(t, balance.Equal(expBalanceMarketMaker), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalanceMarketMaker, balance))
	// wait until packet finalization and verify funds (incl. fee) were added to market maker's wallet address
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	balance, err = dymension.GetBalance(ctx, marketMakerAddr, gaiaIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after packet finalization:", balance)
	expBalanceMarketMaker = expBalanceMarketMaker.Add(dymToRollAppTransferData.Amount)
	require.True(t, balance.Equal(expBalanceMarketMaker), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expBalanceMarketMaker, balance))

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
	// Check the commitment was deleted
	resp, err := dymension.GetNode().QueryPacketCommitments(ctx, "transfer", dymRollAppChan.ChannelID)
	require.NoError(t, err)
	require.Equal(t, len(resp.Commitments) == 0, true, "packet commitments still exist")
}
