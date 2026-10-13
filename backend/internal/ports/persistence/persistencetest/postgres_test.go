package persistencetest

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const childModeEnv = "PERSISTENCETEST_POSTGRES_CHILD"

// TestPostgresTestEnvironmentChild runs only as a subprocess of
// TestPostgresTestEnvironmentModes; it opens a fresh database and runs SQL as
// the Console login.
func TestPostgresTestEnvironmentChild(t *testing.T) {
	if os.Getenv(childModeEnv) != "1" {
		return
	}
	db := FreshPostgresRoles(t)
	conn, err := pgx.Connect(context.Background(), db.Console)
	if err != nil {
		t.Fatal("connect as the Console login failed")
	}
	defer conn.Close(context.Background())
	var user string
	if err := conn.QueryRow(context.Background(), `SELECT current_user`).Scan(&user); err != nil || user != db.ConsoleRole {
		t.Fatalf("SQL as the Console login: user=%q err=%v", user, err)
	}
}

func runChild(t *testing.T, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPostgresTestEnvironmentChild$", "-test.v", "-test.count=1")
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name != PostgresTestURLEnv && name != RequirePostgresEnv && name != childModeEnv {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, append([]string{childModeEnv + "=1"}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestPostgresTestEnvironmentModes: without a test server URL the PostgreSQL
// tests skip in the optional local mode and fail (nonzero exit) under
// REQUIRE_POSTGRES=1. With a URL they run real SQL, and no output reveals it.
func TestPostgresTestEnvironmentModes(t *testing.T) {
	out, err := runChild(t, RequirePostgresEnv+"=1")
	if err == nil || !strings.Contains(out, "--- FAIL: TestPostgresTestEnvironmentChild") || !strings.Contains(out, RequirePostgresEnv+"=1 requires") {
		t.Fatalf("mandatory mode without a URL must fail: err=%v\n%s", err, out)
	}
	if code := exitCode(err); code == 0 {
		t.Fatalf("mandatory mode exit code %d", code)
	}
	for _, env := range [][]string{nil, {RequirePostgresEnv + "=0"}, {RequirePostgresEnv + "="}} {
		out, err := runChild(t, env...)
		if err != nil || !strings.Contains(out, "--- SKIP: TestPostgresTestEnvironmentChild") {
			t.Fatalf("optional mode %v must skip: err=%v\n%s", env, err, out)
		}
	}

	base := PostgresTestURL(t)
	out, err = runChild(t, PostgresTestURLEnv+"="+base, RequirePostgresEnv+"=1")
	if err != nil || !strings.Contains(out, "--- PASS: TestPostgresTestEnvironmentChild") {
		t.Fatalf("configured mandatory mode must run SQL: err=%v\n%s", err, redact(out, base))
	}
	if strings.Contains(out, base) {
		t.Fatal("child output contains the test server URL")
	}
	if u, perr := url.Parse(base); perr == nil {
		if pw, ok := u.User.Password(); ok && pw != "" && strings.Contains(out, pw) {
			t.Fatal("child output contains the test server password")
		}
	}
}

func exitCode(err error) int {
	if e, ok := err.(*exec.ExitError); ok {
		return e.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

func redact(out, base string) string {
	out = strings.ReplaceAll(out, base, "<redacted>")
	if u, err := url.Parse(base); err == nil {
		if pw, ok := u.User.Password(); ok && pw != "" {
			out = strings.ReplaceAll(out, pw, "<redacted>")
		}
	}
	return out
}
