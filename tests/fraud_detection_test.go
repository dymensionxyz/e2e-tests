package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	// "cosmossdk.io/math"
	// transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/strslice"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

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

type Result struct {
	ChainID string `json:"ChainID"` // Maps to the "ChainID" field in the JSON
	Result  string `json:"Result"`  // Maps to the "Result" field in the JSON
}

type BlockValidatedResponse struct {
	JSONRPC string `json:"jsonrpc"` // Maps to the "jsonrpc" field
	Result  Result `json:"result"`  // Maps the nested "result" object
	ID      int    `json:"id"`      // Maps to the "id" field
}

func TestFraudDetection_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "5s"
	maxProofTime := "500ms"
	configFileOverrides := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "50s", true)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
			ChainConfig:   customEpochConfig("5s"),
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

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1)

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

	// Wait for rollapp finalized
	rollapp1Height, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollapp1Height, 300)
	require.True(t, isFinalized)
	require.NoError(t, err)

	// Stop the full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	// Wait for a few blocks before start the node again and sync
	err = testutil.WaitForBlocks(ctx, 30, dymension)
	require.NoError(t, err)

	// Start full node again
	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	valHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	cmd := []string{"curl", "-X", "GET", fmt.Sprintf("http://%s:26657/block_validated?height=%v", rollapp1.FullNodes[0].Name(), valHeight)}
	sdtout, _, err := rollapp1.FullNodes[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)

	var resp BlockValidatedResponse
	err = json.Unmarshal([]byte(sdtout), &resp)
	require.NoError(t, err)
	require.Equal(t, "0", resp.Result.Result)

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

	valHeight, err = rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)
	cmd = []string{"curl", "-X", "GET", fmt.Sprintf("http://%s:26657/block_validated?height=%v", rollapp1.FullNodes[0].Name(), valHeight)}
	sdtout, _, err = rollapp1.FullNodes[0].Exec(ctx, cmd, nil)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(sdtout), &resp)
	require.NoError(t, err)
	require.Equal(t, "2", resp.Result.Result)
}

func TestFraudDetectionDA_P2P_EVM(t *testing.T) {
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
			Name:          "dymension-hub",
			ChainConfig:   customEpochConfig("5s"),
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

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1)

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

	// Stop the full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	// Wait for a few blocks before start the node again and sync
	err = testutil.WaitForBlocks(ctx, 40, dymension)
	require.NoError(t, err)

	// Start full node again
	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	valHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
		cmd := []string{"curl", "-X", "GET", fmt.Sprintf("http://%s:26657/block_validated?height=%v", rollapp1.FullNodes[0].Name(), valHeight+1)}
		sdtout, _, err := rollapp1.FullNodes[0].Exec(ctx, cmd, nil)
		require.NoError(t, err)

		var resp BlockValidatedResponse
		err = json.Unmarshal([]byte(sdtout), &resp)
		require.NoError(t, err)
		if resp.Result.Result == "0" {
			break
		}
	}

	for i := 0; i < 10; i++ {
		testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
		cmd := []string{"curl", "-X", "GET", fmt.Sprintf("http://%s:26657/block_validated?height=%v", rollapp1.FullNodes[0].Name(), valHeight+1)}
		sdtout, _, err := rollapp1.FullNodes[0].Exec(ctx, cmd, nil)
		require.NoError(t, err)

		var resp BlockValidatedResponse
		err = json.Unmarshal([]byte(sdtout), &resp)
		require.NoError(t, err)
		if resp.Result.Result == "1" {
			break
		}
	}

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

	valHeight, err = rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
		cmd := []string{"curl", "-X", "GET", fmt.Sprintf("http://%s:26657/block_validated?height=%v", rollapp1.FullNodes[0].Name(), valHeight+1)}
		sdtout, _, err := rollapp1.FullNodes[0].Exec(ctx, cmd, nil)
		require.NoError(t, err)

		var resp BlockValidatedResponse
		err = json.Unmarshal([]byte(sdtout), &resp)
		require.NoError(t, err)
		if resp.Result.Result == "2" {
			break
		}
	}
}

