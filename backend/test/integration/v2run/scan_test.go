package v2run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/test/integration/v2run"
)

func TestScanDetectsLiteralKnownAppPasswordFromPrivateScanInput(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(t.TempDir(), "app-secrets.txt")
	const pw = "Zx9-generated-app-pass-42"
	if err := v2run.WriteFilePrivate(priv, []byte("PGPASSWORD="+pw+"\nREDIS_PASSWORD=redis-pass-0987654\nACCEPTANCE_SECRET=app-secret-value-1\n")); err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(dir, "cleanup.log")
	if err := os.WriteFile(pub, []byte("clean line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := v2run.ScanPublic(dir, []string{priv}); err != nil || n != 1 {
		t.Fatalf("clean evidence must pass: n=%d err=%v", n, err)
	}
	for _, leaked := range []string{pw, "redis-pass-0987654", "app-secret-value-1"} {
		if err := os.WriteFile(pub, []byte("psql connect "+leaked+" ok\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := v2run.ScanPublic(dir, []string{priv})
		if err == nil || strings.Contains(err.Error(), leaked) {
			t.Fatalf("a literal app secret must be detected and never echoed: %v", err)
		}
	}
}

func TestRunMarkerMakesUnverifiedRunsDirty(t *testing.T) {
	root := t.TempDir()
	mk := func(name string) string {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		return d
	}
	_ = mk("unrelated")
	if got, _ := v2run.DirtyRuns(root); len(got) != 0 {
		t.Fatalf("unrelated dirs are excluded: %v", got)
	}
	d := mk("run-a")
	if err := v2run.WriteRunMarker(d, v2run.RunMarker{RunID: "a", Target: "k8s-4f", State: v2run.MarkerStarted}); err != nil {
		t.Fatal(err)
	}
	if got, _ := v2run.DirtyRuns(root); len(got) != 1 || got[0] != "run-a" {
		t.Fatalf("a STARTED marker without spec/status is dirty: %v", got)
	}
	if err := v2run.WriteRunMarker(d, v2run.RunMarker{RunID: "b", State: v2run.MarkerArmed}); err == nil {
		t.Fatal("another run id must not advance the marker")
	}
	if err := v2run.WriteRunMarker(mk("run-b"), v2run.RunMarker{RunID: "b", State: v2run.MarkerClosedNoEffects}); err == nil {
		t.Fatal("a marker cannot start closed")
	}
	if err := v2run.WriteRunMarker(d, v2run.RunMarker{RunID: "a", State: v2run.MarkerClosedNoEffects, Reason: "operator verified nothing was created"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := v2run.DirtyRuns(root); len(got) != 0 {
		t.Fatalf("an explicitly closed no-effects run is not dirty: %v", got)
	}
	if err := v2run.WriteRunMarker(d, v2run.RunMarker{RunID: "a", State: v2run.MarkerStarted}); err == nil {
		t.Fatal("a closed marker must stay closed")
	}
	// A corrupt marker is dirty, never ignorable.
	c := mk("run-c")
	_ = os.WriteFile(filepath.Join(c, v2run.RunMarkerName), []byte("{"), 0o600)
	if got, _ := v2run.DirtyRuns(root); len(got) != 1 || got[0] != "run-c" {
		t.Fatalf("a corrupt marker is dirty: %v", got)
	}
}

func TestScanDetectsAJSONKeyHandleValues(t *testing.T) {
	dir := t.TempDir()
	handle := filepath.Join(t.TempDir(), "aws-key.json")
	if err := v2run.WriteFilePrivate(handle, []byte(`{"accessKeyId":"AKIAEXAMPLE0000000A","secretAccessKey":"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}`)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.log"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := v2run.ScanPublic(dir, []string{handle}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.log"), []byte("key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := v2run.ScanPublic(dir, []string{handle}); err == nil {
		t.Fatal("a secret access key inside a JSON handle must be detected")
	}
}
