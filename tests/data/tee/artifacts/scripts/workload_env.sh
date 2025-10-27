#!/bin/bash
#
# Common environment variables for TEE Fullnode demo

# GCP Configuration
export PROJECT_ID="dymension-ops"  
export MEMBER="user:duc@decentrio.ventures"  
export PROJECT_REGION="us-central1"  
export PROJECT_ZONE="us-central1-a"  
export PROJECT_LOCATION="global"  # Usually "global" for cross-regional resources

# Artifact Registry Configuration
export ARTIFACT_REPOSITORY="dymension-ops"  
export PROJECT_REPOSITORY_REGION="us"  

# Service Account Configuration
export WORKLOAD_SERVICE_ACCOUNT="dym-dev-team-tee"  # Service account name
export WORKLOAD_IMAGE_NAME="e2e-fullnode"  # Docker image name
export WORKLOAD_IMAGE_TAG="latest"  # Docker image tag

# Workload things
export ROLLAPP_ID="forktest_879525-1"
export ROLLER_RELEASE_TAG="v1.18.0-rc15" # download specific roller version
export ROLLER_RA_COMMIT="b629b7d8779f81be09f94d20987213823569c37b"; # force roller to download specific rollapp version

# 1. Genesis: pass in with env var, then write to file
# 2. Custom env: pass in with env var, then write to file
# 3. DA Config: pass in with env var, then write using config set
export ROLLER_RA_GENESIS_STR=$(cat data/genesis.json) # avoid jq to avoid digest changes
export ROLLER_RA_CUSTOM_STR=$(cat data/roller_custom_env.json | jq -c .)
export ROLLER_DA_CONFIG_STR=$(cat data/da_config.json | jq -c .)

export GCP_IS_CONFIDENTIAL_DEBUG=true