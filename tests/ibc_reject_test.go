package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	test "github.com/decentrio/rollup-e2e-testing"
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
func TestIBCRejectPacketSend(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["node_address"] = "http://dymension_100-1-val-0-TestIBCRejectPacketSend:26657"
	dymintTomlOverrides["rollapp_id"] = "demo-dymension-rollapp"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 3
	numHubFullNodes := 3
	numRollAppFn := 0
	numRollAppVals := 1
	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "demo-dymension-rollapp",
				Images:              []ibc.DockerImage{rollappImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "rol",
				Denom:               "urax",
				CoinType:            "118",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
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
	})
	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/cosmos/relayer", "reece-v2.3.1-ethermint", "100:1000"),
	).Build(t, client, network)
	const ibcPath = "dymension-rollapp"
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

	t.Run("test tranfer packet sent with blocked address", func(t *testing.T) {
		channel, err := ibc.GetTransferChannel(ctx, r, eRep, rollapp1.Config().ChainID, dymension.Config().ChainID)
		require.NoError(t, err)

		stdout, _, err := dymension.GetNode().ExecQuery(ctx, "auth", "module-account", "mint")
		require.NoError(t, err)

		var resp map[string]interface{}
		err = json.Unmarshal(stdout, &resp)
		require.NoError(t, err)

		moduleAddr := resp["account"].(map[string]interface{})["base_account"].(map[string]interface{})["address"].(string)

		fmt.Println("module Addr: ", moduleAddr)
		transfer := ibc.WalletData{
			Address: moduleAddr,
			Denom:   rollapp1.Config().Denom,
			Amount:  transferAmount,
		}

		// Get the IBC denom
		rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
		rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

		tx, err := rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transfer, ibc.TransferOptions{})
		require.NoError(t, err)

		err = tx.Validate()
		require.NoError(t, err)

		err = r.StartRelayer(ctx, eRep, ibcPath)
		require.NoError(t, err)

		err = testutil.WaitForBlocks(ctx, 20, rollapp1, dymension)
		require.NoError(t, err)

		err = r.StopRelayer(ctx, eRep)
		require.NoError(t, err)

		err = testutil.WaitForBlocks(ctx, 10, rollapp1, dymension)
		require.NoError(t, err)

		testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)
		testutil.AssertBalance(t, ctx, dymension, moduleAddr, rollappIBCDenom, math.NewInt(0))
	})
}
