package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
)

func Test_TEE_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	dymintTomlOverrides := make(testutil.Toml)
	dymintTomlOverrides["settlement_layer"] = "dymension"
	dymintTomlOverrides["settlement_node_address"] = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	dymintTomlOverrides["rollapp_id"] = "rollappwasm_1234-1"
	dymintTomlOverrides["settlement_gas_prices"] = "0adym"
	dymintTomlOverrides["max_idle_time"] = "3s"
	dymintTomlOverrides["max_proof_time"] = "500ms"
	dymintTomlOverrides["batch_submit_time"] = "50s"
	dymintTomlOverrides["p2p_blocksync_enabled"] = "false"
	dymintTomlOverrides["da_config"] = []string{""}
	dymintTomlOverrides["da_layer"] = []string{"mock"}

	configFileOverrides := make(map[string]any)
	configFileOverrides["config/dymint.toml"] = dymintTomlOverrides

	// Create chain factory with dymension
	modifyHubGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.app_registration_fee.denom",
			Value: "adym",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.app_registration_fee.amount",
			Value: "1000000000000000000",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.tee_config.enabled",
			Value: true,
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.tee_config.verify",
			Value: true,
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.tee_config.policy_values",
			Value: "",
		},
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.tee_config.policy_query",
			Value: "",
		},

		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.tee_config.policy_structure",
			Value: "",
		},

		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.tee_config.gcp_root_cert_pem",
			Value: "",
		},
	)

	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1

	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:    "rollapp-dym",
				Name:    "rollapp-temp",
				ChainID: "rollappwasm_1234-1",
				Images: []ibc.DockerImage{
					{
						Repository: "ghcr.io/decentrio/rollapp-wasm",
						Version:    "tee",
						UidGid:     "1025:1025",
					},
				},
				Bin:                 "rollappd",
				Bech32Prefix:        "rol",
				Denom:               "urax",
				CoinType:            "118",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappWasmGenesis(rollappWasmGenesisKV),
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "dymension-hub",
			ChainConfig: ibc.ChainConfig{
				Type:                "hub-dym",
				Name:                "dymension",
				ChainID:             "dymension_100-1",
				Images:              []ibc.DockerImage{dymensionImage},
				Bin:                 "dymd",
				Bech32Prefix:        "dym",
				Denom:               "adym",
				CoinType:            "60",
				GasPrices:           "0.0adym",
				EncodingConfig:      encodingConfig(),
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				NoHostMount:         false,
				ModifyGenesis:       modifyDymensionGenesis(modifyHubGenesisKV),
				ConfigFileOverrides: nil,
				SidecarConfigs: []ibc.SidecarConfig{
					{
						ProcessName:      "eibc-client",
						Image:            eibcClientImage,
						HomeDir:          "/root",
						Ports:            nil,
						StartCmd:         []string{"eibc-client", "start", "--config", "/root/.eibc-client/config.yaml"},
						PreStart:         true,
						ValidatorProcess: false,
					},
				},
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	dymension := chains[1].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)

	r := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"), relayer.ImagePull(pullRelayerImage),
	).Build(t, client, "relayer", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1).
		AddRelayer(r, "relayer").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r,
			Path:    ibcPath,
		})

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	err = ic.Build(ctx, eRep, test.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: true,

		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: test.DefaultBlockDatabaseFilepath(),
	}, nil, "", nil, true, 1179360, true)
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/members.json", "members.json")
	require.NoError(t, err)

	err = dymension.Validators[0].CopyFile(ctx, "data/policy.json", "policy.json")
	require.NoError(t, err)

	addrDym, _ := r.GetWallet(dymension.GetChainID())
	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrDym.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	addrRA, _ := r.GetWallet(rollapp1.GetChainID())
	err = rollapp1.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: addrRA.FormattedAddress(),
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   rollapp1.Config().Denom,
	})
	require.NoError(t, err)

	wallet, found := r.GetWallet(rollapp1.Config().ChainID)
	require.True(t, found)

	keyDir := dymension.GetRollApps()[0].GetSequencerKeyDir()
	keyPath := keyDir + "/sequencer_keys"

	//Update white listed relayers
	for i := 0; i < 10; i++ {
		_, err = dymension.GetNode().UpdateWhitelistedRelayers(ctx, "sequencer", keyPath, []string{wallet.FormattedAddress()})
		if err == nil {
			break
		}
		if i == 9 {
			fmt.Println("Max retries reached. Exiting...")
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	CreateChannel(ctx, t, r, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension, rollapp1)

	// Get our Bech32 encoded user addresses
	dymensionUser, lp1, lp2, rollappUser := users[0], users[1], users[2], users[3]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	lp1Addr := lp1.FormattedAddress()
	// lp2Addr := lp2.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	// create operator
	cmd := []string{"keys", "add", "operator",
		"--coin-type", dymension.GetNode().Chain.Config().CoinType,
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}

	_, _, err = dymension.GetNode().ExecBin(ctx, cmd...)
	require.NoError(t, err)

	cmd = []string{dymension.GetNode().Chain.Config().Bin, "keys", "show", "--address", "operator",
		"--home", dymension.GetNode().HomeDir(),
		"--keyring-backend", "test",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	stdout, _, err := dymension.GetNode().Exec(ctx, cmd, nil)
	require.NoError(t, err)

	operatorAddr := string(bytes.TrimSuffix(stdout, []byte("\n")))
	println("Done for set up operator: ", operatorAddr)

	err = dymension.GetNode().SendFunds(ctx, "faucet", ibc.WalletData{
		Address: operatorAddr,
		Amount:  math.NewInt(10_000_000_000_000),
		Denom:   dymension.Config().Denom,
	})
	require.NoError(t, err)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, dymension.Config().ChainID, rollapp1.Config().ChainID)
	require.NoError(t, err)

	err = r.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a normal ibc tx from RA -> Hub
	transferData := ibc.WalletData{
		Address: lp1Addr,
		Denom:   rollapp1.Config().Denom,
		Amount:  bigTransferAmount,
	}
	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, ibc.TransferOptions{})
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	rollappHeight, err := rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount))

	// wait until the packet is finalized
	isFinalized, err := dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err := dymension.GetNode().QueryPendingPacketsByAddress(ctx, lp1Addr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, lp1Addr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get the IBC denom for urax on Hub
	rollappTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, rollapp1.Config().Denom)
	rollappIBCDenom := transfertypes.ParseDenomTrace(rollappTokenDenom).IBCDenom()

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, lp1Addr, rollappIBCDenom, transferData.Amount.Sub(bigBridgingFee))

	// Get the IBC denom
	// dymensionTokenDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, dymension.Config().Denom)
	// dymensionIBCDenom := transfertypes.ParseDenomTrace(dymensionTokenDenom).IBCDenom()

	// // register ibc denom on rollapp1
	// metadata := banktypes.Metadata{
	// 	Description: "IBC token from Dymension",
	// 	DenomUnits: []*banktypes.DenomUnit{
	// 		{
	// 			Denom:    dymensionIBCDenom,
	// 			Exponent: 0,
	// 			Aliases:  []string{"udym"},
	// 		},
	// 		{
	// 			Denom:    "udym",
	// 			Exponent: 6,
	// 		},
	// 	},
	// 	// Setting base as IBC hash denom since bank keepers's SetDenomMetadata uses
	// 	// Base as key path and the IBC hash is what gives this token uniqueness
	// 	// on the executing chain
	// 	Base:    dymensionIBCDenom,
	// 	Display: "udym",
	// 	Name:    "udym",
	// 	Symbol:  "udym",
	// }

	// data := map[string][]banktypes.Metadata{
	// 	"metadata": {metadata},
	// }

	// contentFile, err := json.Marshal(data)
	// require.NoError(t, err)
	// rollapp1.GetNode().WriteFile(ctx, contentFile, "./ibcmetadata.json")
	// deposit := "500000000000" + rollapp1.Config().Denom
	// rollapp1.GetNode().HostName()
	// _, err = rollapp1.GetNode().RegisterIBCTokenDenomProposal(ctx, rollappUser.KeyName(), deposit, rollapp1.GetNode().HomeDir()+"/ibcmetadata.json")
	// require.NoError(t, err)

	// err = rollapp1.VoteOnProposalAllValidators(ctx, "1", cosmos.ProposalVoteYes)
	// require.NoError(t, err, "failed to submit votes")

	// height, err := rollapp1.Height(ctx)
	// require.NoError(t, err, "error fetching height")
	// _, err = cosmos.PollForProposalStatus(ctx, rollapp1.CosmosChain, height, height+30, "1", cosmos.ProposalStatusPassed)
	// require.NoError(t, err, "proposal status did not change to passed")

	// // Compose an IBC transfer and send from dymension -> rollapp
	// transferData = ibc.WalletData{
	// 	Address: rollappUserAddr,
	// 	Denom:   dymension.Config().Denom,
	// 	Amount:  transferAmount.Mul(math.NewInt(5)),
	// }

	// // Compose an IBC transfer and send from Hub -> rollapp
	// _, err = dymension.SendIBCTransfer(ctx, channel.ChannelID, dymensionUserAddr, transferData, ibc.TransferOptions{})
	// require.NoError(t, err)

	// // Assert balance was updated on the hub
	// testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount.Sub(transferData.Amount))

	// err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	// require.NoError(t, err)

	// // Check fund was set to erc20 module account on rollapp
	// erc20MAcc, err := rollapp1.Validators[0].QueryModuleAccount(ctx, "erc20")
	// require.NoError(t, err)
	// erc20MAccAddr := erc20MAcc.Account.BaseAccount.Address
	// rollappErc20MaccBalance, err := rollapp1.GetBalance(ctx, erc20MAccAddr, dymensionIBCDenom)
	// require.NoError(t, err)

	// require.True(t, rollappErc20MaccBalance.Equal(transferData.Amount))
	// require.NoError(t, err)

	// tokenPair, err := rollapp1.GetNode().QueryErc20TokenPair(ctx, dymensionIBCDenom)
	// require.NoError(t, err)
	// require.NotNil(t, tokenPair)

	// // convert erc20
	// _, err = rollapp1.GetNode().ConvertErc20(ctx, rollappUser.KeyName(), tokenPair.Erc20Address, transferData.Amount.String(), rollappUserAddr, rollappUserAddr, rollapp1.Config().ChainID)
	// require.NoError(t, err, "can not convert erc20 to cosmos coin")

	// err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	// require.NoError(t, err)
	// testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, dymensionIBCDenom, transferData.Amount)

	// StartDB(ctx, t, client, network)

	dymHomeDir := strings.Split(dymension.Validators[0].HomeDir(), "/")
	dymFolderName := dymHomeDir[len(dymHomeDir)-1]

	membersData, err := os.ReadFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName))
	require.NoError(t, err)

	var members MembersJSON
	err = json.Unmarshal(membersData, &members)
	require.NoError(t, err)

	newAddress := dymensionUserAddr
	for i := range members.Members {
		members.Members[i].Address = newAddress
	}

	updatedJSON, err := json.MarshalIndent(members, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(fmt.Sprintf("/tmp/%s/members.json", dymFolderName), updatedJSON, 0755)
	require.NoError(t, err)

	cmd = []string{"group", "create-group", "operator", "==A", dymension.GetNode().HomeDir() + "/members.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err := dymension.GetNode().ExecTx(ctx, "operator", cmd...)
	fmt.Println(txHash)
	require.NoError(t, err)

	cmd = []string{"group", "create-group-policy", "operator", "1", "==A", dymension.GetNode().HomeDir() + "/policy.json",
		"--keyring-dir", dymension.GetNode().HomeDir(),
	}
	txHash, err = dymension.GetNode().ExecTx(ctx, "operator", cmd...)

	fmt.Println(txHash)
	require.NoError(t, err)

	testutil.WaitForBlocks(ctx, 5, dymension)

	policiesGroup, err := dymension.GetNode().QueryGroupPoliciesByAdmin(ctx, operatorAddr)
	require.NoError(t, err)
	policyAddr := policiesGroup.GroupPolicies[0].Address

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp1.KeyName(), policyAddr, "1000000"+rollappIBCDenom, "rollappwasm_1234-1", rollappIBCDenom, "0.1", "1000000"+rollappIBCDenom, "0.1", true)
	fmt.Println(txHash)
	require.NoError(t, err)

	txHash, err = dymension.GetNode().GrantAuthorization(ctx, lp2.KeyName(), policyAddr, "10000"+rollappIBCDenom, "rollappwasm_1234-1", rollappIBCDenom, "0.1", "10000"+rollappIBCDenom, "0.1", true)
	fmt.Println(txHash)
	require.NoError(t, err)

	configFile := "data/config.yaml"
	content, err := os.ReadFile(configFile)
	require.NoError(t, err)

	// Unmarshal the YAML content into the Config struct
	var config Config
	err = yaml.Unmarshal(content, &config)
	require.NoError(t, err)

	dymensionHomeDir := strings.Split(dymension.HomeDir(), "/")
	dymensionFolderName := dymensionHomeDir[len(dymensionHomeDir)-1]

	// Modify a field
	config.NodeAddress = fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	config.Gas.Fees = "100adym"
	config.LogLevel = "debug"
	config.OrderPolling.Interval = 30 * time.Second
	config.OrderPolling.Enabled = false
	config.Fulfillers.KeyringBackend = "test"
	config.Fulfillers.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Fulfillers.Scale = 10
	config.Fulfillers.MaxOrdersPerTx = 5
	config.Fulfillers.PolicyAddress = policyAddr
	config.Validation.FallbackLevel = "p2p"
	config.Validation.WaitTime = 5 * time.Minute
	config.Validation.Interval = 10 * time.Second
	config.Operator.AccountName = "operator"
	config.Operator.GroupID = 1
	config.Operator.KeyringBackend = "test"
	config.Operator.KeyringDir = fmt.Sprintf("/root/%s", dymensionFolderName)
	config.Operator.MinFeeShare = "0.1"
	config.Rollapps = map[string]RollappConfig{
		"rollappwasm_1234-1": {
			FullNodes:        []string{fmt.Sprintf("http://ra-rollappwasm_1234-1-fn-0-%s:26657", t.Name())},
			MinConfirmations: 1,
		},
	}
	config.Slack.Enabled = false
	config.Slack.AppToken = ""
	config.Slack.ChannelID = ""

	// Marshal the updated struct back to YAML
	modifiedContent, err := yaml.Marshal(&config)
	require.NoError(t, err)

	err = os.Chmod(configFile, 0777)
	require.NoError(t, err)

	// Write the updated content back to the file
	err = os.WriteFile(configFile, modifiedContent, 0777)
	require.NoError(t, err)

	err = os.Mkdir("/tmp/.eibc-client", 0755)
	require.NoError(t, err)

	err = copyFile("data/config.yaml", "/tmp/.eibc-client/config.yaml")
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	err = dymension.Sidecars[0].CreateContainer(ctx)
	require.NoError(t, err)

	err = dymension.Sidecars[0].StartContainer(ctx)
	require.NoError(t, err)
	err = testutil.WaitForBlocks(ctx, 10, dymension, rollapp1)
	require.NoError(t, err)

	// Send a ibc tx from RA -> Hub
	transferData = ibc.WalletData{
		Address: dymensionUserAddr,
		Denom:   rollapp1.Config().Denom,
		Amount:  transferAmount,
	}

	multiplier := math.NewInt(10)

	eibcFee := transferAmount.Quo(multiplier)

	// set eIBC specific memo
	var options ibc.TransferOptions
	options.Memo = BuildEIbcMemo(eibcFee)

	_, err = rollapp1.SendIBCTransfer(ctx, channel.ChannelID, rollappUserAddr, transferData, options)
	require.NoError(t, err)

	// get eIbc event
	eibcEvents, err := getEIbcEventsWithinBlockRange(ctx, dymension, 10, false)
	require.NoError(t, err)
	for i, eibcEvent := range eibcEvents {
		fmt.Println(i, "EIBC Event:", eibcEvent)
	}

	rollappHeight, err = rollapp1.GetNode().Height(ctx)
	require.NoError(t, err)

	// Assert balance was updated on the hub
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount.Sub(transferData.Amount).Sub(bigTransferAmount))

	// wait until the packet is finalized
	isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), rollappHeight, 300)
	require.NoError(t, err)
	require.True(t, isFinalized)

	res, err = dymension.GetNode().QueryPendingPacketsByAddress(ctx, dymensionUserAddr)
	fmt.Println(res)
	require.NoError(t, err)

	for _, packet := range res.RollappPackets {

		proofHeight, _ := strconv.ParseInt(packet.ProofHeight, 10, 64)
		isFinalized, err = dymension.WaitUntilRollappHeightIsFinalized(ctx, rollapp1.GetChainID(), proofHeight, 300)
		require.NoError(t, err)
		require.True(t, isFinalized)
		txhash, err := dymension.GetNode().FinalizePacket(ctx, dymensionUserAddr, packet.RollappId, fmt.Sprint(packet.ProofHeight), fmt.Sprint(packet.Type), packet.Packet.SourceChannel, fmt.Sprint(packet.Packet.Sequence))
		require.NoError(t, err)

		fmt.Println(txhash)
	}

	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Minus 0.1% of transfer amount for bridge fee
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, rollappIBCDenom, transferData.Amount.Sub(bridgingFee).Sub(eibcFee))

	// Run invariant check
	CheckInvariant(t, ctx, dymension, dymensionUser.KeyName())
}
