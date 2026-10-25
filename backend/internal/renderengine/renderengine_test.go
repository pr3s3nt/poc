package renderengine

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/renderbundle"
)

func TestMain(m *testing.M) {
	RunWorkerIfRequested()
	os.Exit(m.Run())
}

func execute(t *testing.T, source string, data map[string]any) (string, error) {
	t.Helper()
	return Executor{}.Execute(context.Background(), "t.tpl", source, data)
}

func TestHelpersAreDeterministicAndKeepExplicitValues(t *testing.T) {
	data := map[string]any{"In": map[string]any{"off": false, "zero": 0.0, "empty": "", "nil": nil, "obj": map[string]any{"a": 1.0, "b": map[string]any{"c": 1.0}}}}
	out, err := execute(t, `{{ default "d" .In.off }}|{{ default "d" .In.zero }}|{{ default "d" .In.empty }}|{{ default "d" .In.nil }}|{{ default "d" (get .In "absent") }}|`+
		`{{ toJson (merge .In.obj (dict "b" (dict "d" 2) "e" (list 1 2))) }}|{{ toJson .In.obj }}|{{ escapeScore "a$b$$c" }}|{{ toJson (keys .In.obj) }}|{{ quote "x\"y" }}|{{ toJson (sortAlpha (list "b" "a")) }}`, data)
	if err != nil {
		t.Fatal(err)
	}
	want := `false|0||d|d|{"a":1,"b":{"c":1,"d":2},"e":[1,2]}|{"a":1,"b":{"c":1}}|a$$b$$$$c|["a","b"]|"x\"y"|["a","b"]`
	if out != want {
		t.Fatalf("got  %s\nwant %s", out, want)
	}
	for i := 0; i < 3; i++ {
		again, _ := execute(t, `{{ toJson (merge .In.obj (dict "b" (dict "d" 2) "e" (list 1 2))) }}`, data)
		if again != `{"a":1,"b":{"c":1,"d":2},"e":[1,2]}` {
			t.Fatalf("unstable output %s", again)
		}
	}
}

func TestTemplateFailuresAreSafeAndClassified(t *testing.T) {
	cases := map[string]struct{ source, code string }{
		"missing key":      {`{{ .In.absent.deeper }}`, CodeTemplateExecute},
		"fail":             {`{{ fail "secret-sentinel" }}`, CodeTemplateExecute},
		"required nil":     {`{{ required "need" .In.nil }}`, CodeTemplateExecute},
		"unknown function": {`{{ env "HOME" }}`, CodeTemplateParse},
		"no file include":  {`{{ template "other.tpl" }}`, CodeTemplateExecute},
	}
	for name, c := range cases {
		_, err := execute(t, c.source, map[string]any{"In": map[string]any{"nil": nil}})
		var tmpl *TemplateError
		if !errors.As(err, &tmpl) || tmpl.Code != c.code || tmpl.File != "t.tpl" {
			t.Fatalf("%s: got %#v", name, err)
		}
		if strings.Contains(err.Error(), "secret-sentinel") {
			t.Fatalf("%s: error leaks template data", name)
		}
	}
	if _, err := execute(t, `{{ required "x" .In.zero }}{{ required "x" .In.off }}`, map[string]any{"In": map[string]any{"zero": 0.0, "off": false}}); err != nil {
		t.Fatalf("required must accept explicit zero/false: %v", err)
	}
}

