package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/ignite/cli/ignite/pkg/cosmosaccount"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/celes_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

type Member struct {
	Address  string `json:"address"`
	Weight   string `json:"weight"`
	Metadata string `json:"metadata"`
}

type MembersJSON struct {
	Members []Member `json:"members"`
}

type Config struct {
	HomeDir      string             `yaml:"home_dir"`
	NodeAddress  string             `yaml:"node_address"`
	DBPath       string             `yaml:"db_path"`
	Gas          GasConfig          `yaml:"gas"`
	OrderPolling OrderPollingConfig `yaml:"order_polling"`

	Whale           whaleConfig     `yaml:"whale"`
	Bots            botConfig       `yaml:"bots"`
	FulfillCriteria fulfillCriteria `yaml:"fulfill_criteria"`

	LogLevel    string      `yaml:"log_level"`
	SlackConfig slackConfig `yaml:"slack"`
	SkipRefund  bool        `yaml:"skip_refund"`
}

type OrderPollingConfig struct {
	IndexerURL string        `yaml:"indexer_url"`
	Interval   time.Duration `yaml:"interval"`
	Enabled    bool          `yaml:"enabled"`
}

type GasConfig struct {
	Prices            string `yaml:"prices"`
	Fees              string `yaml:"fees"`
	MinimumGasBalance string `yaml:"minimum_gas_balance"`
}

type botConfig struct {
	NumberOfBots   int                          `yaml:"number_of_bots"`
	KeyringBackend cosmosaccount.KeyringBackend `yaml:"keyring_backend"`
	KeyringDir     string                       `yaml:"keyring_dir"`
	TopUpFactor    int                          `yaml:"top_up_factor"`
	MaxOrdersPerTx int                          `yaml:"max_orders_per_tx"`
}

type whaleConfig struct {
	AccountName              string                       `yaml:"account_name"`
	KeyringBackend           cosmosaccount.KeyringBackend `yaml:"keyring_backend"`
	KeyringDir               string                       `yaml:"keyring_dir"`
	AllowedBalanceThresholds map[string]string            `yaml:"allowed_balance_thresholds"`
}

type fulfillCriteria struct {
	MinFeePercentage minFeePercentage `yaml:"min_fee_percentage"`
}

type minFeePercentage struct {
	Chain map[string]float32 `yaml:"chain"`
	Asset map[string]float32 `yaml:"asset"`
}

type slackConfig struct {
	Enabled   bool   `yaml:"enabled"`
	BotToken  string `yaml:"bot_token"`
	AppToken  string `yaml:"app_token"`
	ChannelID string `yaml:"channel_id"`
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	// Open the source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create the destination file
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy the contents from the source to the destination
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Flush the content to the destination file to ensure all data is written
	err = destFile.Sync()
	if err != nil {
		return err
	}

	return nil
}
func StartDB(ctx context.Context, t *testing.T, client *client.Client, net string) {
	fmt.Println("Starting pull image ...")
	out, err := client.ImagePull(ctx, "mongo:7.0", types.ImagePullOptions{})
	require.NoError(t, err)
	defer out.Close()

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			net: {},
		},
	}
	portBindings := nat.PortMap{
		"27017/tcp": []nat.PortBinding{
			{
				HostIP:   "0.0.0.0", // Host IP address (use 0.0.0.0 for all interfaces)
				HostPort: "27017",   // Host port to bind to
			},
		},
	}
	hostConfig := &container.HostConfig{
		PortBindings:    portBindings,
		PublishAllPorts: true,
		AutoRemove:      false,
		DNS:             []string{},
		ExtraHosts:      []string{"host.docker.internal:host-gateway"},
	}
	time.Sleep(2 * time.Minute)
	// Create the container
	fmt.Println("Creating container ...")
	resp, err := client.ContainerCreate(
		ctx,
		&container.Config{
			Image: "mongo:7.0", // Image to run
			Tty:   true,        // Attach to a TTY
		},
		hostConfig, networkConfig, nil, "mongodb-container",
	)
	require.NoError(t, err)

	fmt.Println("Starting container ...")

	// Start the container
	err = client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	require.NoError(t, err)
}

