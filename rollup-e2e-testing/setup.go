package rollupe2etesting

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"cosmossdk.io/math"
	"github.com/decentrio/rollup-e2e-testing/dockerutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Setup represents a full IBC network, encompassing a collection of
// one or more chains, one or more relayer instances, and initial account configuration.
type Setup struct {
	log *zap.Logger

	// Map of chain reference to chain ID.
	chains map[ibc.Chain]string

	// Map of relayer reference to user-supplied instance name.
	relayers map[ibc.Relayer]string

	// Key: relayer and path name; Value: the two chains being linked.
	links map[relayerPath]Link

	// Map of relayer-chain pairs to address and mnemonic, set during Build().
	// Not yet exposed through any exported API.
	relayerWallets map[relayerChain]ibc.Wallet

	// Map of chain to additional genesis wallets to include at chain start.
	AdditionalGenesisWallets map[ibc.Chain][]ibc.WalletData

	// Set during Build and cleaned up in the Close method.
	cs *chainSet
}

type Link struct {
	chains [2]ibc.Chain
	// If set, these options will be used when creating the client in the path link step.
	// If a zero value initialization is used, e.g. CreateClientOptions{},
	// then the default values will be used via ibc.DefaultClientOpts.
	createClientOpts ibc.CreateClientOptions

	// If set, these options will be used when creating the channel in the path link step.
	// If a zero value initialization is used, e.g. CreateChannelOptions{},
	// then the default values will be used via ibc.DefaultChannelOpts.
	createChannelOpts ibc.CreateChannelOptions
}

// NewSetup returns a new Setup.
//
// Typical usage involves multiple calls to AddChain, one or more calls to AddRelayer,
// one or more calls to AddLink, and then finally a single call to Build.
func NewSetup() *Setup {
	return &Setup{
		log: zap.NewNop(),

		chains:   make(map[ibc.Chain]string),
		relayers: make(map[ibc.Relayer]string),

		links: make(map[relayerPath]Link),
	}
}

// relayerPath is a tuple of a relayer and a path name.
type relayerPath struct {
	Relayer ibc.Relayer
	Path    string
}

func (s *Setup) AddRollUp(hub ibc.Chain, rollApps ...ibc.Chain) *Setup {
	h, ok := hub.(ibc.Hub)
	if !ok {
		panic("Error Hub chain")
	}

	s.AddChain(hub)

	for _, rollApp := range rollApps {
		a, ok := rollApp.(ibc.RollApp)
		if !ok {
			panic("Error RollApp chain")
		}

		h.SetRollApp(a)

		s.AddChain(rollApp)
	}
	return s
}

// AddChain adds the given chain to the Setup,
// using the chain ID reported by the chain's config.
// If the given chain already exists,
// or if another chain with the same configured chain ID exists, AddChain panics.
func (s *Setup) AddChain(chain ibc.Chain, additionalGenesisWallets ...ibc.WalletData) *Setup {
	if chain == nil {
		panic(fmt.Errorf("cannot add nil chain"))
	}

	newID := chain.Config().ChainID
	newName := chain.Config().Name

	for c, id := range s.chains {
		if c == chain {
			panic(fmt.Errorf("chain %v was already added", c))
		}
		if id == newID {
			panic(fmt.Errorf("a chain with ID %s already exists", id))
		}
		if c.Config().Name == newName {
			panic(fmt.Errorf("a chain with name %s already exists", newName))
		}
	}

	s.chains[chain] = newID

	if len(additionalGenesisWallets) == 0 {
		return s
	}

	if s.AdditionalGenesisWallets == nil {
		s.AdditionalGenesisWallets = make(map[ibc.Chain][]ibc.WalletData)
	}
	s.AdditionalGenesisWallets[chain] = additionalGenesisWallets

	return s
}

