package environment

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func pointer[T any](v T) *T { return &v }
func TestOnlyPinnedTemplateDefaultsAreMergedWithoutRewritingDesired(t *testing.T) {
	plain := ModuleSpec{Containers: map[string]Container{"main": {Image: "image"}}, Execution: &WorkloadExecution{Version: 1, Kind: "Deployment"}}
	got, _ := plain.ResolveDefaults()
	if !reflect.DeepEqual(got, plain) {
		t.Fatal("backend supplied defaults without template")
	}
	profile := &RuntimeDefaults{Resources: &ContainerResourceRequirements{Requests: &ComputeResources{CPU: "317m", Memory: "123Mi"}}, Replicas: pointer(2), MultiReplicaSafe: true, Execution: &RuntimeExecutionDefaults{AutomountServiceAccountToken: pointer(true), RevisionHistoryLimit: pointer(int32(9))}, Environments: map[string]*RuntimeDefaults{"production": {Resources: &ContainerResourceRequirements{Requests: &ComputeResources{CPU: "719m"}}}}}
	for _, env := range []string{"staging", "production"} {
		t.Run(env, func(t *testing.T) {
			selected := profile.ForEnvironment(env)
			spec := plain
			spec.Replicas = pointer(0)
			spec.Execution = &WorkloadExecution{Version: 1, Kind: "Deployment", Template: TemplatePin{ID: "example", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, TemplateDefaults: &TemplateDefaultsSnapshot{Environment: env, Profile: selected}, AutomountServiceAccountToken: pointer(false), RevisionHistoryLimit: pointer(int32(0))}
			before, _ := json.Marshal(spec)
			resolved, src := spec.ResolveDefaults()
			after, _ := json.Marshal(spec)
			if string(before) != string(after) {
				t.Fatal("desired mutated")
			}
			want := "317m"
			if env == "production" {
				want = "719m"
			}
			if resolved.Containers["main"].Resources.Requests.CPU != want || resolved.Containers["main"].Resources.Limits != nil {
				t.Fatal("template values lost or platform values invented")
			}
			if *resolved.Replicas != 0 || *resolved.Execution.AutomountServiceAccountToken || *resolved.Execution.RevisionHistoryLimit != 0 || src["containers.main.resources.requests.cpu"] != "template" {
				t.Fatal("false/zero/source lost")
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

func TestTemplatePDBBudgetDoesNotConflictWithAuthoredAlternative(t *testing.T) {
	for _, tc := range []struct {
		name               string
		authored, defaults PodDisruptionBudget
	}{
		{"authored minimum", PodDisruptionBudget{Enabled: true, MinAvailable: 2}, PodDisruptionBudget{MaxUnavailable: 1}},
		{"authored maximum", PodDisruptionBudget{Enabled: true, MaxUnavailable: 1}, PodDisruptionBudget{MinAvailable: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := ModuleSpec{Replicas: pointer(3), Containers: map[string]Container{"main": {Image: "image"}}, Execution: &WorkloadExecution{Version: 1, Kind: "Deployment", Template: TemplatePin{ID: "example", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, PodDisruptionBudget: &tc.authored}}
			resolved, _ := spec.ResolveDefaults(&RuntimeDefaults{Execution: &RuntimeExecutionDefaults{PodDisruptionBudget: &tc.defaults}})
			got, _ := json.Marshal(resolved.Execution.PodDisruptionBudget)
			want, _ := json.Marshal(tc.authored)
			if string(got) != string(want) {
				t.Fatal("template added mutually exclusive budget", resolved.Execution.PodDisruptionBudget)
			}
		})
	}
}
