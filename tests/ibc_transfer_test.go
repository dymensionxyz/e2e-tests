package tests

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

func TestSpinUp(t *testing.T) {
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
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
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

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1)

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
}

// TestIBCTransferSuccess ensure that the transfer between Hub and Rollapp is accurate.
func TestIBCTransferSuccess_EVM(t *testing.T) {
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
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
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

	// Update white listed relayers
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

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

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
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
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

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> rollapp
	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

// TestIBCTransferSuccess ensure that the transfer between Hub and Rollapp is accurate.
func TestIBCTransferSuccess_Wasm(t *testing.T) {
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
	dymintTomlOverrides["da_config"] = []string{""}
	dymintTomlOverrides["da_layer"] = []string{"mock"}

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
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
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

	// Update white listed relayers
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

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

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

	// Compose an IBC transfer and send from rollapp -> Hub
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
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

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> rollapp
	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferAmount)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func TestIBCTransferRAToRA_EVM(t *testing.T) {
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
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
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

	wallet1, found := r1.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	wallet2, found := r2.GetWallet(rollapp2.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	keyDir2 := dymension.GetRollApps()[1].GetSequencerKeyDir()
	require.NoError(t, err)
	keyPath2 := keyDir2 + "/sequencer_keys"

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet1.FormattedAddress()})
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

	err = testutil.WaitForBlocks(ctx, 2, dymension)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath2, []string{wallet2.FormattedAddress()})
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

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser1, rollappUser2 := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr1 := rollappUser1.FormattedAddress()
	rollappUserAddr2 := rollappUser2.FormattedAddress()

	channelRA1, err := ibc.GetTransferChannel(ctx, r1, eRep, rollapp1.Config().ChainID, dymension.Config().ChainID)
	require.NoError(t, err)

	channelRA2, err := ibc.GetTransferChannel(ctx, r2, eRep, rollapp2.Config().ChainID, dymension.Config().ChainID)
	require.NoError(t, err)

	fmt.Println("abcd")
	fmt.Println(rollappUserAddr2)
	fmt.Println(channelRA2)
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  bigTransferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channelRA1.ChannelID, rollappUserAddr1, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr1, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

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

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channelRA1.Counterparty.PortID, channelRA1.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, bigTransferAmount.Sub(bigBridgingFee))

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1
	var options ibc.TransferOptions
	// set eIBC specific memo
	memo, err := dymension.GetNode().GetMemo(ctx, eibcFee.String(), channelRA2.Counterparty.ChannelID, rollappUserAddr2, "5m")
	require.NoError(t, err)

	memo = strings.ReplaceAll(memo, "\n", "")

	options.Memo = memo
	_, err = rollapp1.SendIBCTransfer(ctx, channelRA1.ChannelID, rollappUserAddr1, transferData, options)
	require.NoError(t, err)

	balance, err := dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer from rollapp 1 to rollapp2 hub:", balance)
	// require.True(t, balance.Equal(transferAmount.Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmount.Sub(bridgingFee), balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	fulfill_demand_order := false
	// fulfill demand orders from rollapp 1
	for _, eibcEvent := range eibcEvents {
		re := regexp.MustCompile(`^\d+`)
		if re.ReplaceAllString(eibcEvent.Price, "") == rollappIBCDenom && eibcEvent.PacketStatus == "PENDING" {
			fmt.Println("EIBC Event:", eibcEvent)
			txhash, err := dymension.FullfillDemandOrder(ctx, eibcEvent.OrderId, dymensionUserAddr, eibcFee)
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
	// require.True(t, balance.Equal(transferAmountWithoutFee.Add(transferAmount.Sub(bridgingFee)).Sub(bridgingFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmountWithoutFee.Add(transferAmount.Sub(bridgingFee)).Sub(bridgingFee), balance))

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	rollappHeight2, err := rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized on Rollapps
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// wait until the packet is finalized on Rollapps
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollappHeight2, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	// No packet commitments left
	resp, err := rollapp1.GetNode().QueryPacketCommitments(ctx, "transfer", "channel-0")
	require.NoError(t, err)
	require.Equal(t, len(resp.Commitments) > 0, false, "packet commitments exist")

	resp, err = rollapp2.GetNode().QueryPacketCommitments(ctx, "transfer", "channel-1")
	require.NoError(t, err)
	require.Equal(t, len(resp.Commitments) > 0, false, "packet commitments exist")

	expMmBalanceRollappDenom := (bigTransferAmount.Sub(bigBridgingFee))
	// verify correct funds were deducted from market maker's wallet address
	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, rollappIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of marketMakerAddr after fulfilling the order:", balance)
	expMmBalanceRollappDenom = expMmBalanceRollappDenom.Add(eibcFee)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))

	rollappTokenDenom2 := transfertypes.GetPrefixedDenom(channelRA2.PortID, channelRA2.ChannelID, rollappTokenDenom)
	rollappIBCDenom2 := transfertypes.ParseDenomTrace(rollappTokenDenom2).IBCDenom()

	erc20MAcc, err := rollapp2.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, rollapp2, erc20MAccAddr, rollappIBCDenom2, transferData.Amount.Sub(eibcFee).Sub(bridgingFee))
}

