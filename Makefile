#!/usr/bin/make -f

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

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-fulfill-no-balance: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCNoBalanceToFulfillOrder .

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-corrupted-memo: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCCorruptedMemoNegative .

# Executes IBC tests via rollup-e2e-testing
e2e-test-eibc-excessive-fee: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFeeTooHigh .

e2e-test-eibc-timeout: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCTimeoutHubToRollapp .

e2e-test-transfer-multi-hop:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferMultiHop .

e2e-test-rollapp-freeze:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreeze .

# Executes all tests via rollup-e2e-testing
e2e-test-all: e2e-test-ibc-success \
	e2e-test-ibc-timeout \
	e2e-test-ibc-grace-period \
	e2e-test-transfer-multi-hop \
	e2e-test-eibc-fulfillment \
	e2e-test-transfer-multi-hop \
	e2e-test-rollapp-freeze \
	e2e-test-eibc-fulfill-no-balance \
	e2e-test-eibc-corrupted-memo \
	e2e-test-eibc-excessive-fee \
	e2e-test-eibc-timeout

.PHONY: clean-e2e \
	e2e-test-all \
	e2e-test-ibc-success \
	e2e-test-ibc-timeout \
	e2e-test-ibc-grace-period \
	e2e-test-transfer-multi-hop \
	e2e-test-eibc-fulfillment \
	e2e-test-transfer-multi-hop \
	e2e-test-rollapp-freeze \
	e2e-test-eibc-fulfill-no-balance \
	e2e-test-eibc-corrupted-memo \
	e2e-test-eibc-excessive-fee \
	e2e-test-eibc-timeout

