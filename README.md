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
2. [Add additional test cases here with the same format]

## Contributing

We welcome contributions to this repository. If you would like to add more test cases or improve existing ones, please feel free to fork this repository, make your changes, and submit a pull request.
