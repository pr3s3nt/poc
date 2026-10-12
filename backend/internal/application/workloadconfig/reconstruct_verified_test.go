package workloadconfig

import (
	"errors"
	"reflect"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

func pgTypes() map[string]resource.Type {
	return map[string]resource.Type{"postgres": {Key: "postgres", Inputs: []resource.InputField{{Name: "database", Type: "string"}}, Outputs: []resource.OutputField{{Name: "host", Type: "string", Required: true}}}}
}

func moduleWith(vars map[string]string, externals map[string]environment.ResourceEntry) environment.Module {
	return environment.Module{Profile: environment.ModuleProfile,
		Spec:      environment.ModuleSpec{Containers: map[string]environment.Container{"web": {ID: "web", Image: "i", Variables: vars}}},
		Externals: externals}
}

func TestReconstructVerifiedLosslessAndRoundTrips(t *testing.T) {
	set := environment.NewDocument()
	set.Shared["db"] = environment.ResourceEntry{Type: "postgres", Class: "default", Params: map[string]any{"database": "${context.uc12.K}"}}
	module := moduleWith(map[string]string{"A": "${context.uc12.K}", "H": "${shared.db.host}", "E": "${externals.own.host}"},
		map[string]environment.ResourceEntry{"own": {Type: "postgres", Class: "default", Params: map[string]any{"database": "${context.uc12.K}"}}})
	set.Modules["api"] = module
	raw, err := ReconstructVerified("api", module, set, pgTypes())
	if err != nil {
		t.Fatal(err)
	}
	resources := raw["resources"].(map[string]any)
	if resources["own"].(map[string]any)["params"].(map[string]any)["database"] != "${resources.env.K}" ||
		resources["shared_1"].(map[string]any)["params"].(map[string]any)["database"] != "${resources.env.K}" {
		t.Fatalf("params not mapped back: %v", resources)
	}
	rewritten, changed, err := RewriteEnvironmentKey(raw, "K", "N")
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	if err := VerifyRewritten("api", rewritten, module, set, pgTypes(), "K", "N"); err != nil {
		t.Fatal(err)
	}
	if !ModuleReferencesKey(module, set, "K") || ModuleReferencesKey(module, set, "Z") {
		t.Fatal("reference discovery")
	}
	// The input tree is never mutated.
	if !ReferencesEnvironmentKey(raw, "K") || ReferencesEnvironmentKey(rewritten, "K") {
		t.Fatal("rewrite mutated its input or missed a token")
	}
}

func TestReconstructVerifiedFailsClosed(t *testing.T) {
	ok := func() environment.Document {
		set := environment.NewDocument()
		set.Modules["api"] = moduleWith(map[string]string{"A": "${context.uc12.K}"}, nil)
		return set
	}
	cases := map[string]func(*environment.Document){
		"orphan shared": func(d *environment.Document) {
			d.Shared["orphan"] = environment.ResourceEntry{Type: "postgres", Class: "default"}
		},
		"literal": func(d *environment.Document) {
			m := d.Modules["api"]
			m.Spec.Containers["web"].Variables["L"] = "plain"
			d.Modules["api"] = m
		},
		"alias collision": func(d *environment.Document) {
			m := d.Modules["api"]
			m.Externals = map[string]environment.ResourceEntry{"env": {Type: "postgres", Class: "default"}}
			d.Modules["api"] = m
		},
		"service alias collision": func(d *environment.Document) {
			m := d.Modules["api"]
			m.Spec.Containers["web"].Variables["S"] = "${context.service.peer.http}"
			m.Externals = map[string]environment.ResourceEntry{"svc_peer_http": {Type: "postgres", Class: "default"}}
			d.Modules["api"] = m
		},
		"dangling external": func(d *environment.Document) {
			m := d.Modules["api"]
			m.Spec.Containers["web"].Variables["X"] = "${externals.nope.host}"
			d.Modules["api"] = m
		},
		"lossy replicas": func(d *environment.Document) {
			zero := 0
			m := d.Modules["api"]
			m.Spec.Replicas = &zero
			d.Modules["api"] = m
		},
	}
	for name, mutate := range cases {
		set := ok()
		mutate(&set)
		if _, err := ReconstructVerified("api", set.Modules["api"], set, pgTypes()); !errors.Is(err, ErrReferenceRewriteUnsafe) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	set := ok()
	if _, err := ReconstructVerified("api", set.Modules["api"], set, pgTypes()); err != nil {
		t.Fatal(err)
	}
}

func TestRewriteOnlyTouchesEnvironmentAliasesAndExactKeys(t *testing.T) {
	raw := map[string]any{
		"containers": map[string]any{"web": map[string]any{"variables": map[string]any{
			"A": "${resources.env.K}", "B": "${resources.other.K}", "C": "${resources.env.KK}", "D": "pre-${resources.env.K}-post", "E": "$${resources.env.K}", "F": "${ resources.env.K }"}}},
		"resources": map[string]any{"env": map[string]any{"type": "environment"}, "other": map[string]any{"type": "postgres"}},
	}
	got, changed, err := RewriteEnvironmentKey(raw, "K", "N")
	if err != nil || !changed {
		t.Fatal(err)
	}
	vars := got["containers"].(map[string]any)["web"].(map[string]any)["variables"].(map[string]any)
	want := map[string]any{"A": "${resources.env.N}", "B": "${resources.other.K}", "C": "${resources.env.KK}", "D": "pre-${resources.env.N}-post", "E": "$${resources.env.K}", "F": "${ resources.env.N }"}
	if !reflect.DeepEqual(vars, want) {
		t.Fatalf("%v", vars)
	}
	if _, changed, _ := RewriteEnvironmentKey(raw, "ZZ", "N"); changed {
		t.Fatal("unrelated key reported changed")
	}
}

func TestReconstructVerifiedKeepsAdvancedFieldsServiceRefsAndSharedParams(t *testing.T) {
	replicas := 3
	set := environment.NewDocument()
	set.Shared["db"] = environment.ResourceEntry{Type: "postgres", Class: "gold", Params: map[string]any{"database": "${context.uc12.K}"}}
	module := environment.Module{Profile: environment.ModuleProfile,
		Spec: environment.ModuleSpec{
			Containers: map[string]environment.Container{"web": {ID: "web", Image: "img:1", Command: []string{"run"}, Args: []string{"--x"},
				Variables:      map[string]string{"A": "${context.uc12.K}", "H": "${shared.db.host}", "S": "${context.service.peer.http}", "O": "${externals.own.host}"},
				Resources:      &environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "100m"}, Limits: &environment.ComputeResources{Memory: "64Mi"}},
				LivenessProbe:  &environment.Probe{Path: "/live", Port: 8080},
				ReadinessProbe: &environment.Probe{Path: "/ready", Port: 8080}}},
			Service:  &environment.Service{Ports: map[string]environment.Port{"http": {Port: 80, TargetPort: 8080, Protocol: "TCP"}}, PublicRoutes: []environment.PublicPath{{Path: "/api", Port: "http"}}},
			Replicas: &replicas},
		Externals: map[string]environment.ResourceEntry{"own": {Type: "postgres", Class: "default", Params: map[string]any{"database": "keep"}}}}
	set.Modules["api"] = module
	raw, err := ReconstructVerified("api", module, set, pgTypes())
	if err != nil {
		t.Fatal(err)
	}
	rewritten, changed, err := RewriteEnvironmentKey(raw, "K", "N")
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	if err := VerifyRewritten("api", rewritten, module, set, pgTypes(), "K", "N"); err != nil {
		t.Fatal(err)
	}
	web := rewritten["containers"].(map[string]any)["web"].(map[string]any)
	for _, field := range []string{"command", "args", "resources", "livenessProbe", "readinessProbe"} {
		if web[field] == nil {
			t.Errorf("advanced field %s dropped", field)
		}
	}
	vars := web["variables"].(map[string]any)
	if vars["A"] != "${resources.env.N}" || vars["S"] != "${resources.svc_peer_http.url}" || vars["O"] != "${resources.own.host}" || vars["H"] != "${resources.shared_1.host}" {
		t.Fatalf("variables %v", vars)
	}
	if rewritten["replicas"] != float64(3) || rewritten["service"].(map[string]any)["publicRoutes"] == nil {
		t.Fatalf("service/replicas %v", rewritten)
	}
	res := rewritten["resources"].(map[string]any)
	if res["shared_1"].(map[string]any)["class"] != "gold" || res["shared_1"].(map[string]any)["id"] != "db" ||
		res["shared_1"].(map[string]any)["params"].(map[string]any)["database"] != "${resources.env.N}" ||
		res["svc_peer_http"].(map[string]any)["params"].(map[string]any)["workload"] != "peer" {
		t.Fatalf("resources %v", res)
	}
}

