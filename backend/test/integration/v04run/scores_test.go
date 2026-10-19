package v04run

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/seed"
)

func options() Options {
	return Options{Frontend: "acceptance-frontend:v1", Backend: "acceptance-backend:v1", Worker: "acceptance-worker:v1",
		Application: "acc-v04", Environment: "staging", RunID: "v04-run-1", TTLSeconds: 30}
}

func TestBuildKeepsTheRepositoryScoreSourceIdentity(t *testing.T) {
	result, err := Build(options())
	if err != nil {
		t.Fatal(err)
	}
	base := seed.Defaults()
	base.Images = seed.Images{Frontend: "acceptance-frontend:v1", Backend: "acceptance-backend:v1", Worker: "acceptance-worker:v1"}
	source := seed.AcceptanceScoresRedis(base, "acceptance-cache", "default", "acc-v04", "staging", "v04-run-1")
	for _, w := range result.Workloads {
		want := source[w.Name]["containers"].(map[string]any)["main"].(map[string]any)
		got := w.Document["containers"].(map[string]any)["main"].(map[string]any)
		// Requests/limits, probes and the image are the repository source's, byte for byte (V03 resource tests keep protecting them).
		for _, property := range []string{"image", "readinessProbe", "livenessProbe", "resources"} {
			if !reflect.DeepEqual(roundTrip(want[property]), roundTrip(got[property])) {
				t.Fatalf("%s %s differs from the source: %v vs %v", w.Name, property, want[property], got[property])
			}
		}
		if w.SourceHash == w.Hash {
			t.Fatalf("%s: the generated Score must differ from the source (literals become references) and say so", w.Name)
		}
	}
	backend, worker := workload(t, result, "backend"), workload(t, result, "worker")
	if !strings.Contains(backend.YAML, "memory: 256Mi") || !strings.Contains(worker.YAML, "memory: 128Mi") {
		t.Fatalf("limits are not preserved:\n%s\n%s", backend.YAML, worker.YAML)
	}
}

func TestBuildAppliesTheRequestedTTLToBothRuntimeConsumers(t *testing.T) {
	for _, ttl := range []int{17, 45} {
		t.Run(fmt.Sprint(ttl), func(t *testing.T) {
			o := options()
			o.TTLSeconds = ttl
			result, err := Build(o)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"backend", "worker"} {
				variables := workload(t, result, name).Document["containers"].(map[string]any)["main"].(map[string]any)["variables"].(map[string]any)
				if variables["ACCEPTANCE_CACHE_TTL_SECONDS"] != "${resources.env.ACCEPTANCE_CACHE_TTL_SECONDS}" {
					t.Fatalf("%s does not reference the shared TTL setting: %v", name, variables)
				}
			}
			matches := 0
			for _, config := range result.Config {
				if config.Name == "ACCEPTANCE_CACHE_TTL_SECONDS" {
					matches++
					if config.Value != fmt.Sprint(ttl) {
						t.Fatalf("TTL config = %+v, want %d", config, ttl)
					}
				}
			}
			if matches != 1 {
				t.Fatalf("shared TTL config occurs %d times", matches)
			}
			foundDelta := false
			for _, change := range result.Changes {
				if change.Kind == "cache-ttl" && (change.Workload == "backend" || change.Workload == "worker") && strings.Contains(change.Detail, fmt.Sprint(ttl)) {
					foundDelta = true
				}
			}
			if !foundDelta {
				t.Fatalf("the exact TTL delta is not recorded: %+v", result.Changes)
			}
		})
	}
}

func TestBuildRejectsUnreasonableTTL(t *testing.T) {
	for _, ttl := range []int{-1, 3601} {
		o := options()
		o.TTLSeconds = ttl
		if _, err := Build(o); err == nil {
			t.Fatalf("TTL %d was accepted", ttl)
		}
	}
}

func roundTrip(v any) any {
	raw, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(raw, &out)
	return out
}

func workload(t *testing.T, r Result, name string) Workload {
	t.Helper()
	for _, w := range r.Workloads {
		if w.Name == name {
			return w
		}
	}
	t.Fatalf("no workload %s", name)
	return Workload{}
}

