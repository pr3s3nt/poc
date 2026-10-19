package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/placeholder"
)

// scopedAWSRequest is an aws-eks Environment of a new Application with its
// own ENVIRONMENT infrastructure scope (ADR-011).
func scopedAWSRequest(t *testing.T, appKey, envKey string) Request {
	t.Helper()
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	req.App.Key = appKey
	req.Env.ApplicationKey = appKey
	req.Env.Key = envKey
	req.Env.NamespaceIdentity = "app-" + appKey + "-" + envKey
	req.Env.InfrastructureScope = environment.ScopeEnvironment
	rescopeApplication(&req, testSeedCloudApp, appKey)
	return req
}

const testSeedCloudApp = "acceptance-cloud"

// rescopeApplication moves the seeded app_id criteria of from to the request's
// Application: V02 has no profile guard, so a different Application selects
// its Definitions through its own explicit app_id.
func rescopeApplication(req *Request, from, to string) {
	definitions := make([]resource.Definition, len(req.Catalog.Definitions))
	for i, def := range req.Catalog.Definitions {
		criteria := append([]resource.Criterion(nil), def.Criteria...)
		for j := range criteria {
			if criteria[j].ApplicationID == from {
				criteria[j].ApplicationID = to
			}
		}
		def.Criteria = criteria
		definitions[i] = def
	}
	req.Catalog.Definitions = definitions
}

func nodeByType(t *testing.T, plan *Plan, resourceType, class string) Node {
	t.Helper()
	for _, n := range plan.Graph.Nodes {
		if n.ResourceType == resourceType && (class == "" || n.Class == class) {
			return n
		}
	}
	t.Fatalf("no %s/%s node in %v", resourceType, class, plan.Graph.Nodes)
	return Node{}
}

func contractValue(t *testing.T, plan *Plan, definitionKey, input string) any {
	t.Helper()
	for _, c := range plan.Terraform {
		if c.DefinitionKey != definitionKey {
			continue
		}
		for _, in := range c.Inputs {
			if in.Name == input {
				return in.Value
			}
		}
	}
	t.Fatalf("contract %s has no input %s", definitionKey, input)
	return nil
}

func TestPlan_RejectsEveryNonEnvironmentScopeWithoutEchoingIt(t *testing.T) {
	for _, bad := range []environment.InfrastructureScope{"LEGACY_APPLICATION", "APPLICATION", "Env-Scope", "Prod"} {
		for _, profile := range []application.ExecutionProfile{application.ProfileAWSEKS, application.ProfileInternalK8s} {
			req := testRequest(t, profile, "backend")
			req.Env.InfrastructureScope = bad
			plan, err := NewService(testBundles()).Plan(req)
			if plan != nil || !errors.Is(err, environment.ErrInvalidInfrastructureScope) || strings.Contains(err.Error(), string(bad)) {
				t.Fatalf("%s scope %q: plan=%v err=%v", profile, bad, plan, err)
			}
		}
		// An unconfigured snapshot cannot hide the bad scope behind the default.
		req := testRequest(t, application.ProfileInternalK8s, "backend")
		req.Env.ConnectionKey, req.Env.Profile, req.Env.RuntimeStatus = "", "", ""
		req.Env.InfrastructureScope = bad
		if _, err := NewService(testBundles()).Plan(req); !errors.Is(err, environment.ErrInvalidInfrastructureScope) || errors.Is(err, ErrEnvironmentUnconfigured) {
			t.Fatalf("unconfigured scope %q: %v", bad, err)
		}
	}
	// An unset scope on an Environment without a deployment Connection keeps the
	// documented default; the Environment is Definition-placed (ADR-016).
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.Env.InfrastructureScope = ""
	plan, err := NewService(testBundles()).Plan(req)
	if err != nil || plan.Target.Scope != environment.ScopeEnvironment {
		t.Fatalf("unset scope: plan=%v err=%v", plan, err)
	}
}

func TestPlan_EnvironmentWithoutConnectionIsDefinitionPlacedNotUnconfigured(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	var defs []resource.Definition
	for _, def := range req.Catalog.Definitions {
		if def.ResourceTypeKey != TypeWorkload {
			defs = append(defs, def)
		}
	}
	req.Catalog.Definitions = defs
	_, err := NewService(testBundles()).Plan(req)
	var staged *StageError
	if errors.Is(err, ErrEnvironmentUnconfigured) || !errors.As(err, &staged) || staged.Reason != ReasonPlacement {
		t.Fatalf("a missing Environment Connection must not block planning; the missing workload Definition does: %v", err)
	}
	if message, ok := PublicMessage(err); !ok || strings.Contains(message, "Environment Settings") || !strings.Contains(message, "Resource Definitions") {
		t.Fatalf("public message = %q", message)
	}
}

