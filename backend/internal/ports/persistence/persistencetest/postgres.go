package persistencetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// PostgresDatabase is one disposable database on the local test server named
// by ORCHESTRATOR_POSTGRES_TEST_URL, provisioned the way a database
// administrator provisions a fresh option-A database (ADR-014): an owner
// role that owns the empty database, and two separate ordinary LOGIN roles
// with their own passwords for the Console and State runtime. Nothing is
// granted yet; the owner setup does that. Every URL logs in as its own role
// with a password (no SET ROLE).
type PostgresDatabase struct {
	// Admin is the test server's administrator connected to this database.
	// Tests use it only to arrange or inspect fixtures, never as a runtime.
	Admin   string
	Owner   string
	Console string
	State   string

	OwnerRole, ConsoleRole, StateRole string
	Name                              string
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// PostgresTestURLEnv names the administrator URL of the local test server.
const PostgresTestURLEnv = "ORCHESTRATOR_POSTGRES_TEST_URL"

// RequirePostgresEnv set to "1" makes PostgreSQL tests mandatory: a missing
// test server URL fails the test instead of skipping it, so a validation run
// cannot pass with the SQL suites silently skipped.
const RequirePostgresEnv = "REQUIRE_POSTGRES"

// PostgresTestURL returns the test server URL. Without one the test is
// skipped, or fails when REQUIRE_POSTGRES=1. The URL is never logged.
func PostgresTestURL(t testing.TB) string {
	t.Helper()
	base := os.Getenv(PostgresTestURLEnv)
	if base != "" {
		return base
	}
	if os.Getenv(RequirePostgresEnv) == "1" {
		t.Fatalf("%s is not set but %s=1 requires the PostgreSQL tests", PostgresTestURLEnv, RequirePostgresEnv)
	}
	t.Skipf("%s is not set", PostgresTestURLEnv)
	return ""
}

// FreshPostgresRoles creates an empty database owned by a new owner role and
// two new runtime login roles, and drops all of them after the test. The test
// is skipped when no local server is configured, unless REQUIRE_POSTGRES=1.
func FreshPostgresRoles(t *testing.T) PostgresDatabase {
	t.Helper()
	base := PostgresTestURL(t)
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect test server: %v", err)
	}
	name := fmt.Sprintf("uc09_%s_%d", strings.ToLower(strings.NewReplacer("/", "_", "-", "_").Replace(t.Name())), time.Now().UnixNano())
	if len(name) > 60 {
		name = name[len(name)-60:]
		name = "t" + name[1:]
	}
	prefix := "t" + randomHex(t, 6)
	db := PostgresDatabase{Name: name, OwnerRole: prefix + "_owner", ConsoleRole: prefix + "_console", StateRole: prefix + "_state"}
	passwords := map[string]string{}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
		for _, role := range []string{db.ConsoleRole, db.StateRole, db.OwnerRole} {
			_, _ = admin.Exec(ctx, `DROP ROLE IF EXISTS `+pgx.Identifier{role}.Sanitize())
		}
		admin.Close(ctx)
	})
	for _, role := range []string{db.OwnerRole, db.ConsoleRole, db.StateRole} {
		passwords[role] = randomHex(t, 16)
		// The password is hex, so it needs no quoting beyond the literal.
		if _, err := admin.Exec(ctx, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '`+passwords[role]+`'`); err != nil {
			t.Fatalf("create role: %v", err)
		}
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()+` OWNER `+pgx.Identifier{db.OwnerRole}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	parsed.Path = "/" + name
	db.Admin = parsed.String()
	as := func(role string) string {
		u := *parsed
		u.User = url.UserPassword(role, passwords[role])
		return u.String()
	}
	db.Owner, db.Console, db.State = as(db.OwnerRole), as(db.ConsoleRole), as(db.StateRole)
	return db
}
