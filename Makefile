FUZZTIME ?= 30s
MODULE   ?= github.com/example/myproject

.PHONY: build check coverage fuzz init lint test

## Run the full quality pipeline (build, test, lint)
check: build test lint

## Build all packages
build:
	go build ./...

## Generate test coverage report
coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## Run fuzz tests (override duration with FUZZTIME=2m)
fuzz:
	go test -fuzz Fuzz_Decoder ./internal/protocol -fuzztime=$(FUZZTIME)

## Initialize template with new module path (MODULE=github.com/org/repo)
init:
	go run ./cmd/init/ -module=$(MODULE)

## Run linter (must pass with zero issues)
lint:
	golangci-lint run ./...

## Run all tests with race detector
test:
	go test -race ./...
