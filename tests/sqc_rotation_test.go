package tests

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/strslice"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/celes_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"

	"strconv"
)

func Test_SqcRotation_success_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

	// setup config for rollapp 1
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_max_time"] = "3600s"
	dymintTomlOverrides["block_time"] = "20ms"
	dymintTomlOverrides["p2p_gossip_cache_size"] = "1"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"
	dymintTomlOverrides["p2p_blocksync_block_request_interval"] = 10
	dymintTomlOverrides["da_layer"] = "grpc"
	dymintTomlOverrides["da_config"] = "{\"host\":\"host.docker.internal\",\"port\": 7980}"

	configFileOverrides1 := make(map[string]any)
	configTomlOverrides1 := make(testutil.Toml)
	configTomlOverrides1["timeout_commit"] = "2s"
	configTomlOverrides1["timeout_propose"] = "2s"
	configTomlOverrides1["index_all_keys"] = "true"
	configTomlOverrides1["mode"] = "validator"

	configFileOverrides1["config/config.toml"] = configTomlOverrides1

	numHubVals := 1
	numHubFullNodes := 1
	numCelestiaFn := 0
	numRollAppFn := 1
	numRollAppVals := 1
	nodeStore := "/home/celestia/light"
	p2pNetwork := "mocha-4"
	coreIp := "celestia-testnet-consensus.itrocket.net"

	url := "https://api-mocha.celenium.io/v1/block/count"
	headerKey := "User-Agent"
	headerValue := "Apidog/1.0.0 (https://apidog.com)"
	rpcEndpoint := "http://celestia-testnet-consensus.itrocket.net:26657"

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "celes-hub",
			ChainConfig: ibc.ChainConfig{
				Name:           "celestia",
				Denom:          "utia",
				Type:           "hub-celes",
				GasPrices:      "0.002utia",
				TrustingPeriod: "112h",
				ChainID:        "test",
				Bin:            "celestia-appd",
				Images: []ibc.DockerImage{
					{
						Repository: "ghcr.io/decentrio/light",
						Version:    "latest",
						UidGid:     "1025:1025",
					},
				},
				Bech32Prefix:        "celestia",
				CoinType:            "118",
				GasAdjustment:       1.5,
				ConfigFileOverrides: configFileOverrides1,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numCelestiaFn,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	celestia := chains[0].(*celes_hub.CelesHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	ic := test.NewSetup().
		AddChain(celestia)

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,
	}, nil, "", nil)
	require.NoError(t, err)

	validator, err := celestia.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	// Get fund for submit blob
	GetFaucet("http://18.184.170.181:3000/api/get-tia", validator)
	err = testutil.WaitForBlocks(ctx, 2, celestia)
	require.NoError(t, err)

	err = celestia.GetNode().InitCelestiaDaLightNode(ctx, nodeStore, p2pNetwork, nil)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 3, celestia)
	require.NoError(t, err)

	// Change the file permissions
	command := []string{"chmod", "-R", "777", "/home/celestia/light/config.toml"}

	_, _, err = celestia.Exec(ctx, command, nil)
	require.NoError(t, err)

	file, err := os.Open("/tmp/celestia/light/config.toml")
	require.NoError(t, err)
	defer file.Close()

	lastestBlockHeight, err := GetLatestBlockHeight(url, headerKey, headerValue)
	require.NoError(t, err)
	lastestBlockHeight = strings.TrimRight(lastestBlockHeight, "\n")
	heightOfBlock, err := strconv.ParseInt(lastestBlockHeight, 10, 64) // base 10, bit size 64
	require.NoError(t, err)

	hash, err := celestia.GetNode().GetHashOfBlockHeightWithCustomizeRpcEndpoint(ctx, fmt.Sprintf("%d", heightOfBlock-2), rpcEndpoint)
	require.NoError(t, err)

	hash = strings.TrimRight(hash, "\n")
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "  TrustedHash =") {
			lines[i] = fmt.Sprintf("  TrustedHash = \"%s\"", hash)
		} else if strings.HasPrefix(line, "  SampleFrom =") {
			lines[i] = fmt.Sprintf("  SampleFrom = %d", heightOfBlock-2)
		} else if strings.HasPrefix(line, "  Address =") {
			lines[i] = fmt.Sprintf("  Address = \"0.0.0.0\"")
		}
	}

	output := strings.Join(lines, "\n")
	file, err = os.Create("/tmp/celestia/light/config.toml")
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	containerID := fmt.Sprintf("test-val-0-%s", t.Name())

	// Create an exec instance
	execConfig := types.ExecConfig{
		Cmd: strslice.StrSlice([]string{"celestia", "light", "start", "--node.store", nodeStore, "--gateway", "--core.ip", coreIp, "--p2p.network", p2pNetwork, "--keyring.accname", "validator"}), // Replace with your command and arguments
	}

	execIDResp, err := client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		panic(err)
	}

	execID := execIDResp.ID

	// Start the exec instance
	execStartCheck := types.ExecStartCheck{
		Tty: false,
	}

	if err := client.ContainerExecStart(ctx, execID, execStartCheck); err != nil {
		panic(err)
	}

	err = testutil.WaitForBlocks(ctx, 10, celestia)
	require.NoError(t, err)

	celestia_token, err := celestia.GetNode().GetAuthTokenCelestiaDaLight(ctx, p2pNetwork, nodeStore)
	require.NoError(t, err)
	println("check token: ", celestia_token)
	celestia_namespace_id, err := RandomHex(10)
	require.NoError(t, err)
	println("check namespace: ", celestia_namespace_id)

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides["da_layer"] = "grpc"
	dymintTomlOverrides["da_config"] = "{\"host\":\"host.docker.internal\",\"port\": 7980}"
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// setup 3 sequencers
	numHubVals = 3

	cf = test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
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
			ChainConfig:   dymensionConfig,
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err = cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	// relayer for rollapp 1
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer1", network)

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,
	}, nil, "", nil)
	require.Error(t, err)

	// create 3 sequencers
	_, _, err = rollapp1.GetNode().ExecInit(ctx, "sequencer1", "/var/cosmos-chain/sequencer1")
	require.NoError(t, err)

	cmd := append([]string{rollapp1.Validators[0].Chain.Config().Bin}, "dymint", "show-sequencer", "--home", "/var/cosmos-chain/sequencer1")
	pub1, _, err := rollapp1.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, _, err = rollapp1.Validators[1].ExecInit(ctx, "sequencer2", "/var/cosmos-chain/sequencer2")
	require.NoError(t, err)

	cmd2 := append([]string{rollapp1.GetNode().Chain.Config().Bin}, "dymint", "show-sequencer", "--home", "/var/cosmos-chain/sequencer2")
	pub2, _, err := rollapp1.GetNode().Exec(ctx, cmd2, nil)
	require.NoError(t, err)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	sequencer1, sequencer2, dymensionUser, marketMaker, rollappUser := users[0], users[1], users[2], users[3], users[4]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	cmd = []string{}
	cmd = append(command, "sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, rollapp1.GetSequencerKeyDir()+"/metadata_sequencer.json", "1000000000adym",
		"--broadcast-mode", "async")

	_, err = dymension.Validators[1].ExecTx(ctx, sequencer1.KeyName(), command...)
	require.NoError(t, err)

	command = []string{}
	command = append(command, "sequencer", "create-sequencer", string(pub2), rollapp1.Config().ChainID, rollapp1.GetSequencerKeyDir()+"/metadata_sequencer.json", "1100000000adym",
		"--broadcast-mode", "async")

	_, err = dymension.Validators[2].ExecTx(ctx, sequencer2.KeyName(), command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 3, "should have 3 sequences")

	// verify ibc transfer work before unbond sqc1
	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	dymChannels, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)

	require.Len(t, channsRollApp1, 1)
	require.Len(t, dymChannels, 2)

	channel, err := ibc.GetTransferChannel(ctx, r1, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	// send ibc transfer from rollapp1 to dymension (also act as 'normal' transfer for enabling ibc transfer)
	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Wait for finalized
	rollapp1Height, err := rollapp1.Height(ctx)
	require.NoError(t, err)
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 300)
	require.True(t, isFinalized)
	require.NoError(t, err)

	// assert the balances
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Add(transferAmount))
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferAmount))

	// Unbond sequencer1
	err = dymension.Unbond(ctx, sequencer1.KeyName(), "")
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, sequencer1.FormattedAddress())
	require.NoError(t, err)
	require.Equal(t, queryGetSequencerResponse.Sequencer.Status, "OPERATING_STATUS_UNBONDING")

	// Check ibc transfer works during the sequencer rotation
	// send ibc transfer from rollapp1 to dym using eibc
	// TODO: add eibc transfer

	containerID = fmt.Sprintf("rollappevm_1234-1-val-0-%s", t.Name())

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

	rollapp1HomeDir := strings.Split(rollapp1.HomeDir(), "/")
	rollapp1FolderName := rollapp1HomeDir[len(rollapp1HomeDir)-1]

	// Change the file permissions
	command = []string{"chmod", "-R", "777", fmt.Sprintf("/var/cosmos-chain/%s/config/dymint.toml", rollapp1FolderName)}

	_, _, err = celestia.Exec(ctx, command, nil)
	require.NoError(t, err)

	file, err = os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines = []string{}
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "p2p_bootstrap_nodes =") {
			lines[i] = fmt.Sprintf("p2p_bootstrap_nodes = \"%s\"", p2p_bootstrap_node)
		}
	}

	output = strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	// update dymint.toml for full node to connect with Celestia DA
	fnHomeDir := strings.Split(rollapp1.FullNodes[0].HomeDir(), "/")
	fnFolderName := fnHomeDir[len(fnHomeDir)-1]
	command = []string{"chmod", "-R", "777", fmt.Sprintf("/var/cosmos-chain/%s/config/dymint.toml", fnFolderName)}

	_, _, err = celestia.Exec(ctx, command, nil)
	require.NoError(t, err)

	file, err = os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", fnFolderName))
	require.NoError(t, err)
	defer file.Close()

	lines = []string{}
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	da_layer := "celestia"
	for i, line := range lines {
		if strings.HasPrefix(line, "da_layer =") {
			lines[i] = fmt.Sprintf("da_layer = \"%s\"", da_layer)
		} else if strings.HasPrefix(line, "namespace_id =") {
			lines[i] = fmt.Sprintf("namespace_id = \"%s\"", celestia_namespace_id)
		} else if strings.HasPrefix(line, "da_config =") {
			lines[i] = fmt.Sprintf("da_config = \"{\\\"base_url\\\": \\\"http://test-val-0-%s:26658\\\", \\\"timeout\\\": 60000000000, \\\"gas_prices\\\":1.0, \\\"gas_adjustment\\\": 1.3, \\\"namespace_id\\\": \\\"%s\\\", \\\"auth_token\\\":\\\"%s\\\"}\"", t.Name(), celestia_namespace_id, celestia_token)
		}
	}

	output = strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", fnFolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	valHeight, err := rollapp1.Validators[2].Height(ctx)
	require.NoError(t, err)

	// how to wait for notice_period ?? change GOV ??
	// TODO add notice_period handling

	// status of sequencer3 should be bonded
	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, sequencer2.FormattedAddress())
	require.NoError(t, err)
	require.Equal(t, queryGetSequencerResponse.Sequencer.Status, "OPERATING_STATUS_BONDED")

	// status of sequencer1 should be unbonded
	queryGetSequencerResponse, err = dymension.QueryShowSequencer(ctx, sequencer1.FormattedAddress())
	require.NoError(t, err)
	require.Equal(t, queryGetSequencerResponse.Sequencer.Status, "OPERATING_STATUS_UNBONDED")

	// check ibc transfer works after rotation is done
	// TODO: add eibc transfer

	//Poll until full node is sync
	err = testutil.WaitForCondition(
		time.Minute*50,
		time.Second*5, // each epoch is 5 seconds
		func() (bool, error) {
			fullnodeHeight, err := rollapp1.FullNodes[0].Height(ctx)
			require.NoError(t, err)

			fmt.Println("valHeight", valHeight, " || fullnodeHeight", fullnodeHeight)
			if valHeight > fullnodeHeight {
				return false, nil
			}

			return true, nil
		},
	)
	require.NoError(t, err)
}
