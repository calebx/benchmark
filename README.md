# VRPC: Virtual Remote Procedure Call Framework

VRPC is a high-performance RPC framework built on top of gRPC, 
supporting streaming and request batching over TCP or vsock transports. 
It provides both server and client libraries, along with example "enclave" (RPC server) and "host" (HTTP API proxy) applications 
to demonstrate secure and efficient communication, especially suited for enclave-based environments.

## Features
  - gRPC bidirectional streaming with request batching for improved throughput
  - TCP and Virtio vsock transport support (for enclave-host communication)
  - Pluggable Go dispatcher with context-aware timeouts and error handling
  - Client connection pooling for concurrent request invocation
  - Example commands under `cmd/enclave` (RPC server) and `cmd/host` (HTTP proxy)
  - Benchmarking support with `wrk` and provided Lua script (`post.lua`)

## Prerequisites
  - Go 1.20 or later
  - Protocol Buffers compiler (`protoc`) and Go plugins (`protoc-gen-go`, `protoc-gen-go-grpc`)
  - (Optional) Docker for containerizing the enclave server
  - (Optional) `wrk` for HTTP benchmarking

## Code Generation
Generate Go code from the protobuf definition:
```bash
cd vrpc
./vrpc.gen.sh
cd ..
```

## Build
Compile the example applications:
```bash
go build -o build/server ./cmd/enclave
go build -o build/client ./cmd/host
```

### Docker (optional)
Build and run the enclave server in a container:
```bash
docker build -t vrpc .
docker run -p 50001:50001 vrpc
```

## Usage
1. Start the enclave (RPC server):
   ```bash
   ./build/server
   ```
2. In a separate terminal, start the host (HTTP proxy):
   ```bash
   ./build/client
   ```
3. Send a test request:
   ```bash
   curl -X POST http://localhost:1323/echo -H 'Content-Type: application/json' -d '{"xid":"hello"}'
   # response: {"dix":"olleh"}
   ```

## Benchmarking
Use `wrk` to load-test the HTTP endpoint with the provided Lua script:
```bash
wrk -t4 -c100 -d30s -s post.lua http://127.0.0.1:1323/echo
```

## Project Structure
```
. 
├── cmd
│   ├── enclave       # RPC server example (reverses input)
│   └── host          # HTTP proxy example (forwards to enclave)
├── vrpc              # Core VRPC library and transport
│   ├── gen           # Generated gRPC code
│   ├── vrpc.proto    # Protobuf definitions
│   ├── *.go          # Dispatcher, server, client implementations
│   └── vrpc.gen.sh   # Code generation script
├── build             # Compiled binaries (server, client)
├── Dockerfile        # Container image for the enclave server
├── post.lua          # wrk benchmark script
├── go.mod            # Go module definition
└── go.sum            # Go dependencies checksum
```
