# HOW TO SETUP UP SEQUENCER ON BOX (some cmds run local, some on box itself)

# utils: curl https://rpc.dan2.team.rollapp.network:443/tee
#  curl -X POST https://rpc.dan2.team.rollapp.network:443  -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"tee","params":{"dry":true},"id":1}'
# make build && sudo cp /home/daniel_dymension_xyz/go/bin/roller /usr/local/bin/roller

export ROLLER_RELEASE_TAG="v1.18.0-rc09-tee6"
export ROLLER_RA_COMMIT="570bcbd906022cae19e50bbc3b8b13549f437436"
export ROLLER_RA_GENESIS="file:///home/daniel_dymension_xyz/sequencer.genesis.json"
curl https://raw.githubusercontent.com/dymensionxyz/roller/main/install.sh | bash

# api should be 1318
# rpc should be 36657

# note uses dymd 3.2.0-rc02
gcloud compute scp /Users/danwt/Documents/dym/d-tee/demos/demo-full/artifacts/scripts/data/roller_custom_env.json dym-dev-team-daniel:~/roller_custom_env.json --project=dymension-ops --zone=europe-west1-b --tunnel-through-iap

gcloud compute scp /Users/danwt/.rollapp-wasm/config/genesis.json dym-dev-team-daniel:~/sequencer.genesis.json --project=dymension-ops --zone=europe-west1-b --tunnel-through-iap

roller rollapp init rollappwasm_1234-1 --env custom
# when prompted, pass roller_custom_env.json as custom file

PATH=$PATH:$GOPATH/bin
roller rollapp setup --skip-genesis-validation --node-type sequencer

# sudo cp $GOPATH/bin/rollapp-wasm /usr/local/bin/rollappd

roller rollapp config set rollapp_max_idle_time "1m59s";
roller rollapp config set rollapp_batch_submit_time "2m";
roller rollapp set tee_enabled true;
roller rollapp set tee_interval "30s";
roller rollapp set tee_sidecar_url "https://rpc.dan2.team.rollapp.network:443";

vim .roller/da-light-node/config.toml # set rpc to 0.0.0.0
vim .roller/rollapp/config/dymint.toml # set da_config base_url to http://0.0.0.0:26658

roller rollapp services load
roller rollapp services start 

ZONE="europe-west1-b"

gcloud config set project dymension-ops
gcloud compute firewall-rules create allow-celestia-da \
--allow tcp:26658 \
--source-ranges 0.0.0.0/0 \
--target-tags celestia-da \
--description "Allow Celestia DA RPC port"

# Verify it exists
gcloud compute firewall-rules list --filter="name=allow-celestia-da"

# Add the tag to the instance
gcloud compute instances add-tags dym-dev-team-daniel \
--tags celestia-da \
--zone europe-west1-b

# Verify the instance has the tag
gcloud compute instances describe dym-dev-team-daniel \
--zone europe-west1-b \
--format='get(tags.items[])'

# get ip
gcloud compute instances describe dym-dev-team-daniel --zone $ZONE --format='get(networkInterfaces[0].accessConfigs[0].natIP)'

# Then test
# curl http://<ip>:26658
