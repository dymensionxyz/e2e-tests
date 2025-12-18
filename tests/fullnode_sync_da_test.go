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
// Common DA Constants
// =============================================================================

const (
	// DAMnemonic is a deterministic mnemonic used for all DA accounts in tests.
	// Each DA derives its address using its own derivation path and key type.
	DAMnemonic = "toward biology settle legend tuition disease shrimp loyal universe crop pen cement"
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
	AvailAddress = "5F9WpKRbYCMfaQHiJZF8XMKmt9E7pnSV8qBW48EoH6kJMTAr"
)

// MinAvailBalance is the minimum balance required (1 AVAIL = 10^18 base units)
var MinAvailBalance = math.NewInt(1_000_000_000_000_000_000) // 1 AVAIL

// =============================================================================
// Kaspa DA Constants
// =============================================================================

const (
	// KaspaTestnetAPIURL is the REST API endpoint for Kaspa Testnet-10
	KaspaTestnetAPIURL = "https://rest.tn.kaspa.rollapp.network"

	// KaspaTestnetEndpoint is the gRPC endpoint for Kaspa Testnet-10
	KaspaTestnetEndpoint = "val.rpc.tn.kaspa.rollapp.network:16210"

	// KaspaNetworkID is the network identifier for Kaspa Testnet-10
	KaspaNetworkID = "kaspa-testnet-10"

	// KaspaAddress is the address derived from DAMnemonic at path m/44'/111111'/0'/0/0
	// Derived using Kaspa BIP44 derivation with testnet parameters
	KaspaAddress = "kaspatest:qzhuvrnn52zudqgqzwjzv87a9w5xrf8tcz5ewfg75r4nqdcemfd4j2mhhhelx"
)

// MinKaspaBalance is the minimum balance required (1 KAS = 10^8 Sompi)
var MinKaspaBalance = math.NewInt(100_000_000) // 1 KAS

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
	EthAddress = "0xc51Fe2bD5e3bE44AE593a62933948672a7F91Ea3"
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
	BNBAddress = "0xc51Fe2bD5e3bE44AE593a62933948672a7F91Ea3"
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
// Aptos DA Constants
// =============================================================================

const (
	// AptosTestnetEndpoint is the RPC endpoint for Aptos Testnet
	AptosTestnetEndpoint = "https://fullnode.testnet.aptoslabs.com/v1"

	// AptosNetworkID is the network identifier for Aptos Testnet
	AptosNetworkID = "testnet"

	// AptosPrivateKey is the private key for the test account (ed25519)
	AptosPrivateKey = "0x6605eb1d2dfd95dfe21135f4cf76c2e4e8b8a2822b081a746e129058b99af893"

	// AptosAddress is the address derived from AptosPrivateKey
	AptosAddress = "0x053456e2b7eb076a8bb2c90ce593802adddc27220b69601e22cd7e4eb94ecb17"
)

// MinAptosBalance is the minimum balance required (0.01 APT = 10^6 Octas)
var MinAptosBalance = math.NewInt(1_000_000) // 0.01 APT

// =============================================================================
// Solana DA Constants
// =============================================================================

const (
	// SolanaDevnetEndpoint is the RPC endpoint for Solana Devnet
	SolanaDevnetEndpoint = "https://api.devnet.solana.com"

	// SolanaProgramAddress is the program address for DA storage on Solana
	SolanaProgramAddress = "5cfjxBnFMoqdbZXTMHaoXfQm7obMpYMnkT681sRd95Qo"

	// SolanaPrivateKey is the base58-encoded keypair for the test account
	SolanaPrivateKey = "cPBSvzVbMGrfZ1wmJD2Nc8P8V3thhMsJMFV7mfRRYPkAMuLRx2qnjsAU6J8WJ7svR9pJFoJesD7AQyu41AAnK9f"

	// SolanaAddress is the address derived from SolanaPrivateKey
	SolanaAddress = "BebGGyp4CnWW4GhzetAAYg3gfBgvyNDEe1tAoynXyrRb"
)

// MinSolanaBalance is the minimum balance required (0.1 SOL = 10^8 lamports)
var MinSolanaBalance = math.NewInt(100_000_000) // 0.1 SOL

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
// Aptos Balance Check Functions
// =============================================================================

// checkAptosBalance checks if the Aptos account has sufficient balance on Testnet.
func checkAptosBalance(t *testing.T, address string) {
	params := daBalanceCheckParams{
		DAName:      "APTOS",
		Network:     "Testnet",
		Address:     address,
		FaucetURL:   "https://aptoslabs.com/testnet-faucet",
		Denom:       "APT",
		MinRequired: "0.01 APT",
		Note:        "NOTE: This address is derived from the test private key.",
	}

	balance, err := queryAptosTestnetBalance(address)
	if err != nil {
		fatalBalanceCheckFailed(t, params, err)
	}

	// Convert to APT for display (8 decimals, 1 APT = 10^8 Octas)
	balanceAPT := balance.Quo(math.NewInt(100_000_000))

	if balance.LT(MinAptosBalance) {
		fatalInsufficientBalance(t, params, balanceAPT.String())
	}

	t.Logf("Aptos Testnet balance for %s: %s APT", address, balanceAPT.String())
}

