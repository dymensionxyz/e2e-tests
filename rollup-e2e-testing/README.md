<div align="center">
<h1><code>Rollup-e2e-testing</code></h1>
</div>

# Overview

Rollup-e2e-testing is designed as a framework for the purpose of testing rollup models such as Dymension, Rollkit, etc.

The framework is developed based on the architecture of [interchaintest](https://github.com/strangelove-ventures/interchaintest), [osmosis-e2e](https://github.com/osmosis-labs/osmosis/tree/main/tests/e2e), [gaia-e2e](https://github.com/cosmos/gaia/tree/main/tests/e2e),... to help quickly spin up custom testnets and dev environments to test IBC, [Relayer](https://github.com/cosmos/relayer) setup, hub and rollapp infrastructure, smart contracts, etc.

# Tutorial

Use Rollup-e2e-testing as a Module:

This document breaks down code snippets from [ibc_transfer_test.go](../example/ibc_transfer_test.go). This test:

1) Spins up Rollapp and Dymension Hub
2) Creates an IBC Path between them (client, connection, channel)
3) Sends an IBC transaction between them.

It then validates each step and confirms that the balances of each wallet are correct.

Three basic components of `rollup-e2e-testing`:
- **Chain Factory** - Select hub and rollapps binaries to include in tests
- **Relayer Factory** - Select Relayer to use in tests
- **Setup** - Where the testnet is configured and spun up

### Chain Factory

```go
	numHubVals := 1
	numHubFullNodes := 1
	numRollAppFn := 0
	numRollAppVals := 1
	cf := cosmos.NewBuiltinChainFactory(zaptest.NewLogger(t), []*cosmos.ChainSpec{
		{
			Name: "rollapp1",
			ChainConfig: ibc.ChainConfig{
				Type:    "rollapp",
				Name:    "rollapp-temp",
				ChainID: "demo-dymension-rollapp",
				Images: []ibc.DockerImage{
					{
						Repository: "ghcr.io/decentrio/rollapp",
						Version:    "e2e",
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
				NoHostMount:         false,
				ModifyGenesis:       nil,
				ConfigFileOverrides: configFileOverrides,
			},
			NumValidators: &numRollAppVals,
			NumFullNodes:  &numRollAppFn,
		},
		{
			Name:          "dymension-hub",
			ChainConfig:   dymensionConfig,
			NumValidators: &numHubVals,
			NumFullNodes:  &numHubFullNodes,
		},
	})
```
### Relayer Factory

```go
client, network := test.DockerSetup(t)

r := relayer.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t),
	relayer.CustomDockerImage("ghcr.io/cosmos/relayer", "reece-v2.3.1-ethermint", "100:1000"),
    ).Build(t, client, network)
```

### Setup
We prep the "Setup" by adding chains, a relayer, and specifying which chains to create IBC paths for:
```go
const ibcPath = "dymension-demo"
ic := test.NewSetup().
	AddChain(rollapp1).
	AddChain(dymension).
	AddRelayer(r, "relayer").
	AddLink(test.InterchainLink{
		Chain1:  dymension,
		Chain2:  rollapp1,
		Relayer: r,
		Path:    ibcPath,
	})
```
# Environment Variable

- `SHOW_CONTAINER_LOGS`: Controls whether container logs are displayed.

    - Set to `"always"` to show logs for both pass and fail.
    - Set to `"never"` to never show any logs.
    - Leave unset to show logs only for failed tests.

- `KEEP_CONTAINERS`: Prevents testnet cleanup after completion.

    - Set to any non-empty value to keep testnet containers alive.

- `CONTAINER_LOG_TAIL`: Specifies the number of lines to display from container logs. Defaults to 50 lines.

# Branches

|                               **Branch Name**                                | **IBC-Go** | **Cosmos-sdk** |
|:----------------------------------------------------------------------------:|:----------:|:--------------:|
|         [v6](https://github.com/decentrio/rollup-e2e-testing/tree/v6)        |     v6     |     v0.46      |
|     [main](https://github.com/decentrio/rollup-e2e-testing/tree/main)     |     v8     |     v0.50      |

# Example

Send IBC transaction from Rollapp <-> Hub and vice versa.
```
cd example
go test -race -v -run TestIBCTransfer .
```