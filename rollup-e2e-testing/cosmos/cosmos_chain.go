package cosmos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/types"
	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	paramsutils "github.com/cosmos/cosmos-sdk/x/params/client/utils"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	chanTypes "github.com/cosmos/ibc-go/v6/modules/core/04-channel/types"
	"github.com/decentrio/rollup-e2e-testing/blockdb"
	"github.com/decentrio/rollup-e2e-testing/dockerutil"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	dockertypes "github.com/docker/docker/api/types"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ ibc.Chain = (*CosmosChain)(nil)

// CosmosChain is a local docker testnet for a Cosmos SDK chain.
// Implements the ibc.Chain interface.
type CosmosChain struct {
	testName      string
	cfg           ibc.ChainConfig
	numValidators int
	numFullNodes  int
	Validators    Nodes
	FullNodes     Nodes

	log      *zap.Logger
	keyring  keyring.Keyring
	findTxMu sync.Mutex
}

func NewCosmosChain(testName string, chainConfig ibc.ChainConfig, numValidators int, numFullNodes int, log *zap.Logger) *CosmosChain {
	if chainConfig.EncodingConfig == nil {
		cfg := DefaultEncoding()
		chainConfig.EncodingConfig = &cfg
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kr := keyring.NewInMemory(cdc)

	return &CosmosChain{
		testName:      testName,
		cfg:           chainConfig,
		numValidators: numValidators,
		numFullNodes:  numFullNodes,
		log:           log,
		keyring:       kr,
	}
}

func (c *CosmosChain) Logger() *zap.Logger {
	return c.log
}

// Nodes returns all nodes, including validators and fullnodes.
func (c *CosmosChain) Nodes() Nodes {
	return append(c.Validators, c.FullNodes...)
}

// AddFullNodes adds new fullnodes to the network, peering with the existing nodes.
func (c *CosmosChain) AddFullNodes(ctx context.Context, configFileOverrides map[string]any, inc int) error {
	// Get peer string for existing nodes
	peers := c.Nodes().PeerString(ctx)

	// Get genesis.json
	genbz, err := c.Validators[0].GenesisFileContent(ctx)
	if err != nil {
		return err
	}

	prevCount := c.numFullNodes
	c.numFullNodes += inc
	if err := c.initializeNodes(ctx, c.testName, c.getFullNode().DockerClient, c.getFullNode().NetworkID); err != nil {
		return err
	}

	var eg errgroup.Group
	for i := prevCount; i < c.numFullNodes; i++ {
		i := i
		eg.Go(func() error {
			fn := c.FullNodes[i]
			if err := fn.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			if err := fn.SetPeers(ctx, peers); err != nil {
				return err
			}
			if err := fn.OverwriteGenesisFile(ctx, genbz); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("Provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := testutil.ModifyTomlConfigFile(
					ctx,
					fn.logger(),
					fn.DockerClient,
					fn.TestName,
					fn.VolumeName,
					fn.Chain.Config().Name,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			if err := fn.CreateNodeContainer(ctx); err != nil {
				return err
			}
			return fn.StartContainer(ctx)
		})
	}
	return eg.Wait()
}

// Implements Chain interface
func (c *CosmosChain) Config() ibc.ChainConfig {
	return c.cfg
}

// Implements Chain interface
func (c *CosmosChain) Initialize(ctx context.Context, testName string, cli *client.Client, networkID string) error {
	return c.initializeNodes(ctx, testName, cli, networkID)
}

func (c *CosmosChain) getFullNode() *Node {
	c.findTxMu.Lock()
	defer c.findTxMu.Unlock()
	if len(c.FullNodes) > 0 {
		// use first full node
		return c.FullNodes[0]
	}
	// use first validator
	return c.Validators[0]
}

func (c *CosmosChain) GetNode() *Node {
	return c.Validators[0]
}

// Exec implements ibc.Chain.
func (c *CosmosChain) Exec(ctx context.Context, cmd []string, env []string) (stdout, stderr []byte, err error) {
	return c.getFullNode().Exec(ctx, cmd, env)
}

// Implements Chain interface
func (c *CosmosChain) GetRPCAddress() string {
	return fmt.Sprintf("http://%s:26657", c.getFullNode().HostName())
}

// Implements Chain interface
func (c *CosmosChain) GetAPIAddress() string {
	return fmt.Sprintf("http://%s:1317", c.getFullNode().HostName())
}

// Implements Chain interface
func (c *CosmosChain) GetGRPCAddress() string {
	return fmt.Sprintf("%s:9090", c.getFullNode().HostName())
}

// GetHostRPCAddress returns the address of the RPC server accessible by the host.
// This will not return a valid address until the chain has been started.
func (c *CosmosChain) GetHostRPCAddress() string {
	return "http://" + c.getFullNode().hostRPCPort
}

// GetHostAPIAddress returns the address of the REST API server accessible by the host.
// This will not return a valid address until the chain has been started.
func (c *CosmosChain) GetHostAPIAddress() string {
	return "http://" + c.getFullNode().hostAPIPort
}

// GetHostGRPCAddress returns the address of the gRPC server accessible by the host.
// This will not return a valid address until the chain has been started.
func (c *CosmosChain) GetHostGRPCAddress() string {
	return c.getFullNode().hostGRPCPort
}

// HomeDir implements ibc.Chain.
func (c *CosmosChain) HomeDir() string {
	return c.getFullNode().HomeDir()
}

func (c *CosmosChain) GetChainID() string {
	return c.cfg.ChainID
}

// Implements Chain interface
func (c *CosmosChain) CreateKey(ctx context.Context, keyName string) error {
	return c.getFullNode().CreateKey(ctx, keyName)
}

// Implements Chain interface
func (c *CosmosChain) CreateKeyWithKeyDir(ctx context.Context, name string, keyDir string) error {
	return c.getFullNode().CreateKeyWithKeyDir(ctx, name, keyDir)
}

// Implements Chain interface
func (c *CosmosChain) AccountKeyBech32WithKeyDir(ctx context.Context, keyName string, keyDir string) (string, error) {
	return c.getFullNode().AccountKeyBech32WithKeyDir(ctx, keyName, keyDir)
}

// Implements Chain interface
func (c *CosmosChain) RecoverKey(ctx context.Context, keyName, mnemonic string) error {
	return c.getFullNode().RecoverKey(ctx, keyName, mnemonic)
}

// Implements Chain interface
func (c *CosmosChain) GetAddress(ctx context.Context, keyName string) ([]byte, error) {
	b32Addr, err := c.getFullNode().AccountKeyBech32(ctx, keyName)
	if err != nil {
		return nil, err
	}

	return types.GetFromBech32(b32Addr, c.Config().Bech32Prefix)
}

// BuildWallet will return a Cosmos wallet
// If mnemonic != "", it will restore using that mnemonic
// If mnemonic == "", it will create a new key
func (c *CosmosChain) BuildWallet(ctx context.Context, keyName string, mnemonic string) (ibc.Wallet, error) {
	if mnemonic != "" {
		if err := c.RecoverKey(ctx, keyName, mnemonic); err != nil {
			return nil, fmt.Errorf("failed to recover key with name %q on chain %s: %w", keyName, c.cfg.Name, err)
		}
	} else {
		if err := c.CreateKey(ctx, keyName); err != nil {
			return nil, fmt.Errorf("failed to create key with name %q on chain %s: %w", keyName, c.cfg.Name, err)
		}
	}

	addrBytes, err := c.GetAddress(ctx, keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get account address for key %q on chain %s: %w", keyName, c.cfg.Name, err)
	}

	return NewWallet(keyName, addrBytes, mnemonic, c.cfg), nil
}

// BuildRelayerWallet will return a Cosmos wallet populated with the mnemonic so that the wallet can
// be restored in the relayer node using the mnemonic. After it is built, that address is included in
// genesis with some funds.
func (c *CosmosChain) BuildRelayerWallet(ctx context.Context, keyName string) (ibc.Wallet, error) {
	coinType, err := strconv.ParseUint(c.cfg.CoinType, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid coin type: %w", err)
	}

	info, mnemonic, err := c.keyring.NewMnemonic(
		keyName,
		keyring.English,
		hd.CreateHDPath(uint32(coinType), 0, 0).String(),
		"", // Empty passphrase.
		hd.Secp256k1,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mnemonic: %w", err)
	}

	addrBytes, err := info.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}

	return NewWallet(keyName, addrBytes, mnemonic, c.cfg), nil
}

// Implements Chain interface
func (c *CosmosChain) SendFunds(ctx context.Context, keyName string, amount ibc.WalletData) error {
	return c.getFullNode().SendFunds(ctx, keyName, amount)
}

// Implements Chain interface
func (c *CosmosChain) SendIBCTransfer(
	ctx context.Context,
	channelID string,
	keyName string,
	toWallet ibc.WalletData,
	options ibc.TransferOptions,
) (tx ibc.Tx, _ error) {
	txHash, err := c.getFullNode().SendIBCTransfer(ctx, channelID, keyName, toWallet, options)
	if err != nil {
		return tx, fmt.Errorf("send ibc transfer: %w", err)
	}
	txResp, err := c.GetTransaction(txHash)
	if err != nil {
		return tx, fmt.Errorf("failed to get transaction %s: %w", txHash, err)
	}
	if txResp.Code != 0 {
		return tx, fmt.Errorf("error in transaction (code: %d): %s", txResp.Code, txResp.RawLog)
	}
	tx.Height = uint64(txResp.Height)
	tx.TxHash = txHash
	// In cosmos, user is charged for entire gas requested, not the actual gas used.
	tx.GasSpent = txResp.GasWanted

	const evType = "send_packet"
	events := txResp.Events

	var (
		seq, _           = AttributeValue(events, evType, "packet_sequence")
		srcPort, _       = AttributeValue(events, evType, "packet_src_port")
		srcChan, _       = AttributeValue(events, evType, "packet_src_channel")
		dstPort, _       = AttributeValue(events, evType, "packet_dst_port")
		dstChan, _       = AttributeValue(events, evType, "packet_dst_channel")
		timeoutHeight, _ = AttributeValue(events, evType, "packet_timeout_height")
		timeoutTs, _     = AttributeValue(events, evType, "packet_timeout_timestamp")
		data, _          = AttributeValue(events, evType, "packet_data")
	)
	tx.Packet.SourcePort = srcPort
	tx.Packet.SourceChannel = srcChan
	tx.Packet.DestPort = dstPort
	tx.Packet.DestChannel = dstChan
	tx.Packet.TimeoutHeight = timeoutHeight
	tx.Packet.Data = []byte(data)

	seqNum, err := strconv.Atoi(seq)
	if err != nil {
		return tx, fmt.Errorf("invalid packet sequence from events %s: %w", seq, err)
	}
	tx.Packet.Sequence = uint64(seqNum)

	timeoutNano, err := strconv.ParseUint(timeoutTs, 10, 64)
	if err != nil {
		return tx, fmt.Errorf("invalid packet timestamp timeout %s: %w", timeoutTs, err)
	}
	tx.Packet.TimeoutTimestamp = ibc.Nanoseconds(timeoutNano)

	return tx, nil
}

// QueryProposal returns the state and details of a governance proposal.
func (c *CosmosChain) QueryProposal(ctx context.Context, proposalID string) (*ProposalResponse, error) {
	return c.getFullNode().QueryProposal(ctx, proposalID)
}

// UpgradeProposal submits a software-upgrade governance proposal to the chain.
func (c *CosmosChain) UpgradeLegacyProposal(ctx context.Context, keyName string, prop SoftwareUpgradeProposal) (tx TxProposal, _ error) {
	txHash, err := c.getFullNode().UpgradeLegacyProposal(ctx, keyName, prop)
	if err != nil {
		return tx, fmt.Errorf("failed to submit upgrade proposal: %w", err)
	}
	return c.txProposal(txHash)
}

// TextProposal submits a text governance proposal to the chain.
func (c *CosmosChain) TextProposal(ctx context.Context, keyName string, prop TextProposal) (tx TxProposal, _ error) {
	txHash, err := c.getFullNode().TextProposal(ctx, keyName, prop)
	if err != nil {
		return tx, fmt.Errorf("failed to submit upgrade proposal: %w", err)
	}
	return c.txProposal(txHash)
}

// ParamChangeProposal submits a param change proposal to the chain, signed by keyName.
func (c *CosmosChain) ParamChangeProposal(ctx context.Context, keyName string, prop *paramsutils.ParamChangeProposalJSON) (tx TxProposal, _ error) {
	txHash, err := c.getFullNode().ParamChangeProposal(ctx, keyName, prop)
	if err != nil {
		return tx, fmt.Errorf("failed to submit param change proposal: %w", err)
	}

	return c.txProposal(txHash)
}

// SubmitFraudProposal submit a fraud proposal.
func (c *CosmosChain) SubmitFraudProposal(ctx context.Context, keyName, rollappChainID, height, proposerAddr, clientId, title, descrition, deposit string) error {
	_, err := c.getFullNode().SubmitFraudProposal(ctx, keyName, rollappChainID, height, proposerAddr, clientId, title, descrition, deposit)
	if err != nil {
		return fmt.Errorf("failed to submit fraud proposal: %w", err)
	}

	return nil
}

// QueryParam returns the param state of a given key.
func (c *CosmosChain) QueryParam(ctx context.Context, subspace, key string) (*ParamChange, error) {
	return c.getFullNode().QueryParam(ctx, subspace, key)
}

func (c *CosmosChain) txProposal(txHash string) (tx TxProposal, _ error) {
	txResp, err := c.GetTransaction(txHash)
	if err != nil {
		return tx, fmt.Errorf("failed to get transaction %s: %w", txHash, err)
	}
	tx.Height = uint64(txResp.Height)
	tx.TxHash = txHash
	// In cosmos, user is charged for entire gas requested, not the actual gas used.
	tx.GasSpent = txResp.GasWanted
	events := txResp.Events

	tx.DepositAmount, _ = AttributeValue(events, "proposal_deposit", "amount")

	evtSubmitProp := "submit_proposal"
	tx.ProposalID, _ = AttributeValue(events, evtSubmitProp, "proposal_id")
	tx.ProposalType, _ = AttributeValue(events, evtSubmitProp, "proposal_type")

	return tx, nil
}

// ExportState exports the chain state at specific height.
// Implements Chain interface
func (c *CosmosChain) ExportState(ctx context.Context, height int64) (string, error) {
	return c.getFullNode().ExportState(ctx, height)
}

// GetBalance fetches the current balance for a specific account address and denom.
// Implements Chain interface
func (c *CosmosChain) GetBalance(ctx context.Context, address string, denom string) (sdkmath.Int, error) {
	params := &bankTypes.QueryBalanceRequest{Address: address, Denom: denom}
	grpcAddress := c.getFullNode().hostGRPCPort
	conn, err := grpc.Dial(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

// AllBalances fetches an account address's balance for all denoms it holds
func (c *CosmosChain) AllBalances(ctx context.Context, address string) (types.Coins, error) {
	params := bankTypes.QueryAllBalancesRequest{Address: address}
	grpcAddress := c.getFullNode().hostGRPCPort
	conn, err := grpc.Dial(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	queryClient := bankTypes.NewQueryClient(conn)
	res, err := queryClient.AllBalances(ctx, &params)

	if err != nil {
		return nil, err
	}

	return res.GetBalances(), nil
}

func (c *CosmosChain) GetTransaction(txhash string) (*types.TxResponse, error) {
	fn := c.getFullNode()
	return fn.getTransaction(fn.CliContext(), txhash)
}

func (c *CosmosChain) GetGasFeesInNativeDenom(gasPaid int64) int64 {
	gasPrice, _ := strconv.ParseFloat(strings.Replace(c.cfg.GasPrices, c.cfg.Denom, "", 1), 64)
	fees := float64(gasPaid) * gasPrice
	return int64(fees)
}

func (c *CosmosChain) UpgradeVersion(ctx context.Context, cli *client.Client, containerRepo, version string) {
	c.cfg.Images[0].Version = version
	for _, n := range c.Validators {
		n.Image.Version = version
		n.Image.Repository = containerRepo
	}
	for _, n := range c.FullNodes {
		n.Image.Version = version
		n.Image.Repository = containerRepo
	}
	c.pullImages(ctx, cli)
}

func (c *CosmosChain) pullImages(ctx context.Context, cli *client.Client) {
	for _, image := range c.Config().Images {
		rc, err := cli.ImagePull(
			ctx,
			image.Repository+":"+image.Version,
			dockertypes.ImagePullOptions{},
		)
		if err != nil {
			c.log.Error("Failed to pull image",
				zap.Error(err),
				zap.String("repository", image.Repository),
				zap.String("tag", image.Version),
			)
		} else {
			_, _ = io.Copy(io.Discard, rc)
			_ = rc.Close()
		}
	}
}

// NewNode constructs a new cosmos chain node with a docker volume.
func (c *CosmosChain) NewNode(
	ctx context.Context,
	testName string,
	cli *client.Client,
	networkID string,
	image ibc.DockerImage,
	validator bool,
	index int,
) (*Node, error) {
	// Construct the Node first so we can access its name.
	// The Node's VolumeName cannot be set until after we create the volume.
	node := NewNode(c.log, validator, c, cli, networkID, testName, image, index)

	v, err := cli.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{
			dockerutil.CleanupLabel: testName,

			dockerutil.NodeOwnerLabel: node.Name(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume for chain node: %w", err)
	}
	node.VolumeName = v.Name

	if err := dockerutil.SetVolumeOwner(ctx, dockerutil.VolumeOwnerOptions{
		Log: c.log,

		Client: cli,

		VolumeName: v.Name,
		ChainName:  node.Chain.Config().Name,
		ImageRef:   image.Ref(),
		TestName:   testName,
		UidGid:     image.UidGid,
	}); err != nil {
		return nil, fmt.Errorf("set volume owner: %w", err)
	}

	return node, nil
}

// creates the test node objects required for bootstrapping tests
func (c *CosmosChain) initializeNodes(
	ctx context.Context,
	testName string,
	cli *client.Client,
	networkID string,
) error {
	chainCfg := c.Config()
	c.pullImages(ctx, cli)
	image := chainCfg.Images[0]

	newVals := make(Nodes, c.numValidators)
	copy(newVals, c.Validators)
	newFullNodes := make(Nodes, c.numFullNodes)
	copy(newFullNodes, c.FullNodes)

	eg, egCtx := errgroup.WithContext(ctx)
	for i := len(c.Validators); i < c.numValidators; i++ {
		i := i
		eg.Go(func() error {
			val, err := c.NewNode(egCtx, testName, cli, networkID, image, true, i)
			if err != nil {
				return err
			}
			newVals[i] = val
			return nil
		})
	}
	for i := len(c.FullNodes); i < c.numFullNodes; i++ {
		i := i
		eg.Go(func() error {
			fn, err := c.NewNode(egCtx, testName, cli, networkID, image, false, i)
			if err != nil {
				return err
			}
			newFullNodes[i] = fn
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	c.findTxMu.Lock()
	defer c.findTxMu.Unlock()
	c.Validators = newVals
	c.FullNodes = newFullNodes
	return nil
}

type GenesisValidatorPubKey struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
type GenesisValidators struct {
	Address string                 `json:"address"`
	Name    string                 `json:"name"`
	Power   string                 `json:"power"`
	PubKey  GenesisValidatorPubKey `json:"pub_key"`
}
type GenesisFile struct {
	Validators []GenesisValidators `json:"validators"`
}

type ValidatorWithIntPower struct {
	Address      string
	Power        int64
	PubKeyBase64 string
}

// Start bootstraps the hubs and starts it from genesis
func (c *CosmosChain) Start(testName string, ctx context.Context, additionalGenesisWallets ...ibc.WalletData) error {
	chainCfg := c.Config()

	decimalPow := int64(math.Pow10(int(*chainCfg.CoinDecimals)))

	genesisAmount := types.Coin{
		Amount: sdkmath.NewInt(100_000_000_000_000).MulRaw(decimalPow),
		Denom:  chainCfg.Denom,
	}

	genesisSelfDelegation := types.Coin{
		Amount: sdkmath.NewInt(50_000_000_000_000).MulRaw(decimalPow),
		Denom:  chainCfg.Denom,
	}

	if chainCfg.ModifyGenesisAmounts != nil {
		genesisAmount, genesisSelfDelegation = chainCfg.ModifyGenesisAmounts()
	}

	genesisAmounts := []types.Coin{genesisAmount}

	configFileOverrides := chainCfg.ConfigFileOverrides

	eg := new(errgroup.Group)
	// Initialize config and sign gentx for each validator.
	for _, v := range c.Validators {
		v := v
		v.Validator = true
		eg.Go(func() error {
			if err := v.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("Provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := testutil.ModifyTomlConfigFile(
					ctx,
					v.logger(),
					v.DockerClient,
					v.TestName,
					v.VolumeName,
					v.Chain.Config().Name,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			if !c.cfg.SkipGenTx {
				return v.InitValidatorGenTx(ctx, &chainCfg, genesisAmounts, genesisSelfDelegation)
			}
			return nil
		})
	}

	// Initialize config for each full node.
	for _, n := range c.FullNodes {
		n := n
		n.Validator = false
		eg.Go(func() error {
			if err := n.InitFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(testutil.Toml)
				if !ok {
					return fmt.Errorf("Provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := testutil.ModifyTomlConfigFile(
					ctx,
					n.logger(),
					n.DockerClient,
					n.TestName,
					n.VolumeName,
					n.Chain.Config().Name,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// wait for this to finish
	if err := eg.Wait(); err != nil {
		return err
	}

	if c.cfg.PreGenesis != nil {
		err := c.cfg.PreGenesis(chainCfg)
		if err != nil {
			return err
		}
	}

	// for the validators we need to collect the gentxs and the accounts
	// to the first node's genesis file
	validator0 := c.Validators[0]
	for i := 1; i < len(c.Validators); i++ {
		validatorN := c.Validators[i]

		bech32, err := validatorN.AccountKeyBech32(ctx, valKey)
		if err != nil {
			return err
		}

		if err := validator0.AddGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
			return err
		}

		if !c.cfg.SkipGenTx {
			if err := validatorN.copyGentx(ctx, validator0); err != nil {
				return err
			}
		}
	}

	for _, wallet := range additionalGenesisWallets {
		if err := validator0.AddGenesisAccount(ctx, wallet.Address, []types.Coin{{Denom: wallet.Denom, Amount: wallet.Amount}}); err != nil {
			return err
		}
	}

	if !c.cfg.SkipGenTx {
		if err := validator0.CollectGentxs(ctx); err != nil {
			return err
		}
	}

	genbz, err := validator0.GenesisFileContent(ctx)
	if err != nil {
		return err
	}

	genbz = bytes.ReplaceAll(genbz, []byte(`"stake"`), []byte(fmt.Sprintf(`"%s"`, chainCfg.Denom)))

	if c.cfg.ModifyGenesis != nil {
		genbz, err = c.cfg.ModifyGenesis(chainCfg, genbz)
		if err != nil {
			return err
		}
	}

	// Provide EXPORT_GENESIS_FILE_PATH and EXPORT_GENESIS_CHAIN to help debug genesis file
	exportGenesis := os.Getenv("EXPORT_GENESIS_FILE_PATH")
	exportGenesisChain := os.Getenv("EXPORT_GENESIS_CHAIN")
	if exportGenesis != "" && exportGenesisChain == c.cfg.Name {
		c.log.Debug("Exporting genesis file",
			zap.String("chain", exportGenesisChain),
			zap.String("path", exportGenesis),
		)
		_ = os.WriteFile(exportGenesis, genbz, 0600)
	}

	nodes := c.Nodes()

	for _, node := range nodes {
		if err := node.OverwriteGenesisFile(ctx, genbz); err != nil {
			return err
		}
	}

	if err := nodes.LogGenesisHashes(ctx); err != nil {
		return err
	}

	eg, egCtx := errgroup.WithContext(ctx)
	for _, n := range nodes {
		n := n
		eg.Go(func() error {
			return n.CreateNodeContainer(egCtx)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	peers := nodes.PeerString(ctx)

	eg, egCtx = errgroup.WithContext(ctx)
	for _, n := range nodes {
		n := n
		c.log.Info("Starting container", zap.String("container", n.Name()))
		eg.Go(func() error {
			if err := n.SetPeers(egCtx, peers); err != nil {
				return err
			}
			return n.StartContainer(egCtx)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	// Wait for 5 blocks before considering the chains "started"
	return testutil.WaitForBlocks(ctx, 5, c.getFullNode())
}

// Height implements ibc.Chain
func (c *CosmosChain) Height(ctx context.Context) (uint64, error) {
	return c.getFullNode().Height(ctx)
}

// Acknowledgements implements ibc.Chain, returning all acknowledgments in block at height
func (c *CosmosChain) Acknowledgements(ctx context.Context, height uint64) ([]ibc.PacketAcknowledgement, error) {
	var acks []*chanTypes.MsgAcknowledgement
	err := rangeBlockMessages(ctx, c.cfg.EncodingConfig.InterfaceRegistry, c.getFullNode().Client, height, func(msg types.Msg) bool {
		found, ok := msg.(*chanTypes.MsgAcknowledgement)
		if ok {
			acks = append(acks, found)
		}
		return false
	})
	if err != nil {
		return nil, fmt.Errorf("find acknowledgements at height %d: %w", height, err)
	}
	ibcAcks := make([]ibc.PacketAcknowledgement, len(acks))
	for i, ack := range acks {
		ack := ack
		ibcAcks[i] = ibc.PacketAcknowledgement{
			Acknowledgement: ack.Acknowledgement,
			Packet: ibc.Packet{
				Sequence:         ack.Packet.Sequence,
				SourcePort:       ack.Packet.SourcePort,
				SourceChannel:    ack.Packet.SourceChannel,
				DestPort:         ack.Packet.DestinationPort,
				DestChannel:      ack.Packet.DestinationChannel,
				Data:             ack.Packet.Data,
				TimeoutHeight:    ack.Packet.TimeoutHeight.String(),
				TimeoutTimestamp: ibc.Nanoseconds(ack.Packet.TimeoutTimestamp),
			},
		}
	}
	return ibcAcks, nil
}

// Timeouts implements ibc.Chain, returning all timeouts in block at height
func (c *CosmosChain) Timeouts(ctx context.Context, height uint64) ([]ibc.PacketTimeout, error) {
	var timeouts []*chanTypes.MsgTimeout
	err := rangeBlockMessages(ctx, c.cfg.EncodingConfig.InterfaceRegistry, c.getFullNode().Client, height, func(msg types.Msg) bool {
		found, ok := msg.(*chanTypes.MsgTimeout)
		if ok {
			timeouts = append(timeouts, found)
		}
		return false
	})
	if err != nil {
		return nil, fmt.Errorf("find timeouts at height %d: %w", height, err)
	}
	ibcTimeouts := make([]ibc.PacketTimeout, len(timeouts))
	for i, ack := range timeouts {
		ack := ack
		ibcTimeouts[i] = ibc.PacketTimeout{
			Packet: ibc.Packet{
				Sequence:         ack.Packet.Sequence,
				SourcePort:       ack.Packet.SourcePort,
				SourceChannel:    ack.Packet.SourceChannel,
				DestPort:         ack.Packet.DestinationPort,
				DestChannel:      ack.Packet.DestinationChannel,
				Data:             ack.Packet.Data,
				TimeoutHeight:    ack.Packet.TimeoutHeight.String(),
				TimeoutTimestamp: ibc.Nanoseconds(ack.Packet.TimeoutTimestamp),
			},
		}
	}
	return ibcTimeouts, nil
}

// FindTxs implements blockdb.BlockSaver.
func (c *CosmosChain) FindTxs(ctx context.Context, height uint64) ([]blockdb.Tx, error) {
	fn := c.getFullNode()
	c.findTxMu.Lock()
	defer c.findTxMu.Unlock()
	return fn.FindTxs(ctx, height)
}

// StopAllNodes stops and removes all long running containers (validators and full nodes)
func (c *CosmosChain) StopAllNodes(ctx context.Context) error {
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		n := n
		eg.Go(func() error {
			if err := n.StopContainer(ctx); err != nil {
				return err
			}
			return n.RemoveContainer(ctx)
		})
	}
	return eg.Wait()
}

// StartAllNodes creates and starts new containers for each node.
// Should only be used if the chain has previously been started with .Start.
func (c *CosmosChain) StartAllNodes(ctx context.Context) error {
	// prevent client calls during this time
	c.findTxMu.Lock()
	defer c.findTxMu.Unlock()
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		n := n
		eg.Go(func() error {
			if err := n.CreateNodeContainer(ctx); err != nil {
				return err
			}
			return n.StartContainer(ctx)
		})
	}
	return eg.Wait()
}

func (c *CosmosChain) VoteOnProposalAllValidators(ctx context.Context, proposalID string, vote string) error {
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		if n.Validator {
			n := n
			eg.Go(func() error {
				return n.VoteOnProposal(ctx, valKey, proposalID, vote)
			})
		}
	}
	return eg.Wait()
}

// IBCTransfer compose an IBC transfer and send from chainA -> chainB
func (c *CosmosChain) IBCTransfer(ctx context.Context, chainA, chainB ibc.Chain, transferAmount sdkmath.Int, chainAUserAddr,
	chainBUserAddr string, rly ibc.Relayer, ibcPath string, channel *ibc.ChannelOutput,
	eRep *testreporter.RelayerExecReporter, options ibc.TransferOptions) error {
	transfer := ibc.WalletData{
		Address: chainBUserAddr,
		Denom:   chainA.Config().Denom,
		Amount:  transferAmount,
	}

	// Get original account balances
	chainAOrigBal, err := chainA.GetBalance(ctx, chainAUserAddr, chainA.Config().Denom)
	if err != nil {
		return err
	}

	// Get the IBC denom
	chainATokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, chainA.Config().Denom)
	chainAIBCDenom := transfertypes.ParseDenomTrace(chainATokenDenom).IBCDenom()

	chainATX, err := chainA.SendIBCTransfer(ctx, channel.ChannelID, chainAUserAddr, transfer, options)
	if err != nil {
		return err
	}

	err = chainATX.Validate()
	if err != nil {
		return fmt.Errorf("chain-a ibc transfer tx is invalid: %w ", err)
	}

	err = rly.StartRelayer(ctx, eRep, ibcPath)
	if err != nil {
		return err
	}

	err = testutil.WaitForBlocks(ctx, 20, chainA, chainB)
	if err != nil {
		return err
	}

	chainAUpdateBal, err := chainA.GetBalance(ctx, chainAUserAddr, chainA.Config().Denom)
	if err != nil {
		return err
	}

	chainBUpdateBal, err := chainB.GetBalance(ctx, chainBUserAddr, chainAIBCDenom)
	if err != nil {
		return err
	}

	if !chainAOrigBal.Equal(chainAUpdateBal.Add(transferAmount)) {
		return fmt.Errorf("Balance not change")
	}

	if !chainBUpdateBal.Equal(transferAmount) {
		return fmt.Errorf("Balance not change")
	}

	err = rly.StopRelayer(ctx, eRep)
	if err != nil {
		return err
	}

	err = testutil.WaitForBlocks(ctx, 10, chainA, chainB)
	if err != nil {
		return err
	}

	return nil
}
