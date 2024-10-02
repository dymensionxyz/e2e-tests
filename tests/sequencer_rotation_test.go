package tests

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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

func Test_SeqRotation_OneSeq_DA_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "20s", true)

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	// Check IBC Transfer before switch
	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// // Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// // Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SeqRotation_OneSeq_DA_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappwasm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "20s", true)

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyRollappWasmGenesis(modifyRAGenesisKV),
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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	// Check IBC Transfer before switch
	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// // Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// // Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SeqRotation_NoSeq_DA_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "20s", true)

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	// Check IBC Transfer before switch
	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// // Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// // Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SeqRotation_NoSeq_DA_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappwasm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "20s", true)

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyRollappWasmGenesis(modifyRAGenesisKV),
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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	// Check IBC Transfer before switch
	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// // Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// // Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SeqRotation_NoSeq_P2P_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappevm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	// Check IBC Transfer before switch
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

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

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
	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// Compose an IBC transfer and send from rollapp -> Hub
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Check IBC after switch
	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// Get original account balances
	dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
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
	dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SeqRotation_NoSeq_P2P_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyRollappWasmGenesis(modifyRAGenesisKV),
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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappwasm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	// Check IBC Transfer before switch
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

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

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
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Get the IBC denom for the token on RollApp
	var dymensionTokenDenom, dymensionIBCDenom string
	dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// Assert the balance on Dymension after sending the tokens
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// Query the balance of rollappUserAddr on RollApp
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// Compose an IBC transfer and send from rollapp -> Hub
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Check IBC after switch
	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// Get original account balances
	dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
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
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Get the IBC denom for the token on RollApp
	dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// Assert the balance on Dymension after sending the tokens
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// Query the balance of rollappUserAddr on RollApp
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferData.Amount)

}

func Test_SqcRotation_OneSqc_P2P_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappevm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	// Check IBC Transfer before switch
	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	// Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SqcRotation_OneSqc_P2P_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyRollappWasmGenesis(modifyRAGenesisKV),
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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappwasm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	// Check IBC Transfer before switch
	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	// Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SqcRotation_MulSqc_P2P_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 2

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappevm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	rollapp1HomeDir2 := strings.Split(rollapp1.FullNodes[1].HomeDir(), "/")
	rollapp1FolderName2 := rollapp1HomeDir2[len(rollapp1HomeDir2)-1]

	file, err = os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName2))
	require.NoError(t, err)
	defer file.Close()

	lines = []string{}
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output = strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName2))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[1].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[1].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	// Check IBC Transfer before switch
	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	cmd = append([]string{rollapp1.FullNodes[1].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[1].HomeDir())
	pub1, _, err = rollapp1.FullNodes[1].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.FullNodes[1].HomeDir())
	require.NoError(t, err)

	sequencer, err = dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.FullNodes[1].HomeDir())
	require.NoError(t, err)

	fund = ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command = []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.FullNodes[1].HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 3, "should have 3 sequences")

	// Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SqcRotation_MulSqc_P2P_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 2

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
				ModifyGenesis:       modifyRollappWasmGenesis(modifyRAGenesisKV),
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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappwasm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	rollapp1HomeDir2 := strings.Split(rollapp1.FullNodes[1].HomeDir(), "/")
	rollapp1FolderName2 := rollapp1HomeDir2[len(rollapp1HomeDir2)-1]

	file, err = os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName2))
	require.NoError(t, err)
	defer file.Close()

	lines = []string{}
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output = strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName2))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[1].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[1].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	// Check IBC Transfer before switch
	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	cmd = append([]string{rollapp1.FullNodes[1].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[1].HomeDir())
	pub1, _, err = rollapp1.FullNodes[1].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.FullNodes[1].HomeDir())
	require.NoError(t, err)

	sequencer, err = dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.FullNodes[1].HomeDir())
	require.NoError(t, err)

	fund = ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command = []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.FullNodes[1].HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 3, "should have 3 sequences")

	// Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SeqRotation_MulSeq_DA_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "20s", true)

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 2

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	// Check IBC Transfer before switch
	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// // Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// // Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	cmd = append([]string{rollapp1.FullNodes[1].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[1].HomeDir())
	pub1, _, err = rollapp1.FullNodes[1].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.FullNodes[1].HomeDir())
	require.NoError(t, err)

	sequencer, err = dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.FullNodes[1].HomeDir())
	require.NoError(t, err)

	fund = ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command = []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.FullNodes[1].HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 3, "should have 3 sequences")

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}
func Test_SeqRotation_MulSeq_DA_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappwasm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "20s", true)

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 2

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
				ModifyGenesis:       modifyRollappWasmGenesis(modifyRAGenesisKV),
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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	// Check IBC Transfer before switch
	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// // Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// // Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	cmd = append([]string{rollapp1.FullNodes[1].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[1].HomeDir())
	pub1, _, err = rollapp1.FullNodes[1].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.FullNodes[1].HomeDir())
	require.NoError(t, err)

	sequencer, err = dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.FullNodes[1].HomeDir())
	require.NoError(t, err)

	fund = ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command = []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.FullNodes[1].HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 3, "should have 3 sequences")

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	lastBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	afterBlock, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SeqRotation_Unbond_DA_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "20s", true)

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	// Check IBC Transfer before switch
	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// // Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// // Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Unbond sequencer2
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	seqAddr2, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr2)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr2)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	_, err = rollapp1.Height(ctx)
	require.Error(t, err)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SqcRotation_Unbond_P2P_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappevm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	// Check IBC Transfer before switch
	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	// Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Unbond sequencer2
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	seqAddr2, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr2)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	time.Sleep(180 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDING", queryGetSequencerResponse.Sequencer.Status)

	// Chain halted
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.Error(t, err)

	time.Sleep(300 * time.Second)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr2)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	err = rollapp1.StopAllNodes(ctx)
	require.NoError(t, err)

	_ = rollapp1.StartAllNodes(ctx)

	time.Sleep(30 * time.Second)

	_, err = rollapp1.Height(ctx)
	require.Error(t, err)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}