// AddRelayer adds the given relayer with the given name to the Setup.
func (s *Setup) AddRelayer(relayer ibc.Relayer, name string) *Setup {
	if relayer == nil {
		panic(fmt.Errorf("cannot add nil relayer"))
	}

	for r, n := range s.relayers {
		if r == relayer {
			panic(fmt.Errorf("relayer %v was already added", r))
		}
		if n == name {
			panic(fmt.Errorf("a relayer with name %s already exists", n))
		}
	}

	s.relayers[relayer] = name
	return s
}

// InterchainLink describes a link between two chains,
// by specifying the chain names, the relayer name,
// and the name of the path to create.
type InterchainLink struct {
	// Chains involved.
	Chain1, Chain2 ibc.Chain

	// Relayer to use for link.
	Relayer ibc.Relayer

	// Name of path to create.
	Path string

	// If set, these options will be used when creating the client in the path link step.
	// If a zero value initialization is used, e.g. CreateClientOptions{},
	// then the default values will be used via ibc.DefaultClientOpts.
	CreateClientOpts ibc.CreateClientOptions

	// If set, these options will be used when creating the channel in the path link step.
	// If a zero value initialization is used, e.g. CreateChannelOptions{},
	// then the default values will be used via ibc.DefaultChannelOpts.
	CreateChannelOpts ibc.CreateChannelOptions
}

// AddLink adds the given link to the Setup.
// If any validation fails, AddLink panics.
func (s *Setup) AddLink(link InterchainLink) *Setup {
	if _, exists := s.chains[link.Chain1]; !exists {
		cfg := link.Chain1.Config()
		panic(fmt.Errorf("chain with name=%s and id=%s was never added to Setup", cfg.Name, cfg.ChainID))
	}
	if _, exists := s.chains[link.Chain2]; !exists {
		cfg := link.Chain2.Config()
		panic(fmt.Errorf("chain with name=%s and id=%s was never added to Setup", cfg.Name, cfg.ChainID))
	}
	if _, exists := s.relayers[link.Relayer]; !exists {
		panic(fmt.Errorf("relayer %v was never added to Setup", link.Relayer))
	}

	if link.Chain1 == link.Chain2 {
		panic(fmt.Errorf("chains must be different (both were %v)", link.Chain1))
	}

	key := relayerPath{
		Relayer: link.Relayer,
		Path:    link.Path,
	}

	if _, exists := s.links[key]; exists {
		panic(fmt.Errorf("relayer %q already has a path named %q", key.Relayer, key.Path))
	}

	s.links[key] = Link{
		chains:            [2]ibc.Chain{link.Chain1, link.Chain2},
		createChannelOpts: link.CreateChannelOpts,
		createClientOpts:  link.CreateClientOpts,
	}
	return s
}

// InterchainBuildOptions describes configuration for (*Setup).Build.
type InterchainBuildOptions struct {
	TestName string

	Client    *client.Client
	NetworkID string

	// If set, s.Build does not create paths or links in the relayer,
	// but it does still configure keys and wallets for declared relayer-chain links.
	// This is useful for tests that need lower-level access to configuring relayers.
	SkipPathCreation bool

	// Optional. Git sha for test invocation. Once Go 1.18 supported,
	// may be deprecated in favor of runtime/debug.ReadBuildInfo.
	GitSha string

	// If set, saves block history to a sqlite3 database to aid debugging.
	BlockDatabaseFile string
}

