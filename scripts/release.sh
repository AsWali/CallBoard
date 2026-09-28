#!/bin/bash
set -e
cd "$(dirname "$0")/.."
go build -o bin/callboard ./cmd/callboard
v="${VERSION:-$(bin/callboard version | awk '{print $NF}')}"
mkdir -p dist
for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os="${t%/*}" arch="${t#*/}"
  out="dist/callboard-$v-$os-$arch"; [ "$os" = windows ] && out="$out.exe"
  echo "· building $out"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$out" ./cmd/callboard
  echo "✓ $out"
done
(cd dist && shasum -a 256 callboard-"$v"-* > "callboard-$v-sha256.txt") && echo "✓ dist/callboard-$v-sha256.txt"
echo "Release binaries are in dist/. On another machine: download one, rename it to callboard, then: ./callboard install"
