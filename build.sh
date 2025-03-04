#!/bin/bash

# Pre requisites
# go1.22.12
# apt install -y g++-aarch64-linux-gnu
# apt install -y g++-arm-linux-gnueabi

export CGO_ENABLED=1
export TWINE_USERNAME=__token__

# if go1.22.12 is not available, use `go`
if [ -z "$(command -v go1.22.12)" ]; then
    alias go1.22.12=go
fi

# Check if VERSION environment is set
if [ -z "$VERSION" ]; then
  echo "ERROR: VERSION environment variable is not set"
  exit 1
fi

# Check if TWINE_PASSWORD is set
if [ -z "$TWINE_PASSWORD" ]; then
  echo "ERROR: TWINE_PASSWORD environment variable is not set"
  exit 1
fi

GOOS=linux GOARCH=amd64 go1.22.12 build -buildmode=c-shared -o ./filewarmer/lib/file_warmer_linux_amd64.so
GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc go1.22.12 build -buildmode=c-shared -o ./filewarmer/lib/file_warmer_linux_arm64.so
rm -rf dist
rm -rf build
pip install twine
cat setup.py | sed -i "s/version=\"[^\"]*\"/version=\"$VERSION\"/" setup.py
python3 setup.py sdist
twine upload dist/* --non-interactive 