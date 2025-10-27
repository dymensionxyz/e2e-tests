#!/bin/bash
set -e

# Check required environment variables
: ${ROLLAPP_ID:?Error: ROLLAPP_ID is not set}
: ${ROLLER_RELEASE_TAG:?Error: ROLLER_RELEASE_TAG is not set}
: ${ROLLER_RA_COMMIT:?Error: ROLLER_RA_COMMIT is not set}
: ${ROLLER_RA_GENESIS_STR:?Error: ROLLER_RA_GENESIS_STR is not set}
: ${ROLLER_RA_CUSTOM_STR:?Error: ROLLER_RA_CUSTOM_STR is not set}
: ${ROLLER_DA_CONFIG_STR:?Error: ROLLER_DA_CONFIG_STR is not set}

echo "ROLLAPP_ID: $ROLLAPP_ID"
echo "ROLLER_RELEASE_TAG: $ROLLER_RELEASE_TAG"
echo "ROLLER_RA_COMMIT: $ROLLER_RA_COMMIT" # this is used by roller itself

echo "$ROLLER_RA_GENESIS_STR" > /app/sequencer.genesis.json
export ROLLER_RA_GENESIS="file:///app/sequencer.genesis.json" # this is used by roller itself

echo "$ROLLER_RA_CUSTOM_STR" > /app/sequencer.custom.env.json

export ROLLER_SKIP_CELESTIA_BINARY=true

# Enhanced debugging for Go toolchain issues (TODO: needed)
export GOTOOLCHAIN=local  # Prevent automatic toolchain downloads
export GOPROXY=https://proxy.golang.org,direct
export GOSUMDB=sum.golang.org
export CGO_ENABLED=1

echo "Go version: $(go version)"
echo "GOPATH: $GOPATH"
echo "GOPROXY: $GOPROXY"
echo "GOTOOLCHAIN: $GOTOOLCHAIN"

curl https://raw.githubusercontent.com/dymensionxyz/roller/${ROLLER_RELEASE_TAG}/install.sh | bash

echo "downloaded roller";
roller version

roller rollapp init $ROLLAPP_ID --overwrite --env custom --generate-sequencer-address=true --use-default-websocket-endpoint --env-custom-filepath=/app/sequencer.custom.env.json

echo "did roller rollapp init ...";

roller rollapp setup --node-type fullnode --full-node-type tee --skip-da

echo "did roller rollapp setup ...";

roller rollapp config set tee_enabled true;
roller rollapp config set da_config "$ROLLER_DA_CONFIG_STR";
roller rollapp config set da_layer "celestia";

# Configure RPC to listen on all interfaces (not just localhost)
# This is required for external access in Confidential Space environment
sed -i 's/laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' /root/.roller/rollapp/config/config.toml
sed -i 's/address = "127.0.0.1:1317"/address = "0.0.0.0:1317"/' /root/.roller/rollapp/config/app.toml

echo "did roller rollapp config set ...";

echo "Dumping dymint config before starting...";
possible_dymint_paths=(
  "/root/.roller/rollapp/config/dymint.toml"
  "/root/.roller/rollapp/config/config/dymint.toml"
  "/root/.roller/rollapp/config/dymint/config.toml"
)
for cfg_path in "${possible_dymint_paths[@]}"; do
  if [ -f "$cfg_path" ]; then
    echo "Found dymint config: $cfg_path";
    echo "----------------------------------------";
    cat "$cfg_path";
    echo "----------------------------------------";
    break
  fi
done

roller rollapp start;