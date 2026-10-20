package planning_test

import (
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
)

// V06 (ADR-017): managed Aurora/ElastiCache per Definition account and region, actual network references (no blanket EKS edge),
// V06.06 managed-only graph (no unrequested EKS) and V06.07 contract coverage for multiple accounts/regions, ties, wrong permissions,
// incompatible drivers and invalid network. Simulated accounts only: no extra live account or region is needed. The catalog helpers
// live in definition_catalog_plan_test.go.

func TestDefinitionCatalog_ManagedResourcesDependOnTheirNetworkNotOnEKS(t *testing.T) {
	for _, scenario := range []string{"A02", "A03"} {
		plan, err := placedPlan(t, catalogRequest(t, scenario, nil).req)
		if err != nil {
			t.Fatalf("%s: %v", scenario, err)
		}
		if !hasEdge(plan, auroraNode, vpcNode, planning.ReasonReference) {
			t.Fatalf("%s: Aurora must reference the VPC it needs: %+v", scenario, plan.Graph.Edges)
		}
		for _, e := range plan.Graph.Edges {
			if (e.Consumer == auroraNode || e.Consumer == cacheNode) && (e.Provider == eksNode || e.Provider == nsNode) {
				t.Fatalf("%s: a managed resource must not acquire an EKS/namespace edge just because it shares an Environment: %+v", scenario, e)
			}
		}
		// The managed resources are scheduled with the network, independently of the cluster chain.
		if batchOf(plan, auroraNode) <= batchOf(plan, vpcNode) {
			t.Fatalf("%s: Aurora must follow its VPC: %v", scenario, plan.Batches)
		}
		pin := pinOf(t, plan, auroraNode, tfmodule.BindingAWS)
		if pin.ConnectionKey != "aws-a" || pin.Region != "us-east-1" {
			t.Fatalf("%s: Aurora pin %+v", scenario, pin)
		}
	}
	a02, _ := placedPlan(t, catalogRequest(t, "A02", nil).req)
	if !hasEdge(a02, cacheNode, vpcNode, planning.ReasonReference) || hasNode(a02, redisEKSNode) {
		t.Fatalf("A02 uses ElastiCache: %+v", a02.Graph.Edges)
	}
	a03, _ := placedPlan(t, catalogRequest(t, "A03", nil).req)
	if !hasNode(a03, redisEKSNode) || hasNode(a03, cacheNode) || !hasEdge(a03, redisEKSNode, eksNode, planning.ReasonProviderBinding) {
		t.Fatalf("A03 uses Redis in-cluster on EKS: %+v", a03.Graph.Nodes)
	}
}

// V06.06: a plan that only asks for managed resources never acquires an EKS cluster or a namespace. A workload on an
// existing cluster with managed Aurora/ElastiCache creates neither. (No public execution entry for a workload-less
// environment exists: this is a planner contract, nothing else is claimed.)
func TestDefinitionCatalog_ManagedResourcesOnAnExistingClusterInsertNoEKS(t *testing.T) {
	cp := catalogRequest(t, "A02", func(defs []resource.Definition, _ map[string]application.Connection) []resource.Definition {
		out := []resource.Definition{}
		for _, def := range defs {
			if def.Key == "cluster-eks" || def.Key == "namespace-eks" || def.Key == "workload-render" {
				continue // the environment asks for no EKS
			}
			out = append(out, def)
		}
		out = append(out, clusterDefinition("cluster-existing", "k8s-a", resource.Criterion{Class: "internal"}),
			namespaceDefinition(t, "namespace-existing", "internal", resource.Criterion{Class: "default"}),
			workloadDefinition("workload-existing", nil, resource.Criterion{}))
		return out
	})
	plan, err := placedPlan(t, cp.req)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range plan.Graph.Nodes {
		if strings.HasPrefix(node.Descriptor, "k8s-cluster.eks") {
			t.Fatalf("an EKS cluster appeared although nobody requested one: %s", node.Descriptor)
		}
	}
	if !hasNode(plan, auroraNode) || !hasNode(plan, cacheNode) || !hasNode(plan, vpcNode) {
		t.Fatalf("managed resources and their network are planned: %+v", plan.Graph.Nodes)
	}
	if got := plan.Placements[backendNode]; got.Provider != planning.ClusterExisting {
		t.Fatalf("the workload is placed on the existing cluster: %+v", got)
	}
}

