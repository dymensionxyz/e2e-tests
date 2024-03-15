#!/usr/bin/make -f

docker-build-e2e-pre-upgrade:
	@DOCKER_BUILDKIT=1 docker build -t ghcr.io/dymensionxyz/dymension:e2e-pre-upgrade -f pre.Dockerfile .

###############################################################################
###                                E2E tests                                ###
###############################################################################

clean-e2e:
	sh clean.sh
# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-success: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferSuccess .

# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-timeout: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferTimeout .

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-fulfillment:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment .
  
# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-grace-period:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCGracePeriodCompliance .

# Executes upgrade tests via rollup-e2e-testing
e2e-test-upgrade-hub:
	cd tests && go test -timeout=25m -race -v -run TestHubUpgrade .

e2e-test-transfer-multi-hop:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferMultiHop .

# Executes all tests via rollup-e2e-testing
e2e-test-all: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-ibc-grace-period e2e-test-transfer-multi-hop e2e-test-eibc-fulfillment e2e-test-upgrade-hub

.PHONY: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-ibc-grace-period e2e-test-transfer-multi-hop e2e-test-eibc-fulfillment e2e-test-upgrade-hub e2e-test-all clean-e2e


