package tests

import (
	"context"
	"fmt"
	"io"
	"os"
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
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

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

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, false, 780)
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

	txhash, err := dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

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
	config.Bots.NumberOfBots = 3
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

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_Got_Polled_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, false, 780)
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

	txhash, err := dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	StartDB(ctx, t, client, network)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

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
	config.Bots.NumberOfBots = 3
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

	err = dymension.Sidecars[0].StartContainer(ctx)
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

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_Lower_Fee_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, false, 780)
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

	txhash, err := dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

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
	config.Bots.NumberOfBots = 3
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

	multiplier := math.NewInt(10000)

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

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_BothRA_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// setup config for rollapp 2
	configFileOverrides2 := make(map[string]any)
	dymintTomlOverrides2 := make(testutil.Toml)
	dymintTomlOverrides2["settlement_layer"] = "dymension"
	dymintTomlOverrides2["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides2["rollapp_id"] = "decentrio_12345-1"
	dymintTomlOverrides2["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides2["max_idle_time"] = "3s"
	dymintTomlOverrides2["max_proof_time"] = "500ms"
	dymintTomlOverrides2["batch_submit_time"] = "50s"
	dymintTomlOverrides2["p2p_blocksync_enabled"] = "false"

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
				Name:                "rollapp-temp",
				ChainID:             "decentrio_12345-1",
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
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r, "relayer").
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: r2,
			Path:    anotherIbcPath,
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
	}, nil, "", nil, false, 780)
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1, rollapp2)

	// Get our Bech32 encoded user addresses
	dymensionUser, dymensionUser2, rollappUser, rollappUser2 := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	dymensionUserAddr2 := dymensionUser2.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()
	rollappUserAddr2 := rollappUser2.FormattedAddress()

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	channel2, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp2.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  bigTransferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.Counterparty.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
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

	txhash, err := dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp2.Config().Denom,
		Amount:  bigTransferAmount,
	}

	_, err = rollapp2.SendIBCTransfer(ctx, channel2.Counterparty.ChannelID, rollappUserAddr2, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err = rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	txhash, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp2.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	rollappTokenDenom2 := transfertypes.GetPrefixedDenom(channel2.Counterparty.PortID, channel2.Counterparty.ChannelID, rollapp2.Config().Denom)
	rollappIBCDenom2 := transfertypes.ParseDenomTrace(rollappTokenDenom2).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom2, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

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
	config.Bots.NumberOfBots = 3
	config.Bots.MaxOrdersPerTx = 10
	config.Bots.TopUpFactor = 5
	config.Whale.AccountName = dymensionUser.KeyName()
	config.Whale.AllowedBalanceThresholds = map[string]string{"adym": "1000", "ibc/278D6FE92E9722572773C899D688907EB9276DEBB40552278B96C17C41C59A11": "1000"}
	config.Whale.KeyringBackend = "test"
	config.Whale.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.FulfillCriteria.MinFeePercentage.Asset = map[string]float32{"adym": 0.1, rollappIBCDenom: 0.1, rollappIBCDenom2: 0.1}
	config.FulfillCriteria.MinFeePercentage.Chain = map[string]float32{rollapp1.Config().ChainID: 0.1, rollapp2.Config().ChainID: 0.1}
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

	_, err = rollapp1.SendIBCTransfer(ctx, channel.Counterparty.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	transferData = ibc.WalletData{
		Address: dymensionUserAddr2,
		Denom:   rollapp2.Config().Denom,
		Amount:  transferAmount,
	}

	_, err = rollapp2.SendIBCTransfer(ctx, channel2.Counterparty.ChannelID, rollappUserAddr2, transferData, options)
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

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr2, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	rollappHeight, err = rollapp2.GetNode().Height(ctx)
	require.NoError(t, err)

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp2.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr2, rollapp2.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_NoDYMForFee_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, false, 780)
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

	txhash, err := dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

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
	config.Gas.Fees = "10000000dym"
	config.LogLevel = "debug"
	config.HomeDir = "/root/.eibc-client"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Bots.KeyringBackend = "test"
	config.Bots.KeyringDir = "/root/.eibc-client"
	config.Bots.NumberOfBots = 3
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

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_Acknowledgement_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, false, 780)
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

	txhash, err := dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

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
	config.Bots.NumberOfBots = 3
	config.Bots.MaxOrdersPerTx = 10
	config.Bots.TopUpFactor = 5
	config.Whale.AccountName = dymensionUser.KeyName()
	config.Whale.AllowedBalanceThresholds = map[string]string{"adym": "1000", "ibc/278D6FE92E9722572773C899D688907EB9276DEBB40552278B96C17C41C59A11": "1000"}
	config.Whale.KeyringBackend = "test"
	config.Whale.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.FulfillCriteria.MinFeePercentage.Asset = map[string]float32{"adym": 0.1, "ibc/278D6FE92E9722572773C899D688907EB9276DEBB40552278B96C17C41C59A11": 0.1}
	config.FulfillCriteria.MinFeePercentage.Chain = map[string]float32{"rollappevm_1234-1": 0.1, dymension.Config().ChainID: 0.1}
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
	// Set a short timeout for IBC transfer
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 1000000, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr2, transferData, options)
	require.NoError(t, err)

	// Get the IBC denom for dymension on roll app
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// According to delayedack module, we need the rollapp to have finalizedHeight > ibcClientLatestHeight
	// in order to trigger ibc timeout or else it will trigger callback

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Assert funds were returned to the sender after the timeout has occured
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, dymension.Config().Denom, walletAmount.SubRaw(1500))
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, zeroBal)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_NoFundForToken_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, false, 780)
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

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

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
	config.Bots.NumberOfBots = 3
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
	transferData := ibc.WalletData{
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

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_NoFeeCriteria_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, false, 780)
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

	txhash, err := dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	StartDB(ctx, t, client, network)

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
	config.Bots.NumberOfBots = 3
	config.Bots.MaxOrdersPerTx = 10
	config.Bots.TopUpFactor = 5
	config.Whale.AccountName = dymensionUser.KeyName()
	config.Whale.AllowedBalanceThresholds = map[string]string{"adym": "1000", "ibc/278D6FE92E9722572773C899D688907EB9276DEBB40552278B96C17C41C59A11": "1000"}
	config.Whale.KeyringBackend = "test"
	config.Whale.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.FulfillCriteria.MinFeePercentage.Asset = map[string]float32{"adym": 0.1}
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

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bridgingFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func Test_EIBC_Client_NoEnoughBalance_EVM(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, false, 780)
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
		Amount:  transferAmount,
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

	txhash, err := dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	fmt.Println(txhash)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bridgingFee))

	StartDB(ctx, t, client, network)

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
	config.Bots.NumberOfBots = 3
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
		Amount:  bigTransferAmount,
	}

	multiplier := math.NewInt(10)

	eibcFee := bigTransferAmount.Quo(multiplier)

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
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount).Sub(transferAmount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	_, err = dymension.GetNode().FinalizePacketsUntilHeight(ctx, dymensionUserAddr, rollapp1.GetChainID(), fmt.Sprint(rollappHeight))
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr2, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}
