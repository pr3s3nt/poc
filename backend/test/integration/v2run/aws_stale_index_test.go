package v2run_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/test/integration/v2run"
)

type staleIndexAWS struct {
	arn       string
	tagRun    string
	resources []taggedRecord
	dup       bool
	queryErr  bool
	probeOut  bool
	probeErr  string
	probeExit int
	probeByID map[string]nativeResult
	raw       []byte
	omitTag   bool
	calls     [][]string
}

type taggedRecord struct{ arn, tagRun string }

type nativeResult struct {
	stdout string
	stderr string
	exit   int
}

func (f *staleIndexAWS) Run(_ context.Context, args ...string) ([]byte, string, int) {
	f.calls = append(f.calls, append([]string(nil), args...))
	switch {
	case len(args) >= 1 && args[0] == "sts":
		return []byte(`{"Account":"452025861381","Arn":"arn:aws:iam::452025861381:user/test"}`), "", 0
	case len(args) >= 2 && args[0] == "resourcegroupstaggingapi":
		if f.queryErr {
			return nil, "AccessDenied", 254
		}
		if f.raw != nil {
			return f.raw, "", 0
		}
		records := f.resources
		if records == nil {
			records = []taggedRecord{{arn: f.arn, tagRun: f.tagRun}}
		}
		items := make([]any, 0, len(records)+1)
		for _, record := range records {
			item := map[string]any{"ResourceARN": record.arn}
			if !f.omitTag {
				item["Tags"] = []map[string]string{{"Key": "run-id", "Value": record.tagRun}}
			}
			items = append(items, item)
			if f.dup {
				items = append(items, item)
			}
		}
		encoded, _ := json.Marshal(map[string]any{"ResourceTagMappingList": items})
		return encoded, "", 0
	case len(args) >= 2 && args[0] == "ec2":
		if len(args) == 4 && f.probeByID != nil && (args[1] == "describe-subnets" || args[1] == "describe-security-groups") {
			if result, ok := f.probeByID[args[3]]; ok {
				return []byte(result.stdout), result.stderr, result.exit
			}
			return nil, "unexpected native probe", 255
		}
		if f.probeOut {
			return []byte(`{"Subnets":[{"SubnetId":"subnet-0123456789abcdef0"}],"SecurityGroups":[{"GroupId":"sg-0123456789abcdef0"}]}`), "", 0
		}
		if f.probeErr == "" {
			if len(args) > 1 && args[1] == "describe-security-groups" {
				f.probeErr = "An error occurred (InvalidGroup.NotFound) when calling the DescribeSecurityGroups operation: group not found"
			} else {
				f.probeErr = "An error occurred (InvalidSubnetID.NotFound) when calling the DescribeSubnets operation: subnet not found"
			}
		}
		return nil, f.probeErr, f.probeExitValue()
	default:
		return nil, "unexpected AWS command", 255
	}
}

func (f *staleIndexAWS) probeExitValue() int {
	if f.probeExit == 0 {
		return 254
	}
	return f.probeExit
}

