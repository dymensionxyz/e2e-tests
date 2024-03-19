package rollapp

import (
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

func NewRollApp(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger, extraFlags map[string]interface{}) ibc.Chain {
	return dym_rollapp.NewDymRollApp(testName, chainConfig, numValidators, numFullNodes, log, extraFlags)
}
