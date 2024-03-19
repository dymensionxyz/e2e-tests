package rollupe2etesting

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ChainFactory describes how to get chains for tests.
// This type currently supports a Pair method,
// but it may be expanded to a Triplet method in the future.
type ChainFactory interface {
	// Count reports how many chains this factory will produce from its Chains method.
	Count() int

	// Chains returns a set of chains.
	Chains(testName string) ([]ibc.Chain, error)

	// Name returns a descriptive name of the factory,
	// indicating all of its chains.
	// Depending on how the factory was configured,
	// this may report more than two chains.
	Name() string
}

// BuiltinChainFactory implements ChainFactory to return a fixed set of chains.
// Use NewBuiltinChainFactory to create an instance.
type BuiltinChainFactory struct {
	log *zap.Logger

	specs []*ChainSpec
}

var logConfiguredChainsSourceOnce sync.Once

// initBuiltinChainConfig returns an ibc.ChainConfig mapping all configured chains
func initBuiltinChainConfig(log *zap.Logger) (map[string]ibc.ChainConfig, error) {
	var dat []byte
	var err error

	// checks if IBCTEST_CONFIGURED_CHAINS environment variable is set with a path,
	// otherwise, ./configuredChains.yaml gets embedded and used.
	val := os.Getenv("IBCTEST_CONFIGURED_CHAINS")

	if val != "" {
		dat, err = os.ReadFile(val)
		if err != nil {
			return nil, err
		}
	} else {
		// dat = embeddedConfiguredChains
	}

	builtinChainConfigs := make(map[string]ibc.ChainConfig)

	err = yaml.Unmarshal(dat, &builtinChainConfigs)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling pre-configured chains: %w", err)
	}

	logConfiguredChainsSourceOnce.Do(func() {
		if val != "" {
			log.Info("Using user specified configured chains", zap.String("file", val))
		} else {
			log.Info("Using embedded configured chains")
		}
	})

	return builtinChainConfigs, nil
}

// NewBuiltinChainFactory returns a BuiltinChainFactory that returns chains defined by entries.
func NewBuiltinChainFactory(log *zap.Logger, specs []*ChainSpec) *BuiltinChainFactory {
	return &BuiltinChainFactory{log: log, specs: specs}
}

func (f *BuiltinChainFactory) Count() int {
	return len(f.specs)
}

func (f *BuiltinChainFactory) Chains(testName string) ([]ibc.Chain, error) {
	chains := make([]ibc.Chain, len(f.specs))
	for i, s := range f.specs {
		cfg, err := s.Config(f.log)
		if err != nil {
			// Prefer to wrap the error with the chain name if possible.
			if s.Name != "" {
				return nil, fmt.Errorf("failed to build chain config %s: %w", s.Name, err)
			}

			return nil, fmt.Errorf("failed to build chain config at index %d: %w", i, err)
		}

		chain, err := buildChain(f.log, testName, *cfg, s.NumValidators, s.NumFullNodes, s.ExtraFlags)
		if err != nil {
			return nil, err
		}
		chains[i] = chain
	}

	return chains, nil
}

const (
	defaultNumValidators = 2
	defaultNumFullNodes  = 1
)

func buildChain(log *zap.Logger, testName string, cfg ibc.ChainConfig, numValidators, numFullNodes *int, extraFlags map[string]interface{}) (ibc.Chain, error) {
	nv := defaultNumValidators
	if numValidators != nil {
		nv = *numValidators
	}
	nf := defaultNumFullNodes
	if numFullNodes != nil {
		nf = *numFullNodes
	}

	chainType := strings.Split(cfg.Type, "-")

	if chainType[0] == "rollapp" {
		return rollapp.NewRollApp(testName, cfg, nv, nf, log, extraFlags), nil
	} else if chainType[0] == "hub" {
		return hub.NewHub(testName, cfg, nv, nf, log, extraFlags), nil
	}

	return cosmos.NewCosmosChain(testName, cfg, nv, nf, log), nil
}

func (f *BuiltinChainFactory) Name() string {
	parts := make([]string, len(f.specs))
	for i, s := range f.specs {
		// Ignoring error here because if we fail to generate the config,
		// another part of the factory stack should have failed properly before we got here.
		cfg, _ := s.Config(f.log)

		v := s.Version
		if v == "" {
			v = cfg.Images[0].Version
		}

		parts[i] = cfg.Name + "@" + v
	}
	return strings.Join(parts, "+")
}
