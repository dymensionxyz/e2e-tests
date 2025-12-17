package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
// Common DA Constants
// =============================================================================

const (
	// DAMnemonic is a deterministic mnemonic used for all DA accounts in tests.
	// Each DA derives its address using its own derivation path and key type.
	DAMnemonic = "bottom drive obey lake curtain smoke basket hold race lonely fit walk"
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

	// AvailAddress is the address derived from DAMnemonic on Avail (sr25519, SS58 prefix 42)
	// Derived using: availgo.Account.NewKeyPair(mnemonic).SS58Address(42)
	AvailAddress = "5DfhGyQdFobKM8NsWvEeAKk5EQQgYe9AydgJ7rMB6E1EqRzV"
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

	// KaspaAddress is the address derived from DAMnemonic at path m/44'/111111'/0'/0/0
	// Derived using Kaspa BIP44 derivation with testnet parameters
	KaspaAddress = "kaspatest:qprvzewn9zt2gcguayp6v93nehr808gmh07wclf7dd8dj62ns8fpzx8d5zj5q"
)

// MinKaspaBalance is the minimum balance required (1 KAS = 10^8 Sompi)
var MinKaspaBalance = math.NewInt(100_000_000) // 1 KAS

// =============================================================================
// Sui DA Constants
// =============================================================================

const (
	// SuiDevnetEndpoint is the public RPC endpoint for Sui Devnet
	SuiDevnetEndpoint = "https://fullnode.devnet.sui.io:443"

	// SuiNoopContractAddress is the noop contract address on Sui Devnet
	SuiNoopContractAddress = "0x3dbdaa3db8d587deb38be3d4825ff434f1723054a6f43e04c0623f2c21a3f8a2"

	// SuiGasBudget is the gas budget for Sui transactions (0.01 SUI)
	SuiGasBudget = "10000000"

	// SuiAddress is the address derived from DAMnemonic on Sui (ed25519)
	// Derived using: suisigner.NewSignertWithMnemonic(mnemonic).Address
	SuiAddress = "0x936accb491f0facaac668baaedcf4d0cfc6da1120b66f77fa6a43af718669571"
)

// MinSuiBalance is the minimum balance required (0.1 SUI = 10^8 MIST)
var MinSuiBalance = math.NewInt(100_000_000) // 0.1 SUI

// SuiNoopContractPath is the relative path to the noop contract source
const SuiNoopContractPath = "../dymint/da/sui/noop"

// =============================================================================
// Sui Contract Deployment Helpers
// =============================================================================

// checkSuiObjectExists checks if an object exists on Sui using JSON-RPC
func checkSuiObjectExists(objectID string) bool {
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sui_getObject",
		"params": []interface{}{
			objectID,
			map[string]bool{"showType": true},
		},
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return false
	}

	req, err := http.NewRequest("POST", SuiDevnetEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var result struct {
		Result struct {
			Data  interface{} `json:"data"`
			Error interface{} `json:"error"`
		} `json:"result"`
		Error interface{} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}

	// If there's an error or no data, the object doesn't exist
	if result.Error != nil || result.Result.Error != nil || result.Result.Data == nil {
		return false
	}

	return true
}

// ensureSuiNoopContract checks if the noop contract exists and deploys it if not.
// Returns the contract address to use.
func ensureSuiNoopContract(t *testing.T) string {
	// First check if the default contract exists
	if checkSuiObjectExists(SuiNoopContractAddress) {
		t.Logf("Sui noop contract exists at %s", SuiNoopContractAddress)
		return SuiNoopContractAddress
	}

	t.Logf("Sui noop contract not found at %s, deploying new contract...", SuiNoopContractAddress)

	// Deploy new contract
	contractAddr, err := deploySuiNoopContract(t)
	if err != nil {
		t.Fatalf("Failed to deploy Sui noop contract: %v", err)
	}

	t.Logf("Successfully deployed Sui noop contract at %s", contractAddr)
	return contractAddr
}