// Ordinary context references in resource params (for example
// ${context.env.namespace}) are valid Score content: Score conversion leaves
// them unchanged, so reconstruction, redeploy and T30 rewriting keep them.
func TestReconstructKeepsOrdinaryContextRefsInResourceParams(t *testing.T) {
	set := environment.NewDocument()
	set.Shared["db"] = environment.ResourceEntry{Type: "postgres", Class: "default", Params: map[string]any{"database": "${context.env.region}-${context.uc12.K}"}}
	module := moduleWith(map[string]string{"A": "${context.uc12.K}", "H": "${shared.db.host}", "E": "${externals.own.host}"},
		map[string]environment.ResourceEntry{"own": {Type: "postgres", Class: "default", Params: map[string]any{"database": "${context.env.namespace}"}}})
	set.Modules["api"] = module
	for name, build := range map[string]func() (map[string]any, error){
		"edit":     func() (map[string]any, error) { return ReconstructScore("api", module, set) },
		"redeploy": func() (map[string]any, error) { return ReconstructForRedeploy("api", module, set) },
		"verified": func() (map[string]any, error) { return ReconstructVerified("api", module, set, pgTypes()) },
	} {
		raw, err := build()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		res := raw["resources"].(map[string]any)
		params := res["own"].(map[string]any)["params"].(map[string]any)
		if params["database"] != "${context.env.namespace}" {
			t.Fatalf("%s: ordinary refs changed: %v", name, params)
		}
		if got := res["shared_1"].(map[string]any)["params"].(map[string]any)["database"]; got != "${context.env.region}-${resources.env.K}" {
			t.Fatalf("%s: shared param %v", name, got)
		}
	}
	raw, err := ReconstructVerified("api", module, set, pgTypes())
	if err != nil {
		t.Fatal(err)
	}
	rewritten, changed, err := RewriteEnvironmentKey(raw, "K", "N")
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	if err := VerifyRewritten("api", rewritten, module, set, pgTypes(), "K", "N"); err != nil {
		t.Fatal(err)
	}
	if rewritten["resources"].(map[string]any)["own"].(map[string]any)["params"].(map[string]any)["database"] != "${context.env.namespace}" {
		t.Fatal("unrelated context ref dropped by the rewrite")
	}
	// A container variable with an ordinary context ref is still rejected (not an editor-safe reference).
	bad := moduleWith(map[string]string{"A": "${context.env.namespace}"}, nil)
	if _, err := ReconstructScore("api", bad, environment.NewDocument()); err == nil {
		t.Fatal("container variable context literal accepted")
	}
	if _, err := ReconstructForRedeploy("api", bad, environment.NewDocument()); err == nil {
		t.Fatal("redeploy accepted unsupported context binding in a variable")
	}
}

