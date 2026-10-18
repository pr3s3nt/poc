package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	appsvc "orchestrator/internal/application/application"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed"
)

// ADR-016 / UC-03 V03: a public existing-cluster Definition with its own READY
// Internal Driver Account, and workload Definitions with typed placement.

func readyInternal(t *testing.T, st *store.Store, opts seed.Options, key string) {
	t.Helper()
	conn := application.Connection{ID: "id-" + key, Key: key, Name: key, OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady, SecretRef: "secret://x",
		Config: map[string]any{"cluster": "c", "kubeContext": "ctx"}}
	if err := st.SaveConnection(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
}

func v03Cluster(key, account string) resource.Definition {
	return resource.Definition{Key: key, Name: key, ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, DriverAccount: account,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}}}, Criteria: []resource.Criterion{{Class: "internal"}}}
}

func withClusterOutput(t *testing.T, st *store.Store, opts seed.Options) {
	t.Helper()
	if err := st.SaveResourceType(context.Background(), opts.OrganizationKey, resource.Type{Key: "k8s-cluster", Outputs: []resource.OutputField{{Name: "name", Type: "string"}, {Name: "cluster", Type: "any", Required: true}}}); err != nil {
		t.Fatal(err)
	}
}

func TestExistingClusterRegistration_RequiresOwnReadyInternalAccount(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	withClusterOutput(t, st, opts)
	readyInternal(t, st, opts, "lab")
	pending := application.Connection{ID: "id-pending", Key: "pending", Name: "pending", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionVerifying, Config: map[string]any{"cluster": "c", "kubeContext": "ctx"}}
	if err := st.SaveConnection(ctx, pending); err != nil {
		t.Fatal(err)
	}
	created, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, v03Cluster("cluster-lab", "lab"))
	if err != nil || created.Version != 1 || created.DriverAccount != "lab" {
		t.Fatalf("register: %+v %v", created, err)
	}
	cases := map[string]func(*resource.Definition){
		"missing account": func(d *resource.Definition) { d.DriverAccount = "" },
		"unknown account": func(d *resource.Definition) { d.DriverAccount = "nope" },
		"aws account":     func(d *resource.Definition) { d.DriverAccount = opts.CloudConnectionKey },
		"not ready":       func(d *resource.Definition) { d.DriverAccount = "pending" },
		"wrong type":      func(d *resource.Definition) { d.ResourceTypeKey = "postgres" },
		"raw variables": func(d *resource.Definition) {
			d.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"kubeContext": "${context.connection.context}"}}}
		},
		"credential variable": func(d *resource.Definition) {
			d.DriverInputs = map[string]any{"values": map[string]any{"variables": map[string]any{"kubeconfig": "x"}}}
		},
		"reserved key": func(d *resource.Definition) { d.Key = "builtin-existing-cluster" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			def := v03Cluster("cluster-"+strings.ReplaceAll(name, " ", "-"), "lab")
			mutate(&def)
			if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
				t.Fatalf("must be rejected: %v", err)
			}
		})
	}
	// CAS edit keeps working and may change only the account/criteria, not the type.
	edit := v03Cluster("cluster-lab", "lab")
	edit.Criteria = []resource.Criterion{{Class: "internal", ApplicationID: "shop"}}
	saved, err := svc.UpdateResourceDefinition(ctx, opts.OrganizationKey, "cluster-lab", 1, edit)
	if err != nil || saved.Version != 2 {
		t.Fatalf("update: %+v %v", saved, err)
	}
	if _, err := svc.UpdateResourceDefinition(ctx, opts.OrganizationKey, "cluster-lab", 1, edit); !errors.Is(err, catalog.ErrVersionConflict) {
		t.Fatalf("a stale version must conflict: %v", err)
	}
}

func TestExistingClusterRegistration_NeedsTheTypedClusterOutput(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	// A Type without the typed `cluster` output (for example one registered before V03).
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "k8s-cluster", Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}}}); err != nil {
		t.Fatal(err)
	}
	readyInternal(t, st, opts, "lab")
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, v03Cluster("cluster-lab", "lab")); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("a Type without the cluster target output must not serve an existing cluster: %v", err)
	}
	withClusterOutput(t, st, opts)
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, v03Cluster("cluster-lab", "lab")); err != nil {
		t.Fatal(err)
	}
	secret := resource.Type{Key: "k8s-cluster", Outputs: []resource.OutputField{{Name: "cluster", Type: "any", Secret: true}}}
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, v03Cluster("cluster-lab-2", "lab")); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("a secret cluster output is not an opaque public target: %v", err)
	}
}

