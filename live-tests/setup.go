package livetests

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/cosmos/cosmos-sdk/types"
	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/blockdb"
	dymensiontesting "github.com/decentrio/rollup-e2e-testing/dymension"
	hubgenesis "github.com/dymensionxyz/dymension-rdk/x/hub-genesis/types"
	eibc "github.com/dymensionxyz/dymension/v3/x/eibc/types"
	rollapp "github.com/dymensionxyz/dymension/v3/x/rollapp/types"
	ethermintcrypto "github.com/evmos/ethermint/crypto/codec"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	channelIDDymRollappX                     = "channel-17"
	channelIDDymRollappY                     = "channel-22"
	channelIDRollappXDym                     = "channel-0"
	channelIDRollappYDym                     = "channel-0"
	channelIDDymMocha                        = "channel-2"
	channelIDMochaDym                        = "channel-28"
	dymFee                                   = "6000000000000000adym"
	rolxFee                                  = "10000000000000arolx"
	rolyFee                                  = "2000000000000000aroly"
	mochaFee                                 = "17000utia"
	erc20Addr                                = "rolx1glht96kr2rseywuvhhay894qw7ekuc4q4d4qs2"
	erc20Contract                            = "0x80b5a32E4F032B2a058b4F29EC95EEfEEB87aDcd"
	erc20IBCDenom                            = "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D"
	transferAmount                           = sdkmath.NewInt(1_000_000)
	disputed_period_plus_batch_submit_blocks = 80
	zeroBal                                  = sdkmath.ZeroInt()
)

type ForwardMetadata struct {
	Receiver       string        `json:"receiver"`
	Port           string        `json:"port"`
	Channel        string        `json:"channel"`
	Timeout        time.Duration `json:"timeout"`
	Retries        *uint8        `json:"retries,omitempty"`
	Next           *string       `json:"next,omitempty"`
	RefundSequence *uint64       `json:"refund_sequence,omitempty"`
}

func BuildEIbcMemo(eibcFee sdkmath.Int) string {
	return fmt.Sprintf(`{"eibc": {"fee": "%s"}}`, eibcFee.String())
}
func GetERC20Balance(ctx context.Context, denom, grpcAddr string) (sdkmath.Int, error) {
	params := &bankTypes.QueryBalanceRequest{Address: erc20Addr, Denom: denom}
	conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return sdkmath.Int{}, err
	}
	defer conn.Close()

	queryClient := bankTypes.NewQueryClient(conn)
	res, err := queryClient.Balance(ctx, params)

	if err != nil {
		return sdkmath.Int{}, err
	}

	return res.Balance.Amount, nil
}

func encodingConfig() *simappparams.EncodingConfig {
	cfg := cosmos.DefaultEncoding()

	ethermint.RegisterInterfaces(cfg.InterfaceRegistry)
	ethermintcrypto.RegisterInterfaces(cfg.InterfaceRegistry)
	eibc.RegisterInterfaces(cfg.InterfaceRegistry)
	rollapp.RegisterInterfaces(cfg.InterfaceRegistry)
	hubgenesis.RegisterInterfaces(cfg.InterfaceRegistry)
	return &cfg
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
