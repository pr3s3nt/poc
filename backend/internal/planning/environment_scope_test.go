package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	return req
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

func TestPlan_NewAWSEnvironmentsGetDistinctInfrastructureIdentity(t *testing.T) {
	staging, err := NewService().Plan(scopedAWSRequest(t, "pay", "staging"))
	if err != nil {
		t.Fatal(err)
	}
	production, err := NewService().Plan(scopedAWSRequest(t, "pay", "production"))
	if err != nil {
		t.Fatal(err)
	}
	for envKey, plan := range map[string]*Plan{"staging": staging, "production": production} {
		vpc, eks := nodeByType(t, plan, TypeVPC, ""), nodeByType(t, plan, TypeCluster, ClassEKS)
		if vpc.Descriptor != "vpc.default#environments.pay."+envKey || eks.Descriptor != "k8s-cluster.eks#environments.pay."+envKey {
			t.Fatalf("%s descriptors = %s %s", envKey, vpc.Descriptor, eks.Descriptor)
		}
		if vpc.Scope.Type != resource.ScopeEnvironment || vpc.Scope.ID != "pay."+envKey || eks.Scope.Type != resource.ScopeEnvironment {
			t.Fatalf("%s scope = %+v %+v", envKey, vpc.Scope, eks.Scope)
		}
		// Cloud names isolate Environments sharing one account and region.
		for _, definition := range []string{"vpc-aws", "cluster-aws-eks"} {
			want := Context{App: scopedAWSRequest(t, "pay", envKey).App, Env: scopedAWSRequest(t, "pay", envKey).Env, RunID: seedOptions().RunID}.ResourceName()
			if got := contractValue(t, plan, definition, "name"); got != want {
				t.Fatalf("%s %s name = %v, want %s", envKey, definition, got, want)
			}
		}
	}
	if staging.PlanHash == production.PlanHash {
		t.Fatal("distinct Environments must not share a plan hash")
	}
	// The EKS consumer reaches the VPC through @infra, not the legacy path.
	eks := nodeByType(t, staging, TypeCluster, ClassEKS)
	edges := 0
	for _, e := range staging.Graph.Edges {
		if e.Consumer == eks.Descriptor && e.Provider == "vpc.default#environments.pay.staging" {
			edges++
		}
	}
	if edges == 0 {
		t.Fatalf("EKS must depend on its own Environment VPC: %v", staging.Graph.Edges)
	}
	for _, n := range staging.Graph.Nodes {
		if strings.Contains(n.Descriptor, "#applications.") {
			t.Fatalf("new scope must not create an application-scoped node: %s", n.Descriptor)
		}
	}
}

func TestPlan_AuroraResolvesInfrastructureGraphThroughInfraToken(t *testing.T) {
	req := scopedAWSRequest(t, "pay", "staging")
	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	aurora := nodeByType(t, plan, "postgres", "")
	if plan.Matches[aurora.Descriptor].DefinitionKey != "postgres-aws-aurora" {
		t.Fatalf("aurora match = %+v", plan.Matches[aurora.Descriptor])
	}
	found := false
	for _, e := range plan.Graph.Edges {
		if e.Consumer == aurora.Descriptor && e.Provider == "vpc.default#environments.pay.staging" {
			found = true
		}
		if e.Consumer == aurora.Descriptor && strings.Contains(e.Provider, "applications.") {
			t.Fatalf("aurora depends on the legacy VPC: %v", e)
		}
	}
	if !found {
		t.Fatalf("aurora must depend on the Environment VPC: %v", plan.Graph.Edges)
	}
	if name := fmt.Sprint(contractValue(t, plan, "postgres-aws-aurora", "name")); name != (Context{App: req.App, Env: req.Env, RunID: seedOptions().RunID}).ResourceName() || !strings.HasPrefix(name, "orch-pay-staging-") {
		t.Fatalf("aurora name = %s", name)
	}
}

func TestPlan_RejectsEveryNonEnvironmentScopeWithoutEchoingIt(t *testing.T) {
	for _, bad := range []environment.InfrastructureScope{"LEGACY_APPLICATION", "APPLICATION", "Env-Scope", "Prod"} {
		for _, profile := range []application.ExecutionProfile{application.ProfileAWSEKS, application.ProfileInternalK8s} {
			req := testRequest(t, profile, "backend")
			req.Env.InfrastructureScope = bad
			plan, err := NewService().Plan(req)
			if plan != nil || !errors.Is(err, environment.ErrInvalidInfrastructureScope) || strings.Contains(err.Error(), string(bad)) {
				t.Fatalf("%s scope %q: plan=%v err=%v", profile, bad, plan, err)
			}
		}
		// An unconfigured snapshot cannot hide the bad scope behind the default.
		req := testRequest(t, application.ProfileInternalK8s, "backend")
		req.Env.ConnectionKey, req.Env.Profile, req.Env.RuntimeStatus = "", "", ""
		req.Env.InfrastructureScope = bad
		if _, err := NewService().Plan(req); !errors.Is(err, environment.ErrInvalidInfrastructureScope) || errors.Is(err, ErrEnvironmentUnconfigured) {
			t.Fatalf("unconfigured scope %q: %v", bad, err)
		}
	}
	// An unset scope on an unconfigured Environment keeps the documented default.
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.Env.ConnectionKey, req.Env.Profile, req.Env.RuntimeStatus, req.Env.InfrastructureScope = "", "", "", ""
	if _, err := NewService().Plan(req); !errors.Is(err, ErrEnvironmentUnconfigured) {
		t.Fatalf("unset scope: %v", err)
	}
}

