# E2E-Tests

## Overview
This repository contains end-to-end (E2E) and Live-E2E (testing with data on Testnet) tests for Dymension and relevant repositories. These tests ensure the functionality and integration of various components within Dymension.

## Prerequisites
Before running the tests, ensure you have the following installed:

- Docker
- [Go (v1.21 or above)](https://go.dev/doc/install)

## Version Matrix

The version matrix below shows which versions of the E2E-Tests, Dymension, Rollapp, Relayer and libraries are compatible with each other.

| E2E Tests | Dymension | Rollapp-EVM | Rollapp-Wasm | Relayer | 
| ---------- | ---------| ----------- | ------------ | ---------- | 
| v0.0.1     | v3.1.0   | v2.1.z      | 0.1.0-6cf8b0dd   | v0.3.3-v2.5.2-relayer    | 
| v1.0.1     | v3.1.0-73a8a1d8   | v2.2.0-rc03 | 0.1.0-7b60edee   | v0.3.3-v2.5.2-relayer    |
| v1.1.0     | v3.1.0-a74ffb0c   | v2.2.0 | 0.1.0-7b60edee   | v0.3.3-v2.5.2-relayer    |
| main     | main  | main | main   | main-dym    |

## Tests

1. [TestDelayedAck](tests_spec/delayedack.md)
2. [TestEIBC](tests_spec/eibc.md)
3. [TestPFM](tests_spec/pfm.md)
4. [TestGenesisBridge](tests_spec/rollapp_genesis.md)
5. [TestERC20](tests_spec/erc20.md)
6. [TestSequencer](tests_spec/sequencer.md)
7. [TestHubInvariants](tests_spec/hub_invariants.md)
8. [TestRollappUpgrade](tests_spec/rollapp_upgrade.md)
9. [TestRollappHardfork](tests_spec/rollapp_hardfork.md)

## Installation
Clone the repository:
```sh
git clone https://github.com/dymensionxyz/e2e-tests.git
cd e2e-tests
```

## Usage
### Running E2E-Tests
To run the E2E tests, you can use the provided Makefile. Here are some example:

```sh
make e2e-test-ibc-success-evm
```

Optional:

- If you want to keep the containers after running
```sh
export KEEP_CONTAINERS="true"
```
- If you want to use a custom image for the chains
```sh
export DYMENSION_CI="ghcr.io/dymensionxyz/dymension:debug"
export ROLLAPP_WASM_CI="ghcr.io/dymensionxyz/rollapp-wasm:debug"
export ROLLAPP_EVM_CI="ghcr.io/dymensionxyz/rollapp-evm:debug"
export RELAYER_CI="ghcr.io/dymensionxyz/relayer:debug"

```
### Running Live-Tests
To run the Live-tests, please make sure you have chain binary (ex: Hub, Rollapp, Gaia, TIA, ...) installed. Then simply execute commands defined in the Makefile.

```sh
make e2e-live-test-ibc-transfer-success
```

## Contributing

We welcome contributions to this repository. If you would like to add more test cases or improve existing ones, please feel free to fork this repository, make your changes, and submit a pull request.