package testutil

import (
	"context"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func AssertBalance(t *testing.T, ctx context.Context, chain ibc.Chain, address string, denom string, expectedBalance sdkmath.Int) {
	balance, err := chain.GetBalance(ctx, address, denom)
	require.NoError(t, err)
	require.Equal(t, expectedBalance, balance)
}
