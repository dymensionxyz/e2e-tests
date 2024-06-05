#!/usr/bin/make -f

###############################################################################
###                                E2E tests                                ###
###############################################################################

clean-e2e:
	sh clean.sh

e2e-test: clean-e2e
	./run-e2e.sh $(test)

# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-success-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferSuccess_EVM .

e2e-test-ibc-timeout-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferTimeout_EVM .

e2e-test-eibc-fulfillment-only-one-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillOnOneRollApp_EVM .

e2e-test-eibc-fulfillment-evm-2-RAs:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_two_rollapps_EVM .

e2e-test-ibc-grace-period-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCGracePeriodCompliance_EVM .

e2e-test-eibc-fulfillment-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_EVM .

e2e-test-eibc-ack-error-dym-evm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run TestEIBC_AckError_Dym_EVM .

e2e-test-eibc-ack-error-ra-token-evm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run TestEIBC_AckError_RA_Token_EVM .

e2e-test-eibc-ack-error-3rd-party-token-evm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run TestEIBC_AckError_3rd_Party_Token_EVM .

e2e-test-eibc-fulfillment-ignore-hub-to-rollapp-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_ignore_hub_to_RA_EVM .

e2e-test-eibc-fulfillment-ignore-hub-to-rollapp-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_ignore_hub_to_RA_Wasm .

e2e-test-eibc-pfm-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCPFM_EVM .

e2e-test-eibc-fulfill-no-balance-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCNoBalanceToFulfillOrder_EVM .

e2e-test-eibc-corrupted-memo-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCCorruptedMemoNegative_EVM .

e2e-test-eibc-excessive-fee-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFeeTooHigh_EVM .

e2e-test-eibc-timeout-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCTimeoutDymToRollapp_EVM .

e2e-test-eibc-timeout_and_fulfill-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCTimeoutFulFillDymToRollapp_Evm .

e2e-test-eibc-timeout_and_fulfill-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCTimeoutFulFillDymToRollapp_Wasm .

e2e-test-transfer-multi-hop-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferMultiHop_EVM .

e2e-test-pfm-with-grace-period-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCPFMWithGracePeriod_EVM .

e2e-test-batch-finalization-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestBatchFinalization_EVM .

e2e-test-disconnection-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDisconnection_EVM .

e2e-test-rollapp-freeze-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreeze_EVM .

e2e-test-rollapp-freeze-non-broken-invariant-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreezeNoBrokenInvariants_EVM .
  
e2e-test-other-rollapp-not-affected-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestOtherRollappNotAffected_EVM .

e2e-test-freeze-packets-rollback-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestPacketRollbacked_EVM .

e2e-test-rollapp-genesis-event-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappGenesisEvent_EVM .

e2e-test-dym-finalize-block-on-recv-packet-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDymFinalizeBlock_OnRecvPacket_EVM .

e2e-test-dym-finalize-block-on-timeout-packet-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDymFinalizeBlock_OnTimeOutPacket_EVM .

e2e-test-dym-finalize-block-on-ack-packet-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDymFinalizeBlock_OnAckPacket_EVM .

e2e-test-delayedack-pending-packets-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDelayedAck_NoFinalizedStates_EVM .

e2e-test-eibc-fulfillment-thirdparty-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_ThirdParty_EVM .

e2e-test-delayedack-relayer-down-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDelayedAck_RelayerDown_EVM .

e2e-test-sequencer-invariant-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestSequencerInvariant_EVM .

e2e-test-rollapp-invariant-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappInvariant_EVM .
	
e2e-test-eibc-invariant-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCInvariant_EVM .

e2e-test-eibc-not-fulfillment-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCNotFulfillment_EVM .

e2e-test-pfm-gaia-to-rollapp-evm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferGaiaToRollApp_EVM .

e2e-test-erc20-hub-to-rollapp-without-register:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestERC20HubToRollAppWithoutRegister_EVM .
	
e2e-test-rollapp-upgrade-non-state-breaking-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappUpgradeNonStateBreaking_EVM .

e2e-test-rollapp-upgrade-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollapp_EVM_Upgrade .

e2e-test-rollapp_genesis_transfer_rollapp_to_hub_with_trigger_rollapp_evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestTransferRollAppTriggerGenesis_EVM .

e2e-test-rollapp_genesis_transfer_rollapp_to_hub_with_trigger_hub_evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppTransferHubTriggerGenesis_EVM .

e2e-test-rollapp_genesis_transfer_hub_to_rollapp_with_trigger_rollapp_evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestHubTransferRollAppTriggerGenesis_EVM .

e2e-test-rollapp_genesis_transfer_hub_to_rollapp_with_trigger_hub_evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestHubTransferHubTriggerGenesis_EVM .

e2e-test-rollapp_genesis_transfer_back_and_forth_with_trigger_both_evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestTransferTriggerGenesisBoth_EVM .

e2e-test-rollapp-freeze-cant-fulfill-pending-eibc-packet-evm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreezeEibcPending_EVM .

# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-success-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferSuccess_Wasm .

e2e-test-ibc-timeout-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferTimeout_Wasm .

