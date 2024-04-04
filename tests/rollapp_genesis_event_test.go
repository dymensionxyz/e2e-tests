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

// TestRollappGenesisEvent_EVM ensure that genesis event triggered in both rollapp evm and dymension hub
// works properly
func TestRollappGenesisEvent_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := overridesDymintToml("dymension", t.Name(), "rollappevm_1234-1", "0adym")
	configFileOverrides2 := overridesDymintToml("dymension", t.Name(), "rollappevm_12345-1", "0adym")

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

	channsRollApp1, err := r.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)
	require.Len(t, channsRollApp1, 1)

	txHash, err := dymension.FullNodes[0].ExecTx(ctx, dymensionUserAddr, "rollapp", "genesis-event", rollapp1.GetChainID(), channsRollApp1[0].ChannelID, "--gas=auto")
	require.NoError(t, err)

	tx, err := dymension.GetTransaction(txHash)
	require.NoError(t, err)

	recipient, ok := cosmos.AttributeValue(tx.Events, "transfer", "recipient")
	require.True(t, ok, "failed to retrieve transfer recipient")
	coinStr, ok := cosmos.AttributeValue(tx.Events, "transfer", "amount")
	require.True(t, ok, "failed to retrieve transfer amount")

	genesisCoin, err := sdk.ParseCoinNormalized(coinStr)
	require.NoError(t, err)

	validatorAddr, err := dymension.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)
	require.Equal(t, recipient, validatorAddr)

	testutil.AssertBalance(t, ctx, dymension, validatorAddr, genesisCoin.Denom, genesisCoin.Amount)

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

	hubgenesisMAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "hubgenesis")
	require.NoError(t, err)

	hubgenesisMAccAddr := hubgenesisMAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, rollapp1, hubgenesisMAccAddr, rollapp1.Config().Denom, genesisCoin.Amount)

	_, err = rollapp1.Validators[0].ExecTx(ctx, rollappUserAddr, "hubgenesis", "genesis-event", dymension.GetChainID(), channsRollApp1[0].ChannelID)
	require.NoError(t, err)

	testutil.AssertBalance(t, ctx, rollapp1, hubgenesisMAccAddr, rollapp1.Config().Denom, sdk.ZeroInt())

	escrowAddress, err := rollapp1.Validators[0].QueryEscrowAddress(ctx, channsRollApp1[0].PortID, channsRollApp1[0].ChannelID)
	require.NoError(t, err)

	testutil.AssertBalance(t, ctx, rollapp1, escrowAddress, rollapp1.Config().Denom, genesisCoin.Amount)

	denommetadata, err := dymension.GetNode().QueryDenomMetadata(ctx, genesisCoin.Denom)
	require.NoError(t, err)

	require.Equal(t, denommetadata.Description, fmt.Sprintf("auto-generated metadata for %s from rollapp %s", genesisCoin.Denom, rollapp1.GetChainID()))
	require.Equal(t, denommetadata.Base, genesisCoin.Denom)
	denomUnits := []cosmos.DenomUnit{
		{
			Denom:    genesisCoin.Denom,
			Exponent: 0,
			Aliases:  []string{rollapp1.Config().Denom},
		},
		{
			Denom:    "rax",
			Exponent: 6,
			Aliases:  []string{},
		},
	}
	require.Equal(t, denommetadata.DenomUnits, denomUnits)
	require.Equal(t, denommetadata.Display, "rax")
	require.Equal(t, denommetadata.Symbol, "URAX")
	require.Equal(t, denommetadata.Name, fmt.Sprintf("%s %s", rollapp1.GetChainID(), rollapp1.Config().Denom))
}
