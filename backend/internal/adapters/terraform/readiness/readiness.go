// Package readiness is the real, bounded readiness check of the Terraform v2
// executor. After successful Terraform execution, state acknowledgement and
// output validation, provider completion is sufficient for every managed
// resource type. Known Kubernetes/AWS objects additionally require their
// live readiness probes (UC-08 BR-10). Probe failures never fall back to
// provider completion. Queries are bounded and never expose credentials.
package readiness

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/adapters/awsaccess"
	"orchestrator/internal/ports/terraformexec"
)

// ErrUnavailable means completion evidence is incomplete, the deadline expired,
// or an applicable readiness query could not complete.
var ErrUnavailable = errors.New("readiness: the readiness query could not complete")

// Runner runs one external command with an environment and returns stdout.
type Runner func(ctx context.Context, bin string, args []string, env []string) ([]byte, error)

type commandFailure struct{ notFound bool }

func (commandFailure) Error() string { return "command failed" }
func (e commandFailure) Is(target error) bool {
	return e.notFound && target == terraformexec.ErrNamespaceAbsent
}

// Checker implements terraformexec.ReadinessChecker.
type Checker struct {
	Kubectl  string
	AWS      terraformexec.AWSStatusAPI
	Interval time.Duration
	// Dir is the parent of the private per-check directory.
	Dir string
	// Run is the command seam; the default runs the real kubectl.
	Run Runner
	Now func() time.Time
}

var _ terraformexec.ReadinessChecker = (*Checker)(nil)

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Checker) run() Runner {
	if c.Run != nil {
		return c.Run
	}
	return func(ctx context.Context, bin string, args []string, env []string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = env
		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, commandFailure{notFound: strings.Contains(stderr.String(), "Error from server (NotFound)")}
		}
		return out.Bytes(), nil
	}
}

// probe is one object with a readiness query.
type probe struct {
	label string // safe "address=ID"
	kind  string
	name  string
	ns    string
}

var kubernetesKinds = map[string]string{
	"kubernetes_namespace": "namespace", "kubernetes_namespace_v1": "namespace",
	"kubernetes_stateful_set": "statefulset", "kubernetes_stateful_set_v1": "statefulset",
	"kubernetes_deployment": "deployment", "kubernetes_deployment_v1": "deployment",
}

var awsKinds = map[string]terraformexec.StatusQuery{
	"aws_vpc": terraformexec.StatusEC2Vpc, "aws_eks_cluster": terraformexec.StatusEKSCluster, "aws_eks_node_group": terraformexec.StatusEKSNodegroup,
	"aws_rds_cluster": terraformexec.StatusRDSCluster, "aws_rds_cluster_instance": terraformexec.StatusRDSInstance,
	"aws_elasticache_replication_group": terraformexec.StatusElastiCacheReplicationGroup, "aws_elasticache_cluster": terraformexec.StatusElastiCacheCluster,
}

// awsReady is the ready status of each AWS query.
var awsReady = map[terraformexec.StatusQuery]string{
	terraformexec.StatusEC2Vpc: "available", terraformexec.StatusEKSCluster: "ACTIVE", terraformexec.StatusEKSNodegroup: "ACTIVE",
	terraformexec.StatusRDSCluster: "available", terraformexec.StatusRDSInstance: "available",
	terraformexec.StatusElastiCacheReplicationGroup: "available", terraformexec.StatusElastiCacheCluster: "available",
}

