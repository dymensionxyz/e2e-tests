package tests

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/celes_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"

	"strconv"
)

// TestSync_Celes_Rt_Gossip_EVM tests the synchronization of a fullnode using Celestia as DA.
func TestSync_Celes_Rt_Gossip_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "250ms"
	dymintTomlOverrides["max_proof_time"] = "150ms"
	dymintTomlOverrides["batch_submit_time"] = "5s"
	dymintTomlOverrides["block_time"] = "200ms"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides1 := make(map[string]any)
	configTomlOverrides1 := make(testutil.Toml)
	configTomlOverrides1["timeout_commit"] = "2s"
	configTomlOverrides1["timeout_propose"] = "2s"
	configTomlOverrides1["index_all_keys"] = "true"
	configTomlOverrides1["mode"] = "validator"

	configFileOverrides1["config/config.toml"] = configTomlOverrides1

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

	url := "https://api-mocha.celenium.io/v1/block/count"
	headerKey := "User-Agent"
	headerValue := "Apidog/1.0.0 (https://apidog.com)"
	rpcEndpoint := "http://celestia-mocha-archive-rpc.mzonder.com:26657"

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	validator, err := celestia.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	// Get fund for submit blob
	GetFaucet("http://18.184.170.181:3000/api/get-tia", validator)
	err = testutil.WaitForBlocks(ctx, 10, celestia)
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

	// Start Celestia light node with retry mechanism
	err = StartCelestiaLightNodeWithRetry(ctx, t, client, containerID, nodeStore, p2pNetwork, fmt.Sprintf("http://test-val-0-%s:26658", t.Name()), celestia.GetNode())
	require.NoError(t, err)

	celestia_token, err := celestia.GetNode().GetAuthTokenCelestiaDaLight(ctx, p2pNetwork, nodeStore)
	require.NoError(t, err)
	println("check token: ", celestia_token)
	celestia_namespace_id, err := RandomHex(10)
	require.NoError(t, err)
	println("check namespace: ", celestia_namespace_id)
	da_config := []string{fmt.Sprintf("{\"base_url\": \"http://test-val-0-%s:26658\", \"timeout\": 60000000000, \"gas_prices\":1.0, \"gas_adjustment\": 1.3, \"namespace_id\": \"%s\", \"auth_token\":\"%s\"}", t.Name(), celestia_namespace_id, celestia_token)}

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides["namespace_id"] = celestia_namespace_id
	dymintTomlOverrides["da_layer"] = []string{"celestia"}
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

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,
	}, nil, "", nil, true, 1179360, true)
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

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

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
}

// TestSync_Celes_Rt_Gossip_Wasm tests the synchronization of a fullnode using Celestia as DA.
func TestSync_Celes_Rt_Gossip_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "250ms"
	dymintTomlOverrides["max_proof_time"] = "150ms"
	dymintTomlOverrides["batch_submit_time"] = "5s"
	dymintTomlOverrides["block_time"] = "200ms"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides1 := make(map[string]any)
	configTomlOverrides1 := make(testutil.Toml)
	configTomlOverrides1["timeout_commit"] = "2s"
	configTomlOverrides1["timeout_propose"] = "2s"
	configTomlOverrides1["index_all_keys"] = "true"
	configTomlOverrides1["mode"] = "validator"

	configFileOverrides1["config/config.toml"] = configTomlOverrides1

	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "celestia",
		},
	)

	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 1
	numRollAppVals := 1
	numCelestiaFn := 0
	nodeStore := "/home/celestia/light"
	p2pNetwork := "mocha-4"

	url := "https://api-mocha.celenium.io/v1/block/count"
	headerKey := "User-Agent"
	headerValue := "Apidog/1.0.0 (https://apidog.com)"
	rpcEndpoint := "http://celestia-mocha-archive-rpc.mzonder.com:26657"

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	validator, err := celestia.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	// Get fund for submit blob
	GetFaucet("http://18.184.170.181:3000/api/get-tia", validator)
	err = testutil.WaitForBlocks(ctx, 10, celestia)
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

	fmt.Println(hash)

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

	// Start Celestia light node with retry mechanism
	err = StartCelestiaLightNodeWithRetry(ctx, t, client, containerID, nodeStore, p2pNetwork, fmt.Sprintf("http://test-val-0-%s:26658", t.Name()), celestia.GetNode())
	require.NoError(t, err)

	celestia_token, err := celestia.GetNode().GetAuthTokenCelestiaDaLight(ctx, p2pNetwork, nodeStore)
	require.NoError(t, err)
	println("check token: ", celestia_token)
	celestia_namespace_id, err := RandomHex(10)
	require.NoError(t, err)
	println("check namespace: ", celestia_namespace_id)
	da_config := []string{fmt.Sprintf("{\"base_url\": \"http://test-val-0-%s:26658\", \"timeout\": 60000000000, \"gas_prices\":1.0, \"gas_adjustment\": 1.3, \"namespace_id\": \"%s\", \"auth_token\":\"%s\"}", t.Name(), celestia_namespace_id, celestia_token)}

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides["namespace_id"] = celestia_namespace_id
	dymintTomlOverrides["da_layer"] = []string{"celestia"}
	dymintTomlOverrides["da_config"] = da_config
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	cf = test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
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
	chains, err = cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	containerID = fmt.Sprintf("ra-rollappwasm_1234-1-val-0-%s", t.Name())

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

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	valHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	// Poll until full node is sync
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

