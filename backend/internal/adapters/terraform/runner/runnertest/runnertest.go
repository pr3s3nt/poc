// Package runnertest builds verified private artifacts for tests of the
// Terraform runner and executor: a hand-written module tree and either the
// real host Terraform CLI or a stand-in script, digested exactly like the
// registry does. It bypasses registry inspection on purpose so fixtures can
// use the built-in terraform_data resource (no provider download, no side
// effects); the registry path itself is covered by registry tests.
package runnertest

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/tfmodule"
)

// HostTerraform returns the host Terraform CLI path or skips the test. Tests
// that call it exercise the actual CLI and say so.
func HostTerraform(t testing.TB) string {
	t.Helper()
	path, err := exec.LookPath("terraform")
	if err != nil {
		if os.Getenv("REQUIRE_TERRAFORM") == "1" {
			t.Fatalf("terraform CLI is required but not found")
		}
		t.Skip("terraform CLI not found")
	}
	return path
}

// Module is a fixture module: files of the root module directory and the
// directories of called modules, relative to the source root.
type Module struct {
	Files map[string]string // root module files
	// Children are called module directories relative to the source root,
	// e.g. "common" for source = "../common"; the root lives in "root".
	Children map[string]map[string]string
	// Calls maps a module call key to the child directory name.
	Calls map[string]string
}

// Artifact materializes the module and binary in a fresh private directory
// and returns the artifact. cli is the executable copied as the Terraform
// binary (real CLI path or a script).
func Artifact(t testing.TB, m Module, cli string) *tfmodule.VerifiedArtifact {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "bin", "terraform")
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatal(err)
	}
	sum := copyFile(t, cli, binary, 0o500)
	src := filepath.Join(root, "sources", "test@rev1")
	write := func(dir string, files map[string]string) {
		for name, content := range files {
			p := filepath.Join(src, dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	write("root", m.Files)
	contract := tfmodule.Contract{SchemaVersion: tfmodule.ContractSchemaVersion, SourceKind: tfmodule.SourceKindPackaged, Inputs: map[string]tfmodule.Input{}, Outputs: map[string]tfmodule.Output{}}
	art := &tfmodule.VerifiedArtifact{Root: root, SourceDir: root, ModuleDir: filepath.Join(src, "root"), TerraformBinary: binary, LockFile: []byte("# fixture lock: builtin providers only\n")}
	// Retain records the private handle -> workspace mapping next to the
	// workspace (never in public metadata), like the registry does.
	art.Retain = func(handle string) error {
		return os.WriteFile(filepath.Join(filepath.Dir(root), ".retained-"+handle), []byte(root), 0o600)
	}
	names := make([]string, 0, len(m.Calls))
	for key := range m.Calls {
		names = append(names, key)
	}
	for _, key := range sortedStrings(names) {
		dir := m.Calls[key]
		write(dir, m.Children[dir])
		digest, err := registry.DigestDir(filepath.Join(src, dir))
		if err != nil {
			t.Fatal(err)
		}
		contract.Transitive = append(contract.Transitive, tfmodule.TransitiveModule{SourceID: "test", Revision: "rev1", Path: dir, ContentSHA256: digest})
		contract.ModuleCalls = append(contract.ModuleCalls, tfmodule.ModuleCall{Key: key, Source: "../" + dir, Transitive: len(contract.Transitive) - 1})
		art.Modules = append(art.Modules, tfmodule.ModuleInstall{Key: key, Source: "../" + dir, Dir: filepath.Join(src, dir)})
	}
	rootDigest, err := registry.DigestDir(art.ModuleDir)
	if err != nil {
		t.Fatal(err)
	}
	art.Bundle = tfmodule.Bundle{
		Ref:                   tfmodule.SourceRef{SourceID: "test", ModuleID: "fixture", Revision: "rev1", Subdirectory: "root", ContentSHA256: rootDigest},
		TerraformBinarySHA256: sum, TerraformVersion: "1.9.8", ProviderLockDigest: sum, Contract: contract, Status: tfmodule.StatusVerified,
		Providers: map[string]tfmodule.ProviderRequirement{},
	}
	return art
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func copyFile(t testing.TB, src, dst string, mode os.FileMode) string {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dst, mode); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Script writes an executable stand-in "terraform" script and returns its path.
func Script(t testing.TB, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "terraform")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}
