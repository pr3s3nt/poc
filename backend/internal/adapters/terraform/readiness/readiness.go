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
	"orchestrator/internal/adapters/kubernetes/stuckpod"
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
	// StuckThreshold is how long a StatefulSet Pod must wait on a
	// non-recovering reason before it is replaced (UC-08 BR-11).
	StuckThreshold time.Duration
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
		if strings.TrimSpace(r.Address) == "" || strings.TrimSpace(r.Type) == "" {
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
		// Empty (not nil) so a ready poll clears earlier wait reasons.
		waits := []terraformexec.PodWait{}
		queryFailed := false
		for _, p := range kube {
			ready, object, err := c.kubeReady(ctx, kubeconfig, req.Env, p)
			switch {
			case err != nil:
				queryFailed = true
				notReady = append(notReady, p.label)
			case !ready:
				notReady = append(notReady, p.label)
				if p.kind == "statefulset" {
					waits = append(waits, c.recoverStuckPods(ctx, kubeconfig, req, p, object)...)
				}
			}
		}
		if req.Observe != nil && len(kube) > 0 {
			req.Observe(terraformexec.ReadinessObservation{Waiting: waits})
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
			if len(waits) > 0 {
				return fmt.Errorf("%w: %s (%s)", terraformexec.ErrNotReady, strings.Join(notReady, ", "), waitSummary(waits))
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

func (c *Checker) kubectl(ctx context.Context, kubeconfig string, env []string, args ...string) ([]byte, error) {
	base := []string{"--kubeconfig", kubeconfig}
	if ctxName := value(env, "KUBE_CTX"); ctxName != "" {
		base = append(base, "--context", ctxName)
	}
	return c.run()(ctx, c.Kubectl, append(base, args...), []string{"PATH=" + filepath.Join(os.TempDir(), "no-path"), "HOME=" + filepath.Dir(kubeconfig), "KUBECONFIG=" + kubeconfig})
}

// recoverStuckPods applies UC-08 BR-11 to a StatefulSet that is not ready: it
// reports Pods waiting on a non-recovering reason and, only when a new
// revision exists, deletes at most one stuck Pod of an older revision. Pod
// queries and deletion are best effort: a failure (for example a missing
// delete permission) never changes the readiness verdict.
func (c *Checker) recoverStuckPods(ctx context.Context, kubeconfig string, req terraformexec.ReadinessRequest, p probe, object map[string]any) []terraformexec.PodWait {
	selector := matchLabels(object)
	if selector == "" {
		return nil
	}
	out, err := c.kubectl(ctx, kubeconfig, req.Env, "get", "pods", "-n", p.ns, "-l", selector, "-o", "json")
	if err != nil {
		return nil
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(out, &list) != nil {
		return nil
	}
	now := c.now()
	decision := stuckpod.Evaluate(object, list.Items, now, c.StuckThreshold)
	objectName := p.ns + "/" + p.name
	waits := make([]terraformexec.PodWait, 0, len(decision.Reasons))
	for _, r := range decision.Reasons {
		waits = append(waits, podWait(objectName, r))
	}
	if r := decision.Replace; r != nil && validName(r.Pod) {
		if _, err := c.kubectl(ctx, kubeconfig, req.Env, "delete", "pod", r.Pod, "-n", p.ns, "--wait=false"); err == nil && req.Observe != nil {
			req.Observe(terraformexec.ReadinessObservation{Waiting: waits, Replaced: &terraformexec.PodReplacement{Wait: podWait(objectName, r.Reason), At: now.UTC()}})
		}
	}
	return waits
}

func podWait(object string, r stuckpod.Reason) terraformexec.PodWait {
	return terraformexec.PodWait{Object: object, Pod: r.Pod, Container: r.Container, Code: r.Code, Image: r.Image, Restarts: r.Restarts, StuckSeconds: r.StuckSeconds}
}

func waitSummary(waits []terraformexec.PodWait) string {
	parts := make([]string, 0, len(waits))
	for _, w := range waits {
		parts = append(parts, stuckpod.Reason{Pod: w.Pod, Code: w.Code, Image: w.Image, Restarts: w.Restarts}.String())
	}
	return strings.Join(parts, "; ")
}

// matchLabels renders spec.selector.matchLabels as a kubectl label selector.
func matchLabels(object map[string]any) string {
	spec, _ := object["spec"].(map[string]any)
	selector, _ := spec["selector"].(map[string]any)
	labels, _ := selector["matchLabels"].(map[string]any)
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v, _ := labels[k].(string)
		if strings.ContainsAny(k+v, ",=! ") {
			return ""
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

// kubeReady queries one Kubernetes object and returns it for further checks.
func (c *Checker) kubeReady(ctx context.Context, kubeconfig string, env []string, p probe) (bool, map[string]any, error) {
	args := []string{"get", p.kind, p.name, "-o", "json"}
	if p.ns != "" {
		args = append(args, "-n", p.ns)
	}
	out, err := c.kubectl(ctx, kubeconfig, env, args...)
	if err != nil {
		return false, nil, err
	}
	var object map[string]any
	if json.Unmarshal(out, &object) != nil {
		return false, nil, ErrUnavailable
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
		return false, nil, ErrUnavailable
	}
	want := int64(1)
	if doc.Spec.Replicas != nil {
		want = *doc.Spec.Replicas
	}
	switch p.kind {
	case "namespace":
		return doc.Status.Phase == "Active", object, nil
	case "statefulset":
		return doc.Status.ObservedGeneration >= doc.Metadata.Generation && doc.Status.ReadyReplicas >= want && doc.Status.UpdatedReplicas >= want, object, nil
	case "deployment":
		return doc.Status.ObservedGeneration >= doc.Metadata.Generation && doc.Status.AvailableReplicas >= want && doc.Status.UpdatedReplicas >= want, object, nil
	}
	return false, nil, ErrUnavailable
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
