package v03run

import (
	"os"
	"strings"
	"testing"
)

// The product refuses one Vault token for both the connection and the output adapter; the runner must pass
// two distinct files, require the output token, and scan it.
func TestRunnerPassesDistinctVaultTokensToTheTwoAdapters(t *testing.T) {
	raw, err := os.ReadFile("../v03-internal-frontend.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, want := range []string{
		`-connection-vault-token-file "${PRIVATE}/vault-token"`,
		`-output-secret-vault-token-file "${PRIVATE}/output-vault-token"`,
		"ORCH_V03_OUTPUT_VAULT_TOKEN_FILE",
		`cmp -s "${PRIVATE}/vault-token" "${PRIVATE}/output-vault-token"`,
		`"${PRIVATE}/output-vault-token" "${PG_ADMIN}"`, // in the secret scan list
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("runner contract: missing %q", want)
		}
	}
	if strings.Contains(script, `-output-secret-vault-token-file "${PRIVATE}/vault-token"`) {
		t.Fatal("the output adapter must not receive the connection token")
	}
}
