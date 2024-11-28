package tests

import (
	"context"
	"fmt"
	"strconv"
	"testing"

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

func TestIBCTransferBetweenHub3rd_EVM(t *testing.T) {
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

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

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
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
		{
			Name:          "gaia-1",
			Version:       "v14.2.0",
			ChainConfig:   gaiaConfig,
			NumValidators: &numVals,
			NumFullNodes:  &numFullNodes,
		},
	})
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)
	gaia := chains[2].(*cosmos.CosmosChain)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	r2 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddChain(gaia).
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  gaia,
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

	t.Cleanup(func() {
		_ = ic.Close()
	})

	// create ibc path between dymension and gaia
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, gaia, anotherIbcPath)

	gaiaChan, err := r2.GetChannels(ctx, eRep, gaia.GetChainID())
	require.NoError(t, err)
	require.Len(t, gaiaChan, 1)

	dymGaiaChan := gaiaChan[0].Counterparty
	require.NotEmpty(t, dymGaiaChan.ChannelID)

	gaiaDymChan := gaiaChan[0]
	require.NotEmpty(t, gaiaDymChan.ChannelID)

	// Start the relayer and set the cleanup function.
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err = r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer2: %s", err)
			}
		},
	)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, gaia)

	// Get our Bech32 encoded user addresses
	dymensionUser, gaiaUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gaiaOrigBal)

	t.Run("canonial client gaia<->dym", func(t *testing.T) {

		firstHopDenom := transfertypes.GetPrefixedDenom(dymGaiaChan.PortID, dymGaiaChan.ChannelID, gaia.Config().Denom)
		secondHopDenom := transfertypes.GetPrefixedDenom(gaiaDymChan.PortID, gaiaDymChan.ChannelID, dymension.Config().Denom)

		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

		// Send packet from gaia -> dym
		transfer := ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   gaia.Config().Denom,
			Amount:  transferAmount,
		}

		transferTx, err := gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaiaUser.KeyName(), transfer, ibc.TransferOptions{})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		// wait until dymension receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, gaia.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, transferAmount)

		// Send back packet from dym -> gaia
		transfer = ibc.WalletData{
			Address: gaiaUserAddr,
			Denom:   dymension.Config().Denom,
			Amount:  transferAmount,
		}

		transferTx, err = dymension.SendIBCTransfer(ctx, dymGaiaChan.ChannelID, dymensionUser.KeyName(), transfer, ibc.TransferOptions{})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		// wait until gaia receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, gaia, dymension)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, secondHopIBCDenom, transferAmount)
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferAmount))

	})
}

