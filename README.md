## Quick Start
Make sure you have Docker installed. For testing in local machine you need 2 steps:

1. Build a debug image with your code change
```bash
make docker-build-e2e
```
2. Run Test-case you want to test. Example:
```bash
make e2e-test-ibc
```

## Tests handle

| Scenario | Actor | Where | Pre-condition |
|----------|-------|-------|---------------|
| IBC transfer from rollapp to hub succeeds when rollapp has NO FINALIZED STATES AT ALL (just pending) | | | At least 2 rollapps running, Rollapp A and B. Rollapp B (our rollapp) has no finalized state. Rollapp B has height > Rollapp A height. Rollapp B has a channel-id different from the hub-channel-id. |
| Rollapp token transfer should only be received on the hub upon rollapp finalized state | | | At least 2 rollapps running, Rollapp A and B. Rollapp B (our rollapp) is at finalized height < Rollapp A finalized height. Rollapp B has a channel-id different from the hub-channel-id. |
| Rollapp token Demand order is created upon memo submission and fulfilled | User | UI | At least 2 rollapps running, Rollapp A and B. Rollapp B is at finalized height < Rollapp A finalized height. Rollapp B has a channel-id different from the hub-channel-id. |
| EIBC Timeout from hub to rollapp | User | CLI | At least 2 rollapps running, Rollapp A and B. Rollapp B (our rollapp) is at finalized height < Rollapp A finalized height. Rollapp B has a channel-id different from the hub-channel-id. |
| EIBC + PFM | User | CLI | At least 2 rollapps running, Rollapp A and B. Rollapp B (our rollapp) is at finalized height < Rollapp A finalized height. Rollapp B has a channel-id different from the hub-channel-id. |
