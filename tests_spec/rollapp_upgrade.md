## Tests handle

| No | Scenario | Pre-condition | Pre-condition check | Expected result | Expected result check | Covered By |
|----|----------|---------------|---------------------|-----------------|-----------------------|------------|
| 1  | Test non state breaking binary upgrade | Running rollapp |  ✅ | Change to a new binary and rollapp should continue functioning with upgraded version | ✅ | [TestRollappUpgradeNonStateBreaking_EVM](../tests/rollapp_upgrade_test.go#28) [TestRollappUpgradeNonStateBreaking_Wasm](../tests/rollapp_upgrade_test.go#220) |
| 2  | Test upgrade which requires migration | Running rollapp and governor have enough tokens to vote for a governance proposal|✅ | Vote for upgrade and rollapp should continue functioning with upgraded version |✅ | [TestRollappUpgrade_EVM](../tests/rollapp_upgrade_test.go#428) |
