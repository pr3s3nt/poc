package placeholder

import "testing"

type testResolver struct{}

func (testResolver) ResolveResource(binding, key string) (any, error) {
	if binding == "shared.db" && key == "port" {
		return float64(5432), nil
	}
	if binding == "shared.db" && key == "host" {
		return "db.internal", nil
	}
	return nil, errUnknown
}

func (testResolver) ResolveContext(path string) (any, error) {
	if path == "env.namespace" {
		return "acceptance-dev", nil
	}
	return nil, errUnknown
}

var errUnknown = errString("unknown reference")

type errString string

func (e errString) Error() string { return string(e) }

func TestSinglePlaceholderKeepsType(t *testing.T) {
	v, err := ExpandString("${shared.db.port}", testResolver{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if v != float64(5432) {
		t.Fatalf("expected typed number, got %#v", v)
	}
}

func TestInterpolationStringifies(t *testing.T) {
	v, err := ExpandString("postgres://${shared.db.host}:${shared.db.port}/app", testResolver{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if v != "postgres://db.internal:5432/app" {
		t.Fatalf("unexpected interpolation: %#v", v)
	}
}

func TestEscapedPlaceholderStaysLiteral(t *testing.T) {
	v, err := ExpandString("literal $${shared.db.host} value", testResolver{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if v != "literal ${shared.db.host} value" {
		t.Fatalf("escaped placeholder was resolved: %#v", v)
	}
}

func TestContextPlaceholder(t *testing.T) {
	v, err := ExpandString("${context.env.namespace}", testResolver{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if v != "acceptance-dev" {
		t.Fatalf("unexpected context value: %#v", v)
	}
}

func TestResourceReferenceForms(t *testing.T) {
	quoted, err := Parse("resources['postgres.default#shared.acceptance-db'].outputs.host")
	if err != nil {
		t.Fatalf("parse quoted reference: %v", err)
	}
	if quoted.Resource != "postgres.default#shared.acceptance-db" || quoted.OutputKey != "host" {
		t.Fatalf("unexpected quoted ref: %#v", quoted)
	}

	short, err := Parse("resources.vpc.outputs.id")
	if err != nil {
		t.Fatalf("parse short reference: %v", err)
	}
	if short.Resource != "vpc" || short.OutputKey != "id" {
		t.Fatalf("unexpected short ref: %#v", short)
	}

	if _, err := Parse("resources['postgres.default#x']"); err == nil {
		t.Fatal("a reference without .outputs.<name> must be rejected")
	}
	if !LooksLikeResourceReference("resources.vpc") {
		t.Fatal("a malformed resource reference must be detectable")
	}
}

func TestSetPlaceholderForms(t *testing.T) {
	private, err := Parse("externals.cache.host")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if private.Resource != "externals.cache" || private.OutputKey != "host" {
		t.Fatalf("unexpected ref: %#v", private)
	}
	if _, err := Parse("shared.db.host.extra"); err == nil {
		t.Fatal("more segments than <scope>.<name>.<output> must be rejected")
	}
	if _, err := Parse("shared."); err == nil {
		t.Fatal("a scope without a name must be rejected")
	}
}
