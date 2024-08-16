#!/bin/bash

set -eo pipefail

TEST="${1}"

# run the test file directly, this allows log output to be streamed directly in the terminal sessions
# without needed to wait for the test to finish.
# it shouldn't take 30m, but the wasm test can be quite slow, so we can be generous.
cd tests && go test -timeout=45m -race -v -run ${TEST} .