// TestIBCTransferRA3rdSameChainID_EVM create vanilla cosmos chain with the same chain-id as an existing rollapp and test IBC transfer between them
func TestIBCTransferRA_3rdSameChainID_EVM(t *testing.T) {
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

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// add modify gaia config so that it has the same chain-id as rollapp1
	gaiaConfig := gaiaConfig.Clone()
	gaiaConfig.ChainID = "rollappevm_1234-1"

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
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
		{
			Name:          "gaia-1",
			Version:       "v14.2.0",
			ChainConfig:   gaiaConfig,
			NumValidators: &numVals,
			NumFullNodes:  &numFullNodes,
		},
	})
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)
	gaia := chains[2].(*cosmos.CosmosChain)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	r := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)

	r2 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddChain(gaia).
		AddRelayer(r, "relayer").
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  gaia,
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

	t.Cleanup(func() {
		_ = ic.Close()
	})

	// create ibc path between dymension and gaia, and between dymension and rollapp1
	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, gaia, anotherIbcPath)

	// get rollapp -> dym channel
	rollappChan, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, rollappChan, 1)

	rollappDymChan := rollappChan[0]
	require.NotEmpty(t, rollappDymChan.ChannelID)

	dymRollappChan := rollappChan[0].Counterparty
	require.NotEmpty(t, dymRollappChan.ChannelID)

	// Get gaia -> dym channel
	gaiaChan, err := r2.GetChannels(ctx, eRep, gaia.GetChainID())
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

	t.Cleanup(
		func() {
			err = r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
			err = r2.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer2: %s", err)
			}
		},
	)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, gaia, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, gaiaUser, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gaiaOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	t.Run("canonial client test gaia<->dym and rollapp<->dym", func(t *testing.T) {

		// sending between gaia and dymension
		firstHopDenom := transfertypes.GetPrefixedDenom(dymGaiaChan.PortID, dymGaiaChan.ChannelID, gaia.Config().Denom)
		secondHopDenom := transfertypes.GetPrefixedDenom(gaiaDymChan.PortID, gaiaDymChan.ChannelID, dymension.Config().Denom)

		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

		// Send packet from gaia -> dym
		transfer := ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   gaia.Config().Denom,
			Amount:  transferAmount,
		}

		transferTx, err := gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaiaUser.KeyName(), transfer, ibc.TransferOptions{})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		// wait until dymension receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, gaia.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, transferAmount)

		// Send back packet from dym -> gaia
		transfer = ibc.WalletData{
			Address: gaiaUserAddr,
			Denom:   dymension.Config().Denom,
			Amount:  transferAmount,
		}

		transferTx, err = dymension.SendIBCTransfer(ctx, dymGaiaChan.ChannelID, dymensionUser.KeyName(), transfer, ibc.TransferOptions{})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		// wait until gaia receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, gaia, dymension)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, secondHopIBCDenom, transferAmount)

		// sending between rollapp and dymension
		firstHopDenom = transfertypes.GetPrefixedDenom(dymRollappChan.PortID, dymRollappChan.ChannelID, rollapp1.Config().Denom)
		secondHopDenom = transfertypes.GetPrefixedDenom(rollappDymChan.PortID, rollappDymChan.ChannelID, dymension.Config().Denom)

		firstHopDenomTrace = transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace = transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom = firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom = secondHopDenomTrace.IBCDenom()

		// Send packet from rollapp -> dym
		transfer = ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   rollapp1.Config().Denom,
			Amount:  transferAmount,
		}

		_, err = rollapp1.SendIBCTransfer(ctx, rollappDymChan.ChannelID, rollappUser.KeyName(), transfer, ibc.TransferOptions{})
		require.NoError(t, err)

		err = testutil.WaitForBlocks(ctx, 10, dymension)
		require.NoError(t, err)

		// wait until dymension receive transferAmount when rollapp finalized
		rollappHeight, err := rollapp1.GetNode().Height(ctx)
		require.NoError(t, err)

		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetNode().Chain.GetChainID(), rollappHeight, 600)
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

		testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, transferAmount.Sub(bridgingFee))

		// Send back packet from dym -> rollapp
		transfer = ibc.WalletData{
			Address: rollappUserAddr,
			Denom:   dymension.Config().Denom,
			Amount:  transferAmount,
		}

		_, err = dymension.SendIBCTransfer(ctx, dymRollappChan.ChannelID, dymensionUser.KeyName(), transfer, ibc.TransferOptions{})
		require.NoError(t, err)

		// wait until rollapp receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, rollapp1, dymension)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferAmount).Sub(transferAmount))
		erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
		require.NoError(t, err)
		erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
		testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, secondHopIBCDenom, transferAmount)

		// Run invariant check
		CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
	})
}

func TestIBCTransfer_NoLightClient_EVM(t *testing.T) {
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

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// add modify gaia config so that it has the same chain-id as rollapp1
	gaiaConfig := gaiaConfig.Clone()
	gaiaConfig.ChainID = "rollappevm_1234-1"

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
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
		{
			Name:          "gaia-1",
			Version:       "v14.2.0",
			ChainConfig:   gaiaConfig,
			NumValidators: &numVals,
			NumFullNodes:  &numFullNodes,
		},
	})
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)
	gaia := chains[2].(*cosmos.CosmosChain)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	r := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddChain(gaia).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  gaia,
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

	t.Cleanup(func() {
		_ = ic.Close()
	})

	// create ibc path between dymension and gaia, and between dymension and rollapp1
	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, gaia, ibcPath)

	// Get gaia -> dym channel
	gaiaChan, err := r.GetChannels(ctx, eRep, gaia.GetChainID())
	require.NoError(t, err)
	require.Len(t, gaiaChan, 1)

	dymGaiaChan := gaiaChan[0].Counterparty
	require.NotEmpty(t, dymGaiaChan.ChannelID)

	gaiaDymChan := gaiaChan[0]

	// Start the relayer and set the cleanup function.
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, gaia, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, gaiaUser, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gaiaOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	t.Run("canonial client test gaia<->dym", func(t *testing.T) {

		// sending between gaia and dymension
		firstHopDenom := transfertypes.GetPrefixedDenom(dymGaiaChan.PortID, dymGaiaChan.ChannelID, gaia.Config().Denom)
		secondHopDenom := transfertypes.GetPrefixedDenom(gaiaDymChan.PortID, gaiaDymChan.ChannelID, dymension.Config().Denom)

		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

		// Send packet from gaia -> dym
		transfer := ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   gaia.Config().Denom,
			Amount:  transferAmount,
		}

		transferTx, err := gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaiaUser.KeyName(), transfer, ibc.TransferOptions{})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		// wait until dymension receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, gaia.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, transferAmount)

		// Send back packet from dym -> gaia
		transfer = ibc.WalletData{
			Address: gaiaUserAddr,
			Denom:   dymension.Config().Denom,
			Amount:  transferAmount,
		}

		transferTx, err = dymension.SendIBCTransfer(ctx, dymGaiaChan.ChannelID, dymensionUser.KeyName(), transfer, ibc.TransferOptions{})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		// wait until gaia receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, gaia, dymension)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, secondHopIBCDenom, transferAmount)

		// Run invariant check
		CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
	})
}
