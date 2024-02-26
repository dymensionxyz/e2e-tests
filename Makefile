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
  
# Executes all tests via rollup-e2e-testing
e2e-test-all: e2e-test-ibc-success e2e-test-ibc-timeout

.PHONY: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-all