// deploySuiNoopContract deploys the noop contract to Sui devnet and returns the package ID
func deploySuiNoopContract(t *testing.T) (string, error) {
	// Get absolute path to contract directory
	contractPath, err := filepath.Abs(SuiNoopContractPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// First, set up Sui CLI environment for devnet
	setupCmd := exec.Command("sui", "client", "switch", "--env", "devnet")
	setupOutput, err := setupCmd.CombinedOutput()
	if err != nil {
		// Try to create the devnet environment first
		createEnvCmd := exec.Command("sui", "client", "new-env", "--alias", "devnet", "--rpc", SuiDevnetEndpoint)
		createEnvCmd.CombinedOutput() // Ignore error if already exists

		// Try switch again
		setupCmd = exec.Command("sui", "client", "switch", "--env", "devnet")
		setupOutput, err = setupCmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to switch to devnet: %s: %w", string(setupOutput), err)
		}
	}

	// Import the mnemonic if not already imported
	// Note: This may fail if already imported, which is fine
	importCmd := exec.Command("sui", "keytool", "import", DAMnemonic, "ed25519")
	importCmd.CombinedOutput() // Ignore error if already imported

	// Publish the contract
	publishCmd := exec.Command("sui", "client", "publish", "--gas-budget", "100000000", "--json")
	publishCmd.Dir = contractPath
	publishOutput, err := publishCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to publish contract: %s: %w", string(publishOutput), err)
	}

	// Parse the JSON output to get the package ID
	packageID, err := parseSuiPublishOutput(string(publishOutput))
	if err != nil {
		return "", fmt.Errorf("failed to parse publish output: %w", err)
	}

	return packageID, nil
}

// parseSuiPublishOutput parses the JSON output from `sui client publish --json` and extracts the package ID
func parseSuiPublishOutput(output string) (string, error) {
	// The output contains JSON with the transaction result
	// We need to find the published package ID from objectChanges

	var result struct {
		ObjectChanges []struct {
			Type      string `json:"type"`
			PackageId string `json:"packageId"`
		} `json:"objectChanges"`
	}

	// Find JSON in the output (there might be other text before/after)
	jsonStart := strings.Index(output, "{")
	if jsonStart == -1 {
		return "", fmt.Errorf("no JSON found in output: %s", output)
	}

	if err := json.Unmarshal([]byte(output[jsonStart:]), &result); err != nil {
		// Try to extract package ID using regex as fallback
		re := regexp.MustCompile(`"packageId"\s*:\s*"(0x[a-fA-F0-9]+)"`)
		matches := re.FindStringSubmatch(output)
		if len(matches) >= 2 {
			return matches[1], nil
		}
		return "", fmt.Errorf("failed to parse JSON and regex fallback failed: %w", err)
	}

	// Find the published package
	for _, change := range result.ObjectChanges {
		if change.Type == "published" && change.PackageId != "" {
			return change.PackageId, nil
		}
	}

	return "", fmt.Errorf("no published package found in output: %s", output)
}

// =============================================================================
// Eth (Sepolia) DA Constants
// =============================================================================

const (
	// EthSepoliaRPCEndpoint is the public RPC endpoint for Ethereum Sepolia testnet
	EthSepoliaRPCEndpoint = "https://ethereum-sepolia-rpc.publicnode.com"

	// EthSepoliaBeaconAPI is the Beacon API endpoint for Ethereum Sepolia testnet
	EthSepoliaBeaconAPI = "https://ethereum-sepolia-beacon-api.publicnode.com"

	// EthSepoliaNetworkID is the network ID for Ethereum Sepolia testnet
	EthSepoliaNetworkID = 11155111

	// EthAddress is the EVM address derived from DAMnemonic at path m/44'/60'/0'/0/0
	// This is the standard Ethereum BIP44 derivation path
	// Same address is used for Eth and BNB since they share the same derivation path
	EthAddress = "0x8D97689C9818892B700e27F316cc3E41e17fBeb9"
)

// MinEthBalance is the minimum balance required (0.01 ETH = 10^16 wei)
var MinEthBalance = math.NewInt(10_000_000_000_000_000) // 0.01 ETH

// =============================================================================
// BNB (BSC Testnet) DA Constants
// =============================================================================

