package tests

import (
	"context"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"

	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// This test case verifies the system's behavior when an IBC packet sent from the rollapp to the hub times out.
func TestIBCTransferTimeout_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := overridesDymintToml("dymension", t.Name(), "rollappevm_1234-1", "0adym")
	configFileOverrides2 := overridesDymintToml("dymension", t.Name(), "rollappevm_12345-1", "0adym")

	const BLOCK_FINALITY_PERIOD = 10
	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
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
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp1",
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
				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
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
				CoinType:            "118",
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
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
	).Build(t, client, "relayer1", network)
	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r, "relayer1").
		AddRelayer(s, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: s,
			Path:    anotherIbcPath,
		})

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: false,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	})
	require.NoError(t, err)

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	// Compose an IBC transfer and send from rollapp -> dymension
	var transferAmount = math.NewInt(1_000_000)

	// IBC channel for rollapp
	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	// Set a short timeout for IBC transfer
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 1000000, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	// Compose an IBC transfer and send from Rollapp -> Hub
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)
	// Assert balance was updated on the rollapp
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// Stop relayer after the test ends
	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}

			err = s.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].Counterparty.PortID, channsRollApp1[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Assert funds were returned to the sender after the timeout has occured
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, math.NewInt(0))

	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channsRollApp1[0].Counterparty.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)
	// Assert balance was updated on the rollapp
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))
	// Get the IBC denom for dymension on roll app
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].Counterparty.PortID, channsRollApp1[0].Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, math.NewInt(0))

	// According to delayedack module, we need the rollapp to have finalizedHeight > ibcClientLatestHeight
	// in order to trigger ibc timeout or else it will trigger callback
	err = testutil.WaitForBlocks(ctx, 5, rollapp1)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 40, dymension, rollapp1)
	require.NoError(t, err)

	// Assert funds were returned to the sender after the timeout has occured
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, math.NewInt(0))
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

// This test case verifies the system's behavior when an IBC packet sent from the rollapp to the hub times out.
func TestIBCTransferTimeout_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := overridesDymintToml("dymension", t.Name(), "rollappwasm_1234-1", "0adym")
	configFileOverrides2 := overridesDymintToml("dymension", t.Name(), "rollappwasm_12345-1", "0adym")

	const BLOCK_FINALITY_PERIOD = 10
	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
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
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "rollappwasm_12345-1",
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
				CoinType:            "118",
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
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
	).Build(t, client, "relayer1", network)

	s := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
	).Build(t, client, "relayer2", network)

	const ibcPath = "ibc-path"
	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r, "relayer1").
		AddRelayer(s, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: s,
			Path:    anotherIbcPath,
		})
	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: false,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	})
	require.NoError(t, err)

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	// Compose an IBC transfer and send from rollapp -> dymension
	var transferAmount = math.NewInt(1_000_000)

	// IBC channel for rollapp
	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	// Set a short timeout for IBC transfer
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 1000000, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	// Compose an IBC transfer and send from Rollapp -> Hub
	_, err = rollapp1.SendIBCTransfer(ctx, channsRollApp1[0].ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)
	// Assert balance was updated on the rollapp
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = s.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// Stop relayer after the test ends
	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}

			err = s.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].Counterparty.PortID, channsRollApp1[0].Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Assert funds were returned to the sender after the timeout has occured
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, math.NewInt(0))

	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channsRollApp1[0].Counterparty.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)
	// Assert balance was updated on the rollapp
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))
	// Get the IBC denom for dymension on roll app
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channsRollApp1[0].PortID, channsRollApp1[0].ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, math.NewInt(0))

	// According to delayedack module, we need the rollapp to have finalizedHeight > ibcClientLatestHeight
	// in order to trigger ibc timeout or else it will trigger callback
	err = testutil.WaitForBlocks(ctx, 40, dymension, rollapp1)
	require.NoError(t, err)

	// Assert funds were returned to the sender after the timeout has occured
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, math.NewInt(0))
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}
