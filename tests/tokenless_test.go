package tests

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"strconv"
	"encoding/json"

	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/cosmos/cosmos-sdk/x/params/client/utils"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestTokenlessCreateERC20_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp
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

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key: "app_state.bank.denom_metadata",
			Value: []interface{}{
				map[string]interface{}{
					"base": "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
					"denom_units": []interface{}{
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
							"exponent": "0",
						},
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "DYM",
							"exponent": "18",
						},
					},
					"description": "Denom metadata for Rollapp EVM",
					"display":     "DYM",
					"name":        "DYM",
					"symbol":      "DYM",
				},
			},
		},
		cosmos.GenesisKV{
			Key:   "app_state.mint.params.mint_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.bond_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.evm.params.evm_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.evm.params.gas_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
	)

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
				Denom:               "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
				CoinType:            "60",
				GasPrices:           "0.0ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappEVMGenesis(modifyEVMGenesisKV),
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
	// relayer for rollapp
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer1", network)
	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r1, "relayer1").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r1,
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

	wallet, found := r1.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
	require.NoError(t, err)

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	privateKey := rollapp1.Validators[0].UnsafeExportETHKey(ctx, rollappUser.KeyName())

	command := []string{"devd", "tx", "deploy-contract", "erc20", "--secret-key", privateKey, "--rpc", "http://ra-rollappevm_1234-1-val-0-TestTokenlessCreateERC20_EVM:8545"}

	stdout, _, err := rollapp1.Validators[0].Exec(ctx, command, nil)
	require.NoError(t, err)

	text := string(stdout)
	// Regular expression to match Ethereum addresses
	re := regexp.MustCompile(`0x[0-9a-fA-F]{40}`)
	addresses := re.FindAllString(text, -1)

	var contractAddress string
	if len(addresses) > 0 {
		// Assuming the last address in the output is the contract address
		contractAddress = addresses[len(addresses)-1]
		fmt.Println("Contract Address:", contractAddress)
	} else {
		fmt.Println("No address found")
	}

	// register erc20
	hash, err := rollapp1.GetNode().RegisterERC20AsToken(ctx, rollappUser.KeyName(), contractAddress)
	require.NoError(t, err, "can not register erc20 to cosmos coin")
	fmt.Println(hash)

	tokenPair, err := rollapp1.GetNode().QueryErc20TokenPair(ctx, contractAddress)
	require.NoError(t, err)
	require.NotNil(t, tokenPair)

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func TestTokenlessTransferSuccess_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp
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

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key: "app_state.bank.denom_metadata",
			Value: []interface{}{
				map[string]interface{}{
					"base": "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
					"denom_units": []interface{}{
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
							"exponent": "0",
						},
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "DYM",
							"exponent": "18",
						},
					},
					"description": "Denom metadata for Rollapp EVM",
					"display":     "DYM",
					"name":        "DYM",
					"symbol":      "DYM",
				},
			},
		},
		cosmos.GenesisKV{
			Key:   "app_state.mint.params.mint_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.bond_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.evm.params.evm_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.evm.params.gas_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
	)

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
				Denom:               "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
				CoinType:            "60",
				GasPrices:           "0.0ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappEVMGenesis(modifyEVMGenesisKV),
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
	// relayer for rollapp
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer1", network)
	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r1, "relayer1").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r1,
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

	wallet, found := r1.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
	require.NoError(t, err)

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	channel, err := ibc.GetTransferChannel(ctx, r1, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	transferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  bigTransferAmount,
	}

	// Get the IBC denom
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, walletAmount.Add(bigTransferAmount))

	// Send a  ibc tx from RA -> Hub
	transferData = ibc.WalletData{
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

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(bigTransferAmount).Add(transferAmount.Sub(bridgingFee)))

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func TestTokenlessTransferDiffGas_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp
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

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key: "app_state.bank.denom_metadata",
			Value: []interface{}{
				map[string]interface{}{
					"base": "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
					"denom_units": []interface{}{
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
							"exponent": "0",
						},
						map[string]interface{}{
							"aliases":  []interface{}{},
							"denom":    "DYM",
							"exponent": "18",
						},
					},
					"description": "Denom metadata for Rollapp EVM",
					"display":     "DYM",
					"name":        "DYM",
					"symbol":      "DYM",
				},
			},
		},
		cosmos.GenesisKV{
			Key:   "app_state.mint.params.mint_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.bond_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.evm.params.evm_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
		cosmos.GenesisKV{
			Key:   "app_state.evm.params.gas_denom",
			Value: "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
		},
	)

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
				Denom:               "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
				CoinType:            "60",
				GasPrices:           "0.0ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappEVMGenesis(modifyEVMGenesisKV),
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
	// relayer for rollapp
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer1", network)
	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r1, "relayer1").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r1,
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

	wallet, found := r1.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
	require.NoError(t, err)

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	channel, err := ibc.GetTransferChannel(ctx, r1, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	transferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  bigTransferAmount,
	}

	// Get the IBC denom
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, walletAmount.Add(bigTransferAmount))

	// Send a  ibc tx from RA -> Hub
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	command := []string{"rollappd", "tx", "ibc-transfer", "transfer", "transfer", "channel-0", transferData.Address, fmt.Sprintf("%s%s", transferData.Amount.String(), transferData.Denom), "--gas", "auto", "--gas-prices", "1.0urax", "--gas-adjustment", "1.1", "--from", rollappUserAddr, "--keyring-backend", "test", "--output", "json", "-y", "--home", rollapp1.HomeDir(), "--node", fmt.Sprintf("tcp://%s:26657", rollapp1.Validators[0].HostName()), "--chain-id", "rollappevm_1234-1"}

	stdout, _, err := rollapp1.Validators[0].Exec(ctx, command, nil)
	require.NoError(t, err)

	fmt.Println(stdout)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(bigTransferAmount))

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func TestUpdateMinGasPrice_EVM(t *testing.T) {
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

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
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

	// Change the min gas price rollapp param using gov
	neeMinGasPriceParam := json.RawMessage(`[
			{
				"denom": "adym",
				"amount": "1"
			}
		]`)
	_, err = rollapp1.GetNode().ParamChangeProposal(ctx, rollappUser.KeyName(),
		&utils.ParamChangeProposalJSON{
			Title:       "Change min gas price param",
			Description: "Change min gas price param",
			Changes: utils.ParamChangesJSON{
				utils.NewParamChangeJSON("rollappparams", "minGasPrices", neeMinGasPriceParam),
			},
			Deposit: "500000000000" + rollapp1.Config().Denom, // greater than min deposit
		})
	require.NoError(t, err)

	err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)
	
	paramsChange, err := rollapp1.GetNode().QueryParam(ctx, "rollappparams", "minGasPrices")
	require.NoError(t, err)
	require.Equal(t,`[{"denom":"adym","amount":"1.000000000000000000"}]`, paramsChange.Value)

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}