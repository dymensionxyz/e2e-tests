package rollupe2etesting

import (
	"fmt"

	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	rly "github.com/decentrio/rollup-e2e-testing/relayer/rly"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

type TestName interface {
	Name() string
}

// RelayerFactory describes how to start a Relayer.
type RelayerFactory interface {
	// Build returns a Relayer associated with the given arguments.
	Build(
		t TestName,
		cli *client.Client,
		networkID string,
	) ibc.Relayer

	// Name returns a descriptive name of the factory,
	// indicating details of the Relayer that will be built.
	Name() string
}

// builtinRelayerFactory is the built-in relayer factory that understands
// how to start the cosmos relayer in a docker container.
type builtinRelayerFactory struct {
	impl    ibc.RelayerImplementation
	log     *zap.Logger
	options relayer.RelayerOptions
}

func NewBuiltinRelayerFactory(impl ibc.RelayerImplementation, logger *zap.Logger, options ...relayer.RelayerOption) RelayerFactory {
	return builtinRelayerFactory{impl: impl, log: logger, options: options}
}

// Build returns a relayer chosen depending on f.impl.
func (f builtinRelayerFactory) Build(
	t TestName,
	cli *client.Client,
	networkID string,
) ibc.Relayer {
	switch f.impl {
	case ibc.CosmosRly:
		return rly.NewCosmosRelayer(
			f.log,
			t.Name(),
			cli,
			networkID,
			f.options...,
		)
	default:
		panic(fmt.Errorf("RelayerImplementation %v unknown", f.impl))
	}
}

func (f builtinRelayerFactory) Name() string {
	switch f.impl {
	case ibc.CosmosRly:
		// This is using the string "rly" instead of rly.ContainerImage
		// so that the slashes in the image repository don't add ambiguity
		// to subtest paths, when the factory name is used in calls to t.Run.
		for _, opt := range f.options {
			switch o := opt.(type) {
			case relayer.RelayerOptionDockerImage:
				return "rly@" + o.DockerImage.Version
			}
		}
		return "rly@" + rly.DefaultContainerVersion
	default:
		panic(fmt.Errorf("RelayerImplementation %v unknown", f.impl))
	}
}