e2e-test-eibc-ack-error-dym-wasm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run TestEIBC_AckError_Dym_Wasm .

e2e-test-eibc-ack-error-ra-token-wasm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run TestEIBC_AckError_RA_Token_Wasm .

e2e-test-eibc-ack-error-3rd-party-token-wasm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run TestEIBC_AckError_3rd_Party_Token_Wasm .
e2e-test-eibc-fulfillment-only-one-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillOnOneRollApp_Wasm .

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

e2e-test-pfm-with-grace-period-rollapp1-to-rollapp2-erc20: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCPFM_RollApp1To2WithErc20_EVM .

e2e-test-pfm-with-grace-period-rollapp1-to-rollapp2-without-erc20: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCPFM_RollApp1To2WithOutErc20_Wasm .

e2e-test-batch-finalization-wasm:
	cd tests && go test -timeout=25m -race -v -run TestBatchFinalization_Wasm .

e2e-test-disconnection-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDisconnection_Wasm .

e2e-test-rollapp-freeze-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreeze_Wasm .

e2e-test-rollapp-freeze-non-broken-invariant-wasm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreezeNoBrokenInvariants_Wasm .
  
  
e2e-test-other-rollapp-not-affected-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestOtherRollappNotAffected_Wasm .

e2e-test-freeze-packets-rollback-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestPacketRollbacked_Wasm .

e2e-test-eibc-not-fulfillment-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCNotFulfillment_Wasm .

e2e-test-eibc-fulfillment-thirdparty-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCFulfillment_ThirdParty_Wasm .
  
e2e-test-dym-finalize-block-on-recv-packet-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDymFinalizeBlock_OnRecvPacket_Wasm .

e2e-test-dym-finalize-block-on-timeout-packet-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDymFinalizeBlock_OnTimeOutPacket_Wasm .

e2e-test-dym-finalize-block-on-ack-packet-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDymFinalizeBlock_OnAckPacket_Wasm .

e2e-test-pfm-gaia-to-rollapp-wasm:  clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestIBCTransferGaiaToRollApp_Wasm .	

e2e-test-delayedack-pending-packets-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDelayedAck_NoFinalizedStates_Wasm .
  
e2e-test-delayedack-relayer-down-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestDelayedAck_RelayerDown_Wasm .

e2e-test-upgrade-hub: clean-e2e
	cd tests && go test -timeout=40m -race -v -run TestHubUpgrade .
	
e2e-test-sequencer-invariant-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestSequencerInvariant_Wasm .
	
e2e-test-rollapp-invariant-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappInvariant_Wasm .
	
e2e-test-eibc-invariant-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestEIBCInvariant_Wasm .

e2e-test-rollapp-upgrade-non-state-breaking-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollappUpgradeNonStateBreaking_Wasm .

e2e-test-rollapp_genesis_transfer_rollapp_to_hub_with_trigger_rollapp_wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestTransferRollAppTriggerGenesis_Wasm .

e2e-test-rollapp_genesis_transfer_rollapp_to_hub_with_trigger_hub_wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppTransferHubTriggerGenesis_Wasm .	

e2e-test-rollapp-upgrade-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollapp_Wasm_Upgrade .

e2e-test-rollapp_genesis_transfer_hub_to_rollapp_with_trigger_rollapp_wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestHubTransferRollAppTriggerGenesis_Wasm .

e2e-test-rollapp_genesis_transfer_back_and_forth_with_trigger_both_wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestTransferTriggerGenesisBoth_Wasm .

e2e-test-rollapp-freeze-cant-fulfill-pending-eibc-packet-wasm: clean-e2e
	cd tests && go test -timeout=25m -race -v -run TestRollAppFreezeEibcPending_Wasm .

