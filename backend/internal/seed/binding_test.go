package seed_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

func TestApplyBindsAcceptanceFixturesAsLockedLegacyAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	opts.Region = "us-east-1"
	for i := 0; i < 2; i++ {
		if err := seed.Apply(ctx, st, opts); err != nil {
			t.Fatal(err)
		}
	}
	k8s, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || k8s.ConnectionKey != opts.ConnectionKey || k8s.Profile != application.ProfileInternalK8s || k8s.InfrastructureScope != environment.ScopeLegacyApplication || k8s.Version != 2 {
		t.Fatalf("acceptance env = %+v %v", k8s, err)
	}
	aws, err := st.GetEnvironment(ctx, opts.CloudApplicationKey, opts.EnvironmentKey)
	if err != nil || aws.ConnectionKey != opts.CloudConnectionKey || aws.Region != "us-east-1" || aws.RuntimeStatus != application.RuntimePending || aws.InfrastructureScope != environment.ScopeLegacyApplication {
		t.Fatalf("acceptance-cloud env = %+v %v", aws, err)
	}
	// The seeded Applications carry no target of their own.
	if a, _ := st.GetApplication(ctx, opts.ApplicationKey); a.ConnectionKey != "" || a.Profile != "" {
		t.Fatalf("seeded application leaked a target: %+v", a)
	}
}

func definitionByKey(t *testing.T, st *store.Store, org, key string) resource.Definition {
	t.Helper()
	defs, err := st.ListResourceDefinitions(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if d.Key == key {
			return d
		}
	}
	t.Fatalf("definition %s not found", key)
	return resource.Definition{}
}

func TestSeededAWSDefinitionsUseInfraTokenAndResourceName(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"vpc-aws", "cluster-aws-eks", "postgres-aws-aurora"} {
		text := toString(definitionByKey(t, st, opts.OrganizationKey, key).DriverInputs)
		if strings.Contains(text, "applications.@app") || strings.Contains(text, "context.app.id") || strings.Contains(text, "context.infra.name") || strings.Contains(text, "context.run.id") {
			t.Fatalf("%s still hardcodes the application path or an unbounded name: %s", key, text)
		}
		if !strings.Contains(text, "${context.infra.resourceName}") {
			t.Fatalf("%s must name through context.infra.resourceName: %s", key, text)
		}
	}
	if !strings.Contains(toString(definitionByKey(t, st, opts.OrganizationKey, "cluster-aws-eks").DriverInputs), "vpc.default#@infra") ||
		!strings.Contains(toString(definitionByKey(t, st, opts.OrganizationKey, "postgres-aws-aurora").DriverInputs), "vpc.default#@infra") {
		t.Fatal("seeded EKS and Aurora must reference the VPC through @infra")
	}
}

// earlier rewrites a seeded Definition into one of the two earlier seeded forms.
func earlier(t *testing.T, st *store.Store, opts seed.Options, key string, replacer *strings.Replacer) resource.Definition {
	t.Helper()
	current := definitionByKey(t, st, opts.OrganizationKey, key)
	raw, _ := json.Marshal(current.DriverInputs)
	if err := json.Unmarshal([]byte(replacer.Replace(string(raw))), &current.DriverInputs); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveResourceDefinition(context.Background(), opts.OrganizationKey, current); err != nil {
		t.Fatal(err)
	}
	return current
}

var awsKeys = []string{"vpc-aws", "cluster-aws-eks", "postgres-aws-aurora"}

// An earlier installation stored an exact old seeded AWS form: the original
// `<app>-<run>` with the application VPC path, or the interim
// `<infra.name>-<run>`. A restart upgrades both in place, idempotently.
func TestApplyUpgradesExactEarlierSeededAWSDefinitions(t *testing.T) {
	forms := map[string]*strings.Replacer{
		"original": strings.NewReplacer("vpc.default#@infra", "vpc.default#applications.@app", "${context.infra.resourceName}", "${context.app.id}-${context.run.id}"),
		"interim":  strings.NewReplacer("${context.infra.resourceName}", "${context.infra.name}-${context.run.id}"),
	}
	for name, replacer := range forms {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			st := store.New()
			opts := seed.Defaults()
			if err := seed.Apply(ctx, st, opts); err != nil {
				t.Fatal(err)
			}
			seeded := map[string]string{}
			for _, key := range awsKeys {
				seeded[key] = toString(definitionByKey(t, st, opts.OrganizationKey, key).DriverInputs)
				if old := earlier(t, st, opts, key, replacer); toString(old.DriverInputs) == seeded[key] {
					t.Fatalf("%s: the earlier form must differ from the seeded one", key)
				}
			}
			for i := 0; i < 2; i++ {
				if err := seed.Apply(ctx, st, opts); err != nil {
					t.Fatal(err)
				}
			}
			for _, key := range awsKeys {
				if got := toString(definitionByKey(t, st, opts.OrganizationKey, key).DriverInputs); got != seeded[key] {
					t.Fatalf("%s was not upgraded:\n got %s\nwant %s", key, got, seeded[key])
				}
			}
		})
	}
}

// Identical seeded AWS Definitions are not rewritten at all.
func TestApplyDoesNotRewriteCurrentSeededAWSDefinitions(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	before := definitionByKey(t, st, opts.OrganizationKey, "vpc-aws")
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if after := definitionByKey(t, st, opts.OrganizationKey, "vpc-aws"); toString(after) != toString(before) {
		t.Fatalf("current seeded AWS definition changed: %+v", after)
	}
}