const (
	// BNBTestnetRPCEndpoint is the public RPC endpoint for BSC Testnet
	BNBTestnetRPCEndpoint = "https://bsc-testnet-rpc.publicnode.com"

	// BNBTestnetNetworkID is the network ID for BSC Testnet
	BNBTestnetNetworkID = 97

	// BNBAddress is the EVM address derived from DAMnemonic at path m/44'/60'/0'/0/0
	// BNB uses the same derivation path as Ethereum for BSC compatibility
	BNBAddress = "0x8D97689C9818892B700e27F316cc3E41e17fBeb9"
)

// MinBNBBalance is the minimum balance required (0.01 BNB = 10^16 wei)
var MinBNBBalance = math.NewInt(10_000_000_000_000_000) // 0.01 BNB

// =============================================================================
// Walrus DA Constants
// =============================================================================

const (
	// WalrusPublisherURL is the public publisher endpoint for Walrus Testnet
	WalrusPublisherURL = "https://publisher.walrus-testnet.walrus.space"

	// WalrusAggregatorURL is the public aggregator endpoint for Walrus Testnet
	WalrusAggregatorURL = "https://aggregator.walrus-testnet.walrus.space"

	// WalrusBlobOwnerAddr is the blob owner address for Walrus
	WalrusBlobOwnerAddr = "0xcc7f20e6ca6d5b9076068bf9b40421218fdf2cfa6316f48c428c8b6716db9c05"

	// WalrusStoreDurationEpochs is the storage duration in epochs
	WalrusStoreDurationEpochs = 53
)

// NOTE: Walrus uses public publisher, no balance check needed

// =============================================================================
// Common Balance Check Helpers
// =============================================================================

// daBalanceCheckParams holds parameters for DA balance check messages
type daBalanceCheckParams struct {
	DAName      string // e.g., "Avail", "Kaspa"
	Network     string // e.g., "Turing Testnet", "Testnet-10"
	Address     string
	FaucetURL   string
	Denom       string // e.g., "AVAIL", "KAS"
	MinRequired string // e.g., "1 AVAIL", "1 KAS"
	Note        string // Additional note about address derivation
}

// fatalBalanceCheckFailed fails the test with a formatted message when balance query fails
func fatalBalanceCheckFailed(t *testing.T, params daBalanceCheckParams, err error) {
	t.Fatalf(`
================================================================================
%s BALANCE CHECK FAILED
================================================================================
Failed to query %s balance: %v

Address: %s

This could mean:
  - The address has never been funded (not found on chain)
  - Network connectivity issues
  - The API is temporarily unavailable

Please fund this address using the %s faucet:
%s

%s
================================================================================
`, params.DAName, params.DAName, err, params.Address, params.Network, params.FaucetURL, params.Note)
}

// fatalInsufficientBalance fails the test with a formatted message when balance is too low
func fatalInsufficientBalance(t *testing.T, params daBalanceCheckParams, currentBalance string) {
	t.Fatalf(`
================================================================================
INSUFFICIENT %s BALANCE ON %s
================================================================================
The %s account has insufficient balance for blob submission.

Address: %s
Current balance: %s %s
Required minimum: %s

Please fund this address using the %s faucet:
%s

%s
================================================================================
`, params.DAName, params.Network, params.DAName, params.Address, currentBalance, params.Denom, params.MinRequired, params.Network, params.FaucetURL, params.Note)
}

// =============================================================================
// Avail Balance Check Functions
// =============================================================================

