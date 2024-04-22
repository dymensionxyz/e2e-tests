package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/icza/dyno"
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

func TestDisconnection_EVM(t *testing.T) {
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
	numRollAppVals := 1
	numRollAppFn := 0

	// Custom dymension epoch for faster disconnection
	modifiedGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.incentives.params.distr_epoch_identifier",
			Value: "custom",
		},
	)
	customDymensionConfig := dymensionConfig
	customDymensionConfig.ModifyGenesis = func(chainConfig ibc.ChainConfig, inputGenBz []byte) ([]byte, error) {
		g := make(map[string]interface{})
		if err := json.Unmarshal(inputGenBz, &g); err != nil {
			return nil, fmt.Errorf("failed to unmarshal genesis file: %w", err)
		}

		epochData, err := dyno.Get(g, "app_state", "epochs", "epochs")
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve epochs: %w", err)
		}
		epochs := epochData.([]interface{})
		exist := false
		// Check if the "custom" identifier already exists
		for _, epoch := range epochs {
			if epochMap, ok := epoch.(map[string]interface{}); ok {
				if epochMap["identifier"] == "custom" {
					exist = true
				}
			}
		}
		if !exist {
			// Define the new epoch type to be added
			newEpochType := map[string]interface{}{
				"identifier":                 "custom",
				"start_time":                 "0001-01-01T00:00:00Z",
				"duration":                   "5s",
				"current_epoch":              "0",
				"current_epoch_start_time":   "0001-01-01T00:00:00Z",
				"epoch_counting_started":     false,
				"current_epoch_start_height": "0",
			}

			// Add the new epoch to the epochs array
			updatedEpochs := append(epochs, newEpochType)
			if err := dyno.Set(g, updatedEpochs, "app_state", "epochs", "epochs"); err != nil {
				return nil, fmt.Errorf("failed to set epochs in genesis json: %w", err)
			}
		}
		if err := dyno.Set(g, "adym", "app_state", "gov", "deposit_params", "min_deposit", 0, "denom"); err != nil {
			return nil, fmt.Errorf("failed to set denom on gov min_deposit in genesis json: %w", err)
		}
		if err := dyno.Set(g, "10000000000", "app_state", "gov", "deposit_params", "min_deposit", 0, "amount"); err != nil {
			return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
		}
		if err := dyno.Set(g, "adym", "app_state", "gamm", "params", "pool_creation_fee", 0, "denom"); err != nil {
			return nil, fmt.Errorf("failed to set amount on gov min_deposit in genesis json: %w", err)
		}
		outputGenBz, err := json.Marshal(g)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal genesis bytes to json: %w", err)
		}

		return cosmos.ModifyGenesis(modifiedGenesisKV)(chainConfig, outputGenBz)
	}

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
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name:          "dymension-hub",
			ChainConfig:   customDymensionConfig,
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
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
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

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	})
	require.NoError(t, err)

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

	walletAmount := math.NewInt(1_000_000_000_000)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, _ := users[0], users[1]

	// IBC channel for rollapps
	channsDym, err := r.GetChannels(ctx, eRep, dymension.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsDym, 1)

	channelDymRollapp := channsDym[0].ChannelID

	triggerHubGenesisEvent(t, dymension, rollappParam{
		rollappID: rollapp1.Config().ChainID,
		channelID: channelDymRollapp,
		userKey:   dymensionUser.KeyName(),
	})

	// Wait for rollapp finalized
	rollapp1Height, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 300)

	t.Run("hub disconnect", func(t *testing.T) {
		err = dymension.StopAllNodes(ctx)
		require.NoError(t, err)

		// Wait until rollapp stops produce block
		rollappHeight, err := rollapp1.Height(ctx)
		require.NoError(t, err)

		err = testutil.WaitForCondition(
			time.Minute*10,
			time.Second*5, // each epoch is 5 seconds
			func() (bool, error) {
				newRollappHeight, err := rollapp1.Height(ctx)
				require.NoError(t, err)

				if newRollappHeight > rollappHeight {
					rollappHeight = newRollappHeight
					return false, nil
				}

				return true, nil
			},
		)
		require.NoError(t, err)

		err = dymension.StartAllNodes(ctx)
		require.NoError(t, err)

		// Make sure rollapp start pro
		err = testutil.WaitForBlocks(ctx, 1, rollapp1)
		require.NoError(t, err)
	})
}
