package scorek8s

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"orchestrator/internal/renderengine"
)

// The platform patch stage must apply set/delete exactly like score-k8s
// 0.15.0 applies its own --patch-templates (same sjson semantics).
func TestPatchSemanticsMatchScoreK8s(t *testing.T) {
	binary, err := exec.LookPath("score-k8s")
	if err != nil {
		t.Skip("local score-k8s 0.15.0 unavailable")
	}
	ops := `- op: set
  path: 1.spec.replicas
  value: 3
- op: set
  path: 1.metadata.labels.example\.com/team
  value: payments
- op: delete
  path: 1.metadata.annotations
- op: set
  path: 1.spec.template.spec.containers.0.securityContext
  value: {"allowPrivilegeEscalation": false}
- op: delete
  path: 0.metadata.annotations
- op: set
  path: 0.spec.type
  value: ClusterIP
`
	score := []byte(`{"apiVersion":"score.dev/v1b1","metadata":{"name":"web"},"containers":{"main":{"image":"nginx:1"}},"service":{"ports":{"http":{"port":80}}}}`)
	run := func(withPatch bool) []map[string]any {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "score.json"), score, 0o600)
		_ = os.WriteFile(filepath.Join(dir, "p.tpl"), []byte(ops), 0o600)
		args := []string{"init", "--quiet", "--no-sample", "--no-default-provisioners"}
		if withPatch {
			args = append(args, "--patch-templates", filepath.Join(dir, "p.tpl"))
		}
		for _, a := range [][]string{args, {"generate", "--quiet", "score.json", "--namespace", "n", "--output", "m.yaml"}} {
			cmd := exec.Command(binary, a...)
			cmd.Dir, cmd.Env = dir, []string{"HOME=" + dir, "PATH=" + filepath.Dir(binary)}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
		}
		raw, _ := os.ReadFile(filepath.Join(dir, "m.yaml"))
		var out []map[string]any
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		for {
			var m map[string]any
			if dec.Decode(&m) != nil {
				break
			}
			tree, _ := toTree(m)
			out = append(out, tree)
		}
		return out
	}
	plain, patched := run(false), run(true)
	parsed, err := renderengine.ParsePatchOutput("p.tpl", ops)
	if err != nil {
		t.Fatal(err)
	}
	ours, err := renderengine.ApplyPatches("p.tpl", plain, parsed)
	if err != nil {
		t.Fatal(err)
	}
	// The app.kubernetes.io/instance suffix is random per workspace.
	instance := func(ms []map[string]any) string {
		return ms[1]["metadata"].(map[string]any)["labels"].(map[string]any)["app.kubernetes.io/instance"].(string)
	}
	a, _ := json.Marshal(ours)
	normalized := []map[string]any{}
	_ = json.Unmarshal([]byte(strings.ReplaceAll(string(a), instance(ours), instance(patched))), &normalized)
	if !reflect.DeepEqual(normalized, patched) {
		t.Fatalf("platform patch differs from score-k8s\nours: %v\nscore-k8s: %v", ours, patched)
	}
}