func TestVerifyRewrittenHonorsEscapedPlaceholders(t *testing.T) {
	set := environment.NewDocument()
	module := moduleWith(map[string]string{"A": "${context.uc12.K}"},
		map[string]environment.ResourceEntry{"own": {Type: "postgres", Class: "default", Params: map[string]any{"database": "$${context.uc12.K}-${context.uc12.K}"}}})
	set.Modules["api"] = module
	raw, err := ReconstructVerified("api", module, set, pgTypes())
	if err != nil {
		t.Fatal(err)
	}
	rewritten, changed, err := RewriteEnvironmentKey(raw, "K", "N")
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	got := rewritten["resources"].(map[string]any)["own"].(map[string]any)["params"].(map[string]any)["database"]
	if got != "$${context.uc12.K}-${resources.env.N}" {
		t.Fatalf("escaped literal not preserved exactly: %v", got)
	}
	if err := VerifyRewritten("api", rewritten, module, set, pgTypes(), "K", "N"); err != nil {
		t.Fatalf("safe rewrite falsely rejected: %v", err)
	}
	// A rewrite that also touched the escaped literal is NOT the expected change.
	tampered, _ := plainTree(rewritten)
	tampered["resources"].(map[string]any)["own"].(map[string]any)["params"].(map[string]any)["database"] = "$${context.uc12.N}-${resources.env.N}"
	if err := VerifyRewritten("api", tampered, module, set, pgTypes(), "K", "N"); !errors.Is(err, ErrReferenceRewriteUnsafe) {
		t.Fatalf("tampered escape accepted: %v", err)
	}
}
