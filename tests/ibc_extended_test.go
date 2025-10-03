package tests

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/strslice"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v2"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

const HYP_KEY = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

type TransactionData struct {
	Transactions []struct {
		Hash            string   `json:"hash"`
		TransactionType string   `json:"transactionType"`
		ContractName    string   `json:"contractName"`
		ContractAddress string   `json:"contractAddress"`
		Arguments       []string `json:"arguments"`
	} `json:"transactions"`
}

func TestIBCRAToETH_EVM(t *testing.T) {
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
	dymintTomlOverrides["da_config"] = []string{""}
	dymintTomlOverrides["da_layer"] = []string{"mock"}

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
				SidecarConfigs: []ibc.SidecarConfig{
					{
						ProcessName:      "anvil",
						Image:            anvilImage,
						HomeDir:          "/root",
						Ports:            nil,
						StartCmd:         []string{"anvil", "--port", "8545", "--chain-id", "31337", "--block-time", "1", "--host", "0.0.0.0"},
						PreStart:         true,
						ValidatorProcess: false,
					},
					{
						ProcessName:      "hyperlane",
						Image:            hyperlaneImage,
						ValidatorProcess: false,
					},
					{
						ProcessName:      "rust-relayer",
						Image:            hyperlaneAgentImage,
						ValidatorProcess: false,
					},
				},
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
	}, nil, "", nil, false, 1179360, true)
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

	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	err = rollapp1.Sidecars[0].CreateContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.Sidecars[0].StartContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.Sidecars[1].CreateContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.Sidecars[1].StartContainer(ctx)
	require.NoError(t, err)

	err = copyDir("data/.hyperlane/", "/tmp/.hyperlane/")
	require.NoError(t, err)

	err = copyDir("data/configs/", "/tmp/configs/")
	require.NoError(t, err)

	cmd := []string{
		"hyperlane", "core", "deploy", "--key", HYP_KEY,
		"--yes", "--chain", "anvil0",
	}

	stdout, _, err := rollapp1.Sidecars[1].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	_, err = dymension.GetNode().CreateNoop(ctx, "faucet")
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain := "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/isms", dymension.Validators[0].Name()) + " | jq -r '.isms[0].id'"
	cmd = []string{"sh", "-c", domain}
	ism, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, err = dymension.GetNode().CreateHookNoop(ctx, "faucet")
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain = "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/noop_hooks", dymension.Validators[0].Name()) + " | jq -r '.noop_hooks[0].id'"
	cmd = []string{"sh", "-c", domain}
	noop_hook, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, err = dymension.GetNode().CreateMailBox(ctx, "faucet", strings.TrimRight(string(ism), "\n"), "1260813472")
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain = "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/mailboxes", dymension.Validators[0].Name()) + " | jq -r '.mailboxes[0].id'"
	cmd = []string{"sh", "-c", domain}
	mailbox, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, err = dymension.GetNode().CreateMerkleHook(ctx, "faucet", strings.TrimRight(string(mailbox), "\n"))
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain = "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/merkle_tree_hooks", dymension.Validators[0].Name()) + " | jq -r '.merkle_tree_hooks[0].id'"
	cmd = []string{"sh", "-c", domain}
	merkle_hook, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, err = dymension.GetNode().UpdateMailbox(ctx, "faucet", strings.TrimRight(string(mailbox), "\n"), strings.TrimRight(string(noop_hook), "\n"), strings.TrimRight(string(merkle_hook), "\n"))
	require.NoError(t, err)

	_, err = dymension.GetNode().CreateSyntheticToken(ctx, "faucet", strings.TrimRight(string(mailbox), "\n"))
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain = "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/tokens ", dymension.Validators[0].Name()) + " | jq -r '.tokens[0].id'"
	cmd = []string{"sh", "-c", domain}
	tokenID, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	data := map[string]string{
		"interchainGasPaymaster":   strings.TrimRight("\""+string(noop_hook)+"\"", "\n"),
		"interchainSecurityModule": strings.TrimRight("\""+string(ism)+"\"", "\n"),
		"mailbox":                  strings.TrimRight("\""+string(mailbox)+"\"", "\n"),
		"merkleTreeHook":           strings.TrimRight("\""+string(merkle_hook)+"\"", "\n"),
		"validatorAnnounce":        strings.TrimRight("\""+string(mailbox)+"\"", "\n"),
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}

	err = os.WriteFile("/tmp/.hyperlane/chains/dymension/addresses.yaml", yamlData, 0o644)
	if err != nil {
		fmt.Println("write error:", err)
		return
	}

	containerID := rollapp1.Sidecars[0].Name()

	// Create an exec instance
	execConfig := types.ExecConfig{
		Cmd: strslice.StrSlice([]string{"forge", "script", "script/Foo.s.sol:DeployFoo", "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name()), "--private-key", HYP_KEY, "--broadcast"}),
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

	time.Sleep(20 * time.Second)

	reader, _, err := client.CopyFromContainer(ctx, containerID, "/anvil/broadcast/Foo.s.sol/31337/run-latest.json")
	if err != nil {
		panic(fmt.Errorf("failed to copy from container: %w", err))
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	_, err = tr.Next()
	if err != nil {
		panic(fmt.Errorf("failed to read tar: %w", err))
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, tr); err != nil {
		panic(fmt.Errorf("failed to extract file from tar: %w", err))
	}

	var txData TransactionData

	if err := json.Unmarshal(buf.Bytes(), &txData); err != nil {
		fmt.Println("Raw output:", buf.String())
		panic(err)
	}

	if len(txData.Transactions) > 0 {
		fmt.Println("Contract address:", txData.Transactions[0].ContractAddress)
	} else {
		fmt.Println("No transactions found")
	}

	yamlData1, err := os.ReadFile("/tmp/configs/warp-route-deployment.yaml")
	if err != nil {
		panic(fmt.Errorf("cannot read: %w", err))
	}
	content := string(yamlData1)

	reAnvil := regexp.MustCompile(`(?m)^(\s*anvil0:\n(?:.*\n)*?\s*token:\s*)".*?"`)
	content = reAnvil.ReplaceAllString(content, fmt.Sprintf("${1}\"%s\"", txData.Transactions[0].ContractAddress))

	if err := os.WriteFile("/tmp/configs/warp-route-deployment.yaml", []byte(content), 0o644); err != nil {
		panic(fmt.Errorf("cannot write /tmp/configs/warp-route-deployment.yaml: %w", err))
	}

	cmd = []string{"hyperlane", "warp", "deploy", "--key", HYP_KEY, "--yes"}
	stdout, _, err = rollapp1.Sidecars[1].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	anvil_config, err := os.ReadFile("/tmp/.hyperlane/deployments/warp_routes/FOO/anvil0-config.yaml")
	require.NoError(t, err)

	// Define a struct to match the YAML structure
	var config struct {
		Tokens []struct {
			AddressOrDenom string `yaml:"addressOrDenom"`
		} `yaml:"tokens"`
	}

	// Unmarshal the YAML data
	err = yaml.Unmarshal(anvil_config, &config)
	require.NoError(t, err)

	collateral_token_contract_raw := strings.TrimRight(config.Tokens[0].AddressOrDenom, "\n")
	collateral_token_contract := "0x000000000000000000000000" + strings.TrimPrefix(collateral_token_contract_raw, "0x")

	_, err = dymension.GetNode().EnrollRemoteRouter(ctx, "faucet", strings.TrimRight(string(tokenID), "\n"), "31337", collateral_token_contract)
	require.NoError(t, err)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser1, dymensionUser2, rollappUser1, rollappUser2 := users[0], users[1], users[2], users[3]

	dymensionUser1Addr := dymensionUser1.FormattedAddress()
	_ = rollappUser1.FormattedAddress()
	_ = dymensionUser2.FormattedAddress()
	_ = rollappUser2.FormattedAddress()

	err = rollapp1.Sidecars[2].CreateContainer(ctx)
	require.NoError(t, err)

	cmd = []string{
		"./relayer",
		"--db", "/root/.hyperlane/",
		"--relayChains", "anvil0,dymension",
		"--allowLocalCheckpointSyncers", "true",
		"--defaultSigner.key", HYP_KEY,
		"--metrics-port", "9091",
		"--chains.dymension.signer.type", "cosmosKey",
		"--chains.dymension.signer.prefix", "dym",
		"--chains.dymension.signer.key", HYP_KEY,
		"--log.level", "debug",
	}

	env := []string{
		"CONFIG_FILES=/root/configs/agent-config.json",
	}

	go rollapp1.Sidecars[2].Exec(ctx, cmd, env)

	time.Sleep(20 * time.Second)

	recipient, err := dymension.GetNode().QueryHyperlaneEthRecipient(ctx, dymensionUser1Addr)
	require.NoError(t, err)

	fmt.Println(recipient)

	cmd = []string{"cast", "send", "0x4ed7c70F96B99c776995fB64377f0d4aB3B0e1C1", "approve(address,uint256)", collateral_token_contract_raw, "1000000000000000000", "--private-key", HYP_KEY, "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name())}
	stdout, _, err = rollapp1.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	cmd = []string{
		"cast", "send", collateral_token_contract_raw, "transferRemote(uint32,bytes32,uint256)", "1260813472",
		strings.TrimRight(recipient, "\n"), "5", "--private-key", HYP_KEY, "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name()),
	}

	stdout, _, err = rollapp1.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	cmd = []string{
		"cast", "send", collateral_token_contract_raw, "transferRemoteMemo(uint32,bytes32,uint256,bytes)", "1260813472",
		strings.TrimRight(recipient, "\n"), "5", "0x68656c6c6f", "--private-key", HYP_KEY, "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name()),
	}

	stdout, _, err = rollapp1.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	// CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func TestIBCRAToETH_Wasm(t *testing.T) {
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
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{""}
	dymintTomlOverrides["da_layer"] = []string{"mock"}

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
				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
				ConfigFileOverrides: configFileOverrides,
				SidecarConfigs: []ibc.SidecarConfig{
					{
						ProcessName:      "anvil",
						Image:            anvilImage,
						HomeDir:          "/root",
						Ports:            nil,
						StartCmd:         []string{"anvil", "--port", "8545", "--chain-id", "31337", "--block-time", "1", "--host", "0.0.0.0"},
						PreStart:         true,
						ValidatorProcess: false,
					},
					{
						ProcessName:      "hyperlane",
						Image:            hyperlaneImage,
						ValidatorProcess: false,
					},
					{
						ProcessName:      "rust-relayer",
						Image:            hyperlaneAgentImage,
						ValidatorProcess: false,
					},
				},
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
	}, nil, "", nil, false, 1179360, true)
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

	// CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	err = rollapp1.Sidecars[0].CreateContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.Sidecars[0].StartContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.Sidecars[1].CreateContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.Sidecars[1].StartContainer(ctx)
	require.NoError(t, err)

	err = copyDir("data/.hyperlane/", "/tmp/.hyperlane/")
	require.NoError(t, err)

	err = copyDir("data/configs/", "/tmp/configs/")
	require.NoError(t, err)

	cmd := []string{
		"hyperlane", "core", "deploy", "--key", HYP_KEY,
		"--yes", "--chain", "anvil0",
	}

	stdout, _, err := rollapp1.Sidecars[1].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	_, err = dymension.GetNode().CreateNoop(ctx, "faucet")
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain := "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/isms", dymension.Validators[0].Name()) + " | jq -r '.isms[0].id'"
	cmd = []string{"sh", "-c", domain}
	ism, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, err = dymension.GetNode().CreateHookNoop(ctx, "faucet")
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain = "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/noop_hooks", dymension.Validators[0].Name()) + " | jq -r '.noop_hooks[0].id'"
	cmd = []string{"sh", "-c", domain}
	noop_hook, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, err = dymension.GetNode().CreateMailBox(ctx, "faucet", strings.TrimRight(string(ism), "\n"), "1260813472")
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain = "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/mailboxes", dymension.Validators[0].Name()) + " | jq -r '.mailboxes[0].id'"
	cmd = []string{"sh", "-c", domain}
	mailbox, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, err = dymension.GetNode().CreateMerkleHook(ctx, "faucet", strings.TrimRight(string(mailbox), "\n"))
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain = "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/merkle_tree_hooks", dymension.Validators[0].Name()) + " | jq -r '.merkle_tree_hooks[0].id'"
	cmd = []string{"sh", "-c", domain}
	merkle_hook, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, err = dymension.GetNode().UpdateMailbox(ctx, "faucet", strings.TrimRight(string(mailbox), "\n"), strings.TrimRight(string(noop_hook), "\n"), strings.TrimRight(string(merkle_hook), "\n"))
	require.NoError(t, err)

	_, err = dymension.GetNode().CreateSyntheticToken(ctx, "faucet", strings.TrimRight(string(mailbox), "\n"))
	require.NoError(t, err)

	time.Sleep(10 * time.Second)

	domain = "curl -s " + fmt.Sprintf("http://%s:1317/hyperlane/v1/tokens ", dymension.Validators[0].Name()) + " | jq -r '.tokens[0].id'"
	cmd = []string{"sh", "-c", domain}
	tokenID, _, err := dymension.Validators[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	data := map[string]string{
		"interchainGasPaymaster":   strings.TrimRight("\""+string(noop_hook)+"\"", "\n"),
		"interchainSecurityModule": strings.TrimRight("\""+string(ism)+"\"", "\n"),
		"mailbox":                  strings.TrimRight("\""+string(mailbox)+"\"", "\n"),
		"merkleTreeHook":           strings.TrimRight("\""+string(merkle_hook)+"\"", "\n"),
		"validatorAnnounce":        strings.TrimRight("\""+string(mailbox)+"\"", "\n"),
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}

	err = os.WriteFile("/tmp/.hyperlane/chains/dymension/addresses.yaml", yamlData, 0o644)
	if err != nil {
		fmt.Println("write error:", err)
		return
	}

	containerID := rollapp1.Sidecars[0].Name()

	// Create an exec instance
	execConfig := types.ExecConfig{
		Cmd: strslice.StrSlice([]string{"forge", "script", "script/Foo.s.sol:DeployFoo", "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name()), "--private-key", HYP_KEY, "--broadcast"}),
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

	time.Sleep(20 * time.Second)

	reader, _, err := client.CopyFromContainer(ctx, containerID, "/anvil/broadcast/Foo.s.sol/31337/run-latest.json")
	if err != nil {
		panic(fmt.Errorf("failed to copy from container: %w", err))
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	_, err = tr.Next()
	if err != nil {
		panic(fmt.Errorf("failed to read tar: %w", err))
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, tr); err != nil {
		panic(fmt.Errorf("failed to extract file from tar: %w", err))
	}

	var txData TransactionData

	if err := json.Unmarshal(buf.Bytes(), &txData); err != nil {
		fmt.Println("Raw output:", buf.String())
		panic(err)
	}

	if len(txData.Transactions) > 0 {
		fmt.Println("Contract address:", txData.Transactions[0].ContractAddress)
	} else {
		fmt.Println("No transactions found")
	}

	yamlData1, err := os.ReadFile("/tmp/configs/warp-route-deployment.yaml")
	if err != nil {
		panic(fmt.Errorf("cannot read: %w", err))
	}
	content := string(yamlData1)

	reAnvil := regexp.MustCompile(`(?m)^(\s*anvil0:\n(?:.*\n)*?\s*token:\s*)".*?"`)
	content = reAnvil.ReplaceAllString(content, fmt.Sprintf("${1}\"%s\"", txData.Transactions[0].ContractAddress))

	if err := os.WriteFile("/tmp/configs/warp-route-deployment.yaml", []byte(content), 0o644); err != nil {
		panic(fmt.Errorf("cannot write /tmp/configs/warp-route-deployment.yaml: %w", err))
	}

	cmd = []string{"hyperlane", "warp", "deploy", "--key", HYP_KEY, "--yes"}
	stdout, _, err = rollapp1.Sidecars[1].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	anvil_config, err := os.ReadFile("/tmp/.hyperlane/deployments/warp_routes/FOO/anvil0-config.yaml")
	require.NoError(t, err)

	// Define a struct to match the YAML structure
	var config struct {
		Tokens []struct {
			AddressOrDenom string `yaml:"addressOrDenom"`
		} `yaml:"tokens"`
	}

	// Unmarshal the YAML data
	err = yaml.Unmarshal(anvil_config, &config)
	require.NoError(t, err)

	collateral_token_contract_raw := strings.TrimRight(config.Tokens[0].AddressOrDenom, "\n")
	collateral_token_contract := "0x000000000000000000000000" + strings.TrimPrefix(collateral_token_contract_raw, "0x")

	_, err = dymension.GetNode().EnrollRemoteRouter(ctx, "faucet", strings.TrimRight(string(tokenID), "\n"), "31337", collateral_token_contract)
	require.NoError(t, err)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, rollapp1, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser1, dymensionUser2, rollappUser1, rollappUser2 := users[0], users[1], users[2], users[3]

	dymensionUser1Addr := dymensionUser1.FormattedAddress()
	_ = rollappUser1.FormattedAddress()
	_ = dymensionUser2.FormattedAddress()
	_ = rollappUser2.FormattedAddress()

	err = rollapp1.Sidecars[2].CreateContainer(ctx)
	require.NoError(t, err)

	cmd = []string{
		"./relayer",
		"--db", "/root/.hyperlane/",
		"--relayChains", "anvil0,dymension",
		"--allowLocalCheckpointSyncers", "true",
		"--defaultSigner.key", HYP_KEY,
		"--metrics-port", "9091",
		"--chains.dymension.signer.type", "cosmosKey",
		"--chains.dymension.signer.prefix", "dym",
		"--chains.dymension.signer.key", HYP_KEY,
		"--log.level", "debug",
	}

	env := []string{
		"CONFIG_FILES=/root/configs/agent-config.json",
	}

	go rollapp1.Sidecars[2].Exec(ctx, cmd, env)

	time.Sleep(20 * time.Second)

	recipient, err := dymension.GetNode().QueryHyperlaneEthRecipient(ctx, dymensionUser1Addr)
	require.NoError(t, err)

	fmt.Println(recipient)

	cmd = []string{"cast", "send", "0x4ed7c70F96B99c776995fB64377f0d4aB3B0e1C1", "approve(address,uint256)", collateral_token_contract_raw, "1000000000000000000", "--private-key", HYP_KEY, "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name())}
	stdout, _, err = rollapp1.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	cmd = []string{
		"cast", "send", collateral_token_contract_raw, "transferRemote(uint32,bytes32,uint256)", "1260813472",
		strings.TrimRight(recipient, "\n"), "5", "--private-key", HYP_KEY, "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name()),
	}

	stdout, _, err = rollapp1.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	cmd = []string{
		"cast", "send", collateral_token_contract_raw, "transferRemoteMemo(uint32,bytes32,uint256,bytes)", "1260813472",
		strings.TrimRight(recipient, "\n"), "5", "0x68656c6c6f", "--private-key", HYP_KEY, "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name()),
	}

	stdout, _, err = rollapp1.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	// CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		} else {
			srcFile, err := os.Open(path)
			if err != nil {
				return err
			}
			defer srcFile.Close()

			dstFile, err := os.Create(dstPath)
			if err != nil {
				return err
			}
			defer dstFile.Close()

			_, err = io.Copy(dstFile, srcFile)
			if err != nil {
				return err
			}

			return os.Chmod(dstPath, info.Mode())
		}
	})
}
