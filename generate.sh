#!/usr/bin/env bash
# Generates Go code from the .proto files into gen/
set -e

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc is required. Install it with your system package manager (for example: sudo apt-get install protobuf-compiler)." >&2
  exit 1
fi

go_bin="$(go env GOBIN)"
if [ -z "$go_bin" ]; then
  go_bin="$(go env GOPATH)/bin"
fi
case ":$PATH:" in
  *":$go_bin:"*) ;;
  *) PATH="$go_bin:$PATH"; export PATH ;;
esac

if ! command -v protoc-gen-go >/dev/null 2>&1 || ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "Go protobuf plugins are missing. Install them with:" >&2
  echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12" >&2
  echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1" >&2
  exit 1
fi

protoc --go_out=. --go_opt=module=trailbooking \
       --go-grpc_out=. --go-grpc_opt=module=trailbooking \
       proto/trail.proto proto/booking.proto
echo "Generated:"; ls gen/trailpb gen/bookingpb