func TestVerifyAWSLayerAcceptsOnlyManifestOwnedTypedStaleEC2IndexEntries(t *testing.T) {
	const (
		run       = "a02-20261015-r6"
		account   = "452025861381"
		region    = "us-east-1"
		principal = "arn:aws:iam::452025861381:user/test"
	)
	config := v2run.Config{RunID: run, Target: v2run.TargetAWS, AWS: &v2run.AWSConfig{AccountID: account, Region: region, PrincipalARN: principal}}
	manifest := v2run.Manifest{RunID: run, Target: v2run.TargetAWS, Account: account, Region: region, Principal: principal}
	arns := []struct{ kind, arn string }{
		{"subnet", "arn:aws:ec2:us-east-1:452025861381:subnet/subnet-0123456789abcdef0"},
		{"security-group", "arn:aws:ec2:us-east-1:452025861381:security-group/sg-0123456789abcdef0"},
	}
	for _, entry := range arns {
		if err := manifest.AddAsset(v2run.Asset{Kind: "arn", ID: entry.arn, Layer: "aux", Owner: v2run.OwnerCreatedByRun}); err != nil {
			t.Fatal(err)
		}
	}

	for _, entry := range arns {
		t.Run(entry.kind+" typed not-found", func(t *testing.T) {
			aws := &staleIndexAWS{arn: entry.arn, tagRun: run}
			if err := v2run.VerifyAWSLayer(context.Background(), aws, config, manifest, "aux"); err != nil {
				t.Fatalf("exact native not-found should resolve stale index entry: %v", err)
			}
			want := "--subnet-ids"
			if entry.kind == "security-group" {
				want = "--group-ids"
			}
			found := false
			for _, call := range aws.calls {
				if len(call) == 4 && call[0] == "ec2" && call[2] == want && call[3] == strings.TrimPrefix(entry.arn, "arn:aws:ec2:us-east-1:452025861381:"+entry.kind+"/") {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected exact-id native describe call, got %#v", aws.calls)
			}
		})
	}
	t.Run("two stale entries each require exact native absence", func(t *testing.T) {
		aws := &staleIndexAWS{resources: []taggedRecord{{arns[0].arn, run}, {arns[1].arn, run}}, probeByID: map[string]nativeResult{
			"subnet-0123456789abcdef0": {stderr: "An error occurred (InvalidSubnetID.NotFound) when calling the DescribeSubnets operation: gone", exit: 254},
			"sg-0123456789abcdef0":     {stderr: "An error occurred (InvalidGroup.NotFound) when calling the DescribeSecurityGroups operation: gone", exit: 254},
		}}
		if err := v2run.VerifyAWSLayer(context.Background(), aws, config, manifest, "aux"); err != nil {
			t.Fatalf("two exact native absences should pass: %v", err)
		}
		if countNativeProbes(aws.calls) != 2 {
			t.Fatalf("both resources require native probes, got %#v", aws.calls)
		}
		assertAWSVerificationWasReadOnly(t, aws.calls)
	})
	t.Run("one live entry keeps mixed result failing", func(t *testing.T) {
		aws := &staleIndexAWS{resources: []taggedRecord{{arns[0].arn, run}, {arns[1].arn, run}}, probeByID: map[string]nativeResult{
			"subnet-0123456789abcdef0": {stderr: "An error occurred (InvalidSubnetID.NotFound) when calling the DescribeSubnets operation: gone", exit: 254},
			"sg-0123456789abcdef0":     {stdout: `{"SecurityGroups":[{"GroupId":"sg-0123456789abcdef0"}]}`, exit: 0},
		}}
		if err := v2run.VerifyAWSLayer(context.Background(), aws, config, manifest, "aux"); err == nil {
			t.Fatal("one live resource in a mixed RGTA response must fail")
		}
		if countNativeProbes(aws.calls) != 2 {
			t.Fatalf("both resources require native probes, got %#v", aws.calls)
		}
		assertAWSVerificationWasReadOnly(t, aws.calls)
	})
	t.Run("empty result list proves the RGTA sweep is clear", func(t *testing.T) {
		aws := &staleIndexAWS{resources: []taggedRecord{}}
		if err := v2run.VerifyAWSLayer(context.Background(), aws, config, manifest, "aux"); err != nil {
			t.Fatalf("valid empty result should pass: %v", err)
		}
		if countNativeProbes(aws.calls) != 0 {
			t.Fatalf("empty result should not probe native resources: %#v", aws.calls)
		}
	})
}

func TestVerifyAWSLayerFailsClosedOnUnprovenTaggedResources(t *testing.T) {
	const run = "a02-20261015-r6"
	const account = "452025861381"
	const region = "us-east-1"
	const principal = "arn:aws:iam::452025861381:user/test"
	arn := "arn:aws:ec2:us-east-1:452025861381:subnet/subnet-0123456789abcdef0"
	config := v2run.Config{RunID: run, Target: v2run.TargetAWS, AWS: &v2run.AWSConfig{AccountID: account, Region: region, PrincipalARN: principal}}
	base := v2run.Manifest{RunID: run, Target: v2run.TargetAWS, Account: account, Region: region, Principal: principal}
	if err := base.AddAsset(v2run.Asset{Kind: "arn", ID: arn, Layer: "aux", Owner: v2run.OwnerCreatedByRun}); err != nil {
		t.Fatal(err)
	}
	duplicateAsset := base
	duplicateAsset.Assets = append(append([]v2run.Asset(nil), base.Assets...), base.Assets[0])
	cases := []struct {
		name      string
		manifest  v2run.Manifest
		arn       string
		runTag    string
		duplicate bool
		queryErr  bool
		probeOut  bool
		probeErr  string
		probeExit int
		raw       []byte
		omitTag   bool
		wantCall  bool
	}{
		{name: "live subnet", manifest: base, arn: arn, runTag: run, probeOut: true, wantCall: true},
		{name: "unrecorded resource as in early failed run", manifest: v2run.Manifest{RunID: run, Target: v2run.TargetAWS, Account: account, Region: region, Principal: principal}, arn: arn, runTag: run},
		{name: "preexisting manifest asset", manifest: manifestWithOwner(t, base, v2run.OwnerPreexisting), arn: arn, runTag: run},
		{name: "retained manifest asset", manifest: manifestWithOwner(t, base, v2run.OwnerRetained), arn: arn, runTag: run},
		{name: "wrong kind same id", manifest: manifestWithShape(t, base, arn, "subnet", "aux", v2run.OwnerCreatedByRun), arn: arn, runTag: run},
		{name: "wrong layer same id", manifest: manifestWithShape(t, base, arn, "arn", "vpc", v2run.OwnerCreatedByRun), arn: arn, runTag: run},
		{name: "wrong account", manifest: base, arn: "arn:aws:ec2:us-east-1:111122223333:subnet/subnet-0123456789abcdef0", runTag: run},
		{name: "wrong region", manifest: base, arn: "arn:aws:ec2:us-west-2:452025861381:subnet/subnet-0123456789abcdef0", runTag: run},
		{name: "malformed resource id", manifest: manifestWithShape(t, base, "arn:aws:ec2:us-east-1:452025861381:subnet/not-a-subnet-id", "arn", "aux", v2run.OwnerCreatedByRun), arn: "arn:aws:ec2:us-east-1:452025861381:subnet/not-a-subnet-id", runTag: run},
		{name: "wrong run tag", manifest: base, arn: arn, runTag: "foreign-run"},
		{name: "missing run tag", manifest: base, arn: arn, omitTag: true},
		{name: "duplicate RGTA ARN", manifest: base, arn: arn, runTag: run, duplicate: true},
		{name: "duplicate manifest ARN asset", manifest: duplicateAsset, arn: arn, runTag: run},
		{name: "malformed RGTA document", manifest: base, raw: []byte(`{"ResourceTagMappingList":"not-an-array"`)},
		{name: "omitted RGTA list", manifest: base, raw: []byte(`{}`)},
		{name: "null RGTA list", manifest: base, raw: []byte(`{"ResourceTagMappingList":null}`)},
		{name: "RGTA access denied", manifest: base, arn: arn, runTag: run, queryErr: true},
		{name: "wrong not-found code", manifest: base, arn: arn, runTag: run, probeErr: "An error occurred (InvalidGroup.NotFound) when calling the DescribeSubnets operation: not found", wantCall: true},
		{name: "wrong native operation", manifest: base, arn: arn, runTag: run, probeErr: "An error occurred (InvalidSubnetID.NotFound) when calling the DescribeSecurityGroups operation: not found", wantCall: true},
		{name: "permission failure", manifest: base, arn: arn, runTag: run, probeErr: "An error occurred (UnauthorizedOperation) when calling the DescribeSubnets operation: denied", wantCall: true},
		{name: "timeout", manifest: base, arn: arn, runTag: run, probeErr: "request timed out", probeExit: 124, wantCall: true},
		{name: "throttled", manifest: base, arn: arn, runTag: run, probeErr: "An error occurred (Throttling) when calling the DescribeSubnets operation: slow down", wantCall: true},
		{name: "malformed native error", manifest: base, arn: arn, runTag: run, probeErr: "InvalidSubnetID.NotFound", wantCall: true},
		{name: "successful empty native response", manifest: base, arn: arn, runTag: run, probeOut: true, wantCall: true},
		{name: "unknown resource kind", manifest: base, arn: "arn:aws:ec2:us-east-1:452025861381:network-interface/eni-0123456789abcdef0", runTag: run},
		{name: "manifest account mismatch", manifest: manifestWithIdentity(base, "111122223333", region), arn: arn, runTag: run},
		{name: "manifest region mismatch", manifest: manifestWithIdentity(base, account, "us-west-2"), arn: arn, runTag: run},
		{name: "manifest principal mismatch", manifest: manifestWithPrincipal(base, "arn:aws:iam::452025861381:user/other"), arn: arn, runTag: run},
		{name: "manifest run mismatch", manifest: manifestWithRun(base, "a02-20261015-other"), arn: arn, runTag: run},
		{name: "manifest target mismatch", manifest: manifestWithTarget(base, v2run.TargetK8s4f), arn: arn, runTag: run},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aws := &staleIndexAWS{arn: tc.arn, tagRun: tc.runTag, dup: tc.duplicate, queryErr: tc.queryErr, probeOut: tc.probeOut, probeErr: tc.probeErr, probeExit: tc.probeExit, raw: tc.raw, omitTag: tc.omitTag}
			err := v2run.VerifyAWSLayer(context.Background(), aws, config, tc.manifest, "aux")
			if err == nil {
				t.Fatal("unproven tagged resource was accepted as absent")
			}
			probeCalls := countNativeProbes(aws.calls)
			if tc.wantCall != (probeCalls == 1) {
				t.Fatalf("native exact-ID probe count = %d, wantCall=%v", probeCalls, tc.wantCall)
			}
			assertAWSVerificationWasReadOnly(t, aws.calls)
		})
	}
}

