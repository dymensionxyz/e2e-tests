#!/bin/bash
#
# Cleanup script for TEE Fullnode resources

set -e

source workload_env.sh

echo "==========================================="
echo "Cleaning up TEE Fullnode Resources"
echo "==========================================="
echo "Project ID: ${PROJECT_ID}"
echo ""

# Set project
gcloud config set project "${PROJECT_ID}"

# Delete compute instance
echo "Deleting TEE Fullnode instance..."
gcloud compute instances delete tee-fullnode \
  --zone="${PROJECT_ZONE}" \
  --project="${PROJECT_ID}" \
  --quiet 2>/dev/null || echo "Instance not found or already deleted"

# Delete firewall rules
echo ""
echo "Deleting firewall rules..."
gcloud compute firewall-rules delete allow-tee-rpc \
  --project="${PROJECT_ID}" \
  --quiet 2>/dev/null || echo "Firewall rule allow-tee-rpc not found"

gcloud compute firewall-rules delete allow-tee-p2p \
  --project="${PROJECT_ID}" \
  --quiet 2>/dev/null || echo "Firewall rule allow-tee-p2p not found"

# Delete service account
echo ""
echo "Deleting service account..."
gcloud iam service-accounts delete \
  "${WORKLOAD_SERVICE_ACCOUNT}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --project="${PROJECT_ID}" \
  --quiet 2>/dev/null || echo "Service account not found or already deleted"

# Optionally delete artifact repository
echo ""
read -p "Delete artifact repository ${ARTIFACT_REPOSITORY}? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Deleting artifact repository..."
    gcloud artifacts repositories delete "${ARTIFACT_REPOSITORY}" \
      --location="${PROJECT_REPOSITORY_REGION}" \
      --project="${PROJECT_ID}" \
      --quiet 2>/dev/null || echo "Repository not found or already deleted"
fi
