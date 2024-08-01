package tests

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"testing"
// 	"time"

// 	"cosmossdk.io/math"
// 	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
// 	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
// 	test "github.com/decentrio/rollup-e2e-testing"
// 	"github.com/decentrio/rollup-e2e-testing/cosmos"
// 	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
// 	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
// 	"github.com/decentrio/rollup-e2e-testing/ibc"
// 	"github.com/decentrio/rollup-e2e-testing/relayer"
// 	"github.com/decentrio/rollup-e2e-testing/testreporter"
// 	"github.com/decentrio/rollup-e2e-testing/testutil"
// 	"github.com/stretchr/testify/require"
// 	"go.uber.org/zap/zaptest"
// )

// func TestIBCPFMWithGracePeriod_EVM(t *testing.T) {
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
// 	dymintTomlOverrides["batch_submit_max_time"] = "30s"

// 	modifyGenesisKV := append(
// 		dymensionGenesisKV,
// 		cosmos.GenesisKV{
// 			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
// 			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
// 		},
// 	)

// 	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	numVals := 1
// 	numFullNodes := 0
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-test",
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
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 		},
// 		{
// 			Name:          "gaia",
// 			Version:       "v15.1.0",
// 			ChainConfig:   gaiaConfig,
// 			NumValidators: &numVals,
// 			NumFullNodes:  &numFullNodes,
// 		},
// 	})
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)
// 	gaia := chains[2].(*cosmos.CosmosChain)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(
// 		ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer", network)

// 	r2 := test.NewBuiltinRelayerFactory(
// 		ibc.CosmosRly,
// 		zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddChain(gaia).
// 		AddRelayer(r, "relayer").
// 		AddRelayer(r2, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  gaia,
// 			Relayer: r2,
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
// 	}, nil, "", nil)
// 	require.NoError(t, err)

// 	t.Cleanup(func() {
// 		_ = ic.Close()
// 	})

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, gaia, ibcPath)

// 	channsDym, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym, 2)

// 	channsRollApp, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp, 1)

// 	channDymRollApp := channsRollApp[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp.ChannelID)

// 	channsRollAppDym := channsRollApp[0]
// 	require.NotEmpty(t, channsRollAppDym.ChannelID)

// 	channsDym, err = r2.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)

// 	channsGaia, err := r2.GetChannels(ctx, eRep, gaia.GetChainID())
// 	require.NoError(t, err)

// 	require.Len(t, channsDym, 2)
// 	require.Len(t, channsGaia, 1)

// 	channDymGaia := channsGaia[0].Counterparty
// 	require.NotEmpty(t, channDymGaia.ChannelID)

// 	channGaiaDym := channsGaia[0]
// 	require.NotEmpty(t, channGaiaDym.ChannelID)

// 	// Start the relayer and set the cleanup function.
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = r2.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err := r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 			err = r2.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer2: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, gaia)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollappUser, gaiaUser := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollappUserAddr := rollappUser.FormattedAddress()
// 	gaiaUserAddr := gaiaUser.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, gaiaOrigBal)

// 	// rollapp := rollappParam{
// 	// 	rollappID: rollapp1.Config().ChainID,
// 	// 	channelID: channDymRollApp.ChannelID,
// 	// 	userKey:   dymensionUser.KeyName(),
// 	// }
// 	// triggerHubGenesisEvent(t, dymension, rollapp)

// 	t.Run("multihop rollapp->dym->gaia, funds received on gaia after grace period", func(t *testing.T) {
// 		firstHopDenom := transfertypes.GetPrefixedDenom(channDymRollApp.PortID, channDymRollApp.ChannelID, rollapp1.Config().Denom)
// 		secondHopDenom := transfertypes.GetPrefixedDenom(channGaiaDym.PortID, channGaiaDym.ChannelID, firstHopDenom)

// 		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
// 		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

// 		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
// 		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

// 		// Send packet from rollapp1 -> dym -> gaia
// 		transfer := ibc.WalletData{
// 			Address: dymensionUserAddr,
// 			Denom:   rollapp1.Config().Denom,
// 			Amount:  transferAmount,
// 		}