func TestFraudDetectionDA_P2P_Wasm(t *testing.T) {
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
	dymintTomlOverrides["da_config"] = "{\"host\":\"grpc-da-container\",\"port\": 7980}"

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
			Name:          "dymension-hub",
			ChainConfig:   customEpochConfig("5s"),
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

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1)

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

	// Stop the full node
	err = rollapp1.FullNodes[0].StopContainer(ctx)
	require.NoError(t, err)

	// Wait for a few blocks before start the node again and sync
	err = testutil.WaitForBlocks(ctx, 40, dymension)
	require.NoError(t, err)

	// Start full node again
	err = rollapp1.FullNodes[0].StartContainer(ctx)
	require.NoError(t, err)

	valHeight, err := rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
		cmd := []string{"curl", "-X", "GET", fmt.Sprintf("http://%s:26657/block_validated?height=%v", rollapp1.FullNodes[0].Name(), valHeight+1)}
		sdtout, _, err := rollapp1.FullNodes[0].Exec(ctx, cmd, nil)
		require.NoError(t, err)

		var resp BlockValidatedResponse
		err = json.Unmarshal([]byte(sdtout), &resp)
		require.NoError(t, err)
		if resp.Result.Result == "0" {
			break
		}
	}

	for i := 0; i < 10; i++ {
		testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
		cmd := []string{"curl", "-X", "GET", fmt.Sprintf("http://%s:26657/block_validated?height=%v", rollapp1.FullNodes[0].Name(), valHeight+1)}
		sdtout, _, err := rollapp1.FullNodes[0].Exec(ctx, cmd, nil)
		require.NoError(t, err)

		var resp BlockValidatedResponse
		err = json.Unmarshal([]byte(sdtout), &resp)
		require.NoError(t, err)
		if resp.Result.Result == "1" {
			break
		}
	}

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

	valHeight, err = rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		testutil.WaitForBlocks(ctx, 1, dymension, rollapp1)
		cmd := []string{"curl", "-X", "GET", fmt.Sprintf("http://%s:26657/block_validated?height=%v", rollapp1.FullNodes[0].Name(), valHeight+1)}
		sdtout, _, err := rollapp1.FullNodes[0].Exec(ctx, cmd, nil)
		require.NoError(t, err)

		var resp BlockValidatedResponse
		err = json.Unmarshal([]byte(sdtout), &resp)
		require.NoError(t, err)
		if resp.Result.Result == "2" {
			break
		}
	}
}

func TestFraudDetect_Corrupted_DA_Blob_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["fraud_cmds_path"] = ""

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

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numCelestiaFn := 0
	numRollAppFn := 1
	numRollAppVals := 1
	nodeStore := "/home/celestia/light"
	p2pNetwork := "mocha-4"
	coreIp := "mocha-4-consensus.mesa.newmetric.xyz"
	// trustedHash := "\"A62DD37EDF3DFF5A7383C6B5AA2AC619D1F8C8FEB7BA07730E50BBAAC8F2FF0C\""
	// sampleFrom := 2395649

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

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	validator, err := celestia.Validators[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	// Get fund for submit blob
	GetFaucet("http://18.184.170.181:3000/api/get-tia", validator)

	time.Sleep(30 * time.Second)

	err = celestia.GetNode().InitCelestiaDaLightNode(ctx, nodeStore, p2pNetwork, nil)
	require.NoError(t, err)

	time.Sleep(30 * time.Second)

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

	time.Sleep(30 * time.Second)

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

	// Relayer Factory
	client, network = test.DockerSetup(t)

	StartDA(ctx, t, client, network)

	// relayer for rollapp 1
	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil, false, 1179360, true)
	require.NoError(t, err)

	fmt.Println("rollapp1111:", rollapp1)
	keyDir1 := dymension.GetRollApps()[0].GetSequencerKeyDir()
	parts := strings.Split(keyDir1, "/")
	fraudFolderName := parts[len(parts)-1]
	fmt.Println("fraudFolderName:", fraudFolderName)

	fraudCmdsPath := "/tmp/" + fraudFolderName + "/fraud.json"
	fmt.Println("fraudCmdsPath", fraudCmdsPath)

	file, err = os.Open(fmt.Sprintf("/tmp/%s/config/dymint.toml", fraudFolderName))
	require.NoError(t, err)
	defer file.Close()

	lines = []string{}
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "fraud_cmds_path =") {
			lines[i] = fmt.Sprintf("fraud_cmds_path = \"%s\"", fraudCmdsPath)
		}
	}

	output = strings.Join(lines, "\n")
	file, err = os.Create(fmt.Sprintf("/tmp/%s/config/dymint.toml", fraudFolderName))
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write([]byte(output))
	require.NoError(t, err)

	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	err = testutil.WaitForBlocks(ctx, 2, dymension, rollapp1)
	require.NoError(t, err)

	//Update white listed relayers
	_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
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

	// Compose an IBC transfer and send from rollapp -> Hub
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	rollappHeight, err = rollapp1.Validators[0].Height(ctx)
	require.NoError(t, err)

	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
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

	// check freeze
	err = testutil.WaitForBlocks(ctx, 20, rollapp1)
	require.NoError(t, err)
}