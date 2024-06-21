## Quick Start
Make sure you have Docker installed. For testing in local machine you need 2 steps:

1. Build a debug image with your code change
```bash
make docker-build-e2e
```
2. Run Test-case you want to test. Example:
```bash
make e2e-test-ibc
```
## Version Matrix

The version matrix below shows which versions of the E2E-Tests, Dymension, Rollapp, Relayer and libraries are compatible with each other.

| E2E Tests | Dymension | Rollapp-EVM | Rollapp-Wasm | Relayer | 
| ---------- | ---------| ----------- | ------------ | ---------- | 
| v0.0.1     | v3.1.0   | v2.1.z      | 0.1.0-6cf8b0dd   | v0.3.3-v2.5.2-relayer    | 
| v1.0.1     | v3.1.0-73a8a1d8   | v2.2.0-rc03 | 0.1.0-7b60edee   | v0.3.3-v2.5.2-relayer    |
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

## Contributing

We welcome contributions to this repository. If you would like to add more test cases or improve existing ones, please feel free to fork this repository, make your changes, and submit a pull request.
