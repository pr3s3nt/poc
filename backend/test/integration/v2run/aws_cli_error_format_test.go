package v2run_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/test/integration/v2run"
)

func TestVerifyAWSLayerRecognizesOnlyExactNativeAWSCLIErrorFormats(t *testing.T) {
	const (
		run       = "a02-20261015-r6"
		account   = "452025861381"
		region    = "us-east-1"
		principal = "arn:aws:iam::452025861381:user/test"
		arn       = "arn:aws:ec2:us-east-1:452025861381:security-group/sg-0123456789abcdef0"
	)
	config := v2run.Config{RunID: run, Target: v2run.TargetAWS, AWS: &v2run.AWSConfig{AccountID: account, Region: region, PrincipalARN: principal}}
	manifest := v2run.Manifest{RunID: run, Target: v2run.TargetAWS, Account: account, Region: region, Principal: principal}
	if err := manifest.AddAsset(v2run.Asset{Kind: "arn", ID: arn, Layer: v2run.LayerAux, Owner: v2run.OwnerCreatedByRun}); err != nil {
		t.Fatal(err)
	}

	stub := filepath.Join(t.TempDir(), "aws")
	script := `#!/bin/sh
case "$*" in
  *"sts get-caller-identity"*)
    printf '%s\n' '{"Account":"452025861381","Arn":"arn:aws:iam::452025861381:user/test"}'
    ;;
  *"resourcegroupstaggingapi get-resources"*)
    printf '%s\n' '{"ResourceTagMappingList":[{"ResourceARN":"arn:aws:ec2:us-east-1:452025861381:security-group/sg-0123456789abcdef0","Tags":[{"Key":"run-id","Value":"a02-20261015-r6"}]}]}'
    ;;
  *"ec2 describe-security-groups --group-ids sg-0123456789abcdef0"*)
    if [ "$AWS_TEST_MODE" = "live" ]; then
      printf '%s\n' '{"SecurityGroups":[{"GroupId":"sg-0123456789abcdef0"}]}'
      exit 0
    fi
    printf '%s\n' "$AWS_TEST_STDERR" >&2
    exit 254
    ;;
  *"ec2 describe-security-groups --filters"*) printf '%s\n' '{"SecurityGroups":[]}' ;;
  *"ec2 describe-instances"*) printf '%s\n' '{"Reservations":[]}' ;;
  *"ec2 describe-volumes"*) printf '%s\n' '{"Volumes":[]}' ;;
  *"ec2 describe-addresses"*) printf '%s\n' '{"Addresses":[]}' ;;
  *) printf '%s\n' 'unexpected aws command' >&2; exit 97 ;;
esac
`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	legacy := "An error occurred (InvalidGroup.NotFound) when calling the DescribeSecurityGroups operation: The specified group does not exist"
	cli235 := "\naws: [ERROR]: An error occurred (InvalidGroup.NotFound) when calling the DescribeSecurityGroups operation: The specified group does not exist\n"
	cases := []struct {
		name   string
		stderr string
		mode   string
		wantOK bool
	}{
		{name: "legacy botocore error", stderr: legacy, wantOK: true},
		{name: "AWS CLI 2.35 prefixed error", stderr: cli235, wantOK: true},
		{name: "foreign prefix", stderr: "wrapper: " + cli235, wantOK: false},
		{name: "wrong typed code", stderr: "aws: [ERROR]: An error occurred (UnauthorizedOperation) when calling the DescribeSecurityGroups operation: denied", wantOK: false},
		{name: "wrong native operation", stderr: "aws: [ERROR]: An error occurred (InvalidGroup.NotFound) when calling the DescribeSubnets operation: absent", wantOK: false},
		{name: "successful live describe", mode: "live", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli := v2run.ExecAWS{Binary: stub, Env: []string{
				"PATH=" + os.Getenv("PATH"),
				"AWS_TEST_STDERR=" + tc.stderr,
				"AWS_TEST_MODE=" + tc.mode,
			}}
			err := v2run.VerifyAWSLayer(context.Background(), cli, config, manifest, v2run.LayerAux)
			if tc.wantOK && err != nil {
				t.Fatalf("expected exact native absence to pass, got %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Fatal("unproven or live resource was accepted as absent")
			}
		})
	}
}
