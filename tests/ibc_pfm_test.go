package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

type PacketMetadata struct {
	Forward *ForwardMetadata `json:"forward"`
}

type ForwardMetadata struct {
	Receiver       string        `json:"receiver"`
	Port           string        `json:"port"`
	Channel        string        `json:"channel"`
	Timeout        time.Duration `json:"timeout"`
	Retries        *uint8        `json:"retries,omitempty"`
	Next           *string       `json:"next,omitempty"`
	RefundSequence *uint64       `json:"refund_sequence,omitempty"`
}

func TestIBCTransferMultiHop(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "demo-dymension-rollapp"

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
		{
			Name:          "osmosis",
			Version:       "v17.0.0",
			ChainConfig:   osmosisConfig,
			NumValidators: &numVals,
			NumFullNodes:  &numFullNodes,
		},
	})
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)
	osmosis := chains[2].(*cosmos.CosmosChain)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/cosmos/relayer", "reece-v2.3.1-ethermint", "100:1000"),
	).Build(t, client, network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddChain(osmosis).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    pathHubToRollApp,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  osmosis,
			Relayer: r,
			Path:    pathDymToOsmos,
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

	// Create relayer rollapp to dym
	// Generate new path
	err = r.GeneratePath(ctx, eRep, dymension.GetChainID(), rollapp1.GetChainID(), pathHubToRollApp)
	require.NoError(t, err)
	// Create client
	err = r.CreateClients(ctx, eRep, pathHubToRollApp, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, rollapp1, osmosis)
	require.NoError(t, err)

	// Create connection
	err = r.CreateConnections(ctx, eRep, pathHubToRollApp)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, rollapp1, osmosis)
	require.NoError(t, err)
	// Create channel
	err = r.CreateChannel(ctx, eRep, pathHubToRollApp, ibc.CreateChannelOptions{
		SourcePortName: "transfer",
		DestPortName:   "transfer",
		Order:          ibc.Unordered,
		Version:        "ics20-1",
	})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, rollapp1, osmosis)
	require.NoError(t, err)

	channsDym, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym, 1)

	channsRollApp, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp, 1)

	channDymRollApp := channsDym[0]
	require.NotEmpty(t, channDymRollApp.ChannelID)

	channsRollAppDym := channsRollApp[0]
	require.NotEmpty(t, channsRollAppDym.ChannelID)

	// Create relayer dym to osmo
	// Generate new path
	err = r.GeneratePath(ctx, eRep, dymension.GetChainID(), osmosis.GetChainID(), pathDymToOsmos)
	require.NoError(t, err)
	// Create clients
	err = r.CreateClients(ctx, eRep, pathDymToOsmos, ibc.DefaultClientOpts())
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, osmosis)
	require.NoError(t, err)

	// Create connection
	err = r.CreateConnections(ctx, eRep, pathDymToOsmos)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, osmosis)
	require.NoError(t, err)

	// Create channel
	err = r.CreateChannel(ctx, eRep, pathDymToOsmos, ibc.CreateChannelOptions{
		SourcePortName: "transfer",
		DestPortName:   "transfer",
		Order:          ibc.Unordered,
		Version:        "ics20-1",
	})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, osmosis)
	require.NoError(t, err)

	channsDym, err = r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)

	channsOsmosis, err := r.GetChannels(ctx, eRep, osmosis.GetChainID())
	require.NoError(t, err)

	require.Len(t, channsDym, 2)
	require.Len(t, channsOsmosis, 1)

	var channDymOsmos ibc.ChannelOutput
	for _, chann := range channsDym {
		if chann.ChannelID != channDymRollApp.ChannelID {
			channDymOsmos = chann
		}
	}
	require.NotEmpty(t, channDymOsmos.ChannelID)

	channOsmosDym := channsOsmosis[0]
	require.NotEmpty(t, channOsmosDym.ChannelID)

	// Start the relayer and set the cleanup function.
	err = r.StartRelayer(ctx, eRep, pathHubToRollApp, pathDymToOsmos)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, osmosis)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser, osmosisUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	osmosisUserAddr := osmosisUser.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	osmosisOrigBal, err := osmosis.GetBalance(ctx, osmosisUserAddr, osmosis.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, osmosisOrigBal)

	t.Run("multihop rollapp->dym->osmosis", func(t *testing.T) {
		firstHopDenom := transfertypes.GetPrefixedDenom(channDymRollApp.PortID, channDymRollApp.ChannelID, rollapp1.Config().Denom)
		secondHopDenom := transfertypes.GetPrefixedDenom(channOsmosDym.PortID, channOsmosDym.ChannelID, firstHopDenom)

		firstHopDenomTrace := transfertypes.ParseDenomTrace(firstHopDenom)
		secondHopDenomTrace := transfertypes.ParseDenomTrace(secondHopDenom)

		firstHopIBCDenom := firstHopDenomTrace.IBCDenom()
		secondHopIBCDenom := secondHopDenomTrace.IBCDenom()

		zeroBal := math.ZeroInt()
		transferAmount := math.NewInt(100_000)

		// Send packet from rollapp1 -> dym -> osmosis
		transfer := ibc.WalletData{
			Address: dymensionUserAddr,
			Denom:   rollapp1.Config().Denom,
			Amount:  transferAmount,
		}

		firstHopMetadata := &PacketMetadata{
			Forward: &ForwardMetadata{
				Receiver: osmosisUserAddr,
				Channel:  channDymOsmos.ChannelID,
				Port:     channDymOsmos.PortID,
				Timeout:  5 * time.Minute,
			},
		}

		memo, err := json.Marshal(firstHopMetadata)
		require.NoError(t, err)

		transferTx, err := rollapp1.SendIBCTransfer(ctx, channsRollAppDym.ChannelID, rollappUser.KeyName(), transfer, ibc.TransferOptions{Memo: string(memo)})
		require.NoError(t, err)
		err = transferTx.Validate()
		require.NoError(t, err)

		err = testutil.WaitForBlocks(ctx, 15, rollapp1)
		require.NoError(t, err)

		rollAppBalance, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
		require.NoError(t, err)

		dymBalance, err := dymension.GetBalance(ctx, dymensionUserAddr, firstHopIBCDenom)
		require.NoError(t, err)

		osmosisBalance, err := osmosis.GetBalance(ctx, osmosisUserAddr, secondHopIBCDenom)
		require.NoError(t, err)

		require.True(t, rollAppBalance.Equal(walletAmount.Sub(transferAmount)))
		require.True(t, dymBalance.Equal(zeroBal))
		require.True(t, osmosisBalance.Equal(transferAmount))
	})
}
