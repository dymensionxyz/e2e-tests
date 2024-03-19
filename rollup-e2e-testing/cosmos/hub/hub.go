package hub

import (
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

func NewHub(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger, extraFlags map[string]interface{}) ibc.Chain {
	return dym_hub.NewDymHub(testName, chainConfig, numValidators, numFullNodes, log, extraFlags)
}
