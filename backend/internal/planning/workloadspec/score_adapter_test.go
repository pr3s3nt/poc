package workloadspec

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func scoreExample() map[string]any {
	return map[string]any{"apiVersion": ScoreAPIVersion, "metadata": map[string]any{"name": "order-api"}, "containers": map[string]any{"main": map[string]any{"image": "example/api:1", "variables": map[string]any{"PGHOST": "${resources.db.host}", "LITERAL": "$${keep}"}}}, "resources": map[string]any{"db": map[string]any{"type": "postgres", "id": "orders-db", "params": map[string]any{"database": "orders", "username": "app"}}}, "service": map[string]any{"ports": map[string]any{"http": map[string]any{"port": 80, "targetPort": 8080}}}}
}
func TestScoreAndNativeProduceIdenticalDependencyFragments(t *testing.T) {
	raw := scoreExample()
	a, err := FromMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	raw["apiVersion"] = SupportedAPIVersion
	b, err := FromMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Same module, shared identities, params, bindings and compute intent feed the
	// existing planner, rather than testing only a superficial API-version change.
	fa, err := a.Fragment(types())
	if err != nil {
		t.Fatal(err)
	}
	fb, err := b.Fragment(types())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fa, fb) {
		t.Fatalf("Score changed planner input: %#v != %#v", fa, fb)
	}
}
func TestScoreSchemaAndUnsupportedFeaturesFail(t *testing.T) {
	for _, mutate := range []func(map[string]any){
		func(m map[string]any) { m["replicas"] = 2 },
		func(m map[string]any) { m["containers"].(map[string]any)["main"].(map[string]any)["image"] = 42 },
		func(m map[string]any) {
			m["containers"].(map[string]any)["main"].(map[string]any)["files"] = map[string]any{"/etc/test": map[string]any{"source": "/etc/passwd"}}
		},
		func(m map[string]any) { m["metadata"].(map[string]any)["custom"] = "must-not-disappear" },
	} {
		raw := scoreExample()
		mutate(raw)
		if _, err := DecodeScore(raw, nil); err == nil {
			t.Fatal("unsupported Score accepted")
		}
	}
}
func TestScoreCronExtensionAndStrictness(t *testing.T) {
	extension := map[string]any{"apiVersion": ExtensionAPIVersion, "profile": "humanitec/default-cronjob", "spec": map[string]any{"schedules": map[string]any{"daily": map[string]any{"schedule": "0 2 * * *"}}, "cronjob": map[string]any{"suspend": false, "timeZone": "Asia/Bangkok", "concurrencyPolicy": "Forbid"}}}
	cron := scoreExample()
	delete(cron, "service")
	converted, err := DecodeScore(cron, extension)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := FromMap(converted.Workload)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Execution.Kind != "CronJob" || doc.Execution.Schedule.Expression != "0 2 * * *" || doc.Execution.Schedule.TimeZone != "Asia/Bangkok" {
		t.Fatalf("lost schedule: %#v", doc.Execution)
	}
	spec := extension["spec"].(map[string]any)
	spec["schedules"].(map[string]any)["second"] = map[string]any{"schedule": "* * * * *"}
	if _, err = DecodeScore(scoreExample(), extension); err == nil {
		t.Fatal("multiple schedules silently collapsed")
	}
	delete(spec, "schedules")
	spec["pod"] = map[string]any{"nodeSelector": map[string]any{"pool": "other"}}
	if _, err = DecodeScore(scoreExample(), extension); err == nil {
		t.Fatal("unsupported pod fields accepted")
	}
}
func TestScoreExportPreservesAdvancedIntentAndExplicitZeroFalse(t *testing.T) {
	raw := scoreExample()
	raw["apiVersion"] = SupportedAPIVersion
	raw["replicas"] = 0
	raw["metadata"].(map[string]any)["class"] = "custom"
	c := raw["containers"].(map[string]any)["main"].(map[string]any)
	c["workingDir"] = "/app"
	c["securityContext"] = map[string]any{"readOnlyRootFilesystem": false}
	c["variableOrder"] = []any{"PGHOST", "LITERAL"}
	c["readinessProbe"] = map[string]any{"type": "tcp", "port": 8080, "periodSeconds": 12}
	raw["workloadExecution"] = map[string]any{"version": 1, "kind": "Deployment", "automountServiceAccountToken": false}
	doc, err := FromMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	score, extension, err := ExportScore(doc)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(score)
	if strings.Contains(string(b), "securityContext") || strings.Contains(string(b), "replicas") {
		t.Fatal("native fields leaked into Score")
	}
	converted, err := DecodeScore(score, extension)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := FromMap(converted.Workload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonicalJSON(doc.Map()), canonicalJSON(roundtrip.Map())) {
		t.Fatal("advanced intent lost")
	}
}
func TestScoreExtensionCannotOverwritePortableOrPlatformFields(t *testing.T) {
	for _, native := range []map[string]any{
		{"resources": map[string]any{"db": map[string]any{"type": "redis"}}},
		{"containers": map[string]any{"main": map[string]any{"image": "attacker"}}},
		{"workloadExecution": map[string]any{"templateDefaults": map[string]any{}}},
	} {
		e := map[string]any{"apiVersion": ExtensionAPIVersion, "spec": map[string]any{"orchestrator": native}}
		if _, err := DecodeScore(scoreExample(), e); err == nil {
			t.Fatal("extension replaced authoritative fields")
		}
	}
	e := map[string]any{"apiVersion": ExtensionAPIVersion, "spec": map[string]any{"orchestrator": map[string]any{"replicas": 0}, "deployment": map[string]any{"replicas": 2}}}
	if _, err := DecodeScore(scoreExample(), e); err == nil {
		t.Fatal("overlapping extension values accepted")
	}
}

func TestScoreCronSuspendRejectsConflictingNativeIntent(t *testing.T) {
	raw := scoreExample()
	delete(raw, "service")
	ext := map[string]any{"apiVersion": ExtensionAPIVersion, "profile": "humanitec/default-cronjob", "spec": map[string]any{
		"orchestrator": map[string]any{"workloadExecution": map[string]any{"version": 1, "kind": "CronJob", "schedule": map[string]any{"expression": "* * * * *", "suspend": false, "timeZone": "UTC", "concurrencyPolicy": "Forbid"}}},
		"cronjob":      map[string]any{"suspend": true},
	}}
	if _, err := DecodeScore(raw, ext); err == nil {
		t.Fatal("Humanitec field overwrote explicit native suspend=false")
	}
	delete(ext["spec"].(map[string]any), "orchestrator")
	ext["spec"].(map[string]any)["schedules"] = map[string]any{"daily": map[string]any{"schedule": "* * * * *"}}
	converted, err := DecodeScore(raw, ext)
	if err != nil {
		t.Fatal(err)
	}
	schedule := converted.Workload["workloadExecution"].(map[string]any)["schedule"].(map[string]any)
	if schedule["suspend"] != true {
		t.Fatal("Explicit Humanitec suspend did not override normalization default")
	}
}
