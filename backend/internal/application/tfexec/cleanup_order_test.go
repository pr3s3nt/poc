package tfexec

import (
	"reflect"
	"testing"
)

func TestReverseTopologicalCleanupOrderPrioritizesIndependentLayers(t *testing.T) {
	// This is the AWS acceptance dependency graph: namespace -> EKS -> VPC,
	// with Aurora and ElastiCache as independent VPC consumers. IDs deliberately
	// sort in the order that used to let cleanup walk EKS/VPC before the DBs.
	ids := []string{"z-vpc", "y-eks", "a-namespace", "b-aurora", "c-elasticache"}
	layers := map[string]int{"z-vpc": 3, "y-eks": 2, "a-namespace": 1, "b-aurora": 0, "c-elasticache": 0}
	consumers := map[string][]string{
		"z-vpc": {"y-eks", "b-aurora", "c-elasticache"},
		"y-eks": {"a-namespace"},
	}
	got, err := reverseTopologicalCleanupOrder(ids, consumers, func(id string) int { return layers[id] })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b-aurora", "c-elasticache", "a-namespace", "y-eks", "z-vpc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestCleanupPriorityUsesCanonicalResourceTypes(t *testing.T) {
	for _, tc := range []struct {
		descriptor string
		want       int
	}{
		{"postgres.cloud-managed#shared.acceptance-db", 0},
		{"redis.cloud-managed#shared.acceptance-cache", 0},
		{"k8s-namespace.default#env", 1},
		{"k8s-cluster.eks#env", 2},
		{"vpc.default#env", 3},
	} {
		if got := cleanupPriority(tc.descriptor); got != tc.want {
			t.Errorf("cleanupPriority(%q) = %d, want %d", tc.descriptor, got, tc.want)
		}
	}
}

func TestReverseTopologicalCleanupOrderDependencyOverridesLayerPreference(t *testing.T) {
	// The application resource is the provider (priority 0); the namespace is
	// its consumer (priority 1). The edge must force the lower-priority
	// consumer to be destroyed first.
	ids := []string{"z-application-provider", "a-namespace-consumer"}
	got, err := reverseTopologicalCleanupOrder(ids, map[string][]string{"z-application-provider": {"a-namespace-consumer"}}, func(id string) int {
		if id == "z-application-provider" {
			return 0
		}
		return 1
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a-namespace-consumer", "z-application-provider"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestReverseTopologicalCleanupOrderRejectsCycles(t *testing.T) {
	_, err := reverseTopologicalCleanupOrder([]string{"a", "b"}, map[string][]string{"a": {"b"}, "b": {"a"}}, func(string) int { return 0 })
	if err == nil {
		t.Fatal("expected a dependency cycle to be rejected")
	}
}
