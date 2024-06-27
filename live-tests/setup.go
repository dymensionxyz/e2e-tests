package livetests

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	hubgenesis "github.com/dymensionxyz/dymension-rdk/x/hub-genesis/types"
	eibc "github.com/dymensionxyz/dymension/v3/x/eibc/types"
	rollapp "github.com/dymensionxyz/dymension/v3/x/rollapp/types"
	ethermintcrypto "github.com/evmos/ethermint/crypto/codec"
	ethermint "github.com/evmos/ethermint/types"
)

var (
	channelIDDymRollappX                     = "channel-17"
	channelIDDymRollappY                     = "channel-22"
	channelIDDymMocha                        = "channel-2"
	channelIDMochaDym                        = "channel-28"
	channelIDRollappXDym                     = "channel-0"
	channelIDRollappYDym                     = "channel-0"
	dymFee                                   = "6000000000000000adym"
	rolxFee                                  = "10000000000000arolx"
	rolyFee                                  = "2000000000000000aroly"
	mochaFee                                 = "17000utia"
	erc20Addr                                = "rolx1glht96kr2rseywuvhhay894qw7ekuc4q4d4qs2"
	erc20Contract                            = "0x80b5a32E4F032B2a058b4F29EC95EEfEEB87aDcd"
	erc20IBCDenom                            = "ibc/FECACB927EB3102CCCB240FFB3B6FCCEEB8D944C6FEA8DFF079650FEFF59781D"
	transferAmount                           = sdkmath.NewInt(1_000_000)
	disputed_period_plus_batch_submit_blocks = 80
)

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
