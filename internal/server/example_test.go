package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// ExampleNewServer demonstrates the wiring pattern: create a registry,
// register tools, then construct a server with injected I/O. In production
// code stdin/stdout/stderr are os.Stdin, os.Stdout, os.Stderr; in tests they
// are bytes.Buffer for full control.
func ExampleNewServer() {
	r := tools.NewRegistry()
	if err := tools.Register(r, "echo", "Echoes the input message", tools.Echo); err != nil {
		fmt.Println("error:", err)
		return
	}

	var stdout, stderr bytes.Buffer
	stdin := bytes.NewBufferString("") // empty = immediate EOF

	srv := server.NewServer("myserver", "1.0.0", r, stdin, &stdout, &stderr)

	err := srv.Run(context.Background())
	fmt.Println("error:", err)
	// Output:
	// error: <nil>
}

// Example_fullToolLifecycle demonstrates the complete MCP request-response
// cycle: register a tool, send initialize + tools/list + tools/call, and
// read human-readable summaries of each response.
func Example_fullToolLifecycle() {
	r := tools.NewRegistry()
	if err := tools.Register(r, "echo", "Echoes the message back", tools.Echo); err != nil {
		fmt.Println("error:", err)
		return
	}

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"message":"Hello, world!"}}}`,
	}, "\n") + "\n"

	var stdout, stderr bytes.Buffer
	srv := server.NewServer("example", "0.1.0", r, strings.NewReader(input), &stdout, &stderr)
	_ = srv.Run(context.Background())

	dec := json.NewDecoder(&stdout)
	for {
		var resp protocol.Response
		if err := dec.Decode(&resp); err != nil {
			break
		}
		switch string(resp.ID) {
		case "1":
			fmt.Println("Initialize: ok")
		case "2":
			var list struct{ Tools []struct{ Name string } }
			_ = json.Unmarshal(resp.Result, &list)
			names := make([]string, len(list.Tools))
			for i, t := range list.Tools {
				names[i] = t.Name
			}
			fmt.Printf("Tools: %v\n", names)
		case "3":
			var result struct{ Content []struct{ Text string } }
			_ = json.Unmarshal(resp.Result, &result)
			fmt.Printf("Call echo: %s\n", result.Content[0].Text)
		}
	}
	// Output:
	// Initialize: ok
	// Tools: [echo]
	// Call echo: Hello, world!
}