func TestBuildUsesReferencesOnlyAndRecordsEveryDifference(t *testing.T) {
	result, err := Build(options())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range result.Workloads {
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(w.YAML), &parsed); err != nil {
			t.Fatalf("%s is not valid YAML: %v", w.Name, err)
		}
		variables := parsed["containers"].(map[string]any)["main"].(map[string]any)["variables"].(map[string]any)
		for key, value := range variables {
			if !strings.HasPrefix(value.(string), "${resources.") {
				t.Fatalf("%s %s is a literal: %v", w.Name, key, value)
			}
		}
	}
	kinds := map[string]bool{}
	for _, change := range result.Changes {
		kinds[change.Workload+"/"+change.Kind] = true
	}
	for _, want := range []string{"backend/literal-to-environment-key", "worker/literal-to-environment-key", "frontend/literal-to-environment-key", "backend/class", "worker/class", "backend/added-check-config", "frontend/service-reference"} {
		if !kinds[want] {
			t.Errorf("difference %s is not recorded: %+v", want, result.Changes)
		}
	}
	front := workload(t, result, "frontend")
	if !strings.Contains(front.YAML, "${resources.backend.url}") || !strings.Contains(front.YAML, "type: service") {
		t.Fatalf("the frontend must reach the backend through the Service reference:\n%s", front.YAML)
	}
	if strings.Contains(front.YAML, "http://backend:8080") {
		t.Fatal("a literal backend URL leaked into the frontend Score")
	}
}

func TestBuildSharesOneDatabaseAndOneCacheAndNeverHoldsASecretValue(t *testing.T) {
	result, err := Build(options())
	if err != nil {
		t.Fatal(err)
	}
	backend, worker := workload(t, result, "backend"), workload(t, result, "worker")
	for _, alias := range []string{"db", "cache"} {
		b := backend.Document["resources"].(map[string]any)[alias].(map[string]any)
		w := worker.Document["resources"].(map[string]any)[alias].(map[string]any)
		if b["id"] == "" || b["id"] != w["id"] || b["class"] != "in-cluster" || w["class"] != "in-cluster" {
			t.Fatalf("%s: backend %v worker %v", alias, b, w)
		}
	}
	secrets := 0
	for _, key := range result.Config {
		if key.Kind == "secret" {
			secrets++
			if key.Value != "" || key.Name != "ACCEPTANCE_SECRET" {
				t.Fatalf("secret key %+v", key)
			}
		}
	}
	if secrets != 1 {
		t.Fatalf("exactly the acceptance secret is a secret key, got %d", secrets)
	}
	if front := workload(t, result, "frontend"); strings.Contains(front.YAML, "ACCEPTANCE_SECRET") {
		t.Fatal("the frontend must not reference the secret")
	}
	if strings.Contains(worker.YAML, "ACCEPTANCE_SECRET") {
		t.Fatal("the worker must not reference the secret")
	}
	names := map[string]string{}
	for _, key := range result.Config {
		names[key.Name] = key.Kind
	}
	for _, want := range []string{"PORT", "POLL_INTERVAL_MS", "ACCEPTANCE_CONFIG", "ACCEPTANCE_SECRET_SHA256", "ACCEPTANCE_CACHE_APP", "ACCEPTANCE_CACHE_ENV", "ACCEPTANCE_CACHE_RUN", "ACCEPTANCE_CACHE_TTL_SECONDS"} {
		if names[want] != "variable" {
			t.Errorf("configuration key %s missing: %v", want, names)
		}
	}
}

