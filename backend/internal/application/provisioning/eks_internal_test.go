package provisioning

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
)

// ADR-017: the typed target of a cluster created in the same execution.

func eksPins() []tfmodule.ProviderPin {
	return []tfmodule.ProviderPin{{Name: "aws", Kind: tfmodule.BindingAWS, Target: tfmodule.TargetAccount, ConnectionKey: "aws-a", ConnectionKind: "AWS", AccountVersion: 5, Region: "us-east-1"}}
}

func eksResult() execution.ProvisionResult {
	return execution.ProvisionResult{
		Outputs: map[string]any{"name": "orch-shop"},
		Target:  &execution.Target{Kind: "kubernetes", Provider: "eks", ClusterName: "orch-shop", Organization: "acme", Connection: "aws-a", Region: "us-east-1"},
	}
}

func TestCompleteEKSTargetPublishesTheTypedClusterOutputWithTheOriginalIdentity(t *testing.T) {
	result := eksResult()
	if err := completeEKSTarget("k8s-cluster.eks#environments.shop.dev", eksPins(), &result); err != nil {
		t.Fatal(err)
	}
	out := result.Outputs[resource.ClusterTargetOutput]
	if err := resource.ValidateK8sTarget(out); err != nil {
		t.Fatalf("the published target must satisfy the discriminated validator: %v", err)
	}
	ref, err := execution.ParseClusterOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	want := execution.Target{Kind: "kubernetes", ClusterName: "orch-shop", Organization: "acme", Connection: "aws-a", Provider: "eks", Region: "us-east-1"}
	if !reflect.DeepEqual(ref.Target, want) || ref.AccountVersion != 5 || ref.Descriptor != "k8s-cluster.eks#environments.shop.dev" {
		t.Fatalf("ref = %+v", ref)
	}
	// No credential, endpoint, kubeconfig, path or context can be part of the published value.
	text := fmt.Sprint(out)
	for _, forbidden := range []string{"token", "kubeconfig", "endpoint", "/tmp", "context"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("the target mentions %q: %s", forbidden, text)
		}
	}
}

func TestCompleteEKSTargetRefusesAClusterThatIsNotTheOneThePlanPinned(t *testing.T) {
	for name, mutate := range map[string]func(*execution.ProvisionResult, *[]tfmodule.ProviderPin){
		"another account": func(r *execution.ProvisionResult, _ *[]tfmodule.ProviderPin) { r.Target.Connection = "aws-b" },
		"another region":  func(r *execution.ProvisionResult, _ *[]tfmodule.ProviderPin) { r.Target.Region = "eu-west-1" },
		"no cluster name": func(r *execution.ProvisionResult, _ *[]tfmodule.ProviderPin) { r.Target.ClusterName = "" },
		"no AWS pin":      func(_ *execution.ProvisionResult, p *[]tfmodule.ProviderPin) { *p = nil },
		"no account pin":  func(_ *execution.ProvisionResult, p *[]tfmodule.ProviderPin) { (*p)[0].AccountVersion = 0 },
	} {
		result, pins := eksResult(), eksPins()
		mutate(&result, &pins)
		if err := completeEKSTarget("k8s-cluster.eks#e", pins, &result); !errors.Is(err, ErrClusterTarget) {
			t.Errorf("%s: err = %v", name, err)
		}
		if _, published := result.Outputs[resource.ClusterTargetOutput]; published {
			t.Errorf("%s: a target was published for a mismatching cluster", name)
		}
	}
	// An existing cluster publishes its own target; the platform adds nothing.
	existing := execution.ProvisionResult{Outputs: map[string]any{}, Target: &execution.Target{Kind: "kubernetes", Context: "ctx", ClusterName: "k8s-4f", Organization: "acme", Connection: "k8s-a"}}
	if err := completeEKSTarget("k8s-cluster.internal#e", nil, &existing); err != nil || len(existing.Outputs) != 0 {
		t.Fatalf("existing cluster: %v %v", err, existing.Outputs)
	}
}