// // TestGenesisIBCTransferWithReservedMemo_EVM ensure that after the genesis transfer phase for rollapp is completed, transfer memo contain a reserved name field cannot be used.
// func TestGenesisIBCTransferReservedMemo_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	configFileOverrides := make(map[string]any)
// 	dymintTomlOverrides := make(testutil.Toml)
// 	dymintTomlOverrides["settlement_layer"] = "dymension"
// 	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
// 	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
// 	dymintTomlOverrides["max_idle_time"] = "3s"
// 	dymintTomlOverrides["max_proof_time"] = "500ms"
// 	dymintTomlOverrides["batch_submit_time"] = "50s"

// 	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1

// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappevm_1234-1",
// 				Images:              []ibc.DockerImage{rollappEVMImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "ethm",
// 				Denom:               "urax",
// 				CoinType:            "60",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
// 				ConfigFileOverrides: configFileOverrides,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name:          "dymension-hub",
// 			ChainConfig:   dymensionConfig,
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddRelayer(r, "relayer").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollappUser := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollappUserAddr := rollappUser.FormattedAddress()

// 	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount.QuoRaw(2),
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// Assert balance was updated on the hub
// 	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Get the IBC denom for urax on Hub
// 	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

// 	// Minus 0.1% of transfer amount for bridge fee
// 	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee).QuoRaw(2))

// 	// Send ibc transfer with reserved memo after genesis phase is completed
// 	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{
// 		Memo: `{"genesis_transfer": {}}`,
// 	})
// 	require.ErrorContains(t, err, "cannot use transfer genesis memo: unauthorized")

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err = rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Assert fund was not transferred
// 	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))
// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
// }

// // TestGenesisIBCTransferWithReservedMemo_Wasm ensure that after the genesis transfer phase for rollapp is completed, transfer memo contain a reserved name field cannot be used.
// func TestGenesisIBCTransferReservedMemo_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	configFileOverrides := make(map[string]any)
// 	dymintTomlOverrides := make(testutil.Toml)
// 	dymintTomlOverrides["settlement_layer"] = "dymension"
// 	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
// 	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
// 	dymintTomlOverrides["max_idle_time"] = "3s"
// 	dymintTomlOverrides["max_proof_time"] = "500ms"
// 	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
// 	dymintTomlOverrides["batch_submit_time"] = "50s"

// 	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1

// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp",
// 				ChainID:             "rollappwasm_1234-1",
// 				Images:              []ibc.DockerImage{rollappWasmImage},
// 				Bin:                 "rollappd",
// 				Bech32Prefix:        "rol",
// 				Denom:               "urax",
// 				CoinType:            "118",
// 				GasPrices:           "0.0urax",
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				EncodingConfig:      encodingConfig(),
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
// 				ConfigFileOverrides: configFileOverrides,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name:          "dymension-hub",
// 			ChainConfig:   dymensionConfig,
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddRelayer(r, "relayer").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		})

// 	rep := testreporter.NewNopReporter()
// 	eRep := rep.RelayerExecReporter(t)

// 	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
// 		TestName:         t.Name(),
// 		Client:           client,
// 		NetworkID:        network,
// 		SkipPathCreation: true,

// 		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
// 		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
// 	}, nil, "", nil, false, 1179360, true)
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollappUser := users[0], users[1]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollappUserAddr := rollappUser.FormattedAddress()

// 	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
// 	require.NoError(t, err)

// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Send a normal ibc tx from RA -> Hub
// 	transferData := ibc.WalletData{
// 		Address: dymensionUserAddr,
// 		Denom:   rollapp1.Config().Denom,
// 		Amount:  transferAmount.QuoRaw(2),
// 	}
// 	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// Assert balance was updated on the hub
// 	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// wait until the packet is finalized
// 	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Get the IBC denom for urax on Hub
// 	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
// 	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

// 	// Minus 0.1% of transfer amount for bridge fee
// 	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee).QuoRaw(2))

// 	// Send ibc transfer with reserved memo after genesis phase is completed
// 	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{
// 		Memo: `{"genesis_transfer": {}}`,
// 	})
// 	require.ErrorContains(t, err, "cannot use transfer genesis memo: unauthorized")

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	rollappHeight, err = rollapp1.GetNode().Height(ctx)
// 	require.NoError(t, err)

// 	// wait until the packet is finalized
// 	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 	require.NoError(t, err)
// 	require.True(t, isFinalized)

// 	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
// 	require.NoError(t, err)

// 	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
// 	require.NoError(t, err)

// 	// Assert fund was not transferred
// 	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

// 	// Run invariant check
// 	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
// }