// A runaway template is killed and reaped at the deadline; no goroutine is
// left executing it. The loops emit no output on purpose: the test exercises
// runaway CPU, so the output cap cannot end the run before the deadline.
func TestWorkerIsKilledAtDeadline(t *testing.T) {
	items := make([]any, 5000)
	for i := range items {
		items[i] = i
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Executor{}.Execute(ctx, "loop.tpl", `{{ range .L }}{{ range $.L }}{{ range $.L }}{{ end }}{{ end }}{{ end }}`, map[string]any{"L": items})
	var tmpl *TemplateError
	if !errors.As(err, &tmpl) || tmpl.Code != CodeTemplateTimeout {
		t.Fatalf("want timeout, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("worker was not killed promptly: %v", time.Since(start))
	}
}

func TestPatchSemantics(t *testing.T) {
	manifests := []map[string]any{{"kind": "Deployment", "metadata": map[string]any{"name": "a", "annotations": map[string]any{"x": "1"}}, "spec": map[string]any{"replicas": 1.0}}}
	ops, err := ParsePatchOutput("p.tpl", `
- op: set
  path: 0.spec.replicas
  value: 3
- op: set
  path: 0.metadata.labels.example\.com/team
  value: payments
- op: delete
  path: 0.metadata.annotations
- op: set
  path: 0.spec.template.spec.containers.0.name
  value: main
`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatches("p.tpl", manifests, ops)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"kind": "Deployment", "metadata": map[string]any{"name": "a", "labels": map[string]any{"example.com/team": "payments"}}, "spec": map[string]any{"replicas": 3.0, "template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "main"}}}}}}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("got %#v", got[0])
	}
	if manifests[0]["spec"].(map[string]any)["replicas"] != 1.0 {
		t.Fatal("input manifests were changed")
	}
	if empty, err := ParsePatchOutput("p.tpl", "\n  \n"); err != nil || empty != nil {
		t.Fatal("empty output must be a no-op")
	}
	for name, output := range map[string]string{
		"root insert":    "- op: set\n  path: \"1\"\n  value: {}\n",
		"root append":    "- op: set\n  path: \"-1.kind\"\n  value: X\n",
		"root delete":    "- op: delete\n  path: \"0\"\n",
		"unknown op":     "- op: add\n  path: 0.spec\n",
		"unknown field":  "- op: set\n  path: 0.spec\n  from: x\n",
		"not a list":     "op: set\n",
		"index past end": "- op: set\n  path: 3.spec.replicas\n  value: 1\n",
	} {
		ops, err := ParsePatchOutput("p.tpl", output)
		if err == nil {
			_, err = ApplyPatches("p.tpl", manifests, ops)
		}
		if !errors.Is(err, ErrPatch) {
			t.Fatalf("%s: want patch error, got %v", name, err)
		}
	}
	many := strings.Repeat("- op: set\n  path: 0.spec.replicas\n  value: 1\n", MaxPatchOperations+1)
	if _, err := ParsePatchOutput("p.tpl", many); !errors.Is(err, ErrPatch) {
		t.Fatal("too many operations accepted")
	}
}

func validTree() map[string][]byte {
	return map[string][]byte{
		"bundle.yaml": []byte(`apiVersion: orchestrator.dev/render-bundle/v1
kind: RenderBundle
metadata:
  name: t
renderer:
  type: score-k8s
  version: 0.15.0
inputContract: orchestrator.dev/render-input/v1
inputsSchema: inputs.schema.json
defaults: defaults.yaml
normalize: n.tpl
provisioners: [p/a.provisioners.yaml]
patches: [x.tpl]
`),
		"inputs.schema.json":    []byte(`{"type":"object","additionalProperties":false,"properties":{}}`),
		"defaults.yaml":         []byte("a: 1\n"),
		"n.tpl":                 []byte(`{{ toJson (dict) }}`),
		"x.tpl":                 []byte(``),
		"p/a.provisioners.yaml": []byte("- uri: template://x/y\n  type: t\n  outputs: |\n    a: {{ .Params.b | quote }}\n"),
		"examples/README.md":    []byte("doc"),
	}
}

func TestValidateBundleTree(t *testing.T) {
	bundle, err := Validate(validTree())
	if err != nil || bundle.Contract.Defaults["a"] != 1.0 {
		t.Fatalf("valid tree: %v", err)
	}
	mutate := map[string]func(map[string][]byte){
		"missing listed file": func(f map[string][]byte) { delete(f, "x.tpl") },
		"cmd provisioner": func(f map[string][]byte) {
			f["p/a.provisioners.yaml"] = []byte("- uri: cmd://sh\n  type: t\n  outputs: x\n")
		},
		"state provisioner": func(f map[string][]byte) {
			f["p/a.provisioners.yaml"] = []byte("- uri: template://x\n  type: t\n  state: x\n  outputs: x\n")
		},
		"manifests": func(f map[string][]byte) {
			f["p/a.provisioners.yaml"] = []byte("- uri: template://x\n  type: t\n  manifests: x\n  outputs: x\n")
		},
		"bad provisioner name": func(f map[string][]byte) {
			f["p/b.yaml"] = f["p/a.provisioners.yaml"]
			f["bundle.yaml"] = []byte(strings.Replace(string(f["bundle.yaml"]), "p/a.provisioners.yaml", "p/b.yaml", 1))
		},
		"unknown helper": func(f map[string][]byte) { f["n.tpl"] = []byte(`{{ exec "ls" }}`) },
		"duplicate json key": func(f map[string][]byte) {
			f["inputs.schema.json"] = []byte(`{"type":"object","type":"object","additionalProperties":false}`)
		},
		"defaults not object": func(f map[string][]byte) { f["defaults.yaml"] = []byte("- a\n") },
		"too many files": func(f map[string][]byte) {
			for i := 0; i < renderbundle.MaxFiles; i++ {
				f["examples/"+strings.Repeat("x", 1)+string(rune('a'+i%26))+string(rune('a'+i/26))] = []byte("x")
			}
		},
		"file too large": func(f map[string][]byte) { f["examples/big"] = make([]byte, renderbundle.MaxFileBytes+1) },
		"unsafe path":    func(f map[string][]byte) { f["../x"] = []byte("x") },
	}
	for name, change := range mutate {
		tree := validTree()
		change(tree)
		if _, err := Validate(tree); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
