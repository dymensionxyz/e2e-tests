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

	// Compose an IBC transfer and send from rollapp -> hub
	transferDataRollAppXToHub := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}

	multiplier := math.NewInt(10)
	eibcFee := transferAmount.Quo(multiplier) // transferAmount * 0.1

	var options ibc.TransferOptions

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
			txResp, err := cosmos.FullfillDemandOrder(&hub, eibcEvent.ID, marketMakerAddress, dymFee)
			require.NoError(t, err)
			eibcEvent := getEibcEventFromTx(t, &hub, *txResp)
			if eibcEvent != nil {
				fmt.Println("After order fulfillment:", eibcEvent)
			}
			fulfill_demand_order = true
		}
	}

	require.Equal(t, true, fulfill_demand_order)
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