func TestPreviewMatching_ExistingClusterShowsItsInternalAccountNotASystemBinding(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	withClusterOutput(t, st, opts)
	readyInternal(t, st, opts, "lab")
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, v03Cluster("cluster-lab", "lab")); err != nil {
		t.Fatal(err)
	}
	draft := catalog.DefinitionDraft{Key: "cluster-lab", Name: "cluster-lab", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, DriverAccount: "lab", Criteria: []resource.Criterion{{Class: "internal", ApplicationID: opts.ApplicationKey, EnvironmentID: opts.EnvironmentKey}}}
	q := previewQuery(opts, draft, "environments."+opts.ApplicationKey+"."+opts.EnvironmentKey, "internal")
	got := runPreview(t, svc, opts, q)
	if got.Status != catalog.PreviewMatched || got.Winner == nil || got.Winner.Account == nil || got.Winner.Account.Key != "lab" || got.Winner.Account.Type != "INTERNAL" || got.Winner.Account.Version < 1 {
		t.Fatalf("preview = %+v", got)
	}
	if got.Winner.Region != "" || got.ConnectionConflict != nil {
		t.Fatalf("an Internal account carries no region or conflict: %+v", got)
	}
	// An AWS account on the same Definition is a typed conflict; no other candidate replaces the winner.
	draft.DriverAccount = opts.CloudConnectionKey
	conflict := runPreview(t, svc, opts, previewQuery(opts, draft, "environments."+opts.ApplicationKey+"."+opts.EnvironmentKey, "internal"))
	if conflict.Status != catalog.PreviewConnectionConflict || conflict.ConnectionConflict == nil || conflict.ConnectionConflict.Reason != catalog.ConflictAccountKindMismatch {
		t.Fatalf("conflict = %+v", conflict)
	}
}

func TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	withClusterOutput(t, st, opts)
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "workload", Inputs: []resource.InputField{{Name: "backend_url", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	caps, err := svc.ListDefinitionCapabilities(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	var cluster, workload *catalog.Capability
	for i := range caps {
		switch caps[i].DriverType {
		case "existing-cluster":
			cluster = &caps[i]
		case "score-k8s":
			workload = &caps[i]
		}
	}
	if cluster == nil || !cluster.Account.Required || cluster.Account.Type != "INTERNAL" || cluster.Account.RegionVariable != "" || len(cluster.Variables) != 0 || cluster.Source != nil {
		t.Fatalf("existing-cluster capability = %+v", cluster)
	}
	if workload == nil {
		t.Fatal("score-k8s capability missing")
	}
	names := map[string]catalog.CapabilityVariable{}
	for _, v := range workload.Variables {
		names[v.Name] = v
	}
	if !names["namespace"].Required || names["namespace"].Preset != catalog.NamespaceReferencePreset || !names["cluster"].Required || names["cluster"].Preset != catalog.ClusterReferencePreset || !names["render_bundle"].Required || len(names) != 3 {
		t.Fatalf("workload variables = %+v", workload.Variables)
	}
	if strings.Contains(catalog.ClusterReferencePreset, "kubeconfig") {
		t.Fatal("presets must not name credentials")
	}
}

func TestWorkloadRegistration_TypedPlacementInputsAndCompanions(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	withClusterOutput(t, st, opts)
	svc.SetRenderBundles(map[string]resource.RenderBundle{"installed": {ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "bundle"}})
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "workload", Inputs: []resource.InputField{{Name: "backend_url", Type: "string"}, {Name: "replicas", Type: "number"}}}); err != nil {
		t.Fatal(err)
	}
	const (
		nsRef = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
		clRef = "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"
	)
	make := func(variables map[string]any) resource.Definition {
		merged := map[string]any{"render_bundle": "installed", "namespace": nsRef, "cluster": clRef}
		for k, v := range variables {
			if v == nil {
				delete(merged, k)
				continue
			}
			merged[k] = v
		}
		return resource.Definition{Key: "workload-v03", Name: "workload", ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}},
			DriverInputs: map[string]any{"values": map[string]any{"variables": merged}}}
	}
	bad := map[string]map[string]any{
		"only namespace":       {"cluster": nil},
		"only cluster":         {"namespace": nil},
		"literal namespace":    {"namespace": "my-namespace"},
		"wrong namespace out":  {"namespace": strings.ReplaceAll(nsRef, "outputs.name", "outputs.other")},
		"namespace is cluster": {"namespace": clRef},
		"cluster is namespace": {"cluster": nsRef},
		"concatenated":         {"cluster": clRef + "-x"},
		"undeclared input":     {"command": "x"},
		"wrong input type":     {"replicas": "three"},
		// Nothing at workload execution resolves a typed input, so none is accepted (no silent no-op).
		"declared typed input": {"backend_url": "http://backend.invalid:8080"},
		"declared number":      {"replicas": float64(2)},
		"credential input":     {"password": "x"},
	}
	for name, variables := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, make(variables)); !errors.Is(err, catalog.ErrInvalid) {
				t.Fatalf("must be rejected: %v", err)
			}
		})
	}
	def := make(map[string]any{})
	def.Provision = map[string]resource.ProvisionRule{"k8s-namespace.default#environments.@app.@env-extra": {MatchDependents: true}}
	created, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
	if err != nil || created.Version != 1 || created.SourceFingerpr != "bundle" {
		t.Fatalf("register: %+v %v", created, err)
	}
	edit := def
	edit.Provision = nil
	if _, err := svc.UpdateResourceDefinition(ctx, opts.OrganizationKey, "workload-v03", 1, edit); err != nil {
		t.Fatalf("form/JSON edit: %v", err)
	}
}

