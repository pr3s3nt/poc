package drift

import (
	"encoding/json"
	"strings"
	"testing"
)

const plan = `{"format_version":"1.2","complete":true,"applyable":true,"resource_drift":[{"address":"kubernetes_config_map_v1.config","mode":"managed","type":"kubernetes_config_map_v1","change":{"actions":["update"],"before":{"data":{"config":"approved","password":"private-before"}},"after":{"data":{"config":"manual","password":"private-after"}}}}],"resource_changes":[{"address":"kubernetes_config_map_v1.config","mode":"managed","type":"kubernetes_config_map_v1","change":{"actions":["update"],"before":{"data":{"config":"manual","password":"private-after"}},"after":{"data":{"config":"approved","password":"private-before"}},"after_unknown":{},"after_sensitive":{"data":{"password":true}}}}],"output_changes":{}}`

func TestExternalUpdateAndRestoreActionsStayDistinctAndValuesStayPrivate(t *testing.T) {
	r, err := Parse([]byte(plan))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete || len(r.External) != 1 || len(r.Proposed) != 1 || !r.InPlaceOnly() {
		t.Fatal("in-place external drift lost")
	}
	if strings.Join(r.External[0].Paths, ",") != "/data/config,/data/password" {
		t.Fatal("all changed paths not captured")
	}
	public, _ := json.Marshal(r)
	if strings.Contains(string(public), "private-") || strings.Contains(string(public), r.PrivateChangeDigest) || strings.Contains(string(public), "manual") {
		t.Fatal("private values or digest serialized")
	}
	other, err := Parse([]byte(strings.ReplaceAll(plan, "private-after", "different-private")))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(r)
	b, _ := json.Marshal(other)
	if string(a) != string(b) || r.PrivateChangeDigest == other.PrivateChangeDigest {
		t.Fatal("private review failed to distinguish equal public summaries")
	}
}

func TestSensitiveObjectKeysNeverAppearInPublicChangedPaths(t *testing.T) {
	raw := []byte(`{"format_version":"1.2","complete":true,"applyable":true,"resource_changes":[{"address":"example.app","mode":"managed","type":"example","change":{"actions":["update"],"before":{"data":{"private-key-value":"old"}},"after":{"data":{"private-key-value":"new"}},"before_sensitive":{"data":true},"after_sensitive":{"data":true}}}]}`)
	report, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(report)
	if strings.Contains(string(public), "private-key-value") || strings.Join(report.Proposed[0].Paths, ",") != "/data" {
		t.Fatal("sensitive subtree key escaped as a field path")
	}
}
func TestDeleteIgnoredExternalEditOutputsAndUnsafeRepair(t *testing.T) {
	ignored := strings.Replace(plan, `"resource_changes":[{`, `"resource_changes":[{`, 1)
	var document map[string]any
	_ = json.Unmarshal([]byte(ignored), &document)
	document["resource_changes"] = []any{}
	raw, _ := json.Marshal(document)
	r, err := Parse(raw)
	if err != nil || len(r.External) != 1 || len(r.Proposed) != 0 || r.InPlaceOnly() {
		t.Fatal("ignore_changes drift conflated with restoration", err)
	}
	document["resource_drift"].([]any)[0].(map[string]any)["change"].(map[string]any)["actions"] = []any{"delete"}
	raw, _ = json.Marshal(document)
	r, err = Parse(raw)
	if err != nil || r.External[0].Actions[0] != "delete" {
		t.Fatal("disappeared object hidden", err)
	}
	for _, actions := range []string{`["create"]`, `["delete"]`, `["delete","create"]`, `["create","delete"]`, `["forget"]`} {
		r, err := Parse([]byte(strings.ReplaceAll(plan, `["update"]`, actions)))
		if err != nil || r.InPlaceOnly() {
			t.Fatal("unsafe action admitted", actions, err)
		}
	}
	for _, output := range []string{`{"endpoint":{"actions":["update"],"before":"old","after":"new"}}`, `{"endpoint":{"actions":["no-op"],"after_unknown":true}}`} {
		r, err := Parse([]byte(strings.Replace(plan, `"output_changes":{}`, `"output_changes":`+output, 1)))
		if err != nil || r.InPlaceOnly() || len(r.OutputNames) != 1 {
			t.Fatal("consumer output change admitted", err)
		}
	}
}
func TestIncompleteUnsupportedAndUnapplyablePlansFailClosed(t *testing.T) {
	for _, raw := range []string{`{}`, strings.Replace(plan, `"1.2"`, `"2.0"`, 1), strings.Replace(plan, `"complete":true`, `"complete":false`, 1), strings.Replace(plan, `"complete":true`, `"errored":true`, 1), strings.Replace(plan, `"complete":true`, `"deferred_changes":[{}]`, 1), strings.ReplaceAll(plan, `["update"]`, `["new-action"]`), plan + ` {}`} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Fatal("incomplete or unsupported plan accepted")
		}
	}
	r, err := Parse([]byte(strings.Replace(plan, `"applyable":true`, `"applyable":false`, 1)))
	if err != nil || r.InPlaceOnly() {
		t.Fatal("unapplyable plan admitted", err)
	}
}
func TestPrivateDigestIgnoresOnlyTopLevelPresentationMetadata(t *testing.T) {
	a, err := Parse([]byte(plan))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse([]byte(strings.Replace(plan, `"format_version":"1.2"`, `"format_version":"1.2","timestamp":"later","terraform_version":"same-pin","variables":{"secret":{"value":"not a plan action"}}`, 1)))
	if err != nil || a.PrivateChangeDigest != b.PrivateChangeDigest {
		t.Fatal("nondeterministic presentation changed digest", err)
	}
	for _, marker := range []string{`"importing":{"id":"private-id"},`} {
		candidate := strings.Replace(plan, `"after_unknown":{},`, marker, 1)
		r, err := Parse([]byte(candidate))
		if err != nil || r.InPlaceOnly() {
			t.Fatal("unknown or import admitted", err)
		}
	}
}

