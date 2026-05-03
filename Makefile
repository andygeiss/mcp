FUZZTIME ?= 30s
MODULE   ?= github.com/example/myproject

.PHONY: bench build catalog check coverage doc-lint fuzz init inspect-smoke lint new-tool repl setup smoke spec-coverage test

## Run benchmarks and compare against baseline
bench:
	go test -bench=. -count=6 -benchmem ./internal/... > current.txt
	benchstat testdata/benchmarks/baseline.txt current.txt

## Build all packages (reproducible: -trimpath matches release binaries)
build:
	go build -trimpath -ldflags "-X main.version=$$(git describe --tags --always --dirty)" ./cmd/mcp/

## Generate docs/TOOLS.md from the production tool registry (fail on drift)
catalog:
	@TMP=$$(mktemp); \
	 if ! go run ./cmd/mcp-catalog/ > $$TMP 2>&1; then \
	   echo "catalog: generation failed:"; cat $$TMP; rm -f $$TMP; exit 1; \
	 fi; \
	 if cmp -s $$TMP docs/TOOLS.md; then \
	   rm -f $$TMP; \
	   echo "OK: docs/TOOLS.md matches the registry"; \
	 else \
	   mv $$TMP docs/TOOLS.md; \
	   echo "docs/TOOLS.md drifted — file regenerated, please review and commit"; \
	   exit 1; \
	 fi

## Run the full quality pipeline (build, test, lint, doc-lint)
check: build test lint doc-lint

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

## Lint checked-in docs for citations of gitignored paths
doc-lint:
	@found=$$(grep -rnE --include='*.md' '(_bmad-output|_bmad|\.claude|docs/\.archive)/[A-Za-z0-9._-]+' docs/ *.md 2>/dev/null || true); \
	if [ -n "$$found" ]; then \
		echo "FAIL: checked-in docs cite gitignored paths:"; \
		echo "$$found"; \
		echo ""; \
		echo "Move the cited content into a checked-in doc (e.g., docs/agent-rules.md), or remove the citation."; \
		exit 1; \
	fi
	@echo "OK: no gitignored citations in checked-in docs"

## Run fuzz tests (override duration with FUZZTIME=2m)
fuzz:
	go test -fuzz Fuzz_Decoder_With_ArbitraryInput ./internal/protocol -fuzztime=$(FUZZTIME) -timeout=0

## Initialize template with new module path (MODULE=github.com/org/repo)
init:
	go run ./cmd/scaffold/ $(MODULE)

## Run linter (must pass with zero issues)
lint:
	golangci-lint run ./...

## Scaffold a new tool from the template (TOOL=Foo, CamelCase Go identifier)
new-tool:
	@if [ -z "$(TOOL)" ]; then echo "FAIL: TOOL= is required (e.g. make new-tool TOOL=Foo)"; exit 1; fi
	@echo "$(TOOL)" | grep -qE '^[A-Z][A-Za-z0-9]*$$' || { echo "FAIL: TOOL must be a CamelCase Go identifier (got $(TOOL))"; exit 1; }
	@LCASE=$$(echo "$(TOOL)" | tr '[:upper:]' '[:lower:]'); \
	 DEST=internal/tools/$$LCASE.go; \
	 if [ -e "$$DEST" ]; then echo "FAIL: $$DEST already exists"; exit 1; fi; \
	 sed -e '/^\/\/go:build ignore/d' \
	     -e "s/YourTool/$(TOOL)/g" \
	     -e "s/your-tool/$$LCASE/g" \
	     internal/tools/_TOOL_TEMPLATE.go > "$$DEST"; \
	 echo "Created $$DEST"; \
	 echo ""; \
	 echo "Add this to cmd/mcp/main.go inside run():"; \
	 echo "  tools.Register[tools.$(TOOL)Input, tools.$(TOOL)Output](registry, \"$$LCASE\", \"...description...\", tools.$(TOOL))"; \
	 if [ -n "$$EDITOR" ]; then $$EDITOR "$$DEST"; fi

## Spawn the server and drop into the line-based interactive REPL
repl:
	go run ./cmd/mcp-repl/

## Configure local development environment
setup:
	git config core.hooksPath .githooks

## Verify mcp --inspect-only emits valid JSON containing the expected surface (FR7 inspect-smoke)
inspect-smoke:
	@OUT=$$(go run ./cmd/mcp/ --inspect-only 2>&1); \
	 STATUS=$$?; \
	 if [ $$STATUS -ne 0 ]; then \
	   echo "inspect-smoke: --inspect-only exited non-zero ($$STATUS)"; \
	   echo "$$OUT"; exit 1; \
	 fi; \
	 if ! echo "$$OUT" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert isinstance(d.get("tools"), list) and len(d["tools"]) > 0, "tools array missing or empty"; assert d.get("protocolVersion"), "protocolVersion missing"; print("OK: --inspect-only emits valid JSON with", len(d["tools"]), "tool(s)")' 2>&1; then \
	   echo "inspect-smoke: output is not valid JSON or fails contract"; \
	   echo "--- output ---"; echo "$$OUT"; exit 1; \
	 fi

