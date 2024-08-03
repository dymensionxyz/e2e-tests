package livetests

import (
	"context"
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func TestEIBC_Invalid_Fee_RolX_Live(t *testing.T) {
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

	dymensionUser, err := hub.CreateUser("dym1")
	require.NoError(t, err)
	rollappXUser, err := rollappX.CreateUser("rolx1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	rollappXTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	rollappXIBCDenom := transfertypes.ParseDenomTrace(rollappXTokenDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)

	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappXOrigBal)

	var options ibc.TransferOptions

	multiplier := sdkmath.NewInt(10)
	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	options.Memo = BuildEIbcMemo(eibcFee)
	transferData := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, options)

	invalidMemo2 := `{"eibc": {"feebaba": "200"}}`
	options.Memo = invalidMemo2
	transferData = ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, options)
	require.NoError(t, err)

	invalidMemo3 := `{"eibc": {"fee": "this-should-be-number"}}`
	options.Memo = invalidMemo3
	transferData = ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, options)
	// get eIbc event

	rollappXHeight, err := rollappX.Height(ctx)
	require.NoError(t, err)
	fmt.Println(rollappXHeight)

	encoding := encodingConfig()
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, &hub, 20, false, encoding.InterfaceRegistry)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}
	require.True(t, len(eibcEvents) == 1) // verify 1 EIBC event was registered on the hub

	// wait until the packet is finalized on Rollapp 1
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappX.ChainID, rollappXHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, transferAmount)
}

func TestEIBCFeeBgtAmountRolX_Live(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()

	// Configuration for live network
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

	dymensionUser, err := hub.CreateUser("dym1")
	require.NoError(t, err)
	rollappXUser, err := rollappX.CreateUser("rolx1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("hub original balance: ", dymensionOrigBal, dymensionUser.Denom)

	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("rollapp x original balance: ", rollappXOrigBal, rollappXUser.Denom)

	erc20_OrigBal, err := GetERC20Balance(ctx, erc20IBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("rol x user balance of denom send from dym: ", erc20_OrigBal)

	var options ibc.TransferOptions

	// create eIBC memo with fee 2 times the transfer amount
	multiplier := sdkmath.NewInt(2)
	eibcFee := transferAmount.Mul(multiplier) // example transfer amount * 0.1

	// Compose an IBC transfer and send from rollappX -> hub
	// Send IBC transfer with corrupted memo 1
	options.Memo = BuildEIbcMemo(eibcFee)
	transferData := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappX.Denom,
		Amount:  transferAmount,
	}
	_, err = cosmos.SendIBCTransfer(rollappX, channelIDDymRollappX, rollappXUser.Address, transferData, rolxFee, options)
	require.Error(t, err)

	// wait until the packet is finalized on Rollapp 1
	rollappXHeight, err := rollappX.Height(ctx)
	require.NoError(t, err)
	fmt.Println(rollappXHeight)
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappX.ChainID, rollappXHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Verify that the amount is returned to the sender
	testutil.AssertBalance(t, ctx, dymensionUser, dymensionUser.Denom, hub.GrpcAddr, dymensionOrigBal)
	// Verify that the amount is not received by the receiver
	testutil.AssertBalance(t, ctx, rollappXUser, rollappX.Denom, rollappX.GrpcAddr, rollappXOrigBal)
}

func TestEIBCFeeBgtAmountRolY_Live(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()

	// Configuration for live network
	hub := cosmos.CosmosChain{
		RPCAddr:       "rpc-blumbus.mzonder.com:443",
		GrpcAddr:      "grpc-blumbus.mzonder.com:9090",
		ChainID:       "blumbus_111-1",
		Bin:           "dymd",
		GasPrices:     "1000adym",
		GasAdjustment: "1.1",
		Denom:         "adym",
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
	rollappYUser, err := rollappY.CreateUser("roly1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = rollappY.NewClient("https://" + rollappY.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	rollappYUser.GetFaucet("http://18.184.170.181:3000/api/get-rolly")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("hub original balance: ", dymensionOrigBal, dymensionUser.Denom)

	rollappYOrigBal, err := rollappYUser.GetBalance(ctx, rollappYUser.Denom, rollappY.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("rollapp y original balance: ", rollappYOrigBal, rollappYUser.Denom)

	var options ibc.TransferOptions

	// create eIBC memo with fee 2 times the transfer amount
	multiplier := sdkmath.NewInt(2)
	eibcFee := transferAmount.Mul(multiplier) // example transfer amount * 0.1

	// Send IBC transfer with corrupted memo 1
	options.Memo = BuildEIbcMemo(eibcFee)

	// Compose in IBC transfer and send from rollappY -> hub
	transferData := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappY.Denom,
		Amount:  transferAmount,
	}
	_, err = cosmos.SendIBCTransfer(rollappY, channelIDDymRollappY, rollappYUser.Address, transferData, rolyFee, options)
	require.Error(t, err)

	// wait until the packet is finalized on Rollapp 1
	rollappYHeight, err := rollappY.Height(ctx)
	require.NoError(t, err)
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappY.ChainID, rollappYHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Verify that the amount is returned to the sender
	testutil.AssertBalance(t, ctx, dymensionUser, dymensionUser.Denom, hub.GrpcAddr, dymensionOrigBal)
}
