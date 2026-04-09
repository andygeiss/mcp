FUZZTIME ?= 30s
MODULE   ?= github.com/example/myproject

.PHONY: bench build check coverage fuzz init lint setup test

## Run benchmarks and compare against baseline
bench:
	go test -bench=. -count=6 -benchmem ./internal/... > current.txt
	benchstat testdata/benchmarks/baseline.txt current.txt

## Build all packages
build:
	go build ./...

## Run the full quality pipeline (build, test, lint)
check: build test lint

## Generate test coverage report and enforce threshold
coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@total=$$(go tool cover -func=coverage.out | grep '^total:' | awk '{print $$NF}' | tr -d '%'); \
	threshold=75; \
	if [ "$$(echo "$$total < $$threshold" | bc)" -eq 1 ]; then \
		echo "FAIL: coverage $${total}% is below threshold $${threshold}%"; \
		exit 1; \
	fi; \
	echo "OK: coverage $${total}% meets threshold $${threshold}%"

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
