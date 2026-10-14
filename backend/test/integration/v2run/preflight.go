package v2run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Pinned tool versions of the acceptance.
const (
	PinnedTerraformVersion = "1.9.8"
	PinnedScoreK8sVersion  = "0.15.0"
)

// Check is one preflight verdict (names and verdicts only, never values).
type Check struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Note string `json:"note,omitempty"`
}

// FileSHA256 hashes a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyBinaries checks every configured binary against its pinned SHA-256 and
// proves the pinned Terraform and score-k8s versions by running them.
func VerifyBinaries(ctx context.Context, c Config) []Check {
	var out []Check
	for _, name := range sortedKeys(c.Binaries) {
		b := c.Binaries[name]
		sum, err := FileSHA256(b.Path)
		out = append(out, Check{Name: "sha256:" + name, OK: err == nil && strings.EqualFold(sum, b.SHA256)})
	}
	run := func(path string, args ...string) string {
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Env = baseEnv()
		raw, _ := cmd.Output()
		return string(raw)
	}
	if b, ok := c.Binaries["terraform"]; ok {
		var v struct {
			Version string `json:"terraform_version"`
		}
		_ = json.Unmarshal([]byte(run(b.Path, "version", "-json")), &v)
		out = append(out, Check{Name: "version:terraform", OK: v.Version == PinnedTerraformVersion, Note: v.Version})
	}
	if b, ok := c.Binaries["score-k8s"]; ok {
		first := strings.SplitN(run(b.Path, "--version"), "\n", 2)[0]
		out = append(out, Check{Name: "version:score-k8s", OK: strings.HasPrefix(first, "score-k8s "+PinnedScoreK8sVersion+" ("), Note: first})
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// PreflightK8s runs the read-only k8s-4f checks before any effect: the context
// is the real kind context, a Ready node of the expected architecture exists,
// the run namespace and every other orch-v2-test-<run>* namespace are absent,
// the outside-run sentinel has the recorded UID, a storage class exists and the
// operator may create namespaces (a permission check, no creation).
func PreflightK8s(ctx context.Context, k Kubectl, c Config) []Check {
	var out []Check
	add := func(name string, ok bool, note string) { out = append(out, Check{Name: name, OK: ok, Note: note}) }
	raw, stderr, exit := k.Run(ctx, "get", "nodes", "-o", "json")
	var nodes struct {
		Items []struct {
			Status struct {
				Conditions []struct{ Type, Status string } `json:"conditions"`
				NodeInfo   struct {
					Architecture string `json:"architecture"`
				} `json:"nodeInfo"`
			} `json:"status"`
		} `json:"items"`
	}
	if exit != 0 || json.Unmarshal(raw, &nodes) != nil {
		add("cluster-reachable", false, firstLine(stderr))
		return out
	}
	add("cluster-reachable", true, "")
	ready := 0
	archOK := true
	for _, n := range nodes.Items {
		isReady := false
		for _, cond := range n.Status.Conditions {
			isReady = isReady || (cond.Type == "Ready" && cond.Status == "True")
		}
		if isReady {
			ready++
		}
		archOK = archOK && n.Status.NodeInfo.Architecture == c.NodeArch
	}
	add("ready-node", ready >= 1, fmt.Sprintf("%d ready", ready))
	add("node-architecture", archOK && len(nodes.Items) > 0, c.NodeArch)
	nsOut, _, nsExit := k.Run(ctx, "get", "namespaces", "-o", "name")
	clash := false
	for _, line := range strings.Fields(string(nsOut)) {
		if strings.HasPrefix(strings.TrimPrefix(line, "namespace/"), NamespacePrefix+c.RunID) {
			clash = true
		}
	}
	add("run-namespace-absent", nsExit == 0 && !clash, "")
	add("sentinel-uid", VerifySentinel(ctx, k, *c.Kube) == nil, "")
	scOut, _, scExit := k.Run(ctx, "get", "storageclass", "-o", "name")
	add("storage-class", scExit == 0 && strings.TrimSpace(string(scOut)) != "", "")
	can, _, canExit := k.Run(ctx, "auth", "can-i", "create", "namespaces")
	add("can-create-namespace", canExit == 0 && strings.TrimSpace(string(can)) == "yes", "")
	return out
}

// AllOK reports whether every check passed.
func AllOK(checks []Check) bool {
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return len(checks) > 0
}
