package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

// =============================================================================
// Avail DA Constants
// =============================================================================

const (
	// AvailTuringRPCEndpoint is the public RPC endpoint for Avail Turing testnet
	AvailTuringRPCEndpoint = "https://avail-turing-rpc.publicnode.com"

	// AvailTuringSubscanAPI is the Subscan API endpoint for Avail Turing testnet
	AvailTuringSubscanAPI = "https://avail-turing.api.subscan.io"

	// AvailAppID is the application ID for blob submission on Avail
	AvailAppID = 1

	// AvailMnemonic is a deterministic mnemonic for the Avail account used in tests.
	AvailMnemonic = "plug mandate gossip deposit reduce civil lawn extra fantasy grow increase off"

	// AvailAddress is the address derived from AvailMnemonic on Avail Turing testnet (sr25519)
	// You can verify this by importing the mnemonic in Polkadot.js extension
	AvailAddress = "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY"
)

// MinAvailBalance is the minimum balance required (1 AVAIL = 10^18 base units)
var MinAvailBalance = math.NewInt(1_000_000_000_000_000_000) // 1 AVAIL

// =============================================================================
// Kaspa DA Constants
// =============================================================================

const (
	// KaspaTestnetAPIURL is the REST API endpoint for Kaspa Testnet-10
	KaspaTestnetAPIURL = "https://api-tn10.kaspa.org"

	// KaspaTestnetEndpoint is the gRPC endpoint for Kaspa Testnet-10
	KaspaTestnetEndpoint = "rpc.tn.kaspa.rollapp.network:443"

	// KaspaNetworkID is the network identifier for Kaspa Testnet-10
	KaspaNetworkID = "kaspa-testnet-10"

	// KaspaMnemonic is a deterministic mnemonic for the Kaspa account used in tests.
	KaspaMnemonic = "broom home badge wrap unveil smoke birth erupt scan merry deny neglect pull select hold winner crouch hazard wear van spell jewel moment actual"

	// KaspaAddress is the address derived from KaspaMnemonic at path m/44'/111111'/0'/0/0
	// You can verify this using kaspa-wallet tools
	KaspaAddress = "kaspatest:qz0cqguv76hxhdzt4acsl7s24yg5msa0ggfeqjw3yj8twmz8fv7wvjhm6qrgh"
)

// MinKaspaBalance is the minimum balance required (1 KAS = 10^8 Sompi)
var MinKaspaBalance = math.NewInt(100_000_000) // 1 KAS

// =============================================================================
// Avail Balance Check Functions
// =============================================================================

// checkAvailBalance checks if the Avail account has sufficient balance on Turing testnet.
// If balance is below MinAvailBalance, it fails the test with instructions to fund the address.
func checkAvailBalance(t *testing.T, address string) {
	balance, err := queryAvailTuringBalance(address)
	if err != nil {
		t.Logf("Warning: failed to query Avail balance: %v", err)
		t.Logf("Please ensure the Avail account is funded before running this test")
		t.Logf("Address: %s", address)
		t.Logf("Faucet: https://faucet.avail.tools/")
		return
	}

	// Convert to AVAIL for display (18 decimals)
	balanceAVAIL := balance.Quo(math.NewInt(1_000_000_000_000_000_000))

	if balance.LT(MinAvailBalance) {
		t.Fatalf(`
================================================================================
INSUFFICIENT AVAIL BALANCE ON TURING TESTNET
================================================================================
The Avail account has insufficient balance for blob submission.

Address: %s
Current balance: %s AVAIL
Required minimum: 1 AVAIL

Please fund this address with AVAIL tokens using the Avail Turing testnet faucet:
https://faucet.avail.tools/

NOTE: This address is derived from the test mnemonic and will be the same across all test runs.
================================================================================
`, address, balanceAVAIL.String())
	}

	t.Logf("Avail Turing balance for %s: %s AVAIL", address, balanceAVAIL.String())
}