# Executes all tests via rollup-e2e-testing
e2e-test-all: e2e-test-ibc-success-evm \
	e2e-test-ibc-timeout-evm \
	e2e-test-ibc-grace-period-evm \
	e2e-test-eibc-ack-error-dym-evm \
	e2e-test-eibc-ack-error-ra-token-evm \
	e2e-test-eibc-corrupted-memo-evm \
	e2e-test-eibc-excessive-fee-evm \
	e2e-test-eibc-fulfillment-evm \
  	e2e-test-eibc-fulfillment-evm-2-RAs \
  	e2e-test-eibc-fulfill-no-balance-evm \
	e2e-test-eibc-fulfillment-thirdparty-evm \
	e2e-test-eibc-fulfillment-ignore-hub-to-rollapp-evm \
	e2e-test-eibc-invariant-evm \
	e2e-test-eibc-pfm-evm \
	e2e-test-eibc-timeout-evm \
	e2e-test-transfer-multi-hop-evm \
	e2e-test-pfm-with-grace-period-evm \
	e2e-test-pfm-with-grace-period-rollapp1-to-rollapp2-erc20 \
	e2e-test-pfm-gaia-to-rollapp-evm \
	e2e-test-batch-finalization-evm \
	e2e-test-disconnection-evm \
	e2e-test-rollapp-freeze-evm \
  	e2e-test-other-rollapp-not-affected-evm \
	e2e-test-rollapp-genesis-event-evm \
	e2e-test-sequencer-invariant-evm \
	e2e-test-rollapp-invariant-evm \
	e2e-test-rollapp-upgrade-non-state-breaking-evm \
	e2e-test-erc20-hub-to-rollapp-without-register \
	e2e-test-ibc-success-wasm \
	e2e-test-ibc-timeout-wasm \
	e2e-test-ibc-grace-period-wasm \
	e2e-test-eibc-ack-error-dym-wasm \
	e2e-test-eibc-ack-error-ra-token-wasm \
	e2e-test-eibc-fulfillment-wasm \
	e2e-test-eibc-fulfillment-thirdparty-wasm \
	e2e-test-eibc-fulfillment-ignore-hub-to-rollapp-wasm \
	e2e-test-eibc-invariant-wasm
	e2e-test-eibc-pfm-wasm \
	e2e-test-transfer-multi-hop-wasm \
	e2e-test-pfm-with-grace-period-wasm \
	e2e-test-pfm-gaia-to-rollapp-wasm \
	e2e-test-batch-finalization-wasm \
	e2e-test-disconnection-wasm \
	e2e-test-rollapp-freeze-wasm \
 	e2e-test-other-rollapp-not-affected-wasm \
	e2e-test-dym-finalize-block-on-recv-packet \
	e2e-test-dym-finalize-block-on-timeout-packet \
	e2e-test-dym-finalize-block-on-ack-packet\
	e2e-test-delayedack-pending-packets-wasm \
	e2e-test-delayedack-relayer-down-wasm \
	e2e-test-upgrade-hub \
	e2e-test-sequencer-invariant-wasm \
	e2e-test-rollapp-invariant-wasm \
	e2e-test-delayedack-relayer-down-wasm \
	e2e-test-rollapp-upgrade-non-state-breaking-wasm

.PHONY: clean-e2e \
	e2e-test-all \
	e2e-test-ibc-success-evm \
	e2e-test-ibc-timeout-evm \
	e2e-test-ibc-grace-period-evm \
	e2e-test-eibc-ack-error-dym-evm \
	e2e-test-eibc-ack-error-ra-token-evm \
	e2e-test-eibc-fulfillment-evm-2-RAs \
	e2e-test-eibc-corrupted-memo-evm \
	e2e-test-eibc-excessive-fee-evm \
	e2e-test-eibc-fulfillment-evm \
	e2e-test-eibc-fulfill-no-balance-evm \
	e2e-test-eibc-fulfillment-thirdparty-evm \
	e2e-test-eibc-fulfillment-ignore-hub-to-rollapp-evm \
	e2e-test-eibc-invariant-evm \
	e2e-test-eibc-pfm-evm \
	e2e-test-eibc-timeout-evm \
	e2e-test-transfer-multi-hop-evm \
	e2e-test-pfm-with-grace-period-evm \
	e2e-test-pfm-with-grace-period-rollapp1-to-rollapp2-erc20 \
	e2e-test-pfm-gaia-to-rollapp-evm \
	e2e-test-batch-finalization-evm \
	e2e-test-disconnection-evm \
	e2e-test-rollapp-freeze-evm \
  	e2e-test-other-rollapp-not-affected-evm \
	e2e-test-rollapp-genesis-event-evm \
	e2e-test-sequencer-invariant-evm \
	e2e-test-rollapp-invariant-evm \
	e2e-test-rollapp-upgrade-non-state-breaking-evm \
	e2e-test-erc20-hub-to-rollapp-without-register \
	e2e-test-ibc-success-wasm \
	e2e-test-ibc-timeout-wasm \
	e2e-test-ibc-grace-period-wasm \
	e2e-test-eibc-ack-error-dym-wasm \
	e2e-test-eibc-ack-error-ra-token-wasm \
	e2e-test-eibc-fulfillment-wasm \
	e2e-test-eibc-fulfillment-thirdparty-wasm \
	e2e-test-eibc-fulfillment-ignore-hub-to-rollapp-wasm \
  	e2e-test-eibc-invariant-wasm \
	e2e-test-eibc-pfm-wasm \
	e2e-test-transfer-multi-hop-wasm \
	e2e-test-pfm-with-grace-period-wasm \
	e2e-test-pfm-gaia-to-rollapp-wasm \
	e2e-test-batch-finalization-wasm \
	e2e-test-disconnection-wasm \
	e2e-test-rollapp-freeze-wasm \
    e2e-test-other-rollapp-not-affected-wasm \
	e2e-test-delayedack-pending-packets-wasm \
	e2e-test-rollapp-invariant-wasm \
  	e2e-test-other-rollapp-not-affected-wasm \
	e2e-test-dym-finalize-block-on-recv-packet \
	e2e-test-dym-finalize-block-on-timeout-packet \
	e2e-test-dym-finalize-block-on-ack-packet \
	e2e-test-delayedack-pending-packets-wasm \
	e2e-test-upgrade-hub \
  	e2e-test-other-rollapp-not-affected-wasm \
	e2e-test-rollapp-upgrade-non-state-breaking-wasm