func TestReviewedRepairAllowsReconciliationWithoutGrantingOwnershipChanges(t *testing.T) {
	for _, actions := range []string{`["create"]`, `["delete"]`, `["update"]`, `["delete","create"]`, `["create","delete"]`} {
		raw := strings.ReplaceAll(plan, `["update"]`, actions)
		raw = strings.Replace(raw, `"output_changes":{}`, `"output_changes":{"host":{"actions":["update"],"after_unknown":true}}`, 1)
		report, err := Parse([]byte(raw))
		if err != nil || !report.Repairable() {
			t.Fatalf("reviewed action rejected: %s %v", actions, err)
		}
		if !report.ChangesOutputs() {
			t.Fatal("consumer impact hidden")
		}
	}
	for _, actions := range []string{`["forget"]`, `["create","update"]`, `["delete","delete"]`} {
		report, err := Parse([]byte(strings.ReplaceAll(plan, `["update"]`, actions)))
		if err == nil && report.Repairable() {
			t.Fatalf("ownership/invalid action admitted: %s", actions)
		}
	}
	for _, marker := range []string{`"importing":{"id":"private-id"},`, `"importing":{"id":"private-id"},`} {
		report, err := Parse([]byte(strings.Replace(plan, `"after_unknown":{},`, marker, 1)))
		if err == nil && report.Repairable() {
			t.Fatal("import admitted")
		}
	}
	moved := strings.Replace(plan, `"address":"kubernetes_config_map_v1.config"`, `"previous_address":"kubernetes_config_map_v1.old","address":"kubernetes_config_map_v1.config"`, 1)
	report, err := Parse([]byte(moved))
	if err == nil && report.Repairable() {
		t.Fatal("address move admitted")
	}
}

func TestComparisonUsesCorrectiveDesiredAndHidesSensitiveOrArbitraryValues(t *testing.T) {
	raw := `{"format_version":"1.2","resource_drift":[{"address":"aws_instance.app","mode":"managed","type":"aws_instance","change":{"actions":["update"],"before":{"instance_type":"t3.small","tags":{"owner":"old-secret"}},"after":{"instance_type":"t3.large","tags":{"owner":"new-secret"}}}}],"resource_changes":[{"address":"aws_instance.app","mode":"managed","type":"aws_instance","change":{"actions":["update"],"before":{"instance_type":"t3.large"},"after":{"instance_type":"t3.medium"}}}]}`
	r, err := Parse([]byte(raw))
	if err != nil || len(r.Comparisons) != 1 || r.Comparisons[0].Desired != "t3.medium" || r.Comparisons[0].Actual != "t3.large" {
		t.Fatal("safe desired/actual comparison incorrect", err)
	}
	public, _ := json.Marshal(r)
	if strings.Contains(string(public), "old-secret") || strings.Contains(string(public), "new-secret") {
		t.Fatal("arbitrary tag values escaped")
	}
	secret := strings.Replace(raw, `"after":{"instance_type":"t3.medium"}`, `"after":{"instance_type":"t3.medium"},"after_sensitive":{"instance_type":true}`, 1)
	r, err = Parse([]byte(secret))
	if err != nil || len(r.Comparisons) != 0 {
		t.Fatal("provider sensitivity ignored", err)
	}
}
