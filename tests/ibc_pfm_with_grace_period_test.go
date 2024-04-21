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

func TestIBCPFMWithGracePeriod_EVM(t *testing.T) {
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

	modifyGenesisKV := append(dymensionGenesisKV, cosmos.GenesisKV{
		Key:   "app_state.rollapp.params.dispute_period_in_blocks",
		Value: "60",
	})

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

	r := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly, zaptest.NewLogger(t),
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

	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	err = r.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	err = r2.GeneratePath(ctx, eRep, dymension.Config().ChainID, gaia.Config().ChainID, ibcPath)
	require.NoError(t, err)

	err = r2.CreateClients(ctx, eRep, ibcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, dymension, gaia)
	require.NoError(t, err)

	r2.UpdateClients(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.CreateConnections(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	err = r2.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
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

	t.Run("multihop rollapp->dym->gaia, funds received on gaia after grace period", func(t *testing.T) {
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

		err = testutil.WaitForBlocks(ctx, 20, rollapp1)
		require.NoError(t, err)

		rollAppBalance, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
		require.NoError(t, err)

		dymBalance, err := dymension.GetBalance(ctx, dymensionUserAddr, firstHopIBCDenom)
		require.NoError(t, err)

		gaiaBalance, err := gaia.GetBalance(ctx, gaiaUserAddr, secondHopIBCDenom)
		require.NoError(t, err)

		// Make sure that the transfer is not successful yet due to the grace period
		require.True(t, rollAppBalance.Equal(walletAmount.Sub(transferAmount)))
		require.True(t, dymBalance.Equal(zeroBal))
		require.True(t, gaiaBalance.Equal(zeroBal))

		rollappHeight, err := rollapp1.GetNode().Height(ctx)
		require.NoError(t, err)

		// wait until the packet is finalized
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)

		err = testutil.WaitForBlocks(ctx, 20, dymension, gaia)
		require.NoError(t, err)

		gaiaBalance, err = gaia.GetBalance(ctx, gaiaUserAddr, secondHopIBCDenom)
		require.NoError(t, err)
		require.True(t, gaiaBalance.Equal(transferAmount))
	})
}

func TestIBCPFMWithGracePeriod_Wasm(t *testing.T) {
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
	dymintTomlOverrides["empty_blocks_max_time"] = "3s"

	modifyGenesisKV := append(dymensionGenesisKV, cosmos.GenesisKV{
		Key:   "app_state.rollapp.params.dispute_period_in_blocks",
		Value: "60",
	})

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

	r := test.NewBuiltinRelayerFactory(
		ibc.CosmosRly, zaptest.NewLogger(t),
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

	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	err = r.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	err = r2.GeneratePath(ctx, eRep, dymension.Config().ChainID, gaia.Config().ChainID, ibcPath)
	require.NoError(t, err)

	err = r2.CreateClients(ctx, eRep, ibcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, dymension, gaia)
	require.NoError(t, err)

	r2.UpdateClients(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.CreateConnections(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	err = r2.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
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

	t.Run("multihop rollapp->dym->gaia, funds received on gaia after grace period", func(t *testing.T) {
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

		err = testutil.WaitForBlocks(ctx, 20, rollapp1)
		require.NoError(t, err)

		rollAppBalance, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
		require.NoError(t, err)

		dymBalance, err := dymension.GetBalance(ctx, dymensionUserAddr, firstHopIBCDenom)
		require.NoError(t, err)

		gaiaBalance, err := gaia.GetBalance(ctx, gaiaUserAddr, secondHopIBCDenom)
		require.NoError(t, err)

		// Make sure that the transfer is not successful yet due to the grace period
		require.True(t, rollAppBalance.Equal(walletAmount.Sub(transferAmount)))
		require.True(t, dymBalance.Equal(zeroBal))
		require.True(t, gaiaBalance.Equal(zeroBal))

		rollappHeight, err := rollapp1.GetNode().Height(ctx)
		require.NoError(t, err)

		// wait until the packet is finalized
		isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)

		err = testutil.WaitForBlocks(ctx, 20, dymension, gaia)
		require.NoError(t, err)

		gaiaBalance, err = gaia.GetBalance(ctx, gaiaUserAddr, secondHopIBCDenom)
		require.NoError(t, err)
		require.True(t, gaiaBalance.Equal(transferAmount))
	})
}

func TestIBCGracePeriod_RollApp2RollAppWithErc20(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	emptyBlocksMaxTime := "3s"
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, node_address, rollapp1_id, gas_price_rollapp1, emptyBlocksMaxTime)

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "rollappevm_12345-1"
	gas_price_rollapp2 := "0adym"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, node_address, rollapp2_id, gas_price_rollapp2, emptyBlocksMaxTime)


	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: "20",
		},
	)

	modifyRollappGeneisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.erc20.params.enable_erc20",
			Value: true,
		},
	)

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
				ChainID:             "rollappevm_12345-1",
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
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	// relayer for rollapp 1
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
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
	})
	require.NoError(t, err)
	err = r1.GeneratePath(ctx, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID, ibcPath)
	require.NoError(t, err)
	err = r2.GeneratePath(ctx, eRep, dymension.Config().ChainID, rollapp2.Config().ChainID, anotherIbcPath)
	require.NoError(t, err)

	err = r1.CreateClients(ctx, eRep, ibcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)
	err = r2.CreateClients(ctx, eRep, anotherIbcPath, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, dymension)
	require.NoError(t, err)

	r1.UpdateClients(ctx, eRep, ibcPath)
	require.NoError(t, err)
	r2.UpdateClients(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = r1.CreateConnections(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.CreateConnections(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	err = r1.CreateChannel(ctx, eRep, ibcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)
	err = r2.CreateChannel(ctx, eRep, anotherIbcPath, ibc.DefaultChannelOpts())
	require.NoError(t, err)

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()
	rollapp2UserAddr := rollapp2User.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp2, rollapp2UserAddr, rollapp2.Config().Denom, walletAmount)

	// Compose an IBC transfer and send from rollapp -> dymension
	transferAmount := math.NewInt(1_000_000)

	rollApp1Channel, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, rollApp1Channel, 1)

	channDymRollApp1 := rollApp1Channel[0].Counterparty
	require.NotEmpty(t, channDymRollApp1.ChannelID)

	dymChannel, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, 2, len(dymChannel))

	transferDataDymToRollApp := ibc.WalletData{
		Address: rollapp1UserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	rollapp := rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: dymChannel[0].ChannelID,
		userKey:   dymensionUser.KeyName(),
	}
	triggerHubGenesisEvent(t, dymension, rollapp)

	// Compose an IBC transfer and send from dymension -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channDymRollApp1.ChannelID, dymensionUserAddr, transferDataDymToRollApp, ibc.TransferOptions{})
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferDataDymToRollApp.Amount))

	// Get the IBC denom for dymension on roll app
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(dymChannel[0].Counterparty.PortID, dymChannel[0].Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, dymensionIBCDenom, math.NewInt(0))

	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Assert balance was updated on the Rollapp
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferDataDymToRollApp.Amount))
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, dymensionIBCDenom, transferDataDymToRollApp.Amount)

	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp1.SendIBCTransfer(ctx, dymChannel[0].ChannelID, rollapp1UserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Assert balance was updated on the rollapp because transfer amount was deducted from wallet balance
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(dymChannel[0].Counterparty.PortID, dymChannel[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	err = testutil.WaitForBlocks(ctx, 10, dymension)
	require.NoError(t, err)

	// Assert funds are waiting
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, math.NewInt(0))

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Assert balance was updated on the Hub
	testutil.AssertBalance(t, ctx, rollapp1, rollapp1UserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount)

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}