// 		firstHopMetadata := &PacketMetadata{
// 			Forward: &ForwardMetadata{
// 				Receiver: gaiaUserAddr,
// 				Channel:  channDymGaia.ChannelID,
// 				Port:     channDymGaia.PortID,
// 				Timeout:  5 * time.Minute,
// 			},
// 		}

// 		memo, err := json.Marshal(firstHopMetadata)
// 		require.NoError(t, err)

// 		transferTx, err := rollapp1.SendIBCTransfer(ctx, channsRollAppDym.ChannelID, rollappUser.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
// 		require.NoError(t, err)
// 		err = transferTx.Validate()
// 		require.NoError(t, err)

// 		err = testutil.WaitForBlocks(ctx, 10, dymension)
// 		require.NoError(t, err)

// 		rollAppBalance, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
// 		require.NoError(t, err)

// 		dymBalance, err := dymension.GetBalance(ctx, dymensionUserAddr, firstHopIBCDenom)
// 		require.NoError(t, err)

// 		gaiaBalance, err := gaia.GetBalance(ctx, gaiaUserAddr, secondHopIBCDenom)
// 		require.NoError(t, err)

// 		// Make sure that the transfer is not successful yet due to the grace period
// 		require.True(t, rollAppBalance.Equal(walletAmount.Sub(transferAmount)))
// 		require.True(t, dymBalance.Equal(zeroBal))
// 		require.True(t, gaiaBalance.Equal(zeroBal))

// 		rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 		require.NoError(t, err)

// 		// wait until the packet is finalized
// 		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)

// 		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
// 		require.NoError(t, err)

// 		gaiaBalance, err = gaia.GetBalance(ctx, gaiaUserAddr, secondHopIBCDenom)
// 		require.NoError(t, err)

// 		fmt.Println("gaiaaaa", gaiaBalance)
// 		require.True(t, gaiaBalance.Equal(transferAmount))
// 	})
// }

// func TestIBCPFMWithGracePeriod_Wasm(t *testing.T) {
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
// 	dymintTomlOverrides["batch_submit_max_time"] = "30s"

// 	modifyGenesisKV := append(dymensionGenesisKV,
// 		cosmos.GenesisKV{
// 			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
// 			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
// 		},
// 	)

// 	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1
// 	numVals := 1
// 	numFullNodes := 0
// 	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
// 		{
// 			Name: "rollapp1",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-test",
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
// 				ModifyGenesis:       nil,
// 				ConfigFileOverrides: configFileOverrides,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 		},
// 		{
// 			Name:          "gaia",
// 			Version:       "v15.1.0",
// 			ChainConfig:   gaiaConfig,
// 			NumValidators: &numVals,
// 			NumFullNodes:  &numFullNodes,
// 		},
// 	})
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	dymension := chains[1].(*dym_hub.DymHub)
// 	gaia := chains[2].(*cosmos.CosmosChain)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	r := test.NewBuiltinRelayerFactory(
// 		ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer", network)

// 	r2 := test.NewBuiltinRelayerFactory(
// 		ibc.CosmosRly,
// 		zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1).
// 		AddChain(gaia).
// 		AddRelayer(r, "relayer").
// 		AddRelayer(r2, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  gaia,
// 			Relayer: r2,
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
// 	}, nil, "", nil)
// 	require.NoError(t, err)

// 	t.Cleanup(func() {
// 		_ = ic.Close()
// 	})

// 	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, gaia, ibcPath)

// 	channsDym, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym, 2)

// 	channsRollApp, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsRollApp, 1)

// 	channDymRollApp := channsRollApp[0].Counterparty
// 	require.NotEmpty(t, channDymRollApp.ChannelID)

// 	channsRollAppDym := channsRollApp[0]
// 	require.NotEmpty(t, channsRollAppDym.ChannelID)

// 	channsDym, err = r2.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)

// 	channsGaia, err := r2.GetChannels(ctx, eRep, gaia.GetChainID())
// 	require.NoError(t, err)

// 	require.Len(t, channsDym, 2)
// 	require.Len(t, channsGaia, 1)

// 	channDymGaia := channsGaia[0].Counterparty
// 	require.NotEmpty(t, channDymGaia.ChannelID)

// 	channGaiaDym := channsGaia[0]
// 	require.NotEmpty(t, channGaiaDym.ChannelID)

