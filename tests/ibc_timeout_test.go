package tests

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

// This test case verifies the system's behavior when an IBC packet sent from the rollapp to the hub times out.
func TestIBCTransferTimeout_EVM(t *testing.T) {
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
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{""}
	dymintTomlOverrides["da_layer"] = []string{"mock"}

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1
	logger := zaptest.NewLogger(t)

	cf := test.NewBuiltinChainFactory(logger, []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-test",
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
	}, nil, "", nil, false, 1179360, true)
	require.NoError(t, err)

	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	//Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	// Assert the accounts were funded
	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	/* ---------------------------- timeout RA -> Hub --------------------------- */
	// 1. transfer successfully (happy reference transfer)
	// 2. shut down relayer
	// 3. transfer times out

	logger.Info("Case 1: transfer successfully (happy reference transfer)")
	// Get the IBC denom for urax on Hub
	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	logger.Info("************ sending transfer from rollapp to hub ************")
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the rollapp
	expectedRollappBalance := walletAmount.Sub(transferData.Amount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, expectedRollappBalance)

	// wait until the packet is finalized
	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	require.NoError(t, err)
	fmt.Println("waiting for packets to be finalized", "number of packets:", len(res.RollappPackets))
	for _, packet := range res.RollappPackets {
		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		fmt.Println("waiting for packet to be finalized", packet)
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		_, err = dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)
	}
	// make sure all successfully handled
	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	require.NoError(t, err)
	require.Len(t, res.RollappPackets, 0)

	err = testutil.WaitForBlocks(ctx, 3, dymension)
	require.NoError(t, err)
	// Assert the balance was updated on the hub
	expectedHubBalance := transferData.Amount.Sub(bridgingFee)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, expectedHubBalance)

	logger.Info("************ transfer from rollapp to hub successfully ************")

	logger.Info("Case 2: sending timed-out transfer")
	logger.Info("stopping relayer")
	err = r.StopRelayer(ctx, eRep)
	require.NoError(t, err)

	// hubHeight, err := dymension.GetNode().Height(ctx)
	// require.NoError(t, err)
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 100000, // 100 milliseconds
		},
	}

	// Use same transfer data as before, but with a short timeout
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, expectedRollappBalance.Sub(transferData.Amount))

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	logger.Info("********** Starting relayer again")
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r.UpdateClients(ctx, eRep, ibcPath)
	require.NoError(t, err)

	// enough time for relayer submit the timeout
	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, expectedRollappBalance)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, expectedHubBalance)

	logger.Info("********** time out from RA->Hub refunded successfully ************")

	/* ------------------------- testing hub->RA timeout ------------------------ */
	channel, err = ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Set a short timeout for IBC transfer
	options = ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 100000,
		},
	}

	// Get the IBC denom for dymension on roll app
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	logger.Info("********** Compose an IBC transfer and send from Hub -> rollapp ************")
	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)

	// Assert balance was updated on the rollapp
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, zeroBal)

	// According to delayedack module, we need the rollapp to have finalizedHeight > ibcClientLatestHeight
	// in order to trigger ibc timeout or else it will trigger callback

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// wait until the packet is finalized
	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {
		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	// Assert funds were returned to the sender after the timeout has occurred
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, zeroBal)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)

	logger.Info("********** time out from Hub->RA refunded successfully ************")

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

// This test case verifies the system's behavior when an IBC packet sent from the rollapp to the hub times out.
func TestIBCTransferTimeout_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{""}
	dymintTomlOverrides["da_layer"] = []string{"mock"}

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1
	logger := zaptest.NewLogger(t)
	cf := test.NewBuiltinChainFactory(logger, []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-test",
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
	}, nil, "", nil, false, 1179360, true)
	require.NoError(t, err)

	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	//Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	// Assert the accounts were funded
	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	/* ---------------------------- timeout RA -> Hub --------------------------- */
	// 1. transfer successfully (happy reference transfer)
	// 2. shut down relayer
	// 3. transfer times out

	logger.Info("Case 1: transfer successfully (happy reference transfer)")
	// Get the IBC denom for urax on Hub
	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	logger.Info("************ sending transfer from rollapp to hub ************")
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the rollapp
	expectedRollappBalance := walletAmount.Sub(transferData.Amount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, expectedRollappBalance)

	// wait until the packet is finalized
	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	require.NoError(t, err)
	fmt.Println("waiting for packets to be finalized", "number of packets:", len(res.RollappPackets))
	for _, packet := range res.RollappPackets {
		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		fmt.Println("waiting for packet to be finalized", packet)
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		_, err = dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)
	}
	// make sure all successfully handled
	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	require.NoError(t, err)
	require.Len(t, res.RollappPackets, 0)

	err = testutil.WaitForBlocks(ctx, 3, dymension)
	require.NoError(t, err)
	// Assert the balance was updated on the hub
	expectedHubBalance := transferData.Amount.Sub(bridgingFee)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, expectedHubBalance)

	logger.Info("************ transfer from rollapp to hub successfully ************")

	logger.Info("Case 2: sending timed-out transfer")
	logger.Info("stopping relayer")
	err = r.StopRelayer(ctx, eRep)
	require.NoError(t, err)

	// hubHeight, err := dymension.GetNode().Height(ctx)
	// require.NoError(t, err)
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 100000, // 100 milliseconds
		},
	}

	// Use same transfer data as before, but with a short timeout
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, expectedRollappBalance.Sub(transferData.Amount))

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	logger.Info("********** Starting relayer again")
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r.UpdateClients(ctx, eRep, ibcPath)
	require.NoError(t, err)

	// enough time for relayer submit the timeout
	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, expectedRollappBalance)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, expectedHubBalance)

	logger.Info("********** time out from RA->Hub refunded successfully ************")

	/* ------------------------- testing hub->RA timeout ------------------------ */
	channel, err = ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Set a short timeout for IBC transfer
	options = ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 100000,
		},
	}

	// Get the IBC denom for dymension on roll app
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	logger.Info("********** Compose an IBC transfer and send from Hub -> rollapp ************")
	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)

	// Assert balance was updated on the rollapp
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, zeroBal)

	// According to delayedack module, we need the rollapp to have finalizedHeight > ibcClientLatestHeight
	// in order to trigger ibc timeout or else it will trigger callback

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// wait until the packet is finalized
	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {
		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	// Assert funds were returned to the sender after the timeout has occurred
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, zeroBal)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)

	logger.Info("********** time out from Hub->RA refunded successfully ************")

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}
