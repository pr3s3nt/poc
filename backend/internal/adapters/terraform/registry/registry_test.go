package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
)

const (
	repoURL  = "https://git.example.com/acme/modules.git"
	childURL = "https://git.example.com/acme/shared.git"
)

var (
	commitA = strings.Repeat("a", 40)
	commitB = strings.Repeat("b", 40)
)

const rootModule = `terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

variable "name" {
  type = string
}

variable "size" {
  type    = number
  default = 1
}

variable "region" {
  type = string
}

resource "aws_s3_bucket" "b" {
  bucket = var.name
}

output "host" {
  value = aws_s3_bucket.b.bucket_domain_name
}

output "port" {
  value = 6379
}

output "password" {
  value     = "generated"
  sensitive = true
}
`

// memSnapshot is one source revision held in memory.
type memSnapshot struct{ files map[string]string }

func (m memSnapshot) Close() {}

func (m memSnapshot) Tree(dir string) (Tree, error) {
	out := Tree{}
	total := 0
	for p, content := range m.files {
		rel := p
		if dir != "." {
			if !strings.HasPrefix(p, dir+"/") {
				continue
			}
			rel = strings.TrimPrefix(p, dir+"/")
		}
		if err := out.add(rel, []byte(content), &total); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// fakeGit serves repositories by URL and commit and counts every open.
type fakeGit struct {
	repos map[string]map[string]map[string]string
	opens int
}

func (f *fakeGit) Open(_ context.Context, source GitSource, commit string) (Snapshot, error) {
	f.opens++
	files, ok := f.repos[source.URL][commit]
	if !ok {
		return nil, ErrFetch
	}
	return memSnapshot{files: files}, nil
}

func testPolicy() Policy {
	return Policy{Sources: []GitSource{
		{ID: "acme", Kind: tfmodule.SourceKindGit, URL: repoURL, Modules: []GitModule{{ID: "redis", Subdirectory: "modules/redis", ResourceTypes: []string{"redis"}}}},
		{ID: "shared", Kind: tfmodule.SourceKindGit, URL: childURL, Modules: []GitModule{{ID: "net", Subdirectory: "net", ResourceTypes: []string{"network"}}}},
	}}
}

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

type fixture struct {
	svc     *Service
	git     *fakeGit
	store   *store.Store
	cache   *Cache
	release Release
}

func newFixture(t *testing.T, repo map[string]string) fixture {
	t.Helper()
	return newFixtureWithBinary(t, repo, "")
}

// newFixtureWithBinary pins the fixture release to a stand-in Terraform CLI
// file when binary content is given, so execution verification runs without
// the real CLI; the digest check itself is identical.
func newFixtureWithBinary(t *testing.T, repo map[string]string, binary string) fixture {
	t.Helper()
	release, err := DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	binaryPath := ""
	if binary != "" {
		binaryPath = filepath.Join(t.TempDir(), "terraform")
		if err := os.WriteFile(binaryPath, []byte(binary), 0o700); err != nil {
			t.Fatal(err)
		}
		sum, _ := fileSHA256(binaryPath)
		release.Terraform.BinarySHA256 = sum
	}
	git := &fakeGit{repos: map[string]map[string]map[string]string{repoURL: {commitA: repo}}}
	cache, err := OpenCache(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New()
	svc, err := New(Config{Release: release, Policy: testPolicy(), Packaged: terraform.PackagedModules(), Git: git, Cache: cache, Store: st, TerraformBinary: binaryPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	return fixture{svc: svc, git: git, store: st, cache: cache, release: release}
}

func refFor(repo map[string]string) tfmodule.SourceRef {
	digest := memTree(repo, "modules/redis").Digest()
	return tfmodule.SourceRef{SourceID: "acme", ModuleID: "redis", Revision: commitA, Subdirectory: "modules/redis", ContentSHA256: digest}
}

func memTree(files map[string]string, dir string) Tree {
	tree, err := memSnapshot{files: files}.Tree(dir)
	if err != nil {
		panic(err)
	}
	return tree
}

func baseRepo() map[string]string {
	return map[string]string{"modules/redis/main.tf": rootModule, "README.md": "outside the module"}
}

// The release pins the packaged module digests computed from the shipped
// files; preparation of every packaged module succeeds.
func TestReleasePackagedDigestsMatchShippedModules(t *testing.T) {
	release, err := DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	snap := packagedSnapshot{fsys: terraform.PackagedModules()}
	for _, m := range release.Packaged.Modules {
		tree, err := snap.Tree(m.Subdirectory)
		if err != nil {
			t.Fatal(err)
		}
		if got := tree.Digest(); got != m.ContentSHA256 {
			t.Fatalf("packaged %s digest %s, release pins %s", m.ID, got, m.ContentSHA256)
		}
	}
	refs, err := PackagedRefs()
	if err != nil || len(refs) != len(release.Packaged.Modules) {
		t.Fatalf("packaged refs: %v %v", refs, err)
	}
}

// The tree digest is the canonical algorithm: sorted "<path>\0<hex sha256>\n".
func TestTreeDigestAlgorithm(t *testing.T) {
	tree := Tree{"b.tf": []byte("2"), "a/x.tf": []byte("1")}
	h := sha256.New()
	for _, p := range []string{"a/x.tf", "b.tf"} {
		sum := sha256.Sum256(tree[p])
		h.Write([]byte(p + "\x00" + hex.EncodeToString(sum[:]) + "\n"))
	}
	if tree.Digest() != hex.EncodeToString(h.Sum(nil)) {
		t.Fatal("digest does not follow the documented algorithm")
	}
	renamed := Tree{"c.tf": []byte("2"), "a/x.tf": []byte("1")}
	if renamed.Digest() == tree.Digest() {
		t.Fatal("digest must cover file names")
	}
}

func TestUnsafeFileNamesAreRejected(t *testing.T) {
	for _, p := range []string{"../x.tf", "/abs.tf", "a/../b.tf", "a//b.tf", "main.tf.json", "override.tf", "x_override.tf", ".terraformrc", "x.tfrc", ".terraform.lock.hcl", ".terraform/modules/m.json", ".git/config", "bad\x01name.tf"} {
		if err := checkPath(p); !errors.Is(err, ErrUnsafeContent) || !errors.Is(err, moduleregistry.ErrRejected) {
			t.Errorf("%q: %v", p, err)
		}
	}
}

func TestReadTreeRejectsSymlinksAndExecutables(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, setup := range map[string]func(dir string) error{
		"escaping symlink": func(dir string) error {
			return os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "link.tf"))
		},
		"inner symlink":   func(dir string) error { return os.Symlink("main.tf", filepath.Join(dir, "alias.tf")) },
		"executable hook": func(dir string) error { return os.WriteFile(filepath.Join(dir, "hook.sh"), []byte("#!/bin/sh"), 0o700) },
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(rootModule), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := setup(dir); err != nil {
			t.Fatal(err)
		}
		if _, err := readTree(dir); !errors.Is(err, ErrUnsafeContent) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func inspectRoot(t *testing.T, f fixture, files map[string]string) (inspection, error) {
	t.Helper()
	ref := tfmodule.SourceRef{SourceID: "acme", ModuleID: "redis", Revision: commitA, Subdirectory: "modules/redis", ContentSHA256: strings.Repeat("0", 64)}
	r := f.svc.resolver(context.Background(), ref, memSnapshot{files: files})
	defer r.close()
	return inspect(r, rootDir(ref), f.release, tfmodule.SourceKindGit, []string{"redis"}, nil)
}

// Static inspection refuses every construct that could run code, move state,
// override provider bindings, read outside the module or pull unpinned code.
func TestInspectorRejectsForbiddenConstructs(t *testing.T) {
	f := newFixture(t, baseRepo())
	withRoot := func(extra string) map[string]string {
		return map[string]string{"modules/redis/main.tf": rootModule, "modules/redis/extra.tf": extra}
	}
	full := strings.Repeat("c", 40)
	cases := map[string]map[string]string{
		"local-exec provisioner":   withRoot(`resource "aws_s3_bucket" "x" {` + "\n" + `provisioner "local-exec" {` + "\n" + `command = "id"` + "\n}\n}\n"),
		"remote-exec provisioner":  withRoot(`resource "aws_s3_bucket" "x" {` + "\n" + `provisioner "remote-exec" {` + "\n" + `inline = ["id"]` + "\n}\n}\n"),
		"terraform_data hook":      withRoot(`resource "terraform_data" "x" {` + "\n" + `provisioner "local-exec" {` + "\n" + `command = "id"` + "\n}\n}\n"),
		"connection block":         withRoot(`resource "aws_instance" "x" {` + "\n" + `connection {` + "\n" + `host = "h"` + "\n}\n}\n"),
		"backend block":            withRoot("terraform {\nbackend \"s3\" {}\n}\n"),
		"cloud block":              withRoot("terraform {\ncloud {}\n}\n"),
		"provider credentials":     withRoot("provider \"aws\" {\nalias = \"x\"\naccess_key = \"AKIA\"\n}\n"),
		"provider endpoint":        withRoot("provider \"aws\" {\nalias = \"y\"\nendpoints {\ns3 = \"http://evil\"\n}\n}\n"),
		"provider literal region":  withRoot("provider \"aws\" {\nalias = \"z\"\nregion = \"eu-west-1\"\n}\n"),
		"file traversal":           withRoot("locals {\nx = file(\"../../etc/passwd\")\n}\n"),
		"file via module escape":   withRoot("locals {\nx = file(\"${path.module}/../../x\")\n}\n"),
		"templatefile variable":    withRoot("locals {\nx = templatefile(var.name, {})\n}\n"),
		"abspath":                  withRoot("locals {\nx = abspath(\".\")\n}\n"),
		"path.cwd":                 withRoot("locals {\nx = path.cwd\n}\n"),
		"remote state":             withRoot("data \"terraform_remote_state\" \"x\" {\nbackend = \"local\"\n}\n"),
		"undeclared provider":      withRoot("data \"external\" \"x\" {\nprogram = [\"id\"]\n}\n"),
		"import block":             withRoot("import {\nto = aws_s3_bucket.b\nid = \"x\"\n}\n"),
		"registry child":           withRoot("module \"x\" {\nsource = \"terraform-aws-modules/vpc/aws\"\nversion = \"5.0.0\"\n}\n"),
		"http child":               withRoot("module \"x\" {\nsource = \"https://example.com/m.zip\"\n}\n"),
		"git ssh child":            withRoot("module \"x\" {\nsource = \"git::ssh://git@git.example.com/acme/shared.git?ref=" + full + "\"\n}\n"),
		"git branch child":         withRoot("module \"x\" {\nsource = \"git::" + childURL + "//net?ref=main\"\n}\n"),
		"git short sha child":      withRoot("module \"x\" {\nsource = \"git::" + childURL + "//net?ref=abc1234\"\n}\n"),
		"git without ref":          withRoot("module \"x\" {\nsource = \"git::" + childURL + "//net\"\n}\n"),
		"git extra query":          withRoot("module \"x\" {\nsource = \"git::" + childURL + "//net?ref=" + full + "&depth=1\"\n}\n"),
		"git credentials in url":   withRoot("module \"x\" {\nsource = \"git::https://user:pw@git.example.com/acme/shared.git//net?ref=" + full + "\"\n}\n"),
		"unallowlisted git host":   withRoot("module \"x\" {\nsource = \"git::https://evil.example.com/acme/shared.git//net?ref=" + full + "\"\n}\n"),
		"plain https git":          withRoot("module \"x\" {\nsource = \"" + childURL + "//net?ref=" + full + "\"\n}\n"),
		"local escape":             withRoot("module \"x\" {\nsource = \"../../../outside\"\n}\n"),
		"absolute local":           withRoot("module \"x\" {\nsource = \"/srv/modules/x\"\n}\n"),
		"module count":             withRoot("module \"x\" {\nsource = \"../shared\"\ncount = 2\n}\n"),
		"sensitive default":        withRoot("variable \"token\" {\ntype = string\nsensitive = true\ndefault = \"s3cr3t\"\n}\n"),
		"object type":              withRoot("variable \"o\" {\ntype = object({a = string})\n}\n"),
		"provider outside release": withRoot("terraform {\nrequired_providers {\nnull = {\nsource = \"hashicorp/null\"\n}\n}\n}\n"),
		"provider too new":         {"modules/redis/main.tf": strings.Replace(rootModule, ">= 5.0", ">= 6.0", 1)},
		"terraform too new":        {"modules/redis/main.tf": strings.Replace(rootModule, `required_version = ">= 1.5"`, `required_version = ">= 2.0"`, 1)},
		"json configuration":       {"modules/redis/main.tf": rootModule, "modules/redis/x.tf.json": "{}"},
		"override file":            {"modules/redis/main.tf": rootModule, "modules/redis/override.tf": ""},
	}
	for name, files := range cases {
		_, err := inspectRoot(t, f, files)
		if !errors.Is(err, moduleregistry.ErrRejected) {
			t.Errorf("%s: want rejection, got %v", name, err)
			continue
		}
		for _, leak := range []string{"AKIA", "s3cr3t", "/etc/passwd", "user:pw"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: error echoes %q", name, leak)
			}
		}
	}
	if f.git.opens != 0 {
		t.Fatalf("rejected sources were fetched: %d", f.git.opens)
	}
}

func TestInspectorReadsContract(t *testing.T) {
	f := newFixture(t, baseRepo())
	result, err := inspectRoot(t, f, baseRepo())
	if err != nil {
		t.Fatal(err)
	}
	c := result.contract
	if c.Inputs["name"].Required != true || c.Inputs["size"].Default != float64(1) || c.Inputs["size"].Type != "number" {
		t.Fatalf("inputs %+v", c.Inputs)
	}
	if c.Outputs["port"].Type != "number" || c.Outputs["host"].Type != "" || !c.Outputs["password"].Sensitive {
		t.Fatalf("outputs %+v", c.Outputs)
	}
	if !equal(c.ExecutorOwned, []string{"region"}) {
		t.Fatalf("executor owned %v", c.ExecutorOwned)
	}
	aws := result.providers["aws"]
	if aws.Version != "5.80.0" || aws.Binding != tfmodule.BindingAWS || len(aws.Hashes) == 0 || aws.Constraints != ">= 5.0" {
		t.Fatalf("provider %+v", aws)
	}
}

func equal(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, ",") == strings.Join(b, ",")
}

// Local children of the same revision and remote children at an allowlisted
// Git source and full commit are inspected, pinned by digest, cached and
// re-verified; nothing is fetched again at execution.
func TestTransitiveLocalAndRemoteChildrenArePinned(t *testing.T) {
	childFiles := map[string]string{
		"net/main.tf":         "variable \"cidr\" {\ntype = string\ndefault = \"10.0.0.0/16\"\n}\nmodule \"inner\" {\nsource = \"./inner\"\n}\noutput \"id\" {\nvalue = \"n\"\n}\n",
		"net/inner/main.tf":   "output \"x\" {\nvalue = 1\n}\n",
		"unrelated/README.md": "not pinned",
	}
	repo := baseRepo()
	repo["modules/redis/main.tf"] = rootModule + "\nmodule \"shared\" {\nsource = \"../shared\"\n}\nmodule \"net\" {\nsource = \"git::" + childURL + "//net?ref=" + commitB + "\"\n}\n"
	repo["modules/shared/main.tf"] = "output \"y\" {\nvalue = \"s\"\n}\n"
	f := newFixtureWithBinary(t, repo, "stand-in terraform")
	f.git.repos[childURL] = map[string]map[string]string{commitB: childFiles}
	ref := refFor(repo)

	bundle, err := f.svc.Verify(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]tfmodule.ModuleCall{}
	for _, call := range bundle.Contract.ModuleCalls {
		calls[call.Key] = call
	}
	if len(calls) != 3 || calls["net.inner"].Source != "./inner" || !strings.Contains(calls["net"].Source, commitB) {
		t.Fatalf("module calls %+v", bundle.Contract.ModuleCalls)
	}
	pins := map[string]tfmodule.TransitiveModule{}
	for _, m := range bundle.Contract.Transitive {
		pins[m.SourceID+"/"+m.Path] = m
	}
	if len(pins) != 3 || pins["shared/net"].Revision != commitB || pins["shared/net"].ContentSHA256 != memTree(childFiles, "net").Digest() ||
		pins["acme/modules/shared"].Revision != commitA || pins["shared/net/inner"].ContentSHA256 != memTree(childFiles, "net/inner").Digest() {
		t.Fatalf("transitive pins %+v", bundle.Contract.Transitive)
	}
	if _, err := f.store.InsertModuleBundle(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}

	opens := f.git.opens
	artifact, err := f.svc.VerifyForExecution(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if f.git.opens != opens {
		t.Fatal("execution verification fetched a source")
	}
	if len(artifact.Modules) != 3 {
		t.Fatalf("installs %+v", artifact.Modules)
	}
	for _, m := range artifact.Modules {
		if _, err := os.Stat(filepath.Join(m.Dir, "main.tf")); err != nil {
			t.Fatalf("module %s not materialized: %v", m.Key, err)
		}
	}

	// Drift of a remote child's shared cache content fails closed. The
	// private copy of the first invocation is not the shared cache.
	for _, m := range artifact.Modules {
		if m.Key == "net" {
			rel, err := filepath.Rel(artifact.Root, m.Dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(f.cache.bundleDir(ref), rel, "main.tf"), []byte("# drifted"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := artifact.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.VerifyForExecution(context.Background(), ref); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("drifted child: %v", err)
	}
	if f.git.opens != opens {
		t.Fatal("drift verification refetched")
	}
}

func TestTransitiveRemoteChildMustBeFetchableAtPinnedCommit(t *testing.T) {
	repo := baseRepo()
	repo["modules/redis/main.tf"] = rootModule + "\nmodule \"net\" {\nsource = \"git::" + childURL + "//net?ref=" + commitB + "\"\n}\n"
	f := newFixture(t, repo)
	// The child commit does not exist at the allowlisted source.
	if _, err := f.svc.Verify(context.Background(), refFor(repo)); !errors.Is(err, moduleregistry.ErrUnavailable) {
		t.Fatalf("missing child commit: %v", err)
	}
	repo["modules/redis/main.tf"] = rootModule + "\nmodule \"loop\" {\nsource = \"./loop\"\n}\n"
	repo["modules/redis/loop/main.tf"] = "module \"back\" {\nsource = \"../\"\n}\n"
	f = newFixture(t, repo)
	if _, err := f.svc.Verify(context.Background(), refFor(repo)); !errors.Is(err, moduleregistry.ErrRejected) {
		t.Fatalf("cyclic calls: %v", err)
	}
}

// Policy checks happen before any fetch: unallowlisted sources, modules,
// subdirectories and mutable revisions; digest mismatch is drift.
func TestVerifyPolicyAndDigest(t *testing.T) {
	repo := baseRepo()
	f := newFixture(t, repo)
	good := refFor(repo)
	cases := map[string]struct {
		mutate func(*tfmodule.SourceRef)
		want   error
	}{
		"unknown source":     {func(r *tfmodule.SourceRef) { r.SourceID = "evil" }, moduleregistry.ErrNotAllowed},
		"unknown module":     {func(r *tfmodule.SourceRef) { r.ModuleID = "other" }, moduleregistry.ErrNotAllowed},
		"other subdirectory": {func(r *tfmodule.SourceRef) { r.Subdirectory = "modules" }, moduleregistry.ErrNotAllowed},
		"branch":             {func(r *tfmodule.SourceRef) { r.Revision = "main" }, moduleregistry.ErrNotAllowed},
		"short sha":          {func(r *tfmodule.SourceRef) { r.Revision = "abc1234" }, moduleregistry.ErrNotAllowed},
		"tag":                {func(r *tfmodule.SourceRef) { r.Revision = "v1.0.0" }, moduleregistry.ErrNotAllowed},
		"wrong digest":       {func(r *tfmodule.SourceRef) { r.ContentSHA256 = strings.Repeat("9", 64) }, moduleregistry.ErrDrift},
		"packaged revision":  {func(r *tfmodule.SourceRef) { *r = mustPackaged(t, "vpc"); r.Revision = "v2" }, moduleregistry.ErrNotAllowed},
		"packaged digest":    {func(r *tfmodule.SourceRef) { *r = mustPackaged(t, "vpc"); r.ContentSHA256 = strings.Repeat("9", 64) }, moduleregistry.ErrDrift},
		"packaged unknown":   {func(r *tfmodule.SourceRef) { *r = mustPackaged(t, "vpc"); r.ModuleID = "redis" }, moduleregistry.ErrNotAllowed},
	}
	for name, c := range cases {
		ref := good
		c.mutate(&ref)
		before := f.git.opens
		if _, err := f.svc.Verify(context.Background(), ref); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", name, c.want, err)
		}
		if c.want == moduleregistry.ErrNotAllowed && f.git.opens != before {
			t.Errorf("%s: fetched before the policy check", name)
		}
	}
	bundle, err := f.svc.Verify(context.Background(), good)
	if err != nil || bundle.Ref != good || !bundle.ServesType("redis") || bundle.Contract.SourceKind != tfmodule.SourceKindGit {
		t.Fatalf("verify %+v %v", bundle, err)
	}
	if packaged, err := f.svc.Verify(context.Background(), mustPackaged(t, "aurora")); err != nil || !packaged.ServesType("postgres") {
		t.Fatalf("packaged verify %+v %v", packaged, err)
	}
}

func mustPackaged(t *testing.T, id string) tfmodule.SourceRef {
	t.Helper()
	refs, err := PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	return refs[id]
}

// A revision path already verified with one digest never accepts another
// (same-version drift), even when the new content itself is consistent.
func TestSameRevisionDriftIsRejected(t *testing.T) {
	repo := baseRepo()
	f := newFixture(t, repo)
	ref := refFor(repo)
	bundle, err := f.svc.Verify(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.InsertModuleBundle(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	// The source now serves other content for the same commit.
	changed := baseRepo()
	changed["modules/redis/main.tf"] = rootModule + "\n# changed\n"
	f.git.repos[repoURL][commitA] = changed
	drifted := refFor(changed)
	if _, err := f.svc.Verify(context.Background(), drifted); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("same-revision drift: %v", err)
	}
	// The original pin still verifies from the cache without a fetch.
	opens := f.git.opens
	if again, err := f.svc.Verify(context.Background(), ref); err != nil || again.ID != bundle.ID || f.git.opens != opens {
		t.Fatalf("re-verify: %v id=%s opens=%d->%d", err, again.ID, opens, f.git.opens)
	}
	// With the cache entry gone, registration refetches; the source content
	// changed under the same commit, so it is drift, not a silent update.
	if err := os.RemoveAll(f.cache.bundleDir(ref)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Verify(context.Background(), ref); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("refetched drift: %v", err)
	}
}

// Metadata reads never fetch, inspect or touch the cache.
func TestMetadataNeverFetches(t *testing.T) {
	repo := baseRepo()
	f := newFixture(t, repo)
	ref := refFor(repo)
	bundle, err := f.svc.Verify(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.InsertModuleBundle(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(f.cache.dir); err != nil {
		t.Fatal(err)
	}
	opens := f.git.opens
	if got, err := f.svc.Bundle(context.Background(), ref); err != nil || got.ID != bundle.ID {
		t.Fatalf("bundle: %v", err)
	}
	unknown := ref
	unknown.Revision = commitB
	if _, err := f.svc.Bundle(context.Background(), unknown); !errors.Is(err, moduleregistry.ErrNotVerified) {
		t.Fatalf("unverified pin: %v", err)
	}
	other := ref
	other.ContentSHA256 = strings.Repeat("9", 64)
	if _, err := f.svc.Bundle(context.Background(), other); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("other digest: %v", err)
	}
	list, err := f.svc.List(context.Background())
	if err != nil || len(list) != 7 {
		t.Fatalf("list: %d %v", len(list), err)
	}
	if f.git.opens != opens {
		t.Fatal("metadata fetched a source")
	}
}

// Callers cannot change verified bundles in place through any boundary.
func TestReturnedBundlesAreIsolated(t *testing.T) {
	f := newFixture(t, baseRepo())
	ref := mustPackaged(t, "aurora")
	for _, read := range []func() tfmodule.Bundle{
		func() tfmodule.Bundle { b, _ := f.svc.Bundle(context.Background(), ref); return b },
		func() tfmodule.Bundle { b, _ := f.svc.Verify(context.Background(), ref); return b },
		func() tfmodule.Bundle { b, _ := f.store.GetModuleBundle(context.Background(), ref); return b },
		func() tfmodule.Bundle {
			list, _ := f.store.ListModuleBundles(context.Background())
			for _, b := range list {
				if b.Ref == ref {
					return b
				}
			}
			return tfmodule.Bundle{}
		},
	} {
		before := read()
		mutated := read()
		mutated.Providers["aws"].Hashes[0] = "h1:tampered"
		mutated.Contract.Inputs["seconds_until_auto_pause"] = tfmodule.Input{Type: "string"}
		mutated.Contract.ResourceTypes[0] = "tampered"
		if after := read(); !after.SameVerifiedContent(before) {
			t.Fatalf("a caller changed the verified bundle in place")
		}
	}
}

// Execution verification fails closed without a configured binary, with a
// binary of another digest, on missing or drifted content and on a release
// whose provider pins changed (no implicit upgrade).
func TestVerifyForExecutionFailsClosed(t *testing.T) {
	ref := mustPackaged(t, "vpc")
	ctx := context.Background()
	unconfigured := newFixture(t, baseRepo())
	if _, err := unconfigured.svc.VerifyForExecution(ctx, ref); !errors.Is(err, moduleregistry.ErrBinaryUnverified) {
		t.Fatalf("no binary: %v", err)
	}
	f := newFixtureWithBinary(t, baseRepo(), "pinned terraform")
	pinned := f.svc.cfg.TerraformBinary
	other := filepath.Join(t.TempDir(), "terraform")
	if err := os.WriteFile(other, []byte("not terraform"), 0o700); err != nil {
		t.Fatal(err)
	}
	f.svc.cfg.TerraformBinary = other
	if _, err := f.svc.VerifyForExecution(ctx, ref); !errors.Is(err, moduleregistry.ErrReleaseMismatch) {
		t.Fatalf("wrong binary: %v", err)
	}
	f.svc.cfg.TerraformBinary = pinned
	artifact, err := f.svc.VerifyForExecution(ctx, ref)
	if err != nil || artifact.TerraformBinary == pinned || !strings.Contains(string(artifact.LockFile), `"registry.terraform.io/hashicorp/aws"`) {
		t.Fatalf("verified artifact: %v", err)
	}
	// The executed binary is a private read-only copy of the pinned bytes:
	// replacing the configured path afterwards changes nothing it runs.
	if err := os.WriteFile(pinned, []byte("swapped after verification"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := fileSHA256(artifact.TerraformBinary); err != nil || got != f.release.Terraform.BinarySHA256 {
		t.Fatalf("private binary digest %q: %v", got, err)
	}
	if info, err := os.Stat(artifact.TerraformBinary); err != nil || info.Mode().Perm() != 0o500 {
		t.Fatalf("private binary mode: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(artifact.ModuleDir, "main.tf")); err != nil {
		t.Fatalf("module dir: %v", err)
	}
	// A writer of the private copy cannot reach the shared cache: the next
	// verification still produces a clean copy.
	if err := os.WriteFile(filepath.Join(artifact.ModuleDir, "extra.tf"), []byte("# injected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinned, []byte("pinned terraform"), 0o700); err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.VerifyForExecution(ctx, ref)
	if err != nil {
		t.Fatalf("second verification: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second.ModuleDir, "extra.tf")); !os.IsNotExist(err) {
		t.Fatalf("a private copy leaked into the next invocation: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(second.Root); !os.IsNotExist(err) && second.Cleanup != nil {
		t.Fatalf("private copy not removed: %v", err)
	}
	root := artifact.Root
	if err := artifact.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("private copy not removed: %v", err)
	}
	// Drift or a missing artifact in the shared cache stops the run.
	bundleRoot := f.cache.bundleDir(ref)
	cached := filepath.Join(bundleRoot, "sources", "platform@v1-2026.10.13", "vpc", "extra.tf")
	if err := os.WriteFile(cached, []byte("# injected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.VerifyForExecution(ctx, ref); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("injected file: %v", err)
	}
	if err := os.RemoveAll(bundleRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.VerifyForExecution(ctx, ref); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("missing artifact: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(f.cache.dir, "runs"))
	if len(entries) != 0 {
		t.Fatalf("failed verifications left %d private copies", len(entries))
	}
	f.svc.cfg.Release.Providers[0].Version = "5.81.0"
	if _, err := f.svc.VerifyForExecution(ctx, ref); !errors.Is(err, moduleregistry.ErrReleaseMismatch) {
		t.Fatalf("upgraded provider release: %v", err)
	}
}

// Preparation stops when a stored packaged bundle differs from the release.
func TestPrepareRejectsDriftedStoredBundle(t *testing.T) {
	f := newFixture(t, baseRepo())
	release := f.release
	release.Terraform.Version = "1.9.9"
	cache, err := OpenCache(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := New(Config{Release: release, Packaged: terraform.PackagedModules(), Cache: cache, Store: f.store})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Prepare(context.Background()); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("prepare over drifted rows: %v", err)
	}
}

func TestLockFileIsDeterministicAndPinned(t *testing.T) {
	providers := map[string]tfmodule.ProviderRequirement{"aws": {Source: "registry.terraform.io/hashicorp/aws", Version: "5.80.0", Hashes: []string{"zh:b", "h1:a"}}}
	lock := string(LockFile(providers))
	if !strings.Contains(lock, `version     = "5.80.0"`) || !strings.Contains(lock, `constraints = "5.80.0"`) || strings.Index(lock, "h1:a") > strings.Index(lock, "zh:b") {
		t.Fatalf("lock file:\n%s", lock)
	}
	if LockDigest(providers) != LockDigest(map[string]tfmodule.ProviderRequirement{"aws": providers["aws"]}) {
		t.Fatal("lock digest is not deterministic")
	}
}

func TestPolicyValidation(t *testing.T) {
	release, _ := DefaultRelease()
	bad := map[string]GitSource{
		"http url":          {ID: "a", Kind: "git", URL: "http://git.example.com/a.git", Modules: []GitModule{{ID: "m", Subdirectory: ".", ResourceTypes: []string{"t"}}}},
		"credentials":       {ID: "a", Kind: "git", URL: "https://u:p@git.example.com/a.git", Modules: []GitModule{{ID: "m", Subdirectory: ".", ResourceTypes: []string{"t"}}}},
		"query":             {ID: "a", Kind: "git", URL: "https://git.example.com/a.git?ref=main", Modules: []GitModule{{ID: "m", Subdirectory: ".", ResourceTypes: []string{"t"}}}},
		"ssh":               {ID: "a", Kind: "git", URL: "ssh://git@git.example.com/a.git", Modules: []GitModule{{ID: "m", Subdirectory: ".", ResourceTypes: []string{"t"}}}},
		"packaged kind":     {ID: "a", Kind: "packaged", URL: "https://git.example.com/a.git", Modules: []GitModule{{ID: "m", Subdirectory: ".", ResourceTypes: []string{"t"}}}},
		"platform id":       {ID: release.Packaged.SourceID, Kind: "git", URL: "https://git.example.com/a.git", Modules: []GitModule{{ID: "m", Subdirectory: ".", ResourceTypes: []string{"t"}}}},
		"traversal subdir":  {ID: "a", Kind: "git", URL: "https://git.example.com/a.git", Modules: []GitModule{{ID: "m", Subdirectory: "../x", ResourceTypes: []string{"t"}}}},
		"no resource types": {ID: "a", Kind: "git", URL: "https://git.example.com/a.git", Modules: []GitModule{{ID: "m", Subdirectory: "."}}},
		"relative ca file":  {ID: "a", Kind: "git", URL: "https://git.example.com/a.git", CAFile: "ca.pem", Modules: []GitModule{{ID: "m", Subdirectory: ".", ResourceTypes: []string{"t"}}}},
	}
	for name, source := range bad {
		err := Policy{Sources: []GitSource{source}}.Validate(release)
		if err == nil {
			t.Errorf("%s: accepted", name)
		} else if strings.Contains(err.Error(), "u:p") || strings.Contains(err.Error(), "git.example.com") {
			t.Errorf("%s: error echoes the URL: %v", name, err)
		}
	}
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "policy.json")
	raw, _ := json.Marshal(map[string]any{"sources": []any{map[string]any{"id": "a", "kind": "git", "url": "https://git.example.com/a.git", "modules": []any{}, "token": "x"}}})
	if err := os.WriteFile(policyFile, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(policyFile, release); err == nil {
		t.Fatal("unknown policy fields must be rejected")
	}
}

func TestCacheMustBePrivateAndOutsideGit(t *testing.T) {
	open := t.TempDir()
	if err := os.Chmod(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCache(open); err == nil {
		t.Fatal("a world-readable cache was accepted")
	}
	repo := privateDir(t)
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCache(filepath.Join(repo, "cache")); err == nil {
		t.Fatal("a cache inside a Git work tree was accepted")
	}
	if _, err := OpenCache("relative/cache"); err == nil {
		t.Fatal("a relative cache path was accepted")
	}
}

// The registry owns its release pins and Admin allowlist: changing the
// caller's Config values after New, or a Release returned by the service,
// changes neither preparation, registration policy nor execution checks.
func TestServiceOwnsReleaseAndPolicy(t *testing.T) {
	ctx := context.Background()
	release, err := DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "terraform")
	if err := os.WriteFile(binary, []byte("pinned terraform"), 0o700); err != nil {
		t.Fatal(err)
	}
	release.Terraform.BinarySHA256, _ = fileSHA256(binary)
	want := release.Clone()
	policy := testPolicy()
	repo := baseRepo()
	git := &fakeGit{repos: map[string]map[string]map[string]string{repoURL: {commitA: repo}}}
	cache, err := OpenCache(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New()
	svc, err := New(Config{Release: release, Policy: policy, Packaged: terraform.PackagedModules(), Git: git, Cache: cache, Store: st, TerraformBinary: binary})
	if err != nil {
		t.Fatal(err)
	}

	// Mutate every nested slice of the caller-owned values.
	release.Providers[0].Hashes[0] = "h1:tampered"
	release.Providers[0].Version = "9.9.9"
	release.ExecutorVariables[0] = "tampered"
	release.Packaged.Modules[0].ContentSHA256 = strings.Repeat("0", 64)
	release.Packaged.Modules[0].ResourceTypes[0] = "tampered"
	for i := range release.Packaged.Modules {
		if len(release.Packaged.Modules[i].ExecutorOutputs) > 0 {
			release.Packaged.Modules[i].ExecutorOutputs[0] = "tampered"
		}
	}
	policy.Sources[0].URL = "https://evil.example.com/acme/modules.git"
	policy.Sources[0].Modules[0].Subdirectory = "elsewhere"
	policy.Sources[0].Modules[0].ResourceTypes[0] = "tampered"

	if err := svc.Prepare(ctx); err != nil {
		t.Fatalf("prepare after caller mutation: %v", err)
	}
	if got := svc.Release(); !reflect.DeepEqual(got, want) {
		t.Fatalf("release changed through the caller's Config")
	}
	refs, _ := PackagedRefs()
	for id, ref := range refs {
		bundle, err := st.GetModuleBundle(ctx, ref)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if len(bundle.Providers) == 0 {
			t.Fatalf("%s has no provider requirement", id)
		}
		for name, provider := range bundle.Providers {
			pinned, ok := want.provider(provider.Source)
			if !ok || provider.Version != pinned.Version || !reflect.DeepEqual(provider.Hashes, pinned.Hashes) || pinned.Version == "9.9.9" {
				t.Fatalf("%s provider %s pins altered: %+v", id, name, provider)
			}
		}
		for _, typ := range bundle.Contract.ResourceTypes {
			if typ == "tampered" {
				t.Fatalf("%s resource types altered", id)
			}
		}
		for _, name := range append(bundle.Contract.ExecutorOwned, bundle.Contract.ExecutorOutputs...) {
			if name == "tampered" {
				t.Fatalf("%s executor names altered", id)
			}
		}
	}
	bundle, err := svc.Verify(ctx, refFor(repo))
	if err != nil || !bundle.ServesType("redis") || bundle.ServesType("tampered") {
		t.Fatalf("allowlisted source after caller mutation: %+v %v", bundle.Contract.ResourceTypes, err)
	}

	// A returned Release is a copy too.
	returned := svc.Release()
	returned.Providers[0].Hashes[0] = "h1:tampered"
	returned.Providers[0].Version = "9.9.9"
	returned.ExecutorVariables[0] = "tampered"
	returned.Packaged.Modules[0].ContentSHA256 = strings.Repeat("0", 64)
	returned.Packaged.Modules[0].ResourceTypes[0] = "tampered"
	if got := svc.Release(); !reflect.DeepEqual(got, want) {
		t.Fatal("release changed through a returned copy")
	}
	artifact, err := svc.VerifyForExecution(ctx, refs["vpc"])
	if err != nil || !strings.Contains(string(artifact.LockFile), want.Providers[0].Hashes[0]) || strings.Contains(string(artifact.LockFile), "tampered") {
		t.Fatalf("execution verification after mutation: %v", err)
	}
	if _, err := svc.Verify(ctx, refs["vpc"]); err != nil {
		t.Fatalf("packaged pin after mutation: %v", err)
	}
}

func TestReleaseAndPolicyCloneAreDeep(t *testing.T) {
	release, _ := DefaultRelease()
	c := release.Clone()
	c.Providers[0].Hashes[0] = "x"
	c.Packaged.Modules[1].ExecutorOutputs[0] = "x"
	c.Packaged.Modules[0].ResourceTypes[0] = "x"
	c.ExecutorVariables[0] = "x"
	again, _ := DefaultRelease()
	if !reflect.DeepEqual(release, again) {
		t.Fatal("Release.Clone shares nested slices")
	}
	policy := testPolicy()
	p := policy.Clone()
	p.Sources[0].Modules[0].ResourceTypes[0] = "x"
	p.Sources[0].Modules[0].Subdirectory = "x"
	p.Sources[1].URL = "x"
	if !reflect.DeepEqual(policy, testPolicy()) {
		t.Fatal("Policy.Clone shares nested slices")
	}
}

func TestLoadRetainedIsStrict(t *testing.T) {
	dir := t.TempDir()
	write := func(content string) string {
		p := filepath.Join(dir, "retained.json")
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got, err := LoadRetained(""); err != nil || got != nil {
		t.Fatalf("empty path: %v %v", got, err)
	}
	release := string(releaseJSON)
	ok := `[{"release":` + release + `,"terraformBinary":"/opt/terraform/1.9.8/terraform"}]`
	got, err := LoadRetained(write(ok))
	if err != nil || len(got) != 1 || got[0].TerraformBinary != "/opt/terraform/1.9.8/terraform" || got[0].Release.Terraform.Version == "" {
		t.Fatalf("valid file: %+v %v", got, err)
	}
	for name, content := range map[string]string{
		"relative binary": `[{"release":` + release + `,"terraformBinary":"terraform"}]`,
		"unknown field":   `[{"release":` + release + `,"terraformBinary":"/x","url":"https://x"}]`,
		"bad release":     `[{"release":{"terraform":{"version":"1","binarySha256":"x"}},"terraformBinary":"/x"}]`,
		"not a list":      `{}`,
	} {
		if _, err := LoadRetained(write(content)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