// checkAvailBalance checks if the Avail account has sufficient balance on Turing testnet.
// If balance is below MinAvailBalance, it fails the test with instructions to fund the address.
func checkAvailBalance(t *testing.T, address string) {
	params := daBalanceCheckParams{
		DAName:      "AVAIL",
		Network:     "Turing Testnet",
		Address:     address,
		FaucetURL:   "https://faucet.avail.tools/",
		Denom:       "AVAIL",
		MinRequired: "1 AVAIL",
		Note:        "NOTE: This address is derived from the test mnemonic and will be the same across all test runs.",
	}

	balance, err := queryAvailTuringBalance(address)
	if err != nil {
		fatalBalanceCheckFailed(t, params, err)
	}

	// Convert to AVAIL for display (18 decimals)
	balanceAVAIL := balance.Quo(math.NewInt(1_000_000_000_000_000_000))

	if balance.LT(MinAvailBalance) {
		fatalInsufficientBalance(t, params, balanceAVAIL.String())
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
	params := daBalanceCheckParams{
		DAName:      "KASPA",
		Network:     "Testnet-10",
		Address:     address,
		FaucetURL:   "https://faucet.kaspanet.io/",
		Denom:       "KAS",
		MinRequired: "1 KAS",
		Note:        "NOTE: This address is derived from the test mnemonic at path m/44'/111111'/0'/0/0\n      and will be the same across all test runs.",
	}

	balance, err := queryKaspaTestnetBalance(address)
	if err != nil {
		fatalBalanceCheckFailed(t, params, err)
	}

	// Convert to KAS for display (8 decimals, 1 KAS = 10^8 Sompi)
	balanceKAS := balance.Quo(math.NewInt(100_000_000))

	if balance.LT(MinKaspaBalance) {
		fatalInsufficientBalance(t, params, balanceKAS.String())
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
// Sui Balance Check Functions
// =============================================================================

// checkSuiBalance checks if the Sui account has sufficient balance on Devnet.
// If balance is below MinSuiBalance, it fails the test with instructions to fund the address.
func checkSuiBalance(t *testing.T, address string) {
	params := daBalanceCheckParams{
		DAName:      "SUI",
		Network:     "Devnet",
		Address:     address,
		FaucetURL:   "https://faucet.sui.io/?network=devnet",
		Denom:       "SUI",
		MinRequired: "0.1 SUI",
		Note:        "NOTE: This address is derived from the test mnemonic using ed25519\n      and will be the same across all test runs.",
	}

	balance, err := querySuiDevnetBalance(address)
	if err != nil {
		fatalBalanceCheckFailed(t, params, err)
	}

	// Convert to SUI for display (9 decimals, 1 SUI = 10^9 MIST)
	balanceSUI := balance.Quo(math.NewInt(1_000_000_000))

	if balance.LT(MinSuiBalance) {
		fatalInsufficientBalance(t, params, balanceSUI.String())
	}

	t.Logf("Sui Devnet balance for %s: %s SUI", address, balanceSUI.String())
}

// querySuiDevnetBalance queries the balance of an address on Sui Devnet using JSON-RPC
func querySuiDevnetBalance(address string) (math.Int, error) {
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "suix_getBalance",
		"params":  []interface{}{address, "0x2::sui::SUI"},
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", SuiDevnetEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to query Sui RPC: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Result struct {
			TotalBalance string `json:"totalBalance"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return math.Int{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != nil {
		return math.Int{}, fmt.Errorf("Sui RPC error: %s", result.Error.Message)
	}

	if result.Result.TotalBalance == "" {
		return math.ZeroInt(), nil
	}

	balance, ok := math.NewIntFromString(result.Result.TotalBalance)
	if !ok {
		return math.Int{}, fmt.Errorf("failed to parse balance: %s", result.Result.TotalBalance)
	}

	return balance, nil
}

// =============================================================================
// Eth (Sepolia) Balance Check Functions
// =============================================================================

// checkEthBalance checks if the Eth account has sufficient balance on Sepolia testnet.
// If balance is below MinEthBalance, it fails the test with instructions to fund the address.
func checkEthBalance(t *testing.T, address string) {
	params := daBalanceCheckParams{
		DAName:      "ETH",
		Network:     "Sepolia Testnet",
		Address:     address,
		FaucetURL:   "https://sepoliafaucet.com/",
		Denom:       "ETH",
		MinRequired: "0.01 ETH",
		Note:        "NOTE: This address is derived from the test mnemonic at path m/44'/60'/0'/0/0\n      and will be the same across all test runs.",
	}

	balance, err := queryEthSepoliaBalance(address)
	if err != nil {
		fatalBalanceCheckFailed(t, params, err)
	}

	// Convert to ETH for display (18 decimals)
	balanceETH := new(big.Float).Quo(
		new(big.Float).SetInt(balance.BigInt()),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
	)

	if balance.LT(MinEthBalance) {
		fatalInsufficientBalance(t, params, balanceETH.Text('f', 6))
	}

	t.Logf("Eth Sepolia balance for %s: %s ETH", address, balanceETH.Text('f', 6))
}

// queryEthSepoliaBalance queries the balance of an address on Ethereum Sepolia using JSON-RPC
func queryEthSepoliaBalance(address string) (math.Int, error) {
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getBalance",
		"params":  []interface{}{address, "latest"},
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", EthSepoliaRPCEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to query Eth RPC: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return math.Int{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != nil {
		return math.Int{}, fmt.Errorf("Eth RPC error: %s", result.Error.Message)
	}

	// Balance is returned as hex string (e.g., "0x123")
	if result.Result == "" || result.Result == "0x0" {
		return math.ZeroInt(), nil
	}

	// Remove "0x" prefix and parse hex
	balanceHex := result.Result
	if len(balanceHex) > 2 && balanceHex[:2] == "0x" {
		balanceHex = balanceHex[2:]
	}

	balanceInt := new(big.Int)
	balanceInt.SetString(balanceHex, 16)

	return math.NewIntFromBigInt(balanceInt), nil
}

// =============================================================================
// BNB (BSC Testnet) Balance Check Functions
// =============================================================================

// checkBNBBalance checks if the BNB account has sufficient balance on BSC Testnet.
// If balance is below MinBNBBalance, it fails the test with instructions to fund the address.
func checkBNBBalance(t *testing.T, address string) {
	params := daBalanceCheckParams{
		DAName:      "BNB",
		Network:     "BSC Testnet",
		Address:     address,
		FaucetURL:   "https://www.bnbchain.org/en/testnet-faucet",
		Denom:       "BNB",
		MinRequired: "0.01 BNB",
		Note:        "NOTE: This address is derived from the test mnemonic at path m/44'/60'/0'/0/0\n      and will be the same across all test runs.",
	}

	balance, err := queryBNBTestnetBalance(address)
	if err != nil {
		fatalBalanceCheckFailed(t, params, err)
	}

	// Convert to BNB for display (18 decimals)
	balanceBNB := new(big.Float).Quo(
		new(big.Float).SetInt(balance.BigInt()),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
	)

	if balance.LT(MinBNBBalance) {
		fatalInsufficientBalance(t, params, balanceBNB.Text('f', 6))
	}

	t.Logf("BNB BSC Testnet balance for %s: %s BNB", address, balanceBNB.Text('f', 6))
}

// queryBNBTestnetBalance queries the balance of an address on BSC Testnet using JSON-RPC
func queryBNBTestnetBalance(address string) (math.Int, error) {
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getBalance",
		"params":  []interface{}{address, "latest"},
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", BNBTestnetRPCEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to query BNB RPC: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return math.Int{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != nil {
		return math.Int{}, fmt.Errorf("BNB RPC error: %s", result.Error.Message)
	}

	// Balance is returned as hex string (e.g., "0x123")
	if result.Result == "" || result.Result == "0x0" {
		return math.ZeroInt(), nil
	}

	// Remove "0x" prefix and parse hex
	balanceHex := result.Result
	if len(balanceHex) > 2 && balanceHex[:2] == "0x" {
		balanceHex = balanceHex[2:]
	}

	balanceInt := new(big.Int)
	balanceInt.SetString(balanceHex, 16)

	return math.NewIntFromBigInt(balanceInt), nil
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
		AvailTuringRPCEndpoint, AvailAppID, DAMnemonic)}

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
		KaspaTestnetAPIURL, KaspaTestnetEndpoint, KaspaNetworkID, DAMnemonic)}

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

// =============================================================================
// Sui DA Test
// =============================================================================

// TestFullnodeSync_Sui_EVM tests the synchronization of a fullnode using Sui as DA.
// This test submits batches to Sui and verifies the fullnode can sync from DA.
func TestFullnodeSync_Sui_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Check Sui balance before starting the test
	t.Logf("Checking Sui balance for address: %s", SuiAddress)
	checkSuiBalance(t, SuiAddress)

	// Ensure noop contract exists, deploy if needed
	suiContractAddress := ensureSuiNoopContract(t)

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "30s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	// Sui has a smaller blob size limit (~96KB), so we need to limit batch size
	dymintTomlOverrides["batch_submit_bytes"] = 90000

	// Sui DA configuration
	da_config := []string{fmt.Sprintf(`{"endpoint": "%s", "noop_contract_address": "%s", "gas_budget": "%s", "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
		SuiDevnetEndpoint, suiContractAddress, SuiGasBudget, DAMnemonic)}

	dymintTomlOverrides["da_layer"] = []string{"sui"}
	dymintTomlOverrides["da_config"] = da_config

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "sui",
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

	t.Log("Chains started, waiting for rollapp to produce blocks and submit batches to Sui...")

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
	t.Logf("Fullnode successfully synced to height %d using Sui DA", finalHeight)
}

// =============================================================================
// Eth (Sepolia) DA Test
// =============================================================================

// TestFullnodeSync_Eth_EVM tests the synchronization of a fullnode using Ethereum (Sepolia) as DA.
// This test submits batches to Ethereum Sepolia and verifies the fullnode can sync from DA.
func TestFullnodeSync_Eth_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Check Eth balance before starting the test
	t.Logf("Checking Eth balance for address: %s", EthAddress)
	checkEthBalance(t, EthAddress)

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "30s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	// Eth has a blob size limit of ~130KB, so we need to limit batch size
	dymintTomlOverrides["batch_submit_bytes"] = 120000

	// Eth DA configuration
	da_config := []string{fmt.Sprintf(`{"endpoint": "%s", "network_id": %d, "api_url": "%s", "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
		EthSepoliaRPCEndpoint, EthSepoliaNetworkID, EthSepoliaBeaconAPI, DAMnemonic)}

	dymintTomlOverrides["da_layer"] = []string{"eth"}
	dymintTomlOverrides["da_config"] = da_config

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "eth",
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

	t.Log("Chains started, waiting for rollapp to produce blocks and submit batches to Eth Sepolia...")

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
	t.Logf("Fullnode successfully synced to height %d using Eth Sepolia DA", finalHeight)
}

// =============================================================================
// BNB (BSC Testnet) DA Test
// =============================================================================

// TestFullnodeSync_BNB_EVM tests the synchronization of a fullnode using BNB (BSC Testnet) as DA.
// This test submits batches to BNB BSC Testnet and verifies the fullnode can sync from DA.
func TestFullnodeSync_BNB_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Check BNB balance before starting the test
	t.Logf("Checking BNB balance for address: %s", BNBAddress)
	checkBNBBalance(t, BNBAddress)

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "30s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	// BNB has a blob size limit of ~130KB, so we need to limit batch size
	dymintTomlOverrides["batch_submit_bytes"] = 120000

	// BNB DA configuration
	da_config := []string{fmt.Sprintf(`{"endpoint": "%s", "network_id": %d, "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
		BNBTestnetRPCEndpoint, BNBTestnetNetworkID, DAMnemonic)}

	dymintTomlOverrides["da_layer"] = []string{"bnb"}
	dymintTomlOverrides["da_config"] = da_config

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "bnb",
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

	t.Log("Chains started, waiting for rollapp to produce blocks and submit batches to BNB BSC Testnet...")

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
	t.Logf("Fullnode successfully synced to height %d using BNB BSC Testnet DA", finalHeight)
}

// =============================================================================
// Walrus DA Test
// =============================================================================

// TestFullnodeSync_Walrus_EVM tests the synchronization of a fullnode using Walrus as DA.
// This test submits batches to Walrus and verifies the fullnode can sync from DA.
// Note: Walrus uses a public publisher, so no balance check is needed.
func TestFullnodeSync_Walrus_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Note: Walrus uses a public publisher, no balance check needed
	t.Log("Walrus uses public publisher - no balance check required")

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "30s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	// Walrus DA configuration
	// Note: Walrus uses public publisher, no key needed
	da_config := []string{fmt.Sprintf(`{"publisher_url": "%s", "aggregator_url": "%s", "blob_owner_addr": "%s", "store_duration_epochs": %d, "timeout": 300000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
		WalrusPublisherURL, WalrusAggregatorURL, WalrusBlobOwnerAddr, WalrusStoreDurationEpochs)}

	dymintTomlOverrides["da_layer"] = []string{"walrus"}
	dymintTomlOverrides["da_config"] = da_config

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: "walrus",
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

	t.Log("Chains started, waiting for rollapp to produce blocks and submit batches to Walrus...")

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
	t.Logf("Fullnode successfully synced to height %d using Walrus DA", finalHeight)
}
