package tests

import (
	"context"
	"fmt"
	"testing"

	test "github.com/decentrio/rollup-e2e-testing"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/cosmos/hub/dym_hub"
	"github.com/decentrio/rollup-e2e-testing/cosmos/rollapp/dym_rollapp"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/decentrio/rollup-e2e-testing/relayer"
	"github.com/decentrio/rollup-e2e-testing/testreporter"
	"github.com/decentrio/rollup-e2e-testing/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestSequencerInvariant_EVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "rollappevm_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "3s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "100s")

	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1
	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "rollappevm_1234-1",
				Images:              []ibc.DockerImage{rollappEVMImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "ethm",
				Denom:               "urax",
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
				ConfigFileOverrides: configFileOverrides1,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp2",
				ChainID:             "rollappevm_12345-1",
				Images:              []ibc.DockerImage{rollappEVMImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "ethm",
				Denom:               "urax",
				CoinType:            "60",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       modifyRollappEVMGenesis(rollappEVMGenesisKV),
				ConfigFileOverrides: configFileOverrides2,
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
				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
				ConfigFileOverrides: nil,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	// relayer for rollapp 1
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r1, "relayer1").
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r1,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: r2,
			Path:    anotherIbcPath,
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
	})
	require.NoError(t, err)

	_, _, err = rollapp1.GetNode().ExecInit(ctx, "sequencer1", "/var/cosmos-chain/sequencer1")
	require.NoError(t, err)

	command := append([]string{rollapp1.GetNode().Chain.Config().Bin}, "dymint", "show-sequencer", "--home", "/var/cosmos-chain/sequencer1")
	pub1, _, err := rollapp1.GetNode().Exec(ctx, command, nil)
	require.NoError(t, err)

	_, _, err = rollapp1.GetNode().ExecInit(ctx, "sequencer2", "/var/cosmos-chain/sequencer2")
	require.NoError(t, err)

	command = append([]string{rollapp1.GetNode().Chain.Config().Bin}, "dymint", "show-sequencer", "--home", "/var/cosmos-chain/sequencer2")
	pub2, _, err := rollapp1.GetNode().Exec(ctx, command, nil)
	require.NoError(t, err)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension, dymension, rollapp1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, sequencer1, sequencer2, rollappUser := users[0], users[1], users[2], users[3], users[4]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	command = []string{}
	command = append(command, "sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "{\"Moniker\":\"myrollapp-sequencer\",\"Identity\":\"\",\"Website\":\"\",\"SecurityContact\":\"\",\"Details\":\"\"}", "1000000000adym",
		"--broadcast-mode", "block")

	_, err = dymension.GetNode().ExecTx(ctx, sequencer1.KeyName(), command...)
	require.NoError(t, err)

	command = []string{}
	command = append(command, "sequencer", "create-sequencer", string(pub2), rollapp1.Config().ChainID, "{\"Moniker\":\"myrollapp-sequencer\",\"Identity\":\"\",\"Website\":\"\",\"SecurityContact\":\"\",\"Details\":\"\"}", "1000000000adym",
		"--broadcast-mode", "block")

	_, err = dymension.GetNode().ExecTx(ctx, sequencer2.KeyName(), command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 3, "should have 3 sequences")

	err = dymension.Unbond(ctx, sequencer1.KeyName(), "")
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, sequencer1.FormattedAddress())
	require.NoError(t, err)
	require.Equal(t, queryGetSequencerResponse.Sequencer.Status, "OPERATING_STATUS_UNBONDING")

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	dymChannels, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)

	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)

	require.Len(t, channsRollApp1, 1)
	require.Len(t, channsRollApp2, 1)
	require.Len(t, dymChannels, 2)

	triggerHubGenesisEvent(t, dymension,
		rollappParam{
			rollappID: rollapp1.Config().ChainID,
			channelID: channsRollApp1[0].Counterparty.ChannelID,
			userKey:   dymensionUser.KeyName(),
		}, rollappParam{
			rollappID: rollapp2.Config().ChainID,
			channelID: channsRollApp2[0].Counterparty.ChannelID,
			userKey:   dymensionUser.KeyName(),
		})

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// Run eibc variants
	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "sequencer", "sequencers-count")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "sequencer", "sequencers-per-rollapp")
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}