func TestEKSTargetFormsAreDiscriminatedAndNeverMixed(t *testing.T) {
	good := map[string]any{"kind": resource.K8sTargetKind, "clusterName": "c", "organization": "acme", "account": "aws-a", "accountVersion": float64(5), "descriptor": "k8s-cluster.eks#e", "provider": "eks", "region": "us-east-1"}
	if err := resource.ValidateK8sTarget(good); err != nil {
		t.Fatal(err)
	}
	internal := map[string]any{"kind": resource.K8sTargetKind, "clusterName": "c", "organization": "acme", "account": "k8s-a", "context": "ctx", "descriptor": "k8s-cluster.internal#e", "accountVersion": float64(2)}
	if err := resource.ValidateK8sTarget(internal); err != nil {
		t.Fatal(err)
	}
	clone := func(m map[string]any, edit func(map[string]any)) map[string]any {
		out := map[string]any{}
		for k, v := range m {
			out[k] = v
		}
		edit(out)
		return out
	}
	bad := map[string]map[string]any{
		"internal with a region":         clone(internal, func(m map[string]any) { m["region"] = "us-east-1" }),
		"internal with a provider":       clone(internal, func(m map[string]any) { m["provider"] = "eks" }),
		"eks with a kube context":        clone(good, func(m map[string]any) { m["context"] = "ctx" }),
		"eks without a region":           clone(good, func(m map[string]any) { delete(m, "region") }),
		"eks with an invalid region":     clone(good, func(m map[string]any) { m["region"] = "../../etc" }),
		"eks without an account version": clone(good, func(m map[string]any) { delete(m, "accountVersion") }),
		"eks without the descriptor":     clone(good, func(m map[string]any) { delete(m, "descriptor") }),
		"unknown provider":               clone(good, func(m map[string]any) { m["provider"] = "gke" }),
		"credential-bearing field":       clone(good, func(m map[string]any) { m["token"] = "x" }),
		"kubeconfig path":                clone(good, func(m map[string]any) { m["kubeconfig"] = "/tmp/kube" }),
		"endpoint":                       clone(good, func(m map[string]any) { m["endpoint"] = "https://x" }),
	}
	for name, value := range bad {
		if err := resource.ValidateK8sTarget(value); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func eksPlacementPlan() *planning.Plan {
	return &planning.Plan{Placements: map[string]planning.Placement{
		"workload.default#modules.web": {Node: "workload.default#modules.web", Cluster: "k8s-cluster.eks#e", Account: "aws-a", AccountVersion: 5, Namespace: "k8s-namespace.default#e", Provider: planning.ClusterEKS, Region: "us-east-1"},
	}}
}

func eksWorkloadResult(clusterValue map[string]any, nsCluster execution.ClusterRef) *Result {
	return &Result{
		Outputs:      map[string]map[string]any{"k8s-cluster.eks#e": {"cluster": clusterValue}, "k8s-namespace.default#e": {"name": "orch-ns"}},
		NodeClusters: map[string]execution.ClusterRef{"k8s-namespace.default#e": nsCluster},
	}
}

func TestResolveWorkloadTargetBindsTheProducingEKSClusterOnly(t *testing.T) {
	cluster := execution.Target{Kind: "kubernetes", Provider: "eks", ClusterName: "orch-shop", Organization: "acme", Connection: "aws-a", Region: "us-east-1"}
	value := cluster.ClusterOutput("k8s-cluster.eks#e", 5)
	nsRef := execution.ClusterRef{Target: cluster, Descriptor: "k8s-cluster.eks#e", AccountVersion: 5}
	target, err := ResolveWorkloadTarget(eksPlacementPlan(), eksWorkloadResult(value, nsRef), "workload.default#modules.web", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if target.Provider != "eks" || target.Region != "us-east-1" || target.Connection != "aws-a" || target.Namespace != "orch-ns" || target.ClusterName != "orch-shop" {
		t.Fatalf("target = %+v", target)
	}
	for name, edit := range map[string]func(*Result){
		"another region": func(r *Result) {
			r.Outputs["k8s-cluster.eks#e"]["cluster"] = execution.Target{Provider: "eks", ClusterName: "orch-shop", Organization: "acme", Connection: "aws-a", Region: "eu-west-1"}.ClusterOutput("k8s-cluster.eks#e", 5)
		},
		"another account": func(r *Result) {
			r.Outputs["k8s-cluster.eks#e"]["cluster"] = execution.Target{Provider: "eks", ClusterName: "orch-shop", Organization: "acme", Connection: "aws-b", Region: "us-east-1"}.ClusterOutput("k8s-cluster.eks#e", 5)
		},
		"stale version": func(r *Result) {
			r.Outputs["k8s-cluster.eks#e"]["cluster"] = cluster.ClusterOutput("k8s-cluster.eks#e", 4)
		},
		"internal form": func(r *Result) {
			r.Outputs["k8s-cluster.eks#e"]["cluster"] = execution.Target{ClusterName: "k", Organization: "acme", Connection: "aws-a", Context: "c"}.ClusterOutput("k8s-cluster.eks#e", 5)
		},
		"namespace on other": func(r *Result) {
			other := nsRef
			other.Target.Region = "eu-west-1"
			r.NodeClusters["k8s-namespace.default#e"] = other
		},
	} {
		r := eksWorkloadResult(value, nsRef)
		edit(r)
		if _, err := ResolveWorkloadTarget(eksPlacementPlan(), r, "workload.default#modules.web", "acme"); !errors.Is(err, ErrWorkloadTarget) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}

func TestCheckPinsAgainstPlanBindsAnEKSKubernetesPinToTheOriginalAWSAccount(t *testing.T) {
	cluster := "k8s-cluster.eks#environments.shop.dev"
	plan := pinPlan(cluster)
	ctx := planning.Context{Env: environment.Environment{}}
	pin := tfmodule.ProviderPin{Name: "kubernetes", Kind: tfmodule.BindingKubernetes, Target: tfmodule.TargetResource, Provider: cluster, ClusterProvider: planning.ClusterEKS, ClusterAccount: "aws-a", AccountVersion: 5, Region: "us-east-1"}
	account := application.Connection{Key: "aws-a", Kind: application.ConnectionAWS, Version: 5}
	if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{pin}, resource.Definition{Key: "d"}, account, ctx); err != nil {
		t.Fatalf("a matching EKS pin: %v", err)
	}
	for name, edit := range map[string]func(*tfmodule.ProviderPin, *application.Connection){
		"account version drift": func(_ *tfmodule.ProviderPin, a *application.Connection) { a.Version = 6 },
		"other account":         func(_ *tfmodule.ProviderPin, a *application.Connection) { a.Key = "aws-b" },
		"a Kubernetes account":  func(_ *tfmodule.ProviderPin, a *application.Connection) { a.Kind = application.ConnectionKubernetes },
		"a Connection key set":  func(p *tfmodule.ProviderPin, _ *application.Connection) { p.ConnectionKey = "k8s-a" },
		"no cluster account":    func(p *tfmodule.ProviderPin, _ *application.Connection) { p.ClusterAccount = "" },
		"no region":             func(p *tfmodule.ProviderPin, _ *application.Connection) { p.Region = "" },
	} {
		p, a := pin, account
		edit(&p, &a)
		if err := checkPinsAgainstPlan(plan, "redis.default#modules.x", []tfmodule.ProviderPin{p}, resource.Definition{Key: "d"}, a, ctx); !errors.Is(err, ErrProviderPin) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}

func TestPublicClusterErrorKeepsOnlyAFixedCategory(t *testing.T) {
	raw := fmt.Errorf("%w: dial tcp 10.0.0.1:443 token=abc", execution.ErrClusterUnreachable)
	err := publicClusterError(raw)
	if !errors.Is(err, ErrClusterNotUsable) || !errors.Is(err, execution.ErrClusterUnreachable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "10.0.0.1") || strings.Contains(err.Error(), "token") {
		t.Fatalf("raw text leaked: %v", err)
	}
	if !errors.Is(publicClusterError(errors.New("anything else")), ErrClusterNotUsable) {
		t.Fatal("an unknown error is still the fixed sentence")
	}
}
