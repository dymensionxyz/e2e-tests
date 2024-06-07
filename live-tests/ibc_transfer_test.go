package livetests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func TestIBCTransfer_Live(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()

	hub := cosmos.CosmosChain{
		RPCAddr:       "rpc-blumbus.mzonder.com:443",
		GrpcAddr:      "grpc-blumbus.mzonder.com:9090",
		ChainID:       "blumbus_111-1",
		Bin:           "dymd",
		GasPrices:     "1000adym",
		GasAdjustment: "1.1",
		Denom:         "adym",
	}

	rollappX := cosmos.CosmosChain{
		RPCAddr:       "rpc.rolxtwo.evm.ra.blumbus.noisnemyd.xyz:443",
		GrpcAddr:      "3.123.185.77:9090",
		ChainID:       "rolx_100004-1",
		Bin:           "rollapp-evm",
		GasPrices:     "0.0arolx",
		GasAdjustment: "1.1",
		Denom:         "arolx",
	}

	rollappY := cosmos.CosmosChain{
		RPCAddr:       "rpc.roly.wasm.ra.blumbus.noisnemyd.xyz:443",
		GrpcAddr:      "18.153.150.111:9090",
		ChainID:       "rollappy_700002-1",
		Bin:           "rollapp-wasm",
		GasPrices:     "0.0aroly",
		GasAdjustment: "1.1",
		Denom:         "aroly",
	}

	dymensionUser, err := hub.CreateUser("dym1")
	require.NoError(t, err)
	rollappXUser, err := rollappX.CreateUser("rolx1")
	require.NoError(t, err)
	rollappYUser, err := rollappY.CreateUser("roly1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)

	err = rollappY.NewClient("https://" + rollappY.RPCAddr)
	require.NoError(t, err)

	// Data to send in the POST request
	data := map[string]string{
		"address": rollappXUser.Address,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	// Create a new POST request
	req, err := http.NewRequest("POST", "http://18.184.170.181:3000/api/get-rollx", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// Set the request header to indicate that we're sending JSON data
	req.Header.Set("Content-Type", "application/json")

	// Create an HTTP client and send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}

	fmt.Println("Response Status:", resp.Status)
	fmt.Println("Response Body:", string(body))

	// Data to send in the POST request
	data = map[string]string{
		"address": rollappYUser.Address,
	}
	jsonData, err = json.Marshal(data)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	// Create a new POST request
	req, err = http.NewRequest("POST", "http://18.184.170.181:3000/api/get-rolly", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// Set the request header to indicate that we're sending JSON data
	req.Header.Set("Content-Type", "application/json")

	// Create an HTTP client and send the request
	client = &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	// Read the response
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}

	fmt.Println("Response Status:", resp.Status)
	fmt.Println("Response Body:", string(body))

	// Data to send in the POST request
	data = map[string]string{
		"address": dymensionUser.Address,
	}
	jsonData, err = json.Marshal(data)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	// Create a new POST request
	req, err = http.NewRequest("POST", "http://18.184.170.181:3000/api/get-dym", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// Set the request header to indicate that we're sending JSON data
	req.Header.Set("Content-Type", "application/json")

	// Create an HTTP client and send the request
	client = &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	// Read the response
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}

	fmt.Println("Response Status:", resp.Status)
	fmt.Println("Response Body:", string(body))

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	rollappTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	hubTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDRollappXDym, dymensionUser.Denom)
	hubIBCDenom := transfertypes.ParseDenomTrace(hubTokenDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)

	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappXOrigBal)

	rollappYOrigBal, err := rollappYUser.GetBalance(ctx, rollappYUser.Denom, rollappY.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappYOrigBal)

	erc20_OrigBal, err := GetERC20Balance(ctx, erc20IBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(erc20_OrigBal)

	transferAmount := math.NewInt(1_000_000)

	// Compose an IBC transfer and send from dymension -> rollapp
	transferData := ibc.WalletData{
		Address: rollappXUser.Address,
		Denom:   dymensionUser.Denom,
		Amount:  transferAmount,
	}

	testutil.WaitForBlocks(ctx, 3, hub)

	cosmos.SendIBCTransfer(hub, channelIDDymRollappX, dymensionUser.Address, transferData, dymFee, ibc.TransferOptions{})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 3, hub)
	// Compose an IBC transfer and send from rollapp -> hub
	transferData = ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}

	multiplier := math.NewInt(10)
	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	var options ibc.TransferOptions
	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, options)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, hub)

	erc20_Bal, err := GetERC20Balance(ctx, hubIBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(erc20_Bal)
	fmt.Println(rollappIBCDenom)
	testutil.AssertBalance(t, ctx, dymensionUser, rollappIBCDenom, hub.GrpcAddr, transferAmount.Sub(eibcFee))
	require.Equal(t, erc20_OrigBal.Add(transferAmount), erc20_Bal)
}
