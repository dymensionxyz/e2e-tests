package rollupe2etesting

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/decentrio/rollup-e2e-testing/ibc"
	"go.uber.org/zap"
)

// ChainSpec is a wrapper around an ibc.ChainConfig
// that allows callers to easily reference one of the built-in chain configs
// and optionally provide overrides for some settings.
type ChainSpec struct {
	// Name is the name of the built-in config to use as a basis for this chain spec.
	// Required unless every other field is set.
	Name string

	// ChainName sets the Name of the embedded ibc.ChainConfig, i.e. the name of the chain.
	ChainName string

	// Version of the docker image to use.
	// Must be set.
	Version string

	// GasAdjustment and NoHostMount are pointers in ChainSpec
	// so zero-overrides can be detected from omitted overrides.
	GasAdjustment *float64
	NoHostMount   *bool

	// Embedded ChainConfig to allow for simple JSON definition of a ChainSpec.
	ibc.ChainConfig

	// How many validators and how many full nodes to use
	// when instantiating the chain.
	// If unspecified, NumValidators defaults to 2 and NumFullNodes defaults to 1.
	NumValidators, NumFullNodes *int

	// ExtraFlags appends additional flags when starting the chain.
	ExtraFlags map[string]interface{}

	// Generate the automatic suffix on demand when needed.
	autoSuffixOnce sync.Once
	autoSuffix     string
}

// Config returns the underlying ChainConfig,
// with any overrides applied.
func (s *ChainSpec) Config(log *zap.Logger) (*ibc.ChainConfig, error) {
	if s.Version == "" {
		// Version must be set at top-level if not set in inlined config.
		if len(s.ChainConfig.Images) == 0 || s.ChainConfig.Images[0].Version == "" {
			return nil, errors.New("ChainSpec.Version must not be empty")
		}
	}

	// s.Name and chainConfig.Name are interchangeable
	if s.Name == "" && s.ChainConfig.Name != "" {
		s.Name = s.ChainConfig.Name
	} else if s.Name != "" && s.ChainConfig.Name == "" {
		s.ChainConfig.Name = s.Name
	}

	// Empty name is only valid with a fully defined chain config.
	if s.Name == "" {
		// If ChainName is provided and ChainConfig.Name is not set, set it.
		if s.ChainConfig.Name == "" && s.ChainName != "" {
			s.ChainConfig.Name = s.ChainName
		}
		if !s.ChainConfig.IsFullyConfigured() {
			return nil, errors.New("ChainSpec.Name required when not all config fields are set")
		}

		return s.applyConfigOverrides(s.ChainConfig)
	}

	builtinChainConfigs, err := initBuiltinChainConfig(log)
	if err != nil {
		return nil, fmt.Errorf("failed to get pre-configured chains: %w", err)
	}

	// Get built-in config.
	// If chain doesn't have built in config, but is fully configured, register chain label.
	cfg, ok := builtinChainConfigs[s.Name]
	if !ok {
		if !s.ChainConfig.IsFullyConfigured() {
			availableChains := make([]string, 0, len(builtinChainConfigs))
			for k := range builtinChainConfigs {
				availableChains = append(availableChains, k)
			}
			sort.Strings(availableChains)

			return nil, fmt.Errorf("no chain configuration for %s (available chains are: %s)", s.Name, strings.Join(availableChains, ", "))
		}
		cfg = ibc.ChainConfig{}
	}

	cfg = cfg.Clone()

	// Apply any overrides from this ChainSpec.
	cfg = cfg.MergeChainSpecConfig(s.ChainConfig)

	coinType, err := cfg.VerifyCoinType()
	if err != nil {
		return nil, err
	}
	cfg.CoinType = coinType

	// Apply remaining top-level overrides.
	return s.applyConfigOverrides(cfg)
}

func (s *ChainSpec) applyConfigOverrides(cfg ibc.ChainConfig) (*ibc.ChainConfig, error) {
	// If no ChainName provided, generate one based on the spec name.
	cfg.Name = s.ChainName
	if cfg.Name == "" {
		cfg.Name = s.Name + s.suffix()
	}

	// If no ChainID provided, generate one -- prefer chain name but fall back to spec name.
	if cfg.ChainID == "" {
		prefix := s.ChainName
		if prefix == "" {
			prefix = s.Name
		}
		cfg.ChainID = prefix + s.suffix()
	}

	if s.GasAdjustment != nil {
		cfg.GasAdjustment = *s.GasAdjustment
	}
	if s.NoHostMount != nil {
		cfg.NoHostMount = *s.NoHostMount
	}
	if s.SkipGenTx {
		cfg.SkipGenTx = true
	}
	if s.ModifyGenesis != nil {
		cfg.ModifyGenesis = s.ModifyGenesis
	}
	if s.PreGenesis != nil {
		cfg.PreGenesis = s.PreGenesis
	}
	if s.ModifyGenesisAmounts != nil {
		cfg.ModifyGenesisAmounts = s.ModifyGenesisAmounts
	}

	cfg.UsingChainIDFlagCLI = s.UsingChainIDFlagCLI

	if cfg.CoinDecimals == nil {
		evm := int64(18)
		cosmos := int64(6)

		switch cfg.CoinType {
		case "60":
			cfg.CoinDecimals = &evm
		case "118":
			cfg.CoinDecimals = &cosmos
		case "330":
			cfg.CoinDecimals = &cosmos
		case "529":
			cfg.CoinDecimals = &cosmos

		}
	}

	if s.Version != "" && len(cfg.Images) > 0 {
		cfg.Images[0].Version = s.Version
	}

	return &cfg, nil
}

// suffix returns the automatically generated, concurrency-safe suffix for
// generating a chain name or chain ID.
func (s *ChainSpec) suffix() string {
	s.autoSuffixOnce.Do(func() {
		s.autoSuffix = fmt.Sprintf("-%d", atomic.AddInt32(&suffixCounter, 1))
	})

	return s.autoSuffix
}

// suffixCounter is a package-level counter for safely generating unique suffixes per execution environment.
var suffixCounter int32