func Test_SqcRotation_StateUpd_Fail_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "20s", true)

	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.sequencer.params.notice_period",
			Value: "15s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
	)

	modifyRAGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.sequencers.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.staking.params.unbonding_time",
			Value: "300s",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
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

	// relayer for rollapp 1
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
	}, nil, "", nil, true, 195)
	require.NoError(t, err)

	// Check IBC Transfer before switch
	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// // Create some user accounts on both chains
	// users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// // Get our Bech32 encoded user addresses
	// dymensionUser, rollappUser := users[0], users[1]

	// dymensionUserAddr := dymensionUser.FormattedAddress()
	// rollappUserAddr := rollappUser.FormattedAddress()

	// channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	// require.NoError(t, err)

	// err = r.StartRelayer(ctx, eRep, ibcPath)
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Send a normal ibc tx from RA -> Hub
	// transferData := ibc.WalletData{
	// 	Address: dymensionUserAddr,
	// 	Denom:   rollapp1.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// rollappHeight, err := rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	cmd := append([]string{rollapp1.FullNodes[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", rollapp1.FullNodes[0].HomeDir())
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	err = dymension.GetNode().CreateKeyWithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	sequencer, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetNode().HomeDir())
	require.NoError(t, err)

	fund := ibc.WalletData{
		Address: sequencer,
		Denom:   dymension.Config().Denom,
		Amount:  math.NewInt(10_000_000_000_000).MulRaw(100_000_000),
	}
	err = dymension.SendFunds(ctx, "faucet", fund)
	require.NoError(t, err)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	command := []string{"sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "1000000000adym", rollapp1.GetSequencerKeyDir() + "/metadata_sequencer1.json",
		"--broadcast-mode", "async", "--keyring-dir", rollapp1.GetNode().HomeDir() + "/sequencer_keys"}

	_, err = dymension.FullNodes[0].ExecTx(ctx, "sequencer", command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 2, "should have 2 sequences")

	// Unbond sequencer1
	err = dymension.Unbond(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	seqAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", rollapp1.GetSequencerKeyDir())
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	err = dymension.StopAllNodes(ctx)
	require.NoError(t, err)
	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// lastBlock, err := rollapp1.Height(ctx)
	// require.NoError(t, err)

	time.Sleep(16 * time.Second)

	_ = dymension.StartAllNodes(ctx)

	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	require.NoError(t, err)
	// seq still on BONDED 
	require.Equal(t, "OPERATING_STATUS_BONDED", queryGetSequencerResponse.Sequencer.Status)

	// // Chain halted
	// err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	// require.Error(t, err)

	// time.Sleep(300 * time.Second)

	// queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, seqAddr)
	// require.NoError(t, err)
	// require.Equal(t, "OPERATING_STATUS_UNBONDED", queryGetSequencerResponse.Sequencer.Status)

	// err = rollapp1.StopAllNodes(ctx)
	// require.NoError(t, err)

	// _ = rollapp1.StartAllNodes(ctx)

	// time.Sleep(30 * time.Second)

	// afterBlock, err := rollapp1.Height(ctx)
	// require.NoError(t, err)
	// require.True(t, afterBlock > lastBlock)

	// // Compose an IBC transfer and send from rollapp -> Hub
	// _, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check IBC after switch
	// rollappHeight, err = rollapp1.GetNode().Height(ctx)
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// // wait until the packet is finalized
	// isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	// require.NoError(t, err)
	// require.True(t, isFinalized)

	// _, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	// require.NoError(t, err)

	// // Get the IBC denom for urax on Hub
	// rollappTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	// rollappIBCDenom = transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// // Get original account balances
	// dymensionOrigBal, err = dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	// require.NoError(t, err)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount,
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Get the IBC denom
	// dymensionTokenDenom = transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom = transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	// erc20MAcc, err = rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr = erc20MAcc.Account.BaseAccount.Address
	// testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)
}
