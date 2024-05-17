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
