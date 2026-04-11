package resources_test

import (
	"context"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/resources"
)

func Test_Register_With_ValidResource_Should_Succeed(t *testing.T) {
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

func Test_Register_With_DuplicateURI_Should_ReturnError(t *testing.T) {
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

func Test_Lookup_With_RegisteredURI_Should_ReturnResource(t *testing.T) {
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

func Test_Lookup_With_UnknownURI_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()

	// Act
	_, ok := r.Lookup("unknown://uri")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_RegisterTemplate_With_ValidTemplate_Should_Succeed(t *testing.T) {
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

func Test_Resources_Should_ReturnSortedByURI(t *testing.T) {
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

func Test_TextResult_Should_ReturnCorrectContent(t *testing.T) {
	t.Parallel()

	// Act
	result := resources.TextResult("config://app", "hello")

	// Assert
	assert.That(t, "content count", len(result.Contents), 1)
	assert.That(t, "uri", result.Contents[0].URI, "config://app")
	assert.That(t, "text", result.Contents[0].Text, "hello")
}

func Test_BlobResult_Should_ReturnCorrectContent(t *testing.T) {
	t.Parallel()

	// Act
	result := resources.BlobResult("file://img.png", "aGVsbG8=", "image/png")

	// Assert
	assert.That(t, "content count", len(result.Contents), 1)
	assert.That(t, "uri", result.Contents[0].URI, "file://img.png")
	assert.That(t, "blob", result.Contents[0].Blob, "aGVsbG8=")
	assert.That(t, "mime", result.Contents[0].MimeType, "image/png")
}

func Test_RegisterTemplate_With_MimeType_Should_ApplyOption(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()

	// Act
	err := resources.RegisterTemplate(r, "file://{path}", "File", "Read a file",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "content"), nil
		},
		resources.WithMimeType("text/plain"),
	)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "template count", len(r.Templates()), 1)
	assert.That(t, "mime type", r.Templates()[0].MimeType, "text/plain")
}

func Test_LookupTemplate_With_MatchingURI_Should_ReturnTemplate(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "file://{path}", "File", "Read a file",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "content of "+uri), nil
		},
	)

	// Act
	tmpl, ok := r.LookupTemplate("file://readme.md")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", tmpl.Name, "File")
}

func Test_LookupTemplate_With_NonMatchingURI_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "file://{path}", "File", "Read a file",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "data"), nil
		},
	)

	// Act
	_, ok := r.LookupTemplate("config://app")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_MultipleVariables_Should_Match(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "db://{schema}/{table}", "DB Table", "Read a DB table",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "rows"), nil
		},
	)

	// Act
	tmpl, ok := r.LookupTemplate("db://public/users")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", tmpl.Name, "DB Table")
}

func Test_LookupTemplate_With_EmptyVariable_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "file://{path}", "File", "Read a file",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "data"), nil
		},
	)

	// Act — "file://" has no content after the prefix, so {path} would be empty
	_, ok := r.LookupTemplate("file://")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_NoTemplates_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()

	// Act
	_, ok := r.LookupTemplate("file://anything")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_UnclosedBrace_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange — register a template with an unclosed brace (malformed pattern)
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "file://{path", "File", "Read a file",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "data"), nil
		},
	)

	// Act
	_, ok := r.LookupTemplate("file://readme.md")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_LiteralOnly_Should_MatchExactly(t *testing.T) {
	t.Parallel()

	// Arrange — template has no variables, acts as exact match
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "static://exact", "Static", "Exact match",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "static"), nil
		},
	)

	// Act
	tmpl, ok := r.LookupTemplate("static://exact")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", tmpl.Name, "Static")
}

func Test_LookupTemplate_With_LiteralOnlyMismatch_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "static://exact", "Static", "Exact match",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "static"), nil
		},
	)

	// Act
	_, ok := r.LookupTemplate("static://other")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_AdjacentVariables_Should_Match(t *testing.T) {
	t.Parallel()

	// Arrange — two adjacent variables with no literal separator
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "{scheme}{path}", "Adjacent", "Adjacent vars",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "adj"), nil
		},
	)

	// Act — minimum: each variable matches at least one character
	tmpl, ok := r.LookupTemplate("ab")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", tmpl.Name, "Adjacent")
}

func Test_LookupTemplate_With_AdjacentVariablesSingleChar_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange — two adjacent variables need at least 2 characters total
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "{a}{b}", "Adjacent", "Adjacent vars",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "adj"), nil
		},
	)

	// Act — only 1 char, cannot satisfy both variables
	_, ok := r.LookupTemplate("x")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_LiteralPrefixMismatch_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "prefix://{id}", "Pre", "Prefix match",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "data"), nil
		},
	)

	// Act — URI doesn't start with the literal prefix
	_, ok := r.LookupTemplate("other://123")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_TrailingSuffix_Should_Match(t *testing.T) {
	t.Parallel()

	// Arrange — template: prefix/{var}/suffix
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "api/{version}/docs", "API", "API docs",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "docs"), nil
		},
	)

	// Act
	tmpl, ok := r.LookupTemplate("api/v2/docs")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", tmpl.Name, "API")
}

func Test_LookupTemplate_With_TrailingSuffixMismatch_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange — anchor after variable doesn't match
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "api/{version}/docs", "API", "API docs",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "docs"), nil
		},
	)

	// Act — suffix "docs" not present
	_, ok := r.LookupTemplate("api/v2/other")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_ThreeAdjacentVariablesTwoChars_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange — three adjacent variables need at least 3 characters
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "{a}{b}{c}", "Triple", "Triple vars",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "triple"), nil
		},
	)

	// Act — only 2 chars, cannot satisfy 3 variables each needing 1+ char
	_, ok := r.LookupTemplate("xy")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_LookupTemplate_With_ThreeAdjacentVariablesThreeChars_Should_Match(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "{a}{b}{c}", "Triple", "Triple vars",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "triple"), nil
		},
	)

	// Act — exactly 3 chars satisfies 3 adjacent variables
	tmpl, ok := r.LookupTemplate("xyz")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", tmpl.Name, "Triple")
}

func Test_LookupTemplate_With_VariableAnchorNotFound_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange — variable followed by literal anchor that URI doesn't contain
	r := resources.NewRegistry()
	_ = resources.RegisterTemplate(r, "{host}:8080", "Host", "Host with port",
		func(_ context.Context, uri string) (resources.Result, error) {
			return resources.TextResult(uri, "host"), nil
		},
	)

	// Act — URI has no ":8080" anchor
	_, ok := r.LookupTemplate("localhost")

	// Assert
	assert.That(t, "found", ok, false)
}

func Test_Templates_Should_ReturnSortedByURITemplate(t *testing.T) {
	t.Parallel()

	// Arrange
	r := resources.NewRegistry()
	handler := func(_ context.Context, uri string) (resources.Result, error) {
		return resources.TextResult(uri, "data"), nil
	}
	_ = resources.RegisterTemplate(r, "z://{id}", "Z", "desc", handler)
	_ = resources.RegisterTemplate(r, "a://{id}", "A", "desc", handler)

	// Act
	all := r.Templates()

	// Assert
	assert.That(t, "count", len(all), 2)
	assert.That(t, "first", all[0].URITemplate, "a://{id}")
	assert.That(t, "second", all[1].URITemplate, "z://{id}")
}
