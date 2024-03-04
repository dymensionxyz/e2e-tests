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

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-fulfill-no-balance:
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillOrderWithoutBalanceNegative .

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-fulfill-more-than-once:	
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillOrderMoreThanOnceNegative .

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-not-fulfilled:
	cd tests && go test -timeout=25m -race -v -run TestEIBCNotFulfilled .

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-corrupted-memo:
	cd tests && go test -timeout=25m -race -v -run TestEIBCCorruptedMemoNegative .

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-excessive-fee:
	cd tests && go test -timeout=25m -race -v -run TestEIBCFeeMoreThanPacketAmountNegative .

# Executes all tests via rollup-e2e-testing
e2e-test-all: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-ibc-grace-period e2e-test-eibc-fulfillment e2e-test-eibc-fulfill-no-balance e2e-test-eibc-fulfill-more-than-once e2e-test-eibc-not-fulfilled e2e-test-eibc-corrupted-memo e2e-test-eibc-excessive-fee

.PHONY: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-ibc-grace-period e2e-test-eibc-fulfillment e2e-test-eibc-fulfill-no-balance e2e-test-eibc-fulfill-more-than-once e2e-test-eibc-not-fulfilled e2e-test-eibc-corrupted-memo e2e-test-eibc-excessive-fee e2e-test-all