// 	// Start the relayer and set the cleanup function.
// 	err = r.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = r2.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err := r.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 			err = r2.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer2: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, gaia)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollappUser, gaiaUser := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollappUserAddr := rollappUser.FormattedAddress()
// 	gaiaUserAddr := gaiaUser.FormattedAddress()

// 	// Get original account balances
// 	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, dymensionOrigBal)

// 	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, rollappOrigBal)

// 	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
// 	require.NoError(t, err)
// 	require.Equal(t, walletAmount, gaiaOrigBal)

// 	// rollapp := rollappParam{
// 	// 	rollappID: rollapp1.Config().ChainID,
// 	// 	channelID: channDymRollApp.ChannelID,
// 	// 	userKey:   dymensionUser.KeyName(),
// 	// }
// 	// triggerHubGenesisEvent(t, dymension, rollapp)

// 	t.Run("multihop rollapp->dym->gaia, funds received on gaia after grace period", func(t *testing.T) {
// 		firstHopDenom := transfertypes.GetPrefixedDenom(channDymRollApp.PortID, channDymRollApp.ChannelID, rollapp1.Config().Denom)
// 		secondHopDenom := transfertypes.GetPrefixedDenom(channGaiaDym.PortID, channGaiaDym.ChannelID, firstHopDenom)

// 		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
// 		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

// 		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
// 		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

// 		// Send packet from rollapp1 -> dym -> gaia
// 		transfer := ibc.WalletData{
// 			Address: dymensionUserAddr,
// 			Denom:   rollapp1.Config().Denom,
// 			Amount:  transferAmount,
// 		}

// 		firstHopMetadata := &PacketMetadata{
// 			Forward: &ForwardMetadata{
// 				Receiver: gaiaUserAddr,
// 				Channel:  channDymGaia.ChannelID,
// 				Port:     channDymGaia.PortID,
// 				Timeout:  5 * time.Minute,
// 			},
// 		}

// 		memo, err := json.Marshal(firstHopMetadata)
// 		require.NoError(t, err)

// 		transferTx, err := rollapp1.SendIBCTransfer(ctx, channsRollAppDym.ChannelID, rollappUser.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
// 		require.NoError(t, err)
// 		err = transferTx.Validate()
// 		require.NoError(t, err)

// 		err = testutil.WaitForBlocks(ctx, 10, dymension)
// 		require.NoError(t, err)

// 		rollAppBalance, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
// 		require.NoError(t, err)

// 		dymBalance, err := dymension.GetBalance(ctx, dymensionUserAddr, firstHopIBCDenom)
// 		require.NoError(t, err)

// 		gaiaBalance, err := gaia.GetBalance(ctx, gaiaUserAddr, secondHopIBCDenom)
// 		require.NoError(t, err)

// 		// Make sure that the transfer is not successful yet due to the grace period
// 		require.True(t, rollAppBalance.Equal(walletAmount.Sub(transferAmount)))
// 		require.True(t, dymBalance.Equal(zeroBal))
// 		require.True(t, gaiaBalance.Equal(zeroBal))

// 		rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 		require.NoError(t, err)

// 		// wait until the packet is finalized
// 		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)

// 		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
// 		require.NoError(t, err)

// 		gaiaBalance, err = gaia.GetBalance(ctx, gaiaUserAddr, secondHopIBCDenom)
// 		require.NoError(t, err)
// 		fmt.Println("gaiaaaaa", gaiaBalance)
// 		require.True(t, gaiaBalance.Equal(transferAmount))
// 	})
// }

// // PFM with grace period rollApp1 to rollApp2 with Erc20 registed on rollApp2
// func TestIBCPFM_RollApp1To2WithErc20_EVM(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappevm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "30s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "rollappevm_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "3s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "30s")

// 	// Create chain factory with dymension
// 	numHubVals := 1
// 	numHubFullNodes := 1
// 	numRollAppFn := 0
// 	numRollAppVals := 1

