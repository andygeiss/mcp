package server_test

import (
	"bytes"
	"context"
	"fmt"

	"github.com/andygeiss/mcp/internal/server"
	"github.com/andygeiss/mcp/internal/tools"
)

// ExampleNewServer demonstrates the wiring pattern: create a registry,
// register tools, then construct a server with injected I/O. In production
// code stdin/stdout/stderr are os.Stdin, os.Stdout, os.Stderr; in tests they
// are bytes.Buffer for full control.
func ExampleNewServer() {
	// Build the tool registry.
	r := tools.NewRegistry()
	tools.Register(r, "search", "Searches files for a pattern", tools.Search)

	// Inject buffers instead of real file descriptors.
	var stdout, stderr bytes.Buffer
	stdin := bytes.NewBufferString("") // empty = immediate EOF

	srv := server.NewServer("myserver", "1.0.0", r, stdin, &stdout, &stderr)

	// Run processes messages until EOF (returns nil for clean shutdown).
	err := srv.Run(context.Background())
	fmt.Println("error:", err)
	// Output:
	// error: <nil>
}
