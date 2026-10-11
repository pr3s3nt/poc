package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(body), 0o600)
	return p
}

const base = `{"repo":"o/r","repo_path":"/r","state_dir":"/s","worktree_root":"/w","owner_logins":["me"]`

func TestLoadAppliesDefaults(t *testing.T) {
	c, err := Load(write(t, base+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.PollSeconds != 240 || c.Coordinator.Command != "codex" || strings.Join(c.Coordinator.Args, " ") != "--yolo --json" ||
		c.Coder.Command != "clauded" || c.Cloud.Mode != "off" || c.ReadyLabel != "agent:ready" || c.Evidence.Publisher != "release" {
		t.Fatalf("%+v", c)
	}
	if len(c.Coder.TrustUnnumberedKeys) != 0 {
		t.Fatal("unnumbered trust must fail closed by default")
	}
}

func TestValidateRejects(t *testing.T) {
	for name, body := range map[string]string{
		"no owner":           `{"repo":"o/r","repo_path":"/r","state_dir":"/s","worktree_root":"/w"}`,
		"bad repo":           `{"repo":"nope","repo_path":"/r","state_dir":"/s","worktree_root":"/w","owner_logins":["a"]}`,
		"relative path":      `{"repo":"o/r","repo_path":"r","state_dir":"/s","worktree_root":"/w","owner_logins":["a"]}`,
		"unknown adapter":    base + `,"coder":{"adapter":"opencode"}}`,
		"cloud as primary":   base + `,"coder":{"adapter":"claude-cloud"}}`,
		"bad fallback":       base + `,"coder":{"fallback":["opencode"]}}`,
		"unknown cloud mode": base + `,"cloud":{"mode":"unattended"}}`,
		"bad regex":          base + `,"coder":{"ready_regex":"("}}`,
		"bad publisher":      base + `,"evidence":{"publisher":"s3"}}`,
		"enter first":        base + `,"coder":{"trust_unnumbered_keys":["Enter"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(write(t, body)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestOwnerMatchIsCaseInsensitive(t *testing.T) {
	c, _ := Load(write(t, base+`}`))
	if !c.IsOwner("ME") || c.IsOwner("someone") || c.IsOwner("") {
		t.Fatal("owner matching")
	}
}

func TestAutoCloudIsValidButOffByDefault(t *testing.T) {
	c, err := Load(write(t, base+`}`))
	if err != nil || c.Cloud.Mode != "off" {
		t.Fatalf("%v %+v", err, c.Cloud)
	}
	c, err = Load(write(t, base+`,"cloud":{"mode":"auto"},"coder":{"fallback":["claude-cloud"]}}`))
	if err != nil || c.Cloud.Mode != "auto" || c.Cloud.ReportMaxBytes != 65536 || c.Cloud.AmbiguousSendSeconds != 3600 {
		t.Fatalf("%v %+v", err, c.Cloud)
	}
}

func TestAssistedCloudAndGenericAdapterAreAccepted(t *testing.T) {
	if _, err := Load(write(t, base+`,"cloud":{"mode":"assisted"},"coder":{"adapter":"tmux-generic","command":"mytool","fallback":["claude-cloud"]}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestExampleConfigLoads(t *testing.T) {
	c, err := Load("../../config/issueflow.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.Cloud.Mode != "off" || len(c.Coder.Fallback) != 0 {
		t.Fatal("the example must keep the cloud off")
	}
}
