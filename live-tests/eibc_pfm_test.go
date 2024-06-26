package livetests

import (
	"bytes"
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

	mocha := cosmos.CosmosChain{
		RPCAddr:       "rpc.celestia.test-eu1.ccvalidators.com:443",
		GrpcAddr:      "mocha-4-consensus.mesa.newmetric.xyz:9090",
		ChainID:       "mocha-4",
		Bin:           "celestia-appd",
		GasPrices:     "0utia",
		GasAdjustment: "1.1",
		Denom:         "utia",
	}

	dymensionUser, err := hub.CreateUser("dym1")
	require.NoError(t, err)
	rollappXUser, err := rollappX.CreateUser("rolx1")
	require.NoError(t, err)
	mochaUser, err := mocha.CreateUser("mocha1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)

	err = mocha.NewClient("https://" + mocha.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	firstHopDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	firstHopIBCDenom := transfertypes.ParseDenomTrace(firstHopDenom).IBCDenom()

	secondHopDenom := transfertypes.GetPrefixedDenom("transfer", channelIDMochaDym, firstHopDenom)
	secondHopIBCDenom := transfertypes.ParseDenomTrace(secondHopDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)

	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappXOrigBal)

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
		Receiver: mochaUser.Address,
		Channel:  channelIDDymMocha,
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
	ack, err := testutil.PollForAck(ctx, rollappX, encodingConfig.InterfaceRegistry, rollappXHeight, rollappXHeight+30, tx.Packet)
	require.NoError(t, err)

	fmt.Println(rollappXHeight)
	// wait until the packet is finalized on Rollapp 1
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappX.ChainID, rollappXHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	// Make sure the ack contains error
	require.True(t, bytes.Contains(ack.Acknowledgement, []byte("error")))

	testutil.AssertBalance(t, ctx, dymensionUser, firstHopIBCDenom, hub.GrpcAddr, zeroBal)
	testutil.AssertBalance(t, ctx, mochaUser, secondHopIBCDenom, mocha.GrpcAddr, zeroBal)
}
