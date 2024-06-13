package livetests

import (
	"context"
	"fmt"
	"time"
	sdkmath "cosmossdk.io/math"
	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	channelIDDymRollappX = "channel-17"
	channelIDDymRollappY = "channel-22"
	channelIDRollappXDym = "channel-0"
	channelIDRollappYDym = "channel-0"
	dymFee               = "6000000000000000adym"
	rolxFee              = "10000000000000arolx"
	rolyFee              = "2000000000000000aroly"
	erc20Addr            = "rolx1glht96kr2rseywuvhhay894qw7ekuc4q4d4qs2"
	erc20IBCDenom        = "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D"
	transferAmount       = sdkmath.NewInt(1_000_000)
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