// A same-key non-AWS Definition survives seed.Apply whatever it customizes: the
// criteria and Connection may equal the seeded ones while only the name and
// context inputs differ, and no fingerprint marks it as authored.
func TestApplyKeepsSameStructureNonAWSDefinitionWithCustomInputs(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"postgres-internal-statefulset"} {
		d := definitionByKey(t, st, opts.OrganizationKey, key)
		if d.SourceFingerpr != "" {
			t.Fatalf("%s unexpectedly carries a fingerprint", key)
		}
		d.DriverInputs = map[string]any{"name": "authored-${context.app.id}", "kubeContext": "custom-context"}
		if err := st.SaveResourceDefinition(ctx, opts.OrganizationKey, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"postgres-internal-statefulset"} {
		got := definitionByKey(t, st, opts.OrganizationKey, key)
		if !strings.Contains(toString(got.DriverInputs), "authored-${context.app.id}") || !strings.Contains(toString(got.DriverInputs), "custom-context") {
			t.Fatalf("%s custom inputs overwritten: %+v", key, got.DriverInputs)
		}
	}
}

// Platform-authored AWS Definitions of seeded keys stay intact: a fingerprint on
// an otherwise exact earlier form, customized inputs, or another structure.
func TestApplyKeepsAuthoredAWSDefinitionsOfSeededKeys(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	// Structurally seeded, exact earlier template, but a non-empty fingerprint.
	fingerprinted := earlier(t, st, opts, "cluster-aws-eks", strings.NewReplacer("${context.infra.resourceName}", "${context.infra.name}-${context.run.id}"))
	fingerprinted.SourceFingerpr = "sha256:platform-authored"
	if err := st.SaveResourceDefinition(ctx, opts.OrganizationKey, fingerprinted); err != nil {
		t.Fatal(err)
	}
	// Structurally seeded, current template, with a fingerprint: left untouched.
	current := definitionByKey(t, st, opts.OrganizationKey, "postgres-aws-aurora")
	current.SourceFingerpr = "sha256:authored-current"
	if err := st.SaveResourceDefinition(ctx, opts.OrganizationKey, current); err != nil {
		t.Fatal(err)
	}
	// Customized inputs.
	vpc := definitionByKey(t, st, opts.OrganizationKey, "vpc-aws")
	vpc.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"name": "custom-${context.app.id}", "cidr": "10.9.0.0/16"}}}
	if err := st.SaveResourceDefinition(ctx, opts.OrganizationKey, vpc); err != nil {
		t.Fatal(err)
	}
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if got := definitionByKey(t, st, opts.OrganizationKey, "cluster-aws-eks"); got.SourceFingerpr != "sha256:platform-authored" || toString(got.DriverInputs) != toString(fingerprinted.DriverInputs) {
		t.Fatalf("fingerprinted earlier-form Definition overwritten: %+v", got)
	}
	if got := definitionByKey(t, st, opts.OrganizationKey, "postgres-aws-aurora"); got.SourceFingerpr != "sha256:authored-current" || toString(got.DriverInputs) != toString(current.DriverInputs) {
		t.Fatalf("fingerprinted current Definition overwritten: %+v", got)
	}
	if got := definitionByKey(t, st, opts.OrganizationKey, "vpc-aws"); !strings.Contains(toString(got.DriverInputs), "custom-${context.app.id}") {
		t.Fatalf("customized AWS inputs overwritten: %+v", got.DriverInputs)
	}
}

// Platform-authored Definitions that reuse a seeded key with another structure
// (criteria, Connection) are never overwritten by a restart.
func TestApplyKeepsPlatformAuthoredDefinitionsOfSeededKeys(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	cluster := resource.Definition{Key: "cluster-internal-registered", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, ConnectionKey: opts.ConnectionKey}
	cluster.Criteria = []resource.Criterion{{ApplicationID: "only-mine", Class: "internal"}}
	cluster.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"name": "authored"}}}
	if err := st.SaveResourceDefinition(ctx, opts.OrganizationKey, cluster); err != nil {
		t.Fatal(err)
	}
	eks := earlier(t, st, opts, "cluster-aws-eks", strings.NewReplacer("${context.infra.resourceName}", "${context.infra.name}-${context.run.id}"))
	eks.ConnectionKey = "my-aws"
	if err := st.SaveResourceDefinition(ctx, opts.OrganizationKey, eks); err != nil {
		t.Fatal(err)
	}
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if got := definitionByKey(t, st, opts.OrganizationKey, "cluster-internal-registered"); len(got.Criteria) != 1 || got.Criteria[0].ApplicationID != "only-mine" || !strings.Contains(toString(got.DriverInputs), "authored") {
		t.Fatalf("authored existing-cluster Definition overwritten: %+v", got)
	}
	if got := definitionByKey(t, st, opts.OrganizationKey, "cluster-aws-eks"); got.ConnectionKey != "my-aws" || !strings.Contains(toString(got.DriverInputs), "context.infra.name") {
		t.Fatalf("authored AWS Definition overwritten: %+v", got)
	}
}

// JSON sorts map keys and compares complete Definitions as well as their inputs.
func toString(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
