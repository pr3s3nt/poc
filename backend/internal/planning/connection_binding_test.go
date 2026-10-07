package planning

import (
	"errors"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
)

func labCluster(connection string) resource.Definition {
	return resource.Definition{
		Key: "cluster-lab", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster,
		ExecutionProfile: "internal-k8s", ConnectionKey: connection,
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:     []resource.Criterion{{Class: "internal", ResourceID: "connections.lab"}},
	}
}

// UC-06 BR-20: the Environment connection selects the cluster node; a winning
// existing-cluster Definition for another connection is rejected, not used.
func TestPlan_InternalEnvironmentConnectionMustMatchWinningClusterDefinition(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.Env.ConnectionKey = "lab"
	req.Connection.Key = "lab"

	// Only the seeded cluster Definition (connection internal-cluster) matches.
	_, err := NewService().Plan(req)
	if !errors.Is(err, ErrConnectionMismatch) {
		t.Fatalf("seed definition must not retarget the lab Environment: %v", err)
	}
	if message, ok := PublicMessage(err); !ok || !IsPublicMessage(message) {
		t.Fatalf("mismatch must use a fixed public message: %q %v", message, ok)
	}

	// A matching Definition registered for the new connection wins by
	// specificity (resource ID criterion) and plans.
	req.Catalog.Definitions = append(req.Catalog.Definitions, labCluster("lab"))
	plan, err := NewService().Plan(req)
	if err != nil {
		t.Fatalf("matching definition: %v", err)
	}
	match := plan.Matches["k8s-cluster.internal#connections.lab"]
	if match.DefinitionKey != "cluster-lab" || match.ConnectionKey != "lab" {
		t.Fatalf("cluster match = %+v", match)
	}

	// The same specificity rules apply: a winner with another explicit
	// connection is rejected even when a less specific one would match.
	req.Catalog.Definitions[len(req.Catalog.Definitions)-1] = labCluster("internal-cluster")
	if _, err := NewService().Plan(req); !errors.Is(err, ErrConnectionMismatch) {
		t.Fatalf("winner with another connection must be rejected: %v", err)
	}
}

func TestPlan_KubernetesDefinitionConnectionMustMatchInternalEnvironment(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	for i, def := range req.Catalog.Definitions {
		if def.Key == "namespace-kubernetes" {
			req.Catalog.Definitions[i].ConnectionKey = "someone-else"
		}
	}
	if _, err := NewService().Plan(req); !errors.Is(err, ErrConnectionMismatch) {
		t.Fatalf("explicit kubernetes connection must match: %v", err)
	}
	for i, def := range req.Catalog.Definitions {
		if def.Key == "namespace-kubernetes" {
			req.Catalog.Definitions[i].ConnectionKey = req.Env.ConnectionKey
		}
	}
	if _, err := NewService().Plan(req); err != nil {
		t.Fatalf("equal connection: %v", err)
	}
}

func withConnection(req *Request, key, connection string) {
	for i, def := range req.Catalog.Definitions {
		if def.Key == key {
			req.Catalog.Definitions[i].ConnectionKey = connection
		}
	}
}

// UC-06 BR-20 for aws-eks: Terraform VPC and EKS must use the Environment
// account; external databases keep their own Driver Account.
func TestPlan_AWSTargetTerraformDefinitionsMustUseEnvironmentConnection(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	if _, err := NewService().Plan(req); err != nil {
		t.Fatalf("matching account: %v", err)
	}
	for _, key := range []string{"vpc-aws", "cluster-aws-eks"} {
		bad := testRequest(t, application.ProfileAWSEKS, "backend")
		withConnection(&bad, key, "other-account")
		if _, err := NewService().Plan(bad); !errors.Is(err, ErrConnectionMismatch) {
			t.Fatalf("%s with another account: %v", key, err)
		}
	}
	external := testRequest(t, application.ProfileAWSEKS, "backend")
	withConnection(&external, "postgres-aws-aurora", "database-account")
	plan, err := NewService().Plan(external)
	if err != nil {
		t.Fatalf("external database keeps its Driver Account: %v", err)
	}
	for _, match := range plan.Matches {
		if match.DefinitionKey == "postgres-aws-aurora" && match.ConnectionKey != "database-account" {
			t.Fatalf("database connection changed: %+v", match)
		}
	}
}
