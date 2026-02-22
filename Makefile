.PHONY: proto build test clean install help

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
	go build -o dist/otel-relay ./cmd/otel-relay
	go build -o dist/otel-inspector ./cmd/otel-inspector

test:
	go test ./...

clean:
	rm -rf bin/
	go clean

install:
	go install ./cmd/otel-relay
	go install ./cmd/otel-inspector
