# Setup local hub
trash ~/.dymension
export SETTLEMENT_EXECUTABLE=dymd
bash scripts/setup_local.sh
dymd start --log_level debug

# Create a rollapp on local
trash ~/.rollapp-wasm
export BASE_DENOM="awsm"
export BECH32_PREFIX="rol"
export CELESTIA_HOME_DIR="${HOME}/.da"
export CELESTIA_NETWORK="mocha"
export DA_CLIENT="celestia"
export DENOM=$(echo "$BASE_DENOM" | sed 's/^.//')
export EXECUTABLE="rollapp-wasm"
export HUB_PERMISSIONED_KEY="hub-user"
export HUB_KEY_WITH_FUNDS="$HUB_PERMISSIONED_KEY"
export HUB_RPC_ENDPOINT="localhost"
export HUB_RPC_PORT="36657" # default: 36657
export HUB_RPC_URL="http://${HUB_RPC_ENDPOINT}:${HUB_RPC_PORT}"
export KEY_NAME_ROLLAPP="rol-user"
export MONIKER_NAME="local"
export ROLLAPP_CHAIN_ID="rollappwasm_1234-1"
export MONIKER="$ROLLAPP_CHAIN_ID-sequencer"
export ROLLAPP_HOME_DIR="$HOME/.rollapp-wasm"
export ROLLAPP_SETTLEMENT_INIT_DIR_PATH="${ROLLAPP_HOME_DIR}/init"
export SETTLEMENT_LAYER="dymension" 
export SETTLEMENT_EXECUTABLE="dymd"

sh scripts/init.sh

sh scripts/settlement/register_rollapp_to_hub.sh

frpc -c /Users/danwt/Documents/dym/d-tee/demos/demo-full/artifacts/scripts/frpc.client.toml
# rpc is 36657
# api is 1318
# grpc is 8090

# fund sequencer (after roller setup)
SEQUENCER_ADDR_HUB="dym14updrfmrsyh385arxcwuleq8ad2tp30vzx8vth" # NOTE: REPLACE WITH WHAT IS CREATED BY ROLLER
dymd tx bank send hub-user $SEQUENCER_ADDR_HUB 10000000000000000000000adym --from hub-user --fees 3000000000000000adym --gas auto --gas-adjustment 3 -y
dymd q txs --query "message.sender='$SEQUENCER_ADDR_HUB'"

# for debug only
rollapp-wasm start --log_level debug

dymd tx gov submit-proposal /Users/danwt/Documents/dym/d-dymension/scripts/tee/example_proposal.json.populated.json \
    --from hub-user \
    --fees 3000000000000000adym \
    --gas auto \
    --gas-adjustment 3 \
    -y

dymd tx gov vote 1 yes \
    --from hub-user \
    --fees 3000000000000000adym \
    --gas auto \
    --gas-adjustment 3 \
    -y
