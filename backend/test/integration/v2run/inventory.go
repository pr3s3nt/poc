package v2run

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Query is one read-only physical-existence check. Execute Argv (kubectl or
// aws), then call Present with the outputs. An answer is only ever "absent"
// when the service says so (empty result or a documented not-found error);
// AccessDenied, throttling and timeouts are indeterminate and never count as
// absence. Empty Terraform state and destroy exit 0 are never evidence.
type Query struct {
	ID   string
	Argv []string
	// List is the top-level JSON array that holds the objects ("" = none).
	List string
	// Nested flattens one level (for example Reservations[].Instances).
	Nested string
	// Gone reports an object that no longer counts (deleted/terminated).
	Gone func(obj map[string]any) bool
	// IDKey names the identifying field of an object, for safe reporting.
	IDKey string
}

// Presence is the verdict of one query.
type Presence struct {
	Query string
	// Present lists the safe IDs still found.
	Present []string
	// Indeterminate is set when the check could not prove either way.
	Indeterminate string
}

// Absent reports a verified-absent verdict.
func (p Presence) Absent() bool { return len(p.Present) == 0 && p.Indeterminate == "" }

var notFound = regexp.MustCompile(`(?i)(NotFound|NoSuchEntity|ResourceNotFoundException|DBClusterNotFoundFault|ReplicationGroupNotFoundFault|RepositoryNotFoundException|InvalidVpcID\.NotFound|InvalidSubnetID\.NotFound|InvalidGroup\.NotFound|InvalidInternetGatewayID\.NotFound|LoadBalancerNotFound|Error from server \(NotFound\)|\(NotFound\))`)

// Evaluate interprets a query result. exit is the process exit code.
func (q Query) Evaluate(stdout []byte, stderr string, exit int) Presence {
	p := Presence{Query: q.ID}
	if exit != 0 {
		if notFound.MatchString(stderr) {
			return p // documented not-found: absent
		}
		p.Indeterminate = fmt.Sprintf("exit %d without a not-found answer", exit)
		return p
	}
	if q.List == "" {
		// A plain read that exits 0 found the object.
		if trimmed := strings.TrimSpace(string(stdout)); trimmed != "" {
			p.Present = []string{q.ID}
		}
		return p
	}
	var doc map[string]any
	if err := json.Unmarshal(stdout, &doc); err != nil {
		p.Indeterminate = "unparsable response"
		return p
	}
	items, ok := doc[q.List].([]any)
	if !ok {
		if doc[q.List] == nil {
			return p
		}
		p.Indeterminate = "unexpected response shape"
		return p
	}
	var objs []map[string]any
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			p.Indeterminate = "unexpected item shape"
			return p
		}
		if q.Nested != "" {
			inner, _ := m[q.Nested].([]any)
			for _, in := range inner {
				if im, ok := in.(map[string]any); ok {
					objs = append(objs, im)
				}
			}
			continue
		}
		objs = append(objs, m)
	}
	for i, obj := range objs {
		if q.Gone != nil && q.Gone(obj) {
			continue
		}
		id := fmt.Sprintf("%s[%d]", q.List, i)
		if q.IDKey != "" {
			if s, ok := obj[q.IDKey].(string); ok && safeID.MatchString(s) {
				id = s
			}
		}
		p.Present = append(p.Present, id)
	}
	sort.Strings(p.Present)
	return p
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9:/._-]{1,200}$`)

func state(key string, gone ...string) func(map[string]any) bool {
	return func(o map[string]any) bool {
		v := ""
		switch s := o[key].(type) {
		case string:
			v = s
		case map[string]any:
			v, _ = s["Name"].(string)
		}
		for _, g := range gone {
			if strings.EqualFold(v, g) {
				return true
			}
		}
		return false
	}
}

// AWSLinkage names the exact owner chain of the derived assets of one run.
// Derived assets (ENIs, security groups, instances, volumes, load balancers
// created by the EKS service) are found by exact VPC ID, exact cluster name
// and the run tag; there is no name-prefix or wildcard matching.
type AWSLinkage struct {
	VPCID, ClusterName, NodeGroup, DBCluster, CacheGroup string
	Repositories, Roles                                  []string
}

// AWSQueries returns the read-only checks that prove the run's AWS assets are
// gone. Region is fixed by the caller's --region. Service-linked roles are
// preexisting and are never queried for deletion.
func AWSQueries(runID string, l AWSLinkage) []Query {
	aws := func(args ...string) []string {
		return append([]string{"aws", "--region", AWSRegion, "--output", "json"}, args...)
	}
	tag := "Name=tag:run-id,Values=" + runID
	q := []Query{
		{ID: "tagged-resources", Argv: aws("resourcegroupstaggingapi", "get-resources", "--tag-filters", "Key=run-id,Values="+runID), List: "ResourceTagMappingList", IDKey: "ResourceARN"},
	}
	if l.ClusterName != "" {
		q = append(q,
			Query{ID: "eks-cluster", Argv: aws("eks", "describe-cluster", "--name", l.ClusterName)},
			Query{ID: "eks-nodegroups", Argv: aws("eks", "list-nodegroups", "--cluster-name", l.ClusterName), List: "nodegroups"},
			Query{ID: "ec2-instances-cluster", Argv: aws("ec2", "describe-instances", "--filters", "Name=tag:eks:cluster-name,Values="+l.ClusterName), List: "Reservations", Nested: "Instances", Gone: state("State", "terminated"), IDKey: "InstanceId"},
			Query{ID: "ebs-volumes-cluster", Argv: aws("ec2", "describe-volumes", "--filters", "Name=tag:kubernetes.io/cluster/"+l.ClusterName+",Values=owned"), List: "Volumes", IDKey: "VolumeId"},
		)
	}
	q = append(q,
		Query{ID: "ec2-instances-run", Argv: aws("ec2", "describe-instances", "--filters", tag), List: "Reservations", Nested: "Instances", Gone: state("State", "terminated"), IDKey: "InstanceId"},
		Query{ID: "ebs-volumes-run", Argv: aws("ec2", "describe-volumes", "--filters", tag), List: "Volumes", IDKey: "VolumeId"},
		Query{ID: "elastic-ips-run", Argv: aws("ec2", "describe-addresses", "--filters", tag), List: "Addresses", IDKey: "AllocationId"},
	)
	if l.VPCID != "" {
		vpc := "Name=vpc-id,Values=" + l.VPCID
		q = append(q,
			Query{ID: "vpc", Argv: aws("ec2", "describe-vpcs", "--vpc-ids", l.VPCID), List: "Vpcs", IDKey: "VpcId"},
			Query{ID: "subnets", Argv: aws("ec2", "describe-subnets", "--filters", vpc), List: "Subnets", IDKey: "SubnetId"},
			Query{ID: "security-groups", Argv: aws("ec2", "describe-security-groups", "--filters", vpc), List: "SecurityGroups", IDKey: "GroupId"},
			Query{ID: "route-tables", Argv: aws("ec2", "describe-route-tables", "--filters", vpc), List: "RouteTables", IDKey: "RouteTableId"},
			Query{ID: "internet-gateways", Argv: aws("ec2", "describe-internet-gateways", "--filters", "Name=attachment.vpc-id,Values="+l.VPCID), List: "InternetGateways", IDKey: "InternetGatewayId"},
			Query{ID: "nat-gateways", Argv: aws("ec2", "describe-nat-gateways", "--filter", vpc), List: "NatGateways", Gone: state("State", "deleted"), IDKey: "NatGatewayId"},
			Query{ID: "network-interfaces", Argv: aws("ec2", "describe-network-interfaces", "--filters", vpc), List: "NetworkInterfaces", IDKey: "NetworkInterfaceId"},
			Query{ID: "ec2-instances-vpc", Argv: aws("ec2", "describe-instances", "--filters", vpc), List: "Reservations", Nested: "Instances", Gone: state("State", "terminated"), IDKey: "InstanceId"},
		)
	}
	if l.DBCluster != "" {
		q = append(q, Query{ID: "rds-cluster", Argv: aws("rds", "describe-db-clusters", "--db-cluster-identifier", l.DBCluster), List: "DBClusters", IDKey: "DBClusterIdentifier"})
	}
	if l.CacheGroup != "" {
		q = append(q, Query{ID: "elasticache-group", Argv: aws("elasticache", "describe-replication-groups", "--replication-group-id", l.CacheGroup), List: "ReplicationGroups", IDKey: "ReplicationGroupId"})
	}
	for _, repo := range l.Repositories {
		q = append(q, Query{ID: "ecr-" + repo, Argv: aws("ecr", "describe-repositories", "--repository-names", repo), List: "repositories", IDKey: "repositoryName"})
	}
	for _, role := range l.Roles {
		q = append(q, Query{ID: "iam-role-" + role, Argv: aws("iam", "get-role", "--role-name", role)})
	}
	return q
}

// K8sQueries are the read-only checks of the k8s-4f cleanup: the exact run
// namespace and each recorded run-owned PV/PVC by exact name. kubectl is given
// the host kubeconfig handle by the caller (operator verification only).
func K8sQueries(m Manifest, kubectl, kubeconfig, kubeContext string) []Query {
	base := func(args ...string) []string {
		return append([]string{kubectl, "--kubeconfig", kubeconfig, "--context", kubeContext}, args...)
	}
	q := []Query{}
	if m.Namespace != "" {
		q = append(q, Query{ID: "namespace", Argv: base("get", "namespace", m.Namespace, "-o", "name")})
	}
	for _, a := range m.OwnedByRun() {
		switch a.Kind {
		case "pv":
			q = append(q, Query{ID: "pv-" + a.ID, Argv: base("get", "pv", a.ID, "-o", "name")})
		case "pvc":
			q = append(q, Query{ID: "pvc-" + a.ID, Argv: base("-n", m.Namespace, "get", "pvc", a.ID, "-o", "name")})
		}
	}
	return q
}

// PVsClaimedBy lists PVs whose claim lives in namespace: a PV left behind by a
// run-owned PVC. The input is `kubectl get pv -o json`.
func PVsClaimedBy(namespace string, pvList []byte) ([]string, error) {
	var doc struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				ClaimRef *struct {
					Namespace string `json:"namespace"`
				} `json:"claimRef"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(pvList, &doc); err != nil {
		return nil, err
	}
	var out []string
	for _, it := range doc.Items {
		if it.Spec.ClaimRef != nil && it.Spec.ClaimRef.Namespace == namespace {
			out = append(out, it.Metadata.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}
