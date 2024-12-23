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
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferSuccess_EVM .

e2e-test-light-client-same-chain-id: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferRA_3rdSameChainID_EVM .

e2e-test-light-client-hub-3rd: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferBetweenHub3rd_EVM .

e2e-test-light-client-same-chain-id-no-light-client: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransfer_NoLightClient_EVM .

e2e-test-spinup: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSpinUp .
	
e2e-eibc-update-already-fulfill-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillAlreadyFulfilledDemand_EVM .

e2e-eibc-update-unallowed-signer-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCUnallowedSigner_EVM .

e2e-test-update-do-ackerr-timeout-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCUpdateOnAckErrAndTimeout_EVM .

e2e-test-ADMC-hub-to-RA-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Hub_to_RA_reserved_EVM .

e2e-test-ADMC-hub-to-RA-3rd-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Hub_to_RA_3rd_Party_EVM .

e2e-hub-to-RA-migrate-dym-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Hub_to_RA_Migrate_Dym_EVM .

e2e-test-bridge-fee-param-change-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestChangeBridgeFeeParam_EVM .

e2e-test-ibc-transfer-reserved-word-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisIBCTransferReservedMemo_EVM .

e2e-test-ibc-timeout-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferTimeout_EVM .

e2e-test-eibc-fulfillment-only-one-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillOnOneRollApp_EVM .

e2e-test-eibc-fulfillment-evm-2-RAs:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillment_two_rollapps_EVM .

e2e-test-ibc-grace-period-evm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCGracePeriodCompliance_EVM .

e2e-test-eibc-fulfillment-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillment_EVM .

e2e-test-eibc-ack-error-dym-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBC_AckError_Dym_EVM .

e2e-test-eibc-ack-error-ra-token-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBC_AckError_RA_Token_EVM .

e2e-test-eibc-ack-error-3rd-party-token-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBC_AckError_3rd_Party_Token_EVM .

e2e-test-eibc-fulfillment-ignore-hub-to-rollapp-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillment_ignore_hub_to_RA_EVM .

e2e-test-eibc-fulfillment-ignore-hub-to-rollapp-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillment_ignore_hub_to_RA_Wasm .

e2e-test-eibc-pfm-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCPFM_EVM .

e2e-test-eibc-fulfill-no-balance-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCNoBalanceToFulfillOrder_EVM .

e2e-test-eibc-corrupted-memo-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCCorruptedMemoNegative_EVM .

e2e-test-eibc-excessive-fee-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFeeTooHigh_EVM .

e2e-test-eibc-timeout-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCTimeoutDymToRollapp_EVM .

e2e-test-eibc-timeout_and_fulfill-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCTimeoutFulFillDymToRollapp_EVM .

e2e-test-transfer-multi-hop-evm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferMultiHop_EVM .

e2e-test-pfm-with-grace-period-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCPFMWithGracePeriod_EVM .

e2e-test-batch-finalization-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestBatchFinalization_EVM .

e2e-test-disconnection-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDisconnection_EVM .

e2e-test-fullnode-sync-evm: clean-e2e
	cd tests && go test -timeout=35m -race -v -run TestFullnodeSync_EVM .

e2e-test-fullnode-sync-celes-evm: clean-e2e
	cd tests && go test -timeout=35m -race -v -run TestFullnodeSync_Celestia_EVM .

e2e-test-fullnode-celes-rt-gossip-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSync_Celes_Rt_Gossip_EVM .

e2e-test-fullnode-sqc-disconnect-gossip-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSync_Sqc_Disconnect_Gossip_EVM .

e2e-test-fullnode-disconnect-gossip-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSync_Fullnode_Disconnect_Gossip_EVM .

e2e-test-rollapp-freeze-evm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppFreeze_EVM .

e2e-test-rollapp-freeze-non-broken-invariant-evm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppFreezeNoBrokenInvariants_EVM .

e2e-test-rollapp-freeze-sequencer-slashed-jailed-evm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppSqcSlashedJailed_EVM .
  
e2e-test-other-rollapp-not-affected-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestOtherRollappNotAffected_EVM .

e2e-test-freeze-packets-rollback-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestPacketRollbacked_EVM .

e2e-test-dym-finalize-block-on-recv-packet-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDymFinalizeBlock_OnRecvPacket_EVM .

e2e-test-dym-finalize-block-on-timeout-packet-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDymFinalizeBlock_OnTimeOutPacket_EVM .