// 	modifyRollappGeneisKV := append(
// 		rollappEVMGenesisKV,
// 		cosmos.GenesisKV{
// 			Key:   "app_state.erc20.params.enable_erc20",
// 			Value: true,
// 		},
// 	)

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
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp2",
// 				ChainID:             "rollappevm_12345-1",
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
// 				ModifyGenesis:       modifyRollappEVMGenesis(modifyRollappGeneisKV),
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(dymModifyGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	// relayer for rollapp 1
// 	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)
// 	// relayer for rollapp 2
// 	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r1, "relayer1").
// 		AddRelayer(r2, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r1,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: r2,
// 			Path:    anotherIbcPath,
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
// 	}, nil, "", nil)
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	channsDym, err := r1.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym, 2)

// 	rollapp1Chan, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, rollapp1Chan, 1)

// 	dymRollApp1Chan := rollapp1Chan[0].Counterparty
// 	require.NotEmpty(t, dymRollApp1Chan.ChannelID)

// 	rollapp1DymChan := rollapp1Chan[0]
// 	require.NotEmpty(t, rollapp1DymChan.ChannelID)

// 	rollapp2Chan, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, rollapp2Chan, 1)

// 	dymRollApp2Chan := rollapp2Chan[0].Counterparty
// 	require.NotEmpty(t, dymRollApp2Chan.ChannelID)

// 	rollapp2DymChan := rollapp2Chan[0]
// 	require.NotEmpty(t, rollapp2DymChan.ChannelID)

// 	// Start the relayer and set the cleanup function.
// 	err = r1.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err := r1.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 			err = r2.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer2: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

// 	// Wait a few blocks for relayer to start and for user accounts to be created
// 	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1, rollapp2)
// 	require.NoError(t, err)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()
// 	rollapp2UserAddr := rollapp2User.FormattedAddress()

// 	// Assert the accounts were funded
// 	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
// 	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount)
// 	testutil.AssertBalance(t, ctx, rollapp2, rollapp2UserAddr, rollapp2.Config().Denom, walletAmount)

// 	// rollapp1Param := rollappParam{
// 	// 	rollappID: rollapp1.Config().ChainID,
// 	// 	channelID: dymRollApp1Chan.ChannelID,
// 	// 	userKey:   dymensionUser.KeyName(),
// 	// }

// 	// rollapp2Param := rollappParam{
// 	// 	rollappID: rollapp2.Config().ChainID,
// 	// 	channelID: dymRollApp2Chan.ChannelID,
// 	// 	userKey:   dymensionUser.KeyName(),
// 	// }
// 	// triggerHubGenesisEvent(t, dymension, rollapp1Param, rollapp2Param)

// 	t.Run("multihop rollapp1->dym->rollapp2, funds received on rollapp2 after grace period", func(t *testing.T) {
// 		firstHopDenom := transfertypes.GetPrefixedDenom(dymRollApp1Chan.PortID, dymRollApp1Chan.ChannelID, rollapp1.Config().Denom)
// 		secondHopDenom := transfertypes.GetPrefixedDenom(rollapp2DymChan.PortID, rollapp2DymChan.ChannelID, firstHopDenom)

// 		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
// 		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

// 		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
// 		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

// 		// register ibc denom (secondHopIBCDenom) on rollapp2
// 		metadata := banktypes.Metadata{
// 			Description: "IBC token from Dymension",
// 			DenomUnits: []*banktypes.DenomUnit{
// 				{
// 					Denom:    secondHopIBCDenom,
// 					Exponent: 0,
// 					Aliases:  []string{"urax"},
// 				},
// 				{
// 					Denom:    "urax",
// 					Exponent: 6,
// 				},
// 			},
// 			// Setting base as IBC hash denom since bank keepers's SetDenomMetadata uses
// 			// Base as key path and the IBC hash is what gives this token uniqueness
// 			// on the executing chain
// 			Base:    secondHopIBCDenom,
// 			Display: "urax",
// 			Name:    "urax",
// 			Symbol:  "urax",
// 		}

// 		data := map[string][]banktypes.Metadata{
// 			"metadata": {metadata},
// 		}

// 		contentFile, err := json.Marshal(data)
// 		require.NoError(t, err)
// 		rollapp2.GetNode().WriteFile(ctx, contentFile, "./ibcmetadata.json")
// 		deposit := "500000000000" + rollapp1.Config().Denom
// 		rollapp2.GetNode().HostName()
// 		_, err = rollapp2.GetNode().RegisterIBCTokenDenomProposal(ctx, rollapp2User.KeyName(), deposit, rollapp2.GetNode().HomeDir()+"/ibcmetadata.json")
// 		require.NoError(t, err)

