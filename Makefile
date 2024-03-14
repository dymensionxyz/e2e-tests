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

e2e-test-transfer-multi-hop:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferMultiHop .

# Executes IBC tests via rollup-e2e-testing
rollapp-evm-e2e-test-ibc-success: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappEVM_IBCTransferSuccess .

# Executes IBC tests via rollup-e2e-testing
rollapp-evm-e2e-test-ibc-timeout: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappEVM_IBCTransferTimeout .

# Executes IBC tests via rollup-e2e-testing
rollapp-evm-e2e-test-eibc-fulfillment:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappEVM_EIBCFulfillment .
  
# Executes IBC tests via rollup-e2e-testing
rollapp-evm-e2e-test-ibc-grace-period:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappEVM_IBCGracePeriodCompliance .

rollapp-evm-e2e-test-transfer-multi-hop:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappEVM_IBCTransferMultiHop .

# Executes all tests via rollup-e2e-testing
e2e-test-all: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-ibc-grace-period e2e-test-transfer-multi-hop e2e-test-eibc-fulfillment rollapp-evm-e2e-test-ibc-success rollapp-evm-e2e-test-ibc-timeout rollapp-evm-e2e-test-eibc-fulfillment rollapp-evm-e2e-test-ibc-grace-period rollapp-evm-e2e-test-transfer-multi-hop

.PHONY: e2e-test-ibc-success e2e-test-ibc-timeout e2e-test-ibc-grace-period e2e-test-transfer-multi-hop e2e-test-eibc-fulfillment e2e-test-all clean-e2e rollapp-evm-e2e-test-ibc-success rollapp-evm-e2e-test-ibc-timeout rollapp-evm-e2e-test-eibc-fulfillment rollapp-evm-e2e-test-ibc-grace-period rollapp-evm-e2e-test-transfer-multi-hop