func TestSequencerInvariant_Wasm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()

	// setup config for rollapp 1
	settlement_layer_rollapp1 := "dymension"
	settlement_node_address := fmt.Sprintf("http://dymension_100-1-val-0-%s:26657", t.Name())
	rollapp1_id := "rollappevm_1234-1"
	gas_price_rollapp1 := "0adym"
	maxIdleTime1 := "3s"
	maxProofTime := "500ms"
	configFileOverrides1 := overridesDymintToml(settlement_layer_rollapp1, settlement_node_address, rollapp1_id, gas_price_rollapp1, maxIdleTime1, maxProofTime, "100s")

	// setup config for rollapp 2
	settlement_layer_rollapp2 := "dymension"
	rollapp2_id := "rollappevm_12345-1"
	gas_price_rollapp2 := "0adym"
	maxIdleTime2 := "3s"
	configFileOverrides2 := overridesDymintToml(settlement_layer_rollapp2, settlement_node_address, rollapp2_id, gas_price_rollapp2, maxIdleTime2, maxProofTime, "100s")

	modifyGenesisKV := append(
		dymensionGenesisKV,
		cosmos.GenesisKV{
			Key:   "app_state.rollapp.params.dispute_period_in_blocks",
			Value: fmt.Sprint(BLOCK_FINALITY_PERIOD),
		},
	)

	// Create chain factory with dymension
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1
	cf := test.NewBuiltinChainFactory(zaptest.NewLogger(t), []*test.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp",
				ChainID:             "rollappwasm_1234-1",
				Images:              []ibc.DockerImage{rollappWasmImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "rol",
				Denom:               "urax",
				CoinType:            "118",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       nil,
				ConfigFileOverrides: configFileOverrides1,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name: "rollapp2",
			ChainConfig: ibc.ChainConfig{
				Type:                "rollapp-dym",
				Name:                "rollapp-temp2",
				ChainID:             "rollappwasm_12345-1",
				Images:              []ibc.DockerImage{rollappWasmImage},
				Bin:                 "rollappd",
				Bech32Prefix:        "rol",
				Denom:               "urax",
				CoinType:            "118",
				GasPrices:           "0.0urax",
				GasAdjustment:       1.1,
				TrustingPeriod:      "112h",
				EncodingConfig:      encodingConfig(),
				NoHostMount:         false,
				ModifyGenesis:       nil,
				ConfigFileOverrides: configFileOverrides2,
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
				ModifyGenesis:       modifyDymensionGenesis(modifyGenesisKV),
				ConfigFileOverrides: nil,
			},
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)

	rollapp1 := chains[0].(*dym_rollapp.DymRollApp)
	rollapp2 := chains[1].(*dym_rollapp.DymRollApp)
	dymension := chains[2].(*dym_hub.DymHub)

	// Relayer Factory
	client, network := test.DockerSetup(t)
	// relayer for rollapp 1
	r1 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"),
	).Build(t, client, "relayer1", network)
	// relayer for rollapp 2
	r2 := test.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
		relayer.CustomDockerImage(RelayerMainRepo, relayerVersion, "100:1000"),
	).Build(t, client, "relayer2", network)

	ic := test.NewSetup().
		AddRollUp(dymension, rollapp1, rollapp2).
		AddRelayer(r1, "relayer1").
		AddRelayer(r2, "relayer2").
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp1,
			Relayer: r1,
			Path:    ibcPath,
		}).
		AddLink(test.InterchainLink{
			Chain1:  dymension,
			Chain2:  rollapp2,
			Relayer: r2,
			Path:    anotherIbcPath,
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
	})
	require.NoError(t, err)

	_, _, err = rollapp1.GetNode().ExecInit(ctx, "sequencer1", "/var/cosmos-chain/sequencer1")
	require.NoError(t, err)

	command := append([]string{rollapp1.GetNode().Chain.Config().Bin}, "dymint", "show-sequencer", "--home", "/var/cosmos-chain/sequencer1")
	pub1, _, err := rollapp1.GetNode().Exec(ctx, command, nil)
	require.NoError(t, err)

	_, _, err = rollapp1.GetNode().ExecInit(ctx, "sequencer2", "/var/cosmos-chain/sequencer2")
	require.NoError(t, err)

	command = append([]string{rollapp1.GetNode().Chain.Config().Bin}, "dymint", "show-sequencer", "--home", "/var/cosmos-chain/sequencer2")
	pub2, _, err := rollapp1.GetNode().Exec(ctx, command, nil)
	require.NoError(t, err)

	// Create some user accounts on both chains
	users := test.GetAndFundTestUsers(t, ctx, t.Name(), walletAmount, dymension, dymension, dymension, dymension, rollapp1)

	// Wait a few blocks for relayer to start and for user accounts to be created
	err = testutil.WaitForBlocks(ctx, 5, dymension, rollapp1)
	require.NoError(t, err)

	// Get our Bech32 encoded user addresses
	dymensionUser, marketMaker, sequencer1, sequencer2, rollappUser := users[0], users[1], users[2], users[3], users[4]

	dymensionUserAddr := dymensionUser.FormattedAddress()
	marketMakerAddr := marketMaker.FormattedAddress()
	rollappUserAddr := rollappUser.FormattedAddress()

	command = []string{}
	command = append(command, "sequencer", "create-sequencer", string(pub1), rollapp1.Config().ChainID, "{\"Moniker\":\"myrollapp-sequencer\",\"Identity\":\"\",\"Website\":\"\",\"SecurityContact\":\"\",\"Details\":\"\"}", "1000000000adym",
		"--broadcast-mode", "block")

	_, err = dymension.GetNode().ExecTx(ctx, sequencer1.KeyName(), command...)
	require.NoError(t, err)

	command = []string{}
	command = append(command, "sequencer", "create-sequencer", string(pub2), rollapp1.Config().ChainID, "{\"Moniker\":\"myrollapp-sequencer\",\"Identity\":\"\",\"Website\":\"\",\"SecurityContact\":\"\",\"Details\":\"\"}", "1000000000adym",
		"--broadcast-mode", "block")

	_, err = dymension.GetNode().ExecTx(ctx, sequencer2.KeyName(), command...)
	require.NoError(t, err)

	res, err := dymension.QueryShowSequencerByRollapp(ctx, rollapp1.Config().ChainID)
	require.NoError(t, err)
	require.Equal(t, len(res.Sequencers), 3, "should have 3 sequences")

	err = dymension.Unbond(ctx, sequencer1.KeyName(), "")
	require.NoError(t, err)

	queryGetSequencerResponse, err := dymension.QueryShowSequencer(ctx, sequencer1.FormattedAddress())
	require.NoError(t, err)
	require.Equal(t, queryGetSequencerResponse.Sequencer.Status, "OPERATING_STATUS_UNBONDING")

	CreateChannel(ctx, t, r1, eRep, dymension.CosmosChain, rollapp1.CosmosChain, ibcPath)
	CreateChannel(ctx, t, r2, eRep, dymension.CosmosChain, rollapp2.CosmosChain, anotherIbcPath)

	// Assert the accounts were funded
	testutil.AssertBalance(t, ctx, dymension, dymensionUserAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, dymension, marketMakerAddr, dymension.Config().Denom, walletAmount)
	testutil.AssertBalance(t, ctx, rollapp1, rollappUserAddr, rollapp1.Config().Denom, walletAmount)

	dymChannels, err := r1.GetChannels(ctx, eRep, dymension.Config().ChainID)
	require.NoError(t, err)

	channsRollApp1, err := r1.GetChannels(ctx, eRep, rollapp1.GetChainID())
	require.NoError(t, err)

	channsRollApp2, err := r2.GetChannels(ctx, eRep, rollapp2.GetChainID())
	require.NoError(t, err)

	require.Len(t, channsRollApp1, 1)
	require.Len(t, channsRollApp2, 1)
	require.Len(t, dymChannels, 2)

	triggerHubGenesisEvent(t, dymension,
		rollappParam{
			rollappID: rollapp1.Config().ChainID,
			channelID: channsRollApp1[0].Counterparty.ChannelID,
			userKey:   dymensionUser.KeyName(),
		}, rollappParam{
			rollappID: rollapp2.Config().ChainID,
			channelID: channsRollApp2[0].Counterparty.ChannelID,
			userKey:   dymensionUser.KeyName(),
		})

	// Start relayer
	err = r1.StartRelayer(ctx, eRep, ibcPath)
	require.NoError(t, err)
	err = r2.StartRelayer(ctx, eRep, anotherIbcPath)
	require.NoError(t, err)

	err = testutil.WaitForBlocks(ctx, 5, dymension)
	require.NoError(t, err)

	// Run eibc variants
	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "sequencer", "sequencers-count")
	require.NoError(t, err)

	_, err = dymension.GetNode().CrisisInvariant(ctx, dymensionUser.KeyName(), "sequencer", "sequencers-per-rollapp")
	require.NoError(t, err)

	t.Cleanup(
		func() {
			err := r1.StopRelayer(ctx, eRep)
			if err != nil {
				t.Logf("an error occurred while stopping the relayer: %s", err)
			}
		},
	)
}
