package tests

import (
	"context"
	"fmt"
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

// TestADMC_Hub_to_RA_EVM send IBC transfer for a non-existing denom from hub to rollapp with setting {"transferinject":{}} in the memo
// ensures IBC transfer rejected with error, suspected malicious activity
func TestADMC_Hub_to_RA_reserved_EVM(t *testing.T) {
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
	rollapp2_id := "rollappevm_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "1s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "100s")
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
				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
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

	// multiplier := math.NewInt(10)

	// eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	// transferAmountWithoutFee := transferAmount.Sub(eibcFee)

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
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount.QuoRaw(2),
	}

	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

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

	// Send from dymension to rollapp with reserved word in memo
	transferData = ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   gaiaIBCDenom,
		Amount:  bigTransferAmount,
	}

	// Second hop
	_, err = dymension.SendIBCTransfer(ctx, channsRollApp1[0].Counterparty.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{
		Memo: `{"transferinject":{}}`,
	})
	require.NoError(t, err)

	// catch ACK errors
	// rollapp1Height, err := rollapp1.Height(ctx)
	// require.NoError(t, err)

	// ack, err := testutil.PollForAck(ctx, rollapp1, rollapp1Height, rollapp1Height+80, ibcTx.Packet)
	// require.NoError(t, err)

	// // Make sure that the ack contains error
	// require.True(t, bytes.Contains(ack.Acknowledgement, []byte("error")))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// with the use of the reserved word in the memo, the transfer should be rejected
	balance, err = rollapp1.GetBalance(ctx, rollapp1UserAddr, secondHopIBCDenom)
	require.NoError(t, err)
	require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

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
