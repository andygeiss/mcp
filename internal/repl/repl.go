// Package repl implements the line-based interactive client behind
// `make repl` (FR9 / Story 3.2). The REPL spawns the mcp server in a
// subprocess and translates a small command vocabulary into JSON-RPC
// requests, then prints the responses pretty-printed for the operator.
//
// Wire-discipline (NFR6) is preserved: this package is a CLIENT, never a
// server. It only imports internal/protocol — no internal/server import,
// honoring Invariant I1.
package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/andygeiss/mcp/internal/protocol"
)

// terminateGrace is the window between SIGINT and SIGKILL when shutting the
// child server. Two seconds matches the project's other graceful-shutdown
// budgets without being long enough to feel hung.
const terminateGrace = 2 * time.Second

// subList is the subcommand name used by every list-style verb (tools list,
// resources list, prompts list). Hoisted so the goconst linter does not flag
// the repeated literal across the parser switches.
const subList = "list"

// Run drives the REPL loop: spawn the server (which the caller has already
// configured via the supplied *exec.Cmd), perform the initialize handshake,
// then read commands from in, send them as JSON-RPC, pretty-print responses
// to out, and emit operational logs to errw. The function returns when the
// user types `quit`, EOF arrives on in, or ctx is cancelled.
//
// The server subprocess is always cleaned up on return — orphan processes
// would be a transport-discipline violation. The cleanup path is best-effort:
// SIGINT, wait up to terminateGrace, then SIGKILL.
func Run(ctx context.Context, in io.Reader, out, errw io.Writer, server *exec.Cmd) error {
	stdin, err := server.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := server.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	server.Stderr = errw
	if err := server.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer terminate(server)

	enc := json.NewEncoder(stdin)
	dec := json.NewDecoder(stdout)

	if err := initialize(enc, dec, errw); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	_, _ = fmt.Fprintf(errw, "connected to mcp (protocol %s); type 'quit' to exit\n", protocol.MCPVersion)

	return readLoop(ctx, in, out, errw, enc, dec)
}

// readLoop is the inner read-eval-print loop. Extracted from Run so each
// function stays under the cognitive-complexity threshold and so the loop
// itself can be unit-tested with in-process pipes if needed.
func readLoop(ctx context.Context, in io.Reader, out, errw io.Writer, enc *json.Encoder, dec *json.Decoder) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16<<20)
	var idCounter int64

	for {
		if err := ctx.Err(); err != nil {
			// ctx cancellation is the normal shutdown path (SIGINT/SIGTERM),
			// not an error — surface nil to the caller so make repl exits 0.
			return nil //nolint:nilerr // intentional: cancellation is graceful shutdown
		}
		_, _ = fmt.Fprint(errw, "mcp> ")
		if !scanner.Scan() {
			break
		}
		cmd := strings.TrimSpace(scanner.Text())
		if cmd == "" {
			continue
		}
		if cmd == "quit" {
			return nil
		}
		idCounter++
		req, parseErr := parseCommand(idCounter, cmd)
		if parseErr != nil {
			_, _ = fmt.Fprintln(errw, "error:", parseErr)
			continue
		}
		if err := sendAndPrint(enc, dec, req, out); err != nil {
			_, _ = fmt.Fprintln(errw, "error:", err)
		}
	}
	return scanner.Err()
}

// parseCommand maps a single line of REPL input to a JSON-RPC request.
func parseCommand(id int64, line string) (protocol.Request, error) {
	verb, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	switch verb {
	case "tools":
		return parseToolsCommand(id, rest)
	case "resources":
		return parseResourcesCommand(id, rest)
	case "prompts":
		return parsePromptsCommand(id, rest)
	}
	return protocol.Request{}, fmt.Errorf("unknown verb %q (allowed: tools | resources | prompts | quit)", verb)
}

func parseToolsCommand(id int64, rest string) (protocol.Request, error) {
	sub, args, _ := strings.Cut(rest, " ")
	args = strings.TrimSpace(args)
	switch sub {
	case subList:
		return makeRequest(id, "tools/list", nil), nil
	case "call":
		name, payload, _ := strings.Cut(args, " ")
		if name == "" {
			return protocol.Request{}, errors.New("tools call: missing tool name")
		}
		params, err := buildToolsCallParams(name, strings.TrimSpace(payload))
		if err != nil {
			return protocol.Request{}, err
		}
		return makeRequest(id, "tools/call", params), nil
	}
	return protocol.Request{}, fmt.Errorf("unknown tools subcommand %q (allowed: list | call)", sub)
}

