package livetests

import (
	"context"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func TestEIBC_Demand_Order_Ignored_RolX_Live(t *testing.T) {
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
	testutil.WaitForBlocks(ctx, 2, hub)
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)
	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappXOrigBal)

	multiplier := math.NewInt(10)

	var options ibc.TransferOptions

	// Compose an IBC transfer and send from rollapp -> hub
	transferData := ibc.WalletData{
		Address: rollappXUser.Address,
		Denom:   dymensionUser.Denom,
		Amount:  transferAmount,
	}

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)
	cosmos.SendIBCTransfer(hub, channelIDDymRollappX, dymensionUser.Address, transferData, dymFee, options)
	require.NoError(t, err)

	encoding := encodingConfig()
	_, err = getEIbcEventsWithinBlockRange(ctx, &hub, 30, false, encoding.InterfaceRegistry)
	require.Error(t, err, "There wasn't a single 'eibc' event registered within the specified block range on the hub")
}