// queryAptosTestnetBalance queries the balance of an address on Aptos Testnet
func queryAptosTestnetBalance(address string) (math.Int, error) {
	url := fmt.Sprintf("%s/accounts/%s/resources", AptosTestnetEndpoint, address)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to query Aptos API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == 404 {
		return math.ZeroInt(), nil // Account not found = zero balance
	}

	if resp.StatusCode != 200 {
		return math.Int{}, fmt.Errorf("Aptos API error: %s", string(body))
	}

	var resources []struct {
		Type string `json:"type"`
		Data struct {
			Coin struct {
				Value string `json:"value"`
			} `json:"coin"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resources); err != nil {
		return math.Int{}, fmt.Errorf("failed to parse response: %w", err)
	}

	// Find the APT coin resource
	for _, r := range resources {
		if r.Type == "0x1::coin::CoinStore<0x1::aptos_coin::AptosCoin>" {
			balance, ok := math.NewIntFromString(r.Data.Coin.Value)
			if !ok {
				return math.Int{}, fmt.Errorf("failed to parse balance: %s", r.Data.Coin.Value)
			}
			return balance, nil
		}
	}

	return math.ZeroInt(), nil
}

// =============================================================================
// Solana Balance Check Functions
// =============================================================================

// checkSolanaBalance checks if the Solana account has sufficient balance on Devnet.
func checkSolanaBalance(t *testing.T, address string) {
	params := daBalanceCheckParams{
		DAName:      "SOLANA",
		Network:     "Devnet",
		Address:     address,
		FaucetURL:   "https://faucet.solana.com/",
		Denom:       "SOL",
		MinRequired: "0.1 SOL",
		Note:        "NOTE: This address is derived from the test private key.",
	}

	balance, err := querySolanaDevnetBalance(address)
	if err != nil {
		fatalBalanceCheckFailed(t, params, err)
	}

	// Convert to SOL for display (9 decimals, 1 SOL = 10^9 lamports)
	balanceSOL := balance.Quo(math.NewInt(1_000_000_000))

	if balance.LT(MinSolanaBalance) {
		fatalInsufficientBalance(t, params, balanceSOL.String())
	}

	t.Logf("Solana Devnet balance for %s: %s SOL", address, balanceSOL.String())
}

// querySolanaDevnetBalance queries the balance of an address on Solana Devnet
func querySolanaDevnetBalance(address string) (math.Int, error) {
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getBalance",
		"params":  []interface{}{address},
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", SolanaDevnetEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to query Solana RPC: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return math.Int{}, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Result struct {
			Value uint64 `json:"value"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return math.Int{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != nil {
		return math.Int{}, fmt.Errorf("Solana RPC error: %s", result.Error.Message)
	}

	return math.NewIntFromUint64(result.Result.Value), nil
}

// =============================================================================
// DA Test Configuration and Common Helper
// =============================================================================

// daTestConfig holds the configuration for a DA fullnode sync test
type daTestConfig struct {
	DALayer         string            // DA layer name (e.g., "avail", "kaspa")
	DAConfig        string            // DA-specific JSON configuration
	BatchSubmitBytes int              // Optional batch size limit (0 = use default)
	BalanceCheck    func(t *testing.T) // Optional balance check function (nil = skip)
}

// runFullnodeSyncDATest runs a fullnode sync test with the given DA configuration
func runFullnodeSyncDATest(t *testing.T, cfg daTestConfig) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// Run balance check if provided
	if cfg.BalanceCheck != nil {
		cfg.BalanceCheck(t)
	}

	// Build dymint TOML overrides
	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappevm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "30s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"

	if cfg.BatchSubmitBytes > 0 {
		dymintTomlOverrides["batch_submit_bytes"] = cfg.BatchSubmitBytes
	}

	dymintTomlOverrides["da_layer"] = []string{cfg.DALayer}
	dymintTomlOverrides["da_config"] = []string{cfg.DAConfig}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	modifyEVMGenesisKV := append(
		rollappEVMGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollappparams.params.da",
			Value: cfg.DALayer,
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

	t.Logf("Chains started, waiting for rollapp to produce blocks and submit batches to %s...", cfg.DALayer)

	// Wait for enough blocks to ensure multiple batches are submitted
	targetBlocks := 50
	err = testutil.WaitForBlocks(ctx, targetBlocks, rollapp1)
	require.NoError(t, err)

	// Query the hub for the last submitted state
	rollappState, err := dymension.QueryRollappState(ctx, rollapp1.GetChainID(), false)
	require.NoError(t, err)

	// Calculate the last submitted height
	startHeight, err := strconv.ParseInt(rollappState.StateInfo.StartHeight, 10, 64)
	require.NoError(t, err)
	numBlocks, err := strconv.ParseInt(rollappState.StateInfo.NumBlocks, 10, 64)
	require.NoError(t, err)
	targetHeight := startHeight + numBlocks - 1
	t.Logf("Last submitted state: StartHeight=%d, NumBlocks=%d, TargetHeight=%d", startHeight, numBlocks, targetHeight)

	// Stop the sequencer
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
			return fullnodeHeight >= targetHeight, nil
		},
	)
	require.NoError(t, err)

	finalHeight, err := rollapp1.FullNodes[0].Height(ctx)
	require.NoError(t, err)
	t.Logf("Fullnode successfully synced to height %d using %s DA", finalHeight, cfg.DALayer)
}

// =============================================================================
// DA Tests
// =============================================================================

func TestFullnodeSync_Avail_EVM(t *testing.T) {
	runFullnodeSyncDATest(t, daTestConfig{
		DALayer: "avail",
		DAConfig: fmt.Sprintf(`{"endpoint": "%s", "app_id": %d, "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
			AvailTuringRPCEndpoint, AvailAppID, DAMnemonic),
		BalanceCheck: func(t *testing.T) {
			t.Logf("Checking Avail balance for address: %s", AvailAddress)
			checkAvailBalance(t, AvailAddress)
		},
	})
}

func TestFullnodeSync_Kaspa_EVM(t *testing.T) {
	runFullnodeSyncDATest(t, daTestConfig{
		DALayer: "kaspa",
		DAConfig: fmt.Sprintf(`{"api_url": "%s", "endpoint": "%s", "network_id": "%s", "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
			KaspaTestnetAPIURL, KaspaTestnetEndpoint, KaspaNetworkID, DAMnemonic),
		BalanceCheck: func(t *testing.T) {
			t.Logf("Checking Kaspa balance for address: %s", KaspaAddress)
			checkKaspaBalance(t, KaspaAddress)
		},
	})
}

func TestFullnodeSync_Eth_EVM(t *testing.T) {
	runFullnodeSyncDATest(t, daTestConfig{
		DALayer: "eth",
		DAConfig: fmt.Sprintf(`{"endpoint": "%s", "network_id": %d, "api_url": "%s", "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
			EthSepoliaRPCEndpoint, EthSepoliaNetworkID, EthSepoliaBeaconAPI, DAMnemonic),
		BatchSubmitBytes: 120000,
		BalanceCheck: func(t *testing.T) {
			t.Logf("Checking Eth balance for address: %s", EthAddress)
			checkEthBalance(t, EthAddress)
		},
	})
}

func TestFullnodeSync_BNB_EVM(t *testing.T) {
	runFullnodeSyncDATest(t, daTestConfig{
		DALayer: "bnb",
		DAConfig: fmt.Sprintf(`{"endpoint": "%s", "network_id": %d, "mnemonic": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
			BNBTestnetRPCEndpoint, BNBTestnetNetworkID, DAMnemonic),
		BatchSubmitBytes: 120000,
		BalanceCheck: func(t *testing.T) {
			t.Logf("Checking BNB balance for address: %s", BNBAddress)
			checkBNBBalance(t, BNBAddress)
		},
	})
}

func TestFullnodeSync_Walrus_EVM(t *testing.T) {
	runFullnodeSyncDATest(t, daTestConfig{
		DALayer: "walrus",
		DAConfig: fmt.Sprintf(`{"publisher_url": "%s", "aggregator_url": "%s", "blob_owner_addr": "%s", "store_duration_epochs": %d, "timeout": 300000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
			WalrusPublisherURL, WalrusAggregatorURL, WalrusBlobOwnerAddr, WalrusStoreDurationEpochs),
		BalanceCheck: func(t *testing.T) {
			t.Log("Walrus uses public publisher - no balance check required")
		},
	})
}

func TestFullnodeSync_Aptos_EVM(t *testing.T) {
	runFullnodeSyncDATest(t, daTestConfig{
		DALayer: "aptos",
		DAConfig: fmt.Sprintf(`{"network_id": "%s", "private_key": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
			AptosNetworkID, AptosPrivateKey),
		BatchSubmitBytes: 60000, // Aptos has 64KB limit
		BalanceCheck: func(t *testing.T) {
			t.Logf("Checking Aptos balance for address: %s", AptosAddress)
			checkAptosBalance(t, AptosAddress)
		},
	})
}

func TestFullnodeSync_Solana_EVM(t *testing.T) {
	runFullnodeSyncDATest(t, daTestConfig{
		DALayer: "solana",
		DAConfig: fmt.Sprintf(`{"endpoint": "%s", "program_address": "%s", "private_key": "%s", "timeout": 60000000000, "retry_attempts": 4, "retry_delay": 3000000000}`,
			SolanaDevnetEndpoint, SolanaProgramAddress, SolanaPrivateKey),
		BalanceCheck: func(t *testing.T) {
			t.Logf("Checking Solana balance for address: %s", SolanaAddress)
			checkSolanaBalance(t, SolanaAddress)
		},
	})
}
