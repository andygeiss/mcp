FUZZTIME ?= 30s
MODULE   ?= github.com/example/myproject

.PHONY: bench build check coverage fuzz init lint setup smoke test

## Run benchmarks and compare against baseline
bench:
	go test -bench=. -count=6 -benchmem ./internal/... > current.txt
	benchstat testdata/benchmarks/baseline.txt current.txt

## Build all packages (reproducible: -trimpath matches release binaries)
build:
	go build -trimpath -ldflags "-X main.version=$$(git describe --tags --always --dirty)" ./cmd/mcp/

## Run the full quality pipeline (build, test, lint)
check: build test lint

## Generate test coverage report and enforce threshold
coverage:
	go test -race -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out
	@total=$$(go tool cover -func=coverage.out | grep '^total:' | awk '{print $$NF}' | tr -d '%'); \
	threshold=90; \
	if [ "$$(echo "$$total < $$threshold" | bc)" -eq 1 ]; then \
		echo "FAIL: coverage $${total}% is below threshold $${threshold}%"; \
		exit 1; \
	fi; \
	echo "OK: coverage $${total}% meets threshold $${threshold}%"

## Run fuzz tests (override duration with FUZZTIME=2m)
fuzz:
	go test -fuzz Fuzz_Decoder_With_ArbitraryInput ./internal/protocol -fuzztime=$(FUZZTIME) -timeout=0

## Initialize template with new module path (MODULE=github.com/org/repo)
init:
	go run ./cmd/scaffold/ $(MODULE)

## Run linter (must pass with zero issues)
lint:
	golangci-lint run ./...

## Configure local development environment
setup:
	git config core.hooksPath .githooks

## Verify the server initializes and lists tools (FR5a smoke test)
smoke:
	@STDERR=$$(mktemp); \
	 OUT=$$(printf '%s\n%s\n%s\n' \
	   '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"smoke","version":"0.0.1"}}}' \
	   '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' \
	   '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
	   | go run ./cmd/mcp/ 2>$$STDERR); \
	 if echo "$$OUT" | grep -q '"result":{"tools":'; then \
	   N=$$(echo "$$OUT" | grep -o '"name":"' | wc -l | tr -d ' '); \
	   echo "Your server works. It exposes $$N tool(s)."; \
	   rm -f $$STDERR; exit 0; \
	 else \
	   echo "Smoke test failed."; \
	   echo ""; \
	   echo "Common causes:"; \
	   echo "  - Forgot to register your tool in cmd/mcp/main.go?"; \
	   echo "  - Tool handler doesn't compile? Run: go build ./..."; \
	   echo ""; \
	   echo "--- stderr ---"; \
	   cat $$STDERR; \
	   rm -f $$STDERR; exit 1; \
	 fi

## Run all tests with race detector
test:
	go test -race ./...