// 		err = rollapp2.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
// 		require.NoError(t, err, "failed to submit votes")

// 		height, err := rollapp2.Height(ctx)
// 		require.NoError(t, err, "error fetching height")
// 		_, err = cosmos.PollForProposalStatus(ctx, rollapp2.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
// 		require.NoError(t, err, "proposal status did not change to passed")

// 		// Send packet from rollapp1 -> dym -> rollapp2
// 		transfer := ibc.WalletData{
// 			Address: dymensionUserAddr,
// 			Denom:   rollapp1.Config().Denom,
// 			Amount:  transferAmount,
// 		}

// 		firstHopMetadata := &PacketMetadata{
// 			Forward: &ForwardMetadata{
// 				Receiver: rollapp2UserAddr,
// 				Channel:  dymRollApp2Chan.ChannelID,
// 				Port:     dymRollApp2Chan.PortID,
// 				Timeout:  5 * time.Minute,
// 			},
// 		}

// 		memo, err := json.Marshal(firstHopMetadata)
// 		require.NoError(t, err)

// 		transferTx, err := rollapp1.SendIBCTransfer(ctx, rollapp2DymChan.ChannelID, rollapp1User.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
// 		require.NoError(t, err)
// 		err = transferTx.Validate()
// 		require.NoError(t, err)

// 		err = testutil.WaitForBlocks(ctx, 10, rollapp1)
// 		require.NoError(t, err)

// 		rollAppBalance, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 		require.NoError(t, err)

// 		dymBalance, err := dymension.GetBalance(ctx, dymensionUserAddr, firstHopIBCDenom)
// 		require.NoError(t, err)

// 		erc20MAcc, err := rollapp2.Validators[0].QueryModuleAccount(ctx, "erc20")
// 		require.NoError(t, err)
// 		erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
// 		rollapp2Erc20MaccBalance, err := rollapp2.GetBalance(ctx, erc20MAccAddr, secondHopIBCDenom)
// 		require.NoError(t, err)

// 		// Make sure that the transfer is not successful yet due to the grace period
// 		require.True(t, rollAppBalance.Equal(walletAmount.Sub(transferAmount)))
// 		require.True(t, dymBalance.Equal(zeroBal))
// 		require.True(t, rollapp2Erc20MaccBalance.Equal(zeroBal))

// 		rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 		require.NoError(t, err)

// 		// wait until the packet is finalized
// 		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)

// 		err = testutil.WaitForBlocks(ctx, 20, dymension, rollapp2)
// 		require.NoError(t, err)

// 		rollapp2Erc20MaccBalance, err = rollapp2.GetBalance(ctx, erc20MAccAddr, secondHopIBCDenom)
// 		require.NoError(t, err)
// 		fmt.Println("rollapp2Erc20MaccBalance", rollapp2Erc20MaccBalance)
// 		require.True(t, rollapp2Erc20MaccBalance.Equal(transferAmount.Sub(bridgingFee)))
// 	})
// 	// Check the commitment was deleted
// 	resp, err := rollapp2.GetNode().QueryPacketCommitments(ctx, "transfer", rollapp2DymChan.ChannelID)
// 	require.NoError(t, err)
// 	require.Equal(t, 0, len(resp.Commitments))
// }

// func TestIBCPFM_RollApp1To2WithOutErc20_Wasm(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip()
// 	}

// 	ctx := context.Background()

// 	// setup config for rollapp 1
// 	settlement_layer_rollapp1 := "dymension"
// 	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
// 	rollapp1_id := "rollappwasm_1234-1"
// 	gas_price_rollapp1 := "0adym"
// 	maxIdleTime1 := "3s"
// 	maxProofTime := "500ms"
// 	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "30s")

// 	// setup config for rollapp 2
// 	settlement_layer_rollapp2 := "dymension"
// 	rollapp2_id := "rollappwasm_12345-1"
// 	gas_price_rollapp2 := "0adym"
// 	maxIdleTime2 := "3s"
// 	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "30s")