// Build starts all the chains and configures the relayers associated with the Setup.
// It is the caller's responsibility to directly call StartRelayer on the relayer implementations.
//
// Calling Build more than once will cause a panic.
func (s *Setup) Build(ctx context.Context, rep *testreporter.RelayerExecReporter, opts InterchainBuildOptions) error {
	chains := make([]ibc.Chain, 0, len(s.chains))
	for chain := range s.chains {
		chains = append(chains, chain)
	}
	s.cs = newChainSet(s.log, chains)

	// Initialize the chains (pull docker images, etc.).
	if err := s.cs.Initialize(ctx, opts.TestName, opts.Client, opts.NetworkID); err != nil {
		return fmt.Errorf("failed to initialize chains: %w", err)
	}

	err := s.generateRelayerWallets(ctx) // Build the relayer wallet mapping.
	if err != nil {
		return err
	}

	walletAmounts, err := s.genesisWalletAmounts(ctx)
	if err != nil {
		// Error already wrapped with appropriate detail.
		return err
	}

	if err := s.cs.Configuration(ctx, opts.TestName, walletAmounts); err != nil {
		return fmt.Errorf("failed to configuration chains: %w", err)
	}

	if err := s.cs.Start(ctx, opts.TestName, walletAmounts); err != nil {
		return fmt.Errorf("failed to start chains: %w", err)
	}

	if err := s.cs.TrackBlocks(ctx, opts.TestName, opts.BlockDatabaseFile, opts.GitSha); err != nil {
		return fmt.Errorf("failed to track blocks: %w", err)
	}

	if err := s.configureRelayerKeys(ctx, rep); err != nil {
		// Error already wrapped with appropriate detail.
		return err
	}
	for range s.relayerChains() {
		filePath := "/tmp/rly/config/config.yaml"

		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Fatalf("Failed to read file: %s", err)
		}

		err = os.WriteFile(filePath, []byte(strings.ReplaceAll(string(content), `extra-codecs: []`, `extra-codecs: ["ethermint"]`)), 0644)
		if err != nil {
			log.Fatalf("Failed to write to file: %s", err)
		}
	}
	// Some tests may want to configure the relayer from a lower level,
	// but still have wallets configured.
	if opts.SkipPathCreation {
		return nil
	}

	// For every relayer link, teach the relayer about the link and create the link.
	for rp, link := range s.links {
		rp := rp
		link := link
		c0 := link.chains[0]
		c1 := link.chains[1]

		if err := rp.Relayer.GeneratePath(ctx, rep, c0.Config().ChainID, c1.Config().ChainID, rp.Path); err != nil {
			return fmt.Errorf(
				"failed to generate path %s on relayer %s between chains %s and %s: %w",
				rp.Path, rp.Relayer, s.chains[c0], s.chains[c1], err,
			)
		}
	}

	// Now link the paths in parallel
	// Creates clients, connections, and channels for each link/path.
	var eg errgroup.Group
	for rp, link := range s.links {
		rp := rp
		link := link
		c0 := link.chains[0]
		c1 := link.chains[1]
		eg.Go(func() error {
			// If the user specifies a zero value CreateClientOptions struct then we fall back to the default
			// client options.
			if link.createClientOpts == (ibc.CreateClientOptions{}) {
				link.createClientOpts = ibc.DefaultClientOpts()
			}

			// Check that the client creation options are valid and fully specified.
			if err := link.createClientOpts.Validate(); err != nil {
				return err
			}

			// If the user specifies a zero value CreateChannelOptions struct then we fall back to the default
			// channel options for an ics20 fungible token transfer channel.
			if link.createChannelOpts == (ibc.CreateChannelOptions{}) {
				link.createChannelOpts = ibc.DefaultChannelOpts()
			}

			// Check that the channel creation options are valid and fully specified.
			if err := link.createChannelOpts.Validate(); err != nil {
				return err
			}

			if err := rp.Relayer.LinkPath(ctx, rep, rp.Path, link.createChannelOpts, link.createClientOpts); err != nil {
				return fmt.Errorf(
					"failed to link path %s on relayer %s between chains %s and %s: %w",
					rp.Path, rp.Relayer, s.chains[c0], s.chains[c1], err,
				)
			}
			return nil
		})
	}

	return eg.Wait()
}

// WithLog sets the logger on the interchain object.
// Usually the default nop logger is fine, but sometimes it can be helpful
// to see more verbose logs, typically by passing zaptest.NewLogger(t).
func (s *Setup) WithLog(log *zap.Logger) *Setup {
	s.log = log
	return s
}

// Close cleans up any resources created during Build,
// and returns any relevant errors.
func (s *Setup) Close() error {
	return s.cs.Close()
}

