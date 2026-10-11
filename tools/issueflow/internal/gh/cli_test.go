package gh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"issueflow/internal/runner"
)

type script struct {
	r    *runner.Router
	resp func(args []string) runner.Result
}

func newCLI(resp func(args []string) runner.Result) (*CLI, *runner.Router) {
	r := runner.NewRouter(nil)
	r.Handle("gh", func(c runner.Call) (runner.Result, error, bool) { return resp(c.Args), nil, true })
	return &CLI{Repo: "o/r", R: r}, r
}

func has(r *runner.Router, sub string) bool {
	for _, c := range r.Calls() {
		if strings.Contains(c.String(), sub) {
			return true
		}
	}
	return false
}

func TestEnsureReleaseCreatesOnlyAfterConfirmedNotFound(t *testing.T) {
	ctx := context.Background()
	created := false
	cli, r := newCLI(func(a []string) runner.Result {
		switch {
		case a[0] == "release" && a[1] == "view" && !created:
			return runner.Result{Exit: 1, Stderr: "release not found"}
		case a[0] == "release" && a[1] == "create":
			created = true
			return runner.Result{}
		case a[0] == "release" && a[1] == "view":
			return runner.Result{Stdout: `{"url":"https://x/r","isPrerelease":true}`}
		}
		return runner.Result{Exit: 9}
	})
	rel, err := cli.EnsureRelease(ctx, "tag1", "t", "n")
	if err != nil || !rel.Prerelease || rel.URL != "https://x/r" || !created {
		t.Fatalf("%+v %v", rel, err)
	}
	if !has(r, "--prerelease") {
		t.Fatal("must create a prerelease")
	}
}

func TestEnsureReleaseNeverCreatesOnOtherErrors(t *testing.T) {
	for name, res := range map[string]runner.Result{
		"auth":    {Exit: 1, Stderr: "HTTP 401: Bad credentials"},
		"network": {Exit: 1, Stderr: "dial tcp: lookup api.github.com: no such host"},
		"parse":   {Stdout: "<html>not json</html>"},
		"403":     {Exit: 1, Stderr: "HTTP 403: Resource not accessible by integration"},
	} {
		t.Run(name, func(t *testing.T) {
			cli, r := newCLI(func(a []string) runner.Result { return res })
			if _, err := cli.EnsureRelease(context.Background(), "tag1", "t", "n"); err == nil {
				t.Fatal("error swallowed")
			}
			if has(r, "release create") {
				t.Fatal("a release was created after an error that is not 'not found'")
			}
		})
	}
}

func TestEnsureReleaseRaceAcceptsConcurrentCreator(t *testing.T) {
	views := 0
	cli, _ := newCLI(func(a []string) runner.Result {
		switch a[1] {
		case "view":
			views++
			if views == 1 {
				return runner.Result{Exit: 1, Stderr: "release not found"}
			}
			return runner.Result{Stdout: `{"url":"u","isPrerelease":true}`}
		case "create":
			return runner.Result{Exit: 1, Stderr: "a release with the same tag name already exists"}
		}
		return runner.Result{Exit: 9}
	})
	if rel, err := cli.EnsureRelease(context.Background(), "t", "t", "n"); err != nil || rel.URL != "u" {
		t.Fatalf("%+v %v", rel, err)
	}
}

