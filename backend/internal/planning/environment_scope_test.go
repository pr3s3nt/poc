package planning

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

// scopedAWSRequest is an aws-eks Environment of a new Application: its own
// ENVIRONMENT infrastructure scope (ADR-011), unlike the seeded fixtures.
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

func TestPlan_LegacyAWSEnvironmentKeepsApplicationIdentityAndNames(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend") // LEGACY_APPLICATION
	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	app := req.App.Key
	vpc, eks := nodeByType(t, plan, TypeVPC, ""), nodeByType(t, plan, TypeCluster, ClassEKS)
	if vpc.Descriptor != "vpc.default#applications."+app || eks.Descriptor != "k8s-cluster.eks#applications."+app || vpc.Scope.Type != resource.ScopeApplication || vpc.Scope.ID != app {
		t.Fatalf("legacy identity changed: %+v %+v", vpc, eks)
	}
	for _, definition := range []string{"vpc-aws", "cluster-aws-eks", "postgres-aws-aurora"} {
		if got, want := fmt.Sprint(contractValue(t, plan, definition, "name")), app+"-"+seedOptions().RunID; got != want {
			t.Fatalf("legacy %s name = %s, want %s", definition, got, want)
		}
	}
	// A second legacy Environment of the same Application reuses the same nodes.
	other := req
	other.Env.Key = "staging"
	other.Env.NamespaceIdentity = "app-" + app + "-staging"
	again, err := NewService().Plan(other)
	if err != nil {
		t.Fatal(err)
	}
	if nodeByType(t, again, TypeVPC, "").Descriptor != vpc.Descriptor {
		t.Fatal("legacy Environments of one Application must share the application VPC")
	}
}

func TestPlan_HardcodedApplicationInfrastructureReferenceFailsInNewScope(t *testing.T) {
	for scope, wantErr := range map[environment.InfrastructureScope]bool{environment.ScopeEnvironment: true, environment.ScopeLegacyApplication: false} {
		req := scopedAWSRequest(t, "pay", "staging")
		req.Env.InfrastructureScope = scope
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
		if scope == environment.ScopeLegacyApplication {
			req.App.Key = "pay"
			req.Env.ApplicationKey = "pay"
		}
		_, err := NewService().Plan(req)
		if wantErr {
			var staged *StageError
			if !errors.Is(err, ErrLegacyInfrastructureReference) || !errors.As(err, &staged) || staged.Reason != ReasonLegacyInfrastructure {
				t.Fatalf("new scope must reject the hardcoded reference with guidance: %v", err)
			}
			if message, ok := PublicMessage(err); !ok || !strings.Contains(message, "@infra") {
				t.Fatalf("public message = %q", message)
			}
			continue
		}
		if err != nil {
			t.Fatalf("legacy scope keeps old references valid: %v", err)
		}
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
	if base.Target.ConnectionKey != "internal-cluster" || base.Target.Profile != application.ProfileInternalK8s || base.Target.Scope != environment.ScopeLegacyApplication {
		t.Fatalf("plan target = %+v", base.Target)
	}
	again, _ := NewService().Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if again.PlanHash != base.PlanHash {
		t.Fatal("plan hash must be deterministic")
	}
	// Same graph, different pinned region or scope: the hash must change.
	for name, mutate := range map[string]func(*Request){
		"scope":  func(r *Request) { r.Env.InfrastructureScope = environment.ScopeEnvironment },
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

func TestContext_AppAliasesResolveTheSelectedEnvironment(t *testing.T) {
	req := scopedAWSRequest(t, "pay", "production")
	req.Env.Region = "eu-west-1"
	ctx := Context{OrganizationKey: req.OrganizationKey, App: req.App, Env: req.Env}
	values := ctx.Values()
	if values["app.profile"] != "aws-eks" || values["app.region"] != "eu-west-1" || values["env.profile"] != "aws-eks" || values["env.region"] != "eu-west-1" || values["infra.name"] != "pay-production" {
		t.Fatalf("context values = %v", values)
	}
	legacy := Context{App: req.App, Env: req.Env}
	legacy.Env.InfrastructureScope = environment.ScopeLegacyApplication
	if legacy.Values()["infra.name"] != "pay" {
		t.Fatalf("legacy infra.name = %v", legacy.Values()["infra.name"])
	}
	// The Application value object is never mutated by the projection.
	if req.App.Profile != "" || req.App.Region != "" {
		t.Fatalf("application leaked: %+v", req.App)
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

func TestResourceName_SamePrefixDifferentApplicationEnvironmentOrRunDoNotCollide(t *testing.T) {
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
	add("other run", uuidApp, "production", longRun+"2")
	add("shifted NUL boundary", uuidApp+"p", "roduction", longRun)
	// An environment key bound to another account/Connection keeps the same
	// Application, Environment and run identity, so only the key triple matters.
	a := resourceNameContext(uuidApp, "production", longRun, environment.ScopeEnvironment)
	b := a
	b.Env.ConnectionKey, b.Env.Region = "other-account", "eu-west-1"
	if a.ResourceName() != b.ResourceName() {
		t.Fatal("name must depend only on application, environment and run identity")
	}
}

func TestResourceName_LegacyApplicationNamesAreExactlyUnchanged(t *testing.T) {
	ctx := resourceNameContext("pay", "production", "run-test", environment.ScopeLegacyApplication)
	if got := ctx.ResourceName(); got != "pay-run-test" {
		t.Fatalf("legacy name = %s", got)
	}
	if ctx.Values()["infra.resourceName"] != "pay-run-test" || ctx.InfraName() != "pay" || ctx.InfraPath() != "applications.pay" {
		t.Fatalf("legacy identity changed: %v", ctx.Values())
	}
	// Scope and state identities of new Environments are unaffected by the name.
	scoped := resourceNameContext("pay", "production", "run-test", environment.ScopeEnvironment)
	if scoped.InfraPath() != "environments.pay.production" || scoped.InfraName() != "pay-production" {
		t.Fatalf("new scope identity = %s %s", scoped.InfraPath(), scoped.InfraName())
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
