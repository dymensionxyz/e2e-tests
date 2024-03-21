package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/params/client/utils"
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

// TestIBCTransferSuccess ensure that the transfer between Hub and Rollapp is accurate.
func TestRollappGenesisEvent(t *testing.T) {
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

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	extraFlags := map[string]interface{}{"genesis-accounts-path": true}
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
				Images:              []ibc.DockerImage{rollappImage},
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
			ExtraFlags:    extraFlags,
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
	).Build(t, client, network)

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

	_ = dymensionUserAddr
	_ = rollappUserAddr

	deployerWhitelistParams := json.RawMessage(fmt.Sprintf(`[{"address":"%s"}]`, dymensionUserAddr))
	propTx, err := dymension.ParamChangeProposal(ctx, dymensionUser.KeyName(), &utils.ParamChangeProposalJSON{
		Title:       "Add new deployer_whitelist",
		Description: "Add current dymensionUserAddr to the deployer_whitelist",
		Changes: utils.ParamChangesJSON{
			utils.NewParamChangeJSON("rollapp", "DeployerWhitelist", deployerWhitelistParams),
		},
		Deposit: "500000000000" + dymension.Config().Denom, // greater than min deposit
	})
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+30, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	new_params, err := dymension.QueryParam(ctx, "rollapp", "DeployerWhitelist")
	require.NoError(t, err)
	require.Equal(t, new_params.Value, string(deployerWhitelistParams))

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	txHash, err := dymension.FullNodes[0].ExecTx(ctx, dymensionUserAddr, "rollapp", "genesis-event", rollapp1.GetChainID(), channel.ChannelID)
	require.NoError(t, err)

	tx, err := dymension.GetTransaction(txHash)
	require.NoError(t, err)

	recipient, _ := cosmos.AttributeValue(tx.Events, "transfer", "recipient")
	coinStr, _ := cosmos.AttributeValue(tx.Events, "transfer", "amount")

	coin, err := sdk.ParseCoinNormalized(coinStr)
	require.NoError(t, err)

	validatorAddr, err := dymension.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)
	require.Equal(t, recipient, validatorAddr)

	testutil.AssertBalance(t, ctx, dymension, validatorAddr, coin.Denom, coin.Amount)

	genesisTriggererWhitelistParams := json.RawMessage(fmt.Sprintf(`[{"address":"%s"}]`, rollappUserAddr))
	propTx, err = rollapp1.ParamChangeProposal(ctx, rollappUser.KeyName(), &utils.ParamChangeProposalJSON{
		Title:       "Add new genesis_triggerer_whitelist",
		Description: "Add current rollappUserAddr to the genesis_triggerer_whitelist",
		Changes: utils.ParamChangesJSON{
			utils.NewParamChangeJSON("hubgenesis", "GenesisTriggererWhitelist", genesisTriggererWhitelistParams),
		},
		Deposit: "500000000000" + rollapp1.Config().Denom, // greater than min deposit
	})
	require.NoError(t, err)

	err = rollapp1.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err = rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, height+30, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	new_params, err = rollapp1.QueryParam(ctx, "hubgenesis", "GenesisTriggererWhitelist")
	require.NoError(t, err)
	require.Equal(t, new_params.Value, string(genesisTriggererWhitelistParams))

	txHash, err = rollapp1.Validators[0].ExecTx(ctx, rollappUserAddr, "hubgenesis", "genesis-event", dymension.GetChainID(), channel.ChannelID)
	require.NoError(t, err)

	tx, err = rollapp1.GetTransaction(txHash)
	require.NoError(t, err)

	fmt.Println("tx: ", *tx)
}
