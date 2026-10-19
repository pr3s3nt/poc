// Package seed registers fixture catalogs and initial identities for local,
// test and isolated acceptance modes. Seed modes are harness choices; product
// placement comes from Resource Definitions and resource references.
package seed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/persistence"
)

// RenderBundleID is the score-k8s render bundle the seeded workload Definitions use (the real
// renderer and the fake adapters both install a bundle of this ID).
const RenderBundleID = "score-k8s-internal-v1"

// Catalog modes of Options.Catalog.
const (
	CatalogLegacy       = ""
	CatalogTerraformK8s = "terraform-k8s"
	CatalogNone         = "none"
)

// ProfileAcceptanceV2 is the isolated real-acceptance seed profile (I04-10):
// fixed fixture accounts, the exact owned Application/Environment namespace
// identity, and NO catalog, Connection or Environment binding. Everything a
// business step needs is authored through the Console.
const ProfileAcceptanceV2 = "acceptance-v2"

// Images names the acceptance application container images.
type Images struct {
	Frontend string
	Backend  string
	Worker   string
	Postgres string
}

// Options configures the seeded Organization, Applications and Environments.
type Options struct {
	Profile         string
	OrganizationKey string
	BaseDomain      string

	ApplicationKey    string
	ApplicationName   string
	NamespaceIdentity string
	ConnectionKey     string
	KubeContext       string
	ClusterName       string

	CloudApplicationKey    string
	CloudApplicationName   string
	CloudNamespaceIdentity string
	CloudConnectionKey     string
	Region                 string
	AccountID              string

	EnvironmentKey  string
	EnvironmentName string
	EnvironmentType string

	// Catalog selects which Resource Types and Definitions Apply registers:
	// CatalogLegacy (default) is the walking-skeleton catalog whose in-cluster
	// namespace and PostgreSQL use the legacy Kubernetes driver;
	// CatalogTerraformK8s replaces exactly those with the verified Terraform
	// namespace/PostgreSQL/Redis modules; CatalogNone registers nothing, so a
	// real acceptance run authors every Type, Definition, pin and mapping
	// through the Console. ProfileAcceptanceV2 requires CatalogNone; the test and local profiles may select it for development harnesses.
	Catalog string

	RunID            string
	Images           Images
	SharedDatabaseID string
	Database         string
	DatabaseUser     string
	Cloud            CloudConfig
	// PlatformModules are the verified packaged module pins (by module ID)
	// the seeded Terraform Definitions reference. Empty means the pins of the
	// embedded platform release.
	PlatformModules map[string]tfmodule.SourceRef
}

// CloudConfig holds cost-driven fixture values used to seed AWS Definitions.
// The verification harness fills them after a pricing lookup; they are not an
// Environment deployment target.
type CloudConfig struct {
	VPCCidr             string
	KubernetesVersion   string
	NodeInstanceType    string
	NodeCapacityType    string
	NodeAmiType         string
	NodeDiskSize        float64
	AuroraEngineVersion string
	AuroraMinACU        float64
	AuroraMaxACU        float64
	// EKSPublicAccessCIDRs is the PE/Admin allowlist (specific IPv4 /32 hosts)
	// of the EKS public API. There is deliberately no default: empty leaves the
	// seeded EKS Definition without the variable, so planning refuses it before
	// any effect until a PE/Admin supplies the list.
	EKSPublicAccessCIDRs []string
}

// DefaultCloudConfig returns the cheapest configuration that still runs the
// three acceptance workloads. Values are re-checked against the Pricing API by
// test/integration/aws-verify.sh before any apply.
func DefaultCloudConfig() CloudConfig {
	return CloudConfig{
		VPCCidr:             "10.42.0.0/16",
		KubernetesVersion:   "1.35",
		NodeInstanceType:    "t4g.medium",
		NodeCapacityType:    "SPOT",
		NodeAmiType:         "AL2023_ARM_64_STANDARD",
		NodeDiskSize:        20,
		AuroraEngineVersion: "16.14",
		AuroraMinACU:        0,
		AuroraMaxACU:        1,
	}
}