// Test_Sqc_Disconnect_Gossip_EVM tests the synchronization of a fullnode using Celestia as DA.
func TestSync_Sqc_Disconnect_Gossip_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "250ms"
	dymintTomlOverrides["max_proof_time"] = "150ms"
	dymintTomlOverrides["batch_submit_time"] = "5s"
	dymintTomlOverrides["block_time"] = "200ms"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides1 := make(map[string]any)
	configTomlOverrides1 := make(testutil.Toml)
	configTomlOverrides1["timeout_commit"] = "2s"
	configTomlOverrides1["timeout_propose"] = "2s"
	configTomlOverrides1["index_all_keys"] = "true"
	configTomlOverrides1["mode"] = "validator"

	configFileOverrides1["config/config.toml"] = configTomlOverrides1

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

	url := "https://api-mocha.celenium.io/v1/block/count"
	headerKey := "User-Agent"
	headerValue := "Apidog/1.0.0 (https://apidog.com)"
	rpcEndpoint := "http://celestia-mocha-archive-rpc.mzonder.com:26657"

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	validator, err := celestia.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	// Get fund for submit blob
	GetFaucet("http://18.184.170.181:3000/api/get-tia", validator)
	err = testutil.WaitForBlocks(ctx, 10, celestia)
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

	// Start Celestia light node with retry mechanism
	err = StartCelestiaLightNodeWithRetry(ctx, t, client, containerID, nodeStore, p2pNetwork, fmt.Sprintf("http://test-val-0-%s:26658", t.Name()), celestia.GetNode())
	require.NoError(t, err)

	celestia_token, err := celestia.GetNode().GetAuthTokenCelestiaDaLight(ctx, p2pNetwork, nodeStore)
	require.NoError(t, err)
	println("check token: ", celestia_token)
	celestia_namespace_id, err := RandomHex(10)
	require.NoError(t, err)
	println("check namespace: ", celestia_namespace_id)
	da_config := []string{fmt.Sprintf("{\"base_url\": \"http://test-val-0-%s:26658\", \"timeout\": 60000000000, \"gas_prices\":1.0, \"gas_adjustment\": 1.3, \"namespace_id\": \"%s\", \"auth_token\":\"%s\"}", t.Name(), celestia_namespace_id, celestia_token)}

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides["namespace_id"] = celestia_namespace_id
	dymintTomlOverrides["da_layer"] = []string{"celestia"}
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

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,
	}, nil, "", nil, true, 1179360, true)
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

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	_ = rollapp1.FullNodes[0].StartContainer(ctx)

	rollappHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

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

	err = rollapp1.Validators[0].StopContainer(ctx)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, celestia)
	require.NoError(t, err)

	_ = rollapp1.Validators[0].StartContainer(ctx)

	valHeight, err = rollapp1.Validators[0].Height(ctx)
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
}

