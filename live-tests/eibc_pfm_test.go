package livetests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func TestEIBCPFM_Live(t *testing.T) {
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
		RPCAddr:       "rpc.nimtwo.evm.ra.blumbus.noisnemyd.xyz:443",
		GrpcAddr:      "grpc.nimtwo.evm.ra.blumbus.noisnemyd.xyz:443",
		ChainID:       "nim_9999-1",
		Bin:           "rollapp-evm",
		GasPrices:     "0.0anim",
		GasAdjustment: "1.1",
		Denom:         "anim",
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

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")
	// rollappYUser.GetFaucet("http://18.184.170.181:3000/api/get-rolly")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	rollappXTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	rollappXIBCDenom := transfertypes.ParseDenomTrace(rollappXTokenDenom).IBCDenom()

	// rollappYTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappY, rollappYUser.Denom)
	// rollappYIBCDenom := transfertypes.ParseDenomTrace(rollappYTokenDenom).IBCDenom()

	hubTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDRollappXDym, dymensionUser.Denom)
	hubIBCDenom := transfertypes.ParseDenomTrace(hubTokenDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)

	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappXOrigBal)

	// rollappYOrigBal, err := rollappYUser.GetBalance(ctx, rollappYUser.Denom, rollappY.GrpcAddr)
	// require.NoError(t, err)
	// fmt.Println(rollappYOrigBal)

	erc20_OrigBal, err := GetERC20Balance(ctx, erc20IBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(erc20_OrigBal)

	// Compose an IBC transfer and send from rollapp1 -> hub -> rollapp2
	transferData := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}

	forwardMetadata := &ForwardMetadata{
		Receiver: rollappYUser.Address,
		Channel:  channelIDDymRollappY,
		Port:     "transfer",
		Timeout:  5 * time.Minute,
	}

	forwardMetadataJson, err := json.Marshal(forwardMetadata)
	require.NoError(t, err)

	multiplier := math.NewInt(10)
	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	memo := fmt.Sprintf(`{"eibc": {"fee": "%s"}, "forward": %s}`, eibcFee.String(), string(forwardMetadataJson))
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, ibc.TransferOptions{Memo: string(memo)})
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, hub)

	erc20_Bal, err := GetERC20Balance(ctx, hubIBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(erc20_Bal)
	fmt.Println(rollappXIBCDenom)

	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, math.ZeroInt())
	require.Equal(t, erc20_OrigBal, erc20_Bal)
	testutil.WaitForBlocks(ctx, 3, hub)


	erc20_Bal, err = GetERC20Balance(ctx, hubIBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(erc20_Bal)
}
