// Package seed registers the Phase 6 catalog: Resource Types, Resource
// Definitions, Connections, Applications and Environments. Both Execution
// Profiles are registered, so a single process can serve an internal-k8s and an
// aws-eks Application and matching can tell them apart with `app_id`.
package seed

import (
	"context"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
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

func driverInputs(module string, variables map[string]any) map[string]any {
	values := map[string]any{"variables": variables}
	if module != "" {
		values["source"] = map[string]any{"module": module}
	}
	return map[string]any{"values": values}
}

// ResourceDefinitions returns the Resource Definitions of both Execution
// Profiles (UC-03). Matching uses only the five standard criteria fields.
func ResourceDefinitions(o Options) []resource.Definition {
	cloud := o.Cloud
	if cloud.KubernetesVersion == "" {
		cloud = DefaultCloudConfig()
	}
	return []resource.Definition{
		{
			Key:             "namespace-kubernetes",
			ResourceTypeKey: "k8s-namespace",
			DriverType:      resource.DriverKubernetes,
			DriverInputs:    driverInputs("", map[string]any{"name": "${context.env.namespace}"}),
			Criteria:        []resource.Criterion{{}},
		},
		{
			Key:             "cluster-internal-registered",
			ResourceTypeKey: "k8s-cluster",
			DriverType:      resource.DriverExistingCluster,
			ConnectionKey:   o.ConnectionKey,
			DriverInputs: driverInputs("", map[string]any{
				"name":        "${context.connection.cluster}",
				"kubeContext": "${context.connection.context}",
			}),
			Criteria: []resource.Criterion{{Class: "internal"}},
		},
		{
			Key:             "cluster-aws-eks",
			ResourceTypeKey: "k8s-cluster",
			DriverType:      resource.DriverTerraform,
			ConnectionKey:   o.CloudConnectionKey,
			DriverInputs: driverInputs("eks", map[string]any{
				"name":               "${context.app.id}-${context.run.id}",
				"subnet_ids":         "${resources['vpc.default#applications.@app'].outputs.subnetIds}",
				"kubernetes_version": cloud.KubernetesVersion,
				"node_instance_type": cloud.NodeInstanceType,
				"node_capacity_type": cloud.NodeCapacityType,
				"node_ami_type":      cloud.NodeAmiType,
				"node_disk_size":     cloud.NodeDiskSize,
			}),
			Criteria: []resource.Criterion{{Class: "eks"}},
		},
		{
			Key:             "vpc-aws",
			ResourceTypeKey: "vpc",
			DriverType:      resource.DriverTerraform,
			ConnectionKey:   o.CloudConnectionKey,
			DriverInputs: driverInputs("vpc", map[string]any{
				"name": "${context.app.id}-${context.run.id}",
				"cidr": cloud.VPCCidr,
			}),
			Criteria: []resource.Criterion{{}},
		},
		{
			Key:             "postgres-internal-statefulset",
			ResourceTypeKey: "postgres",
			DriverType:      resource.DriverKubernetes,
			DriverInputs: driverInputs("", map[string]any{
				"image":     o.Images.Postgres,
				"storage":   "1Gi",
				"namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}",
			}),
			Criteria: []resource.Criterion{{ApplicationID: o.ApplicationKey}},
		},
		{
			Key:             "postgres-aws-aurora",
			ResourceTypeKey: "postgres",
			DriverType:      resource.DriverTerraform,
			ConnectionKey:   o.CloudConnectionKey,
			DriverInputs: driverInputs("aurora", map[string]any{
				"name":           "${context.app.id}-${context.run.id}",
				"engine_version": cloud.AuroraEngineVersion,
				"min_capacity":   cloud.AuroraMinACU,
				"max_capacity":   cloud.AuroraMaxACU,
				"vpc_id":         "${resources['vpc.default#applications.@app'].outputs.id}",
				"vpc_cidr":       "${resources['vpc.default#applications.@app'].outputs.cidr}",
				"subnet_ids":     "${resources['vpc.default#applications.@app'].outputs.subnetIds}",
			}),
			Criteria: []resource.Criterion{{ApplicationID: o.CloudApplicationKey}},
		},
	}
}

// Apply writes the seeded catalog, both Applications and their Environments.
func Apply(ctx context.Context, store persistence.Store, o Options) error {
	if err := store.SaveOrganization(ctx, application.Organization{Key: o.OrganizationKey, Name: "Acme", DefaultConnectionKey: o.ConnectionKey}); err != nil {
		return err
	}
	if o.Profile == "local" || o.Profile == "test" {
		passwordHash, err := password.Hash("test-password")
		if err != nil {
			return err
		}
		if err := store.SaveUserAccount(ctx, identity.UserAccount{ID: "developer", OrganizationKey: o.OrganizationKey, Username: "developer", PasswordHash: passwordHash, Role: identity.RoleDeveloper, Status: identity.AccountActive}); err != nil {
			return err
		}
	}
	for _, t := range ResourceTypes() {
		if err := store.SaveResourceType(ctx, t); err != nil {
			return err
		}
	}
	for _, d := range ResourceDefinitions(o) {
		if err := store.SaveResourceDefinition(ctx, d); err != nil {
			return err
		}
	}

	internalConnection := application.Connection{
		Key:             o.ConnectionKey,
		OrganizationKey: o.OrganizationKey,
		Kind:            application.ConnectionKubernetes,
		Config:          map[string]any{"cluster": o.ClusterName, "kubeContext": o.KubeContext},
		SecretRef:       "secret://connections/" + o.ConnectionKey,
		Status:          application.ConnectionReady,
		Verification:    map[string]any{"cluster": o.ClusterName, "verified": true},
	}
	cloudConnection := application.Connection{
		Key:             o.CloudConnectionKey,
		OrganizationKey: o.OrganizationKey,
		Kind:            application.ConnectionAWS,
		Config:          map[string]any{"region": o.Region, "accountId": o.AccountID},
		SecretRef:       "secret://connections/" + o.CloudConnectionKey,
		Status:          application.ConnectionReady,
		Verification:    map[string]any{"accountId": o.AccountID, "region": o.Region, "verified": true},
	}
	for _, conn := range []application.Connection{internalConnection, cloudConnection} {
		if err := store.SaveConnection(ctx, conn); err != nil {
			return err
		}
	}

	internal := application.Application{
		Key:             o.ApplicationKey,
		OrganizationKey: o.OrganizationKey,
		Name:            o.ApplicationName,
		Subdomain:       "acceptance",
		Profile:         application.ProfileInternalK8s,
		ConnectionKey:   o.ConnectionKey,
		RuntimeStatus:   application.RuntimeReady,
		Version:         1,
	}
	cloud := application.Application{
		Key:             o.CloudApplicationKey,
		OrganizationKey: o.OrganizationKey,
		Name:            o.CloudApplicationName,
		Subdomain:       "acceptance-cloud",
		Profile:         application.ProfileAWSEKS,
		ConnectionKey:   o.CloudConnectionKey,
		Region:          o.Region,
		RuntimeStatus:   application.RuntimePending,
		Version:         1,
	}

	type appEnv struct {
		app       application.Application
		namespace string
	}
	pairs := []appEnv{{app: internal, namespace: o.NamespaceIdentity}}
	if o.Region != "" {
		// The cloud Application is only usable once a region is configured.
		pairs = append(pairs, appEnv{app: cloud, namespace: o.CloudNamespaceIdentity})
	}

	for _, pair := range pairs {
		if err := pair.app.Validate(); err != nil {
			return err
		}
		if err := store.SaveApplication(ctx, pair.app); err != nil {
			return err
		}
		emptySet := environment.DeploymentSet{
			ID:             ids.New(),
			EnvironmentKey: pair.app.Key + "/" + o.EnvironmentKey,
			Document:       environment.NewDocument(),
			DocumentHash:   "empty",
		}
		env := environment.Environment{
			Key:                    o.EnvironmentKey,
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
			return store.SaveEnvironment(ctx, env)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
