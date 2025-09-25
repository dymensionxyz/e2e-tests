#!/bin/bash
set -e

echo "Building local Hyperlane CLI Docker image..."

cd /Users/danwt/Documents/dym/d-e2e-tests

# Build the Docker image with the hyperlane monorepo
docker build -f hyperlane-cli.dockerfile \
  -t local/hyperlane-cli:latest \
  /Users/danwt/Documents/dym

echo "Docker image built: local/hyperlane-cli:latest"

# Clean up any existing containers and volumes
echo "Cleaning up existing containers and volumes..."
docker container rm -f $(docker container ls -a -q) || true
docker volume prune --all --force
docker network prune --force

# Run the test with the containers kept for debugging
echo "Running the test..."
export KEEP_CONTAINERS="true"
make e2e-test-ibc-rol-eth-wasm