func (s *Setup) genesisWalletAmounts(ctx context.Context) (map[ibc.Chain][]ibc.WalletData, error) {
	// Faucet addresses are created separately because they need to be explicitly added to the chains.
	faucetAddresses, err := s.cs.CreateCommonAccount(ctx, FaucetAccountKeyName)
	if err != nil {
		return nil, fmt.Errorf("failed to create faucet accounts: %w", err)
	}

	// Wallet amounts for genesis.
	walletAmounts := make(map[ibc.Chain][]ibc.WalletData, len(s.cs.chains))

	// Add faucet for each chain first.
	for c := range s.chains {
		// The values are nil at this point, so it is safe to directly assign the slice.
		walletAmounts[c] = []ibc.WalletData{
			{
				Address: faucetAddresses[c],
				Denom:   c.Config().Denom,
				Amount:  math.NewInt(100_000_000_000_000), // Faucet wallet gets 100T units of denom.
			},
		}

		if s.AdditionalGenesisWallets != nil {
			walletAmounts[c] = append(walletAmounts[c], s.AdditionalGenesisWallets[c]...)
		}
	}

	// Then add all defined relayer wallets.
	for rc, wallet := range s.relayerWallets {
		c := rc.C
		walletAmounts[c] = append(walletAmounts[c], ibc.WalletData{
			Address: wallet.FormattedAddress(),
			Denom:   c.Config().Denom,
			Amount:  math.NewInt(1_000_000_000_000), // Every wallet gets 1t units of denom.
		})
	}

	return walletAmounts, nil
}

// generateRelayerWallets populates s.relayerWallets.
func (s *Setup) generateRelayerWallets(ctx context.Context) error {
	if s.relayerWallets != nil {
		panic(fmt.Errorf("cannot call generateRelayerWallets more than once"))
	}

	relayerChains := s.relayerChains()
	s.relayerWallets = make(map[relayerChain]ibc.Wallet, len(relayerChains))
	for r, chains := range relayerChains {
		for _, c := range chains {
			// Just an ephemeral unique name, only for the local use of the keyring.
			accountName := s.relayers[r] + "-" + s.chains[c]
			newWallet, err := c.BuildRelayerWallet(ctx, accountName)
			if err != nil {
				return err
			}
			s.relayerWallets[relayerChain{R: r, C: c}] = newWallet
		}
	}

	return nil
}

// configureRelayerKeys adds the chain configuration for each relayer
// and adds the preconfigured key to the relayer for each relayer-chain.
func (s *Setup) configureRelayerKeys(ctx context.Context, rep *testreporter.RelayerExecReporter) error {
	// Possible optimization: each relayer could be configured concurrently.
	// But we are only testing with a single relayer so far, so we don't need this yet.

	for r, chains := range s.relayerChains() {
		for _, c := range chains {
			rpcAddr, grpcAddr := c.GetRPCAddress(), c.GetGRPCAddress()
			if !r.UseDockerNetwork() {
				rpcAddr, grpcAddr = c.GetHostRPCAddress(), c.GetHostGRPCAddress()
			}

			chainName := s.chains[c]
			if err := r.AddChainConfiguration(ctx,
				rep,
				c.Config(), chainName,
				rpcAddr, grpcAddr,
			); err != nil {
				return fmt.Errorf("failed to configure relayer %s for chain %s: %w", s.relayers[r], chainName, err)
			}

			wallet, err := r.AddKey(ctx,
				rep,
				chainName, chainName,
				c.Config().CoinType,
			)
			if err != nil {
				return fmt.Errorf("failed to add key to relayer %s for chain %s: %w", s.relayers[r], chainName, err)
			}

			err = c.SendFunds(ctx, FaucetAccountKeyName, ibc.WalletData{
				Address: wallet.FormattedAddress(),
				Amount:  math.NewInt(50_000_000_000_000),
				Denom:   c.Config().Denom,
			})
			if err != nil {
				return fmt.Errorf("failed to get funds from faucet: %w", err)
			}
		}
	}

	return nil
}

