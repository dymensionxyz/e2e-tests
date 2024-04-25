#!/usr/bin/make -f

###############################################################################
###                                E2E tests                                ###
###############################################################################

clean-e2e:
	sh clean.sh

# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-success-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferSuccess_EVM .

e2e-test-ibc-timeout-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferTimeout_EVM .

e2e-test-ibc-grace-period-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCGracePeriodCompliance_EVM .

e2e-test-eibc-fulfillment-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_EVM .

e2e-test-eibc-pfm-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCPFM_EVM .

e2e-test-eibc-fulfill-no-balance-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCNoBalanceToFulfillOrder .

e2e-test-eibc-corrupted-memo-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCCorruptedMemoNegative .

e2e-test-eibc-excessive-fee-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFeeTooHigh .

e2e-test-eibc-timeout-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCTimeoutHubToRollapp .
	
e2e-test-transfer-multi-hop-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferMultiHop_EVM .

e2e-test-pfm-with-grace-period-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCPFMWithGracePeriod_EVM .

e2e-test-batch-finalization-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestBatchFinalization_EVM .

e2e-test-disconnection-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDisconnection_EVM .

e2e-test-fullnode-sync-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestFullnodeSync_EVM .

e2e-test-rollapp-freeze-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreeze_EVM .
  
e2e-test-other-rollapp-not-affected-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestOtherRollappNotAffected_EVM .

e2e-test-rollapp-genesis-event-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappGenesisEvent_EVM .

e2e-test-delayedack-pending-packets-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDelayedAck_NoFinalizedStates_EVM .

e2e-test-eibc-fulfillment-thirdparty-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_ThirdParty_EVM .

# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-success-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferSuccess_Wasm .

e2e-test-ibc-timeout-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferTimeout_Wasm .

e2e-test-eibc-fulfillment-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_Wasm .

e2e-test-eibc-pfm-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCPFM_Wasm .

e2e-test-ibc-grace-period-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCGracePeriodCompliance_Wasm .

e2e-test-transfer-multi-hop-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferMultiHop_Wasm .

e2e-test-pfm-with-grace-period-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCPFMWithGracePeriod_Wasm .
	
e2e-test-batch-finalization-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestBatchFinalization_Wasm .

e2e-test-disconnection-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDisconnection_Wasm .

e2e-test-fullnode-sync-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestFullnodeSync_Wasm .

e2e-test-rollapp-freeze-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreeze_Wasm .
  
e2e-test-other-rollapp-not-affected-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestOtherRollappNotAffected_Wasm .

e2e-test-eibc-fulfillment-thirdparty-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_ThirdParty_Wasm .
  
e2e-test-pfm-gaia-to-rollapp-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferGaiaToRollApp_EVM .

e2e-test-pfm-gaia-to-rollapp-wasm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferGaiaToRollApp_Wasm .	

e2e-test-delayedack-pending-packets-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDelayedAck_NoFinalizedStates_Wasm .
	

# Executes all tests via rollup-e2e-testing
e2e-test-all: e2e-test-ibc-success-evm \
	e2e-test-ibc-timeout-evm \
	e2e-test-ibc-grace-period-evm \
	e2e-test-eibc-corrupted-memo-evm \
	e2e-test-eibc-excessive-fee-evm \
	e2e-test-eibc-fulfillment-evm \
    e2e-test-eibc-fulfill-no-balance-evm \
	e2e-test-eibc-fulfillment-thirdparty-evm \
	e2e-test-eibc-pfm-evm \
	e2e-test-eibc-timeout-evm \
	e2e-test-transfer-multi-hop-evm \
	e2e-test-pfm-with-grace-period-evm \
	e2e-test-pfm-gaia-to-rollapp-evm \
	e2e-test-batch-finalization-evm \
	e2e-test-disconnection-evm \
	e2e-test-rollapp-freeze-evm \
    e2e-test-other-rollapp-not-affected-evm \
	e2e-test-rollapp-genesis-event-evm \
	e2e-test-ibc-success-wasm \
	e2e-test-ibc-timeout-wasm \
	e2e-test-ibc-grace-period-wasm \
	e2e-test-eibc-fulfillment-wasm \
	e2e-test-eibc-fulfillment-thirdparty-wasm \
	e2e-test-eibc-pfm-wasm \
	e2e-test-transfer-multi-hop-wasm \
	e2e-test-pfm-with-grace-period-wasm \
	e2e-test-pfm-gaia-to-rollapp-wasm \
	e2e-test-batch-finalization-wasm \
	e2e-test-disconnection-wasm \
	e2e-test-rollapp-freeze-wasm \
    e2e-test-other-rollapp-not-affected-wasm \ 
	e2e-test-delayedack-pending-packets-wasm

.PHONY: clean-e2e \
	e2e-test-all \
	e2e-test-ibc-success-evm \
	e2e-test-ibc-timeout-evm \
	e2e-test-ibc-grace-period-evm \
	e2e-test-eibc-corrupted-memo-evm \
	e2e-test-eibc-excessive-fee-evm \
	e2e-test-eibc-fulfillment-evm \
	e2e-test-eibc-fulfill-no-balance-evm \
	e2e-test-eibc-fulfillment-thirdparty-evm \
	e2e-test-eibc-pfm-evm \
	e2e-test-eibc-timeout-evm \
	e2e-test-transfer-multi-hop-evm \
	e2e-test-pfm-with-grace-period-evm \
	e2e-test-pfm-gaia-to-rollapp-evm \
	e2e-test-batch-finalization-evm \
	e2e-test-disconnection-evm \
	e2e-test-rollapp-freeze-evm \
    e2e-test-other-rollapp-not-affected-evm \
	e2e-test-rollapp-genesis-event-evm \
	e2e-test-ibc-success-wasm \
	e2e-test-ibc-timeout-wasm \
	e2e-test-ibc-grace-period-wasm \
	e2e-test-eibc-fulfillment-wasm \
	e2e-test-eibc-fulfillment-thirdparty-wasm \
	e2e-test-eibc-pfm-wasm \
	e2e-test-transfer-multi-hop-wasm \
	e2e-test-pfm-with-grace-period-wasm \
	e2e-test-pfm-gaia-to-rollapp-wasm \
	e2e-test-batch-finalization-wasm \
	e2e-test-disconnection-wasm \
	e2e-test-rollapp-freeze-wasm \
    e2e-test-other-rollapp-not-affected-wasm \
	e2e-test-delayedack-pending-packets-wasm

