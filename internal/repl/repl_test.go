// White-box test file: tests the unexported parseCommand function directly.
// The black-box surface (Run) is exercised by the integration tests.
//
//nolint:testpackage // intentionally same-package for unexported parser tests
package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

func Test_parseCommand_With_ToolsList_Should_BuildToolsListRequest(t *testing.T) {
	t.Parallel()

	// Act
	req, err := parseCommand(1, "tools list")

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "method", req.Method, "tools/list")
	assert.That(t, "jsonrpc version", req.JSONRPC, protocol.Version)
	assert.That(t, "id encoded", string(req.ID), "1")
}

func Test_parseCommand_With_ToolsCall_Should_EncodeNameAndArguments(t *testing.T) {
	t.Parallel()

	// Act
	req, err := parseCommand(2, `tools call echo {"message":"hi"}`)

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "method", req.Method, "tools/call")
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	assert.That(t, "unmarshal params", json.Unmarshal(req.Params, &params), nil)
	assert.That(t, "tool name", params.Name, "echo")
	assert.That(t, "arguments preserved", string(params.Arguments), `{"message":"hi"}`)
}

func Test_parseCommand_With_ToolsCallNoArgs_Should_DefaultToEmptyObject(t *testing.T) {
	t.Parallel()

	// Act
	req, err := parseCommand(3, "tools call ping")

	// Assert
	assert.That(t, "no error", err, nil)
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal(req.Params, &params)
	assert.That(t, "tool name", params.Name, "ping")
	assert.That(t, "arguments default {}", string(params.Arguments), `{}`)
}

func Test_parseCommand_With_ToolsCallInvalidJSON_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := parseCommand(4, `tools call echo {bad json}`)

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names failure mode", strings.Contains(err.Error(), "must be a JSON object"), true)
}

func Test_parseCommand_With_ToolsCallMissingName_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := parseCommand(5, "tools call")

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names failure", strings.Contains(err.Error(), "missing tool name"), true)
}

func Test_parseCommand_With_ResourcesRead_Should_EncodeURI(t *testing.T) {
	t.Parallel()

	// Act
	req, err := parseCommand(6, "resources read file:///foo")

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "method", req.Method, "resources/read")
	var params map[string]string
	_ = json.Unmarshal(req.Params, &params)
	assert.That(t, "uri", params["uri"], "file:///foo")
}

func Test_parseCommand_With_ResourcesReadMissingURI_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := parseCommand(7, "resources read")

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names missing uri", strings.Contains(err.Error(), "missing uri"), true)
}

func Test_parseCommand_With_PromptsList_Should_BuildPromptsListRequest(t *testing.T) {
	t.Parallel()

	// Act
	req, err := parseCommand(8, "prompts list")

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "method", req.Method, "prompts/list")
}

func Test_parseCommand_With_PromptsGet_Should_EncodeArguments(t *testing.T) {
	t.Parallel()

	// Act
	req, err := parseCommand(9, `prompts get summarize {"topic":"go"}`)

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "method", req.Method, "prompts/get")
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal(req.Params, &params)
	assert.That(t, "prompt name", params.Name, "summarize")
	assert.That(t, "args", string(params.Arguments), `{"topic":"go"}`)
}

func Test_parseCommand_With_UnknownVerb_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := parseCommand(10, "frobnicate something")

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names allowed verbs", strings.Contains(err.Error(), "allowed: tools | resources | prompts | quit"), true)
}

func Test_parseCommand_With_UnknownToolsSubcommand_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := parseCommand(11, "tools nonsense")

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names allowed", strings.Contains(err.Error(), "allowed: list | call"), true)
}

func Test_parseCommand_With_UnknownResourcesSubcommand_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := parseCommand(12, "resources nonsense")

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names allowed", strings.Contains(err.Error(), "allowed: list | read"), true)
}

func Test_parseCommand_With_UnknownPromptsSubcommand_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := parseCommand(13, "prompts nonsense")

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names allowed", strings.Contains(err.Error(), "allowed: list | get"), true)
}

func Test_parseCommand_With_PromptsGetMissingName_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Act
	_, err := parseCommand(14, "prompts get")

	// Assert
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names missing name", strings.Contains(err.Error(), "missing prompt name"), true)
}

// pipePair returns a connected encoder/decoder backed by an in-memory
// io.Pipe so the loop functions can be tested without spawning a process.
// The returned func releases both writer ends.
func pipePair() (*json.Encoder, *json.Decoder, *json.Encoder, *json.Decoder, func()) {
	cliR, cliW := io.Pipe()
	srvR, srvW := io.Pipe()
	return json.NewEncoder(cliW), json.NewDecoder(cliR), json.NewEncoder(srvW), json.NewDecoder(srvR), func() {
		_ = cliW.Close()
		_ = srvW.Close()
	}
}