// relayerChain is a tuple of a Relayer and a Chain.
type relayerChain struct {
	R ibc.Relayer
	C ibc.Chain
}

// relayerChains builds a mapping of relayers to the chains they connect to.
// The order of the chains is arbitrary.
func (s *Setup) relayerChains() map[ibc.Relayer][]ibc.Chain {
	// First, collect a mapping of relayers to sets of chains,
	// so we don't have to manually deduplicate entries.
	uniq := make(map[ibc.Relayer]map[ibc.Chain]struct{}, len(s.relayers))

	for rp, link := range s.links {
		r := rp.Relayer
		if uniq[r] == nil {
			uniq[r] = make(map[ibc.Chain]struct{}, 2) // Adding at least 2 chains per relayer.
		}
		uniq[r][link.chains[0]] = struct{}{}
		uniq[r][link.chains[1]] = struct{}{}
	}

	// Then convert the sets to slices.
	out := make(map[ibc.Relayer][]ibc.Chain, len(uniq))
	for r, chainSet := range uniq {
		chains := make([]ibc.Chain, 0, len(chainSet))
		for chain := range chainSet {
			chains = append(chains, chain)
		}

		out[r] = chains
	}
	return out
}

// GetAndFundTestUsers generates and funds chain users with the native chain denom.
// The caller should wait for some blocks to complete before the funds will be accessible.
func GetAndFundTestUsers(
	t *testing.T,
	ctx context.Context,
	keyNamePrefix string,
	amount math.Int,
	chains ...ibc.Chain,
) []ibc.Wallet {
	users := make([]ibc.Wallet, len(chains))
	var eg errgroup.Group
	for i, chain := range chains {
		i := i
		chain := chain
		eg.Go(func() error {
			user, err := GetAndFundTestUserWithMnemonic(ctx, keyNamePrefix, "", amount, chain)
			if err != nil {
				return err
			}
			users[i] = user
			return nil
		})
	}
	require.NoError(t, eg.Wait())

	chainHeights := make([]testutil.ChainHeighter, len(chains))
	for i := range chains {
		chainHeights[i] = chains[i]
	}
	return users
}

// GetAndFundTestUserWithMnemonic restores a user using the given mnemonic
// and funds it with the native chain denom.
// The caller should wait for some blocks to complete before the funds will be accessible.
func GetAndFundTestUserWithMnemonic(
	ctx context.Context,
	keyNamePrefix, mnemonic string,
	amount math.Int,
	chain ibc.Chain,
) (ibc.Wallet, error) {
	chainCfg := chain.Config()
	keyName := fmt.Sprintf("%s-%s-%s", keyNamePrefix, chainCfg.ChainID, dockerutil.RandLowerCaseLetterString(3))
	user, err := chain.BuildWallet(ctx, keyName, mnemonic)
	if err != nil {
		return nil, fmt.Errorf("failed to get source user wallet: %w", err)
	}

	err = chain.SendFunds(ctx, FaucetAccountKeyName, ibc.WalletData{
		Address: user.FormattedAddress(),
		Amount:  amount,
		Denom:   chainCfg.Denom,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get funds from faucet: %w", err)
	}

	return user, nil
}

const (
	FaucetAccountKeyName = "faucet"
)

// KeepDockerVolumesOnFailure sets whether volumes associated with a particular test
// are retained or deleted following a test failure.
//
// The value is false by default, but can be initialized to true by setting the
// environment variable IBCTEST_SKIP_FAILURE_CLEANUP to a non-empty value.
// Alternatively, importers of the e2e package may call KeepDockerVolumesOnFailure(true).
func KeepDockerVolumesOnFailure(b bool) {
	dockerutil.KeepVolumesOnFailure = b
}

// DockerSetup returns a new Docker Client and the ID of a configured network, associated with t.
//
// If any part of the setup fails, t.Fatal is called.
func DockerSetup(t dockerutil.DockerSetupTestingT) (*client.Client, string) {
	t.Helper()
	return dockerutil.DockerSetup(t)
}
