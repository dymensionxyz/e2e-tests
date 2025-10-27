#!/bin/bash

set -e

source workload_env.sh
source workload_common.sh

set_gcp_project "${PROJECT_ID}"

cd ../src/workload

docker build --platform linux/amd64 -t tee-fullnode-workload -f Dockerfile .

IMAGE_REFERENCE="${PROJECT_REPOSITORY_REGION}-docker.pkg.dev/${PROJECT_ID}/${ARTIFACT_REPOSITORY}/${WORKLOAD_IMAGE_NAME}:${WORKLOAD_IMAGE_TAG}"

docker tag tee-fullnode-workload "${IMAGE_REFERENCE}"

create_artifact_repository "${ARTIFACT_REPOSITORY}" "${PROJECT_REPOSITORY_REGION}"

gcloud auth configure-docker "${PROJECT_REPOSITORY_REGION}-docker.pkg.dev"

docker push "${IMAGE_REFERENCE}"

gcloud artifacts repositories add-iam-policy-binding "${ARTIFACT_REPOSITORY}" \
  --location="${PROJECT_REPOSITORY_REGION}" \
  --member="serviceAccount:${WORKLOAD_SERVICE_ACCOUNT}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/artifactregistry.reader" \
  --project="${PROJECT_ID}"

cd ../../scripts

echo "Creating TEE Fullnode instance..."
# Note: 100GB disk required for building rollapp from source (Go compilation needs space)
# Determine image family based on debug flag
if [[ "$GCP_IS_CONFIDENTIAL_DEBUG" == "true" ]]; then
  IMAGE_FAMILY="confidential-space-debug-preview-cgpu"
else
  IMAGE_FAMILY="confidential-space-preview-cgpu"
fi

gcloud compute instances delete tee-e2e-fullnode --zone="$PROJECT_ZONE" --project="$PROJECT_ID" --quiet

# pd-balanced is required for c3-standard-4 machines (pd-standard not supported)
gcloud compute instances create tee-e2e-fullnode \
  --confidential-compute-type=TDX \
  --shielded-secure-boot \
  --maintenance-policy=TERMINATE \
  --scopes=cloud-platform \
  --zone="${PROJECT_ZONE}" \
  --image-project=confidential-space-images \
  --image-family="${IMAGE_FAMILY}" \
  --service-account="${WORKLOAD_SERVICE_ACCOUNT}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --boot-disk-size=100GB \
  --boot-disk-type=pd-balanced \
  --metadata ^~^tee-image-reference="${IMAGE_REFERENCE}"~\
tee-restart-policy=Always~\
tee-container-log-redirect=true~\
tee-env-ROLLAPP_ID="${ROLLAPP_ID}"~\
tee-env-ROLLER_RELEASE_TAG="${ROLLER_RELEASE_TAG}"~\
tee-env-ROLLER_RA_COMMIT="${ROLLER_RA_COMMIT}"~\
tee-env-ROLLER_RA_GENESIS_STR="${ROLLER_RA_GENESIS_STR}"~\
tee-env-ROLLER_RA_CUSTOM_STR="${ROLLER_RA_CUSTOM_STR}"~\
tee-env-ROLLER_DA_CONFIG_STR="${ROLLER_DA_CONFIG_STR}"~\
  --machine-type=c3-standard-4 \
  --project="${PROJECT_ID}"

# Create firewall rules for RPC and P2P access
create_firewall_rule "allow-tee-rpc" "26657"
create_firewall_rule "allow-tee-p2p" "26656"

wait_for_instance "tee-e2e-fullnode" "${PROJECT_ZONE}"

echo "Waiting for container to initialize (60 seconds)..."
sleep 60

EXTERNAL_IP=$(get_instance_external_ip "tee-e2e-fullnode" "${PROJECT_ZONE}")

echo "Finished: TEE Fullnode External IP: ${EXTERNAL_IP}"