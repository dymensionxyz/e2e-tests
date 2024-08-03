package livetests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/decentrio/e2e-testing-live/cosmos"
	"github.com/decentrio/e2e-testing-live/testutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

// TestDelayackRollappToHubNoFinalized_Live test IBC transfer from rollapp to hub, so that its succeeds (creates rollapp packet) when rollapp has NO FINALIZED STATES AT ALL (just pending)
func TestDelayackRollappToHubNoFinalizedRolX_Live(t *testing.T) {
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
	rollappXUser, err := rollappX.CreateUser("rolx1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	rollappXTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	rollappXIBCDenom := transfertypes.ParseDenomTrace(rollappXTokenDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("hub original balance: ", dymensionOrigBal, dymensionUser.Denom)

	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("rollapp x original balance: ", rollappXOrigBal, rollappXUser.Denom)

	erc20_OrigBal, err := GetERC20Balance(ctx, erc20IBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("rol x user balance of denom send from dym: ", erc20_OrigBal)

	// Compose an IBC transfer and send from rollapp -> hub
	transferData := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}

	var options ibc.TransferOptions
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, options)
	require.NoError(t, err)
	// Amount should not be received by hub yet
	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, sdkmath.NewInt(0))

	// Check the state info status every 5 seconds to ensure it is always pending
	// Run QueryRollappState in parallel with WaitForBlocks
	var wg sync.WaitGroup
	wg.Add(2)

	// Channel to capture errors
	errChan := make(chan error, 1)

	// Goroutine to query Rollapp state every 5 seconds
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		timeout := time.After(500 * time.Second) // Set a timeout for the whole check

		for {
			select {
			case <-ticker.C:
				res, err := hub.QueryRollappState(rollappX.ChainID, false)
				if err != nil {
					errChan <- err
					return
				}
				if res.StateInfo.Status != "PENDING" {
					errChan <- fmt.Errorf("unexpected state: %s", res.StateInfo.Status)
					return
				}
			case <-timeout:
				return
			}
		}
	}()

	// Goroutine to wait for blocks
	go func() {
		defer wg.Done()
		testutil.WaitForBlocks(ctx, disputed_period_plus_batch_submit_blocks, hub)
		fmt.Println("wait for blocks done")
	}()

	// Wait for both goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors from goroutines
	if err := <-errChan; err != nil {
		require.NoError(t, err)
	}

	// TODO: sub bridging fee on new version
	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, transferAmount)

}

// TestDelayackRollappToHub_Live test delayack, rollapp token transfer should only be recieved on the hub upon rollapp finalized state (assume no elBC packet, i.e no memo)
func TestDelayackRollappToHubRolX_Live(t *testing.T) {
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
	rollappXUser, err := rollappX.CreateUser("rolx1")
	require.NoError(t, err)

	err = hub.NewClient("https://" + hub.RPCAddr)
	require.NoError(t, err)

	err = rollappX.NewClient("https://" + rollappX.RPCAddr)
	require.NoError(t, err)

	dymensionUser.GetFaucet("http://18.184.170.181:3000/api/get-dym")
	rollappXUser.GetFaucet("http://18.184.170.181:3000/api/get-rollx")

	// Wait for blocks
	testutil.WaitForBlocks(ctx, 5, hub)

	// Get the IBC denom
	rollappXTokenDenom := transfertypes.GetPrefixedDenom("transfer", channelIDDymRollappX, rollappXUser.Denom)
	rollappXIBCDenom := transfertypes.ParseDenomTrace(rollappXTokenDenom).IBCDenom()

	dymensionOrigBal, err := dymensionUser.GetBalance(ctx, dymensionUser.Denom, hub.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("hub original balance: ", dymensionOrigBal, dymensionUser.Denom)

	rollappXOrigBal, err := rollappXUser.GetBalance(ctx, rollappXUser.Denom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("rollapp x original balance: ", rollappXOrigBal, rollappXUser.Denom)

	erc20_OrigBal, err := GetERC20Balance(ctx, erc20IBCDenom, rollappX.GrpcAddr)
	require.NoError(t, err)
	fmt.Println("rol x user balance of denom send from dym: ", erc20_OrigBal)

	// Compose an IBC transfer and send from rollapp -> hub
	transferData := ibc.WalletData{
		Address: dymensionUser.Address,
		Denom:   rollappXUser.Denom,
		Amount:  transferAmount,
	}

	var options ibc.TransferOptions
	cosmos.SendIBCTransfer(rollappX, channelIDRollappXDym, rollappXUser.Address, transferData, rolxFee, options)
	require.NoError(t, err)
	// Amount should not be received by hub yet
	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, sdkmath.NewInt(0))

	rollappXHeight, err := rollappX.Height(ctx)
	require.NoError(t, err)
	fmt.Println(rollappXHeight)
	// wait until the packet is finalized on Rollapp 1
	isFinalized, err := hub.WaitUntilRollappHeightIsFinalized(ctx, rollappX.ChainID, rollappXHeight, 500)
	require.NoError(t, err)
	require.True(t, isFinalized)

	fmt.Println("rollapp x ibc denom: ", rollappXIBCDenom, dymensionUser)
	// TODO: sub bridging fee on new version
	testutil.AssertBalance(t, ctx, dymensionUser, rollappXIBCDenom, hub.GrpcAddr, transferAmount)
}
