package tests

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/ignite/cli/ignite/pkg/cosmosaccount"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
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

type ValidationLevel string

type Config struct {
	NodeAddress  string                   `yaml:"node_address"`
	Gas          GasConfig                `yaml:"gas"`
	OrderPolling OrderPollingConfig       `yaml:"order_polling"`
	Rollapps     map[string]RollappConfig `yaml:"rollapps"`

	Operator   OperatorConfig   `yaml:"operator"`
	Fulfillers FulfillerConfig  `yaml:"fulfillers"`
	Validation ValidationConfig `yaml:"validation"`
	Slack      SlackConfig      `yaml:"slack"`

	LogLevel string `yaml:"log_level"`
}

type OrderPollingConfig struct {
	IndexerURL string        `yaml:"indexer_url"`
	Interval   time.Duration `yaml:"interval"`
	Enabled    bool          `yaml:"enabled"`
}

type GasConfig struct {
	Prices string `yaml:"prices"`
	Fees   string `yaml:"fees"`
}

type FulfillerConfig struct {
	Scale           int                          `yaml:"scale"`
	OperatorAddress string                       `yaml:"operator_address"`
	PolicyAddress   string                       `yaml:"policy_address"`
	KeyringBackend  cosmosaccount.KeyringBackend `yaml:"keyring_backend"`
	KeyringDir      string                       `yaml:"keyring_dir"`
	BatchSize       int                          `yaml:"batch_size"`
	MaxOrdersPerTx  int                          `yaml:"max_orders_per_tx"`
}

type OperatorConfig struct {
	AccountName    string                       `yaml:"account_name"`
	KeyringBackend cosmosaccount.KeyringBackend `yaml:"keyring_backend"`
	KeyringDir     string                       `yaml:"keyring_dir"`
	GroupID        int                          `yaml:"group_id"`
	MinFeeShare    string                       `yaml:"min_fee_share"`
}

type ValidationConfig struct {
	FallbackLevel ValidationLevel `yaml:"fallback_level"`
	WaitTime      time.Duration   `yaml:"wait_time"`
	Interval      time.Duration   `yaml:"interval"`
}

type RollappConfig struct {
	FullNodes        []string `yaml:"full_nodes"`
	MinConfirmations int      `yaml:"min_confirmations"`
}

