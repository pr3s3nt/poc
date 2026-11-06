package environment

import (
	"encoding/json"
	"reflect"
	"testing"
)

func pointer[T any](v T) *T { return &v }
func TestDefaultsPolicyIsOptInAndDoesNotRewriteDesired(t *testing.T) {
	legacy := ModuleSpec{Containers: map[string]Container{"main": {Image: "nginx:alpine"}}}
	effective, sources := legacy.ResolveDefaults()
	if !reflect.DeepEqual(legacy, effective) || sources["containers.main.image"] != "workload" {
		t.Fatal("legacy changed")
	}
	for _, env := range []string{"staging", "production"} {
		t.Run(env, func(t *testing.T) {
			spec := ModuleSpec{Replicas: pointer(0), Containers: map[string]Container{"main": {Image: "nginx:alpine", SecurityContext: &ContainerSecurityContext{RunAsNonRoot: pointer(false), RunAsUser: pointer(int64(0))}}, "init": {Image: "busybox", Role: "init", InitOrder: 1}}, Execution: &WorkloadExecution{Version: 1, Kind: "Deployment", Defaults: &DefaultsPolicy{Version: "workload-defaults-v1", Environment: env}, RevisionHistoryLimit: pointer(int32(0)), AutomountServiceAccountToken: pointer(false)}}
			before, _ := json.Marshal(spec)
			got, src := spec.ResolveDefaults()
			after, _ := json.Marshal(spec)
			if string(before) != string(after) {
				t.Fatal("resolver mutated desired")
			}
			if *got.Replicas != 0 || *got.Execution.RevisionHistoryLimit != 0 || *got.Execution.AutomountServiceAccountToken || *got.Containers["main"].SecurityContext.RunAsNonRoot {
				t.Fatal("false/zero lost")
			}
			cpu := "100m"
			if env == "production" {
				cpu = "200m"
			}
			if got.Containers["main"].Resources.Requests.CPU != cpu || got.Containers["main"].Resources.Limits.CPU != "" || got.Containers["init"].Resources != nil {
				t.Fatal("incorrect resources", got)
			}
			if src["containers.main.resources.requests.cpu"] != "environment" || src["workloadExecution.revisionHistoryLimit"] != "workload" {
				t.Fatal(src)
			}
			profile := &RuntimeDefaults{Resources: &ContainerResourceRequirements{Requests: &ComputeResources{CPU: "500m", Memory: "256Mi"}}}
			next, source := spec.ResolveDefaults(profile)
			if next.Containers["main"].Resources.Requests.CPU != "500m" || source["containers.main.resources.requests.cpu"] != "template" {
				t.Fatal("profile precedence")
			}
		})
	}
}
func TestRuntimePolicyValidationRejectsIncompatibleControllerFields(t *testing.T) {
	containers := map[string]Container{"main": {Image: "busybox"}}
	for _, raw := range []string{
		`{"version":1,"kind":"Deployment","updateStrategy":{"type":"Recreate","rollingUpdate":{"maxSurge":1}}}`,
		`{"version":1,"kind":"StatefulSet","updateStrategy":{"type":"RollingUpdate","rollingUpdate":{"maxSurge":1}}}`,
		`{"version":1,"kind":"CronJob","revisionHistoryLimit":0}`,
		`{"version":1,"kind":"Deployment","podDisruptionBudget":{"enabled":true,"minAvailable":1,"maxUnavailable":1}}`,
		`{"version":1,"kind":"Deployment","podDisruptionBudget":{"enabled":true,"maxUnavailable":"101%"}}`,
		`{"version":1,"kind":"StatefulSet","persistentVolumeClaimRetentionPolicy":{"whenDeleted":"Delete","whenScaled":"Retain"}}`,
		`{"version":1,"kind":"StatefulSet","volumeClaimTemplates":[{"name":"data","size":"1Gi","accessModes":["ReadWriteOnce"]}],"claimMounts":[{"claim":"missing","container":"main","path":"/data"}]}`,
	} {
		var e WorkloadExecution
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatal(err)
		}
		if err := e.ValidateRuntime(containers, 2); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	e := &WorkloadExecution{Kind: "Deployment", PodDisruptionBudget: &PodDisruptionBudget{Enabled: true, MaxUnavailable: "25%"}}
	if err := e.ValidateRuntime(containers, 2); err != nil {
		t.Fatal(err)
	}
	if err := e.ValidateRuntime(containers, 1); err == nil {
		t.Fatal("single replica PDB accepted")
	}
}

func TestProductionReplicasRequirePublishedCompatibility(t *testing.T) {
	profile := &RuntimeDefaults{ProductionReplicas: pointer(2), MultiReplicaSafe: true}
	for _, env := range []string{"staging", "production"} {
		spec := ModuleSpec{Containers: map[string]Container{"main": {Image: "image"}}, Execution: &WorkloadExecution{Version: 1, Kind: "Deployment", Defaults: &DefaultsPolicy{Version: "workload-defaults-v1", Environment: env, Template: profile}}}
		got, _ := spec.ResolveDefaults()
		want := 1
		if env == "production" {
			want = 2
		}
		if got.ReplicaCount() != want {
			t.Fatal(env, got.ReplicaCount())
		}
		spec.Replicas = pointer(0)
		got, _ = spec.ResolveDefaults()
		if got.ReplicaCount() != 0 {
			t.Fatal("template replaced explicit scale to zero")
		}
	}
	profile.MultiReplicaSafe = false
	spec := ModuleSpec{Containers: map[string]Container{"main": {Image: "image"}}, Execution: &WorkloadExecution{Version: 1, Kind: "Deployment", Defaults: &DefaultsPolicy{Version: "workload-defaults-v1", Environment: "production", Template: profile}}}
	got, _ := spec.ResolveDefaults()
	if got.ReplicaCount() != 1 {
		t.Fatal("unsafe profile scaled arbitrary workload")
	}
}