// 	modifyGenesisKV := append(dymensionGenesisKV,
// 		cosmos.GenesisKV{
// 			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
// 			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
// 		},
// 	)

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
// 				ModifyGenesis:       nil,
// 				ConfigFileOverrides: configFileOverrides1,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "rollapp2",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "rollapp-dym",
// 				Name:                "rollapp-temp2",
// 				ChainID:             "rollappwasm_12345-1",
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
// 				ModifyGenesis:       nil,
// 				ConfigFileOverrides: configFileOverrides2,
// 			},
// 			NumValidators: &numRollAppVals,
// 			NumFullNodes:  &numRollAppFn,
// 		},
// 		{
// 			Name: "dymension-hub",
// 			ChainConfig: ibc.ChainConfig{
// 				Type:                "hub-dym",
// 				Name:                "dymension",
// 				ChainID:             "dymension_100-1",
// 				Images:              []ibc.DockerImage{dymensionImage},
// 				Bin:                 "dymd",
// 				Bech32Prefix:        "dym",
// 				Denom:               "adym",
// 				CoinType:            "60",
// 				GasPrices:           "0.0adym",
// 				EncodingConfig:      encodingConfig(),
// 				GasAdjustment:       1.1,
// 				TrustingPeriod:      "112h",
// 				NoHostMount:         false,
// 				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
// 				ConfigFileOverrides: nil,
// 			},
// 			NumValidators: &numHubVals,
// 			NumFullNodes:  &numHubFullNodes,
// 			ExtraFlags:    extraFlags,
// 		},
// 	})

// 	// Get chains from the chain factory
// 	chains, err := cf.Chains(t.Name())
// 	require.NoError(t, err)

// 	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
// 	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
// 	dymension := chains[2].(*dym_hub.DymHub)

// 	// Relayer Factory
// 	client, network := test.DockerSetup(t)

// 	// relayer for rollapp 1
// 	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer1", network)
// 	// relayer for rollapp 2
// 	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
// 		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
// 	).Build(t, client, "relayer2", network)

// 	ic := test.NewSetup().
// 		AddRollUp(dymension, rollapp1, rollapp2).
// 		AddRelayer(r1, "relayer1").
// 		AddRelayer(r2, "relayer2").
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp1,
// 			Relayer: r1,
// 			Path:    ibcPath,
// 		}).
// 		AddLink(test.InterchainLink{
// 			Chain1:  dymension,
// 			Chain2:  rollapp2,
// 			Relayer: r2,
// 			Path:    anotherIbcPath,
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
// 	}, nil, "", nil)
// 	require.NoError(t, err)

// 	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
// 	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

// 	channsDym, err := r1.GetChannels(ctx, eRep, dymension.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, channsDym, 2)

// 	rollapp1Chan, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, rollapp1Chan, 1)

// 	dymRollApp1Chan := rollapp1Chan[0].Counterparty
// 	require.NotEmpty(t, dymRollApp1Chan.ChannelID)

// 	rollapp1DymChan := rollapp1Chan[0]
// 	require.NotEmpty(t, rollapp1DymChan.ChannelID)

// 	rollapp2Chan, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
// 	require.NoError(t, err)
// 	require.Len(t, rollapp2Chan, 1)

// 	dymRollApp2Chan := rollapp2Chan[0].Counterparty
// 	require.NotEmpty(t, dymRollApp2Chan.ChannelID)

// 	rollapp2DymChan := rollapp2Chan[0]
// 	require.NotEmpty(t, rollapp2DymChan.ChannelID)

// 	// Start the relayer and set the cleanup function.
// 	err = r1.StartRelayer(ctx, eRep, ibcPath)
// 	require.NoError(t, err)

// 	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
// 	require.NoError(t, err)

// 	t.Cleanup(
// 		func() {
// 			err := r1.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer: %s", err)
// 			}
// 			err = r2.StopRelayer(ctx, eRep)
// 			if err != nil {
// 				t.Logf("an error occurred while stopping the relayer2: %s", err)
// 			}
// 		},
// 	)

// 	// Create some user accounts on both chains
// 	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

// 	// Get our Bech32 encoded user addresses
// 	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

// 	dymensionUserAddr := dymensionUser.FormattedAddress()
// 	rollapp1UserAddr := rollapp1User.FormattedAddress()
// 	rollapp2UserAddr := rollapp2User.FormattedAddress()