// V06.07 contract coverage (simulated accounts/regions; no extra live account is needed): accounts and regions are
// kept per Definition and are NOT required to be equal; invalid combinations fail without an alternate winner.
func TestDefinitionCatalog_AccountsAndRegionsAreKeptPerDefinitionAndNeverMustBeEqual(t *testing.T) {
	cp := catalogRequest(t, "A02", func(defs []resource.Definition, _ map[string]application.Connection) []resource.Definition {
		for i, def := range defs {
			if def.Key != "pg-aurora" && def.Key != "redis-elasticache" {
				continue
			}
			values := defs[i].DriverInputs["values"].(map[string]any)
			values["variables"].(map[string]any)["region"] = "eu-west-1"
			defs[i].DriverAccount = "aws-b"
		}
		return defs
	})
	plan, err := placedPlan(t, cp.req)
	if err != nil {
		t.Fatalf("different accounts and regions are valid when the references are: %v", err)
	}
	if eks := pinOf(t, plan, eksNode, tfmodule.BindingAWS); eks.ConnectionKey != "aws-a" || eks.Region != "us-east-1" || eks.AccountVersion != 5 {
		t.Fatalf("EKS pin = %+v", eks)
	}
	for _, node := range []string{auroraNode, cacheNode} {
		if pin := pinOf(t, plan, node, tfmodule.BindingAWS); pin.ConnectionKey != "aws-b" || pin.Region != "eu-west-1" || pin.AccountVersion != 9 {
			t.Fatalf("%s pin = %+v", node, pin)
		}
	}
	if plan.Placements[backendNode].Account != "aws-a" {
		t.Fatalf("the workload follows its cluster's account, never the database's: %+v", plan.Placements[backendNode])
	}
}

func TestDefinitionCatalog_InvalidCombinationsFailWithoutAnAlternateWinner(t *testing.T) {
	var staged *planning.StageError
	cases := map[string]struct {
		mutate func(defs []resource.Definition, accounts map[string]application.Connection) []resource.Definition
		assert func(t *testing.T, err error)
	}{
		"wrong permissions: the winning account is not READY": {
			mutate: func(defs []resource.Definition, a map[string]application.Connection) []resource.Definition {
				c := a["aws-a"]
				c.Status = application.ConnectionVerifying
				a["aws-a"] = c
				return defs
			},
			assert: func(t *testing.T, err error) {
				if !errors.Is(err, planning.ErrAccountConflict) {
					t.Fatalf("err = %v", err)
				}
			},
		},
		"incompatible driver: an AWS module with an Internal account": {
			mutate: func(defs []resource.Definition, a map[string]application.Connection) []resource.Definition {
				for i := range defs {
					if defs[i].Key == "pg-aurora" {
						defs[i].DriverAccount = "k8s-a"
					}
				}
				return defs
			},
			assert: func(t *testing.T, err error) {
				if !errors.Is(err, planning.ErrAccountConflict) {
					t.Fatalf("err = %v", err)
				}
			},
		},
		"missing region: the Definition names no literal AWS region": {
			mutate: func(defs []resource.Definition, a map[string]application.Connection) []resource.Definition {
				for i := range defs {
					if defs[i].Key == "pg-aurora" {
						delete(defs[i].DriverInputs["values"].(map[string]any)["variables"].(map[string]any), "region")
					}
				}
				return defs
			},
			assert: func(t *testing.T, err error) {
				if err == nil || !strings.Contains(err.Error(), "region") {
					t.Fatalf("err = %v", err)
				}
			},
		},
		"tie: two equally specific Aurora Definitions": {
			mutate: func(defs []resource.Definition, a map[string]application.Connection) []resource.Definition {
				for _, def := range defs {
					if def.Key == "pg-aurora" {
						twin := def
						twin.Key = "pg-aurora-twin"
						defs = append(defs, twin)
					}
				}
				return defs
			},
			assert: func(t *testing.T, err error) {
				if !errors.As(err, &staged) || staged.Reason != planning.ReasonResourceAmbiguous {
					t.Fatalf("err = %v", err)
				}
			},
		},
		"invalid network: Aurora references an output the VPC Type does not declare": {
			mutate: func(defs []resource.Definition, a map[string]application.Connection) []resource.Definition {
				for i := range defs {
					if defs[i].Key == "pg-aurora" {
						defs[i].DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["vpc_id"] = "${resources['vpc.default#environments.@app.@env'].outputs.vpcId}"
					}
				}
				return defs
			},
			assert: func(t *testing.T, err error) {
				if !errors.As(err, &staged) || staged.Stage != planning.StageReference {
					t.Fatalf("err = %v", err)
				}
			},
		},
		"incomplete network: Aurora without its VPC wiring": {
			mutate: func(defs []resource.Definition, a map[string]application.Connection) []resource.Definition {
				for i := range defs {
					if defs[i].Key == "pg-aurora" {
						delete(defs[i].DriverInputs["values"].(map[string]any)["variables"].(map[string]any), "vpc_id")
					}
				}
				return defs
			},
			assert: func(t *testing.T, err error) {
				if err == nil {
					t.Fatal("Aurora without vpc_id was planned")
				}
			},
		},
		"dependency: no Definition provides the VPC": {
			mutate: func(defs []resource.Definition, a map[string]application.Connection) []resource.Definition {
				out := []resource.Definition{}
				for _, def := range defs {
					if def.Key != "vpc-aws" {
						out = append(out, def)
					}
				}
				return out
			},
			assert: func(t *testing.T, err error) {
				if !errors.As(err, &staged) || staged.Reason != planning.ReasonResourceNoMatch {
					t.Fatalf("err = %v", err)
				}
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			plan, err := placedPlan(t, catalogRequest(t, "A02", tc.mutate).req)
			if plan != nil || err == nil {
				t.Fatalf("an invalid combination planned: %v", plan)
			}
			tc.assert(t, err)
		})
	}
}