func parseResourcesCommand(id int64, rest string) (protocol.Request, error) {
	sub, args, _ := strings.Cut(rest, " ")
	args = strings.TrimSpace(args)
	switch sub {
	case subList:
		return makeRequest(id, "resources/list", nil), nil
	case "read":
		if args == "" {
			return protocol.Request{}, errors.New("resources read: missing uri")
		}
		params, err := json.Marshal(map[string]string{"uri": args})
		if err != nil {
			return protocol.Request{}, fmt.Errorf("encode params: %w", err)
		}
		return makeRequest(id, "resources/read", params), nil
	}
	return protocol.Request{}, fmt.Errorf("unknown resources subcommand %q (allowed: list | read)", sub)
}

func parsePromptsCommand(id int64, rest string) (protocol.Request, error) {
	sub, args, _ := strings.Cut(rest, " ")
	args = strings.TrimSpace(args)
	switch sub {
	case subList:
		return makeRequest(id, "prompts/list", nil), nil
	case "get":
		name, payload, _ := strings.Cut(args, " ")
		if name == "" {
			return protocol.Request{}, errors.New("prompts get: missing prompt name")
		}
		params, err := buildPromptsGetParams(name, strings.TrimSpace(payload))
		if err != nil {
			return protocol.Request{}, err
		}
		return makeRequest(id, "prompts/get", params), nil
	}
	return protocol.Request{}, fmt.Errorf("unknown prompts subcommand %q (allowed: list | get)", sub)
}

func buildToolsCallParams(name, payload string) (json.RawMessage, error) {
	args := json.RawMessage(`{}`)
	if payload != "" {
		var probe map[string]any
		if err := json.Unmarshal([]byte(payload), &probe); err != nil {
			return nil, fmt.Errorf("tools call: arguments must be a JSON object: %w", err)
		}
		args = json.RawMessage(payload)
	}
	body, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, fmt.Errorf("encode params: %w", err)
	}
	return body, nil
}

func buildPromptsGetParams(name, payload string) (json.RawMessage, error) {
	args := json.RawMessage(`{}`)
	if payload != "" {
		var probe map[string]any
		if err := json.Unmarshal([]byte(payload), &probe); err != nil {
			return nil, fmt.Errorf("prompts get: arguments must be a JSON object: %w", err)
		}
		args = json.RawMessage(payload)
	}
	body, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, fmt.Errorf("encode params: %w", err)
	}
	return body, nil
}

// makeRequest constructs a JSON-RPC 2.0 Request with the given id and method.
// Params are emitted as-is (json.RawMessage); nil collapses to omitempty.
func makeRequest(id int64, method string, params json.RawMessage) protocol.Request {
	idBytes, _ := json.Marshal(id)
	return protocol.Request{
		ID:      idBytes,
		JSONRPC: protocol.Version,
		Method:  method,
		Params:  params,
	}
}

// sendAndPrint writes the request and reads the matching response, then
// pretty-prints the response to out. Sequential dispatch (AR5) is enforced
// by the read happening before returning.
func sendAndPrint(enc *json.Encoder, dec *json.Decoder, req protocol.Request, out io.Writer) error {
	if err := enc.Encode(req); err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	var resp protocol.Response
	if err := dec.Decode(&resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	pretty, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("pretty-print response: %w", err)
	}
	_, _ = out.Write(pretty)
	_, _ = out.Write([]byte("\n"))
	return nil
}

// initialize sends the MCP initialize handshake, reads the response, and
// follows up with the notifications/initialized notification (per server
// state-machine contract). Reusing the existing protocol types keeps the
// REPL aligned with the on-the-wire shape.
func initialize(enc *json.Encoder, dec *json.Decoder, errw io.Writer) error {
	params, _ := json.Marshal(map[string]any{
		"protocolVersion": protocol.MCPVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "mcp-repl", "version": "0.0.1"},
	})
	idBytes, _ := json.Marshal(int64(0))
	req := protocol.Request{ID: idBytes, JSONRPC: protocol.Version, Method: "initialize", Params: params}
	if err := enc.Encode(req); err != nil {
		return fmt.Errorf("send initialize: %w", err)
	}
	var resp protocol.Response
	if err := dec.Decode(&resp); err != nil {
		return fmt.Errorf("read initialize response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize rejected: %s (code %d)", resp.Error.Message, resp.Error.Code)
	}
	notif := protocol.Notification{JSONRPC: protocol.Version, Method: "notifications/initialized"}
	if err := enc.Encode(notif); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}
	_, _ = fmt.Fprintln(errw, "initialize OK")
	return nil
}

// terminate is the subprocess cleanup path. SIGINT first to give the server
// a chance to log a clean shutdown line on stderr; SIGKILL after the grace
// budget expires so the REPL never leaves an orphan behind.
func terminate(server *exec.Cmd) {
	if server.Process == nil {
		return
	}
	_ = server.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() {
		_ = server.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(terminateGrace):
		_ = server.Process.Kill()
		<-done
	}
}

// keep the context import live: ctx.Err() is checked per iteration in Run.
var _ = context.Background
