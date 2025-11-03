package livetests

import (
	"bytes"
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

func TestEIBC_AckError_Dym_RolY_Live(t *testing.T) {
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

	mocha := cosmos.CosmosChain{
		RPCAddr:       "rpc.celestia.test-eu1.ccvalidators.com:443",
		GrpcAddr:      "mocha-4-consensus.mesa.newmetric.xyz:9090",
		ChainID:       "mocha-4",
		Bin:           "celestia-appd",
		GasPrices:     "0utia",
		GasAdjustment: "1.1",
		Denom:         "utia",
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
	rollappYUser, err := rollappY.CreateUser("roly1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = mocha.NewClient("https://" + mocha.RPCAddr)
	require.NoError(t, err)

	err = rollappY.NewClient("https://" + rollappY.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	mochaUser.GetFaucet("http://18.184.170.181:3000/api/get-tia")
	rollappYUser.GetFaucet("http://18.184.170.181:3000/api/get-rolly")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	mochaTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymMocha, mocha.Denom)
	mochaIBCDenom := transfertypes.ParseDenomTrace(mochaTokenDenom).IBCDenom()

	// tia from rollapp -> hub
	mochaRollappYDenom := transfertypes.GetPrefixedDenom("transfer", channelIDRollappYDym, mochaTokenDenom)
	mochaRollappYIBCDenom := transfertypes.ParseDenomTrace(mochaRollappYDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)

	rollappYOrigBal, err := rollappYUser.GetBalance(ctx, rollappYUser.Denom, rollappY.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappYOrigBal)

	mochaOrigBal, err := mochaUser.GetBalance(ctx, mochaUser.Denom, mocha.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(mochaOrigBal)

	// Use mocha to trigger ack error because rollapp isn't registered with mocha token
	var options ibc.TransferOptions

	transferAmountMM := math.NewInt(1000)
	// Compose an IBC transfer and send mocha from mocha -> dymension
	transferData := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   mocha.Denom,
		Amount:  transferAmountMM.Mul(math.NewInt(2)),
	}

	_, err = cosmos.SendIBCTransfer(mocha, channelIDMochaDym, mochaUser.Address, transferData, mochaFee, options)
	require.NoError(t, err)

	t.Log("mochaIBCDenom:", mochaIBCDenom)

	testutil.WaitForBlocks(ctx, 10, hub)

	testutil.AssertBalance(t, ctx, dymensionUser, mochaIBCDenom, hub.GrpcAddr, transferAmountMM.Mul(math.NewInt(2)))

	// Compose an IBC transfer and send mocha from dymension -> rollapp
	transferData = ibc.WalletData{
		Address: rollappYUser.Address,
		Denom:   mochaIBCDenom,
		Amount:  transferAmountMM,
	}

	_, err = cosmos.SendIBCTransfer(hub, channelIDDymRollappY, dymensionUser.Address, transferData, dymFee, options)
	require.NoError(t, err)

	rollappYHeight, err := rollappY.Height(ctx)
	require.NoError(t, err)
	fmt.Println(rollappYHeight)
	// wait until the packet is finalized on Rollapp 1
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappY.ChainID, rollappYHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	testutil.AssertBalance(t, ctx, rollappYUser, mochaRollappYIBCDenom, rollappY.GrpcAddr, transferAmountMM)

	fmt.Println("Ibc denom of mocha on rollapp x:", mochaRollappYIBCDenom)
	// Compose an IBC transfer and send from rollapp -> hub
	multiplier := math.NewInt(10)
	eibcFee := transferAmountMM.Quo(multiplier)

	options.Memo = BuildEIbcMemo(eibcFee)
	transferData = ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   mochaRollappYIBCDenom,
		Amount:  transferAmountMM,
	}

	txResp, err := cosmos.SendIBCTransfer(rollappY, channelIDRollappYDym, rollappYUser.Address, transferData, rolyFee, options)
	require.NoError(t, err)

	testutil.AssertBalance(t, ctx, dymensionUser, mochaIBCDenom, hub.GrpcAddr, transferAmountMM)

	Mocha_Rolly_Bal, err := rollappYUser.GetBalance(ctx, mochaRollappYIBCDenom, rollappY.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("rollapp user mocha balance right after ibc transfer from rollapp -> hub: ", Mocha_Rolly_Bal, mochaRollappYIBCDenom)
	require.Equal(t, zeroBal, Mocha_Rolly_Bal)

	ibcTx, err := cosmos.GetIbcTxFromTxResponse(*txResp)
	require.NoError(t, err)

	// catch ACK errors
	rollappHeight, err := rollappY.Height(ctx)
	require.NoError(t, err)

	encodingConfig := encodingConfig()
	ack, err := testutil.PollForAck(ctx, rollappY, encodingConfig.InterfaceRegistry, rollappHeight, rollappHeight+120, ibcTx.Packet)
	require.NoError(t, err)

	// Make sure that the ack contains error
	require.True(t, bytes.Contains(ack.Acknowledgement, []byte("error")))
	// fund was return to roly user and dym user balance stay the same
	testutil.AssertBalance(t, ctx, rollappYUser, mochaRollappYIBCDenom, rollappY.GrpcAddr, transferAmountMM)
	testutil.AssertBalance(t, ctx, dymensionUser, mochaIBCDenom, hub.GrpcAddr, transferAmountMM)
}