// Check implements terraformexec.ReadinessChecker.
func (c *Checker) Check(ctx context.Context, req terraformexec.ReadinessRequest) error {
	if len(req.Resources) == 0 || req.Deadline.IsZero() || !c.now().Before(req.Deadline) || ctx.Err() != nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, req.Deadline)
	defer cancel()

	var kube []probe
	type awsProbe struct {
		label string
		query terraformexec.StatusQuery
		id    string
	}
	var aws []awsProbe
	for _, r := range req.Resources {
		if strings.TrimSpace(r.Address) == "" || strings.TrimSpace(r.Type) == "" || strings.TrimSpace(r.ID) == "" {
			return ErrUnavailable
		}
		label := r.Address + "=" + r.ID
		if kind, ok := kubernetesKinds[r.Type]; ok {
			p := probe{label: label, kind: kind, name: r.ID}
			if ns, name, found := strings.Cut(r.ID, "/"); found {
				p.ns, p.name = ns, name
			}
			if !validName(p.name) || (p.ns != "" && !validName(p.ns)) || (kind != "namespace" && p.ns == "") {
				return ErrUnavailable
			}
			kube = append(kube, p)
			continue
		}
		if query, ok := awsKinds[r.Type]; ok {
			if r.ID == "" {
				return ErrUnavailable
			}
			id := r.ID
			if query == terraformexec.StatusEKSNodegroup {
				// The provider ID of a node group is "<cluster>:<nodegroup>".
				cluster, group, found := strings.Cut(r.ID, ":")
				if !found {
					return ErrUnavailable
				}
				id = cluster + "/" + group
			}
			aws = append(aws, awsProbe{label: label, query: query, id: id})
		}
	}
	if ctx.Err() != nil {
		return ErrUnavailable
	}
	if len(kube)+len(aws) == 0 {
		// The caller already proved execution, state acknowledgement and
		// outputs. Provider completion applies to all other resource types;
		// an additional readiness probe is not a resource admission allowlist.
		return nil
	}
	deadline := req.Deadline

	var kubeconfig string
	if len(kube) > 0 {
		dir, err := os.MkdirTemp(c.Dir, "readiness-")
		if err != nil {
			return ErrUnavailable
		}
		defer os.RemoveAll(dir)
		kubeconfig, err = writeKubeconfig(dir, req.Env)
		if err != nil {
			return ErrUnavailable
		}
	}
	if len(aws) > 0 && c.AWS == nil {
		return ErrUnavailable
	}
	interval := c.Interval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	for {
		var notReady []string
		queryFailed := false
		for _, p := range kube {
			ready, err := c.kubeReady(ctx, kubeconfig, req.Env, p)
			switch {
			case err != nil:
				queryFailed = true
				notReady = append(notReady, p.label)
			case !ready:
				notReady = append(notReady, p.label)
			}
		}
		var awsConn terraformexec.AWSConnection
		if len(aws) > 0 {
			var ok bool
			awsConn, ok = awsFromEnv(ctx, req.Env)
			if !ok {
				return ErrUnavailable
			}
		}
		for _, p := range aws {
			status, err := c.AWS.Status(ctx, awsConn, p.query, p.id)
			switch {
			case err != nil:
				queryFailed = true
				notReady = append(notReady, p.label)
			case status != awsReady[p.query]:
				notReady = append(notReady, p.label)
			}
		}
		if len(notReady) == 0 {
			return nil
		}
		if ctx.Err() != nil || !c.now().Before(deadline) {
			sort.Strings(notReady)
			if queryFailed {
				return ErrUnavailable
			}
			return fmt.Errorf("%w: %s", terraformexec.ErrNotReady, strings.Join(notReady, ", "))
		}
		select {
		case <-time.After(interval):
		case <-ctx.Done():
		}
	}
}

func validName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

