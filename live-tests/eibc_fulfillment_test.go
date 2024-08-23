package livetests

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func TestEIBCFulfillRolX_Live(t *testing.T) {
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

	// create market maker
	marketMaker, err := hub.CreateUser("dym2")
	require.NoError(t, err)
	rollappXUser, err := rollappX.CreateUser("rolx1")
	require.NoError(t, err)
	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)
	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	testutil.WaitForBlocks(ctx, 2, hub)
	marketMaker.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	testutil.WaitForBlocks(ctx, 2, hub)
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")

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

	rollappXHeight, err := rollappX.Height(ctx)
	require.NoError(t, err)
	fmt.Println(rollappXHeight)
	// wait until the packet is finalized on Rollapp 1
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappX.ChainID, rollappXHeight, 600)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// wait rollapp few more blocks for some reason
	testutil.WaitForBlocks(ctx, 10, hub)

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


	// Check non-fulfill
	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, math.ZeroInt())

	// get eIbc event
	encoding := encodingConfig()
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, &hub, 30, true, encoding.InterfaceRegistry)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	// fulfill demand orders from rollapp 1
	for _, eibcEvent := range eibcEvents {
		re := regexp.MustCompile(`^\d+`)
		if re.ReplaceAllString(eibcEvent.Price, "") == rollappXIBCDenom && eibcEvent.PacketStatus == "PENDING"{
			fmt.Println("EIBC Event:", eibcEvent)
			_, err := cosmos.FullfillDemandOrder(&hub, eibcEvent.OrderId, marketMaker.Address, dymFee)
			require.NoError(t, err)
		}
	}
	testutil.WaitForBlocks(ctx, 5, hub)
	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, transferAmount.Sub(eibcFee))
}