func TestPlan_HashPinsTheNonSecretTarget(t *testing.T) {
	base, err := NewService(testBundles()).Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatal(err)
	}
	if base.Target.ConnectionKey != "" || base.Target.Profile != "" || base.Target.Scope != environment.ScopeEnvironment {
		t.Fatalf("plan target = %+v", base.Target)
	}
	again, _ := NewService(testBundles()).Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if again.PlanHash != base.PlanHash {
		t.Fatal("plan hash must be deterministic")
	}
	// Same graph, different pinned region: the hash must change.
	for name, mutate := range map[string]func(*Request){
		"region": func(r *Request) { r.Env.Region = "eu-central-1" },
	} {
		req := testRequest(t, application.ProfileInternalK8s, "backend")
		mutate(&req)
		changed, err := NewService(testBundles()).Plan(req)
		if err != nil {
			t.Fatal(err)
		}
		if changed.PlanHash == base.PlanHash {
			t.Fatalf("%s must change the plan hash", name)
		}
	}
}

func TestContext_ValuesResolveTheSelectedEnvironment(t *testing.T) {
	req := scopedAWSRequest(t, "pay", "production")
	req.Env.Region = "eu-west-1"
	ctx := Context{OrganizationKey: req.OrganizationKey, App: req.App, Env: req.Env}
	values := ctx.Values()
	if values["env.profile"] != "aws-eks" || values["env.region"] != "eu-west-1" || values["infra.name"] != "pay-production" {
		t.Fatalf("context values = %v", values)
	}
}

func TestContext_AppProfileRegionAliasesAreUnknown(t *testing.T) {
	req := scopedAWSRequest(t, "pay", "production")
	req.Env.Region = "eu-west-1"
	ctx := Context{OrganizationKey: req.OrganizationKey, App: req.App, Env: req.Env}
	for _, key := range []string{"app.profile", "app.region"} {
		if _, ok := ctx.Values()[key]; ok {
			t.Fatalf("%s must be absent from context values", key)
		}
		if v, err := ctx.ResolveContext(key); err == nil || !strings.Contains(err.Error(), "unknown context value") {
			t.Fatalf("ResolveContext(%q) = %v, %v; want unknown error", key, v, err)
		}
	}
	for _, in := range []string{"${context.app.profile}", "${context.app.region}", "region=${context.app.region}-x", "${context.env.region}/${context.app.profile}"} {
		if out, err := placeholder.ExpandString(in, ctx); err == nil {
			t.Fatalf("ExpandString(%q) = %v; want error", in, out)
		}
	}
}

func TestContext_EnvProfileRegionComeFromEnvironment(t *testing.T) {
	req := scopedAWSRequest(t, "pay", "production")
	req.Env.Region = "eu-west-1"
	ctx := Context{OrganizationKey: req.OrganizationKey, App: req.App, Env: req.Env}
	cases := map[string]string{
		"${context.env.profile}":                       "aws-eks",
		"${context.env.region}":                        "eu-west-1",
		"${context.env.profile}-${context.env.region}": "aws-eks-eu-west-1",
		"${context.app.id}":                            "pay",
		"${context.app.name}":                          req.App.Name,
		"${context.infra.name}":                        "pay-production",
	}
	for in, want := range cases {
		got, err := placeholder.ExpandString(in, ctx)
		if err != nil || got != want {
			t.Fatalf("ExpandString(%q) = %v, %v; want %q", in, got, err, want)
		}
	}
	// A second Environment of the same Application decides independently.
	other := ctx
	other.Env.Key, other.Env.Region = "staging", "eu-central-1"
	if got, err := placeholder.ExpandString("${context.env.region}", other); err != nil || got != "eu-central-1" {
		t.Fatalf("second environment region = %v, %v", got, err)
	}
	if got, _ := placeholder.ExpandString("${context.env.region}", ctx); got != "eu-west-1" {
		t.Fatalf("first environment region changed: %v", got)
	}
}

const (
	uuidApp = "7f3c9a52-1b8e-4d6f-9a3c-2e5b8d1f4a60"
	longRun = "envconn-kind-20261007121743-27878-with-a-very-long-suffix"
)

func resourceNameContext(app, env, run string, scope environment.InfrastructureScope) Context {
	return Context{
		App:   application.Application{Key: app},
		Env:   environment.Environment{ApplicationKey: app, Key: env, InfrastructureScope: scope},
		RunID: run,
	}
}

