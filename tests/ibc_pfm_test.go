package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

func TestIBCTransferMultiHop_EVM(t *testing.T) {
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
	dymintTomlOverrides["batch_submit_max_time"] = "60s"

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
			Name:          "gaia",
			Version:       "v15.1.0",
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
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/dymensionxyz/go-relayer", "main-dym", "100:1000"),
	).Build(t, client, "relayer", network)

	r2 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(IBCRelayerImage, IBCRelayerVersion, "100:1000"),
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

	t.Cleanup(func() {
		_ = ic.Close()
	})

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, gaia, ibcPath)

	channsDym, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym, 2)

	channsRollApp, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp, 1)

	channDymRollApp := channsRollApp[0].Counterparty
	require.NotEmpty(t, channDymRollApp.ChannelID)

	channsRollAppDym := channsRollApp[0]
	require.NotEmpty(t, channsRollAppDym.ChannelID)

	channsDym, err = r2.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)

	channsGaia, err := r2.GetChannels(ctx, eRep, gaia.GetChainID())
	require.NoError(t, err)

	require.Len(t, channsDym, 2)
	require.Len(t, channsGaia, 1)

	channDymGaia := channsGaia[0].Counterparty
	require.NotEmpty(t, channDymGaia.ChannelID)

	channGaiaDym := channsGaia[0]
	require.NotEmpty(t, channGaiaDym.ChannelID)

	// Start the relayer and set the cleanup function.
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, gaia)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser, gaiaUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gaiaOrigBal)

	rollapp := rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: channDymRollApp.ChannelID,
		userKey:   dymensionUser.KeyName(),
	}
	triggerHubGenesisEvent(t, dymension, rollapp)

	t.Run("multihop rollapp->dym->gaia", func(t *testing.T) {
		firstHopDenom := transfertypes.GetPrefixedDenom(channDymRollApp.PortID, channDymRollApp.ChannelID, rollapp1.Config().Denom)
		secondHopDenom := transfertypes.GetPrefixedDenom(channGaiaDym.PortID, channGaiaDym.ChannelID, firstHopDenom)

		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

		// Send packet from rollapp1 -> dym -> gaia
		transfer := ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   rollapp1.Config().Denom,
			Amount:  transferAmount,
		}

		firstHopMetadata := &PacketMetadata{
			Forward: &ForwardMetadata{
				Receiver: gaiaUserAddr,
				Channel:  channDymGaia.ChannelID,
				Port:     channDymGaia.PortID,
				Timeout:  5 * time.Minute,
			},
		}

		memo, err := json.Marshal(firstHopMetadata)
		require.NoError(t, err)

		transferTx, err := rollapp1.SendIBCTransfer(ctx, channsRollAppDym.ChannelID, rollappUser.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		rollappHeight, err := rollapp1.GetNode().Height(ctx)
		require.NoError(t, err)

		// wait until the packet is finalized
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)

		// wait until gaia receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, zeroBal)
		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, secondHopIBCDenom, transferAmount)
	})
}

func TestIBCTransferMultiHop_Wasm(t *testing.T) {
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
	dymintTomlOverrides["batch_submit_max_time"] = "60s"

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
				ModifyGenesis:       nil,
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
			Name:          "gaia",
			Version:       "v15.1.0",
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
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/dymensionxyz/go-relayer", "main-dym", "100:1000"),
	).Build(t, client, "relayer", network)

	r2 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(IBCRelayerImage, IBCRelayerVersion, "100:1000"),
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

	t.Cleanup(func() {
		_ = ic.Close()
	})

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, gaia, ibcPath)

	channsDym, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym, 2)

	channsRollApp, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp, 1)

	channDymRollApp := channsRollApp[0].Counterparty
	require.NotEmpty(t, channDymRollApp.ChannelID)

	channsRollAppDym := channsRollApp[0]
	require.NotEmpty(t, channsRollAppDym.ChannelID)

	channsDym, err = r2.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)

	channsGaia, err := r2.GetChannels(ctx, eRep, gaia.GetChainID())
	require.NoError(t, err)

	require.Len(t, channsDym, 2)
	require.Len(t, channsGaia, 1)

	channDymGaia := channsGaia[0].Counterparty
	require.NotEmpty(t, channDymGaia.ChannelID)

	channGaiaDym := channsGaia[0]
	require.NotEmpty(t, channGaiaDym.ChannelID)

	// Start the relayer and set the cleanup function.
	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, gaia)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser, gaiaUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gaiaOrigBal)

	rollapp := rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: channDymRollApp.ChannelID,
		userKey:   dymensionUser.KeyName(),
	}
	triggerHubGenesisEvent(t, dymension, rollapp)

	t.Run("multihop rollapp->dym->gaia", func(t *testing.T) {
		firstHopDenom := transfertypes.GetPrefixedDenom(channDymRollApp.PortID, channDymRollApp.ChannelID, rollapp1.Config().Denom)
		secondHopDenom := transfertypes.GetPrefixedDenom(channGaiaDym.PortID, channGaiaDym.ChannelID, firstHopDenom)

		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

		// Send packet from rollapp1 -> dym -> gaia
		transfer := ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   rollapp1.Config().Denom,
			Amount:  transferAmount,
		}

		firstHopMetadata := &PacketMetadata{
			Forward: &ForwardMetadata{
				Receiver: gaiaUserAddr,
				Channel:  channDymGaia.ChannelID,
				Port:     channDymGaia.PortID,
				Timeout:  5 * time.Minute,
			},
		}

		memo, err := json.Marshal(firstHopMetadata)
		require.NoError(t, err)

		transferTx, err := rollapp1.SendIBCTransfer(ctx, channsRollAppDym.ChannelID, rollappUser.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		rollappHeight, err := rollapp1.GetNode().Height(ctx)
		require.NoError(t, err)

		// wait until the packet is finalized
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)

		// wait until gaia receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, zeroBal)
		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, secondHopIBCDenom, transferAmount)
	})
}