// Defaults fills the values the walking skeleton does not need to override.
func Defaults() Options {
	return Options{
		Profile:                "local",
		OrganizationKey:        "acme",
		BaseDomain:             "example.com",
		ApplicationKey:         "acceptance",
		ApplicationName:        "Acceptance Application",
		NamespaceIdentity:      "acceptance-dev",
		ConnectionKey:          "internal-cluster",
		KubeContext:            "kind-idp-internal",
		ClusterName:            "idp-internal",
		CloudApplicationKey:    "acceptance-cloud",
		CloudApplicationName:   "Acceptance Application (cloud)",
		CloudNamespaceIdentity: "acceptance-cloud-dev",
		CloudConnectionKey:     "aws-account",
		EnvironmentKey:         "dev",
		EnvironmentName:        "Development",
		EnvironmentType:        "development",
		SharedDatabaseID:       "acceptance-db",
		Database:               "acceptance",
		DatabaseUser:           "app",
		Images: Images{
			Frontend: "acceptance-frontend:dev",
			Backend:  "acceptance-backend:dev",
			Worker:   "acceptance-worker:dev",
			Postgres: "postgres:16-alpine",
		},
		Cloud: DefaultCloudConfig(),
	}
}

// ResourceTypes returns the Resource Type contracts used by the MVP (UC-02).
// Inputs are the keys a Score document may pass as `params`.
func ResourceTypes() []resource.Type { return ResourceTypesFor(Options{}) }

// RedisType is the Redis Resource Type contract of the acceptance application.
func RedisType() resource.Type {
	return resource.Type{Key: "redis", Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true},
		{Name: "username", Type: "string", Required: true}, {Name: "password", Type: "string", Required: true, Secret: true},
		{Name: "tls", Type: "bool", Required: true}}}
}

// ResourceTypesFor returns the Types Apply registers for the catalog mode: none
// for CatalogNone, plus Redis for CatalogTerraformK8s.
func ResourceTypesFor(o Options) []resource.Type {
	if o.CatalogFree() {
		return nil
	}
	types := resourceTypes()
	if o.Catalog == CatalogTerraformK8s {
		types = append(types, RedisType())
	}
	return types
}

func resourceTypes() []resource.Type {
	return []resource.Type{
		{Key: "workload"},
		{
			Key:     "k8s-namespace",
			Outputs: []resource.OutputField{{Name: "name", Type: "string", Required: true}},
		},
		{
			Key: "k8s-cluster",
			Outputs: []resource.OutputField{
				{Name: "name", Type: "string", Required: true},
				{Name: "endpoint", Type: "string"},
				{Name: "kubeContext", Type: "string", Required: true},
				{Name: "kubeconfig", Type: "string"},
				// The opaque scoped Kubernetes target of an existing cluster (ADR-016).
				{Name: "cluster", Type: "any"},
			},
		},
		{
			Key: "vpc",
			Outputs: []resource.OutputField{
				{Name: "id", Type: "string", Required: true},
				{Name: "cidr", Type: "string"},
				{Name: "subnetIds", Type: "any"},
				// Optional: the shared private subnets Aurora/ElastiCache reference.
				{Name: "privateSubnetIds", Type: "any"},
			},
		},
		{
			Key: "postgres",
			Inputs: []resource.InputField{
				{Name: "database", Type: "string", Required: true},
				{Name: "username", Type: "string", Required: true},
			},
			Outputs: []resource.OutputField{
				{Name: "host", Type: "string", Required: true},
				{Name: "port", Type: "number", Required: true},
				{Name: "database", Type: "string", Required: true},
				{Name: "username", Type: "string", Required: true},
				{Name: "password", Type: "string", Required: true, Secret: true},
			},
		},
	}
}