// Test_Sqc_Disconnect_Gossip_Wasm tests the synchronization of a fullnode using Celestia as DA.
func TestSync_Sqc_Disconnect_Gossip_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "250ms"
	dymintTomlOverrides["max_proof_time"] = "150ms"
	dymintTomlOverrides["batch_submit_time"] = "5s"
	dymintTomlOverrides["block_time"] = "200ms"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides1 := make(map[string]any)
	configTomlOverrides1 := make(testutil.Toml)
	configTomlOverrides1["timeout_commit"] = "2s"
	configTomlOverrides1["timeout_propose"] = "2s"
	configTomlOverrides1["index_all_keys"] = "true"
	configTomlOverrides1["mode"] = "validator"

	configFileOverrides1["config/config.toml"] = configTomlOverrides1

	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
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

	url := "https://api-mocha.celenium.io/v1/block/count"
	headerKey := "User-Agent"
	headerValue := "Apidog/1.0.0 (https://apidog.com)"
	rpcEndpoint := "http://celestia-mocha-archive-rpc.mzonder.com:26657"

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	validator, err := celestia.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	// Get fund for submit blob
	GetFaucet("http://18.184.170.181:3000/api/get-tia", validator)
	err = testutil.WaitForBlocks(ctx, 10, celestia)
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

	// Start Celestia light node with retry mechanism
	err = StartCelestiaLightNodeWithRetry(ctx, t, client, containerID, nodeStore, p2pNetwork, fmt.Sprintf("http://test-val-0-%s:26658", t.Name()), celestia.GetNode())
	require.NoError(t, err)

	celestia_token, err := celestia.GetNode().GetAuthTokenCelestiaDaLight(ctx, p2pNetwork, nodeStore)
	require.NoError(t, err)
	println("check token: ", celestia_token)
	celestia_namespace_id, err := RandomHex(10)
	require.NoError(t, err)
	println("check namespace: ", celestia_namespace_id)
	da_config := []string{fmt.Sprintf("{\"base_url\": \"http://test-val-0-%s:26658\", \"timeout\": 60000000000, \"gas_prices\":1.0, \"gas_adjustment\": 1.3, \"namespace_id\": \"%s\", \"auth_token\":\"%s\"}", t.Name(), celestia_namespace_id, celestia_token)}

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides["namespace_id"] = celestia_namespace_id
	dymintTomlOverrides["da_layer"] = []string{"celestia"}
	dymintTomlOverrides["da_config"] = da_config
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	cf = test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
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
	chains, err = cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	containerID = fmt.Sprintf("ra-rollappwasm_1234-1-val-0-%s", t.Name())

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

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	_ = rollapp1.FullNodes[0].StartContainer(ctx)

	rollappHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

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

	err = rollapp1.Validators[0].StopContainer(ctx)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, celestia)
	require.NoError(t, err)

	_ = rollapp1.Validators[0].StartContainer(ctx)

	valHeight, err = rollapp1.Validators[0].Height(ctx)
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
}

// TestSync_Fullnode_Disconnect_Gossip_EVM tests the synchronization of a fullnode using Celestia as DA.
func TestSync_Fullnode_Disconnect_Gossip_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "250ms"
	dymintTomlOverrides["max_proof_time"] = "150ms"
	dymintTomlOverrides["batch_submit_time"] = "5s"
	dymintTomlOverrides["block_time"] = "200ms"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides1 := make(map[string]any)
	configTomlOverrides1 := make(testutil.Toml)
	configTomlOverrides1["timeout_commit"] = "2s"
	configTomlOverrides1["timeout_propose"] = "2s"
	configTomlOverrides1["index_all_keys"] = "true"
	configTomlOverrides1["mode"] = "validator"

	configFileOverrides1["config/config.toml"] = configTomlOverrides1

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

	url := "https://api-mocha.celenium.io/v1/block/count"
	headerKey := "User-Agent"
	headerValue := "Apidog/1.0.0 (https://apidog.com)"
	rpcEndpoint := "http://celestia-mocha-archive-rpc.mzonder.com:26657"

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	validator, err := celestia.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	// Get fund for submit blob
	GetFaucet("http://18.184.170.181:3000/api/get-tia", validator)
	err = testutil.WaitForBlocks(ctx, 10, celestia)
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

	// Start Celestia light node with retry mechanism
	err = StartCelestiaLightNodeWithRetry(ctx, t, client, containerID, nodeStore, p2pNetwork, fmt.Sprintf("http://test-val-0-%s:26658", t.Name()), celestia.GetNode())
	require.NoError(t, err)

	celestia_token, err := celestia.GetNode().GetAuthTokenCelestiaDaLight(ctx, p2pNetwork, nodeStore)
	require.NoError(t, err)
	println("check token: ", celestia_token)
	celestia_namespace_id, err := RandomHex(10)
	require.NoError(t, err)
	println("check namespace: ", celestia_namespace_id)
	da_config := []string{fmt.Sprintf("{\"base_url\": \"http://test-val-0-%s:26658\", \"timeout\": 60000000000, \"gas_prices\":1.0, \"gas_adjustment\": 1.3, \"namespace_id\": \"%s\", \"auth_token\":\"%s\"}", t.Name(), celestia_namespace_id, celestia_token)}

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides["namespace_id"] = celestia_namespace_id
	dymintTomlOverrides["da_layer"] = []string{"celestia"}
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

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,
	}, nil, "", nil, true, 1179360, true)
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

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

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

	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, celestia)
	require.NoError(t, err)

	_ = rollapp1.FullNodes[0].StartContainer(ctx)

	valHeight, err = rollapp1.Validators[0].Height(ctx)
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
}

