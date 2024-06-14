package livetests

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/blockdb"
	dymensiontesting "github.com/decentrio/rollup-e2e-testing/dymension"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func TestEIBCFulfill_Live(t *testing.T) {
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
	marketmaker, err := hub.CreateUser("dym2")
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
	rollappYUser.GetFaucet("http://18.184.170.181:3000/api/get-rolly")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	rollappXTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	rollappXIBCDenom := transfertypes.ParseDenomTrace(rollappXTokenDenom).IBCDenom()

	// rollappYTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappY, rollappYUser.Denom)
	// rollappYIBCDenom := transfertypes.ParseDenomTrace(rollappYTokenDenom).IBCDenom()

	// hubTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDRollappXDym, dymensionUser.Denom)
	// hubIBCDenom := transfertypes.ParseDenomTrace(hubTokenDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(dymensionOrigBal)

	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappXOrigBal)

	rollappYOrigBal, err := rollappYUser.GetBalance(ctx, rollappYUser.Denom, rollappY.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(rollappYOrigBal)

	erc20_OrigBal, err := GetERC20Balance(ctx, erc20IBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println(erc20_OrigBal)

	// Compose an IBC transfer and send from rollappx -> marketmaker
	transferDataRollAppXToMm := ibc.WalletData{
		Address: marketmaker.Address,
		Denom:   rollappX.Denom,
		Amount:  transferAmount,
	}

	testutil.WaitForBlocks(ctx, 3, hub)

	var options ibc.TransferOptions
	cosmos.SendIBCTransfer(hub, channelIDDymRollappX, dymensionUser.Address, transferDataRollAppXToMm, dymFee, options)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 3, hub)

	testutil.AssertBalance(t, ctx, rollappXUser, rollappXUser.Denom, rollappX.GrpcAddr, rollappXOrigBal.Sub(transferAmount))
	// Minus 0.1% of transfer amount for bridge fee
	expMmBalanceRollappDenom := transferDataRollAppXToMm.Amount.Sub(transferDataRollAppXToMm.Amount.Quo(math.NewInt(1000)))
	balance, err := marketmaker.GetBalance(ctx, rollappXIBCDenom, hub.GrpcAddr)
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

	multiplier := math.NewInt(10)
	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	// set eIBC specific memo
	options.Memo = BuildEIbcMemo(eibcFee)
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferDataRollAppXToHub, rolxFee, options)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 10, hub)

	// get eIbc event

	encoding := encodingConfig()
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, &hub, 30, false, encoding.InterfaceRegistry)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	var fulfill_demand_order = false
	// fulfill demand orders from rollapp 1
	for _, eibcEvent := range eibcEvents {
		re := regexp.MustCompile(`^\d+`)
		if re.ReplaceAllString(eibcEvent.Price, "") == rollappXIBCDenom && eibcEvent.PacketStatus == "PENDING" {
			fmt.Println("EIBC Event:", eibcEvent)
			txResp, err := cosmos.FullfillDemandOrder(&hub, eibcEvent.ID, marketmaker.Address, dymFee)
			require.NoError(t, err)
			eibcEvent := getEibcEventFromTx(t, &hub, *txResp)
			if eibcEvent != nil {
				fmt.Println("After order fulfillment:", eibcEvent)
			}
			fulfill_demand_order = true
		}
	}

	require.Equal(t, true, fulfill_demand_order)

	// erc20_Bal, err := GetERC20Balance(ctx, hubIBCDenom, rollappX.GrpcAddr)
	// require.NoError(t, err)
	// fmt.Println(erc20_Bal)
	// fmt.Println(rollappXIBCDenom)

	// testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, math.ZeroInt())
	// require.Equal(t, erc20_OrigBal, erc20_Bal)

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappYUser.Address,
	// 	Denom:   dymensionUser.Denom,
	// 	Amount:  transferAmount,
	// }

	// cosmos.SendIBCTransfer(hub, channelIDDymRollappY, dymensionUser.Address, transferData, dymFee, options)
	// require.NoError(t, err)

	// testutil.WaitForBlocks(ctx, 3, hub)

	// // Compose an IBC transfer and send from rollapp -> hub
	// transferData = ibc.WalletData{
	// 	Address: dymensionUser.Address,
	// 	Denom:   rollappYUser.Denom,
	// 	Amount:  transferAmount,
	// }

	// cosmos.SendIBCTransfer(rollappY, channelIDRollappYDym, rollappYUser.Address, transferData, rolyFee, options)
	// require.NoError(t, err)

	// testutil.WaitForBlocks(ctx, 10, hub)

	// erc20_Bal, err = GetERC20Balance(ctx, hubIBCDenom, rollappX.GrpcAddr)
	// require.NoError(t, err)
	// fmt.Println(erc20_Bal)
	// fmt.Println(rollappYIBCDenom)

	// testutil.AssertBalance(t, ctx, dymensionUser, rollappYIBCDenom, hub.GrpcAddr, math.ZeroInt())
	// require.Equal(t, erc20_OrigBal, erc20_Bal)
}

