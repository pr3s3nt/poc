package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/seed"
)

// The Terraform state backend never starts without PostgreSQL and a valid
// external key, and is absent (not keyless) when no key is configured.
func TestTerraformStateBackendFailsClosed(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("ab", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{Seed: seed.Defaults(), TerraformStateKeyFile: keyFile, TerraformStateKeyID: "key-1"}
	if _, err := Build(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "requires PostgreSQL") {
		t.Fatalf("in-memory store accepted for durable state: %v", err)
	}
	app, err := Build(context.Background(), Options{Seed: seed.Defaults()})
	if err != nil {
		t.Fatal(err)
	}
	if app.TerraformStates != nil || app.TerraformStateHandler != nil {
		t.Fatal("state backend enabled without a key")
	}
}

func writeKey(t *testing.T) string {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("cd", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	return keyFile
}

// Option A wiring: the Console store uses the Console login and the state
// backend its own State login to the same installation. Every other
// combination fails closed, nothing falls back to another login, and every
// pool opened by a failed Build is closed again.
func TestTerraformStateBackendUsesSeparateStateLogin(t *testing.T) {
	ctx := context.Background()
	db := postgrestest.Fresh(t)
	other := postgrestest.Fresh(t)
	key := writeKey(t)
	base := func() Options {
		return Options{Seed: seed.Defaults(), DatabaseURL: db.Console, TerraformStateDatabaseURL: db.State, TerraformStateKeyFile: key, TerraformStateKeyID: "key-1"}
	}
	app, err := Build(ctx, base())
	if err != nil {
		t.Fatalf("build with Console and State logins: %v", err)
	}
	if app.TerraformStates == nil || app.TerraformStateHandler == nil {
		t.Fatal("state backend not built")
	}
	app.Close()
	app.Close() // idempotent

	for label, change := range map[string]func(*Options){
		"key without State login":   func(o *Options) { o.TerraformStateDatabaseURL = "" },
		"State login without key":   func(o *Options) { o.TerraformStateKeyFile = "" },
		"State URL equals Console":  func(o *Options) { o.TerraformStateDatabaseURL = db.Console },
		"owner as State login":      func(o *Options) { o.TerraformStateDatabaseURL = db.Owner },
		"admin as State login":      func(o *Options) { o.TerraformStateDatabaseURL = db.Admin },
		"owner as Console login":    func(o *Options) { o.DatabaseURL = db.Owner },
		"State as Console login":    func(o *Options) { o.DatabaseURL = db.State; o.TerraformStateDatabaseURL = db.Console },
		"State login of another DB": func(o *Options) { o.TerraformStateDatabaseURL = other.State },
		"missing key id":            func(o *Options) { o.TerraformStateKeyID = "" },
	} {
		opts := base()
		change(&opts)
		if app, err := Build(ctx, opts); err == nil {
			app.Close()
			t.Errorf("%s: built", label)
		}
	}
	// No pool of a failed Build is left connected (each fresh database is
	// only reachable through its own logins).
	var leaked int
	if err := connectAdmin(t, db.Admin).QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid()`).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("%d connections left open after Build failures and Close: %v", leaked, err)
	}
}

func connectAdmin(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

// Credential files must be private regular files; a state key file can never
// double as a database credential file.
func TestCredentialFilesAreOwnerOnlyAndDistinct(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good", "postgres://u:secret-pw@h/db\n", 0o600)
	if v, err := ReadCredentialFile("database URL", good); err != nil || v != "postgres://u:secret-pw@h/db" {
		t.Fatalf("owner-only file: %q %v", v, err)
	}
	readOnly := write("ro", "x", 0o400)
	if _, err := ReadCredentialFile("database URL", readOnly); err != nil {
		t.Fatalf("0400 file: %v", err)
	}
	for label, path := range map[string]string{
		"group readable": write("group", "postgres://u:secret-pw@h/db", 0o640),
		"world readable": write("world", "postgres://u:secret-pw@h/db", 0o644),
		"empty":          write("empty", "  \n", 0o600),
		"missing":        filepath.Join(dir, "missing"),
		"directory":      dir,
		"unset":          "",
	} {
		_, err := ReadCredentialFile("database URL", path)
		if err == nil {
			t.Errorf("%s accepted", label)
		} else if strings.Contains(err.Error(), "secret-pw") {
			t.Errorf("%s error echoes the credential", label)
		}
	}
	link := filepath.Join(dir, "link")
	if err := os.Link(good, link); err != nil {
		t.Fatal(err)
	}
	if err := DistinctFiles(map[string]string{"database URL": good, "Terraform state key": link}); err == nil {
		t.Fatal("the same file served as key and credential")
	}
	if err := DistinctFiles(map[string]string{"database URL": good, "State database URL": readOnly, "Terraform state key": ""}); err != nil {
		t.Fatal(err)
	}
}