func manifestWithOwner(t *testing.T, base v2run.Manifest, owner string) v2run.Manifest {
	t.Helper()
	base.Assets = nil
	for _, id := range []string{"arn:aws:ec2:us-east-1:452025861381:subnet/subnet-0123456789abcdef0"} {
		if err := base.AddAsset(v2run.Asset{Kind: "arn", ID: id, Layer: "aux", Owner: owner}); err != nil {
			t.Fatal(err)
		}
	}
	return base
}

func manifestWithIdentity(base v2run.Manifest, account, region string) v2run.Manifest {
	base.Account, base.Region = account, region
	return base
}

func manifestWithPrincipal(base v2run.Manifest, principal string) v2run.Manifest {
	base.Principal = principal
	return base
}

func manifestWithRun(base v2run.Manifest, runID string) v2run.Manifest {
	base.RunID = runID
	return base
}

func manifestWithTarget(base v2run.Manifest, target string) v2run.Manifest {
	base.Target = target
	return base
}

func manifestWithShape(t *testing.T, base v2run.Manifest, id, kind, layer, owner string) v2run.Manifest {
	t.Helper()
	base.Assets = nil
	if err := base.AddAsset(v2run.Asset{Kind: kind, ID: id, Layer: layer, Owner: owner}); err != nil {
		t.Fatal(err)
	}
	return base
}

func countNativeProbes(calls [][]string) int {
	count := 0
	for _, call := range calls {
		if len(call) > 1 && call[0] == "ec2" && (call[1] == "describe-subnets" || call[1] == "describe-security-groups") {
			count++
		}
	}
	return count
}

func assertAWSVerificationWasReadOnly(t *testing.T, calls [][]string) {
	t.Helper()
	for _, call := range calls {
		if len(call) == 0 {
			t.Fatal("empty AWS CLI call")
		}
		if call[0] == "sts" || call[0] == "resourcegroupstaggingapi" {
			continue
		}
		if call[0] == "ec2" && len(call) > 1 && strings.HasPrefix(call[1], "describe-") {
			continue
		}
		t.Fatalf("verification issued a non-read-only AWS command: %#v", call)
	}
}