// The Aurora name derives from the same Environment identity and keeps the cluster/writer suffixes inside 63 characters.
func TestDefinitionCatalog_AuroraNameIsBoundedAndEqualsTheNetworkIdentity(t *testing.T) {
	cp := catalogRequest(t, "A03", nil)
	cp.req.RunID = "envconn-kind-20261007121743-27878-with-a-very-long-suffix"
	plan, err := placedPlan(t, cp.req)
	if err != nil {
		t.Fatal(err)
	}
	aurora, _ := contractInput(t, plan, "pg-aurora", "name").(string)
	vpc, _ := contractInput(t, plan, "vpc-aws", "name").(string)
	if aurora == "" || aurora != vpc || len(aurora) > 49 || len(aurora+"-cluster") > 63 || len(aurora+"-writer") > 63 {
		t.Fatalf("aurora name %q, vpc name %q", aurora, vpc)
	}
}

// Several managed resources of one Environment share ONE network node: the VPC is a resource reference, never repeated per
// database or cache, and every managed resource orders after it.
func TestDefinitionCatalog_TwoDatabasesAndTwoCachesShareOneVPCNode(t *testing.T) {
	cp := catalogRequestWithScore(t, "A02", nil, func(doc map[string]any) {
		resources := doc["resources"].(map[string]any)
		for name, copyOf := range map[string]string{"db2": "db", "cache2": "cache"} {
			src := resources[copyOf].(map[string]any)
			dup := map[string]any{}
			for k, v := range src {
				dup[k] = v
			}
			dup["id"] = copyOf + "-two"
			resources[name] = dup
		}
	})
	plan, err := placedPlan(t, cp.req)
	if err != nil {
		t.Fatal(err)
	}
	vpcs, managed := 0, 0
	for _, node := range plan.Graph.Nodes {
		switch {
		case node.ResourceType == "vpc":
			vpcs++
		case node.Class == "cloud-managed":
			managed++
			if !hasEdge(plan, node.Descriptor, vpcNode, planning.ReasonReference) {
				t.Errorf("%s does not order after the shared VPC", node.Descriptor)
			}
		}
	}
	if vpcs != 1 || managed != 4 {
		t.Fatalf("want one VPC and four managed resources, got %d VPC and %d managed: %+v", vpcs, managed, plan.Graph.Nodes)
	}
}
