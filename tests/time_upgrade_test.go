package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
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

func Test_TimeBaseUpgrade_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"
	dymintTomlOverrides["da_config"] = "{\"host\":\"grpc-da-container\",\"port\": 7980}"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 1
	numRollAppVals := 1

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

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
				ModifyGenesis:       modifyRollappEVMGenesis(modifyEVMGenesisKV),
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

	StartDA(ctx, t, client, network)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappevm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.FullNodes[0].HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
	if err != nil {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		require.NoError(t, err)
	}

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 1, "should have 1 sequences")

	// Assuming block height is located in status["SyncInfo"]["latest_block_height"]
	rollappHeightBeforeUpgrade, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err, "Failed to query Rollapp1 height before upgrade")
	fmt.Printf("Rollapp1 current block height before upgrade version: %d\n", rollappHeightBeforeUpgrade)

	height, err := rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height before submit upgrade proposal")

	haltHeight := height + haltHeightDelta
	check, _ := rollapp1.GetNode().CliContext().GetNode()
	blockInfo, _ := check.Block(ctx, &height)
	blockTime := blockInfo.Block.Header.Time
	fmt.Println("blockTime:", blockTime.Format(time.RFC3339))
	if err != nil {
		panic(fmt.Errorf("failed to get latest block time: %w", err))
	}

	upgradeTime := blockTime.Add(90 * time.Second).Format(time.RFC3339)
	fmt.Println("Upgrade Time:", upgradeTime)
	msg := map[string]interface{}{
		"@type":        "/rollapp.timeupgrade.types.MsgSoftwareUpgrade",
		"authority":    "ethm10d07y265gmmuvt4z0w9aw880jnsr700jpva843",
		"drs":          4,
		"upgrade_time": upgradeTime,
	}

	rawMsg, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Err:", err)
	}

	proposal := cosmos.TxProposalV1{
		Deposit:     "500000000000" + rollapp1.Config().Denom,
		Title:       "Update Dymension to DRS-2",
		Summary:     "This proposal aims to upgrade the Dymension rollapp to DRS 2, implementing new features and improvements, with a scheduled upgrade time.",
		Description: "Upgrade Dymension to DRS-2 version with scheduled upgrade time",
		Messages:    []json.RawMessage{rawMsg},
		Expedited:   true,
	}

	_, err = rollapp1.GetNode().SubmitProposal(ctx, rollappUser.KeyName(), proposal)
	require.NoError(t, err, "error submitting software upgrade proposal tx")

	txProposal, err := rollapp1.GovDeposit(ctx, rollappUser.KeyName(), "1", "500000000000urax")
	fmt.Printf("Successfully deposited for proposal: %v\n", txProposal)

	err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	_, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
	require.NoError(t, err)

	prop, err := rollapp1.QueryProposal(ctx, "1")
	require.NoError(t, err)

	fmt.Println("prop: ", prop)

	require.Equal(t, cosmos.ProposalStatusPassed, prop.Status)
	require.NoError(t, err, "proposal status did not change to passed in expected number of blocks")

	timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, time.Second*90)
	defer timeoutCtxCancel()

	height, err = rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height before upgrade")

	// bring down nodes to prepare for upgrade
	check2, _ := rollapp1.GetNode().CliContext().GetNode()
	blockInfo2, _ := check2.Block(ctx, &height)
	blockTime2 := blockInfo2.Block.Header.Time
	fmt.Println("blockTime2:", blockTime2.Format(time.RFC3339))
	parsedUpgradeTime, err := time.Parse(time.RFC3339, upgradeTime)
	fmt.Println("parsedUpgradeTime:", parsedUpgradeTime)
	if err != nil {
		panic(fmt.Errorf("failed to parse upgrade time: %w", err))
	}

	if blockTime2.After(parsedUpgradeTime) {
		err = rollapp1.StopAllNodes(ctx)
	} else {
		// fmt.Println("blockTime2 is before upgradeTime")
		err = testutil.WaitForTime(timeoutCtx, blockTime2, parsedUpgradeTime)
		if err != nil {
			fmt.Errorf("failed to wait for upgrade time: %w", err)
		}
		err = rollapp1.StopAllNodes(ctx)
	}
	require.NoError(t, err, "error stopping node(s)")

	// upgrade version on all nodes
	rollapp1.UpgradeVersion(ctx, client, RollappEVMMainRepo, rollappEVMVersion)

	// start all nodes back up.
	// validators reach consensus on first block after upgrade height
	// and chain block production resumes.

	err = rollapp1.StartAllNodes(ctx)
	require.NoError(t, err, "error starting upgraded node(s)")

	rollappHeightAfterUpgrade, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err, "Failed to query Rollapp1 height after upgrade")
	fmt.Printf("Rollapp1 current block height after upgrade version: %d\n", rollappHeightAfterUpgrade)
	require.Greater(t, rollappHeightAfterUpgrade, rollappHeightBeforeUpgrade, "Block height after upgrade should be greater than before upgrade")

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))
}