// kubeReady queries one Kubernetes object.
func (c *Checker) kubeReady(ctx context.Context, kubeconfig string, env []string, p probe) (bool, error) {
	args := []string{"--kubeconfig", kubeconfig}
	if ctxName := value(env, "KUBE_CTX"); ctxName != "" {
		args = append(args, "--context", ctxName)
	}
	args = append(args, "get", p.kind, p.name, "-o", "json")
	if p.ns != "" {
		args = append(args, "-n", p.ns)
	}
	out, err := c.run()(ctx, c.Kubectl, args, []string{"PATH=" + filepath.Join(os.TempDir(), "no-path"), "HOME=" + filepath.Dir(kubeconfig), "KUBECONFIG=" + kubeconfig})
	if err != nil {
		return false, err
	}
	var doc struct {
		Metadata struct {
			Generation int64 `json:"generation"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int64 `json:"replicas"`
		} `json:"spec"`
		Status struct {
			Phase              string `json:"phase"`
			ObservedGeneration int64  `json:"observedGeneration"`
			ReadyReplicas      int64  `json:"readyReplicas"`
			UpdatedReplicas    int64  `json:"updatedReplicas"`
			AvailableReplicas  int64  `json:"availableReplicas"`
		} `json:"status"`
	}
	if json.Unmarshal(out, &doc) != nil {
		return false, ErrUnavailable
	}
	want := int64(1)
	if doc.Spec.Replicas != nil {
		want = *doc.Spec.Replicas
	}
	switch p.kind {
	case "namespace":
		return doc.Status.Phase == "Active", nil
	case "statefulset":
		return doc.Status.ObservedGeneration >= doc.Metadata.Generation && doc.Status.ReadyReplicas >= want && doc.Status.UpdatedReplicas >= want, nil
	case "deployment":
		return doc.Status.ObservedGeneration >= doc.Metadata.Generation && doc.Status.AvailableReplicas >= want && doc.Status.UpdatedReplicas >= want, nil
	}
	return false, ErrUnavailable
}

func value(env []string, key string) string {
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, key+"="); ok {
			return v
		}
	}
	return ""
}

// writeKubeconfig materializes the scoped Kubernetes access of the provider
// environment as a private kubeconfig (Connection file or EKS token) for
// kubectl; it never falls back to a default or host config.
func writeKubeconfig(dir string, env []string) (string, error) {
	path := filepath.Join(dir, "kubeconfig")
	if src := value(env, "KUBE_CONFIG_PATH"); src != "" {
		data, err := os.ReadFile(src)
		if err != nil || len(data) == 0 {
			return "", ErrUnavailable
		}
		return path, os.WriteFile(path, data, 0o600)
	}
	host, token, ca := value(env, "KUBE_HOST"), value(env, "KUBE_TOKEN"), value(env, "KUBE_CLUSTER_CA_CERT_DATA")
	if !strings.HasPrefix(host, "https://") || token == "" || ca == "" {
		return "", ErrUnavailable
	}
	doc := map[string]any{
		"apiVersion": "v1", "kind": "Config", "current-context": "scoped",
		"clusters": []any{map[string]any{"name": "c", "cluster": map[string]any{"server": host, "certificate-authority-data": base64.StdEncoding.EncodeToString([]byte(ca))}}},
		"users":    []any{map[string]any{"name": "u", "user": map[string]any{"token": token}}},
		"contexts": []any{map[string]any{"name": "scoped", "context": map[string]any{"cluster": "c", "user": "u"}}},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return "", ErrUnavailable
	}
	return path, os.WriteFile(path, data, 0o600)
}

func awsFromEnv(ctx context.Context, env []string) (terraformexec.AWSConnection, bool) {
	if uri := value(env, "AWS_CONTAINER_CREDENTIALS_FULL_URI"); uri != "" {
		conn, err := awsaccess.ReadCredentialEndpoint(ctx, uri, value(env, "AWS_CONTAINER_AUTHORIZATION_TOKEN"), value(env, "AWS_REGION"))
		return conn, err == nil
	}
	conn := terraformexec.AWSConnection{
		AccessKeyID: value(env, "AWS_ACCESS_KEY_ID"), SecretAccessKey: value(env, "AWS_SECRET_ACCESS_KEY"),
		SessionToken: value(env, "AWS_SESSION_TOKEN"), Region: value(env, "AWS_REGION"),
	}
	return conn, conn.AccessKeyID != "" && conn.SecretAccessKey != "" && conn.Region != ""
}