## Verify the server initializes and lists tools (FR5a smoke test)
smoke:
	@STDERR=$$(mktemp); \
	 OUT=$$(printf '%s\n%s\n%s\n' \
	   '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"smoke","version":"0.0.1"}}}' \
	   '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' \
	   '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
	   | go run ./cmd/mcp/ 2>$$STDERR); \
	 TOOLS_LINE=$$(echo "$$OUT" | grep '"result":{"tools":'); \
	 if [ -n "$$TOOLS_LINE" ]; then \
	   if ! echo "$$TOOLS_LINE" | grep -q '"outputSchema":'; then \
	     echo "Smoke test failed: tools/list response missing outputSchema field."; \
	     echo "  AC8 of Story 2.2 requires every tool to advertise outputSchema."; \
	     echo "--- response ---"; echo "$$TOOLS_LINE"; \
	     rm -f $$STDERR; exit 1; \
	   fi; \
	   N=$$(echo "$$TOOLS_LINE" | grep -o '"inputSchema":' | wc -l | tr -d ' '); \
	   echo "Your server works. It exposes $$N tool(s) with outputSchema advertised."; \
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

## Regenerate the spec-coverage audit fragments and the canonical aggregate at docs/spec-coverage.txt
spec-coverage:
	@PROTO=Test_RenderSpecCoverage_Should_MatchProtocolFragment; \
	 OUT=$$(go test -race -run "^$$PROTO$$" -count=1 -v ./internal/protocol/ 2>&1); \
	 STATUS=$$?; \
	 if ! echo "$$OUT" | grep -qE "^(--- PASS|--- FAIL): $$PROTO"; then \
	   echo "spec-coverage: $$PROTO did not run (renamed?)"; \
	   echo "$$OUT"; exit 1; \
	 fi; \
	 echo "$$OUT"; \
	 if [ $$STATUS -ne 0 ]; then exit $$STATUS; fi
	@INSPECT=Test_RenderSpecCoverage_Should_MatchInspectFragment; \
	 OUT=$$(go test -race -run "^$$INSPECT$$" -count=1 -v ./internal/inspect/ 2>&1); \
	 STATUS=$$?; \
	 if ! echo "$$OUT" | grep -qE "^(--- PASS|--- FAIL): $$INSPECT"; then \
	   echo "spec-coverage: $$INSPECT did not run (renamed?)"; \
	   echo "$$OUT"; exit 1; \
	 fi; \
	 echo "$$OUT"; \
	 if [ $$STATUS -ne 0 ]; then exit $$STATUS; fi
	@SCHEMA=Test_RenderSpecCoverage_Should_MatchSchemaFragment; \
	 OUT=$$(go test -race -run "^$$SCHEMA$$" -count=1 -v ./internal/schema/ 2>&1); \
	 STATUS=$$?; \
	 if ! echo "$$OUT" | grep -qE "^(--- PASS|--- FAIL): $$SCHEMA"; then \
	   echo "spec-coverage: $$SCHEMA did not run (renamed?)"; \
	   echo "$$OUT"; exit 1; \
	 fi; \
	 echo "$$OUT"; \
	 if [ $$STATUS -ne 0 ]; then exit $$STATUS; fi
	@SRV=Test_RenderSpecCoverage_Should_MatchServerFragment; \
	 OUT=$$(go test -race -tags=integration -run "^$$SRV$$" -count=1 -v ./internal/server/ 2>&1); \
	 STATUS=$$?; \
	 if ! echo "$$OUT" | grep -qE "^(--- PASS|--- FAIL): $$SRV"; then \
	   echo "spec-coverage: $$SRV did not run (renamed?)"; \
	   echo "$$OUT"; exit 1; \
	 fi; \
	 echo "$$OUT"; \
	 if [ $$STATUS -ne 0 ]; then exit $$STATUS; fi
	@TMP=$$(mktemp); \
	 if ! go run ./cmd/spec-coverage/ > $$TMP 2>&1; then \
	   echo "spec-coverage: aggregator failed:"; cat $$TMP; rm -f $$TMP; exit 1; \
	 fi; \
	 if cmp -s $$TMP docs/spec-coverage.txt; then \
	   rm -f $$TMP; \
	   echo "OK: docs/spec-coverage.txt matches the aggregated registry"; \
	 else \
	   mv $$TMP docs/spec-coverage.txt; \
	   echo "docs/spec-coverage.txt drifted — file regenerated, please review and commit (run \`make spec-coverage\`)"; \
	   exit 1; \
	 fi

## Run all tests with race detector
test:
	go test -race ./...