func Test_EIBC_Client_Success_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = "{\"host\":\"grpc-da-container\",\"port\": 7980}"

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
	)

	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 1
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
				ModifyGenesis:       modifyRollappEVMGenesis(modifyEVMGenesisKV),
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "dymension-hub",
			ChainConfig: ibc.ChainConfig{
				Type:                "hub-dym",
				Name:                "dymension",
				ChainID:             "dymension_100-1",
				Images:              []ibc.DockerImage{dymensionImage},
				Bin:                 "dymd",
				Bech32Prefix:        "dym",
				Denom:               "adym",
				CoinType:            "60",
				GasPrices:           "0.0adym",
				EncodingConfig:      encodingConfig(),
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				NoHostMount:         false,
				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
				ConfigFileOverrides: nil,
				SidecarConfigs: []ibc.SidecarConfig{
					{
						ProcessName:      "eibc-client",
						Image:            eibcClientImage,
						HomeDir:          "/root",
						Ports:            nil,
						StartCmd:         []string{"eibc-client", "start", "--config", "/root/.eibc-client/config.yaml"},
						PreStart:         true,
						ValidatorProcess: false,
					},
				},
			},
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
	}, nil, "", nil, true, 1179360)
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

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, dymensionUser2, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	dymensionUserAddr2 := dymensionUser2.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  bigTransferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

	membersData, err := ioutil.ReadFile("./data/members.json")
	require.NoError(t, err)

	var members MembersJSON
	err = json.Unmarshal(membersData, &members)
	require.NoError(t, err)

	newAddress := dymensionUserAddr
	for i := range members.Members {
		members.Members[i].Address = newAddress
	}

	updatedJSON, err := json.MarshalIndent(members, "", "  ")
	require.NoError(t, err)

	err = ioutil.WriteFile("./data/members.json", updatedJSON, 0755)
	require.NoError(t, err)

	txHash, err := dymension.GetNode().CreateGroup(ctx, dymensionUser.KeyName(), "==A", dymension.HomeDir() + "/members.json")
	fmt.Println(txHash)
	require.NoError(t, err)

	txHash, err = dymension.GetNode().CreateGroupPolicy(ctx, dymensionUser.KeyName(), "==A",  dymension.HomeDir() + "/policy.json", "1")
	fmt.Println(txHash)
	require.NoError(t, err)

	println("check addrDym: ", addrDym.FormattedAddress())
	txHash, err = dymension.GetNode().GrantAuthorization(ctx, dymensionUser.KeyName(), addrDym.FormattedAddress(), "10000adym", "rollappevm_1234-1", rollappIBCDenom, "0.1", "10000dym", "0.1")
	fmt.Println(txHash)
	require.NoError(t, err)

	configFile := "data/config.yaml"
	content, err := os.ReadFile(configFile)
	require.NoError(t, err)

	// Unmarshal the YAML content into the Config struct
	var config Config
	err = yaml.Unmarshal(content, &config)
	require.NoError(t, err)

	dymensionHomeDir := strings.Split(dymension.HomeDir(), "/")
	dymensionFolderName := dymensionHomeDir[len(dymensionHomeDir)-1]

	// Modify a field
	config.NodeAddress = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	config.DBPath = "mongodb://mongodb-container:27017"
	config.Gas.MinimumGasBalance = "100adym"
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.HomeDir = "/root/.eibc-client"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Bots.KeyringBackend = "test"
	config.Bots.KeyringDir = "/root/.eibc-client"
	config.Bots.NumberOfBots = 10
	config.Bots.MaxOrdersPerTx = 10
	config.Bots.TopUpFactor = 5
	config.Whale.AccountName = dymensionUser.KeyName()
	config.Whale.AllowedBalanceThresholds = map[string]string{"adym": "1000", "ibc/278D6FE92E9722572773C899D688907EB9276DEBB40552278B96C17C41C59A11": "1000"}
	config.Whale.KeyringBackend = "test"
	config.Whale.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.FulfillCriteria.MinFeePercentage.Asset = map[string]float32{"adym": 0.1, "ibc/278D6FE92E9722572773C899D688907EB9276DEBB40552278B96C17C41C59A11": 0.1}
	config.FulfillCriteria.MinFeePercentage.Chain = map[string]float32{"rollappevm_1234-1": 0.1}
	config.SkipRefund = true

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0755)
	require.NoError(t, err)

	err = copyFile("data/config.yaml", "/tmp/.eibc-client/config.yaml")
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	err = dymension.Sidecars[0].CreateContainer(ctx)
	require.NoError(t, err)

	err = dymension.Sidecars[0].StartContainer(ctx)
	require.NoError(t, err)
	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a ibc tx from RA -> Hub
	transferData = ibc.WalletData{
		Address: dymensionUserAddr2,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier)

	// set eIBC specific memo
	var options ibc.TransferOptions
	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount).Sub(bigTransferAmount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr2)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr2, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_NoFulfillRollapp_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides1 := make(map[string]any)
	configTomlOverrides1 := make(testutil.Toml)
	configTomlOverrides1["timeout_commit"] = "2s"
	configTomlOverrides1["timeout_propose"] = "2s"
	configTomlOverrides1["index_all_keys"] = "true"
	configTomlOverrides1["mode"] = "validator"

	configFileOverrides1["config/config.toml"] = configTomlOverrides1

	// Create chain factory with dymension
	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "celestia",
		},
	)

	numHubVals := 1
	numHubFullNodes := 1
	numCelestiaFn := 0
	numRollAppFn := 1
	numRollAppVals := 1
	nodeStore := "/home/celestia/light"
	p2pNetwork := "mocha-4"
	coreIp := "mocha-4-consensus.mesa.newmetric.xyz"

	url := "https://api-mocha.celenium.io/v1/block/count"
	headerKey := "User-Agent"
	headerValue := "Apidog/1.0.0 (https://apidog.com)"
	rpcEndpoint := "http://rpc-mocha.pops.one:26657"

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
	}, nil, "", nil, true, 1179360)
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
		Cmd: strslice.StrSlice([]string{"celestia", "light", "start", "--node.store", nodeStore, "--gateway", "--core.ip", coreIp, "--p2p.network", p2pNetwork, "--keyring.keyname", "validator"}), // Replace with your command and arguments
	}

	execIDResp, err := client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		fmt.Println("Err:", err)
	}

	execID := execIDResp.ID

	// Start the exec instance
	execStartCheck := types.ExecStartCheck{
		Tty: false,
	}

	if err := client.ContainerExecStart(ctx, execID, execStartCheck); err != nil {
		fmt.Println("Err:", err)
	}

	err = testutil.WaitForBlocks(ctx, 10, celestia)
	require.NoError(t, err)

	celestia_token, err := celestia.GetNode().GetAuthTokenCelestiaDaLight(ctx, p2pNetwork, nodeStore)
	require.NoError(t, err)
	println("check token: ", celestia_token)
	celestia_namespace_id, err := RandomHex(10)
	require.NoError(t, err)
	println("check namespace: ", celestia_namespace_id)
	da_config := fmt.Sprintf("{\"base_url\": \"http://test-val-0-%s:26658\", \"timeout\": 60000000000, \"gas_prices\":1.0, \"gas_adjustment\": 1.3, \"namespace_id\": \"%s\", \"auth_token\":\"%s\"}", t.Name(), celestia_namespace_id, celestia_token)

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides["namespace_id"] = celestia_namespace_id
	dymintTomlOverrides["da_config"] = da_config
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

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
				ModifyGenesis:       modifyRollappEVMGenesis(modifyEVMGenesisKV),
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "dymension-hub",
			ChainConfig: ibc.ChainConfig{
				Type:                "hub-dym",
				Name:                "dymension",
				ChainID:             "dymension_100-1",
				Images:              []ibc.DockerImage{dymensionImage},
				Bin:                 "dymd",
				Bech32Prefix:        "dym",
				Denom:               "adym",
				CoinType:            "60",
				GasPrices:           "0.0adym",
				EncodingConfig:      encodingConfig(),
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				NoHostMount:         false,
				ModifyGenesis:       modifyDymensionGenesis(dymensionGenesisKV),
				ConfigFileOverrides: nil,
				SidecarConfigs: []ibc.SidecarConfig{
					{
						ProcessName:      "eibc-client",
						Image:            eibcClientImage,
						HomeDir:          "/root",
						Ports:            nil,
						StartCmd:         []string{"eibc-client", "start", "--config", "/root/.eibc-client/config.yaml"},
						PreStart:         true,
						ValidatorProcess: false,
					},
				},
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err = cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	// Relayer Factory
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		})

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil, true, 1179360)
	require.NoError(t, err)

	containerID = fmt.Sprintf("ra-rollappevm_1234-1-val-0-%s", t.Name())

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

	file, err = os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", rollapp1FolderName))
	require.NoError(t, err)
	defer file.Close()

	lines = []string{}
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "namespace_id =") {
			lines[i] = fmt.Sprintf("namespace_id = \"%s\"", celestia_namespace_id)
		} else if strings.HasPrefix(line, "da_config =") {
			lines[i] = fmt.Sprintf("da_config = \"{\\\"base_url\\\": \\\"http://test-val-0-%s:26658\\\", \\\"timeout\\\": 60000000000, \\\"gas_prices\\\":1.0, \\\"gas_adjustment\\\": 1.3, \\\"namespace_id\\\": \\\"%s\\\", \\\"auth_token\\\":\\\"%s\\\"}\"", t.Name(), celestia_namespace_id, celestia_token)
		}
	}

	output = strings.Join(lines, "\n")
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

	valHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

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

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, dymensionUser2, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	dymensionUserAddr2 := dymensionUser2.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  bigTransferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

	txHash, err := dymension.GetNode().CreateGroup(ctx, dymensionUser.KeyName(), "==A", "members.json")
	fmt.Println(txHash)
	require.NoError(t, err)

	txHash, err = dymension.GetNode().CreateGroupPolicy(ctx, dymensionUser.KeyName(), "==A", "policy.json", "1")
	fmt.Println(txHash)
	require.NoError(t, err)

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, dymensionUser.KeyName(), "policyAddr", "10000adym", "rollappevm_1234-1", rollappIBCDenom, "0.1", "10000dym", "0.1")
	fmt.Println(txHash)
	require.NoError(t, err)

	configFile := "data/config.yaml"
	content, err := os.ReadFile(configFile)
	require.NoError(t, err)

	// Unmarshal the YAML content into the Config struct
	var config Config
	err = yaml.Unmarshal(content, &config)
	require.NoError(t, err)

	dymensionHomeDir := strings.Split(dymension.HomeDir(), "/")
	dymensionFolderName := dymensionHomeDir[len(dymensionHomeDir)-1]

	// Modify a field
	config.NodeAddress = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	config.DBPath = "mongodb://mongodb-container:27017"
	config.Gas.MinimumGasBalance = "100adym"
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.HomeDir = "/root/.eibc-client"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Bots.KeyringBackend = "test"
	config.Bots.KeyringDir = "/root/.eibc-client"
	config.Bots.NumberOfBots = 10
	config.Bots.MaxOrdersPerTx = 10
	config.Bots.TopUpFactor = 5
	config.Whale.AccountName = dymensionUser.KeyName()
	config.Whale.AllowedBalanceThresholds = map[string]string{"adym": "1000", "ibc/278D6FE92E9722572773C899D688907EB9276DEBB40552278B96C17C41C59A11": "1000"}
	config.Whale.KeyringBackend = "test"
	config.Whale.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.FulfillCriteria.MinFeePercentage.Asset = map[string]float32{"adym": 0.1, "ibc/278D6FE92E9722572773C899D688907EB9276DEBB40552278B96C17C41C59A11": 0.1}
	config.FulfillCriteria.MinFeePercentage.Chain = map[string]float32{"rollappevm_1234-1": 0.1}
	config.SkipRefund = true

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0755)
	require.NoError(t, err)

	err = copyFile("data/config.yaml", "/tmp/.eibc-client/config.yaml")
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	err = dymension.Sidecars[0].CreateContainer(ctx)
	require.NoError(t, err)

	err = dymension.Sidecars[0].StartContainer(ctx)
	require.NoError(t, err)
	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a ibc tx from RA -> Hub
	transferData = ibc.WalletData{
		Address: dymensionUserAddr2,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	multiplier := math.NewInt(100)

	eibcFee := transferAmount.Quo(multiplier)

	// set eIBC specific memo
	var options ibc.TransferOptions
	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount).Sub(bigTransferAmount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}