e2e-test-dym-finalize-block-on-ack-packet-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDymFinalizeBlock_OnAckPacket_EVM .

e2e-test-delayedack-pending-packets-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDelayedAck_NoFinalizedStates_EVM .

e2e-test-eibc-fulfillment-thirdparty-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillment_ThirdParty_EVM .

e2e-test-delayedack-relayer-down-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDelayedAck_RelayerDown_EVM .

e2e-test-sequencer-invariant-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSequencerInvariant_EVM .

e2e-test-rollapp-invariant-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollappInvariant_EVM .
	
e2e-test-eibc-invariant-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCInvariant_EVM .

e2e-test-eibc-not-fulfillment-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCNotFulfillment_EVM .

e2e-test-pfm-gaia-to-rollapp-evm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferGaiaToRollApp_EVM .
	
e2e-test-rollapp-upgrade-non-state-breaking-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollappUpgradeNonStateBreaking_EVM .

e2e-test-rollapp-upgrade-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollapp_EVM_Upgrade .

e2e-test-rollapp_genesis_transfer_rollapp_to_hub_with_trigger_rollapp_evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestTransferRollAppTriggerGenesis_EVM .

e2e-test-rollapp_genesis_transfer_rollapp_to_hub_with_trigger_hub_evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppTransferHubTriggerGenesis_EVM .

e2e-test-rollapp_genesis_transfer_hub_to_rollapp_with_trigger_rollapp_evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestHubTransferRollAppTriggerGenesis_EVM .

e2e-test-rollapp_genesis_transfer_hub_to_rollapp_with_trigger_hub_evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestHubTransferHubTriggerGenesis_EVM .

e2e-test-rollapp_genesis_transfer_back_and_forth_with_trigger_both_evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestTransferTriggerGenesisBoth_EVM .

e2e-test-rollapp-freeze-cant-fulfill-pending-eibc-packet-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppFreezeEibcPending_EVM .

e2e-test-rollapp-freeze-state-not-progressing-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppFreezeStateNotProgressing_EVM .

e2e-test-erc20-rollapp-to-hub-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestERC20RollAppToHubWithRegister_EVM .

e2e-test-rollapp-hardfork-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestHardFork_EVM .

e2e-test-rollapp-hardfork-recover-ibc-client-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestHardForkRecoverIbcClient_EVM .

e2e-test-rollapp-hardforkduetodrs-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestHardForkDueToDrs_EVM .
	
e2e-test-rollapp-hardforkduetofraud-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestHardForkDueToFraud_EVM .

e2e-test-rollapp-genesis-transfer-bridge-blocking-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisTransferBridgeBlocking_EVM .

e2e-test-rollapp-genesis-transfer-connection-blocking-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisTransferConnectionBlock_EVM .

e2e-test-genesis-bridge-invalid-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisBridgeInvalid_EVM .

e2e-test-genesis-bridge-before-channel-set-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisBridgeBeforeChannelSet_EVM .

e2e-test-non-rollapp-unaffected-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_Non_Rollappchain_Unaffected_EVM .

e2e-test-admc-originates-hub-to-rollapp-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Originates_HubtoRA_EVM .

e2e-test-admc-migrate-empty-user-memo-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Migrate_Empty_User_Memo_EVM .

e2e-test-admc-migrate-with-user-memo-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Migrate_With_User_Memo_EVM .

e2e-test-eibc-fee-market-success-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBC_Fee_Market_Success_EVM .
	
e2e-test-admc-metadata-not-found-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_MetaData_NotFound_EVM .

e2e-test-update-do-timeout-unallowed-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCUpdateOnTimeout_Unallowed_EVM .

e2e-test-sequencer-celestia-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSequencerCelestia_EVM .

e2e-test-sequencer-hub-disconnection-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSequencerHubDisconnection_EVM .

e2e-test-fullnode-sync-block-sync-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSync_BlockSync_EVM .

e2e-test-fullnode-disconnect-block-sync-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSync_BlockSync_fn_disconnect_EVM .

e2e-test-sequencer-rotation-oneseq-da-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_OneSeq_DA_EVM .

e2e-test-sequencer-rotation-oneseq-da-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_OneSeq_DA_Wasm .
	
e2e-test-sequencer-rotation-oneseq-p2p-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_OneSqc_P2P_EVM .

e2e-test-sequencer-rotation-oneseq-p2p-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_OneSqc_P2P_Wasm .

e2e-test-sequencer-rotation-mulseq-da-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_MulSeq_DA_EVM .

