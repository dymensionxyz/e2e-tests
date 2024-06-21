package livetests

import (
	"context"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func TestEIBC_3rd_Token_Live(t *testing.T) {
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

	// create market maker
	marketMaker, err := hub.CreateUser("dym2")
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
	testutil.WaitForBlocks(ctx, 2, hub)
	marketMaker.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	testutil.WaitForBlocks(ctx, 2, hub)
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")
	testutil.WaitForBlocks(ctx, 2, hub)
	rollappYUser.GetFaucet("http://18.184.170.181:3000/api/get-rolly")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	rollappXTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	rollappXIBCDenom := transfertypes.ParseDenomTrace(rollappXTokenDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)
	mmOrigBal, err := marketMaker.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(mmOrigBal)
	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappXOrigBal)
	rollappYOrigBal, err := rollappYUser.GetBalance(ctx, rollappYUser.Denom, rollappY.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappYOrigBal)
	erc20_OrigBal, err := GetERC20Balance(ctx, erc20IBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(erc20_OrigBal)

	transferAmountMM := math.NewInt(100000000000)
	// Compose an IBC transfer and send from rollappx -> marketmaker
	transferDataRollAppXToMm := ibc.WalletData{
		Address: marketMaker.Address,
		Denom:   rollappX.Denom,
		Amount:  transferAmountMM,
	}
	testutil.WaitForBlocks(ctx, 3, hub)

	multiplier := math.NewInt(10)

	var options ibc.TransferOptions

	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferDataRollAppXToMm, rolxFee, options)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 80, hub)

	// TODO: Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := transferDataRollAppXToMm.Amount
	balance, err := marketMaker.GetBalance(ctx, rollappXIBCDenom, hub.GrpcAddr)
	require.NoError(t, err)

	fmt.Println("Balance of marketMakerAddr after preconditions:", balance)
	require.True(t, balance.Equal(expMmBalanceRollappDenom), fmt.Sprintf("Value mismatch. Expected %s, actual %s", expMmBalanceRollappDenom, balance))
	// end of preconditions

	// Compose an IBC transfer and send from rollapp -> hub
	transferDataRollAppXToHub := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferDataRollAppXToHub, rolxFee, options)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, hub)

	// // get eIbc event

	// encoding := encodingConfig()
	// eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, &hub, 30, false, encoding.InterfaceRegistry)
	// require.NoError(t, err)
	// for i, eibcEvent := range eibcEvents {
	// 	fmt.Println(i, "EIBC Event:", eibcEvent)
	// }

	// var fulfill_demand_order = false
	// // fulfill demand orders from rollapp 1
	// for _, eibcEvent := range eibcEvents {
	// 	re := regexp.MustCompile(`^\d+`)
	// 	if re.ReplaceAllString(eibcEvent.Price, "") == rollappXIBCDenom && eibcEvent.PacketStatus == "PENDING" {
	// 		fmt.Println("EIBC Event:", eibcEvent)
	// 		txResp, err := cosmos.FullfillDemandOrder(&hub, eibcEvent.ID, marketMaker.Address, dymFee)
	// 		require.NoError(t, err)
	// 		eibcEvent := getEibcEventFromTx(t, &hub, *txResp)
	// 		if eibcEvent != nil {
	// 			fmt.Println("After order fulfillment:", eibcEvent)
	// 		}
	// 		fulfill_demand_order = true
	// 	}
	// }

	// require.Equal(t, true, fulfill_demand_order)
}
