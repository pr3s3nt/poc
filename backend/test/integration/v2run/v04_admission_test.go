package v2run_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/test/integration/v2run"
)

func writeIntegratedV04Fixture(t *testing.T) (string, *v2run.AWSConfig) {
	t.Helper()
	dir := t.TempDir()
	manifest := "fixture source manifest\n"
	mh := sha256.Sum256([]byte(strings.TrimSuffix(manifest, "\n")))
	source := map[string]string{"gitHead": strings.Repeat("a", 40), "gitDiffSha256": strings.Repeat("b", 64), "sourceFilesSha256": hex.EncodeToString(mh[:])}
	writeJSON := func(name string, v any) {
		t.Helper()
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	writeJSON("run.json", map[string]any{"scenario": "V04", "runId": "v04-owned-1234", "target": v2run.TargetK8s4f, "fake": "none", "source": source})
	var assertions []map[string]any
	for _, id := range v2run.V04RequiredAssertions() {
		assertions = append(assertions, map[string]any{"id": id, "pass": true, "source": "private-read"})
	}
	writeJSON("assertions.json", assertions)
	files := map[string]string{
		"source-files.sha256": manifest, "phases.json": "[]\n", "marks.json": "[]\n", "assertions.txt": "PASS contract fixture\n", "result.txt": "PASS\n",
		"V04-raw.mp4": "unit fixture, not a real video", "V04-captioned.mp4": "unit fixture, not a real video", "captions.srt": "fixture", "captions.ass": "fixture", "ffprobe.json": "{}\n",
		"decode.log": "", "decode-captioned.log": "", "checks.txt": "fixture check results\n", "cleanup.txt": "fixture cleanup results\n", "cleanup-state.txt": "proven\n",
	}
	for name, body := range files {
		if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	hashes := map[string]string{}
	for _, name := range append([]string{"run.json", "assertions.json"}, mapKeys(files)...) {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		h := sha256.Sum256(b)
		hashes[name] = hex.EncodeToString(h[:])
	}
	writeJSON("v04-admission-receipt.json", map[string]any{
		"version": 2, "acceptanceScope": "MAIN_FLOW_2026_10_19", "completedAt": time.Now().UTC(), "scenario": "V04", "runId": "v04-owned-1234", "target": v2run.TargetK8s4f, "source": source, "evidenceSha256": hashes,
		"cleanupState": v2run.MarkerClosedClean, "privateMaterialRemoved": true, "sqlReopenPassed": true, "finalVideoPassed": true, "finalSecretScanPassed": true, "status": "PASS", "exitCode": 0,
	})
	return dir, &v2run.AWSConfig{V04IntegratedEvidence: dir, V04ExpectedRunID: "v04-owned-1234", V04ExpectedSourceGitHead: source["gitHead"], V04ExpectedGitDiffSHA256: source["gitDiffSha256"], V04ExpectedSourceFilesSHA256: source["sourceFilesSha256"]}
}

func mapKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func mutateV04Receipt(t *testing.T, dir string, change func(map[string]any)) {
	t.Helper()
	p := filepath.Join(dir, "v04-admission-receipt.json")
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	var r map[string]any
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	change(r)
	b, e = json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
}

func rewriteV04Evidence(t *testing.T, dir, name string, b []byte) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	mutateV04Receipt(t, dir, func(r map[string]any) { r["evidenceSha256"].(map[string]any)[name] = hex.EncodeToString(h[:]) })
}

func TestAWSGateAcceptsOnlyBoundCompleteIntegratedV04Predecessor(t *testing.T) {
	_, aws := writeIntegratedV04Fixture(t)
	c := awsConfig(t, "V05", "t4g.medium")
	c.AWS = aws
	if e := v2run.AWSGate(c); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func(*testing.T, string){
		"missing main-flow scope": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { delete(r, "acceptanceScope") })
		},
		"wrong acceptance scope": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { r["acceptanceScope"] = "HISTORICAL_NEGATIVES" })
		},
		"cleanup pending": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { r["cleanupState"] = "INCOMPLETE" })
		},
		"missing exit code": func(t *testing.T, d string) { mutateV04Receipt(t, d, func(r map[string]any) { delete(r, "exitCode") }) },
		"failed exit code":  func(t *testing.T, d string) { mutateV04Receipt(t, d, func(r map[string]any) { r["exitCode"] = 1 }) },
		"private flag false": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { r["privateMaterialRemoved"] = false })
		},
		"private directory still exists": func(t *testing.T, d string) {
			if e := os.Mkdir(filepath.Join(d, "private"), 0700); e != nil {
				t.Fatal(e)
			}
		},
		"unresolved cleanup marker": func(t *testing.T, d string) {
			if e := os.WriteFile(filepath.Join(d, "CLEANUP-NOT-PROVEN.txt"), []byte("pending"), 0600); e != nil {
				t.Fatal(e)
			}
		},
		"physical cleanup not proven": func(t *testing.T, d string) { rewriteV04Evidence(t, d, "cleanup-state.txt", []byte("failed\n")) },
		"video failed": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { r["finalVideoPassed"] = false })
		},
		"secret scan failed": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { r["finalSecretScanPassed"] = false })
		},
		"browser PASS without reopen": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { r["sqlReopenPassed"] = false })
		},
		"future completion": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { r["completedAt"] = time.Now().Add(time.Hour) })
		},
		"stale source": func(t *testing.T, d string) {
			mutateV04Receipt(t, d, func(r map[string]any) { r["source"].(map[string]any)["gitHead"] = strings.Repeat("f", 40) })
		},
		"altered evidence after receipt": func(t *testing.T, d string) {
			if e := os.WriteFile(filepath.Join(d, "assertions.json"), []byte("[]\n"), 0600); e != nil {
				t.Fatal(e)
			}
		},
		"only negatives without real app": func(t *testing.T, d string) {
			b, e := os.ReadFile(filepath.Join(d, "assertions.json"))
			if e != nil {
				t.Fatal(e)
			}
			var a []map[string]any
			if e = json.Unmarshal(b, &a); e != nil {
				t.Fatal(e)
			}
			var negative []map[string]any
			for _, v := range a {
				if strings.HasPrefix(v["id"].(string), "negative.") {
					negative = append(negative, v)
				}
			}
			b, e = json.Marshal(negative)
			if e != nil {
				t.Fatal(e)
			}
			rewriteV04Evidence(t, d, "assertions.json", b)
		},
		"failed happy assertion": func(t *testing.T, d string) {
			b, _ := os.ReadFile(filepath.Join(d, "assertions.json"))
			var a []map[string]any
			_ = json.Unmarshal(b, &a)
			a[0]["pass"] = false
			b, _ = json.Marshal(a)
			rewriteV04Evidence(t, d, "assertions.json", b)
		},
		"simulated assertion": func(t *testing.T, d string) {
			b, _ := os.ReadFile(filepath.Join(d, "assertions.json"))
			var a []map[string]any
			_ = json.Unmarshal(b, &a)
			a[0]["source"] = "fake"
			b, _ = json.Marshal(a)
			rewriteV04Evidence(t, d, "assertions.json", b)
		},
		"duplicate assertion": func(t *testing.T, d string) {
			b, _ := os.ReadFile(filepath.Join(d, "assertions.json"))
			var a []map[string]any
			_ = json.Unmarshal(b, &a)
			a = append(a, a[0])
			b, _ = json.Marshal(a)
			rewriteV04Evidence(t, d, "assertions.json", b)
		},
		"manifest does not match source": func(t *testing.T, d string) {
			rewriteV04Evidence(t, d, "source-files.sha256", []byte("different source\n"))
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			d, a := writeIntegratedV04Fixture(t)
			change(t, d)
			c := awsConfig(t, "V05", "t4g.medium")
			c.AWS = a
			if e := v2run.AWSGate(c); e == nil {
				t.Fatal("invalid predecessor admitted AWS")
			}
		})
	}
}

func TestIntegratedV04ReuseDependsOnSourceAndAssertionsNotAge(t *testing.T) {
	dir, a := writeIntegratedV04Fixture(t)
	mutateV04Receipt(t, dir, func(r map[string]any) { r["completedAt"] = time.Now().Add(-48 * time.Hour) })
	for _, scenario := range []string{"V05", "A02", "A03"} {
		c := awsConfig(t, scenario, "t4g.medium")
		c.AWS = a
		if e := v2run.AWSGate(c); e != nil {
			t.Fatalf("unchanged predecessor for %s: %v", scenario, e)
		}
	}
}

func TestHistoricalM04CannotAdmitCurrentAWSScenarios(t *testing.T) {
	for _, scenario := range []string{"V05", "A02", "A03"} {
		c := awsConfig(t, scenario, "t4g.medium")
		c.AWS.I0411Evidence = []string{evidenceRun(t, "A01", "PASS", v2run.MarkerClosedClean), evidenceRun(t, "E01", "PASS", v2run.MarkerClosedClean), evidenceRun(t, "E02", "PASS", v2run.MarkerClosedClean)}
		if e := v2run.AWSGate(c); e == nil {
			t.Fatalf("historical proof admitted current %s", scenario)
		}
	}
}
