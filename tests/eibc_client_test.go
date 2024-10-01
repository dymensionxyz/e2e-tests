package tests

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

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
	time.Sleep(1 * time.Minute)
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
						StartCmd:         []string{"eibc-client", "init"},
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

	cmd := append([]string{"eibc-client"}, "start", "--config", "/root/.eibc-client/config.yaml")

	StartDB(ctx, t, client, network)
	err = dymension.Sidecars[0].CreateContainer(ctx)
	require.NoError(t, err)

	err = dymension.Sidecars[0].StartContainer(ctx)
	require.NoError(t, err)
	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)
	configFile := "data/config.yaml"
	content, err := os.ReadFile(configFile)
	require.NoError(t, err)

	// Unmarshal the YAML content into the Config struct
	var config Config
	err = yaml.Unmarshal(content, &config)
	require.NoError(t, err)

	// Modify a field
	config.NodeAddress = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	config.DBPath = "mongodb://mongodb-container:27017"
	config.Gas.MinimumGasBalance = "1000000000000000000adym"
	config.LogLevel = "info"
	config.HomeDir = "/root/.eibc-client"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Bots.KeyringBackend = "test"
	config.Bots.KeyringDir = "/root/.eibc-client"
	config.Bots.NumberOfBots = 30
	config.Bots.MaxOrdersPerTx = 10
	config.Bots.TopUpFactor = 5
	config.Whale.AccountName = ""
	config.Whale.AllowedBalanceThresholds = map[string]string{"adym": "1000000000000"}
	config.Whale.KeyringBackend = "test"
	config.Whale.KeyringDir = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0777)
	require.NoError(t, err)

	err = copyFile("data/config.yaml", "/tmp/.eibc-client/config.yaml")
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	_, _, err = dymension.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

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
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferAmount.Sub(bridgingFee))

	// Get original account balances
	dymensionOrigBal, err := dymension.GetBalance(ctx, dymensionUserAddr, dymension.Config().Denom)
	require.NoError(t, err)

	// Compose an IBC transfer and send from dymension -> rollapp
	transferData = ibc.WalletData{
		Address: rollappUserAddr,
		Denom:   dymension.Config().Denom,
		Amount:  transferAmount,
	}

	// Compose an IBC transfer and send from Hub -> rollapp
	_, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom
	dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, dymensionOrigBal.Sub(transferData.Amount))
	erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	require.NoError(t, err)
	erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	testutil.AssertBalance(t, ctx, rollapp1, erc20MAccAddr, dymensionIBCDenom, transferData.Amount)

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}
