package tests

import (
	"context"
	"strconv"
	"testing"

	// "encoding/json"
	"fmt"

	// "github.com/cosmos/cosmos-sdk/x/params/client/utils"
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

// TestChangeBridgeFeeParam_EVM create a prop to change bridge fee and after prop passed, transfer from rollapp to hub should have the new bridge fee.
func Test_RollAppStateUpdateSuccess_EVM(t *testing.T) {
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

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,

		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

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
				ModifyGenesis:       modifyRollappEVMGenesis(modifyRAGenesisKV),
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
	StartDA(ctx, t, client, network)

	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer1", network)
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
	}, nil, "", nil, false, 1179360, true)
	require.NoError(t, err)

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Start both relayers
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r1, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get ibc denom of rollapp on hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.Counterparty.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // send from rollapp to hub again and make sure new bridge fee is applied
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.Counterparty.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount).Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	// fmt.Println(res)
	// require.NoError(t, err)

	// for _, packet := range res.RollappPackets {

	// 	proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
	// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
	// 	require.NoError(t, err)
	// 	require.True(t, isFinalized)
	// 	txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
	// 	require.NoError(t, err)

	// 	fmt.Println(txhash)
	// }

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee).Add(transferAmount))
	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}