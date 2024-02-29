package tests

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"cosmossdk.io/math"
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

const (
	haltHeightDelta    = uint64(20)
	blocksAfterUpgrade = uint64(10)
	votingPeriod       = "30s"
	maxDepositPeriod   = "10s"
)

func TestHubUpgrade(t *testing.T) {
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

	dymensionConfig.ModifyGenesis = modifyGenesisShortProposals(votingPeriod, maxDepositPeriod)
	dymensionConfig.Images = []ibc.DockerImage{preUpgradeDymensionImage}

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
	const ibcPath = "ibc-path"
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
	// Make sure gov params has changed
	dymNode := dymension.FullNodes[0]
	votingParams, err := dymNode.QueryParam(ctx, "gov", "voting_params")
	require.NoError(t, err)
	fmt.Println("gov voting params: ", votingParams.Value)

	// Create some user accounts on both chains
	user := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension)[0]

	// Copy file to node
	fileName := "bytecode/dymd.tar"
	_, file := filepath.Split(fileName)
	err = dymNode.CopyFile(ctx, fileName, file)
	require.NoError(t, err, "err writing binary file to docker volume")

	// Get the file's checksum
	fileContent, err := os.ReadFile(fileName)
	require.NoError(t, err, "err reading binary file")
	sum := sha256.Sum256(fileContent)

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height before submit upgrade proposal")

	haltHeight := height + haltHeightDelta

	proposal := cosmos.SoftwareUpgradeProposal{
		Deposit:     "500000000000" + dymension.Config().Denom, // greater than min deposit
		Title:       "Chain Upgrade 1",
		Name:        "v3",
		Description: "First chain software upgrade",
		Height:      haltHeight,
		Info:        fmt.Sprintf("{ \"binaries\": { \"linux/amd64\":\"file://%s?checksum=sha256:%x\" } }", path.Join(dymNode.HomeDir(), file), sum),
	}

	upgradeTx, err := dymension.UpgradeLegacyProposal(ctx, user.KeyName(), proposal)
	require.NoError(t, err, "error submitting software upgrade proposal tx")
	fmt.Println("upgradeTx", upgradeTx)

	err = dymension.VoteOnProposalAllValidators(ctx, upgradeTx.ProposalID, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	_, err = cosmos.PollForProposalStatus(ctx, dymension.CosmosChain, height, haltHeight, upgradeTx.ProposalID, cosmos.ProposalStatusPassed)
	prop, _ := dymension.QueryProposal(ctx, upgradeTx.ProposalID)
	fmt.Println("prop: ", prop)
	require.Equal(t, prop.Status, cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed in expected number of blocks")

	timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, time.Second*45)
	defer timeoutCtxCancel()

	height, err = dymension.Height(ctx)
	require.NoError(t, err, "error fetching height before upgrade")

	// this should timeout due to chain halt at upgrade height.
	_ = testutil.WaitForBlocks(timeoutCtx, int(haltHeight-height)+1, dymension)

	height, err = dymension.Height(ctx)
	require.NoError(t, err, "error fetching height after chain should have halted")

	// make sure that chain is halted
	require.Equal(t, haltHeight, height, "height is not equal to halt height")

	// bring down nodes to prepare for upgrade
	err = dymension.StopAllNodes(ctx)
	require.NoError(t, err, "error stopping node(s)")

	// upgrade version on all nodes
	dymension.UpgradeVersion(ctx, client, DymensionMainRepo, dymensionVersion)

	// start all nodes back up.
	// validators reach consensus on first block after upgrade height
	// and chain block production resumes.
	err = dymension.StartAllNodes(ctx)
	require.NoError(t, err, "error starting upgraded node(s)")

	timeoutCtx, timeoutCtxCancel = context.WithTimeout(ctx, time.Second*45)
	defer timeoutCtxCancel()

	err = testutil.WaitForBlocks(timeoutCtx, int(blocksAfterUpgrade), dymension)
	require.NoError(t, err, "chain did not produce blocks after upgrade")

	height, err = dymension.Height(ctx)
	require.NoError(t, err, "error fetching height after upgrade")

	require.GreaterOrEqual(t, height, haltHeight+blocksAfterUpgrade, "height did not increment enough after upgrade")

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> rollapp
	var transferAmount = math.NewInt(1_000_000)

	err = dymension.IBCTransfer(ctx,
		dymension, rollapp1, transferAmount, dymensionUserAddr,
		rollappUserAddr, r, ibcPath, channel,
		eRep, ibc.TransferOptions{})
	require.NoError(t, err)
}