func driverInputs(variables map[string]any) map[string]any {
	return map[string]any{"values": map[string]any{"variables": variables}}
}

// terraformInputs is the strict Terraform v2 Driver Inputs shape: the exact
// verified module pin, Definition values, explicit input/output mappings and
// the Driver Account AWS provider binding.
func terraformInputs(ref tfmodule.SourceRef, variables map[string]any, inputs, outputs []string) map[string]any {
	inputMapping := map[string]any{}
	for _, name := range inputs {
		inputMapping[name] = map[string]any{"param": name}
	}
	outputMapping := map[string]any{}
	for _, name := range outputs {
		outputMapping[name] = name
	}
	return map[string]any{"values": map[string]any{
		"source":            ref.Map(),
		"variables":         variables,
		"input_mapping":     inputMapping,
		"output_mapping":    outputMapping,
		"provider_bindings": map[string]any{"aws": map[string]any{"target": tfmodule.TargetAccount}},
	}}
}

func (o Options) platformModules() map[string]tfmodule.SourceRef {
	if len(o.PlatformModules) > 0 {
		return o.PlatformModules
	}
	refs, err := registry.PackagedRefs()
	if err != nil {
		panic("seed: the embedded platform release manifest is invalid: " + err.Error())
	}
	return refs
}

// ResourceDefinitions returns the seeded Resource Definitions (UC-03 V02-08).
// Matching uses only the five criteria fields: the internal and cloud demands
// are told apart by the explicit app_id of their seeded Application, never by
// an execution profile, and AWS Definitions name their Driver Account and
// literal region explicitly.
func ResourceDefinitions(o Options) []resource.Definition {
	cloud := o.Cloud
	if cloud.KubernetesVersion == "" {
		allowlist := cloud.EKSPublicAccessCIDRs
		cloud = DefaultCloudConfig()
		cloud.EKSPublicAccessCIDRs = allowlist
	}
	eksVariables := map[string]any{
		"name":               "${context.infra.resourceName}",
		"subnet_ids":         "${resources['vpc.default#@infra'].outputs.subnetIds}",
		"kubernetes_version": cloud.KubernetesVersion,
		"node_instance_type": cloud.NodeInstanceType,
		"node_capacity_type": cloud.NodeCapacityType,
		"node_ami_type":      cloud.NodeAmiType,
		"node_disk_size":     cloud.NodeDiskSize,
		"ebs_csi_driver":     true,
	}
	if len(cloud.EKSPublicAccessCIDRs) > 0 {
		list := make([]any, len(cloud.EKSPublicAccessCIDRs))
		for i, c := range cloud.EKSPublicAccessCIDRs {
			list[i] = c
		}
		eksVariables["public_access_cidrs"] = list
	}
	modules := o.platformModules()
	if o.Catalog == CatalogNone {
		return nil
	}
	defs := baseDefinitions(o, cloud, eksVariables, modules)
	if o.Catalog != CatalogTerraformK8s {
		return defs
	}
	out := make([]resource.Definition, 0, len(defs)+2)
	for _, d := range defs {
		switch d.Key {
		case "namespace-kubernetes", "namespace-kubernetes-internal", "postgres-internal-statefulset":
			continue // legacy Kubernetes driver: not v2 provider proof
		}
		out = append(out, d)
	}
	return append(out, terraformK8sDefinitions(modules, o.ApplicationKey)...)
}

