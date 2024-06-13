package livetests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"bytes"
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

	rollappNim := cosmos.CosmosChain{
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
	rollappNimUser, err := rollappNim.CreateUser("rolnim1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)

	err = rollappNim.NewClient("https://" + rollappNim.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")
	// rollappYUser.GetFaucet("http://18.184.170.181:3000/api/get-rolly")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	firstHopDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	firstHopIBCDenom := transfertypes.ParseDenomTrace(firstHopDenom).IBCDenom()

	secondHopDenom := transfertypes.GetPrefixedDenom("transfer", channelIDNimDym, firstHopDenom)
	secondHopIBCDenom := transfertypes.ParseDenomTrace(secondHopDenom).IBCDenom()

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
		Receiver: rollappNimUser.Address,
		Channel:  channelIDDymNim,
		Port:     "transfer",
		Timeout:  5 * time.Minute,
	}

	forwardMetadataJson, err := json.Marshal(forwardMetadata)
	require.NoError(t, err)

	multiplier := math.NewInt(10)
	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	memo := fmt.Sprintf(`{"eibc": {"fee": "%s"}, "forward": %s}`, eibcFee.String(), string(forwardMetadataJson))
	txResp, err := cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, ibc.TransferOptions{Memo: string(memo)})
	require.NoError(t, err)

	tx, err := cosmos.GetIbcTxFromTxResponse(*txResp)
	require.NoError(t, err)

	rollappXHeight, err := rollappX.Height(ctx)
	require.NoError(t, err)
	encodingConfig := encodingConfig()
	ack, err := testutil.PollForAck(ctx, rollappX, encodingConfig.InterfaceRegistry,  rollappXHeight, rollappXHeight+30, tx.Packet)
	require.NoError(t, err)
	testutil.WaitForBlocks(ctx, 10, hub)

	// Make sure the ack contains error
	require.True(t, bytes.Contains(ack.Acknowledgement, []byte("error")))

	testutil.AssertBalance(t, ctx, rollappXUser, rollappXUser.Denom, rollappX.GrpcAddr, rollappXOrigBal)
	testutil.AssertBalance(t, ctx, dymensionUser, firstHopIBCDenom, hub.GrpcAddr, zeroBal)
	testutil.AssertBalance(t, ctx, rollappNimUser, secondHopIBCDenom, rollappNim.GrpcAddr, zeroBal)
}