func getEibcEventFromTx(t *testing.T, dymension *cosmos.CosmosChain, txResp types.TxResponse) *dymensiontesting.EibcEvent {
	const evType = "eibc"
	events := txResp.Events

	var (
		id, _           = cosmos.AttributeValue(events, evType, "id")
		price, _        = cosmos.AttributeValue(events, evType, "price")
		fee, _          = cosmos.AttributeValue(events, evType, "fee")
		isFulfilled, _  = cosmos.AttributeValue(events, evType, "is_fulfilled")
		packetStatus, _ = cosmos.AttributeValue(events, evType, "packet_status")
	)

	eibcEvent := new(dymensiontesting.EibcEvent)
	eibcEvent.ID = id
	eibcEvent.Price = price
	eibcEvent.Fee = fee
	checkFulfilled, err := strconv.ParseBool(isFulfilled)
	if err != nil {
		require.NoError(t, err)
		return nil
	}
	eibcEvent.IsFulfilled = checkFulfilled
	eibcEvent.PacketStatus = packetStatus

	return eibcEvent
}

func getEIbcEventsWithinBlockRange(
	ctx context.Context,
	dymension *cosmos.CosmosChain,
	blockRange uint64,
	breakOnFirstOccurence bool,
	interfaceRegistry codectypes.InterfaceRegistry,
) ([]dymensiontesting.EibcEvent, error) {
	var eibcEventsArray []dymensiontesting.EibcEvent

	height, err := dymension.Height(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Dymension height: %w", err)
	}
	fmt.Printf("Dymension height: %d\n", height)

	err = testutil.WaitForBlocks(ctx, int(blockRange), *dymension)
	if err != nil {
		return nil, fmt.Errorf("error waiting for blocks: %w", err)
	}

	eibcEvents, err := getEventsOfType(dymension, height-5, height+blockRange, "eibc", breakOnFirstOccurence, interfaceRegistry)
	if err != nil {
		return nil, fmt.Errorf("error getting events of type 'eibc': %w", err)
	}

	if len(eibcEvents) == 0 {
		return nil, fmt.Errorf("There wasn't a single 'eibc' event registered within the specified block range on the hub")
	}

	for _, event := range eibcEvents {
		eibcEvent, err := dymensiontesting.MapToEibcEvent(event)
		if err != nil {
			return nil, fmt.Errorf("error mapping to EibcEvent: %w", err)
		}
		eibcEventsArray = append(eibcEventsArray, eibcEvent)
	}

	return eibcEventsArray, nil
}

func getEventsOfType(chain *cosmos.CosmosChain, startHeight uint64, endHeight uint64, eventType string, breakOnFirstOccurence bool, interfaceRegistry codectypes.InterfaceRegistry) ([]blockdb.Event, error) {
	var eventTypeArray []blockdb.Event
	shouldReturn := false

	for height := startHeight; height <= endHeight && !shouldReturn; height++ {
		txs, err := chain.FindTxs(context.Background(), height, interfaceRegistry)
		if err != nil {
			return nil, fmt.Errorf("error fetching transactions at height %d: %w", height, err)
		}

		for _, tx := range txs {
			for _, event := range tx.Events {
				if event.Type == eventType {
					eventTypeArray = append(eventTypeArray, event)
					if breakOnFirstOccurence {
						shouldReturn = true
						fmt.Printf("%s event found on block height: %d", eventType, height)
						break
					}
				}
			}
			if shouldReturn {
				break
			}
		}
	}

	return eventTypeArray, nil
}