// terraformK8sDefinitions are the in-cluster Definitions over the verified
// packaged modules. The Kubernetes provider is bound to the matched internal
// cluster resource (ADR-016), never inferred from variables. The seeded internal Application selects
// the in-cluster database/cache through its app_id criterion. The namespace name is the exact Environment
// namespace (generation suffix included).
func terraformK8sDefinitions(modules map[string]tfmodule.SourceRef, appKey string) []resource.Definition {
	nsOut := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	const clusterRef = "k8s-cluster.internal#environments.@app.@env"
	build := func(key, name, typ, module string, criteria []resource.Criterion, variables map[string]any, inputs, outputs []string) resource.Definition {
		inputMapping := map[string]any{}
		for _, name := range inputs {
			inputMapping[name] = map[string]any{"param": name}
		}
		outputMapping := map[string]any{}
		for _, name := range outputs {
			outputMapping[name] = name
		}
		return resource.Definition{Key: key, Name: name, Version: 1, ResourceTypeKey: typ, DriverType: resource.DriverTerraform,
			DriverInputs: map[string]any{"values": map[string]any{
				"source": modules[module].Map(), "variables": variables, "input_mapping": inputMapping, "output_mapping": outputMapping,
				"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetResource, "resource_ref": clusterRef}},
			}},
			Criteria: criteria}
	}
	internalApp := []resource.Criterion{{ApplicationID: appKey}}
	return []resource.Definition{
		build("namespace-k8s-module", "Namespace Kubernetes (Terraform module)", "k8s-namespace", "k8s-namespace", []resource.Criterion{{}}, map[string]any{"name": "${context.env.namespace}"}, nil, []string{"name"}),
		build("postgres-k8s-module", "PostgreSQL trong cluster (Terraform module)", "postgres", "postgres-k8s", internalApp, map[string]any{"namespace": nsOut}, []string{"database", "username"}, []string{"host", "port", "database", "username", "password"}),
		build("redis-k8s-module", "Redis trong cluster (Terraform module)", "redis", "redis-k8s", internalApp, map[string]any{"namespace": nsOut}, nil, []string{"host", "port", "username", "password", "tls"}),
	}
}

