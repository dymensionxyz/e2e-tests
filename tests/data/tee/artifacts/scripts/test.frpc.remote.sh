#!/bin/bash

# Check architecture
echo "Checking system architecture..."
uname -m

# Clean up old files
rm -rf ~/frp_*

# Download correct frp based on architecture
ARCH=$(uname -m)
if [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    echo "Downloading ARM64 version..."
    wget https://github.com/fatedier/frp/releases/download/v0.61.0/frp_0.61.0_linux_arm64.tar.gz
    tar -xzf frp_0.61.0_linux_arm64.tar.gz
    cd frp_0.61.0_linux_arm64
else
    echo "Downloading AMD64 version..."
    wget https://github.com/fatedier/frp/releases/download/v0.61.0/frp_0.61.0_linux_amd64.tar.gz
    tar -xzf frp_0.61.0_linux_amd64.tar.gz
    cd frp_0.61.0_linux_amd64
fi

# Run server
./frps -c ~/frpc.server.toml