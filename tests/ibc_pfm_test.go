package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/math"
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
	dymintTomlOverrides["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["gas_prices"] = "0adym"
	dymintTomlOverrides["empty_blocks_max_time"] = "3s"

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
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
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

	err = r.GeneratePath(ctx, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID, ibcPath)
	require.NoError(t, err)

	err = r.CreateClients(ctx, eRep, ibcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, dymension)
	require.NoError(t, err)

	r.UpdateClients(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r.CreateConnections(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

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

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, gaia)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

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

		zeroBal := math.ZeroInt()
		transferAmount := math.NewInt(100_000)

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

		err = testutil.WaitForBlocks(ctx, 40, rollapp1, gaia)
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
	dymintTomlOverrides["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["gas_prices"] = "0adym"

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
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
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

	err = r.GeneratePath(ctx, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID, ibcPath)
	require.NoError(t, err)

	err = r.CreateClients(ctx, eRep, ibcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, dymension)
	require.NoError(t, err)

	r.UpdateClients(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r.CreateConnections(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

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

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, gaia)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

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

		zeroBal := math.ZeroInt()
		transferAmount := math.NewInt(100_000)

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

		err = testutil.WaitForBlocks(ctx, 40, rollapp1, gaia)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferAmount))
		testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, firstHopIBCDenom, zeroBal)
		testutil.AssertBalance(t, ctx, gaia, gaiaUserAddr, secondHopIBCDenom, transferAmount)
	})
}