func baseDefinitions(o Options, cloud CloudConfig, eksVariables map[string]any, modules map[string]tfmodule.SourceRef) []resource.Definition {
	// AWS Definitions carry their own scoped READY account and a literal region
	// (UC-03 V02-03); the seeded cloud Application selects them by app_id.
	withRegion := func(variables map[string]any) map[string]any {
		if o.Region != "" {
			variables["region"] = o.Region
		}
		return variables
	}
	cloudApp := []resource.Criterion{{ApplicationID: o.CloudApplicationKey}}
	internalApp := []resource.Criterion{{ApplicationID: o.ApplicationKey}}
	return []resource.Definition{
		// ADR-016: the internal Application's placement is decided by Definitions. The cluster
		// Definition uses the seeded Internal Driver Account; the workload and namespace
		// Definitions reference its typed `cluster` output.
		{
			Key:             "cluster-internal",
			Name:            "Cluster nội bộ (cluster có sẵn)",
			Version:         1,
			ResourceTypeKey: "k8s-cluster",
			DriverType:      resource.DriverExistingCluster,
			DriverAccount:   o.ConnectionKey,
			DriverInputs:    driverInputs(map[string]any{}),
			Criteria:        []resource.Criterion{{ApplicationID: o.ApplicationKey, Class: "internal"}},
		},
		{
			Key:             "namespace-kubernetes-internal",
			Name:            "Namespace Kubernetes của ứng dụng nội bộ",
			Version:         1,
			ResourceTypeKey: "k8s-namespace",
			DriverType:      resource.DriverTerraform,
			DriverInputs: map[string]any{"values": map[string]any{
				"source": modules["k8s-namespace"].Map(), "variables": map[string]any{"name": "${context.env.namespace}"},
				"input_mapping": map[string]any{}, "output_mapping": map[string]any{"name": "name"},
				"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": tfmodule.TargetResource, "resource_ref": "k8s-cluster.internal#environments.@app.@env"}},
			}},
			Criteria: internalApp,
		},
		{
			Key:             "workload-score-k8s",
			Name:            "Mẫu dựng workload score-k8s",
			Version:         1,
			ResourceTypeKey: "workload",
			DriverType:      resource.DriverScoreK8s,
			DriverInputs: driverInputs(map[string]any{
				"render_bundle": RenderBundleID,
				"namespace":     "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}",
				"cluster":       "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}",
			}),
			Criteria: internalApp,
		},
		{
			Key:             "workload-score-k8s-cloud",
			Name:            "Mẫu dựng workload score-k8s (cloud)",
			Version:         1,
			ResourceTypeKey: "workload",
			DriverType:      resource.DriverScoreK8s,
			DriverInputs:    driverInputs(map[string]any{"render_bundle": RenderBundleID}),
			Criteria:        cloudApp,
		},
		{
			Key:             "namespace-kubernetes",
			Name:            "Namespace Kubernetes (driver cũ)",
			Version:         1,
			ResourceTypeKey: "k8s-namespace",
			DriverType:      resource.DriverKubernetes,
			DriverInputs:    driverInputs(map[string]any{"name": "${context.env.namespace}"}),
			Criteria:        []resource.Criterion{{}},
		},
		{
			Key:             "cluster-aws-eks",
			Name:            "Cluster EKS (AWS)",
			Version:         1,
			ResourceTypeKey: "k8s-cluster",
			DriverType:      resource.DriverTerraform,
			DriverAccount:   o.CloudConnectionKey,
			DriverInputs:    terraformInputs(modules["eks"], withRegion(eksVariables), nil, []string{"name", "endpoint", "kubeContext", "kubeconfig"}),
			Criteria:        []resource.Criterion{{Class: "eks"}},
		},
		{
			Key:             "vpc-aws",
			Name:            "VPC (AWS)",
			Version:         1,
			ResourceTypeKey: "vpc",
			DriverType:      resource.DriverTerraform,
			DriverAccount:   o.CloudConnectionKey,
			DriverInputs: terraformInputs(modules["vpc"], withRegion(map[string]any{
				"name": "${context.infra.resourceName}",
				"cidr": cloud.VPCCidr,
			}), nil, []string{"id", "cidr", "subnetIds", "privateSubnetIds"}),
			Criteria: []resource.Criterion{{}},
		},
		{
			Key:             "postgres-internal-statefulset",
			Name:            "PostgreSQL StatefulSet trong cluster (driver cũ)",
			Version:         1,
			ResourceTypeKey: "postgres",
			DriverType:      resource.DriverKubernetes,
			DriverInputs: driverInputs(map[string]any{
				"image":     o.Images.Postgres,
				"storage":   "1Gi",
				"namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}",
			}),
			Criteria: internalApp,
		},
		{
			Key:             "postgres-aws-aurora",
			Name:            "PostgreSQL Aurora (AWS)",
			Version:         1,
			ResourceTypeKey: "postgres",
			DriverType:      resource.DriverTerraform,
			DriverAccount:   o.CloudConnectionKey,
			DriverInputs: terraformInputs(modules["aurora"], withRegion(map[string]any{
				"name":               "${context.infra.resourceName}",
				"engine_version":     cloud.AuroraEngineVersion,
				"min_capacity":       cloud.AuroraMinACU,
				"max_capacity":       cloud.AuroraMaxACU,
				"vpc_id":             "${resources['vpc.default#@infra'].outputs.id}",
				"subnet_ids":         "${resources['vpc.default#@infra'].outputs.subnetIds}",
				"private_subnet_ids": "${resources['vpc.default#@infra'].outputs.privateSubnetIds}",
			}), []string{"database", "username"}, []string{"host", "port", "database", "username", "password"}),
			Criteria: cloudApp,
		},
	}
}

// Stable IDs of the UC-00 accounts seeded only for local/test. Production
// denies exactly these accounts, not every account with the same username.
const (
	FixedDeveloperAccountID        = "5eed0000-0000-4000-8000-000000000001"
	FixedPlatformEngineerAccountID = "5eed0000-0000-4000-8000-000000000002"
)

// FixedTestPassword is the shared password of the fixed local/test accounts.
const FixedTestPassword = "test-password"

