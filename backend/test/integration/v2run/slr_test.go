package v2run_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/test/integration/v2run"
)

// slrAWS answers iam get-role / list-attached-role-policies from a table.
type slrAWS struct {
	roles   map[string]string // role name -> trust JSON, present only when listed
	policy  map[string]string // role name -> attached policy ARN
	failGet string
}

func (f *slrAWS) Run(_ context.Context, args ...string) ([]byte, string, int) {
	name := ""
	for i, a := range args {
		if a == "--role-name" {
			name = args[i+1]
		}
	}
	if f.failGet == name {
		return nil, "An error occurred (AccessDenied) when calling the GetRole operation", 254
	}
	trust, ok := f.roles[name]
	if !ok {
		return nil, "An error occurred (NoSuchEntity) when calling the GetRole operation", 254
	}
	if args[1] == "list-attached-role-policies" {
		return []byte(`{"AttachedPolicies":[{"PolicyArn":"` + f.policy[name] + `"}]}`), "", 0
	}
	return []byte(`{"Role":{"Arn":"arn:aws:iam::452025861381:role/aws-service-role/x/` + name + `","RoleId":"AROA` + name + `","Path":"/aws-service-role/x/","CreateDate":"2026-01-01T00:00:00Z","AssumeRolePolicyDocument":` + trust + `}}`), "", 0
}

func TestServiceLinkedRolesMustBePreservedExactly(t *testing.T) {
	ctx := context.Background()
	f := &slrAWS{roles: map[string]string{"AWSServiceRoleForAmazonEKS": `{"a":1}`, "AWSServiceRoleForRDS": `{"b":2}`}, policy: map[string]string{"AWSServiceRoleForAmazonEKS": "arn:p1", "AWSServiceRoleForRDS": "arn:p2"}}
	before, err := v2run.SnapshotSLRs(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if !before["AWSServiceRoleForAmazonEKS"].Present || before["AWSServiceRoleForElastiCache"].Present {
		t.Fatalf("snapshot %+v", before)
	}
	same, _ := v2run.SnapshotSLRs(ctx, f)
	if err := v2run.CompareSLRs(before, same); err != nil {
		t.Fatal(err)
	}
	// ElastiCache's role is created by the service during the run: allowed, and it is never required to disappear.
	f.roles["AWSServiceRoleForElastiCache"], f.policy["AWSServiceRoleForElastiCache"] = `{"c":3}`, "arn:p3"
	grown, _ := v2run.SnapshotSLRs(ctx, f)
	if err := v2run.CompareSLRs(before, grown); err != nil {
		t.Fatalf("a role created by the service stays and is fine: %v", err)
	}
	for name, mutate := range map[string]func(){
		"deleted":        func() { delete(f.roles, "AWSServiceRoleForRDS") },
		"trust changed":  func() { f.roles["AWSServiceRoleForAmazonEKS"] = `{"a":2}` },
		"policy changed": func() { f.policy["AWSServiceRoleForAmazonEKS"] = "arn:other" },
	} {
		g := &slrAWS{roles: map[string]string{}, policy: map[string]string{}}
		for k, v := range f.roles {
			g.roles[k] = v
		}
		for k, v := range f.policy {
			g.policy[k] = v
		}
		f2 := f
		f = g
		mutate()
		after, err := v2run.SnapshotSLRs(ctx, f)
		f = f2
		if err != nil {
			t.Fatal(err)
		}
		if err := v2run.CompareSLRs(before, after); err == nil {
			t.Fatalf("%s must break preservation", name)
		}
	}
	// An unreadable role is an error, never "absent".
	f.failGet = "AWSServiceRoleForRDS"
	if _, err := v2run.SnapshotSLRs(ctx, f); err == nil || strings.Contains(err.Error(), "NoSuchEntity") {
		t.Fatalf("an AccessDenied must fail the snapshot: %v", err)
	}
	if err := v2run.CompareSLRs(nil, before); err == nil {
		t.Fatal("no snapshot means no proof")
	}
}