func TestWorkloadRegistration_RequiredTypeInputIsRefusedAndRenderOnlyIsAWSLegacyOnly(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	withClusterOutput(t, st, opts)
	svc.SetRenderBundles(map[string]resource.RenderBundle{"installed": {ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "bundle"}})
	renderOnly := func(app string) resource.Definition {
		return resource.Definition{Key: "workload-legacy", Name: "legacy", ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{ApplicationID: app}},
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "installed"}}}}
	}
	// A new Application has no Environment binding: it is Definition-placed, so render-only is refused for it.
	created, err := appsvc.NewService(st).Create(ctx, appsvc.CreateCommand{OrganizationKey: opts.OrganizationKey, Name: "Placed", Subdomain: "placed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, renderOnly(created.Application.Key)); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("render-only for a Definition-placed Application: %v", err)
	}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, resource.Definition{Key: "workload-any", Name: "any", ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}},
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "installed"}}}}); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("render-only without an Application scope: %v", err)
	}
	// Applications bound to a legacy execution profile keep the render-only form (AWS until V05).
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, renderOnly(opts.CloudApplicationKey)); err != nil {
		t.Fatalf("AWS legacy render-only: %v", err)
	}
	// A workload Type with a required input cannot be served: the driver supplies none.
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "workload", Inputs: []resource.InputField{{Name: "backend_url", Type: "string", Required: true}}}); err != nil {
		t.Fatal(err)
	}
	other := renderOnly(opts.CloudApplicationKey)
	other.Key = "workload-legacy-2"
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, other); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("a required workload Type input must be refused: %v", err)
	}
}

func TestWorkloadRegistration_TypeOutputsAreRefusedWithoutMutation(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, nil)
	withClusterOutput(t, st, opts)
	svc.SetRenderBundles(map[string]resource.RenderBundle{"installed": {ID: "installed", Version: "0.15.0", BinaryDigest: "binary", Digest: "bundle"}})
	if err := st.SaveResourceType(ctx, opts.OrganizationKey, resource.Type{Key: "workload", Outputs: []resource.OutputField{{Name: "url", Type: "string"}}}); err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{Key: "workload-out", Name: "w", ResourceTypeKey: "workload", DriverType: resource.DriverScoreK8s, Criteria: []resource.Criterion{{}},
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "installed",
			"namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}", "cluster": "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"}}}}
	before, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("a workload Type with outputs must be refused: %v", err)
	}
	after, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	if len(after) != len(before) {
		t.Fatal("a refused registration must store nothing")
	}
}