// queryAvailTuringBalance queries the balance of an address on Avail Turing testnet using Subscan API
func queryAvailTuringBalance(address string) (math.Int, error) {
	url := fmt.Sprintf("%s/api/v2/scan/search", AvailTuringSubscanAPI)

	// Subscan API request body
	requestBody := map[string]string{
		"key": address,
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to query Subscan API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Account struct {
				Balance string `json:"balance"`
			} `json:"account"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return math.Int{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != 0 {
		return math.Int{}, fmt.Errorf("Subscan API error: %s", result.Message)
	}

	if result.Data.Account.Balance == "" {
		return math.ZeroInt(), nil
	}

	// Subscan returns balance as a float string in AVAIL units (18 decimals)
	// We need to parse it as float and convert to base units
	balanceFloat, err := strconv.ParseFloat(result.Data.Account.Balance, 64)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to parse balance: %s", result.Data.Account.Balance)
	}

	// Convert from AVAIL to base units (multiply by 10^18)
	// Use big.Float for precision
	bigBalance := new(big.Float).SetFloat64(balanceFloat)
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	bigBalance.Mul(bigBalance, multiplier)

	// Convert to big.Int (truncate decimals)
	balanceInt := new(big.Int)
	bigBalance.Int(balanceInt)

	return math.NewIntFromBigInt(balanceInt), nil
}

// =============================================================================
// Kaspa Balance Check Functions
// =============================================================================

// checkKaspaBalance checks if the Kaspa account has sufficient balance on Testnet-10.
// If balance is below MinKaspaBalance, it fails the test with instructions to fund the address.
func checkKaspaBalance(t *testing.T, address string) {
	balance, err := queryKaspaTestnetBalance(address)
	if err != nil {
		t.Logf("Warning: failed to query Kaspa balance: %v", err)
		t.Logf("Please ensure the Kaspa account is funded before running this test")
		t.Logf("Address: %s", address)
		t.Logf("Faucet: https://faucet.kaspanet.io/")
		return
	}

	// Convert to KAS for display (8 decimals, 1 KAS = 10^8 Sompi)
	balanceKAS := balance.Quo(math.NewInt(100_000_000))

	if balance.LT(MinKaspaBalance) {
		t.Fatalf(`
================================================================================
INSUFFICIENT KASPA BALANCE ON TESTNET-10
================================================================================
The Kaspa account has insufficient balance for blob submission.

Address: %s
Current balance: %s KAS
Required minimum: 1 KAS

Please fund this address with KAS tokens using the Kaspa testnet faucet:
https://faucet.kaspanet.io/

NOTE: This address is derived from the test mnemonic and will be the same across all test runs.
================================================================================
`, address, balanceKAS.String())
	}

	t.Logf("Kaspa Testnet-10 balance for %s: %s KAS", address, balanceKAS.String())
}

// queryKaspaTestnetBalance queries the balance of an address on Kaspa Testnet-10 using REST API
func queryKaspaTestnetBalance(address string) (math.Int, error) {
	url := fmt.Sprintf("%s/addresses/%s/balance", KaspaTestnetAPIURL, address)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to query Kaspa API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return math.Int{}, fmt.Errorf("Kaspa API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Address string `json:"address"`
		Balance uint64 `json:"balance"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return math.Int{}, fmt.Errorf("failed to parse response: %w", err)
	}

	return math.NewIntFromUint64(result.Balance), nil
}

// =============================================================================
// Avail DA Test
// =============================================================================

