package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

const anotherIbcPath = "dymension-demo2"

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

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 3, dymension, rollapp1)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
	require.NoError(t, err)

	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, "rollappevm_1234-1")
	require.NoError(t, err)

	// Access the index value
	index := oldLatestIndex.StateIndex.Index
	uintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)

	targetIndex := uintIndex + 1

	// Convert the uint to string
	myUintStr := fmt.Sprintf("%d", 50)

	// Create a JSON string with the uint value quoted
	jsonData := fmt.Sprintf(`"%s"`, myUintStr)

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
		oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, "rollappevm_1234-1")
		require.NoError(t, err)

		index := oldLatestIndex.StateIndex.Index
		uintIndex, err := strconv.ParseUint(index, 10, 64)

		require.NoError(t, err)
		if uintIndex >= targetIndex {
			break
		}
	}

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	err = dymension.SubmitFraudProposal(
		ctx, dymensionUser.KeyName(),
		"rollappevm_1234-1",
		fmt.Sprint(rollappHeight-2),
		sequencerAddr,
		"07-tendermint-0",
		"fraud",
		"fraud",
		"500000000000"+dymension.Config().Denom,
	)
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, "2", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	// Wait a few blocks for the gov to pass and to verify if the state index increment
	err = testutil.WaitForBlocks(ctx, 50, dymension, rollapp1)
	require.NoError(t, err)

	// Check if rollapp has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, "rollappevm_1234-1")
	require.NoError(t, err)
	require.Equal(t, rollappParams.Rollapp.Frozen, true, "rollapp does not frozen")

	// Check rollapp state index not increment
	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, "rollappevm_1234-1")
	require.NoError(t, err)
	require.Equal(t, latestIndex.StateIndex.Index, targetIndex, "rollapp state index still increment")

	// IBC Transfer not working
	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> rollapp
	var transferAmount = math.NewInt(1_000_000)

	err = dymension.IBCTransfer(ctx,
		dymension, rollapp1, transferAmount, dymensionUserAddr,
		rollappUserAddr, r, ibcPath, channel,
		eRep, ibc.TransferOptions{})
	require.Error(t, err)
	require.Equal(t, strings.Contains(err.Error(), "status Frozen"), true)

	// Compose an IBC transfer and send from rollapp -> dymension
	err = rollapp1.IBCTransfer(ctx,
		rollapp1, dymension, transferAmount, rollappUserAddr,
		dymensionUserAddr, r, ibcPath, channel,
		eRep, ibc.TransferOptions{})
	require.Error(t, err)
}