func TestUploadNeverClobbers(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a-b.txt")
	os.WriteFile(f, []byte("x"), 0o600)
	cli, r := newCLI(func(a []string) runner.Result { return runner.Result{} })
	if err := cli.UploadAsset(context.Background(), "tag", f); err != nil {
		t.Fatal(err)
	}
	if has(r, "--clobber") {
		t.Fatal("upload must never overwrite an existing asset")
	}
	if err := cli.UploadAsset(context.Background(), "tag", f+".missing"); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestListAssetsParsesDigest(t *testing.T) {
	cli, _ := newCLI(func(a []string) runner.Result {
		return runner.Result{Stdout: `{"assets":[{"name":"a","size":3,"url":"u","digest":"sha256:abc"},{"name":"b","size":4,"url":"v"}]}`}
	})
	as, err := cli.ListAssets(context.Background(), "t")
	if err != nil || len(as) != 2 || as[0].Digest != "sha256:abc" || as[1].Digest != "" {
		t.Fatalf("%+v %v", as, err)
	}
}

func TestIssueAndCommentReads(t *testing.T) {
	cli, r := newCLI(func(a []string) runner.Result {
		switch {
		case a[0] == "api" && strings.Contains(a[len(a)-1], "/events"):
			return runner.Result{Stdout: `[{"event":"labeled","actor":{"login":"x"},"label":{"name":"agent:ready"}}][{"event":"labeled","actor":{"login":"owner"},"label":{"name":"agent:ready"}}]`}
		case a[0] == "api" && strings.HasSuffix(a[len(a)-1], "/comments") && len(a) == 3:
			return runner.Result{Stdout: `[{"id":5,"body":"hi <!-- m -->","user":{"login":"u"},"created_at":"2026-01-01T00:00:00Z"}][{"id":3,"body":"old","user":{"login":"u"},"created_at":"2026-01-01T00:00:00Z"}]`}
		}
		return runner.Result{Exit: 9, Stderr: "unexpected " + strings.Join(a, " ")}
	})
	ctx := context.Background()
	if actor, err := cli.LabelActor(ctx, 9, "agent:ready"); err != nil || actor != "owner" {
		t.Fatalf("most recent labeler expected: %q %v", actor, err)
	}
	cs, err := cli.Comments(ctx, 9, 3)
	if err != nil || len(cs) != 1 || cs[0].ID != 5 {
		t.Fatalf("%+v %v", cs, err)
	}
	if id, ok, _ := cli.FindMarker(ctx, 9, "<!-- m -->"); !ok || id != 5 {
		t.Fatal("marker not found")
	}
	if has(r, "--clobber") {
		t.Fatal("unexpected")
	}
}

// More than one page of ready Issues: the smallest overall number must win even
// when it is only on the last page, and pull requests are skipped.
func TestListIssuesPaginatesOldestFirstAndSkipsPulls(t *testing.T) {
	page := func(from, to int, pr int) string {
		var items []string
		for n := from; n <= to; n++ {
			extra := ""
			if n == pr {
				extra = `,"pull_request":{}`
			}
			items = append(items, fmt.Sprintf(`{"number":%d,"title":"t%d","body":"b","state":"open","user":{"login":"o"},"labels":[{"name":"agent:ready"}]%s}`, n, n, extra))
		}
		return "[" + strings.Join(items, ",") + "]"
	}
	// gh --paginate prints pages one after another; here the global smallest (7)
	// sits on the second page, and number 150 is a pull request.
	out := page(200, 299, 0) + page(100, 199, 150) + page(7, 7, 0)
	cli, r := newCLI(func(a []string) runner.Result { return runner.Result{Stdout: out} })
	is, err := cli.ListIssues(context.Background(), "agent:ready")
	if err != nil {
		t.Fatal(err)
	}
	if len(is) != 100+100-1+1 || is[0].Number != 7 || is[1].Number != 100 || is[len(is)-1].Number != 299 {
		t.Fatalf("got %d issues, first %d", len(is), is[0].Number)
	}
	for _, i := range is {
		if i.Number == 150 {
			t.Fatal("a pull request was returned as an issue")
		}
	}
	args := r.Calls()[0].String()
	for _, want := range []string{"--paginate", "sort=created", "direction=asc", "per_page=100", "labels=agent%3Aready", "state=open"} {
		if !strings.Contains(args, want) {
			t.Errorf("request lacks %q: %s", want, args)
		}
	}
	if is[0].State != "OPEN" || is[0].Author != "o" || is[0].Labels[0] != "agent:ready" {
		t.Fatalf("%+v", is[0])
	}
}

func TestCommentUsesFieldNotShellInterpolation(t *testing.T) {
	cli, r := newCLI(func(a []string) runner.Result { return runner.Result{Stdout: `{"id":77}`} })
	body := "`rm -rf /` $(whoami) \"quotes\"\nline2"
	id, err := cli.Comment(context.Background(), 4, body)
	if err != nil || id != 77 {
		t.Fatalf("%d %v", id, err)
	}
	c := r.Calls()[0]
	found := false
	for _, a := range c.Args {
		if a == "body="+body {
			found = true
		}
	}
	if !found {
		t.Fatalf("body must be passed as one argument, got %v", c.Args)
	}
	if got := fmt.Sprint(c.Name); got != "gh" {
		t.Fatal(got)
	}
}

func TestFakeNeverOverwritesAssets(t *testing.T) {
	f := NewFake(nil)
	p := filepath.Join(t.TempDir(), "n")
	os.WriteFile(p, []byte("x"), 0o600)
	if err := f.UploadAsset(context.Background(), "t", p); err != nil {
		t.Fatal(err)
	}
	if err := f.UploadAsset(context.Background(), "t", p); err == nil {
		t.Fatal("fake must refuse to overwrite like the real publisher contract")
	}
}
