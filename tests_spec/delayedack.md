# Test Handling Table

## From ROLLAPP to HUB

| No | Scenario | Pre-condition | Pre-condition check | Expected result | Expected result check | Covered By |
|----|----------|---------------|---------------------|-----------------|-----------------------|------------|
| 1  | IBC transfer from rollapp to hub succeeds when rollapp has NO FINALIZED STATES AT ALL | At least 2 rollapps running, Rollapp A and B. Rollapp B (our rollapp) has no finalized state. Rollapp B has height > Rollapp A height. Rollapp B has a channel-id different from the hub-channel-id and **no finalized state**| 🛑 <br> (missing) | Rollapp tokens successfully transferred to hub despite no finalized states. | Partly solved <br> (lack query `packet commitment` left on the rollapp) | lacking |
| 2  | Rollapp token transfer should only be received on the hub upon rollapp finalized state (assume no eIBC packet, i.e no memo) | At least 2 rollapps running, Rollapp A and B. Rollapp B (our rollapp) is at finalized height < Rollapp A finalized height. Rollapp B has a channel-id different from the hub-channel-id. | 🛑 <br> (missing) | Rollapp tokens received on hub only after Rollapp B reaches finalized state. | Partly solved <br> (lack query `packet commitment` left on the rollapp) | [ibc_grace_period_test](../tests/ibc_grace_period_test.go) |
