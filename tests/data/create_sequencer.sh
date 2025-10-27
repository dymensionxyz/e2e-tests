export EXECUTABLE="rollapp-wasm"
export ROLLAPP_CHAIN_ID="forktest_879525-1"
export KEY_NAME_ROLLAPP="rol-user"
export CELESTIA_NETWORK="mock" # for a testnet RollApp use "mocha", for mainnet - "celestia"
export CELESTIA_HOME_DIR="${HOME}/.da"
export BASE_DENOM="awsm"
export DENOM=$(echo "$BASE_DENOM" | sed 's/^.//')
export MONIKER="$ROLLAPP_CHAIN_ID-sequencer"
export ROLLAPP_HOME_DIR="$HOME/.rollapp-wasm"
export ROLLAPP_SETTLEMENT_INIT_DIR_PATH="${ROLLAPP_HOME_DIR}/init"
export SETTLEMENT_LAYER="dymension"
export HUB_RPC_ENDPOINT="http://localhost"
export HUB_RPC_PORT="36657" # default: 36657
export HUB_RPC_URL="${HUB_RPC_ENDPOINT}:${HUB_RPC_PORT}"
export HUB_CHAIN_ID="dymension_100-1"
export HUB_REST_URL="http://localhost:1318"
export HUB_KEY_WITH_FUNDS="hub-user"
export ROLLAPP_ALIAS="testtest1"
export BECH32_PREFIX=rol

bash scripts/init.sh
dymd keys add sequencer --keyring-dir ~/.rollapp-wasm/sequencer_keys --keyring-backend test
SEQUENCER_ADDR=`dymd keys show sequencer --address --keyring-backend test --keyring-dir ~/.rollapp-wasm/sequencer_keys`
BOND_AMOUNT="$(dymd q sequencer params -o json --node ${HUB_RPC_URL} | jq -r '.params.min_bond.amount')$(dymd q sequencer params -o jsono | jq -r '.params.min_bond.denom')"
dymd tx bank send hub-user $SEQUENCER_ADDR 10000000000000000000000adym --from hub-user --fees 3000000000000000adym --gas auto --gas-adjustment 3 -y


vi /home/ext_duc_decentrio_ventures/.rollapp-wasm/config/genesis.json


bash scripts/settlement/register_rollapp_to_hub.sh
export BOND_AMOUNT=10000000000000000000000adym
bash scripts/settlement/register_sequencer_to_hub.sh
dasel put -f "${ROLLAPP_HOME_DIR}"/config/dymint.toml "settlement_node_address" -v "$HUB_RPC_URL"
dasel put -f "${ROLLAPP_HOME_DIR}"/config/dymint.toml "rollapp_id" -v "$ROLLAPP_CHAIN_ID"
dasel put -f "${ROLLAPP_HOME_DIR}"/config/dymint.toml "max_idle_time" -v "2s"
dasel put -f "${ROLLAPP_HOME_DIR}"/config/dymint.toml "max_proof_time" -v "1s"
dasel put -f "${ROLLAPP_HOME_DIR}"/config/app.toml "minimum-gas-prices" -v "1awsm"

batch_acceptance_attempts = '5'
batch_acceptance_timeout = '2m0s'
batch_submit_bytes = 500000
batch_submit_time = '1h0m0s'
block_time = '200ms'
da_config = ['{ "base_url": "https://celestia.tee.e2e.rollapp.network:443", "timeout": 60000000000, "gas_prices": 0.020000, "gas_adjustment": 1.3, "namespace_id": "228e01d583048593a5c1", "auth_token": "", "backoff": { "initial_delay": 6000000000, "max_delay": 6000000000, "growth_factor": 2 }, "retry_attempts": 4, "retry_delay": 3000000000 }']
da_layer = ['celestia', 'avail', 'loadnetwork', 'sui', 'aptos', 'bnb']
dym_account_name = 'sequencer'
keyring_backend = 'test'
keyring_home_dir = '/home/ext_duc_decentrio_ventures/.rollapp-wasm/sequencer_keys'
max_idle_time = '2s'
max_proof_time = '1s'
max_skew_time = '168h0m0s'
p2p_blocksync_block_request_interval = '30s'
p2p_blocksync_enabled = 'true'
p2p_bootstrap_nodes = ''
p2p_bootstrap_retry_time = '30s'
p2p_discovery_enabled = 'true'
p2p_gossip_cache_size = 50
p2p_listen_address = '/ip4/0.0.0.0/tcp/26656'
p2p_persistent_nodes = ''
retry_attempts = '10'
retry_max_delay = '10s'
retry_min_delay = '5s'
rollapp_id = 'forktest_879525-1'
settlement_gas_fees = ''
settlement_gas_limit = 0
settlement_gas_prices = '1000000000adym'
settlement_layer = 'dymension'
settlement_node_address = 'http://localhost:36657'

[db]
  badger_num_compactors = 0
  in_memory = false
  sync_writes = true

[instrumentation]
  prometheus = false
  prometheus_listen_addr = ':2112'
