package tests

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

const HYP_KEY = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
const REMOTE_ROUTER_ADDRESS = "0x0000000000000000000000000000000000000000000000000000000000000000"

type TransactionData struct {
	Transactions []struct {
		Hash            string   `json:"hash"`
		TransactionType string   `json:"transactionType"`
		ContractName    string   `json:"contractName"`
		ContractAddress string   `json:"contractAddress"`
		Arguments       []string `json:"arguments"`
	} `json:"transactions"`
}

type KaspaUtxo struct {
	Address  string `json:"address"`
	Outpoint struct {
		TransactionId string `json:"transactionId"`
		Index         int    `json:"index"`
	} `json:"outpoint"`
	UtxoEntry struct {
		Amount          string `json:"amount"`
		ScriptPublicKey struct {
			ScriptPublicKey string `json:"scriptPublicKey"`
		} `json:"scriptPublicKey"`
		BlockDaaScore string `json:"blockDaaScore"`
		IsCoinbase    bool   `json:"isCoinbase"`
	} `json:"utxoEntry"`
}

func GetKaspaUtxos(address string) ([]KaspaUtxo, error) {
	url := "https://api-tn10.kaspa.org/addresses/" + address + "/utxos"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Kaspa API error: %s", string(body))
	}

	var utxos []KaspaUtxo
	err = json.Unmarshal(body, &utxos)
	if err != nil {
		return nil, err
	}
	return utxos, nil
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

	//Update white listed relayers
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

	cmd := []string{"hyperlane", "core", "deploy", "--key", HYP_KEY,
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

	err = os.WriteFile("/tmp/.hyperlane/chains/dymension/addresses.yaml", yamlData, 0644)
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

	if err := os.WriteFile("/tmp/configs/warp-route-deployment.yaml", []byte(content), 0644); err != nil {
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

	cmd = []string{"./relayer",
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

	cmd = []string{"cast", "send", collateral_token_contract_raw, "transferRemote(uint32,bytes32,uint256)", "1260813472",
		strings.TrimRight(recipient, "\n"), "5", "--private-key", HYP_KEY, "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name()),
	}

	stdout, _, err = rollapp1.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	cmd = []string{"cast", "send", collateral_token_contract_raw, "transferRemoteMemo(uint32,bytes32,uint256,bytes)", "1260813472",
		strings.TrimRight(recipient, "\n"), "5", "0x68656c6c6f", "--private-key", HYP_KEY, "--rpc-url", fmt.Sprintf("http://%s:8545", rollapp1.Sidecars[0].Name()),
	}

	stdout, _, err = rollapp1.Sidecars[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	fmt.Println(string(stdout))

	// CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}

type ValidatorInfo struct {
	ValidatorISMAddr      string `json:"validator_ism_addr"`
	ValidatorISMPrivKey   string `json:"validator_ism_priv_key"`
	ValidatorEscrowSecret string `json:"validator_escrow_secret"`
	ValidatorEscrowPubKey string `json:"validator_escrow_pub_key"`
	MultisigEscrowAddr    string `json:"multisig_escrow_addr"`
}

type Token struct {
	ID string `json:"id"`
}

type TokenResponse struct {
	Tokens []Token `json:"tokens"`
}

type Ism struct {
	ID string `json:"id"`
}

type IsmResponse struct {
	Isms []Ism `json:"isms"`
}

func TestIBCHubToKaspa_EVM(t *testing.T) {
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
						ProcessName:      "hyperlane",
						Image:            hyperlaneImage,
						ValidatorProcess: false,
					},
					{
						ProcessName:      "rust-relayer",
						Image:            hyperlaneAgentKaspaImage,
						ValidatorProcess: false,
					},
					{
						ProcessName:      "kaspa",
						Image:            validatorImage,
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

	//Update white listed relayers
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

	// err = rollapp1.Sidecars[1].CreateContainer(ctx)
	// require.NoError(t, err)

	// err = rollapp1.Sidecars[1].StartContainer(ctx)
	// require.NoError(t, err)

	err = rollapp1.Sidecars[2].CreateContainer(ctx)
	require.NoError(t, err)

	// err = rollapp1.Sidecars[2].StartContainer(ctx)
	// require.NoError(t, err)

	cmd := []string{"cargo", "run", "validator-with-escrow"}

	stdout, _, err := rollapp1.Sidecars[2].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	// Extract JSON from stdout
	jsonStart := bytes.IndexByte(stdout, '{')
	jsonEnd := bytes.LastIndexByte(stdout, '}')
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd < jsonStart {
		t.Fatalf("Could not find JSON object in stdout: %s", string(stdout))
	}
	jsonBytes := stdout[jsonStart : jsonEnd+1]

	var info ValidatorInfo
	err = json.Unmarshal(jsonBytes, &info)
	require.NoError(t, err)
	fmt.Println(info.ValidatorISMAddr)
	fmt.Println(info.ValidatorISMPrivKey)
	fmt.Println(info.ValidatorEscrowSecret)
	fmt.Println(info.ValidatorEscrowPubKey)
	fmt.Println(info.MultisigEscrowAddr)

	// Always trim quotes from ismAddr
	ismAddr := strings.Trim(info.ValidatorISMAddr, "\"")

	err = copyDir("data/kaspa/.kaspa/", "/tmp/.kaspa/")
	require.NoError(t, err)

	cmd = []string{"cargo", "run", "--", "deposit",
		"--escrow-address", info.MultisigEscrowAddr,
		"--amount", "100000000",
		"--wrpc-url", "185.69.54.99:17210",
		"--network-id", "testnet-10",
		"--wallet-secret", "lkjsdf"}

	_, _, err = rollapp1.Sidecars[2].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	_, _ = dymension.GetNode().SetupKaspaBridge(ctx, "faucet", ismAddr, REMOTE_ROUTER_ADDRESS)

	tokenIDs, err := dymension.GetNode().QueryTokenID(ctx)
	require.NoError(t, err)

	var resp TokenResponse
	err = json.Unmarshal([]byte(tokenIDs), &resp)
	require.NoError(t, err)

	fmt.Println(tokenIDs)
	tokenID := resp.Tokens[0].ID

	mailboxes, err := dymension.GetNode().QueryMailboxes(ctx, ismAddr, REMOTE_ROUTER_ADDRESS)
	require.NoError(t, err)

	fmt.Println(mailboxes)

	var mailboxID string
	type mailboxList struct {
		Mailboxes []struct {
			ID string `json:"id"`
		} `json:"mailboxes"`
	}
	var mailboxObj mailboxList
	err = json.Unmarshal([]byte(mailboxes), &mailboxObj)
	require.NoError(t, err)
	if len(mailboxObj.Mailboxes) > 0 {
		mailboxID = mailboxObj.Mailboxes[0].ID
	}
	if mailboxID != "" {
		configPath := "data/kaspa/configs/agent-config.json"
		configData, err := os.ReadFile(configPath)
		require.NoError(t, err)
		var config map[string]interface{}
		err = json.Unmarshal(configData, &config)
		require.NoError(t, err)
		chains, ok := config["chains"].(map[string]interface{})
		require.True(t, ok)
		kaspatest, ok := chains["kaspatest10"].(map[string]interface{})
		require.True(t, ok)
		kaspatest["hubMailboxId"] = mailboxID
		kaspatest["validatorPubsKaspa"] = info.ValidatorEscrowPubKey
		kaspatest["escrowAddress"] = info.MultisigEscrowAddr
		kaspatest["kaspaEscrowPrivateKey"] = info.ValidatorEscrowSecret
		kaspatest["hubTokenId"] = tokenID
		newConfigData, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)
		err = os.WriteFile(configPath, newConfigData, 0644)
		require.NoError(t, err)
		fmt.Println("Updated hubMailboxId in agent-config.json:", mailboxID)
	}

	err = os.Mkdir("/tmp/dbs", 0777)
	require.NoError(t, err)

	err = copyDir("data/kaspa/configs/", "/tmp/configs/")
	require.NoError(t, err)

	err = rollapp1.Sidecars[1].CreateContainer(ctx)
	require.NoError(t, err)

	// err = rollapp1.Sidecars[0].StartContainer(ctx)
	// require.NoError(t, err)

	cmd = []string{"./validator",
		"--db", "/root/dbs/hyperlane_db_validator",
		"--originChainName", "kaspatest10",
		"--reorgPeriod", "1",
		"--checkpointSyncer.type", "localStorage",
		"--checkpointSyncer.path", "ARBITRARY_VALUE_FOOBAR",
		"--validator.key", "0x" + info.ValidatorISMPrivKey,
		"--metrics-port", "9090",
		"--log.level", "info",
	}

	env := []string{
		"CONFIG_FILES=/root/configs/agent-config.json",
	}

	go rollapp1.Sidecars[1].Exec(ctx, cmd, env)

	time.Sleep(1 * time.Minute)
	fmt.Println(url.QueryEscape(info.MultisigEscrowAddr))
	utxos, err := GetKaspaUtxos(url.QueryEscape(info.MultisigEscrowAddr))
	require.NoError(t, err)
	fmt.Println(utxos)

	txidHex := utxos[0].Outpoint.TransactionId
	// decode hex to bytes
	txidBytes, err := hex.DecodeString(txidHex)
	require.NoError(t, err)
	// encode to base64
	txidBase64 := base64.StdEncoding.EncodeToString(txidBytes)
	fmt.Println("TransactionId base64:", txidBase64)

	isms, err := dymension.GetNode().QueryIsms(ctx)
	require.NoError(t, err)

	var ismresp IsmResponse
	err = json.Unmarshal([]byte(isms), &ismresp)
	require.NoError(t, err)

	msg := map[string]interface{}{
		"@type":     "/dymensionxyz.dymension.kas.MsgBootstrap",
		"authority": "dym10d07y265gmmuvt4z0w9aw880jnsr700jgllrna",
		"mailbox":   mailboxID,
		"ism":       ismresp.Isms[0].ID,
		"outpoint": map[string]interface{}{
			"transactionId": txidBase64,
			"index":         0,
		},
	}

	rawMsg, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Err:", err)
	}

	proposal := cosmos.TxProposalV1{
		Deposit:     "500000000000" + dymension.Config().Denom,
		Title:       "Bootstrap KAS Module",
		Summary:     "This proposal initializes the Kaspa-Dymension bridge (KAS) module with its core components, enabling cross-chain operations.",
		Description: "This proposal initializes the Kaspa-Dymension bridge (KAS) module with its core components, enabling cross-chain operations.",
		Messages:    []json.RawMessage{rawMsg},
		Expedited:   false,
	}

	_, err = dymension.GetNode().SubmitProposal(ctx, "faucet", proposal)
	require.NoError(t, err, "error submitting change param proposal tx")

	err = dymension.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")

	height, err := dymension.Height(ctx)
	require.NoError(t, err, "error fetching height")
	_, err = cosmos.PollForProposalStatusV50(ctx, dymension.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	require.NoError(t, err, "proposal status did not change to passed")

	cmd = []string{"./relayer",
		"--db", "/root/dbs/hyperlane_db_relayer",
		"--relayChains", "kaspatest10,dymension",
		"--allowLocalCheckpointSyncers", "true",
		"--defaultSigner.key", HYP_KEY,
		"--chains.dymension.signer.type", "cosmosKey",
		"--chains.dymension.signer.prefix", "dym",
		"--chains.dymension.signer.key", HYP_KEY,
		"--chains.kaspatest10.signer.type", "cosmosKey",
		"--chains.kaspatest10.signer.prefix", "dym",
		"--chains.kaspatest10.signer.key", HYP_KEY,
		"--metrics-port", "9091",
		"--log.level", "debug",
	}

	go rollapp1.Sidecars[1].Exec(ctx, cmd, env)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, rollapp1)

	dymensionUser1, rollappUser1 := users[0], users[1]

	dymensionUser1Addr := dymensionUser1.FormattedAddress()
	_ = rollappUser1.FormattedAddress()

	message, err := dymension.GetNode().QueryHyperlaneMessageKaspa(ctx, tokenID, dymensionUser1Addr, "100000000")
	require.NoError(t, err)

	fmt.Println(message)

	cmd = []string{"cargo", "run", "--", "deposit",
		"--escrow-address", info.MultisigEscrowAddr,
		"--amount", "100000000",
		"--wrpc-url", "185.69.54.99:17210",
		"--network-id", "testnet-10",
		"--wallet-secret", "lkjsdf",
		"--payload", strings.TrimPrefix(message, "0x")}

	_, _, err = rollapp1.Sidecars[2].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	res, err := http.Get(fmt.Sprintf("https://api-tn10.kaspa.org/addresses/%s/balance", info.MultisigEscrowAddr))
	require.NoError(t, err)
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	var data KaspaBalanceResponse
	err = json.Unmarshal(body, &data)
	require.NoError(t, err)

	require.Equal(t, uint64(200000000), data.Balance)
}

type KaspaBalanceResponse struct {
	Address string `json:"address"`
	Balance uint64 `json:"balance"` // Đơn vị: sompi
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
