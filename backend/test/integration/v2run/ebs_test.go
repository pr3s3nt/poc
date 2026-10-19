package v2run_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"orchestrator/test/integration/v2run"
)

type recoveredCSIAWS struct {
	t       *testing.T
	config  v2run.Config
	created time.Time
	deleted bool
	calls   []string
}

func (f *recoveredCSIAWS) Run(_ context.Context, args ...string) ([]byte, string, int) {
	command := strings.Join(args, " ")
	f.calls = append(f.calls, command)
	switch {
	case command == "sts get-caller-identity":
		raw, _ := json.Marshal(map[string]string{"Account": f.config.AWS.AccountID, "Arn": f.config.AWS.PrincipalARN})
		return raw, "", 0
	case command == "eks describe-cluster --name owned-cluster":
		return nil, "aws: [ERROR]: An error occurred (ResourceNotFoundException) when calling the DescribeCluster operation: No cluster found", 254
	case command == "ec2 describe-volumes --filters Name=tag:kubernetes.io/cluster/owned-cluster,Values=owned" || command == "ec2 describe-volumes --volume-ids vol-0123456789abcdef0":
		if f.deleted {
			return nil, "An error occurred (InvalidVolume.NotFound) when calling the DescribeVolumes operation: The volume does not exist", 254
		}
		tags := []map[string]string{}
		for k, v := range map[string]string{
			"kubernetes.io/cluster/owned-cluster": "owned", "KubernetesCluster": "owned-cluster",
			"kubernetes.io/created-for/pvc/namespace": v2run.NamespacePrefix + f.config.RunID,
			"kubernetes.io/created-for/pvc/name":      "postgres-data",
			"CSIVolumeName":                           "pvc-4fa243fe-a408-4539-b0f8-dd660d7df63b",
			"kubernetes.io/created-for/pv/name":       "pvc-4fa243fe-a408-4539-b0f8-dd660d7df63b",
		} {
			tags = append(tags, map[string]string{"Key": k, "Value": v})
		}
		raw, _ := json.Marshal(map[string]any{"Volumes": []any{map[string]any{
			"VolumeId": "vol-0123456789abcdef0", "State": "available", "CreateTime": f.created,
			"Attachments": []any{}, "Tags": tags,
		}}})
		return raw, "", 0
	case command == "ec2 delete-volume --volume-id vol-0123456789abcdef0":
		f.deleted = true
		return []byte(`{}`), "", 0
	default:
		f.t.Fatalf("unexpected AWS call: %s", command)
		return nil, "unexpected", 255
	}
}

func TestCleanupOwnedCSIOrphanAfterOriginalClusterDestroy(t *testing.T) {
	c := awsConfig(t, "V05", "t4g.medium")
	now := time.Now().UTC()
	m := v2run.NewManifest(c, now.Add(-time.Hour))
	m.Armed = true
	m.Assets = []v2run.Asset{{Kind: "eks-cluster", ID: "owned-cluster", Owner: v2run.OwnerCreatedByRun}}
	cli := &recoveredCSIAWS{t: t, config: c, created: now.Add(-time.Minute)}
	if err := v2run.CleanupOrphanEBS(context.Background(), cli, c, m); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"sts get-caller-identity", "eks describe-cluster --name owned-cluster",
		"ec2 describe-volumes --filters Name=tag:kubernetes.io/cluster/owned-cluster,Values=owned",
		"ec2 describe-volumes --volume-ids vol-0123456789abcdef0",
		"ec2 delete-volume --volume-id vol-0123456789abcdef0",
		"ec2 describe-volumes --volume-ids vol-0123456789abcdef0",
	}
	if !cli.deleted || !reflect.DeepEqual(cli.calls, want) {
		t.Fatalf("exact-ID reread/delete/native absence sequence missing: %v", cli.calls)
	}
	steps := v2run.AWSCleanupSteps("ctl", "cleanup", "manifest", "handles", "config", "vault", c)
	for _, s := range steps {
		if s.Layer == v2run.LayerEKS && (strings.Join(s.Argv, " ") != "ctl aws-ebs-clean -config config -manifest manifest" || strings.Join(s.Verify, " ") != "ctl aws-verify -config config -manifest manifest -layer eks" || len(s.Guard) == 0) {
			t.Fatalf("original guard and native layer verification must surround CSI recovery: %+v", s)
		}
	}
}