func TestPlan_HardcodedApplicationInfrastructureReferenceFails(t *testing.T) {
	req := scopedAWSRequest(t, "pay", "staging")
	defs := append([]resource.Definition(nil), req.Catalog.Definitions...)
	for i, def := range defs {
		if def.Key != "cluster-aws-eks" {
			continue
		}
		variables := map[string]any{}
		for k, v := range def.Variables() {
			variables[k] = v
		}
		variables["subnet_ids"] = "${resources['vpc.default#applications.pay'].outputs.subnetIds}"
		defs[i].DriverInputs = map[string]any{"values": map[string]any{"source": def.DriverValues()["source"], "variables": variables}}
	}
	req.Catalog.Definitions = defs
	_, err := NewService().Plan(req)
	var staged *StageError
	if !errors.Is(err, ErrLegacyInfrastructureReference) || !errors.As(err, &staged) || staged.Reason != ReasonLegacyInfrastructure {
		t.Fatalf("the hardcoded reference must be rejected with guidance: %v", err)
	}
	if message, ok := PublicMessage(err); !ok || !strings.Contains(message, "@infra") {
		t.Fatalf("public message = %q", message)
	}
}

func TestPlan_UnconfiguredEnvironmentFailsBeforeAnyPlanning(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.Env.ConnectionKey, req.Env.Profile, req.Env.RuntimeStatus = "", "", ""
	_, err := NewService().Plan(req)
	if !errors.Is(err, ErrEnvironmentUnconfigured) {
		t.Fatalf("unconfigured plan: %v", err)
	}
	if message, ok := PublicMessage(err); !ok || !strings.Contains(message, "Environment Settings") {
		t.Fatalf("public message = %q", message)
	}
}

func TestPlan_HashPinsTheNonSecretTarget(t *testing.T) {
	base, err := NewService().Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatal(err)
	}
	if base.Target.ConnectionKey != "internal-cluster" || base.Target.Profile != application.ProfileInternalK8s || base.Target.Scope != environment.ScopeEnvironment {
		t.Fatalf("plan target = %+v", base.Target)
	}
	again, _ := NewService().Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if again.PlanHash != base.PlanHash {
		t.Fatal("plan hash must be deterministic")
	}
	// Same graph, different pinned region: the hash must change.
	for name, mutate := range map[string]func(*Request){
		"region": func(r *Request) { r.Env.Region = "eu-central-1" },
	} {
		req := testRequest(t, application.ProfileInternalK8s, "backend")
		mutate(&req)
		changed, err := NewService().Plan(req)
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

func TestConnectionMismatchComparesTheEnvironmentBinding(t *testing.T) {
	def := resource.Definition{Key: "cluster-x", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, ConnectionKey: "other"}
	k8s := environment.Environment{Key: "staging", ApplicationKey: "a", ConnectionKey: "lab", Profile: application.ProfileInternalK8s}
	if !ConnectionMismatch(k8s, def) {
		t.Fatal("a cluster Definition for another Connection must mismatch")
	}
	k8s.ConnectionKey = "other"
	if ConnectionMismatch(k8s, def) {
		t.Fatal("the selected Connection must match")
	}
	// Two Environments of one Application decide independently.
	aws := environment.Environment{Key: "production", ApplicationKey: "a", ConnectionKey: "cloud", Profile: application.ProfileAWSEKS, Region: "eu-west-1"}
	vpc := resource.Definition{Key: "vpc-x", ResourceTypeKey: TypeVPC, DriverType: resource.DriverTerraform, ConnectionKey: "cloud-b"}
	if !ConnectionMismatch(aws, vpc) {
		t.Fatal("a VPC Definition for another account must mismatch")
	}
	db := resource.Definition{Key: "db", ResourceTypeKey: "postgres", DriverType: resource.DriverTerraform, ConnectionKey: "cloud-b"}
	if ConnectionMismatch(aws, db) {
		t.Fatal("external database Definitions keep their Driver Account")
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

func TestPlan_RealUUIDLongRunNamesAreBoundedInEveryAWSDefinition(t *testing.T) {
	req := scopedAWSRequest(t, uuidApp, "production")
	req.RunID = longRun
	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	want := resourceNameContext(uuidApp, "production", longRun, environment.ScopeEnvironment).ResourceName()
	for _, definition := range []string{"vpc-aws", "cluster-aws-eks", "postgres-aws-aurora"} {
		got := fmt.Sprint(contractValue(t, plan, definition, "name"))
		if got != want || len(got) > 49 || len(got+"-cluster") > 63 || len(got+"-writer") > 63 || got[0] < 'a' || got[0] > 'z' {
			t.Fatalf("%s name = %q, want %q", definition, got, want)
		}
	}
	// Descriptors and scopes stay the readable full-key identity, not the hash.
	vpc := nodeByType(t, plan, TypeVPC, "")
	if vpc.Descriptor != "vpc.default#environments."+uuidApp+".production" || vpc.Scope.ID != uuidApp+".production" {
		t.Fatalf("identity changed: %+v", vpc)
	}
}
