package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"cosmossdk.io/math"
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

// TestRollAppFreeze ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing.
func TestRollAppFreeze(t *testing.T) {
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

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollappUserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 3, dymension, rollapp1)
	require.NoError(t, err)

	rollAppheight, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	t.Logf("rollAppheight: %d", rollAppheight)

	keyDir := dymension.GetRollApp().GetSequencerKeyDir()

	// fraudHeight := rollAppheight - 2

	err = testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
	require.NoError(t, err)

	oldLatestIndex, err := dymension.GetNode().GetLatestIndex(ctx, "rollappevm_1234-1")
	require.NoError(t, err)

	// Define a struct to represent the JSON structure
	var data struct {
		StateIndex struct {
			RollappID string `json:"rollappId"`
			Index     string `json:"index"`
		} `json:"stateIndex"`
	}

	// Unmarshal the JSON string into the struct
	err = json.Unmarshal([]byte(oldLatestIndex), &data)
	require.NoError(t, err)

	// Access the index value
	index := data.StateIndex.Index
	uintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)

	targetIndex := uintIndex + 1

	jsonData, err := json.Marshal(20)
	require.NoError(t, err)

	disputeblock := json.RawMessage(jsonData)
	propTx, err := dymension.ParamChangeProposal(ctx, dymensionUser.KeyName(), &utils.ParamChangeProposalJSON{
		Title:       "Add new deployer_whitelist",
		Description: "Add current dymensionUserAddr to the deployer_whitelist",
		Changes: utils.ParamChangesJSON{
			utils.NewParamChangeJSON("rollapp", "DisputePeriodInBlocks", disputeblock),
		},
		Deposit: "500000000000" + dymension.Config().Denom, // greater than min deposit
	})
	require.NoError(t, err)

	t.Logf("testststsst %s ", propTx.ProposalID)

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+30, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	new_params, err := dymension.QueryParam(ctx, "rollapp", "DisputePeriodInBlocks")
	require.NoError(t, err)
	require.Equal(t, new_params.Value, string(disputeblock))

	// Loop until the latest index updates
	for {
		oldLatestIndex, err := dymension.GetNode().GetLatestIndex(ctx, "rollappevm_1234-1")
		require.NoError(t, err)
		var data struct {
			StateIndex struct {
				RollappID string `json:"rollappId"`
				Index     string `json:"index"`
			} `json:"stateIndex"`
		}
		err = json.Unmarshal(oldLatestIndex, &data)
		require.NoError(t, err)

		index := data.StateIndex.Index
		uintIndex, err := strconv.ParseUint(index, 10, 64)
		require.NoError(t, err)

		t.Logf("Logging --------------------------: %d and %d", uintIndex, targetIndex)
		if uintIndex == targetIndex {
			break
		}
		if uintIndex > targetIndex {
			targetIndex = uintIndex
			break
		}
	}

	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)
	submitFraudStr := "fraud"

	prop := cosmos.SubmitFraudProposal{
		RollappId:    "rollappevm_1234-1",
		Height:       fmt.Sprint(targetIndex + 1),
		ProposerAddr: sequencerAddr,
		ClientId:     "07-tendermint-0",
		Tittle:       submitFraudStr,
		Description:  submitFraudStr,
	}
	for {
		err = dymension.GetNode().SubmitFraudProposal(ctx, "sequencer", prop, keyDir)
		if err == nil {
			break
		}
	}

	t.Logf("Success SubmitFraudProposal")
}
