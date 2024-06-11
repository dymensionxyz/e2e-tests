package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestERC20HubToRollAppWithoutRegister_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1

	// Enable erc20
	modifyRollappGeneisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.erc20.params.enable_erc20",
			Value: true,
		},
	)
	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
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
				Denom:               "urax",
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappEVMGenesis(modifyRollappGeneisKV),
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
	})
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

	channel, err := ibc.GetTransferChannel(ctx, r1, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	rollapp := rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: channel.ChannelID,
		userKey:   dymensionUser.KeyName(),
	}
	triggerHubGenesisEvent(t, dymension, rollapp)

	// Get the IBC denom of Hub on rollapp
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	transferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)
	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	balance, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(walletAmount.Sub(transferAmount)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// verify funds weren't added to receiver's address
	balance, err = rollapp1.GetBalance(ctx, rollappUserAddr, dymensionIBCDenom)
	require.NoError(t, err)
	fmt.Println("Balance of rollappUserAddr right after sending eIBC transfer:", balance)
	require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents[0])

	// wait a few blocks and verify sender received funds on the hub
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.DemandOrders))

	// wait until packet finalization and verify funds
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)
	balance, err = rollapp1.GetBalance(ctx, rollappUserAddr, dymensionIBCDenom)
	require.NoError(t, err)
	require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferData.Amount, balance))

	balance, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.True(t, balance.Equal(walletAmount), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferData.Amount, balance))

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

func TestERC20RollAppToHubWithRegister_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1

	// Enable erc20
	modifyRollappGeneisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.erc20.params.enable_erc20",
			Value: true,
		},
	)
	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
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
				Denom:               "urax",
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappEVMGenesis(modifyRollappGeneisKV),
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
	})
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

	channel, err := ibc.GetTransferChannel(ctx, r1, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	rollapp := rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: channel.ChannelID,
		userKey:   dymensionUser.KeyName(),
	}
	triggerHubGenesisEvent(t, dymension, rollapp)

	metadataCoin := banktypes.Metadata{
		Description: "description of the token",
		Base:        "urax",
		// NOTE: Denom units MUST be increasing
		DenomUnits: []*banktypes.DenomUnit{
			{
				Denom:    "urax",
				Exponent: 0,
			},
			{
				Denom:    "rax",
				Exponent: uint32(18),
			},
		},
		Name:    "urax",
		Symbol:  "Decentrio",
		Display: "urax",
	}

	data := map[string][]banktypes.Metadata{
		"metadata": {metadataCoin},
	}

	firstHeight, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	contentFile, err := json.Marshal(data)
	require.NoError(t, err)
	rollapp1.GetNode().WriteFile(ctx, contentFile, "./ibcmetadata.json")
	deposit := "500000000000" + rollapp1.Config().Denom
	rollapp1.GetNode().HostName()
	_, err = rollapp1.GetNode().RegisterIBCTokenDenomProposal(ctx, rollappUser.KeyName(), deposit, rollapp1.GetNode().HomeDir()+"/ibcmetadata.json")
	require.NoError(t, err)

	err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	secondHeight, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	var contract string
	for h := firstHeight; h <= secondHeight; h++ {
		txs, err := rollapp1.GetNode().FindTxs(ctx, h)
		require.NoError(t, err)
		for _, tx := range txs {
			for _, event := range tx.Events {
				if event.Type == "register_coin" {
					for _, data := range event.Attributes {
						if data.Key == "erc20_token" {
							contract = data.Value
						}
					}
				}
			}
		}
	}
	require.NotNil(t, contract)

	receiver := common.BytesToAddress([]byte(rollappUserAddr))
	coinConvert := types.Coin{Denom: "urax", Amount: transferAmount}
	_, err = rollapp1.GetNode().ConvertCoin(ctx, rollappUser.KeyName(), coinConvert.String(), receiver.String())
	require.NoError(t, err, "can not convert cosmos coin to erc20")
	testutil.WaitForBlocks(ctx, 1, rollapp1)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferAmount))
	
	// convert erc20 back to urax
	err = rollapp1.GetNode().ConvertErc20(ctx, contract, transferAmount.String(), receiver.String(), rollappUserAddr)
 	require.NoError(t, err, "can not convert erc20 to cosmos coin")

	// // Get the IBC denom of Hub on rollapp
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// transferData := ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)
	// // Start relayer
	// err = r1.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// balance, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)
	// fmt.Println("Balance of dymensionUserAddr right after sending eIBC transfer:", balance)
	// require.True(t, balance.Equal(walletAmount.Sub(transferAmount)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// // wait a few blocks and verify sender received funds on the hub
	// err = testutil.WaitForBlocks(ctx, 5, dymension)
	// require.NoError(t, err)

	// // verify funds weren't added to receiver's address
	// balance, err = rollapp1.GetBalance(ctx, rollappUserAddr, dymensionIBCDenom)
	// require.NoError(t, err)
	// fmt.Println("Balance of rollappUserAddr right after sending eIBC transfer:", balance)
	// require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", zeroBal, balance))

	// // get eIbc event
	// eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 30, false)
	// require.NoError(t, err)
	// fmt.Println("Event:", eibcEvents[0])

	// // wait a few blocks and verify sender received funds on the hub
	// err = testutil.WaitForBlocks(ctx, 5, dymension)
	// require.NoError(t, err)

	// resp, err := dymension.QueryEIBCDemandOrders(ctx, "PENDING")
	// require.NoError(t, err)
	// require.Equal(t, 1, len(resp.DemandOrders))

	// // wait until packet finalization and verify funds
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)
	// balance, err = rollapp1.GetBalance(ctx, rollappUserAddr, dymensionIBCDenom)
	// require.NoError(t, err)
	// require.True(t, balance.Equal(zeroBal), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferData.Amount, balance))

	// balance, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)
	// require.True(t, balance.Equal(walletAmount), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferData.Amount, balance))

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}
