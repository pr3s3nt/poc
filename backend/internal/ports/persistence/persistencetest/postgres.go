package persistencetest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// FreshPostgresDatabase creates an empty database on the local test server
// named by ORCHESTRATOR_POSTGRES_TEST_URL and drops it after the test. The
// test is skipped when no local server is configured.
func FreshPostgresDatabase(t *testing.T) string {
	t.Helper()
	base := os.Getenv("ORCHESTRATOR_POSTGRES_TEST_URL")
	if base == "" {
		t.Skip("ORCHESTRATOR_POSTGRES_TEST_URL is not set")
	}
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
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
		admin.Close(context.Background())
	})
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}
