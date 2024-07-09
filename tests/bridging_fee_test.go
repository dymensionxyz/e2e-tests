package tests

import (
	"context"
	"testing"

	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

func Test_Non_Rollappchain_Unaffected_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name:          "gaia-1",
			Version:       "v14.2.0",
			ChainConfig:   gaiaConfig,
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
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

	gaia1 := chains[0].(*cosmos.CosmosChain)
	dymension := chains[1].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)

	ic := test.NewSetup().
		AddChain(dymension).
		AddChain(gaia1).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  gaia1,
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
	}, nil, "", nil)
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, gaia1, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, gaia1)

	// Get our Bech32 encoded user addresses
	dymensionUser, gaiaUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	gaiaUserAddr := gaiaUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, gaia1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, gaia1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   gaia1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = gaia1.SendIBCTransfer(ctx, channel.ChannelID, gaiaUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, gaia1)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, gaia1, gaiaUserAddr, gaia1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// Get the IBC denom for urax on Hub
	gaiaTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, gaia1.Config().Denom)
	gaiaIBCDenom := transfertypes.ParseDenomTrace(gaiaTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, gaiaIBCDenom, transferAmount)

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> gaia
	transferData = ibc.WalletData{
		Address: gaiaUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from Hub -> gaia
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	err = testutil.WaitForBlocks(ctx, 10, dymension, gaia1)
	require.NoError(t, err)

	// Get the IBC denom
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	testutil.AssertBalance(t, ctx, gaia1, gaiaUserAddr, dymensionIBCDenom, transferAmount)
}
