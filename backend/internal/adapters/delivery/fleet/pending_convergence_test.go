package fleet

import (
	"context"
	"encoding/json"
	"errors"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActiveReceiptConvergenceKeepsExactChainAndBumpsOnlyOnce(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	ctx := context.Background()
	i, _, r := publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	file := filepath.Join(dir, "repo.json")
	before, _ := os.ReadFile(file)
	os.WriteFile(filepath.Join(dir, "bundle-commit"), []byte("old-commit"), 0600)
	os.WriteFile(filepath.Join(dir, "bd-commit"), []byte("old-commit"), 0600)
	observed, err := p.Observe(ctx, i, r)
	if err != nil || observed.Ready(i.Revision, time.Now(), time.Minute) {
		t.Fatal("stale chain admitted", err)
	}
	unchanged, _ := os.ReadFile(file)
	if string(before) != string(unchanged) {
		t.Fatal("read-only observation changed controller")
	}
	os.WriteFile(filepath.Join(dir, "lose-patch-ack"), nil, 0600)
	if err := p.ConvergePending(ctx, i, r); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	var repo map[string]any
	json.Unmarshal(after, &repo)
	if repo["spec"].(map[string]any)["forceSyncGeneration"] != float64(1) {
		t.Fatal("missing bounded Git render generation")
	}
	if err := p.ConvergePending(ctx, i, r); err != nil {
		t.Fatal(err)
	}
	repeated, _ := os.ReadFile(file)
	if string(after) != string(repeated) {
		t.Fatal("same receipt advanced again")
	}
	// A render request itself never manufactures Ready.
	observed, err = p.Observe(ctx, i, r)
	if err != nil || observed.Ready(i.Revision, time.Now(), time.Minute) {
		t.Fatal("nudge bypassed controller chain", err)
	}
	os.WriteFile(filepath.Join(dir, "bundle-commit"), []byte(r.Handle["commit"]), 0600)
	observed, err = p.Observe(ctx, i, r)
	if err != nil || observed.Ready(i.Revision, time.Now(), time.Minute) {
		t.Fatal("old BundleDeployment admitted", err)
	}
	os.WriteFile(filepath.Join(dir, "bd-commit"), []byte(r.Handle["commit"]), 0600)
	observed, err = p.Observe(ctx, i, r)
	if err != nil || !observed.Ready(i.Revision, time.Now(), time.Minute) {
		t.Fatal("actual exact chain not acknowledged", err)
	}
}

func TestActiveReceiptConvergenceDoesNotTouchJobsOrForeignControllers(t *testing.T) {
	for _, kind := range []string{"job", "foreign-controller", "lost-owner", "sibling-job"} {
		t.Run(kind, func(t *testing.T) {
			p, w, dir, intent := applicationFixture(t)
			ctx := context.Background()
			i, _, r := publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
			file := filepath.Join(dir, "repo.json")
			data, _ := os.ReadFile(file)
			var repo map[string]any
			json.Unmarshal(data, &repo)
			switch kind {
			case "job":
				i.Revision.Phase = domain.Job
			case "foreign-controller":
				repo["metadata"].(map[string]any)["uid"] = "foreign"
			case "lost-owner":
				p.Manager.Guard = func(context.Context, port.Intent) error { return ErrOwnership }
			case "sibling-job":
				pins, _ := readUnitPins(repo)
				pins["job"] = unitPin{Path: "environments/staging/worker/job", EnvironmentID: i.Revision.Scope.EnvironmentID, Revision: "job-revision", Digest: "job-digest"}
				repo["spec"].(map[string]any)["paths"] = sharedPaths(pins)
				raw, _ := json.Marshal(pins)
				repo["metadata"].(map[string]any)["annotations"].(map[string]any)[unitPinsAnnotation] = string(raw)
			}
			body, _ := json.Marshal(repo)
			os.WriteFile(file, body, 0600)
			os.WriteFile(filepath.Join(dir, "bundle-commit"), []byte("old"), 0600)
			err := p.ConvergePending(ctx, i, r)
			if kind == "foreign-controller" || kind == "lost-owner" {
				if !errors.Is(err, ErrOwnership) {
					t.Fatal("ownership refusal missing", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(file)
			if string(body) != string(after) {
				t.Fatal("protected controller changed")
			}
		})
	}
}

func TestSharedAdmissionPreservesAnExistingRenderCounter(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	file := filepath.Join(dir, "repo.json")
	data, _ := os.ReadFile(file)
	var repo map[string]any
	json.Unmarshal(data, &repo)
	repo["spec"].(map[string]any)["forceSyncGeneration"] = float64(7)
	body, _ := json.Marshal(repo)
	os.WriteFile(file, body, 0600)
	i, _, r := publishApplicationUnit(t, p, w, dir, intent("production", "frontend"))
	after, _ := os.ReadFile(file)
	json.Unmarshal(after, &repo)
	if repo["spec"].(map[string]any)["forceSyncGeneration"] != float64(7) {
		t.Fatal("ordinary admission reset/advanced render counter")
	}
	if _, err := p.Withdraw(context.Background(), i, r); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(file)
	json.Unmarshal(after, &repo)
	if repo["spec"].(map[string]any)["forceSyncGeneration"] != float64(7) {
		t.Fatal("withdrawal reset/advanced render counter")
	}
}
