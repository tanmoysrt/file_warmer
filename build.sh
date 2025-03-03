#!/bin/bash

# Pre requisites
# go1.22.12
# apt install -y g++-aarch64-linux-gnu
# apt install -y g++-arm-linux-gnueabi

export CGO_ENABLED=1
GOOS=linux GOARCH=amd64 go1.22.12 build -buildmode=c-shared -o ./lib/fwup_linux_amd64.so
GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc go1.22.12 build -buildmode=c-shared -o ./lib/fwup_linux_arm64.so

