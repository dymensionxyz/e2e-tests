package livetests

import (
	"context"
	"fmt"
	"regexp"
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
	mocha := cosmos.CosmosChain{
		RPCAddr:       "rpc.celestia.test-eu1.ccvalidators.com:443",
		GrpcAddr:      "mocha-4-consensus.mesa.newmetric.xyz:9090",
		ChainID:       "mocha-4",
		Bin:           "celestia-appd",
		GasPrices:     "0utia",
		GasAdjustment: "1.1",
		Denom:         "utia",
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

	mochaUser, err := mocha.CreateUser("mocha1")
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
	err = mocha.NewClient("https://" + mocha.RPCAddr)
	require.NoError(t, err)
	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)
	err = rollappY.NewClient("https://" + rollappY.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	// Wait for blocks
	testutil.WaitForBlocks(ctx, 2, hub)
	marketMaker.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	mochaUser.GetFaucet("http://18.184.170.181:3000/api/get-tia")
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")
	rollappYUser.GetFaucet("http://18.184.170.181:3000/api/get-rolly")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	rollappXTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	rollappXIBCDenom := transfertypes.ParseDenomTrace(rollappXTokenDenom).IBCDenom()

	mochaTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymMocha, mocha.Denom)
	mochaIBCDenom := transfertypes.ParseDenomTrace(mochaTokenDenom).IBCDenom()

	secondHopDenom := transfertypes.GetPrefixedDenom("transfer", channelIDRollappXDym, mochaTokenDenom)
	secondHopIBCDenom := transfertypes.ParseDenomTrace(secondHopDenom).IBCDenom()

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
	mochaOrigBal, err := mochaUser.GetBalance(ctx, mochaUser.Denom, mocha.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(mochaOrigBal)

	transferAmountMM := math.NewInt(1000)

	var options ibc.TransferOptions

	transferData := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   mocha.Denom,
		Amount:  transferAmountMM,
	}
	cosmos.SendIBCTransfer(mocha, channelIDMochaDym, mochaUser.Address, transferData, mochaFee, options)
	require.NoError(t, err)

	t.Log("mochaIBCDenom:", mochaIBCDenom)

	testutil.WaitForBlocks(ctx, 10, hub)

	testutil.AssertBalance(t, ctx, dymensionUser, mochaIBCDenom, hub.GrpcAddr, transferAmountMM)

	transferData = ibc.WalletData{
		Address: rollappXUser.Address,
		Denom:   mochaIBCDenom,
		Amount:  transferAmountMM,
	}
	cosmos.SendIBCTransfer(hub, channelIDDymRollappX, dymensionUser.Address, transferData, dymFee, options)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, hub)

	testutil.AssertBalance(t, ctx, rollappXUser, secondHopIBCDenom, rollappX.GrpcAddr, transferAmountMM)

	// Compose an IBC transfer and send from rollappx -> marketmaker
	transferDataRollAppXToMm := ibc.WalletData{
		Address: marketMaker.Address,
		Denom:   secondHopIBCDenom,
		Amount:  transferAmount,
	}

	multiplier := math.NewInt(10)

	// market maker needs to have funds on the hub first to be able to fulfill upcoming demand order
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferDataRollAppXToMm, rolxFee, options)
	require.NoError(t, err)

	rollappXHeight, err := rollappX.Height(ctx)
	require.NoError(t, err)
	fmt.Println(rollappXHeight)
	// wait until the packet is finalized on Rollapp 1
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappX.ChainID, rollappXHeight, 400)
	require.NoError(t, err)
	require.True(t, isFinalized)

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
		Denom:   secondHopIBCDenom,
		Amount:  transferAmount,
	}

	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferDataRollAppXToHub, rolxFee, options)
	require.NoError(t, err)

	// get eIbc event

	encoding := encodingConfig()
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, &hub, 20, false, encoding.InterfaceRegistry)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	// fulfill demand orders from rollapp 1
	for _, eibcEvent := range eibcEvents {
		re := regexp.MustCompile(`^\d+`)
		if re.ReplaceAllString(eibcEvent.Price, "") == rollappXIBCDenom && eibcEvent.PacketStatus == "PENDING" {
			fmt.Println("EIBC Event:", eibcEvent)
			_, err := cosmos.FullfillDemandOrder(&hub, eibcEvent.ID, marketMaker.Address, dymFee)
			require.NoError(t, err)
		}
	}

	// verify funds minus fee were added to receiver's address
	balance, err = dymensionUser.GetBalance(ctx, secondHopIBCDenom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("Balance of dymensionUserAddr after fulfilling the order:", balance)
	require.True(t, balance.Equal(transferAmount.Sub(eibcFee)), fmt.Sprintf("Value mismatch. Expected %s, actual %s", transferAmount.Sub(eibcFee), balance))
}
