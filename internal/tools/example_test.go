package tools_test

import (
	"context"
	"fmt"

	"github.com/andygeiss/mcp/internal/tools"
)

// ExampleRegister shows the full tool-author workflow: define an input struct,
// register a handler, then verify it appears in the registry.
func ExampleRegister() {
	type GreetInput struct {
		Name string `json:"name" description:"Who to greet"`
	}

	r := tools.NewRegistry()
	if err := tools.Register(r, "greet", "Says hello", func(_ context.Context, input GreetInput) tools.Result {
		return tools.TextResult("Hello, " + input.Name + "!")
	}); err != nil {
		fmt.Println("error:", err)
		return
	}

	t, ok := r.Lookup("greet")
	fmt.Println("found:", ok)
	fmt.Println("name:", t.Name)
	fmt.Println("schema type:", t.InputSchema.Type)

	all := r.Tools()
	fmt.Println("total tools:", len(all))
	// Output:
	// found: true
	// name: greet
	// schema type: object
	// total tools: 1
}
