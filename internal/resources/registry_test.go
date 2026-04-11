package resources_test

import (
	"context"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/resources"
)

func TestRegister_With_ValidResource_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()

	// Act
	err := resources.Register(r, "config://app", "App Config", "Application configuration",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "config data"), nil
		},
		resources.WithMimeType("application/json"),
	)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "resource count", len(r.Resources()), 1)
	assert.That(t, "resource name", r.Resources()[0].Name, "App Config")
	assert.That(t, "resource uri", r.Resources()[0].URI, "config://app")
	assert.That(t, "resource mime", r.Resources()[0].MimeType, "application/json")
}

func TestRegister_With_DuplicateURI_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	handler := func(_ context.Context, uri string) (resources.Result, error) {
		return resources.TextResult(uri, "data"), nil
	}
	_ = resources.Register(r, "config://app", "Config", "desc", handler)

	// Act
	err := resources.Register(r, "config://app", "Config2", "desc2", handler)

	// Assert
	if err == nil {
		t.Fatal("expected error for duplicate URI")
	}
}

func TestLookup_With_RegisteredURI_Should_ReturnResource(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	_ = resources.Register(r, "config://app", "Config", "desc",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "data"), nil
		},
	)

	// Act
	res, ok := r.Lookup("config://app")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", res.Name, "Config")
}

func TestLookup_With_UnknownURI_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()

	// Act
	_, ok := r.Lookup("unknown://uri")

	// Assert
	assert.That(t, "found", ok, false)
}

func TestRegisterTemplate_With_ValidTemplate_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()

	// Act
	err := resources.RegisterTemplate(r, "file://{path}", "File", "Read a file",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "file content"), nil
		},
	)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "template count", len(r.Templates()), 1)
	assert.That(t, "template name", r.Templates()[0].Name, "File")
}

func TestResources_Should_ReturnSortedByURI(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	handler := func(_ context.Context, uri string) (resources.Result, error) {
		return resources.TextResult(uri, "data"), nil
	}
	_ = resources.Register(r, "z://last", "Z", "desc", handler)
	_ = resources.Register(r, "a://first", "A", "desc", handler)
	_ = resources.Register(r, "m://middle", "M", "desc", handler)

	// Act
	all := r.Resources()

	// Assert
	assert.That(t, "count", len(all), 3)
	assert.That(t, "first", all[0].URI, "a://first")
	assert.That(t, "second", all[1].URI, "m://middle")
	assert.That(t, "third", all[2].URI, "z://last")
}

func TestTextResult_Should_ReturnCorrectContent(t *testing.T) {
	t.Parallel()

	// Act
	result := resources.TextResult("config://app", "hello")

	// Assert
	assert.That(t, "content count", len(result.Contents), 1)
	assert.That(t, "uri", result.Contents[0].URI, "config://app")
	assert.That(t, "text", result.Contents[0].Text, "hello")
}