type SlackConfig struct {
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
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, lp2, rollappUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	// lp2Addr := lp2.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: lp1Addr,
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

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, lp1Addr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, lp1Addr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	// Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// // register ibc denom on rollapp1
	// metadata := banktypes.Metadata{
	// 	Description: "IBC token from Dymension",
	// 	DenomUnits: []*banktypes.DenomUnit{
	// 		{
	// 			Denom:    dymensionIBCDenom,
	// 			Exponent: 0,
	// 			Aliases:  []string{"udym"},
	// 		},
	// 		{
	// 			Denom:    "udym",
	// 			Exponent: 6,
	// 		},
	// 	},
	// 	// Setting base as IBC hash denom since bank keepers's SetDenomMetadata uses
	// 	// Base as key path and the IBC hash is what gives this token uniqueness
	// 	// on the executing chain
	// 	Base:    dymensionIBCDenom,
	// 	Display: "udym",
	// 	Name:    "udym",
	// 	Symbol:  "udym",
	// }

	// data := map[string][]banktypes.Metadata{
	// 	"metadata": {metadata},
	// }

	// contentFile, err := json.Marshal(data)
	// require.NoError(t, err)
	// rollapp1.GetNode().WriteFile(ctx, contentFile, "./ibcmetadata.json")
	// deposit := "500000000000" + rollapp1.Config().Denom
	// rollapp1.GetNode().HostName()
	// _, err = rollapp1.GetNode().RegisterIBCTokenDenomProposal(ctx, rollappUser.KeyName(), deposit, rollapp1.GetNode().HomeDir()+"/ibcmetadata.json")
	// require.NoError(t, err)

	// err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	// require.NoError(t, err, "failed to submit votes")

	// height, err := rollapp1.Height(ctx)
	// require.NoError(t, err, "error fetching height")
	// _, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	// require.NoError(t, err, "proposal status did not change to passed")

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount.Mul(math.NewInt(5)),
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check fund was set to erc20 module account on rollapp
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// rollappErc20MaccBalance, err := rollapp1.GetBalance(ctx, erc20MAccAddr, dymensionIBCDenom)
	// require.NoError(t, err)

	// require.True(t, rollappErc20MaccBalance.Equal(transferData.Amount))
	// require.NoError(t, err)

	// tokenPair, err := rollapp1.GetNode().QueryErc20TokenPair(ctx, dymensionIBCDenom)
	// require.NoError(t, err)
	// require.NotNil(t, tokenPair)

	// // convert erc20
	// _, err = rollapp1.GetNode().ConvertErc20(ctx, rollappUser.KeyName(), tokenPair.Erc20Address, transferData.Amount.String(), rollappUserAddr, rollappUserAddr, rollapp1.Config().ChainID)
	// require.NoError(t, err, "can not convert erc20 to cosmos coin")

	// err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	// require.NoError(t, err)
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferData.Amount)

	// StartDB(ctx, t, client, network)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+rollappIBCDenom, "rollappevm_1234-1", rollappIBCDenom, "0.1", "1000000"+rollappIBCDenom, "0.1", true)
	fmt.Println(txHash)
	require.NoError(t, err)

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp2.KeyName(), policyAddr, "10000"+rollappIBCDenom, "rollappevm_1234-1", rollappIBCDenom, "0.1", "10000"+rollappIBCDenom, "0.1", true)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.1"
	config.Rollapps = map[string]RollappConfig{
		"rollappevm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappevm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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
		Address: dymensionUserAddr,
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
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_Success_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, lp2, rollappUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	// lp2Addr := lp2.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: lp1Addr,
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

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, lp1Addr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, lp1Addr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	// Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// // register ibc denom on rollapp1
	// metadata := banktypes.Metadata{
	// 	Description: "IBC token from Dymension",
	// 	DenomUnits: []*banktypes.DenomUnit{
	// 		{
	// 			Denom:    dymensionIBCDenom,
	// 			Exponent: 0,
	// 			Aliases:  []string{"udym"},
	// 		},
	// 		{
	// 			Denom:    "udym",
	// 			Exponent: 6,
	// 		},
	// 	},
	// 	// Setting base as IBC hash denom since bank keepers's SetDenomMetadata uses
	// 	// Base as key path and the IBC hash is what gives this token uniqueness
	// 	// on the executing chain
	// 	Base:    dymensionIBCDenom,
	// 	Display: "udym",
	// 	Name:    "udym",
	// 	Symbol:  "udym",
	// }

	// data := map[string][]banktypes.Metadata{
	// 	"metadata": {metadata},
	// }

	// contentFile, err := json.Marshal(data)
	// require.NoError(t, err)
	// rollapp1.GetNode().WriteFile(ctx, contentFile, "./ibcmetadata.json")
	// deposit := "500000000000" + rollapp1.Config().Denom
	// rollapp1.GetNode().HostName()
	// _, err = rollapp1.GetNode().RegisterIBCTokenDenomProposal(ctx, rollappUser.KeyName(), deposit, rollapp1.GetNode().HomeDir()+"/ibcmetadata.json")
	// require.NoError(t, err)

	// err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	// require.NoError(t, err, "failed to submit votes")

	// height, err := rollapp1.Height(ctx)
	// require.NoError(t, err, "error fetching height")
	// _, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	// require.NoError(t, err, "proposal status did not change to passed")

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount.Mul(math.NewInt(5)),
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check fund was set to erc20 module account on rollapp
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// rollappErc20MaccBalance, err := rollapp1.GetBalance(ctx, erc20MAccAddr, dymensionIBCDenom)
	// require.NoError(t, err)

	// require.True(t, rollappErc20MaccBalance.Equal(transferData.Amount))
	// require.NoError(t, err)

	// tokenPair, err := rollapp1.GetNode().QueryErc20TokenPair(ctx, dymensionIBCDenom)
	// require.NoError(t, err)
	// require.NotNil(t, tokenPair)

	// // convert erc20
	// _, err = rollapp1.GetNode().ConvertErc20(ctx, rollappUser.KeyName(), tokenPair.Erc20Address, transferData.Amount.String(), rollappUserAddr, rollappUserAddr, rollapp1.Config().ChainID)
	// require.NoError(t, err, "can not convert erc20 to cosmos coin")

	// err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	// require.NoError(t, err)
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferData.Amount)

	// StartDB(ctx, t, client, network)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+rollappIBCDenom, "rollappwasm_1234-1", rollappIBCDenom, "0.1", "1000000"+rollappIBCDenom, "0.1", true)
	fmt.Println(txHash)
	require.NoError(t, err)

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp2.KeyName(), policyAddr, "10000"+rollappIBCDenom, "rollappwasm_1234-1", rollappIBCDenom, "0.1", "10000"+rollappIBCDenom, "0.1", true)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.1"
	config.Rollapps = map[string]RollappConfig{
		"rollappwasm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappwasm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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
		Address: dymensionUserAddr,
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
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, lp2, rollappUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	// lp2Addr := lp2.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: lp1Addr,
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

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, lp1Addr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, lp1Addr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	// Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// // register ibc denom on rollapp1
	// metadata := banktypes.Metadata{
	// 	Description: "IBC token from Dymension",
	// 	DenomUnits: []*banktypes.DenomUnit{
	// 		{
	// 			Denom:    dymensionIBCDenom,
	// 			Exponent: 0,
	// 			Aliases:  []string{"udym"},
	// 		},
	// 		{
	// 			Denom:    "udym",
	// 			Exponent: 6,
	// 		},
	// 	},
	// 	// Setting base as IBC hash denom since bank keepers's SetDenomMetadata uses
	// 	// Base as key path and the IBC hash is what gives this token uniqueness
	// 	// on the executing chain
	// 	Base:    dymensionIBCDenom,
	// 	Display: "udym",
	// 	Name:    "udym",
	// 	Symbol:  "udym",
	// }

	// data := map[string][]banktypes.Metadata{
	// 	"metadata": {metadata},
	// }

	// contentFile, err := json.Marshal(data)
	// require.NoError(t, err)
	// rollapp1.GetNode().WriteFile(ctx, contentFile, "./ibcmetadata.json")
	// deposit := "500000000000" + rollapp1.Config().Denom
	// rollapp1.GetNode().HostName()
	// _, err = rollapp1.GetNode().RegisterIBCTokenDenomProposal(ctx, rollappUser.KeyName(), deposit, rollapp1.GetNode().HomeDir()+"/ibcmetadata.json")
	// require.NoError(t, err)

	// err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	// require.NoError(t, err, "failed to submit votes")

	// height, err := rollapp1.Height(ctx)
	// require.NoError(t, err, "error fetching height")
	// _, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	// require.NoError(t, err, "proposal status did not change to passed")

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount.Mul(math.NewInt(5)),
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check fund was set to erc20 module account on rollapp
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// rollappErc20MaccBalance, err := rollapp1.GetBalance(ctx, erc20MAccAddr, dymensionIBCDenom)
	// require.NoError(t, err)

	// require.True(t, rollappErc20MaccBalance.Equal(transferData.Amount))
	// require.NoError(t, err)

	// tokenPair, err := rollapp1.GetNode().QueryErc20TokenPair(ctx, dymensionIBCDenom)
	// require.NoError(t, err)
	// require.NotNil(t, tokenPair)

	// // convert erc20
	// _, err = rollapp1.GetNode().ConvertErc20(ctx, rollappUser.KeyName(), tokenPair.Erc20Address, transferData.Amount.String(), rollappUserAddr, rollappUserAddr, rollapp1.Config().ChainID)
	// require.NoError(t, err, "can not convert erc20 to cosmos coin")

	// err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	// require.NoError(t, err)
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferData.Amount)

	// StartDB(ctx, t, client, network)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+"adym", "rollappevm_1234-1", "adym", "0.1", "1000000"+"adym", "0.1", true)
	fmt.Println(txHash)
	require.NoError(t, err)

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp2.KeyName(), policyAddr, "10000"+rollappIBCDenom, "rollappevm_1234-1", rollappIBCDenom, "0.1", "10000"+rollappIBCDenom, "0.1", true)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.1"
	config.Rollapps = map[string]RollappConfig{
		"rollappevm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappevm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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
		Address: dymensionUserAddr,
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
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bridgingFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_NoFulfillRollapp_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, lp2, rollappUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	// lp2Addr := lp2.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: lp1Addr,
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

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, lp1Addr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, lp1Addr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	// Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// // register ibc denom on rollapp1
	// metadata := banktypes.Metadata{
	// 	Description: "IBC token from Dymension",
	// 	DenomUnits: []*banktypes.DenomUnit{
	// 		{
	// 			Denom:    dymensionIBCDenom,
	// 			Exponent: 0,
	// 			Aliases:  []string{"udym"},
	// 		},
	// 		{
	// 			Denom:    "udym",
	// 			Exponent: 6,
	// 		},
	// 	},
	// 	// Setting base as IBC hash denom since bank keepers's SetDenomMetadata uses
	// 	// Base as key path and the IBC hash is what gives this token uniqueness
	// 	// on the executing chain
	// 	Base:    dymensionIBCDenom,
	// 	Display: "udym",
	// 	Name:    "udym",
	// 	Symbol:  "udym",
	// }

	// data := map[string][]banktypes.Metadata{
	// 	"metadata": {metadata},
	// }

	// contentFile, err := json.Marshal(data)
	// require.NoError(t, err)
	// rollapp1.GetNode().WriteFile(ctx, contentFile, "./ibcmetadata.json")
	// deposit := "500000000000" + rollapp1.Config().Denom
	// rollapp1.GetNode().HostName()
	// _, err = rollapp1.GetNode().RegisterIBCTokenDenomProposal(ctx, rollappUser.KeyName(), deposit, rollapp1.GetNode().HomeDir()+"/ibcmetadata.json")
	// require.NoError(t, err)

	// err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	// require.NoError(t, err, "failed to submit votes")

	// height, err := rollapp1.Height(ctx)
	// require.NoError(t, err, "error fetching height")
	// _, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	// require.NoError(t, err, "proposal status did not change to passed")

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount.Mul(math.NewInt(5)),
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check fund was set to erc20 module account on rollapp
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// rollappErc20MaccBalance, err := rollapp1.GetBalance(ctx, erc20MAccAddr, dymensionIBCDenom)
	// require.NoError(t, err)

	// require.True(t, rollappErc20MaccBalance.Equal(transferData.Amount))
	// require.NoError(t, err)

	// tokenPair, err := rollapp1.GetNode().QueryErc20TokenPair(ctx, dymensionIBCDenom)
	// require.NoError(t, err)
	// require.NotNil(t, tokenPair)

	// // convert erc20
	// _, err = rollapp1.GetNode().ConvertErc20(ctx, rollappUser.KeyName(), tokenPair.Erc20Address, transferData.Amount.String(), rollappUserAddr, rollappUserAddr, rollapp1.Config().ChainID)
	// require.NoError(t, err, "can not convert erc20 to cosmos coin")

	// err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	// require.NoError(t, err)
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferData.Amount)

	// StartDB(ctx, t, client, network)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+"adym", "rollappwasm_1234-1", "adym", "0.1", "1000000"+"adym", "0.1", true)
	fmt.Println(txHash)
	require.NoError(t, err)

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp2.KeyName(), policyAddr, "10000"+rollappIBCDenom, "rollappwasm_1234-1", rollappIBCDenom, "0.1", "10000"+rollappIBCDenom, "0.1", true)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.1"
	config.Rollapps = map[string]RollappConfig{
		"rollappwasm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappwasm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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
		Address: dymensionUserAddr,
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
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bridgingFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_Timeout_EVM(t *testing.T) {
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
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
		cosmos.GenesisKV{
			Key:   "app_state.erc20.params.enable_erc20",
			Value: false,
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+dymension.Config().Denom, "rollappevm_1234-1", dymension.Config().Denom, "0.0001", "1000000"+dymension.Config().Denom, "0.0001", false)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.0001"
	config.Rollapps = map[string]RollappConfig{
		"rollappevm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappevm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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

	// Send a ibc tx from Hub -> RA
	transferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	var options ibc.TransferOptions

	// Set a short timeout for IBC transfer
	options = ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 1000000, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

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

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.SubRaw(1500))
	// upon timeout error eibc should be created, and eibc should fulfill it
	// lp should get his funds after claiming the finalized tx
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, dymension.Config().Denom, walletAmount.AddRaw(1500))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_Timeout_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+dymension.Config().Denom, "rollappwasm_1234-1", dymension.Config().Denom, "0.0001", "1000000"+dymension.Config().Denom, "0.0001", false)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.0001"
	config.Rollapps = map[string]RollappConfig{
		"rollappwasm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappwasm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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

	// Send a ibc tx from Hub -> RA
	transferData := ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// set eIBC specific memo
	var options ibc.TransferOptions

	// Set a short timeout for IBC transfer
	options = ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 1000000, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, options)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

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

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.SubRaw(1500))
	// upon timeout error eibc should be created, and eibc should fulfill it
	// lp should get his funds after claiming the finalized tx
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, dymension.Config().Denom, walletAmount.AddRaw(1500))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_AckErr_EVM(t *testing.T) {
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
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "grpc",
		},
		cosmos.GenesisKV{
			Key:   "app_state.erc20.params.enable_erc20",
			Value: false,
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, _ := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+dymension.Config().Denom, "rollappevm_1234-1", dymension.Config().Denom, "0.0001", "1000000"+dymension.Config().Denom, "0.0001", false)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.0001"
	config.Rollapps = map[string]RollappConfig{
		"rollappevm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappevm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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

	// Send a ibc tx from Hub -> RA
	transferData := ibc.WalletData{
		Address: "rollappUserAddr",
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	dymHeight, err := dymension.GetNode().Height(ctx)
	require.NoError(t, err)

	ibcTx, err := dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	ack, err := testutil.PollForAck(ctx, dymension, dymHeight, dymHeight+30, ibcTx.Packet)
	require.NoError(t, err)

	fmt.Println("ack:", ack.Acknowledgement)

	// Make sure that the ack contains error
	require.True(t, bytes.Contains(ack.Acknowledgement, []byte("error")))

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

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

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.SubRaw(1500))
	// upon timeout error eibc should be created, and eibc should fulfill it
	// lp should get his funds after claiming the finalized tx
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, dymension.Config().Denom, walletAmount.AddRaw(1500))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_AckErr_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, _ := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+dymension.Config().Denom, "rollappwasm_1234-1", dymension.Config().Denom, "0.0001", "1000000"+dymension.Config().Denom, "0.0001", false)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.0001"
	config.Rollapps = map[string]RollappConfig{
		"rollappwasm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappwasm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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

	// Send a ibc tx from Hub -> RA
	transferData := ibc.WalletData{
		Address: "rollappUserAddr",
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	dymHeight, err := dymension.GetNode().Height(ctx)
	require.NoError(t, err)

	ibcTx, err := dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	ack, err := testutil.PollForAck(ctx, dymension, dymHeight, dymHeight+30, ibcTx.Packet)
	require.NoError(t, err)

	fmt.Println("ack:", ack.Acknowledgement)

	// Make sure that the ack contains error
	require.True(t, bytes.Contains(ack.Acknowledgement, []byte("error")))

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

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

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.SubRaw(1500))
	// upon timeout error eibc should be created, and eibc should fulfill it
	// lp should get his funds after claiming the finalized tx
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, dymension.Config().Denom, walletAmount.AddRaw(1500))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_Update_Order_EVM(t *testing.T) {
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
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: lp1Addr,
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

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, lp1Addr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, lp1Addr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+rollappIBCDenom, "rollappevm_1234-1", rollappIBCDenom, "0.1", "1000000"+rollappIBCDenom, "0.1", false)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.1"
	config.Rollapps = map[string]RollappConfig{
		"rollappevm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappevm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	multiplier := math.NewInt(1000)

	eibcFee := transferAmount.Quo(multiplier)

	// set eIBC specific memo
	var options ibc.TransferOptions
	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents[0])

	multiplier = math.NewInt(10)

	newEibcFee := transferAmount.Quo(multiplier)

	_, err = dymension.UpdateDemandOrder(ctx, eibcEvents[0].OrderId, dymensionUserAddr, newEibcFee)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

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
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(newEibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_Update_Order_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{"{\"host\":\"grpc-da-container\",\"port\": 7980}"}
	dymintTomlOverrides["da_layer"] = []string{"grpc"}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
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

	testutil.WaitForBlocks(ctx, 2, dymension)

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

	// Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, rollappUser := users[0], users[1], users[2]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{
		"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{
		dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: lp1Addr,
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

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, lp1Addr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, lp1Addr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
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

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0o755)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{
		"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+rollappIBCDenom, "rollappwasm_1234-1", rollappIBCDenom, "0.1", "1000000"+rollappIBCDenom, "0.1", false)
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
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.1"
	config.Rollapps = map[string]RollappConfig{
		"rollappwasm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappwasm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0o777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0o777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0o755)
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
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	multiplier := math.NewInt(1000)

	eibcFee := transferAmount.Quo(multiplier)

	// set eIBC specific memo
	var options ibc.TransferOptions
	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	fmt.Println("Event:", eibcEvents[0])

	multiplier = math.NewInt(10)

	newEibcFee := transferAmount.Quo(multiplier)

	_, err = dymension.UpdateDemandOrder(ctx, eibcEvents[0].OrderId, dymensionUserAddr, newEibcFee)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

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
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(newEibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}
