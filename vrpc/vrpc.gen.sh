#!/usr/bin/env sh

# ensure we installed the protoc-gen-go and protoc-gen-go-grpc binaries
# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

clang-format -i ./vrpc.proto
protoc --go_out=. --go-grpc_out=. ./vrpc.proto