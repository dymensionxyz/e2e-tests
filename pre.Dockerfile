FROM golang:1.19-alpine3.16 as go-builder

ARG LINK_STATICALLY

ENV PACKAGES curl make git libc-dev bash gcc linux-headers eudev-dev python3

RUN apk add --no-cache $PACKAGES

RUN git clone -b 'v3.0.0' --single-branch --depth 1 https://github.com/dymensionxyz/dymension.git /dymension

WORKDIR /dymension

RUN make build

FROM alpine:3.16.1

RUN apk add curl jq bash vim 

COPY --from=go-builder /dymension/build/dymd /usr/local/bin/

WORKDIR /dymension

COPY scripts/* ./scripts/

ENV CHAIN_ID=local-testnet
ENV KEY_NAME=local-user
ENV MONIKER_NAME=local

RUN chmod +x ./scripts/*.sh

EXPOSE 26656 26657 1317 9090