// FixedTestAccountIDs lists the stable IDs of the fixed local/test accounts.
func FixedTestAccountIDs() []string {
	return []string{FixedDeveloperAccountID, FixedPlatformEngineerAccountID}
}

// fixedTestUsernames lists the usernames of the fixed local/test accounts.
var fixedTestUsernames = []string{"developer", "platform-engineer"}

// FixedTestAccountIDsIn returns the stable fixed IDs plus the IDs of legacy
// fixed accounts found in the store. Accounts seeded before stable IDs had
// random IDs; one counts as fixed only when its username is a fixed username
// AND its stored hash verifies the fixed password. An account that shares the
// username but has another password is not a fixed test account.
func FixedTestAccountIDsIn(ctx context.Context, store persistence.Store) ([]string, error) {
	out := FixedTestAccountIDs()
	for _, username := range fixedTestUsernames {
		account, err := store.GetUserAccountByUsername(ctx, username)
		if errors.Is(err, persistence.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if password.Verify(account.PasswordHash, FixedTestPassword) {
			out = append(out, account.ID)
		}
	}
	return out, nil
}

// AllowsFixedTestAccounts reports whether the profile may seed and accept the
// fixed local/test accounts.
func AllowsFixedTestAccounts(profile string) bool {
	return profile == "local" || profile == "test" || profile == ProfileAcceptanceV2
}

func (o Options) defaultConnectionKey() string {
	if o.CatalogFree() {
		return "" // Connections are registered through the Console
	}
	return o.ConnectionKey
}

// CatalogFree reports whether Apply registers no catalog, Connection or
// Environment binding for these options.
func (o Options) CatalogFree() bool { return o.Catalog == CatalogNone }

// Validate refuses combinations that would hide a legacy catalog in a real
// acceptance profile or an unknown catalog mode.
func (o Options) Validate() error {
	switch o.Catalog {
	case CatalogLegacy, CatalogTerraformK8s, CatalogNone:
	default:
		return fmt.Errorf("seed: unknown catalog mode %q", o.Catalog)
	}
	if o.Profile == ProfileAcceptanceV2 && o.Catalog != CatalogNone {
		return errors.New("seed: the acceptance-v2 profile seeds no catalog; the catalog is authored through the Console")
	}
	if o.Catalog == CatalogNone && o.Profile != ProfileAcceptanceV2 && o.Profile != "test" && o.Profile != "local" {
		return errors.New("seed: the empty catalog is only for the acceptance-v2, test and local profiles")
	}
	return nil
}

// Apply writes the seeded catalog, both Applications and their Environments.
func Apply(ctx context.Context, store persistence.Store, o Options) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if _, err := store.GetOrganization(ctx, o.OrganizationKey); errors.Is(err, persistence.ErrNotFound) {
		if err := store.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: o.OrganizationKey, Name: "Acme", DefaultConnectionKey: o.defaultConnectionKey()}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if AllowsFixedTestAccounts(o.Profile) {
		passwordHash, err := password.Hash(FixedTestPassword)
		if err != nil {
			return err
		}
		if _, err := store.GetUserAccountByUsername(ctx, "developer"); errors.Is(err, persistence.ErrNotFound) {
			if err := store.SaveUserAccount(ctx, identity.UserAccount{ID: FixedDeveloperAccountID, OrganizationKey: o.OrganizationKey, Username: "developer", PasswordHash: passwordHash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if _, err := store.GetUserAccountByUsername(ctx, "platform-engineer"); errors.Is(err, persistence.ErrNotFound) {
			if err := store.SaveUserAccount(ctx, identity.UserAccount{ID: FixedPlatformEngineerAccountID, OrganizationKey: o.OrganizationKey, Username: "platform-engineer", PasswordHash: passwordHash, Role: identity.RolePlatformEngineer, Status: identity.AccountActive}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	for _, t := range ResourceTypesFor(o) {
		if err := store.SaveResourceType(ctx, o.OrganizationKey, t); err != nil {
			return err
		}
	}
	if o.CatalogFree() {
		return applyFixtureApplication(ctx, store, o)
	}

	internalConnection := application.Connection{
		ID:              ids.New(),
		Key:             o.ConnectionKey,
		Name:            o.ConnectionKey,
		OrganizationKey: o.OrganizationKey,
		Kind:            application.ConnectionKubernetes,
		// The seeded Kubernetes record is explicit KUBECONFIG metadata without an
		// uploaded credential: VERIFYING with an empty reference, never READY.
		// It satisfies the Organization default marker but cannot execute.
		AuthenticationType: application.AuthKubeconfig,
		Config:             map[string]any{"cluster": o.ClusterName, "kubeContext": o.KubeContext},
		Status:             application.ConnectionVerifying,
		Verification:       map[string]any{"cluster": o.ClusterName, "verified": false},
	}
	cloudConnection := application.Connection{
		ID:              ids.New(),
		Key:             o.CloudConnectionKey,
		Name:            o.CloudConnectionKey,
		OrganizationKey: o.OrganizationKey,
		Kind:            application.ConnectionAWS,
		// Process-configured AWS seed metadata; it does not hold durable credentials.
		AuthenticationType: application.AuthAWSAccessKey,
		Config:             map[string]any{"region": o.Region, "accountId": o.AccountID},
		SecretRef:          "secret://connections/" + o.CloudConnectionKey,
		Status:             application.ConnectionReady,
		Verification:       map[string]any{"accountId": o.AccountID, "region": o.Region, "verified": true},
	}
	for _, conn := range []application.Connection{internalConnection, cloudConnection} {
		if _, err := store.GetConnection(ctx, conn.OrganizationKey, conn.Key); errors.Is(err, persistence.ErrNotFound) {
			if err := store.SaveConnection(ctx, conn); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	// Definitions reference their Driver Account, so they follow the accounts.
	if err := refreshSeededDefinitions(ctx, store, o); err != nil {
		return err
	}

	internal := application.Application{
		ID:              ids.New(),
		Key:             o.ApplicationKey,
		OrganizationKey: o.OrganizationKey,
		Name:            o.ApplicationName,
		Subdomain:       "acceptance",
		Version:         1,
	}
	// Each acceptance Environment is bound once with explicit ENVIRONMENT
	// infrastructure scope (ADR-011).
	type appEnv struct {
		app       application.Application
		namespace string
		// binding is nil for the internal Application: Resource Definitions decide its placement
		// (ADR-016), so its Environment carries no deployment Connection.
		binding *persistence.EnvironmentBinding
	}
	pairs := []appEnv{{app: internal, namespace: o.NamespaceIdentity}}

	for _, pair := range pairs {
		if existing, err := store.GetApplication(ctx, pair.app.Key); err == nil {
			pair.app = existing
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		} else {
			if err := pair.app.Validate(); err != nil {
				return err
			}
			if err := store.SaveApplication(ctx, pair.app); err != nil {
				return err
			}
		}
		if existing, err := store.GetEnvironment(ctx, pair.app.Key, o.EnvironmentKey); err == nil {
			if existing.Configured() || pair.binding == nil {
				continue
			}
			pair.binding.ApplicationKey, pair.binding.EnvironmentKey, pair.binding.ExpectedVersion = pair.app.Key, o.EnvironmentKey, existing.Version
			if _, err := store.BindEnvironment(ctx, *pair.binding); err != nil {
				return err
			}
			continue
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		emptySet := environment.DeploymentSet{
			ID:             ids.New(),
			EnvironmentID:  "",
			EnvironmentKey: pair.app.Key + "/" + o.EnvironmentKey,
			Document:       environment.NewDocument(),
			DocumentHash:   "empty",
		}
		envID := ids.New()
		emptySet.EnvironmentID = envID
		env := environment.Environment{
			ID:                     envID,
			Key:                    o.EnvironmentKey,
			ApplicationID:          pair.app.ID,
			ApplicationKey:         pair.app.Key,
			Name:                   o.EnvironmentName,
			Type:                   o.EnvironmentType,
			NamespaceIdentity:      pair.namespace,
			CurrentDeploymentSetID: emptySet.ID,
			Version:                1,
		}
		if err := env.Validate(); err != nil {
			return err
		}
		// OC-02: Environment, empty Deployment Set and current pointer are atomic.
		err := store.Transact(ctx, func(ctx context.Context) error {
			if err := store.SaveDeploymentSet(ctx, emptySet); err != nil {
				return err
			}
			if err := store.SaveEnvironment(ctx, env); err != nil {
				return err
			}
			if pair.binding == nil {
				return nil
			}
			pair.binding.ApplicationKey, pair.binding.EnvironmentKey, pair.binding.ExpectedVersion = pair.app.Key, o.EnvironmentKey, env.Version
			_, err := store.BindEnvironment(ctx, *pair.binding)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// refreshSeededDefinitions creates the missing seeded Definitions. Every
// existing record is Platform-owned and is never rewritten, whatever its key,
// type, driver inputs or source fingerprint.
func refreshSeededDefinitions(ctx context.Context, store persistence.Store, o Options) error {
	existing, err := store.ListResourceDefinitions(ctx, o.OrganizationKey)
	if err != nil {
		return err
	}
	stored := map[string]bool{}
	for _, d := range existing {
		stored[d.Key] = true
	}
	for _, seeded := range ResourceDefinitions(o) {
		if stored[seeded.Key] {
			continue
		}
		if err := store.SaveResourceDefinition(ctx, o.OrganizationKey, seeded); err != nil {
			return err
		}
	}
	return nil
}

// applyFixtureApplication seeds ONLY the isolated Application fixture of a
// real acceptance run: the Application with its staging and production
// Environments (the two the Console offers), the staging Environment carrying
// the exact owned namespace identity (production gets a distinct unused one)
// and no binding. Connection, profile and everything else are Console steps.
func applyFixtureApplication(ctx context.Context, store persistence.Store, o Options) error {
	app := application.Application{ID: ids.New(), Key: o.ApplicationKey, OrganizationKey: o.OrganizationKey, Name: o.ApplicationName, Subdomain: "acceptance", Version: 1}
	if existing, err := store.GetApplication(ctx, app.Key); err == nil {
		app = existing
	} else if !errors.Is(err, persistence.ErrNotFound) {
		return err
	} else {
		if err := app.Validate(); err != nil {
			return err
		}
		if err := store.SaveApplication(ctx, app); err != nil {
			return err
		}
	}
	identities := map[string]string{"staging": o.NamespaceIdentity, "production": o.NamespaceIdentity + "-p"}
	if len(identities["production"]) > 63 {
		identities["production"] = identities["production"][:61] + "-p"
	}
	for _, key := range []string{"staging", "production"} {
		if _, err := store.GetEnvironment(ctx, app.Key, key); err == nil {
			continue
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		set := environment.DeploymentSet{ID: ids.New(), EnvironmentKey: app.Key + "/" + key, Document: environment.NewDocument(), DocumentHash: "empty"}
		envID := ids.New()
		set.EnvironmentID = envID
		env := environment.Environment{ID: envID, Key: key, ApplicationID: app.ID, ApplicationKey: app.Key, Name: strings.Title(key), Type: key,
			NamespaceIdentity: identities[key], CurrentDeploymentSetID: set.ID, Version: 1}
		if err := env.Validate(); err != nil {
			return err
		}
		if err := store.Transact(ctx, func(ctx context.Context) error {
			if err := store.SaveDeploymentSet(ctx, set); err != nil {
				return err
			}
			return store.SaveEnvironment(ctx, env)
		}); err != nil {
			return err
		}
	}
	return nil
}
