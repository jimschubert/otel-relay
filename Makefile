VERSION=dev
COMMIT=$(shell git rev-parse --short HEAD)

LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT}"

.PHONY: proto build test clean install help

default: build

help:
	@echo "Available targets:"
	@echo "  proto    - Generate Go code from protobuf definitions"
	@echo "  build    - Build all binaries"
	@echo "  test     - Run tests"
	@echo "  clean    - Remove built binaries"
	@echo "  install  - Install binaries to GOPATH/bin"

proto:
	protoc --go_out=. --go_opt=module=github.com/jimschubert/otel-relay \
		--go-grpc_out=. --go-grpc_opt=module=github.com/jimschubert/otel-relay \
		proto/inspector.proto

build:
	CGO_ENABLED=0 go build ${LDFLAGS} -o dist/otel-relay ./cmd/otel-relay
	CGO_ENABLED=0 go build ${LDFLAGS} -o dist/otel-inspector ./cmd/otel-inspector

test:
	go test -race ./...

clean:
	rm -rf dist/
	go clean

install:
	go install ./cmd/otel-relay
	go install ./cmd/otel-inspector

fix:
	go fix ./...