func Test_sendAndPrint_With_SuccessfulResponse_Should_PrettyPrintToOut(t *testing.T) {
	t.Parallel()

	// Arrange — set up the client side and a goroutine playing the server.
	cliEnc, srvDec, srvEnc, cliDec, closeAll := pipePair()
	defer closeAll()
	go func() {
		var req protocol.Request
		_ = srvDec.Decode(&req)
		_ = srvEnc.Encode(protocol.Response{
			ID:      req.ID,
			JSONRPC: protocol.Version,
			Result:  json.RawMessage(`{"ok":true}`),
		})
	}()

	// Act
	var out bytes.Buffer
	id, _ := json.Marshal(int64(1))
	req := protocol.Request{ID: id, JSONRPC: protocol.Version, Method: "tools/list"}
	err := sendAndPrint(cliEnc, cliDec, req, &out)

	// Assert
	assert.That(t, "no error", err, nil)
	assert.That(t, "ok field present", strings.Contains(out.String(), `"ok": true`), true)
}

func Test_initialize_With_SuccessfulHandshake_Should_LogReady(t *testing.T) {
	t.Parallel()

	cliEnc, srvDec, srvEnc, cliDec, closeAll := pipePair()
	defer closeAll()
	go func() {
		// Read initialize, send response, read notifications/initialized.
		var req protocol.Request
		_ = srvDec.Decode(&req)
		_ = srvEnc.Encode(protocol.Response{
			ID:      req.ID,
			JSONRPC: protocol.Version,
			Result:  json.RawMessage(`{}`),
		})
		var notif protocol.Request
		_ = srvDec.Decode(&notif)
	}()

	var errw bytes.Buffer
	err := initialize(cliEnc, cliDec, &errw)
	assert.That(t, "no error", err, nil)
	assert.That(t, "logs initialize OK", strings.Contains(errw.String(), "initialize OK"), true)
}

func Test_initialize_With_ServerError_Should_ReturnError(t *testing.T) {
	t.Parallel()

	cliEnc, srvDec, srvEnc, cliDec, closeAll := pipePair()
	defer closeAll()
	go func() {
		var req protocol.Request
		_ = srvDec.Decode(&req)
		_ = srvEnc.Encode(protocol.Response{
			ID:      req.ID,
			JSONRPC: protocol.Version,
			Error:   &protocol.Error{Code: -32000, Message: "not initialized"},
		})
	}()

	var errw bytes.Buffer
	err := initialize(cliEnc, cliDec, &errw)
	assert.That(t, "error returned", err != nil, true)
	assert.That(t, "names server message", strings.Contains(err.Error(), "not initialized"), true)
}

func Test_readLoop_With_QuitCommand_Should_ExitCleanly(t *testing.T) {
	t.Parallel()

	cliEnc, srvDec, srvEnc, cliDec, closeAll := pipePair()
	defer closeAll()
	go func() {
		// Server goroutine: respond to whatever the loop sends. quit alone
		// produces no requests; we just drain anything that arrives.
		for {
			var req protocol.Request
			if err := srvDec.Decode(&req); err != nil {
				return
			}
			_ = srvEnc.Encode(protocol.Response{ID: req.ID, JSONRPC: protocol.Version, Result: json.RawMessage(`null`)})
		}
	}()

	var out, errw bytes.Buffer
	err := readLoop(context.Background(), strings.NewReader("quit\n"), &out, &errw, cliEnc, cliDec)
	assert.That(t, "no error on quit", err, nil)
}

func Test_readLoop_With_CancelledContext_Should_ExitWithoutError(t *testing.T) {
	t.Parallel()

	cliEnc, _, _, cliDec, closeAll := pipePair()
	defer closeAll()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out, errw bytes.Buffer
	err := readLoop(ctx, strings.NewReader(""), &out, &errw, cliEnc, cliDec)
	assert.That(t, "no error on cancellation", err, nil)
}

func Test_readLoop_With_InvalidCommand_Should_StayAtPromptAndContinue(t *testing.T) {
	t.Parallel()

	cliEnc, _, _, cliDec, closeAll := pipePair()
	defer closeAll()

	var out, errw bytes.Buffer
	// Two lines: a bad verb followed by quit. The loop must report the
	// parse error to errw and then accept quit.
	script := "frobnicate\nquit\n"
	err := readLoop(context.Background(), strings.NewReader(script), &out, &errw, cliEnc, cliDec)
	assert.That(t, "no error", err, nil)
	assert.That(t, "errw names unknown verb", strings.Contains(errw.String(), "unknown verb"), true)
}