// TestFullnodeSync_Avail_EVM tests the synchronization of a fullnode using Avail as DA.
// This test submits batches to Avail and verifies the fullnode can sync from DA.
func TestFullnodeSync_Avail_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Check Avail balance before starting the test
	t.Logf("Checking Avail balance for address: %s", AvailAddress)
	checkAvailBalance(t, AvailAddress)

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "30s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	// Avail DA configuration
	da_config := []string{fmt.Sprintf(`{"endpoint": "%s", "app_id": %d, "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
		AvailTuringRPCEndpoint, AvailAppID, AvailMnemonic)}

	dymintTomlOverrides["da_layer"] = []string{"avail"}
	dymintTomlOverrides["da_config"] = da_config

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "avail",
		},
	)

	// Create chain factory
	numHubVals := 1
	numHubFullNodes := 0
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

	// Docker setup
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
	}, nil, "", nil, false, 1179360, true)
	require.NoError(t, err)

	t.Log("Chains started, waiting for rollapp to produce blocks and submit batches to Avail...")

	// Wait for enough blocks to ensure multiple batches are submitted
	// batch_submit_time is 30s, so wait for sufficient time
	targetBlocks := 50
	err = testutil.WaitForBlocks(ctx, targetBlocks, rollapp1)
	require.NoError(t, err)

	// Query the hub for the last submitted state (target for fullnode sync)
	// Use finalized=false to get the latest submitted state
	rollappState, err := dymension.QueryRollappState(ctx, rollapp1.GetChainID(), false)
	require.NoError(t, err)

	// Calculate the last submitted height: StartHeight + NumBlocks - 1
	startHeight, err := strconv.ParseInt(rollappState.StateInfo.StartHeight, 10, 64)
	require.NoError(t, err)
	numBlocks, err := strconv.ParseInt(rollappState.StateInfo.NumBlocks, 10, 64)
	require.NoError(t, err)
	targetHeight := startHeight + numBlocks - 1
	t.Logf("Last submitted state: StartHeight=%d, NumBlocks=%d, TargetHeight=%d", startHeight, numBlocks, targetHeight)

	// Stop the sequencer to prevent more batches from being submitted
	t.Log("Stopping sequencer...")
	err = rollapp1.Validators[0].StopContainer(ctx)
	require.NoError(t, err)
	t.Log("Sequencer stopped, waiting for fullnode to sync...")

	// Poll until fullnode syncs to the target height
	err = testutil.WaitForCondition(
		time.Minute*10,
		time.Second*5,
		func() (bool, error) {
			fullnodeHeight, err := rollapp1.FullNodes[0].Height(ctx)
			if err != nil {
				t.Logf("Error getting fullnode height: %v", err)
				return false, nil
			}

			t.Logf("Fullnode height: %d / Target: %d", fullnodeHeight, targetHeight)

			// Fullnode should catch up to at least the target height
			if fullnodeHeight >= targetHeight {
				return true, nil
			}

			return false, nil
		},
	)
	require.NoError(t, err)

	finalHeight, err := rollapp1.FullNodes[0].Height(ctx)
	require.NoError(t, err)
	t.Logf("Fullnode successfully synced to height %d using Avail DA", finalHeight)
}

// =============================================================================
// Kaspa DA Test
// =============================================================================

// TestFullnodeSync_Kaspa_EVM tests the synchronization of a fullnode using Kaspa as DA.
// This test submits batches to Kaspa and verifies the fullnode can sync from DA.
func TestFullnodeSync_Kaspa_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Check Kaspa balance before starting the test
	t.Logf("Checking Kaspa balance for address: %s", KaspaAddress)
	checkKaspaBalance(t, KaspaAddress)

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "30s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	// Kaspa DA configuration
	da_config := []string{fmt.Sprintf(`{"api_url": "%s", "endpoint": "%s", "network_id": "%s", "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
		KaspaTestnetAPIURL, KaspaTestnetEndpoint, KaspaNetworkID, KaspaMnemonic)}

	dymintTomlOverrides["da_layer"] = []string{"kaspa"}
	dymintTomlOverrides["da_config"] = da_config

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "kaspa",
		},
	)

	// Create chain factory
	numHubVals := 1
	numHubFullNodes := 0
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

	// Docker setup
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
	}, nil, "", nil, false, 1179360, true)
	require.NoError(t, err)

	t.Log("Chains started, waiting for rollapp to produce blocks and submit batches to Kaspa...")

	// Wait for enough blocks to ensure multiple batches are submitted
	// batch_submit_time is 30s, so wait for sufficient time
	targetBlocks := 50
	err = testutil.WaitForBlocks(ctx, targetBlocks, rollapp1)
	require.NoError(t, err)

	// Query the hub for the last submitted state (target for fullnode sync)
	// Use finalized=false to get the latest submitted state
	rollappState, err := dymension.QueryRollappState(ctx, rollapp1.GetChainID(), false)
	require.NoError(t, err)

	// Calculate the last submitted height: StartHeight + NumBlocks - 1
	startHeight, err := strconv.ParseInt(rollappState.StateInfo.StartHeight, 10, 64)
	require.NoError(t, err)
	numBlocks, err := strconv.ParseInt(rollappState.StateInfo.NumBlocks, 10, 64)
	require.NoError(t, err)
	targetHeight := startHeight + numBlocks - 1
	t.Logf("Last submitted state: StartHeight=%d, NumBlocks=%d, TargetHeight=%d", startHeight, numBlocks, targetHeight)

	// Stop the sequencer to prevent more batches from being submitted
	t.Log("Stopping sequencer...")
	err = rollapp1.Validators[0].StopContainer(ctx)
	require.NoError(t, err)
	t.Log("Sequencer stopped, waiting for fullnode to sync...")

	// Poll until fullnode syncs to the target height
	err = testutil.WaitForCondition(
		time.Minute*10,
		time.Second*5,
		func() (bool, error) {
			fullnodeHeight, err := rollapp1.FullNodes[0].Height(ctx)
			if err != nil {
				t.Logf("Error getting fullnode height: %v", err)
				return false, nil
			}

			t.Logf("Fullnode height: %d / Target: %d", fullnodeHeight, targetHeight)

			// Fullnode should catch up to at least the target height
			if fullnodeHeight >= targetHeight {
				return true, nil
			}

			return false, nil
		},
	)
	require.NoError(t, err)

	finalHeight, err := rollapp1.FullNodes[0].Height(ctx)
	require.NoError(t, err)
	t.Logf("Fullnode successfully synced to height %d using Kaspa DA", finalHeight)
}
