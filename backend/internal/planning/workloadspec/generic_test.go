package workloadspec

import (
	"orchestrator/internal/domain/resource"
	"testing"
)

func TestGenericWorkloadShorthandAndZeroReplicas(t *testing.T) {
	doc, err := Parse([]byte(`{"name":"order-api","image":"example/order:1","replicas":0,"ports":[{"name":"http","containerPort":8080}],"service":{"enabled":true,"port":80},"env":{"LOG_LEVEL":"info"}}`))
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := doc.Fragment(map[string]resource.Type{})
	if err != nil {
		t.Fatal(err)
	}
	if fragment.WorkloadID != "order-api" || fragment.Module.Spec.Replicas == nil || *fragment.Module.Spec.Replicas != 0 {
		t.Fatal("workload identity/explicit zero lost")
	}
	if fragment.Module.Spec.Service.Ports["http"].TargetPort != 8080 || fragment.Module.Spec.Service.Ports["http"].Port != 80 {
		t.Fatal("service mapping lost")
	}
	if fragment.Module.Spec.Containers["main"].Variables["LOG_LEVEL"] != "info" {
		t.Fatal("env lost")
	}
	roundtrip, err := FromMap(doc.Map())
	if err != nil {
		t.Fatal(err)
	}
	if !roundtrip.replicaSet || roundtrip.Replicas != 0 {
		t.Fatal("zero lost on roundtrip")
	}
}
