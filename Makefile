FUZZTIME ?= 30s
MODULE   ?= github.com/example/myproject

.PHONY: bench build check cover fuzz init lint setup test

## Run benchmarks (6 iterations for benchstat)
bench:
	go test -bench=. -count=6 ./...

## Build all packages
build:
	go build ./...

## Run the full quality pipeline (build, test, lint)
check: build test lint

## Run tests with coverage report
cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## Run fuzz tests (override duration with FUZZTIME=2m)
fuzz:
	go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=$(FUZZTIME)

## Initialize template with new module path (MODULE=github.com/org/repo)
init:
	go run ./cmd/init/ $(MODULE)

## Run linter (must pass with zero issues)
lint:
	golangci-lint run ./...

## Configure local development environment
setup:
	git config core.hooksPath .githooks

## Run all tests with race detector
test:
	go test -race ./...
