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
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/celes_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"

	"flag"
	"log"
	"net"
	"strconv"

	grpcda "github.com/dymensionxyz/dymint/da/grpc"
	"github.com/dymensionxyz/dymint/da/grpc/mockserv"
	"github.com/dymensionxyz/dymint/store"
)

// StartDA start grpc DALC server
func StartDA() {
	conf := grpcda.DefaultConfig

	flag.IntVar(&conf.Port, "port", conf.Port, "listening port")
	flag.StringVar(&conf.Host, "host", "0.0.0.0", "listening address")
	flag.Parse()

	kv := store.NewDefaultKVStore(".", "db", "dymint")
	lis, err := net.Listen("tcp", conf.Host+":"+strconv.Itoa(conf.Port))
	if err != nil {
		log.Panic(err)
	}
	log.Println("DA grpc listening on:", lis.Addr())
	srv := mockserv.GetServer(kv, conf, nil)
	if err := srv.Serve(lis); err != nil {
		log.Println("error while serving:", err)
	}
}

func StopDA() {
	conf := grpcda.DefaultConfig
	kv := store.NewDefaultKVStore(".", "db", "dymint")
	srv := mockserv.GetServer(kv, conf, nil)
	log.Println("Stopping DA...")
	srv.Stop()
}

func TestFullnodeSync_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	go StartDA()

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
	}, nil, "", nil, false, 780)
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

func TestFullnodeSync_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// start grpc DA
	go StartDA()

	configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["da_config"] = "{\"host\":\"host.docker.internal\",\"port\": 7980}"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides
	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppVals := 1
	numRollAppFn := 1

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
	}, nil, "", nil, false, 780)
	require.NoError(t, err)

	// Wait for rollapp finalized
	rollapp1Height, err := rollapp1.Height(ctx)
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

	// Poll until full node is sync
	err = testutil.WaitForCondition(
		time.Minute*10,
		time.Second*5, // each epoch is 5 seconds
		func() (bool, error) {
			fullnodeHeight, err := rollapp1.FullNodes[0].Height(ctx)
			require.NoError(t, err)

			if valHeight > fullnodeHeight {
				return false, nil
			}

			return true, nil
		},
	)
	require.NoError(t, err)
}

// TestFullnodeSync_Celestia_EVM tests the synchronization of a fullnode using Celestia as DA.
func TestFullnodeSync_Celestia_EVM(t *testing.T) {
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
	}, nil, "", nil, true, 780)
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

	ic = test.NewSetup().
		AddRollUp(dymension, rollapp1)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil, false, 780)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.Validators[0].Height(ctx)
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
}

// TestFullnodeSync_Celestia_EVM tests the synchronization of a fullnode using Celestia as DA.
func TestFullnodeSync_Celestia_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// configFileOverrides := make(map[string]any)
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

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
	}, nil, "", nil, true, 780)
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

	// Create an exec instance
	execConfig := types.ExecConfig{
		Cmd: strslice.StrSlice([]string{"celestia", "light", "start", "--node.store", nodeStore, "--gateway", "--core.ip", coreIp, "--p2p.network", p2pNetwork, "--keyring.keyname", "validator"}), // Replace with your command and arguments
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

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil, true, 780)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.Validators[0].Height(ctx)
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
}