// TestRollAppFreeze ensure upon freeze gov proposal passed, no updates can be made to the rollapp and not IBC txs are passing.
func TestOtherRollappNotAffected(t *testing.T) {
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

	configFileOverrides2 := make(map[string]any)
	dymintTomlOverrides2 := make(testutil.Toml)
	dymintTomlOverrides2["settlement_layer"] = "dymension"
	dymintTomlOverrides2["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides2["rollapp_id"] = "rollappevm_12345-1"
	dymintTomlOverrides2["gas_prices"] = "0adym"

	configFileOverrides2["config/dymint.toml"] = dymintTomlOverrides2
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
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp1",
				ChainID:             "rollappevm_12345-1",
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
				ConfigFileOverrides: configFileOverrides2,
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
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage("ghcr.io/decentrio/relayer", "e2e-amd", "100:1000"),
	).Build(t, client, network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: r,
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
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1, rollapp2)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollapp1User, rollapp2User := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollapp1UserAddr := rollapp1User.FormattedAddress()
	rollapp2UserAddr := rollapp2User.FormattedAddress()

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, dymensionOrigBal)

	rollappOrigBal, err := rollapp1.GetBalance(ctx, rollapp1UserAddr, rollapp1.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollappOrigBal)

	rollapp2OrigBal, err := rollapp2.GetBalance(ctx, rollapp2UserAddr, rollapp2.Config().Denom)
	require.NoError(t, err)
	require.Equal(t, walletAmount, rollapp2OrigBal)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 3, dymension, rollapp1)
	require.NoError(t, err)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()

	err = testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
	require.NoError(t, err)

	// Convert the uint to string
	myUintStr := fmt.Sprintf("%d", 50)

	// Create a JSON string with the uint value quoted
	jsonData := fmt.Sprintf(`"%s"`, myUintStr)

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

	err = dymension.VoteOnProposalAllValidators(ctx, propTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, height+30, propTx.ProposalID, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	new_params, err := dymension.QueryParam(ctx, "rollapp", "DisputePeriodInBlocks")
	require.NoError(t, err)
	require.Equal(t, new_params.Value, string(disputeblock))

	oldLatestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, "rollappevm_1234-1")
	require.NoError(t, err)

	// Access the index value
	index := oldLatestIndex.StateIndex.Index
	oldUintIndex, err := strconv.ParseUint(index, 10, 64)
	require.NoError(t, err)

	targetIndex := oldUintIndex + 1

	rollapp2Index, err := dymension.GetNode().QueryLatestStateIndex(ctx, "rollappevm_12345-1")
	require.NoError(t, err)

	// Loop until the latest index updates
	for {
		latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, "rollappevm_1234-1")
		require.NoError(t, err)

		index := latestIndex.StateIndex.Index
		uintIndex, err := strconv.ParseUint(index, 10, 64)
		require.NoError(t, err)

		if uintIndex >= targetIndex {
			break
		}
	}

	sequencerAddr, err := dymension.AccountKeyBech32WithKeyDir(ctx, "sequencer", keyDir)
	require.NoError(t, err)

	submitFraudStr := "fraud"
	deposit := "500000000000" + dymension.Config().Denom

	rollappHeight, err := rollapp1.Height(ctx)
	require.NoError(t, err)

	fraudHeight := fmt.Sprint(rollappHeight - 5)

	// Submit fraud proposal and all votes yes so the gov will pass and got executed.
	err = dymension.SubmitFraudProposal(ctx, dymensionUser.KeyName(), "rollappevm_1234-1", fraudHeight, sequencerAddr, "07-tendermint-0", submitFraudStr, submitFraudStr, deposit)
	require.NoError(t, err)

	err = dymension.VoteOnProposalAllValidators(ctx, "2", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	// Wait a few blocks for the gov to pass and to verify if the state index increment
	err = testutil.WaitForBlocks(ctx, 50, dymension, rollapp1)
	require.NoError(t, err)

	// Check if rollapp1 has frozen or not
	rollappParams, err := dymension.QueryRollappParams(ctx, "rollappevm_1234-1")
	require.NoError(t, err)
	require.Equal(t, rollappParams.Rollapp.Frozen, true, "rollapp does not frozen")

	// Check rollapp1 state index not increment
	latestIndex, err := dymension.GetNode().QueryLatestStateIndex(ctx, "rollappevm_1234-1")
	require.NoError(t, err)
	require.Equal(t, latestIndex.StateIndex.Index, fmt.Sprint(targetIndex), "rollapp state index still increment")

	// IBC channel for rollapps
	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	channel2, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp2.Config().ChainID)
	require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> rollapp
	var transferAmount = math.NewInt(1_000_000)

	// Confirm IBC Transfer not working between Dymension <-> rollapp1
	err = dymension.IBCTransfer(ctx,
		dymension, rollapp1, transferAmount, dymensionUserAddr,
		rollapp1UserAddr, r, ibcPath, channel,
		eRep, ibc.TransferOptions{})
	require.Error(t, err)

	err = rollapp1.IBCTransfer(ctx,
		rollapp1, dymension, transferAmount, rollapp1UserAddr,
		dymensionUserAddr, r, ibcPath, channel,
		eRep, ibc.TransferOptions{})
	require.Error(t, err)

	// Check other rollapp state index still increase 
	rollapp2IndexLater, err := dymension.GetNode().QueryLatestStateIndex(ctx, "rollappevm_12345-1")
	require.NoError(t, err)
	require.True(t, rollapp2IndexLater.StateIndex.Index > rollapp2Index.StateIndex.Index, "Another rollapp got freeze")

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// IBC Transfer working between Dymension <-> rollapp2
	err = dymension.IBCTransfer(ctx,
		dymension, rollapp2, transferAmount, dymensionUserAddr,
		rollapp2UserAddr, r, anotherIbcPath, channel2,
		eRep, ibc.TransferOptions{})
	require.NoError(t, err)

	err = rollapp2.IBCTransfer(ctx,
		rollapp2, dymension, transferAmount, rollapp2UserAddr,
		dymensionUserAddr, r, anotherIbcPath, channel2,
		eRep, ibc.TransferOptions{})
	require.NoError(t, err)
}
