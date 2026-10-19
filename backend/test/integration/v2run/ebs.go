package v2run

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

type orphanEBSVolume struct {
	ID          string `json:"VolumeId"`
	State       string
	CreateTime  time.Time
	Attachments []json.RawMessage
	Tags        []struct{ Key, Value string }
}

var orphanVolumeID = regexp.MustCompile(`^vol-[0-9a-f]{8,17}$`)
var orphanPVID = regexp.MustCompile(`^pvc-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var orphanClusterGone = regexp.MustCompile(`(?m)^(?:aws: \[ERROR\]: )?An error occurred \(ResourceNotFoundException\) when calling the DescribeCluster operation:`)
var orphanVolumeGone = regexp.MustCompile(`(?m)^(?:aws: \[ERROR\]: )?An error occurred \(InvalidVolume.NotFound\) when calling the DescribeVolumes operation:`)

// CleanupOrphanEBS reconciles CSI volumes after the original engine has
// destroyed the exact run-owned cluster. It never detaches or deletes live,
// preexisting or ambiguously owned resources. The full layer verifier remains
// responsible for proving all derived resources absent afterwards.
func CleanupOrphanEBS(ctx context.Context, cli AWSCLI, c Config, m Manifest) error {
	if err := verifyAWSIdentity(ctx, cli, c); err != nil {
		return err
	}
	if m.Target != TargetAWS || !m.Armed || m.RunID != c.RunID || m.Account != c.AWS.AccountID || m.Region != c.AWS.Region || m.Principal != c.AWS.PrincipalARN || m.CreatedAt.IsZero() || !m.ExpiresAt.After(m.CreatedAt) {
		return errors.New("CSI volume cleanup requires the original armed AWS manifest")
	}
	var cluster string
	for _, a := range m.Assets {
		if a.Kind != "eks-cluster" {
			continue
		}
		if a.Owner != OwnerCreatedByRun || a.ID == "" || cluster != "" && cluster != a.ID {
			return errors.New("CSI cluster ownership is ambiguous")
		}
		cluster = a.ID
	}
	if cluster == "" {
		return nil // no cluster was created; the final native verifier still runs
	}
	_, stderr, code := cli.Run(ctx, "eks", "describe-cluster", "--name", cluster)
	if code == 0 || !orphanClusterGone.MatchString(strings.TrimSpace(stderr)) {
		return errors.New("CSI cleanup requires native absence of the original cluster")
	}
	read := func(args ...string) ([]orphanEBSVolume, error) {
		raw, _, exit := cli.Run(ctx, append([]string{"ec2", "describe-volumes"}, args...)...)
		var v struct{ Volumes []orphanEBSVolume }
		if exit != 0 || json.Unmarshal(raw, &v) != nil || v.Volumes == nil {
			return nil, errors.New("CSI volume inventory is indeterminate")
		}
		for i := range v.Volumes {
			sort.Slice(v.Volumes[i].Tags, func(a, b int) bool { return v.Volumes[i].Tags[a].Key < v.Volumes[i].Tags[b].Key })
		}
		return v.Volumes, nil
	}
	protected := map[string]bool{}
	for _, asset := range m.Protected() {
		protected[asset.ID] = true
	}
	owned := func(v orphanEBSVolume) bool {
		tags := map[string]string{}
		for _, t := range v.Tags {
			if _, exists := tags[t.Key]; exists {
				return false
			}
			tags[t.Key] = t.Value
		}
		return !protected[v.ID] && orphanVolumeID.MatchString(v.ID) && v.State == "available" && v.Attachments != nil && len(v.Attachments) == 0 &&
			!v.CreateTime.Before(m.CreatedAt) && !v.CreateTime.After(m.ExpiresAt) &&
			tags["kubernetes.io/cluster/"+cluster] == "owned" && tags["KubernetesCluster"] == cluster &&
			tags["kubernetes.io/created-for/pvc/namespace"] == NamespacePrefix+c.RunID &&
			tags["kubernetes.io/created-for/pvc/name"] != "" && orphanPVID.MatchString(tags["CSIVolumeName"]) &&
			tags["CSIVolumeName"] == tags["kubernetes.io/created-for/pv/name"]
	}
	volumes, err := read("--filters", "Name=tag:kubernetes.io/cluster/"+cluster+",Values=owned")
	if err != nil {
		return err
	}
	// Validate the entire inventory before the first mutation.
	seen := map[string]bool{}
	for _, v := range volumes {
		if !owned(v) || seen[v.ID] {
			return errors.New("CSI volume ownership/state is not proven; recovery retained")
		}
		seen[v.ID] = true
	}
	for _, v := range volumes {
		current, err := read("--volume-ids", v.ID)
		if err != nil || len(current) != 1 || current[0].ID != v.ID || !owned(current[0]) {
			return errors.New("exact CSI volume identity changed before deletion")
		}
		before, _ := json.Marshal(v)
		after, _ := json.Marshal(current[0])
		if string(before) != string(after) {
			return errors.New("exact CSI volume metadata changed before deletion")
		}
		if _, _, code := cli.Run(ctx, "ec2", "delete-volume", "--volume-id", v.ID); code != 0 {
			return errors.New("exact CSI volume deletion failed; recovery retained")
		}
		for {
			_, stderr, code := cli.Run(ctx, "ec2", "describe-volumes", "--volume-ids", v.ID)
			if code != 0 && orphanVolumeGone.MatchString(strings.TrimSpace(stderr)) {
				break
			}
			if code != 0 {
				return errors.New("exact CSI volume absence is indeterminate")
			}
			select {
			case <-ctx.Done():
				return errors.New("CSI volume absence deadline exceeded; recovery retained")
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}
