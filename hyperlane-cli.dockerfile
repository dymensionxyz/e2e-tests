FROM node:20-alpine

WORKDIR /root

RUN apk add --update --no-cache git g++ make py3-pip jq

RUN yarn set version 4.5.1

# Option 1: Install from npm registry (the original package)
# RUN npm install -g @daniel.dymension.xyz/hyperlane-cli

# Option 2: Build from your local fork
# First install dependencies and build the entire monorepo
COPY d-hyperlane-monorepo /hyperlane-monorepo
WORKDIR /hyperlane-monorepo

RUN yarn install && yarn build

# Install the CLI globally from the local build
WORKDIR /hyperlane-monorepo/typescript/cli
RUN npm link

WORKDIR /root