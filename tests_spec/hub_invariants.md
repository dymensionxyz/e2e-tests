## Tests handle

| Scenario | Pre-condition | Pre-condition check | Expected result | Expected result check | Covered By |
|----------|---------------|---------------------|-----------------|-----------------------|------------|
| eibc invariants | <ul> <li> At least x amount of eIBC are finalized </li> <li> at least x amount of eIBC are pending </li> <li> at least one epoch has passed since first eIBC message (for rollapp packet deletion. Epoch by default in that case is 1hr) </li></ul> | 🛑 <br> (missing)  |  run crisis for the eibc module and pass successfully | 🛑 <br> (missing) | TODO |
| rollapp invariants | <ul> <li> At least 2 rollapps are registered  </li> <li> at least 1 is an evm rollapp </li> <li> at least 2 grace periods has passed </li> <li>a few states are in pending mode currently </li></ul> | 🛑 <br> (missing)  |  run crisis for the rollapp invariant  and pass successfully | 🛑 <br> (missing) | TODO |
| sequencer invariants | <ul> <li> multiple sequencers registered per rollapp </li> <li> a few bonded, one unbonding and one bonded  </li></ul> | 🛑 <br> (missing)  |  run crisis for the sequencer invariant  and pass successfully | 🛑 <br> (missing) | TODO |