e2e-test-sequencer-rotation-mulseq-da-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_MulSeq_DA_Wasm .
	
e2e-test-sequencer-rotation-multi-seq-p2p-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_MulSqc_P2P_EVM .

e2e-test-sequencer-rotation-multi-seq-p2p-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_MulSqc_P2P_Wasm .

e2e-test-sequencer-rotation-noseq-da-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_NoSeq_DA_EVM .

e2e-test-sequencer-rotation-noseq-da-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_NoSeq_DA_Wasm .

e2e-test-sequencer-rotation-noseq-p2p-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_NoSeq_P2P_EVM .

e2e-test-sequencer-rotation-noseq-p2p-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_NoSeq_P2P_Wasm .
	
e2e-test-sequencer-rotation-unbond-da-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_Unbond_DA_EVM .

e2e-test-sequencer-rotation-unbond-da-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_Unbond_DA_Wasm .

e2e-test-sequencer-rotation-unbond-p2p-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_Unbond_P2P_EVM .

e2e-test-sequencer-rotation-unbond-p2p-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_Unbond_P2P_Wasm .

e2e-test-sequencer-rotation-accumdata-da-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_AccumData_DA_EVM .

e2e-test-sequencer-rotation-accumdata-da-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_AccumData_DA_Wasm .

e2e-test-sequencer-rotation-accumdata-p2p-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_AccumData_P2P_EVM .

e2e-test-sequencer-rotation-accumdata-p2p-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_AccumData_P2P_Wasm .
	
e2e-test-sequencer-rotation-state-update-fail-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_StateUpd_Fail_EVM .

e2e-test-sequencer-rotation-state-update-fail-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_StateUpd_Fail_Wasm .

e2e-test-sequencer-rotation-history-sync-da-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_HisSync_DA_EVM .

e2e-test-sequencer-rotation-history-sync-da-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRotation_HisSync_DA_Wasm .

e2e-test-sequencer-rotation-history-sync-p2p-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_HisSync_P2P_EVM .

e2e-test-sequencer-rotation-history-sync-p2p-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_HisSync_P2P_Wasm .

e2e-test-sequencer-rotation-history-sync-old-sequencer-unbonded-da-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_HisSync_Unbond_DA_EVM .

e2e-test-sequencer-rotation-history-sync-old-sequencer-unbonded-da-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_HisSync_Unbond_DA_Wasm .

e2e-test-sequencer-rotation-history-sync-old-sequencer-unbonded-p2p-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_HisSync_Unbond_P2P_EVM .

e2e-test-sequencer-rotation-history-sync-old-sequencer-unbonded-p2p-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SqcRotation_HisSync_Unbond_P2P_Wasm .

e2e-test-sequencer-rotation-forced-da-evm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run Test_SeqRotation_Forced_DA_EVM .

e2e-test-sequencer-rewardsaddress-update-evm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run Test_SeqRewardsAddress_Update_EVM .

e2e-test-sequencer-rewardsaddress-register-evm: clean-e2e
	cd tests && go test -timeout=30m -race -v -run Test_SeqRewardsAddress_Register_EVM .
  
e2e-test-eibc-client-success-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_EIBC_Client_Success_EVM .

e2e-test-eibc-client-nofulfillrollapp-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_EIBC_Client_NoFulfillRollapp_EVM .

e2e-test-genesis-bridge-no-relay-ack-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisBridgeNoRelayAck_EVM .

e2e-test-timebaseupgrade-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_TimeBaseUpgrade_EVM .

e2e-test-sequencer-rotation-roatate-request-no-da-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRot_RotReq_No_DA_EVM .

e2e-test-fraud-detection-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestFraudDetection_EVM .

e2e-test-fraud-detection-rotation-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestFraudDetect_Sequencer_Rotation_EVM .

e2e-test-timebaseupgradeinpast-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_TimeBaseUpgradeInPast_EVM .

e2e-test-zero-fee-rotated-sequencer-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestZeroFee_RotatedSequencer_EVM .
	
e2e-test-zero-fee-relay-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestZeroFee_RelaySuccess_EVM .

e2e-test-hardfork-kick-proposer-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_HardFork_KickProposer_EVM .

e2e-test-rollapp-state-update-success-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_RollAppStateUpdateSuccess_EVM .

e2e-test-rollapp-state-update-fail-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_RollAppStateUpdateFail_EVM .

e2e-test-rollapp-state-update-fail-celes-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_RollAppStateUpdateFail_Celes_EVM .

