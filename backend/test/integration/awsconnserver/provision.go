package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/bootstrap"
)

// manifest names exactly what "provision" created so "drop" removes only it.
type manifest struct {
	Database string   `json:"database"`
	Roles    []string `json:"roles"`
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// provision creates one fresh database with an owner and two runtime login
// roles on the DISPOSABLE server named by -admin-url-file (an administrator
// URL), runs the owner setup and writes the Console login URL to
// <out-dir>/console.url (0600) plus manifest.json. Passwords are random,
// never printed. The server must be one the task owns.
func provision(args []string) int {
	fs := flag.NewFlagSet("provision", flag.ExitOnError)
	adminFile := fs.String("admin-url-file", "", "owner-only file with the administrator URL of the disposable PostgreSQL server")
	outDir := fs.String("out-dir", "", "existing 0700 directory for console.url and manifest.json")
	_ = fs.Parse(args)
	admin, err := bootstrap.ReadCredentialFile("administrator URL", *adminFile)
	if err != nil || *outDir == "" {
		fmt.Fprintln(os.Stderr, "awsconnserver provision: -admin-url-file and -out-dir are required")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "awsconnserver provision: cannot connect to the administrator URL")
		return 1
	}
	defer conn.Close(ctx)
	suffix, err := randomHex(6)
	if err != nil {
		return 1
	}
	m := manifest{Database: "i0406_" + suffix}
	roles := map[string]string{}
	for _, kind := range []string{"owner", "console", "state"} {
		role := "i0406_" + suffix + "_" + kind
		password, err := randomHex(16)
		if err != nil {
			return 1
		}
		if _, err := conn.Exec(ctx, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '`+password+`'`); err != nil {
			fmt.Fprintln(os.Stderr, "awsconnserver provision: create role failed")
			return 1
		}
		m.Roles = append(m.Roles, role)
		roles[kind] = password
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{m.Database}.Sanitize()+` OWNER `+pgx.Identifier{m.Roles[0]}.Sanitize()); err != nil {
		fmt.Fprintln(os.Stderr, "awsconnserver provision: create database failed")
		return 1
	}
	parsed, err := url.Parse(admin)
	if err != nil {
		return 1
	}
	parsed.Path = "/" + m.Database
	as := func(role, password string) string {
		u := *parsed
		u.User = url.UserPassword(role, password)
		return u.String()
	}
	if _, err := postgres.Setup(ctx, as(m.Roles[0], roles["owner"]), postgres.Roles{Console: m.Roles[1], State: m.Roles[2]}); err != nil {
		fmt.Fprintln(os.Stderr, "awsconnserver provision: owner setup failed")
		return 1
	}
	if err := os.WriteFile(filepath.Join(*outDir, "console.url"), []byte(as(m.Roles[1], roles["console"])), 0o600); err != nil {
		return 1
	}
	encoded, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(*outDir, "manifest.json"), encoded, 0o600); err != nil {
		return 1
	}
	fmt.Printf("provisioned database %s with %d roles\n", m.Database, len(m.Roles))
	return 0
}

// drop removes only the database and roles listed in the manifest.
func drop(args []string) int {
	fs := flag.NewFlagSet("drop", flag.ExitOnError)
	adminFile := fs.String("admin-url-file", "", "administrator URL file")
	manifestFile := fs.String("manifest", "", "manifest.json written by provision")
	_ = fs.Parse(args)
	admin, err := bootstrap.ReadCredentialFile("administrator URL", *adminFile)
	raw, readErr := os.ReadFile(*manifestFile)
	var m manifest
	if err != nil || readErr != nil || json.Unmarshal(raw, &m) != nil || !strings.HasPrefix(m.Database, "i0406_") {
		fmt.Fprintln(os.Stderr, "awsconnserver drop: a valid -admin-url-file and -manifest are required")
		return 2
	}
	for _, role := range m.Roles {
		if !strings.HasPrefix(role, "i0406_") {
			fmt.Fprintln(os.Stderr, "awsconnserver drop: refusing a role that was not created by provision")
			return 2
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "awsconnserver drop: cannot connect to the administrator URL")
		return 1
	}
	defer conn.Close(ctx)
	status := 0
	if _, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{m.Database}.Sanitize()+` WITH (FORCE)`); err != nil {
		fmt.Fprintln(os.Stderr, "awsconnserver drop: drop database failed")
		status = 1
	}
	for _, role := range m.Roles {
		if _, err := conn.Exec(ctx, `DROP ROLE IF EXISTS `+pgx.Identifier{role}.Sanitize()); err != nil {
			fmt.Fprintln(os.Stderr, "awsconnserver drop: drop role failed")
			status = 1
		}
	}
	if status == 0 {
		fmt.Printf("dropped database %s and %d roles\n", m.Database, len(m.Roles))
	}
	return status
}
