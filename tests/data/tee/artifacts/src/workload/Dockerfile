# TEE Fullnode Workload Dockerfile
# Use Ubuntu 22.04 as base image - will run inside Confidential Space VM
FROM ubuntu:22.04

# Install runtime dependencies including build tools
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    wget \
    git \
    build-essential \
    procps \
    bash \
    jq \
    tar \
    gzip \
    make \
    gcc \
    g++ \
    clang\
    curl\
    aria2\
    wget\
    tar\
    jq\
    libssl-dev\
    pkg-config\
    make\
    sudo\
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.23.4+ for building roller from source (rollapp requires >= 1.23.4)
RUN wget -q https://go.dev/dl/go1.24.6.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz && \
    rm go1.24.6.linux-amd64.tar.gz

# Set Go environment
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:${PATH}"

# Create application directory
WORKDIR /app

# Copy entrypoint script
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Expose RPC and P2P ports for external access
EXPOSE 26657 26656

# Set launch policy labels for Confidential Space (see https://cloud.google.com/confidential-computing/confidential-space/docs/create-customize-workloads)
LABEL "tee.launch_policy.allow_env_override"="ROLLAPP_ID,ROLLER_RELEASE_TAG,ROLLER_RA_COMMIT,ROLLER_RA_GENESIS_STR,ROLLER_RA_CUSTOM_STR,ROLLER_DA_CONFIG_STR"
LABEL "tee.launch_policy.log_redirect"="always"
LABEL "tee.launch_policy.allow_cmd_override"="false"

# Set entrypoint
ENTRYPOINT ["/app/entrypoint.sh"]