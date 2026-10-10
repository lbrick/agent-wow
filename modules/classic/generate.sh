#!/bin/sh
# Regenerates the classic modules' Go bindings and descriptor sets. Runs from
# the repository root inside golang:1.27.1 (see README.md in this directory):
#
#   podman run --rm -v "$PWD:/src" -w /src docker.io/library/golang:1.27.1 sh modules/classic/generate.sh
set -eu
command -v protoc >/dev/null || { apt-get update -qq && apt-get install -y -qq protobuf-compiler >/dev/null; }
go build -o /tmp/bin/ google.golang.org/protobuf/cmd/protoc-gen-go google.golang.org/grpc/cmd/protoc-gen-go-grpc
for m in world movement combat time; do
  protoc -I . -I /usr/include \
    --plugin=protoc-gen-go=/tmp/bin/protoc-gen-go --plugin=protoc-gen-go-grpc=/tmp/bin/protoc-gen-go-grpc \
    --go_out=. --go_opt=module=github.com/agent-wow/agent-wow \
    --go-grpc_out=. --go-grpc_opt=module=github.com/agent-wow/agent-wow \
    --include_imports --descriptor_set_out=modules/classic/$m/module.pb \
    modules/classic/$m/module.proto
  echo "generated $m"
done
