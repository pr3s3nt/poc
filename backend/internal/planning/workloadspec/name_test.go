package workloadspec

import (
	"errors"
	"strings"
	"testing"
)

// UC-16 BR-01: the workload name is a Kubernetes DNS label, rejected at Save/Import.

func withName(name string) map[string]any {
	doc := base()
	doc["metadata"] = map[string]any{"name": name}
	return doc
}

func TestWorkloadNameAcceptsDNSLabels(t *testing.T) {
	for _, name := range []string{"a", "api", "my-app", "web2", "0db", strings.Repeat("a", 63)} {
		if _, err := FromMap(withName(name)); err != nil {
			t.Fatalf("%q rejected: %v", name, err)
		}
	}
}

func TestWorkloadNameRejectsNonDNSLabels(t *testing.T) {
	for _, name := range []string{"", "My App", "MyApp", "my app", "my_app", "my.app", "-app", "app-", " app", "ứng-dụng", strings.Repeat("a", 64)} {
		_, err := FromMap(withName(name))
		if !errors.Is(err, ErrInvalidName) || !errors.Is(err, ErrInvalid) {
			t.Fatalf("%q: err = %v, want ErrInvalidName and ErrInvalid", name, err)
		}
	}
}

func TestCronJobWorkloadNameHasAtMost52Characters(t *testing.T) {
	cron := func(name string) map[string]any {
		doc := withName(name)
		doc["workloadExecution"] = map[string]any{"version": 1, "kind": "CronJob", "schedule": map[string]any{"expression": "0 0 * * *", "timeZone": "UTC", "concurrencyPolicy": "Forbid"}}
		return doc
	}
	if _, err := FromMap(cron(strings.Repeat("a", 52))); err != nil {
		t.Fatalf("52 characters rejected: %v", err)
	}
	if _, err := FromMap(cron(strings.Repeat("a", 53))); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("53 characters: err = %v, want ErrInvalidName", err)
	}
	if _, err := FromMap(withName(strings.Repeat("a", 53))); err != nil {
		t.Fatalf("53-character Deployment name rejected: %v", err)
	}
}
