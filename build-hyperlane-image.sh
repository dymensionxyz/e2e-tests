#!/bin/bash
set -e

echo "Building local Hyperlane CLI Docker image..."

# Build the Docker image from your local fork
docker build -f hyperlane-cli.dockerfile \
  --build-context context=/Users/danwt/Documents/dym \
  -t local/hyperlane-cli:latest \
  .

echo "Docker image built: local/hyperlane-cli:latest"