// 	// Assert the accounts were funded
// 	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
// 	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount)
// 	testutil.AssertBalance(t, ctx, rollapp2, rollapp2UserAddr, rollapp2.Config().Denom, walletAmount)

// 	// rollapp1Param := rollappParam{
// 	// 	rollappID: rollapp1.Config().ChainID,
// 	// 	channelID: dymRollApp1Chan.ChannelID,
// 	// 	userKey:   dymensionUser.KeyName(),
// 	// }

// 	// rollapp2Param := rollappParam{
// 	// 	rollappID: rollapp2.Config().ChainID,
// 	// 	channelID: dymRollApp2Chan.ChannelID,
// 	// 	userKey:   dymensionUser.KeyName(),
// 	// }
// 	// triggerHubGenesisEvent(t, dymension, rollapp1Param, rollapp2Param)

// 	t.Run("multihop rollapp1->dym->rollapp2, funds received on rollapp2 after grace period", func(t *testing.T) {
// 		firstHopDenom := transfertypes.GetPrefixedDenom(dymRollApp1Chan.PortID, dymRollApp1Chan.ChannelID, rollapp1.Config().Denom)
// 		secondHopDenom := transfertypes.GetPrefixedDenom(rollapp2DymChan.PortID, rollapp2DymChan.ChannelID, firstHopDenom)

// 		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
// 		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

// 		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
// 		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

// 		// Send packet from rollapp1 -> dym -> rollapp2
// 		transfer := ibc.WalletData{
// 			Address: dymensionUserAddr,
// 			Denom:   rollapp1.Config().Denom,
// 			Amount:  transferAmount,
// 		}

// 		firstHopMetadata := &PacketMetadata{
// 			Forward: &ForwardMetadata{
// 				Receiver: rollapp2UserAddr,
// 				Channel:  dymRollApp2Chan.ChannelID,
// 				Port:     dymRollApp2Chan.PortID,
// 				Timeout:  5 * time.Minute,
// 			},
// 		}

// 		memo, err := json.Marshal(firstHopMetadata)
// 		require.NoError(t, err)

// 		transferTx, err := rollapp1.SendIBCTransfer(ctx, rollapp2DymChan.ChannelID, rollapp1User.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
// 		require.NoError(t, err)
// 		err = transferTx.Validate()
// 		require.NoError(t, err)

// 		err = testutil.WaitForBlocks(ctx, 20, rollapp1)
// 		require.NoError(t, err)

// 		rollAppBalance, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
// 		require.NoError(t, err)

// 		dymBalance, err := dymension.GetBalance(ctx, dymensionUserAddr, firstHopIBCDenom)
// 		require.NoError(t, err)

// 		rollapp2UserBalance, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, secondHopIBCDenom)
// 		require.NoError(t, err)

// 		// Make sure that the transfer is not successful yet due to the grace period
// 		require.True(t, rollAppBalance.Equal(walletAmount.Sub(transferAmount)))
// 		require.True(t, dymBalance.Equal(zeroBal))
// 		require.True(t, rollapp2UserBalance.Equal(zeroBal))

// 		rollappHeight, err := rollapp1.GetNode().Height(ctx)
// 		require.NoError(t, err)

// 		// wait until the packet is finalized
// 		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
// 		require.NoError(t, err)
// 		require.True(t, isFinalized)

// 		err = testutil.WaitForBlocks(ctx, 20, dymension, rollapp2)
// 		require.NoError(t, err)

// 		rollapp2UserBalance, err = rollapp2.GetBalance(ctx, rollapp2UserAddr, secondHopIBCDenom)
// 		require.NoError(t, err)
// 		// Minus 0.1% of transfer amount for bridge fee
// 		require.True(t, rollapp2UserBalance.Equal(transferAmount.Sub(transferAmount.Quo(math.NewInt(1000)))))
// 	})
// 	// Check the commitment was deleted
// 	resp, err := rollapp2.GetNode().QueryPacketCommitments(ctx, "transfer", rollapp2DymChan.ChannelID)
// 	require.NoError(t, err)
// 	require.Equal(t, 0, len(resp.Commitments))
// }