// TestSync_Fullnode_Disconnect_Gossip_Wasm tests the synchronization of a fullnode using Celestia as DA.
func TestSync_Fullnode_Disconnect_Gossip_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "250ms"
	dymintTomlOverrides["max_proof_time"] = "150ms"
	dymintTomlOverrides["batch_submit_time"] = "5s"
	dymintTomlOverrides["block_time"] = "200ms"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "true"

	configFileOverrides1 := make(map[string]any)
	configTomlOverrides1 := make(testutil.Toml)
	configTomlOverrides1["timeout_commit"] = "2s"
	configTomlOverrides1["timeout_propose"] = "2s"
	configTomlOverrides1["index_all_keys"] = "true"
	configTomlOverrides1["mode"] = "validator"

	configFileOverrides1["config/config.toml"] = configTomlOverrides1

	modifyWasmGenesisKV := append(
		rollappWasmGenesisKV,
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

	url := "https://api-mocha.celenium.io/v1/block/count"
	headerKey := "User-Agent"
	headerValue := "Apidog/1.0.0 (https://apidog.com)"
	rpcEndpoint := "http://celestia-mocha-archive-rpc.mzonder.com:26657"

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
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	validator, err := celestia.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	// Get fund for submit blob
	GetFaucet("http://18.184.170.181:3000/api/get-tia", validator)
	err = testutil.WaitForBlocks(ctx, 10, celestia)
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

	// Start Celestia light node with retry mechanism
	err = StartCelestiaLightNodeWithRetry(ctx, t, client, containerID, nodeStore, p2pNetwork, fmt.Sprintf("http://test-val-0-%s:26658", t.Name()), celestia.GetNode())
	require.NoError(t, err)

	celestia_token, err := celestia.GetNode().GetAuthTokenCelestiaDaLight(ctx, p2pNetwork, nodeStore)
	require.NoError(t, err)
	println("check token: ", celestia_token)
	celestia_namespace_id, err := RandomHex(10)
	require.NoError(t, err)
	println("check namespace: ", celestia_namespace_id)
	da_config := []string{fmt.Sprintf("{\"base_url\": \"http://test-val-0-%s:26658\", \"timeout\": 60000000000, \"gas_prices\":1.0, \"gas_adjustment\": 1.3, \"namespace_id\": \"%s\", \"auth_token\":\"%s\"}", t.Name(), celestia_namespace_id, celestia_token)}

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides["namespace_id"] = celestia_namespace_id
	dymintTomlOverrides["da_layer"] = []string{"celestia"}
	dymintTomlOverrides["da_config"] = da_config
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	cf = test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
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
	chains, err = cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	containerID = fmt.Sprintf("ra-rollappwasm_1234-1-val-0-%s", t.Name())

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

	// Start full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

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

	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 30, celestia)
	require.NoError(t, err)

	_ = rollapp1.FullNodes[0].StartContainer(ctx)

	valHeight, err = rollapp1.Validators[0].Height(ctx)
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
}
