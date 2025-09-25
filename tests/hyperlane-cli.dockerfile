FROM node:20-alpine

WORKDIR /root

RUN apk add --update --no-cache git g++ make py3-pip jq

RUN yarn set version 4.5.1

# Copy and build the local Hyperlane CLI from your fork
COPY d-hyperlane-monorepo /hyperlane-monorepo
WORKDIR /hyperlane-monorepo

# Build the CLI
RUN yarn install && \
    yarn build

# Install the CLI globally from the local build
WORKDIR /hyperlane-monorepo/typescript/cli
RUN npm link

WORKDIR /root