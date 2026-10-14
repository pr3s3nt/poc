package v2run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ServiceLinkedRoles are the account-wide roles the EKS, node group, RDS and
// ElastiCache services use. They pre-exist or are created by AWS itself and must
// be PRESERVED: the run never deletes them and the snapshot proves it.
var ServiceLinkedRoles = []string{"AWSServiceRoleForAmazonEKS", "AWSServiceRoleForAmazonEKSNodegroup", "AWSServiceRoleForRDS", "AWSServiceRoleForElastiCache"}

// SLRState is the comparable shape of one role.
type SLRState struct {
	Present      bool     `json:"present"`
	ARN          string   `json:"arn,omitempty"`
	RoleID       string   `json:"roleId,omitempty"`
	Path         string   `json:"path,omitempty"`
	CreateDate   string   `json:"createDate,omitempty"`
	TrustSHA256  string   `json:"trustSha256,omitempty"`
	Attached     []string `json:"attachedPolicies,omitempty"`
	AttachedHash string   `json:"attachedSha256,omitempty"`
}

// SnapshotSLRs reads (read-only) every service-linked role. NoSuchEntity is an
// honest "absent"; any other failure is an error (never assumed absent).
func SnapshotSLRs(ctx context.Context, cli AWSCLI) (map[string]SLRState, error) {
	out := map[string]SLRState{}
	for _, name := range ServiceLinkedRoles {
		raw, stderr, exit := cli.Run(ctx, "iam", "get-role", "--role-name", name)
		if exit != 0 {
			if notFound.MatchString(stderr) {
				out[name] = SLRState{}
				continue
			}
			return nil, fmt.Errorf("the service-linked role %s could not be read: %s", name, firstLine(stderr))
		}
		var doc struct {
			Role struct {
				Arn, RoleId, Path, CreateDate string
				AssumeRolePolicyDocument      any
			}
		}
		if err := json.Unmarshal(raw, &doc); err != nil || doc.Role.Arn == "" {
			return nil, fmt.Errorf("the service-linked role %s answer is unreadable", name)
		}
		trust, _ := json.Marshal(doc.Role.AssumeRolePolicyDocument)
		sum := sha256.Sum256(trust)
		attachedRaw, stderr2, exit2 := cli.Run(ctx, "iam", "list-attached-role-policies", "--role-name", name)
		if exit2 != 0 {
			return nil, fmt.Errorf("the policies of %s could not be read: %s", name, firstLine(stderr2))
		}
		var attached struct{ AttachedPolicies []struct{ PolicyArn string } }
		if err := json.Unmarshal(attachedRaw, &attached); err != nil {
			return nil, fmt.Errorf("the policies of %s are unreadable", name)
		}
		var arns []string
		for _, p := range attached.AttachedPolicies {
			arns = append(arns, p.PolicyArn)
		}
		sort.Strings(arns)
		h := sha256.Sum256([]byte(strings.Join(arns, ",")))
		out[name] = SLRState{Present: true, ARN: doc.Role.Arn, RoleID: doc.Role.RoleId, Path: doc.Role.Path, CreateDate: doc.Role.CreateDate,
			TrustSHA256: hex.EncodeToString(sum[:]), Attached: arns, AttachedHash: hex.EncodeToString(h[:])}
	}
	return out, nil
}

// CompareSLRs proves preservation: every role present before is present after
// with the same ARN, ID, path, creation date, trust policy and attached
// policies. A role absent before may be created by the AWS service during the
// run and then stays (it is reported, never a failure and never deleted).
func CompareSLRs(before, after map[string]SLRState) error {
	if len(before) == 0 {
		return errors.New("no service-linked role snapshot was taken before the run")
	}
	var problems []string
	for _, name := range ServiceLinkedRoles {
		b, ok := before[name]
		if !ok {
			problems = append(problems, name+": missing from the snapshot")
			continue
		}
		if !b.Present {
			continue
		}
		a := after[name]
		switch {
		case !a.Present:
			problems = append(problems, name+": was deleted")
		case a.ARN != b.ARN || a.RoleID != b.RoleID || a.Path != b.Path || a.CreateDate != b.CreateDate:
			problems = append(problems, name+": identity changed")
		case a.TrustSHA256 != b.TrustSHA256:
			problems = append(problems, name+": trust policy changed")
		case a.AttachedHash != b.AttachedHash:
			problems = append(problems, name+": attached policies changed")
		}
	}
	if len(problems) > 0 {
		return errors.New("preexisting service-linked roles not preserved: " + strings.Join(problems, "; "))
	}
	return nil
}