e2e-test-genesis-bridge-unbond-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisTransferBridgeUnBond_EVM

e2e-test-genesis-bridge-kick-proposer-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenTransferBridgeKickProposer_EVM

e2e-test-fraud-detection-da-p2p-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestFraudDetectionDA_P2P_EVM .

e2e-test-erc20-rollapp-to-hub-new-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestERC20RollAppToHubNewRegister_EVM .

e2e-test-tokenless-create-erc20-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestTokenlessCreateERC20_EVM .

e2e-test-tokenless-transfer-success-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestTokenlessTransferSuccess_EVM .

e2e-test-tokenless-transfer-diff-gas-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestTokenlessTransferDiffGas_EVM .

e2e-test-fraud-detection-produce-block-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestFraudDetect_L2_Produce_Block_EVM .

# Executes IBC tests via rollup-e2e-testing
e2e-test-ibc-success-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferSuccess_Wasm .

e2e-eibc-update-already-fulfill-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCAlreadyFulfilledDemand_Wasm .

e2e-eibc-update-unallowed-signer-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCUnallowedSigner_Wasm .
  
e2e-test-ADMC-hub-to-RA-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Hub_to_RA_reserved_Wasm .

e2e-test-ADMC-hub-to-RA-3rd-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Hub_to_RA_3rd_Party_Wasm .

e2e-hub-to-RA-migrate-dym-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Hub_to_RA_Migrate_Dym_Wasm .

e2e-test-ibc-transfer-reserved-word-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisIBCTransferReservedMemo_Wasm .

e2e-test-ibc-timeout-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferTimeout_Wasm .

e2e-test-eibc-ack-error-dym-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBC_AckError_Dym_Wasm .

e2e-test-eibc-ack-error-ra-token-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBC_AckError_RA_Token_Wasm .

e2e-test-eibc-ack-error-3rd-party-token-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBC_AckError_3rd_Party_Token_Wasm .
e2e-test-eibc-fulfillment-only-one-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillOnOneRollApp_Wasm .

e2e-test-eibc-fulfillment-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillment_Wasm .

e2e-test-eibc-pfm-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCPFM_Wasm .

e2e-test-ibc-grace-period-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCGracePeriodCompliance_Wasm .

e2e-test-transfer-multi-hop-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferMultiHop_Wasm .

e2e-test-pfm-with-grace-period-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCPFMWithGracePeriod_Wasm .

e2e-test-pfm-with-grace-period-rollapp1-to-rollapp2-erc20: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCPFM_RollApp1To2WithErc20_EVM .

e2e-test-pfm-with-grace-period-rollapp1-to-rollapp2-without-erc20: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCPFM_RollApp1To2WithOutErc20_Wasm .

e2e-test-batch-finalization-wasm:
	cd tests && go test -timeout=45m -race -v -run TestBatchFinalization_Wasm .

e2e-test-disconnection-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDisconnection_Wasm .

e2e-test-fullnode-sync-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestFullnodeSync_Wasm .

e2e-test-fullnode-sync-celes-wasm: clean-e2e
	cd tests && go test -timeout=35m -race -v -run TestFullnodeSync_Celestia_Wasm .

e2e-test-fullnode-celes-rt-gossip-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSync_Celes_Rt_Gossip_Wasm .

e2e-test-fullnode-sqc-disconnect-gossip-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSync_Sqc_Disconnect_Gossip_Wasm .

e2e-test-fullnode-disconnect-gossip-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSync_Fullnode_Disconnect_Gossip_Wasm .

e2e-test-rollapp-freeze-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppFreeze_Wasm .

e2e-test-rollapp-freeze-non-broken-invariant-wasm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppFreezeNoBrokenInvariants_Wasm .

e2e-test-rollapp-freeze-sequencer-slashed-jailed-wasm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppSqcSlashedJailed_Wasm .  
  
e2e-test-other-rollapp-not-affected-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestOtherRollappNotAffected_Wasm .

e2e-test-freeze-packets-rollback-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestPacketRollbacked_Wasm .

e2e-test-eibc-not-fulfillment-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCNotFulfillment_Wasm .

e2e-test-eibc-fulfillment-thirdparty-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCFulfillment_ThirdParty_Wasm .
  
e2e-test-dym-finalize-block-on-recv-packet-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDymFinalizeBlock_OnRecvPacket_Wasm .

e2e-test-dym-finalize-block-on-timeout-packet-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDymFinalizeBlock_OnTimeOutPacket_Wasm .