func Test_TimeBaseUpgrade_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"
	dymintTomlOverrides["da_config"] = "{\"host\":\"grpc-da-container\",\"port\": 7980}"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 1
	numRollAppVals := 1

	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
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
				ModifyGenesis:       modifyRollappWasmGenesis(modifyWasmGenesisKV),
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

	StartDA(ctx, t, client, network)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappwasm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.FullNodes[0].HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
	if err != nil {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		require.NoError(t, err)
	}

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 1, "should have 1 sequences")

	// Assuming block height is located in status["SyncInfo"]["latest_block_height"]
	rollappHeightBeforeUpgrade, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err, "Failed to query Rollapp1 height before upgrade")
	fmt.Printf("Rollapp1 current block height before upgrade version: %d\n", rollappHeightBeforeUpgrade)

	height, err := rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height before submit upgrade proposal")

	haltHeight := height + haltHeightDelta
	check, _ := rollapp1.GetNode().CliContext().GetNode()
	blockInfo, _ := check.Block(ctx, &height)
	blockTime := blockInfo.Block.Header.Time
	fmt.Println("blockTime:", blockTime.Format(time.RFC3339))
	if err != nil {
		panic(fmt.Errorf("failed to get latest block time: %w", err))
	}

	upgradeTime := blockTime.Add(90 * time.Second).Format(time.RFC3339)
	fmt.Println("Upgrade Time:", upgradeTime)
	msg := map[string]interface{}{
		"@type":        "/rollapp.timeupgrade.types.MsgSoftwareUpgrade",
		"authority":    "ethm10d07y265gmmuvt4z0w9aw880jnsr700jpva843",
		"drs":          4,
		"upgrade_time": upgradeTime,
	}

	rawMsg, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Err:", err)
	}

	proposal := cosmos.TxProposalV1{
		Deposit:     "500000000000" + rollapp1.Config().Denom,
		Title:       "Update Dymension to DRS-2",
		Summary:     "This proposal aims to upgrade the Dymension rollapp to DRS 2, implementing new features and improvements, with a scheduled upgrade time.",
		Description: "Upgrade Dymension to DRS-2 version with scheduled upgrade time",
		Messages:    []json.RawMessage{rawMsg},
		Expedited:   true,
	}

	_, err = rollapp1.GetNode().SubmitProposal(ctx, rollappUser.KeyName(), proposal)
	require.NoError(t, err, "error submitting software upgrade proposal tx")

	txProposal, err := rollapp1.GovDeposit(ctx, rollappUser.KeyName(), "1", "500000000000urax")
	fmt.Printf("Successfully deposited for proposal: %v\n", txProposal)

	err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	_, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
	require.NoError(t, err)

	prop, err := rollapp1.QueryProposal(ctx, "1")
	require.NoError(t, err)

	fmt.Println("prop: ", prop)

	require.Equal(t, cosmos.ProposalStatusPassed, prop.Status)
	require.NoError(t, err, "proposal status did not change to passed in expected number of blocks")

	timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, time.Second*90)
	defer timeoutCtxCancel()

	height, err = rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height before upgrade")

	// bring down nodes to prepare for upgrade
	check2, _ := rollapp1.GetNode().CliContext().GetNode()
	blockInfo2, _ := check2.Block(ctx, &height)
	blockTime2 := blockInfo2.Block.Header.Time
	fmt.Println("blockTime2:", blockTime2.Format(time.RFC3339))
	parsedUpgradeTime, err := time.Parse(time.RFC3339, upgradeTime)
	fmt.Println("parsedUpgradeTime:", parsedUpgradeTime)
	if err != nil {
		panic(fmt.Errorf("failed to parse upgrade time: %w", err))
	}

	if blockTime2.After(parsedUpgradeTime) {
		err = rollapp1.StopAllNodes(ctx)
	} else {
		// fmt.Println("blockTime2 is before upgradeTime")
		err = testutil.WaitForTime(timeoutCtx, blockTime2, parsedUpgradeTime)
		if err != nil {
			fmt.Errorf("failed to wait for upgrade time: %w", err)
		}
		err = rollapp1.StopAllNodes(ctx)
	}
	require.NoError(t, err, "error stopping node(s)")

	// upgrade version on all nodes
	rollapp1.UpgradeVersion(ctx, client, RollappWasmMainRepo, rollappWasmVersion)

	// start all nodes back up.
	// validators reach consensus on first block after upgrade height
	// and chain block production resumes.

	err = rollapp1.StartAllNodes(ctx)
	require.NoError(t, err, "error starting upgraded node(s)")

	rollappHeightAfterUpgrade, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err, "Failed to query Rollapp1 height after upgrade")
	fmt.Printf("Rollapp1 current block height after upgrade version: %d\n", rollappHeightAfterUpgrade)
	require.Greater(t, rollappHeightAfterUpgrade, rollappHeightBeforeUpgrade, "Block height after upgrade should be greater than before upgrade")

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))
}


