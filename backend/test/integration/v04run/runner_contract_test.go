package v04run

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The runner contract: what the shell script may and may not do. These are static checks of the real script.
func TestRunnerScriptContract(t *testing.T) {
	script := read(t, "../v04-internal-full.sh")
	for _, want := range []string{
		"set -euo pipefail",
		`-connection-vault-token-file "${PRIVATE}/vault-token"`,
		`-output-secret-vault-token-file "${PRIVATE}/output-vault-token"`,
		`cmp -s "${PRIVATE}/vault-token" "${PRIVATE}/output-vault-token"`,
		"ORCH_V04_OUTPUT_VAULT_TOKEN_FILE",
		`"${PRIVATE}/app-secrets.txt"`, `"${PRIVATE}/store-root-header"`, `"${PRIVATE}/secret-store.token"`, // in the secret scan list
		"aux-secret-vault.pid",                  // the supervisor stops the run-owned Vault
		"SS_START_TIME",                         // exact Linux process identity is retained through cleanup
		"setsid nohup \"${SUPERVISOR}\"",        // external supervisor armed before any mutation
		`v04-internal-full.mjs" --scenario V04`, // the scenario of THIS gate
		"--verify-evidence",
		"-vault-delivery vso",
		"-terraform-invocation-timeout", "-terraform-readiness-timeout", "-deploy-deadline", // finite deadlines
		"v03-cleanup.sh", "v03-supervisor.sh", // the proven UID-guarded cleanup is reused, not copied
		"ORCH_V04_DEADLINE_SECONDS",
		"ORCH_V04_DEADLINE_SECONDS:-3600", // canonical local deadline is at most one hour by default
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("runner contract: missing %q", want)
		}
	}
	for _, forbidden := range []string{"ORCH_V03_", "kubectl delete", "docker ", "aws ", "AWS_PROFILE", "helm "} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("runner contract: the script must not contain %q", forbidden)
		}
	}
	if regexp.MustCompile(`(^|[^A-Z_])KUBECONFIG=`).MatchString(script) {
		t.Fatal("the script must not export a KUBECONFIG")
	}
	if strings.Contains(script, `-output-secret-vault-token-file "${PRIVATE}/vault-token"`) {
		t.Fatal("the output adapter must not receive the connection token")
	}
	// The product starts with a cleared environment: no host kubeconfig, credential chain or token reaches it.
	if !strings.Contains(script, `ISOLATED=(env -i "PATH=${PATH}"`) {
		t.Fatal("the product must start through env -i")
	}
}

func TestEveryOperatorCommandTheRunnerUsesExists(t *testing.T) {
	registered := map[string]bool{}
	for _, file := range []string{"../v2ctl/main.go", "../v2ctl/extra.go", "../v2ctl/v03.go", "../v2ctl/v04.go"} {
		for _, m := range regexp.MustCompile(`extra\["([a-z0-9-]+)"\]|"([a-z0-9-]+)": [a-zA-Z]+[,}]`).FindAllStringSubmatch(read(t, file), -1) {
			if m[1] != "" {
				registered[m[1]] = true
			}
			if m[2] != "" {
				registered[m[2]] = true
			}
		}
	}
	script := read(t, "../v04-internal-full.sh") + read(t, "../v03-cleanup.sh") + read(t, "../../../../frontend/test/e2e/v04-internal-full.mjs") + read(t, "../../../../frontend/test/e2e/full-app-flows.mjs")
	for _, m := range regexp.MustCompile(`"\$\{CTL\}" ([a-z0-9]+-[a-z0-9-]+)|v2ctl\(input, '([a-z0-9-]+)'`).FindAllStringSubmatch(script, -1) {
		name := m[1] + m[2]
		if !registered[name] {
			t.Errorf("the runner or scenario calls v2ctl %s, which is not registered", name)
		}
	}
}

func TestRunnerScriptIsSyntacticallyValidBash(t *testing.T) {
	if out, err := exec.Command("bash", "-n", "../v04-internal-full.sh").CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}