e2e-test-dym-finalize-block-on-ack-packet-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDymFinalizeBlock_OnAckPacket_Wasm .

e2e-test-pfm-gaia-to-rollapp-wasm:  clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestIBCTransferGaiaToRollApp_Wasm .	

e2e-test-delayedack-pending-packets-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDelayedAck_NoFinalizedStates_Wasm .
  
e2e-test-delayedack-relayer-down-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestDelayedAck_RelayerDown_Wasm .

e2e-test-upgrade-hub: clean-e2e
	cd tests && go test -timeout=40m -race -v -run TestHubUpgrade .
	
e2e-test-sequencer-invariant-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestSequencerInvariant_Wasm .
	
e2e-test-rollapp-invariant-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollappInvariant_Wasm .
	
e2e-test-eibc-invariant-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCInvariant_Wasm .

e2e-test-rollapp-upgrade-non-state-breaking-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollappUpgradeNonStateBreaking_Wasm .

e2e-test-rollapp_genesis_transfer_rollapp_to_hub_with_trigger_rollapp_wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestTransferRollAppTriggerGenesis_Wasm .

e2e-test-rollapp_genesis_transfer_rollapp_to_hub_with_trigger_hub_wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppTransferHubTriggerGenesis_Wasm .	

e2e-test-rollapp-upgrade-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollapp_Wasm_Upgrade .

e2e-test-rollapp_genesis_transfer_hub_to_rollapp_with_trigger_rollapp_wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestHubTransferRollAppTriggerGenesis_Wasm .

e2e-test-rollapp_genesis_transfer_back_and_forth_with_trigger_both_wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestTransferTriggerGenesisBoth_Wasm .

e2e-test-genesis-bridge-invalid-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisBridgeInvalid_Wasm .

e2e-test-rollapp-freeze-cant-fulfill-pending-eibc-packet-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppFreezeEibcPending_Wasm .

e2e-test-rollapp-freeze-state-not-progressing-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestRollAppFreezeStateNotProgressing_Wasm .

e2e-test-rollapp-hardfork-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestHardFork_Wasm .

e2e-test-rollapp-genesis-transfer-bridge-blocking-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisTransferBridgeBlocking_Wasm .

e2e-test-admc-originates-hub-to-rollapp-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Originates_HubtoRA_Wasm .

e2e-test-rollapp-genesis-transfer-connection-blocking-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisTransferConnectionBlock_Wasm .
e2e-test-admc-migrate-empty-user-memo-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Migrate_Empty_User_Memo_Wasm .

e2e-test-admc-migrate-with-user-memo-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_Migrate_With_User_Memo_Wasm .

e2e-test-eibc-fee-market-success-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBC_Fee_Market_Success_Wasm .
	
e2e-test-admc-metadata-not-found-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestADMC_MetaData_NotFound_Wasm .

e2e-test-update-do-ackerr-timeout-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCUpdateOnAckErrAndTimeout_Wasm .

e2e-test-update-do-timeout-unallowed-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCUpdateOnTimeout_Unallowed_Wasm .

e2e-test-eibc-timeout_and_fulfill-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestEIBCTimeoutFulFillDymToRollapp_Wasm .

e2e-test-genesis-bridge-no-relay-ack-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisBridgeNoRelayAck_Wasm .

e2e-test-sequencer-rotation-roatate-request-no-da-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_SeqRot_RotReq_No_DA_Wasm .

e2e-test-zero-fee-relay-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestZeroFee_RelaySuccess_Wasm .

e2e-test-without-genesis-account-evm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisBridgeWithoutGenesisAcc_EVM .

e2e-test-rollapp-state-update-success-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_RollAppStateUpdateSuccess_Wasm .

e2e-test-rollapp-state-update-fail-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_RollAppStateUpdateFail_Wasm .

e2e-test-rollapp-state-update-fail-celes-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run Test_RollAppStateUpdateFail_Celes_Wasm .

e2e-test-genesis-bridge-unbond-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenesisTransferBridgeUnBond_Wasm

e2e-test-genesis-bridge-kick-proposer-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestGenTransferBridgeKickProposer_Wasm

e2e-test-fraud-detection-da-p2p-wasm: clean-e2e
	cd tests && go test -timeout=45m -race -v -run TestFraudDetectionDA_P2P_Wasm .

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
	e2e-test-sequencer-invariant-evm \
	e2e-test-rollapp-invariant-evm \
	e2e-test-rollapp-upgrade-non-state-breaking-evm \
	e2e-test-erc20-hub-to-rollapp-without-register \
	e2e-test-rollapp-hardfork-evm \
	e2e-test-rollapp-genesis-transfer-bridge-blocking-evm \
	e2e-test-rollapp-genesis-transfer-connection-blocking-evm \
	e2e-test-non-rollapp-unaffected-evm \
	e2e-test-admc-migrate-empty-user-memo-evm \
	e2e-test-admc-migrate-with-user-memo-evm \
	e2e-test-eibc-fee-market-success-evm \
	e2e-test-admc-metadata-not-found-evm \
	e2e-test-fullnode-sync-block-sync-evm \
	e2e-test-seq-rotation-one-seq-evm \
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
	e2e-test-rollapp-upgrade-non-state-breaking-wasm \
	e2e-test-rollapp-hardfork-wasm \ 
	e2e-test-rollapp-genesis-transfer-bridge-blocking-wasm \
	e2e-test-admc-migrate-empty-user-memo-wasm \
	e2e-test-admc-migrate-with-user-memo-wasm \
	e2e-test-eibc-fee-market-success-wasm \
	e2e-test-admc-metadata-not-found-wasm

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
	e2e-test-sequencer-invariant-evm \
	e2e-test-rollapp-invariant-evm \
	e2e-test-rollapp-upgrade-non-state-breaking-evm \
	e2e-test-erc20-hub-to-rollapp-without-register \
	e2e-test-rollapp-hardfork-evm \
	e2e-test-rollapp-genesis-transfer-bridge-blocking-evm \
	e2e-test-rollapp-genesis-transfer-connection-blocking-evm \
	e2e-test-non-rollapp-unaffected-evm \
	e2e-test-admc-migrate-empty-user-memo-evm \
	e2e-test-admc-migrate-with-user-memo-evm \
	e2e-test-eibc-fee-market-success-evm \
	e2e-test-admc-metadata-not-found-evm \
	e2e-test-fullnode-sync-block-sync-evm \
	e2e-test-seq-rotation-one-seq-evm \
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
	e2e-test-rollapp-upgrade-non-state-breaking-wasm \
	e2e-test-rollapp-hardfork-wasm \
	e2e-test-rollapp-genesis-transfer-bridge-blocking-wasm \
	e2e-test-admc-migrate-empty-user-memo-wasm \
	e2e-test-admc-migrate-with-user-memo-wasm \
	e2e-test-eibc-fee-market-success-wasm
	e2e-test-admc-metadata-not-found-wasm

###############################################################################
###                              E2E live tests                             ###
###############################################################################

clean-e2e-live:
	sh clean-live.sh

e2e-live-test-ibc-transfer-success-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestIBCTransferRolX_Live .

e2e-live-test-ibc-transfer-success-roly: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestIBCTransferRolY_Live .

e2e-live-test-delayedack-rollapp-to-hub-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestDelayackRollappToHubRolX_Live .

e2e-live-test-eibc-timeout-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBCTimeoutRolX_Live .

e2e-live-test-eibc-timeout-roly: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBCTimeoutRolY_Live .

e2e-live-test-delayack-ack-error-from-dym-roly: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBC_AckError_Dym_RolY_Live .

e2e-live-test-eibc-3rd-token-roly: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBC_3rd_Token_RolY_Live .

e2e-live-test-eibc-3rd-token-timeout-roly: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBC_3rd_Token_Timeout_RolY_Live .

e2e-live-test-eibc-pfm-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBCPFMRolX_Live .
	
e2e-live-test-eibc-invalid-fee-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBC_Invalid_Fee_RolX_Live .
	
e2e-live-test-eibc-fulfillment-rolx: clean-e2e-live
	cd live-tests && go test -timeout=35m -race -v -run TestEIBCFulfillRolX_Live .
	
e2e-live-test-delayack-rollapp-to-hub-no-finalized-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestDelayackRollappToHubNoFinalizedRolX_Live .

e2e-live-test-eibc-no-memo-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBC_No_Memo_RolX_Live .

e2e-live-test-eibc-demand-order-ignored-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBC_Demand_Order_Ignored_RolX_Live .
  
e2e-live-test-eibc-eibc-fee-bgt-amount-rolx: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBCFeeBgtAmountRolX_Live .

e2e-live-test-eibc-eibc-fee-bgt-amount-roly: clean-e2e-live
	cd live-tests && go test -timeout=45m -race -v -run TestEIBCFeeBgtAmountRolY_Live .