func Test_TimeBaseUpgradeInPast_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"
	dymintTomlOverrides["da_config"] = "{\"host\":\"grpc-da-container\",\"port\": 7980}"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 1
	numRollAppVals := 1

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

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
				ModifyGenesis:       modifyRollappEVMGenesis(modifyEVMGenesisKV),
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

	StartDA(ctx, t, client, network)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappevm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.FullNodes[0].HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
	if err != nil {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		require.NoError(t, err)
	}

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 1, "should have 1 sequences")

	// Assuming block height is located in status["SyncInfo"]["latest_block_height"]
	rollappHeightBeforeUpgrade, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err, "Failed to query Rollapp1 height before upgrade")
	fmt.Printf("Rollapp1 current block height before upgrade version: %d\n", rollappHeightBeforeUpgrade)

	height, err := rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height before submit upgrade proposal")

	haltHeight := height + haltHeightDelta
	check, _ := rollapp1.GetNode().CliContext().GetNode()
	blockInfo, _ := check.Block(ctx, &height)
	// println("check data blocktime: ",blockInfo.Block.String())
	// blockTime, err := rollapp1.GetLatestBlockTime(ctx)
	blockTime := blockInfo.Block.Header.Time
	fmt.Println("blockTime:", blockTime.Format(time.RFC3339))
	if err != nil {
		panic(fmt.Errorf("failed to get latest block time: %w", err))
	}

	upgradeTime := "2024-09-06T18:10:00Z" // upgrade time in the past
	fmt.Println("Upgrade Time:", upgradeTime)
	msg := map[string]interface{}{
		"@type":        "/rollapp.timeupgrade.types.MsgSoftwareUpgrade",
		"authority":    "ethm10d07y265gmmuvt4z0w9aw880jnsr700jpva843",
		"drs":          2,
		"upgrade_time": upgradeTime,
	}

	rawMsg, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Err:", err)
	}

	proposal := cosmos.TxProposalV1{
		Deposit:     "500000000000" + rollapp1.Config().Denom,
		Title:       "Update Dymension to DRS-2",
		Summary:     "This proposal aims to upgrade the Dymension rollapp to DRS 2, implementing new features and improvements, with a scheduled upgrade time.",
		Description: "Upgrade Dymension to DRS-2 version with scheduled upgrade time",
		Messages:    []json.RawMessage{rawMsg},
		Expedited:   true,
	}

	_, err = rollapp1.GetNode().SubmitProposal(ctx, rollappUser.KeyName(), proposal)
	require.NoError(t, err, "failed to submit proposal")

	txProposal, err := rollapp1.GovDeposit(ctx, rollappUser.KeyName(), "1", "500000000000urax")
	fmt.Printf("Successfully deposited for proposal: %v\n", txProposal)

	err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	_, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
	require.Error(t, err) // this should error out as upgrade time is in the past

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))
}

