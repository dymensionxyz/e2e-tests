package livetests

import (
	"context"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func TestEIBCTimeoutRolX_Live(t *testing.T) {
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
		JsonRPCAddr:   "https://json-rpc.rolxtwo.evm.ra.blumbus.noisnemyd.xyz",
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

	height, err := rollappX.Height(ctx)
	require.NoError(t, err)
	erc20_OrigBal, err := rollappXUser.GetERC20Balance(rollappX.JsonRPCAddr, erc20Contract, int64(height))
	require.NoError(t, err)
	fmt.Println(erc20_OrigBal)

	// Compose an IBC transfer and send from dymension -> rollapp
	transferData := ibc.WalletData{
		Address: rollappXUser.Address,
		Denom:   dymensionUser.Denom,
		Amount:  transferAmount,
	}

	testutil.WaitForBlocks(ctx, 3, hub)

	// Set a short timeout for IBC transfer
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 1000000, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	cosmos.SendIBCTransfer(hub, channelIDDymRollappX, dymensionUser.Address, transferData, dymFee, options)
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

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, options)
	require.NoError(t, err)

	rollappXHeight, err := rollappX.Height(ctx)
	require.NoError(t, err)

	fmt.Println(rollappXHeight)
	// wait until the packet is finalized on Rollapp 1
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappX.ChainID, rollappXHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	height, err = rollappX.Height(ctx)
	require.NoError(t, err)
	erc20_Bal, err := rollappXUser.GetERC20Balance(rollappX.JsonRPCAddr, erc20Contract, int64(height))
	require.NoError(t, err)
	fmt.Println(erc20_Bal)
	require.Equal(t, erc20_OrigBal, erc20_Bal)
	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, math.ZeroInt())
}

func TestEIBCTimeoutRolY_Live(t *testing.T) {
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

	// Get the IBC denom
	rollappYTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappY, rollappYUser.Denom)
	rollappYIBCDenom := transfertypes.ParseDenomTrace(rollappYTokenDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)

	rollappYOrigBal, err := rollappYUser.GetBalance(ctx, rollappYUser.Denom, rollappY.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappYOrigBal)

	height, err := rollappY.Height(ctx)
	require.NoError(t, err)
	erc20_OrigBal, err := rollappYUser.GetERC20Balance(rollappY.JsonRPCAddr, erc20Contract, int64(height))
	require.NoError(t, err)
	fmt.Println(erc20_OrigBal)

	// Set a short timeout for IBC transfer
	options := ibc.TransferOptions{
		Timeout: &ibc.IBCTimeout{
			NanoSeconds: 1000000, // 1 ms - this will cause the transfer to timeout before it is picked by a relayer
		},
	}

	// Compose an IBC transfer and send from dymension -> rollapp
	transferData := ibc.WalletData{
		Address: rollappYUser.Address,
		Denom:   dymensionUser.Denom,
		Amount:  transferAmount,
	}

	cosmos.SendIBCTransfer(hub, channelIDDymRollappY, dymensionUser.Address, transferData, dymFee, options)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 3, hub)

	// Compose an IBC transfer and send from rollapp -> hub
	transferData = ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappYUser.Denom,
		Amount:  transferAmount,
	}

	cosmos.SendIBCTransfer(rollappY, channelIDRollappYDym, rollappYUser.Address, transferData, rolyFee, options)
	require.NoError(t, err)

	rollappYHeight, err := rollappY.Height(ctx)
	require.NoError(t, err)

	fmt.Println(rollappYHeight)
	// wait until the packet is finalized on Rollapp 1
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappY.ChainID, rollappYHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	height, err = rollappY.Height(ctx)
	require.NoError(t, err)
	erc20_Bal, err := rollappYUser.GetERC20Balance(rollappY.JsonRPCAddr, erc20Contract, int64(height))
	require.NoError(t, err)
	fmt.Println(erc20_Bal)
	require.Equal(t, erc20_OrigBal, erc20_Bal)

	testutil.AssertBalance(t, ctx, dymensionUser, rollappYIBCDenom, hub.GrpcAddr, math.ZeroInt())
}
