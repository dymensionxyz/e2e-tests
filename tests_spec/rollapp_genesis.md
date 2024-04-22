## Tests handle

| No | Scenario | Pre-condition | Pre-condition check | Expected result | Expected result check | Covered By |
|----|----------|---------------|---------------------|-----------------|-----------------------|------------|
| 1  | Rollapp genesis tokens on the hub upon channel creation | <ul> <li> Create relevant genesis accounts and metadata info </li> <li> Upon channel creation, trigger genesis events in both rollapp and hub (special tx)  </li> </ul> |  🛑 <br> (missing) | <ul> <li> denometadata should be created </li> <li> VFC should be created on hub (i.e erc20) </li> <li> Rollapp tokens with relevant amount should be locked on the rollapp </li> <li> genesis accounts should have the relevant balance on the hub </li> </ul> | 🛑 <br> (missing) | TODO |
| 2  | Transfer rollapp genesis tokens on the hub  | |🛑 <br> (missing) | <ul> <li> should be able to transfer the tokens to a different address on the hub </li> <li> should be able to import the erc20 rep and transfer it on the rollapp </li> </ul> | 🛑 <br> (missing) | TODO |
| 3  | Transfer rollapp tokens between hub ↔ rollapp  BEFORE rollapp has any finalized state on the hub (and both events were triggered)  | <ul> <li> Rollapp shouldn’t have any finalized state on the hub </li> <li> Rollapp and hub genesis event were triggered  </li> </ul> |🛑 <br> (missing) | <ul> <li> transfer from hub to rollapp should work as expected </li> <li> transfer from rollapp to hub should work as expected </li> <li> transferring all the genesis tokens from hub to rollapp should work as expected </li> </ul> | 🛑 <br> (missing) | TODO |
| 4  | Transfer rollapp tokens between hub ↔ rollapp  AFTER rollapp has any finalized state on the hub  | <ul> <li> Rollapp should have any finalized state on the hub </li> <li> Rollapp and hub genesis event were triggered </li> </ul> |🛑 <br> (missing) | <ul> <li> transfer from hub to rollapp should work as expected </li> <li> transfer from rollapp to hub should work as expected </li> <li> transferring all the genesis tokens from hub to rollapp should work as expected </li> </ul> | 🛑 <br> (missing) | TODO |
| 5  | Transfer from hub to rollapp after only hub side has gone through genesis event  | <ul> <li> Create relevant genesis accounts </li> <li> Open a channel </li> <li> Trigger genesis event on the hub </li> <li> try transferring tokens from hub to rollapp </li> <li> ERC20 was registered on the rollapp for the hub </li> </ul> |🛑 <br> (missing) | <ul> <li>  Should fail as no tokens are locked in the escrow address on the rollapp side </li> </ul> | 🛑 <br> (missing) | TODO |
| 6  | Transfer from rollapp to hub after only hub side has gone through genesis event | <ul> <li> Create relevant genesis accounts </li> <li> Open a channel </li> <li> Trigger genesis event on the hub </li> <li> try transferring tokens from rollapp to hub </li> </ul> |🛑 <br> (missing) | <ul> <li> Should succeed </li> </ul> | 🛑 <br> (missing) | TODO |
| 7  | Transfer from rollapp to hub after only rollapp side has gone through genesis event | <ul> <li> Create relevant genesis accounts </li> <li> Open a channel </li> <li> Trigger genesis event only on the rollapp </li> <li> Try transferring from rollapp to hub </li> </ul> |🛑 <br> (missing) | <ul> <li>  Should fail as no tokens are locked in the escrow address on the rollapp side </li> </ul> |🛑 <br> (missing) | <ul> <li> Should fail with ack error on validation </li> </ul> | 🛑 <br> (missing) | TODO |
| 8  | Transfer from hub to rollapp from rollapp-genesis-account on the hub after only rollapp side has gone through genesis event | <ul> <li> Create relevant genesis accounts </li> <li> Open a channel </li> <li> Trigger genesis event only on the rollapp </li> <li> Try transferring from hub to rollapp </li> </ul>|🛑 <br> (missing) | <ul> <li> Should fail as there are no balances for those addresses on the hub </li> </ul> | 🛑 <br> (missing) | TODO |