func TestBuildIsDeterministicAndRejectsIncompleteOptions(t *testing.T) {
	first, err := Build(options())
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Build(options())
	for i := range first.Workloads {
		if first.Workloads[i].Hash != second.Workloads[i].Hash || first.Workloads[i].YAML != second.Workloads[i].YAML {
			t.Fatal("score generation must be deterministic")
		}
	}
	for name, mutate := range map[string]func(*Options){
		"no frontend": func(o *Options) { o.Frontend = "" },
		"no run":      func(o *Options) { o.RunID = "" },
	} {
		o := options()
		mutate(&o)
		if _, err := Build(o); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestGeneratedScoresParseAndConvertWithTheProductScoreParser(t *testing.T) {
	result, err := Build(options())
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]resource.Type{"postgres": {Key: "postgres", Inputs: []resource.InputField{{Name: "database", Type: "string", Required: true}, {Name: "username", Type: "string", Required: true}},
		Outputs: []resource.OutputField{{Name: "host", Type: "string"}, {Name: "port", Type: "number"}, {Name: "database", Type: "string"}, {Name: "username", Type: "string"}, {Name: "password", Type: "string", Secret: true}}},
		"redis": seed.RedisType()}
	for _, w := range result.Workloads {
		doc, err := score.FromMap(w.Document)
		if err != nil {
			t.Fatalf("%s: %v", w.Name, err)
		}
		fragment, err := doc.Fragment(types)
		if err != nil {
			t.Fatalf("%s: %v", w.Name, err)
		}
		if fragment.WorkloadID != w.Name {
			t.Fatalf("workload id %q", fragment.WorkloadID)
		}
		// Backend and worker declare the same shared entries (one PostgreSQL, one Redis).
		if w.Name != "frontend" && (len(fragment.Shared) != 2 || fragment.Shared["acceptance-db"].Class != "in-cluster" || fragment.Shared["acceptance-cache"].Class != "in-cluster") {
			t.Fatalf("%s shared = %+v", w.Name, fragment.Shared)
		}
	}
}

func TestProbeScoreRequestsTheRunOwnedReadinessFailureResource(t *testing.T) {
	text, doc, err := ProbeScore(options())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := score.FromMap(doc)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := parsed.Fragment(map[string]resource.Type{"redis": {Key: "redis", Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := fragment.Module.Spec.Containers["main"].Variables["REDIS_HOST"]; got != "${shared.readiness-probe.host}" {
		t.Fatalf("E02 workload must actually consume the failed provider output: %q", got)
	}
	if entry := fragment.Shared["readiness-probe"]; entry.Type != "redis" || entry.Class != "readiness-probe" {
		t.Fatalf("shared = %+v", fragment.Shared)
	}
	if strings.Contains(text, "ACCEPTANCE_SECRET") || strings.Contains(text, "acceptance-db") || strings.Contains(text, "Bad-Name") {
		t.Fatal("the probe must neither use an application secret, a database, nor a pre-apply-invalid name")
	}
}

func TestBuildCanOmitTheOptionalApplicationSecretForEKSTargets(t *testing.T) {
	o := options()
	o.OmitAppSecret = true
	result, err := Build(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range result.Workloads {
		if strings.Contains(w.YAML, "ACCEPTANCE_SECRET") {
			t.Fatalf("%s still references the optional secret:\n%s", w.Name, w.YAML)
		}
	}
	for _, key := range result.Config {
		if key.Kind == "secret" || key.Name == "ACCEPTANCE_SECRET_SHA256" {
			t.Fatalf("the optional secret key %s must not be configured", key.Name)
		}
	}
	// The database and cache credentials are never optional: they stay references to resource outputs.
	backend := workload(t, result, "backend")
	for _, want := range []string{"${resources.db.password}", "${resources.cache.password}", "${resources.env.ACCEPTANCE_CONFIG}"} {
		if !strings.Contains(backend.YAML, want) {
			t.Fatalf("backend lost %s", want)
		}
	}
}

func TestPreApplyProbeScoreUsesDistinctRunOwnedPostgresAndInvalidIdentifier(t *testing.T) {
	text, doc, err := PreApplyProbeScore(options())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := score.FromMap(doc)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := parsed.Fragment(map[string]resource.Type{"postgres": {Key: "postgres", Inputs: []resource.InputField{{Name: "database", Type: "string", Required: true}, {Name: "username", Type: "string", Required: true}}, Outputs: []resource.OutputField{{Name: "host", Type: "string"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := fragment.Module.Spec.Containers["main"].Variables["PGHOST"]; got != "${shared.v04-e01-v04-run-1.host}" {
		t.Fatalf("E01 workload must actually consume the failed provider output: %q", got)
	}
	entry := fragment.Shared["v04-e01-v04-run-1"]
	if entry.Type != "postgres" || entry.Class != "invalid-name-probe" || entry.Params["database"] != "Bad.Name" || entry.Params["username"] != "app" {
		t.Fatalf("shared E01 = %+v", entry)
	}
	if !strings.Contains(text, "probe-e01") || strings.Contains(text, "readiness-probe") || strings.Contains(text, "acceptance-db") {
		t.Fatalf("E01 must stay distinct from E02 and the live database: %s", text)
	}
}
