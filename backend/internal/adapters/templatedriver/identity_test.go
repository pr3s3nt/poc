package templatedriver

import (
	"runtime/debug"
	"testing"
)

// ADR-041: the renderer identity follows the Go toolchain and the renderer
// libraries only, so an unrelated rebuild keeps template pins valid.
func TestRendererIdentityIgnoresUnrelatedRebuilds(t *testing.T) {
	deps := func(yaml, other string) []*debug.Module {
		return []*debug.Module{
			{Path: "gopkg.in/yaml.v3", Version: yaml, Sum: "h1:y"},
			{Path: "github.com/tidwall/sjson", Version: "v1.2.5", Sum: "h1:s"},
			{Path: "github.com/jackc/pgx/v5", Version: other, Sum: "h1:p"},
		}
	}
	base := identityFrom("go1.25.3", deps("v3.0.1", "v5.7.1"))
	if base == "" || base != identityFrom("go1.25.3", deps("v3.0.1", "v5.7.1")) {
		t.Fatal("identity is not deterministic")
	}
	if identityFrom("go1.25.3", deps("v3.0.1", "v5.9.0")) != base {
		t.Fatal("an unrelated dependency changed the renderer identity")
	}
	if identityFrom("go1.25.4", deps("v3.0.1", "v5.7.1")) == base {
		t.Fatal("a Go toolchain change must change the renderer identity")
	}
	if identityFrom("go1.25.3", deps("v3.0.2", "v5.7.1")) == base {
		t.Fatal("a renderer library change must change the renderer identity")
	}
	replaced := deps("v3.0.1", "v5.7.1")
	replaced[0].Replace = &debug.Module{Path: "example.com/fork/yaml", Version: "v3.0.1", Sum: "h1:f"}
	if identityFrom("go1.25.3", replaced) == base {
		t.Fatal("a replaced renderer library must change the renderer identity")
	}
}

func TestRendererBinaryDigestIsAvailable(t *testing.T) {
	r := New()
	if err := r.CheckBinary(); err != nil {
		t.Fatal(err)
	}
	if d := r.BinaryDigest(); len(d) != 64 || d != r.BinaryDigest() {
		t.Fatal("renderer identity unavailable or unstable", d)
	}
}