func TestIBCTransferGaiaToRollApp_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()
	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "60s")

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
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/dymensionxyz/go-relayer", "main-dym", "100:1000"),
	).Build(t, client, "relayer", network)

	r2 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(IBCRelayerImage, IBCRelayerVersion, "100:1000"),
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
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = ic.Close()
	})

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, gaia, anotherIbcPath)

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
			err := r.StopRelayer(ctx, eRep)
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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, gaia)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser, gaiaUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gaiaOrigBal)

	rollapp := rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: dymRollAppChan.ChannelID,
		userKey:   dymensionUser.KeyName(),
	}
	triggerHubGenesisEvent(t, dymension, rollapp)

	t.Run("multihop gaia->dym->rollapp", func(t *testing.T) {

		firstHopDenom := transfertypes.GetPrefixedDenom(dymGaiaChan.PortID, dymGaiaChan.ChannelID, gaia.Config().Denom)
		secondHopDenom := transfertypes.GetPrefixedDenom(rollappDymChan.PortID, rollappDymChan.ChannelID, firstHopDenom)

		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

		// Send packet from gaia -> dym -> rollapp1
		transfer := ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   gaia.Config().Denom,
			Amount:  transferAmount,
		}

		firstHopMetadata := &PacketMetadata{
			Forward: &ForwardMetadata{
				Receiver: rollappUserAddr,
				Channel:  dymRollAppChan.ChannelID,
				Port:     dymRollAppChan.PortID,
				Timeout:  5 * time.Minute,
			},
		}

		memo, err := json.Marshal(firstHopMetadata)
		require.NoError(t, err)

		transferTx, err := gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaiaUser.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		// wait until dymension receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
		require.NoError(t, err)

		rollappHeight, err := rollapp1.GetNode().Height(ctx)
		require.NoError(t, err)

		// wait until the packet is finalized
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)

		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, gaia.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, zeroBal)
		testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, secondHopIBCDenom, transferAmount)
	})
}

func TestIBCTransferGaiaToRollApp_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()
	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "60s")

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
				ModifyGenesis:       nil,
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
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/dymensionxyz/go-relayer", "main-dym", "100:1000"),
	).Build(t, client, "relayer", network)

	r2 := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(IBCRelayerImage, IBCRelayerVersion, "100:1000"),
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
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = ic.Close()
	})

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, gaia, anotherIbcPath)

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
			err := r.StopRelayer(ctx, eRep)
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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, gaia)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser, gaiaUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	gaiaOrigBal, err := gaia.GetBalance(ctx, gaiaUserAddr, gaia.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, gaiaOrigBal)

	rollapp := rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: dymRollAppChan.ChannelID,
		userKey:   dymensionUser.KeyName(),
	}
	triggerHubGenesisEvent(t, dymension, rollapp)

	t.Run("multihop gaia->dym->rollapp", func(t *testing.T) {

		firstHopDenom := transfertypes.GetPrefixedDenom(dymGaiaChan.PortID, dymGaiaChan.ChannelID, gaia.Config().Denom)
		secondHopDenom := transfertypes.GetPrefixedDenom(rollappDymChan.PortID, rollappDymChan.ChannelID, firstHopDenom)

		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

		// Send packet from gaia -> dym -> rollapp1
		transfer := ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   gaia.Config().Denom,
			Amount:  transferAmount,
		}

		firstHopMetadata := &PacketMetadata{
			Forward: &ForwardMetadata{
				Receiver: rollappUserAddr,
				Channel:  dymRollAppChan.ChannelID,
				Port:     dymRollAppChan.PortID,
				Timeout:  5 * time.Minute,
			},
		}

		memo, err := json.Marshal(firstHopMetadata)
		require.NoError(t, err)

		transferTx, err := gaia.SendIBCTransfer(ctx, gaiaDymChan.ChannelID, gaiaUser.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		// wait until dymension receive transferAmount
		err = testutil.WaitForBlocks(ctx, 10, dymension, gaia)
		require.NoError(t, err)

		rollappHeight, err := rollapp1.GetNode().Height(ctx)
		require.NoError(t, err)

		// wait until the packet is finalized
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)

		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, gaia.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, zeroBal)
		testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, secondHopIBCDenom, transferAmount)
	})
}
