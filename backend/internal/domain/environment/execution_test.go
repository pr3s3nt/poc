package environment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExecutionStorageBoundaries(t *testing.T) {
	valid := func() *WorkloadExecution {
		return &WorkloadExecution{Version: 1, Kind: "StatefulSet", Template: TemplatePin{ID: "stateful", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Mounts: []StorageMount{{Resource: "data", Container: "app", Path: "/app/data"}}}
	}
	for _, tc := range []struct {
		name     string
		change   func(*WorkloadExecution)
		replicas int
		wantErr  bool
	}{
		{"valid", func(*WorkloadExecution) {}, 1, false},
		{"shared claim replicas", func(*WorkloadExecution) {}, 2, true},
		{"unknown container", func(e *WorkloadExecution) { e.Mounts[0].Container = "other" }, 1, true},
		{"unknown dependency", func(e *WorkloadExecution) { e.Mounts[0].Resource = "other" }, 1, true},
		{"secret overlap", func(e *WorkloadExecution) { e.Mounts[0].Path = "/var/run" }, 1, true},
		{"overlapping mounts", func(e *WorkloadExecution) {
			e.Mounts = append(e.Mounts, StorageMount{Resource: "data", Container: "app", Path: "/app/data/nested"})
		}, 1, true},
		{"relative", func(e *WorkloadExecution) { e.Mounts[0].Path = "app/data" }, 1, true},
		{"unclean", func(e *WorkloadExecution) { e.Mounts[0].Path = "/app/../data" }, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := valid()
			tc.change(e)
			err := e.Validate(map[string]Container{"app": {}}, tc.replicas, false, map[string]bool{"data": true})
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestSchedulePolicyRoundTrip(t *testing.T) {
	zero := int32(0)
	yes := Schedule{Expression: "0 0 * * *", TimeZone: "Asia/Ho_Chi_Minh", ConcurrencyPolicy: "Forbid", BackoffLimit: &zero, SuccessfulJobsHistoryLimit: &zero}
	raw, err := json.Marshal(yes)
	if err != nil {
		t.Fatal(err)
	}
	var got Schedule
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.BackoffLimit == nil || *got.BackoffLimit != 0 || got.SuccessfulJobsHistoryLimit == nil || got.Suspend {
		t.Fatal("explicit zero/false lost")
	}
	if err = got.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"Local", "", "unknown/zone"} {
		got.TimeZone = zone
		if got.Validate() == nil {
			t.Fatalf("accepted timezone %q", zone)
		}
	}
}

func TestCronGrammarAndClone(t *testing.T) {
	for _, expression := range []string{"99 0 * * *", "*/0 * * * *", "*/+1 * * * *", "* * * *", "* * * * * *", "0 0 * * 7", "0 0 31-1 * *", "@daily", "TZ=UTC 0 0 * * *", "0 0 * * MON"} {
		if validCron(expression) {
			t.Fatalf("accepted invalid/unsupported cron %q", expression)
		}
	}
	for _, expression := range []string{"0 0 * * *", "*/15 8-18 * * 1-5", "1,20,59 0 1 */2 *"} {
		if !validCron(expression) {
			t.Fatalf("refused %q", expression)
		}
	}
	limit := int32(0)
	source := &WorkloadExecution{Schedule: &Schedule{BackoffLimit: &limit}, Mounts: []StorageMount{{Path: "/data"}}}
	copy := source.Clone()
	copy.Mounts[0].Path = "/other"
	*copy.Schedule.BackoffLimit = 3
	if source.Mounts[0].Path != "/data" || *source.Schedule.BackoffLimit != 0 {
		t.Fatal("clone aliases mutable source")
	}
}

func TestSecretFilesRejectUnsafeProjection(t *testing.T) {
	base := func() *WorkloadExecution {
		return &WorkloadExecution{Version: 1, Kind: "Deployment", Template: TemplatePin{ID: "backend", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, SecretFiles: []SecretFile{{Container: "main", Ref: "${resources.env.KEY}", Path: "/keys/private", Mode: 0400}}}
	}
	for _, tc := range []struct {
		name   string
		change func(*WorkloadExecution)
	}{
		{"writable", func(e *WorkloadExecution) { e.SecretFiles[0].Mode = 0600 }},
		{"relative", func(e *WorkloadExecution) { e.SecretFiles[0].Path = "keys/private" }},
		{"protected", func(e *WorkloadExecution) { e.SecretFiles[0].Path = "/var/run/secrets/token" }},
		{"protected root", func(e *WorkloadExecution) { e.SecretFiles[0].Path = "/vault" }},
		{"protected parent", func(e *WorkloadExecution) { e.SecretFiles[0].Path = "/var/run" }},
		{"duplicate", func(e *WorkloadExecution) { e.SecretFiles = append(e.SecretFiles, e.SecretFiles[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := base()
			tc.change(e)
			if e.Validate(map[string]Container{"main": {}}, 1, false, nil) == nil {
				t.Fatal("unsafe projection accepted")
			}
		})
	}
}