func TestResourceName_NewScopeIsBoundedProviderSafeAndDeterministic(t *testing.T) {
	pattern := regexp.MustCompile(`^orch-[a-z0-9]+-[a-z0-9]+-[0-9a-f]{24}$`)
	for _, env := range []string{"production", "staging", "a-very-long-environment-key-name", "---", "Prod_1"} {
		name := resourceNameContext(uuidApp, env, longRun, environment.ScopeEnvironment).ResourceName()
		if len(name) > 49 || !pattern.MatchString(name) || strings.Contains(name, "--") || strings.HasSuffix(name, "-") {
			t.Fatalf("env %q name %q violates the bounded pattern", env, name)
		}
		// Module suffixes stay inside the IAM role (64) and Aurora cluster (63) limits.
		for _, suffix := range []string{"-writer", "-cluster"} {
			if len(name+suffix) > 63 {
				t.Fatalf("%s%s is %d characters", name, suffix, len(name+suffix))
			}
		}
		if again := resourceNameContext(uuidApp, env, longRun, environment.ScopeEnvironment).ResourceName(); again != name {
			t.Fatalf("name is not deterministic: %s != %s", again, name)
		}
	}
	// Digit-leading UUID keys still start with a letter; empty prefixes fall back.
	if name := resourceNameContext("1234", "!!!", "r", environment.ScopeEnvironment).ResourceName(); !strings.HasPrefix(name, "orch-1234-env-") {
		t.Fatalf("fallback name = %s", name)
	}
	if name := resourceNameContext("---", "x", "r", environment.ScopeEnvironment).ResourceName(); !strings.HasPrefix(name, "orch-app-x-") {
		t.Fatalf("app fallback name = %s", name)
	}
}

func TestResourceName_SamePrefixDifferentApplicationOrEnvironmentDoNotCollide(t *testing.T) {
	names := map[string]string{}
	add := func(label, app, env, run string) {
		name := resourceNameContext(app, env, run, environment.ScopeEnvironment).ResourceName()
		if other, dup := names[name]; dup {
			t.Fatalf("%s collides with %s: %s", label, other, name)
		}
		names[name] = label
	}
	add("base", uuidApp, "production", longRun)
	add("other app, same 8 prefix", "7f3c9a52-ffff-4d6f-9a3c-000000000000", "production", longRun)
	add("other env, same 10 prefix", uuidApp, "productionX", longRun)
	add("shifted NUL boundary", uuidApp+"p", "roduction", longRun)
	// An environment key bound to another account/Connection keeps the same
	// Application, Environment and run identity, so only the key triple matters.
	a := resourceNameContext(uuidApp, "production", longRun, environment.ScopeEnvironment)
	b := a
	b.Env.ConnectionKey, b.Env.Region = "other-account", "eu-west-1"
	if a.ResourceName() != b.ResourceName() {
		t.Fatal("name must depend only on application, environment and target generation")
	}
}

// Generation 0 keeps its current identity exactly; later generations never
// reuse a provider-visible name, descriptor path or scope of another one.
// The run ID is provenance only: a later run (a redeploy) must produce the
// same provider-visible name, or every redeploy would replace its objects.
func TestResourceName_IsIndependentOfRunID(t *testing.T) {
	a := resourceNameContext(uuidApp, "production", "run-a", environment.ScopeEnvironment)
	b := resourceNameContext(uuidApp, "production", "run-b-with-another-id", environment.ScopeEnvironment)
	if a.ResourceName() != b.ResourceName() {
		t.Fatalf("run ID changed the physical name: %s != %s", a.ResourceName(), b.ResourceName())
	}
}

func TestResourceName_GenerationZeroIdentityUnchangedAndLaterGenerationsIsolated(t *testing.T) {
	ctx := resourceNameContext("pay", "production", "run-test", environment.ScopeEnvironment)
	sum := sha256.Sum256([]byte("pay\x00production"))
	wantName := "orch-pay-production-" + hex.EncodeToString(sum[:])[:24]
	if got := ctx.ResourceName(); got != wantName || ctx.Values()["infra.resourceName"] != wantName {
		t.Fatalf("generation 0 name = %s, want %s", ctx.ResourceName(), wantName)
	}
	if ctx.InfraName() != "pay-production" || ctx.InfraPath() != "environments.pay.production" {
		t.Fatalf("generation 0 identity = %s %s", ctx.InfraName(), ctx.InfraPath())
	}
	// An unset scope reads as the same ENVIRONMENT identity.
	unset := resourceNameContext("pay", "production", "run-test", "")
	if unset.ResourceName() != wantName || unset.InfraPath() != ctx.InfraPath() {
		t.Fatalf("unset scope identity = %s %s", unset.ResourceName(), unset.InfraPath())
	}
	names, paths := map[string]int64{}, map[string]int64{}
	for generation := int64(0); generation <= 5; generation++ {
		c := ctx
		c.Env.TargetGeneration = generation
		name, path := c.ResourceName(), c.InfraPath()
		if other, dup := names[name]; dup {
			t.Fatalf("generation %d name collides with %d: %s", generation, other, name)
		}
		if other, dup := paths[path]; dup {
			t.Fatalf("generation %d path collides with %d: %s", generation, other, path)
		}
		names[name], paths[path] = generation, generation
		if generation > 0 && path != "environments.pay.production"+environment.GenerationScopeSuffix(generation) {
			t.Fatalf("generation %d path = %s", generation, path)
		}
	}
}
