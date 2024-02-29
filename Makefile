#!/usr/bin/make -f

###############################################################################
###                                E2E tests                                ###
###############################################################################

# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-success:
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferSuccess .

# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-timeout:
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferTimeout .

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-fulfillment:
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment .
  
# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-grace-period:
	cd tests && go test -timeout=25m -race -v -run TestIBCGracePeriodCompliance .

# Executes all tests via rollup-e2e-testing
e2e-test-all: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-ibc-grace-period e2e-test-eibc-fulfillment

.PHONY: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-ibc-grace-period e2e-test-eibc-fulfillment e2e-test-all

