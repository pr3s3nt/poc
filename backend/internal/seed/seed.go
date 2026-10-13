// Package seed registers the Phase 6 catalog: Resource Types, Resource
// Definitions, Connections, Applications and Environments. Both Execution
// Profiles are registered, so a single process can serve an internal-k8s and an
// aws-eks Application and matching can tell them apart by profile.
package seed

import (
	"context"
	"errors"

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

// CloudConfig holds the cost-driven choices of the aws-eks profile. The
// verification harness fills it after a pricing lookup.
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
}

// DefaultCloudConfig returns the cheapest configuration that still runs the
// three acceptance workloads. Values are re-checked against the Pricing API by
// test/integration/aws-verify.sh before any apply.
func DefaultCloudConfig() CloudConfig {
	return CloudConfig{
		VPCCidr:             "10.42.0.0/16",
		KubernetesVersion:   "1.34",
		NodeInstanceType:    "t4g.small",
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
func ResourceTypes() []resource.Type {
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
			},
		},
		{
			Key: "vpc",
			Outputs: []resource.OutputField{
				{Name: "id", Type: "string", Required: true},
				{Name: "cidr", Type: "string"},
				{Name: "subnetIds", Type: "any"},
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
// the Environment AWS provider binding.
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
		"provider_bindings": map[string]any{"aws": map[string]any{"target": tfmodule.TargetEnvironment}},
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

// ResourceDefinitions returns the Resource Definitions of both Execution
// Profiles (UC-03). The profile guard precedes five-field criteria matching.
func ResourceDefinitions(o Options) []resource.Definition {
	cloud := o.Cloud
	if cloud.KubernetesVersion == "" {
		cloud = DefaultCloudConfig()
	}
	modules := o.platformModules()
	return []resource.Definition{
		{
			Key:             "namespace-kubernetes",
			ResourceTypeKey: "k8s-namespace",
			DriverType:      resource.DriverKubernetes,
			DriverInputs:    driverInputs(map[string]any{"name": "${context.env.namespace}"}),
			Criteria:        []resource.Criterion{{}},
		},
		{
			Key:             "cluster-aws-eks",
			ResourceTypeKey: "k8s-cluster",
			DriverType:      resource.DriverTerraform,
			DriverInputs: terraformInputs(modules["eks"], map[string]any{
				"name":               "${context.infra.resourceName}",
				"subnet_ids":         "${resources['vpc.default#@infra'].outputs.subnetIds}",
				"kubernetes_version": cloud.KubernetesVersion,
				"node_instance_type": cloud.NodeInstanceType,
				"node_capacity_type": cloud.NodeCapacityType,
				"node_ami_type":      cloud.NodeAmiType,
				"node_disk_size":     cloud.NodeDiskSize,
			}, nil, []string{"name", "endpoint", "kubeContext", "kubeconfig"}),
			Criteria: []resource.Criterion{{Class: "eks"}},
		},
		{
			Key:             "vpc-aws",
			ResourceTypeKey: "vpc",
			DriverType:      resource.DriverTerraform,
			DriverInputs: terraformInputs(modules["vpc"], map[string]any{
				"name": "${context.infra.resourceName}",
				"cidr": cloud.VPCCidr,
			}, nil, []string{"id", "cidr", "subnetIds"}),
			Criteria: []resource.Criterion{{}},
		},
		{
			Key:              "postgres-internal-statefulset",
			ResourceTypeKey:  "postgres",
			ExecutionProfile: "internal-k8s",
			DriverType:       resource.DriverKubernetes,
			DriverInputs: driverInputs(map[string]any{
				"image":     o.Images.Postgres,
				"storage":   "1Gi",
				"namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}",
			}),
			Criteria: []resource.Criterion{{}},
		},
		{
			Key:              "postgres-aws-aurora",
			ResourceTypeKey:  "postgres",
			ExecutionProfile: "aws-eks",
			DriverType:       resource.DriverTerraform,
			DriverInputs: terraformInputs(modules["aurora"], map[string]any{
				"name":           "${context.infra.resourceName}",
				"engine_version": cloud.AuroraEngineVersion,
				"min_capacity":   cloud.AuroraMinACU,
				"max_capacity":   cloud.AuroraMaxACU,
				"vpc_id":         "${resources['vpc.default#@infra'].outputs.id}",
				"vpc_cidr":       "${resources['vpc.default#@infra'].outputs.cidr}",
				"subnet_ids":     "${resources['vpc.default#@infra'].outputs.subnetIds}",
			}, []string{"database", "username"}, []string{"host", "port", "database", "username", "password"}),
			Criteria: []resource.Criterion{{}},
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
func AllowsFixedTestAccounts(profile string) bool { return profile == "local" || profile == "test" }

// Apply writes the seeded catalog, both Applications and their Environments.
func Apply(ctx context.Context, store persistence.Store, o Options) error {
	if _, err := store.GetOrganization(ctx, o.OrganizationKey); errors.Is(err, persistence.ErrNotFound) {
		if err := store.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: o.OrganizationKey, Name: "Acme", DefaultConnectionKey: o.ConnectionKey}); err != nil {
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
	for _, t := range ResourceTypes() {
		if err := store.SaveResourceType(ctx, o.OrganizationKey, t); err != nil {
			return err
		}
	}
	if err := refreshSeededDefinitions(ctx, store, o); err != nil {
		return err
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

	internal := application.Application{
		ID:              ids.New(),
		Key:             o.ApplicationKey,
		OrganizationKey: o.OrganizationKey,
		Name:            o.ApplicationName,
		Subdomain:       "acceptance",
		Version:         1,
	}
	cloud := application.Application{
		ID:              ids.New(),
		Key:             o.CloudApplicationKey,
		OrganizationKey: o.OrganizationKey,
		Name:            o.CloudApplicationName,
		Subdomain:       "acceptance-cloud",
		Version:         1,
	}

	// Each acceptance Environment is bound once with explicit ENVIRONMENT
	// infrastructure scope (ADR-011).
	type appEnv struct {
		app       application.Application
		namespace string
		binding   persistence.EnvironmentBinding
	}
	pairs := []appEnv{{app: internal, namespace: o.NamespaceIdentity, binding: persistence.EnvironmentBinding{
		ConnectionKey: o.ConnectionKey, Profile: application.ProfileInternalK8s, RuntimeStatus: application.RuntimeReady, Scope: environment.ScopeEnvironment}}}
	if o.Region != "" {
		// The cloud Application is only usable once a region is configured.
		pairs = append(pairs, appEnv{app: cloud, namespace: o.CloudNamespaceIdentity, binding: persistence.EnvironmentBinding{
			ConnectionKey: o.CloudConnectionKey, Profile: application.ProfileAWSEKS, Region: o.Region, RuntimeStatus: application.RuntimePending, Scope: environment.ScopeEnvironment}})
	}

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
			if existing.Configured() {
				continue
			}
			pair.binding.ApplicationKey, pair.binding.EnvironmentKey, pair.binding.ExpectedVersion = pair.app.Key, o.EnvironmentKey, existing.Version
			if _, err := store.BindEnvironment(ctx, pair.binding); err != nil {
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
			pair.binding.ApplicationKey, pair.binding.EnvironmentKey, pair.binding.ExpectedVersion = pair.app.Key, o.EnvironmentKey, env.Version
			_, err := store.BindEnvironment(ctx, pair.binding)
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