func Test_TimeBaseUpgradeInPast_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"
	dymintTomlOverrides["da_config"] = "{\"host\":\"grpc-da-container\",\"port\": 7980}"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 1
	numRollAppVals := 1

	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
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
				ModifyGenesis:       modifyRollappWasmGenesis(modifyWasmGenesisKV),
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

	StartDA(ctx, t, client, network)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	containerID := fmt.Sprintf("ra-rollappwasm_1234-1-val-0-%s", t.Name())

	// Get the container details
	containerJSON, err := client.ContainerInspect(context.Background(), containerID)
	require.NoError(t, err)

	// Extract the IP address from the network settings
	// If the container is using a custom network, the IP might be under a specific network name
	var ipAddress string
	for _, network := range containerJSON.NetworkSettings.Networks {
		ipAddress = network.IPAddress
		break // Assuming we only need the IP from the first network
	}

	nodeId, err := rollapp1.Validators[0].GetNodeId(ctx)
	require.NoError(t, err)
	nodeId = strings.TrimRight(nodeId, "\n")
	p2p_bootstrap_node := fmt.Sprintf("/ip4/%s/tcp/26656/p2p/%s", ipAddress, nodeId)

	rollapp1HomeDir := strings.Split(rollapp1.FullNodes[0].HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	file, err := os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
	if err != nil {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		require.NoError(t, err)
	}

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, rollappUser := users[0], users[1]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 1, "should have 1 sequences")

	// Assuming block height is located in status["SyncInfo"]["latest_block_height"]
	rollappHeightBeforeUpgrade, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err, "Failed to query Rollapp1 height before upgrade")
	fmt.Printf("Rollapp1 current block height before upgrade version: %d\n", rollappHeightBeforeUpgrade)

	height, err := rollapp1.Height(ctx)
	require.NoError(t, err, "error fetching height before submit upgrade proposal")

	haltHeight := height + haltHeightDelta
	check, _ := rollapp1.GetNode().CliContext().GetNode()
	blockInfo, _ := check.Block(ctx, &height)
	// println("check data blocktime: ",blockInfo.Block.String())
	// blockTime, err := rollapp1.GetLatestBlockTime(ctx)
	blockTime := blockInfo.Block.Header.Time
	fmt.Println("blockTime:", blockTime.Format(time.RFC3339))
	if err != nil {
		panic(fmt.Errorf("failed to get latest block time: %w", err))
	}

	upgradeTime := "2024-09-06T18:10:00Z" // upgrade time in the past
	fmt.Println("Upgrade Time:", upgradeTime)
	msg := map[string]interface{}{
		"@type":        "/rollapp.timeupgrade.types.MsgSoftwareUpgrade",
		"authority":    "ethm10d07y265gmmuvt4z0w9aw880jnsr700jpva843",
		"drs":          2,
		"upgrade_time": upgradeTime,
	}

	rawMsg, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Err:", err)
	}

	proposal := cosmos.TxProposalV1{
		Deposit:     "500000000000" + rollapp1.Config().Denom,
		Title:       "Update Dymension to DRS-2",
		Summary:     "This proposal aims to upgrade the Dymension rollapp to DRS 2, implementing new features and improvements, with a scheduled upgrade time.",
		Description: "Upgrade Dymension to DRS-2 version with scheduled upgrade time",
		Messages:    []json.RawMessage{rawMsg},
		Expedited:   true,
	}

	_, err = rollapp1.GetNode().SubmitProposal(ctx, rollappUser.KeyName(), proposal)
	require.NoError(t, err, "failed to submit proposal")

	txProposal, err := rollapp1.GovDeposit(ctx, rollappUser.KeyName(), "1", "500000000000urax")
	fmt.Printf("Successfully deposited for proposal: %v\n", txProposal)

	err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	_, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, haltHeight, "1", cosmos.ProposalStatusPassed)
	require.Error(t, err) // this should error out as upgrade time is in the past

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))
}

