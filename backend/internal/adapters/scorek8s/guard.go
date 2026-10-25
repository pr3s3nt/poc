package scorek8s

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/renderengine"
)

// guardError is one violated Git render invariant (contract RB-C05). The
// reason is for tests and internal diagnosis; delivery never shows it.
type guardError struct{ reason string }

func (e *guardError) Error() string { return "score-k8s: render invariant violated: " + e.reason }

func violated(format string, args ...any) error {
	return &guardError{reason: fmt.Sprintf(format, args...)}
}

// clusterScopedKinds are well-known cluster-scoped kinds refused before any
// API discovery (ADR-024); the deployer still checks discovery at apply.
var clusterScopedKinds = set("Namespace", "Node", "PersistentVolume", "ClusterRole", "ClusterRoleBinding", "CustomResourceDefinition",
	"StorageClass", "PriorityClass", "RuntimeClass", "IngressClass", "GatewayClass", "CSIDriver", "CSINode", "VolumeAttachment",
	"MutatingWebhookConfiguration", "ValidatingWebhookConfiguration", "ValidatingAdmissionPolicy", "ValidatingAdmissionPolicyBinding",
	"APIService", "CertificateSigningRequest", "FlowSchema", "PriorityLevelConfiguration", "ClusterIssuer")

func set(names ...string) map[string]bool {
	out := map[string]bool{}
	for _, name := range names {
		out[name] = true
	}
	return out
}

// reservedLabel reports platform identity label keys an author cannot own.
func reservedLabel(key string) bool {
	return strings.HasPrefix(key, "app.kubernetes.io/name") || strings.HasPrefix(key, "app.kubernetes.io/managed-by") || strings.HasPrefix(key, "orchestrator.io/")
}

// reservedAnnotation reports keys only platform delivery may set.
func reservedAnnotation(key string) bool {
	return strings.HasPrefix(key, "vault.hashicorp.com/") || strings.HasPrefix(key, "orchestrator.io/")
}

// objectName is a Kubernetes DNS subdomain object name.
var objectName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// guard validates Git-rendered objects against the platform-supplied intent
// (contract RB-C08): the primary workload and Service keep identity and
// explicit Score intent; any other namespaced object is bundle policy, except
// cluster-scoped, Secret and secret-delivery objects. It does not compare with
// a native policy render.
func guard(req execution.RenderRequest, delivery kubernetes.Delivery, objects []execution.Manifest) error {
	if len(objects) > renderengine.MaxObjects {
		return violated("at most %d objects are allowed", renderengine.MaxObjects)
	}
	var workload, service map[string]any
	workloadKind := ""
	seen := map[string]bool{}
	for _, m := range objects {
		key := m.APIVersion + "/" + m.Kind + "/" + m.Name
		if seen[key] {
			return violated("objects must be unique by kind and name")
		}
		seen[key] = true
		if m.Namespace != "" && m.Namespace != req.Namespace {
			return violated("objects must be in the workload namespace")
		}
		if len(m.Name) > 253 || !objectName.MatchString(m.Name) {
			return violated("object names must be DNS subdomains")
		}
		group := m.APIVersion
		if i := strings.Index(group, "/"); i >= 0 {
			group = group[:i]
		} else {
			group = ""
		}
		switch {
		case m.APIVersion == "v1" && (m.Kind == "Secret" || m.Kind == "Namespace"):
			return violated("Secret and Namespace objects are platform-owned")
		case group == "secrets.hashicorp.com":
			return violated("secret delivery objects are platform-owned")
		case clusterScopedKinds[m.Kind]:
			return violated("cluster-scoped objects are not allowed")
		}
		primaryWorkload := m.APIVersion == "apps/v1" && (m.Kind == "Deployment" || m.Kind == "StatefulSet")
		primary := m.Name == req.WorkloadID && (primaryWorkload || m.APIVersion == "v1" && m.Kind == "Service")
		if err := guardOwnedMetadata(object(m.Object["metadata"]), delivery, primary); err != nil {
			return err
		}
		if !primary {
			continue
		}
		switch {
		case primaryWorkload:
			if workload != nil {
				return violated("only one primary workload object is allowed")
			}
			workload, workloadKind = m.Object, m.Kind
		case m.APIVersion == "v1" && m.Kind == "Service":
			service = m.Object
		}
	}
	if workload == nil {
		return violated("the primary Deployment or StatefulSet is missing")
	}
	wantService := req.Module.Spec.Service != nil && len(req.Module.Spec.Service.Ports) > 0
	if (service != nil) != wantService {
		return violated("a Service is required exactly when the Score declares service ports")
	}
	if err := guardWorkload(req, delivery, workloadKind, workload); err != nil {
		return err
	}
	if service != nil {
		return guardService(req, delivery, service)
	}
	return nil
}

// guardOwnedMetadata: reserved labels/annotations are platform values only.
// Primary objects must carry the identity labels.
func guardOwnedMetadata(meta map[string]any, delivery kubernetes.Delivery, primary bool) error {
	if meta == nil {
		return violated("object metadata is missing")
	}
	labels := object(meta["labels"])
	if _, present := meta["labels"]; present && labels == nil {
		return violated("labels must be an object")
	}
	for key, value := range labels {
		if _, text := value.(string); !text {
			return violated("label values must be strings")
		}
		if reservedLabel(key) {
			if want, platform := delivery.Labels[key]; !platform || value != want {
				if key != "app.kubernetes.io/name" || primary {
					return violated("reserved labels are platform-owned")
				}
			}
		}
	}
	if primary {
		for key, value := range delivery.Labels {
			if labels[key] != value {
				return violated("the primary objects must carry the platform identity labels")
			}
		}
	}
	annotations := object(meta["annotations"])
	if _, present := meta["annotations"]; present && annotations == nil {
		return violated("annotations must be an object")
	}
	for key, value := range annotations {
		if _, text := value.(string); !text {
			return violated("annotation values must be strings")
		}
		if reservedAnnotation(key) {
			return violated("reserved annotations are platform-owned")
		}
	}
	return nil
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func list(v any) []any            { l, _ := v.([]any); return l }

func sameStringMap(actual any, want map[string]string) bool {
	m := object(actual)
	if len(m) != len(want) {
		return false
	}
	for key, value := range want {
		if m[key] != value {
			return false
		}
	}
	return true
}

// guardWorkload checks the primary Deployment or StatefulSet: identity,
// selector, platform delivery references and explicit Score intent. Other
// pod and container fields are bundle policy (ADR-024).
func guardWorkload(req execution.RenderRequest, delivery kubernetes.Delivery, kind string, d map[string]any) error {
	spec := object(d["spec"])
	if req.Module.Spec.Replicas != nil {
		if replicas, ok := spec["replicas"].(float64); !ok || int(replicas) != req.Module.Spec.ReplicaCount() {
			return violated("explicit Score replicas must be kept")
		}
	} else if raw, present := spec["replicas"]; present {
		if replicas, ok := raw.(float64); !ok || replicas < 0 || replicas != float64(int(replicas)) {
			return violated("replicas must be a non-negative integer")
		}
	}
	selector := object(spec["selector"])
	if !sameStringMap(selector["matchLabels"], delivery.Selector) {
		return violated("the %s selector must be the platform selector", kind)
	}
	template := object(spec["template"])
	templateMeta := object(template["metadata"])
	labels := object(templateMeta["labels"])
	for key, value := range delivery.Labels {
		if labels[key] != value {
			return violated("the pod template must carry the platform identity labels")
		}
	}
	for key, value := range labels {
		if want, platform := delivery.Labels[key]; reservedLabel(key) && (!platform || value != want) {
			return violated("reserved labels are platform-owned")
		}
	}
	annotations := object(templateMeta["annotations"])
	for key, value := range annotations {
		if reservedAnnotation(key) && delivery.PodAnnotations[key] != value {
			return violated("reserved annotations are platform-owned")
		}
	}
	for key, value := range delivery.PodAnnotations {
		if annotations[key] != value {
			return violated("the pod template must carry the platform delivery annotations")
		}
	}
	pod := object(template["spec"])
	if delivery.ImagePullSecret != "" {
		found := false
		for _, ref := range list(pod["imagePullSecrets"]) {
			if object(ref)["name"] == delivery.ImagePullSecret {
				found = true
			}
		}
		if !found {
			return violated("the platform image pull Secret must be kept")
		}
	}
	if delivery.ServiceAccount != "" && pod["serviceAccountName"] != delivery.ServiceAccount {
		return violated("the delivery ServiceAccount is platform-owned")
	}
	containers := map[string]map[string]any{}
	for _, raw := range list(pod["containers"]) {
		c := object(raw)
		name, _ := c["name"].(string)
		if _, dup := containers[name]; dup || name == "" {
			return violated("container names must be unique")
		}
		containers[name] = c
	}
	for name, declared := range req.Module.Spec.Containers {
		c, ok := containers[name]
		if !ok {
			return violated("every Score container must be kept")
		}
		if err := guardContainer(c, declared, delivery.Env[name]); err != nil {
			return err
		}
	}
	return nil
}

func stringList(v any) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(items))
	for i, item := range items {
		text, isText := item.(string)
		if !isText {
			return nil, false
		}
		out[i] = text
	}
	return out, true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func guardContainer(c map[string]any, declared environment.Container, bindings []kubernetes.EnvBinding) error {
	if c["image"] != declared.Image {
		return violated("the Score image must be kept")
	}
	command, ok := stringList(c["command"])
	if !ok || !equalStrings(command, declared.Command) {
		return violated("the Score command must be kept")
	}
	args, ok := stringList(c["args"])
	if !ok || !equalStrings(args, declared.Args) {
		return violated("the Score args must be kept")
	}
	for probe, want := range map[string]*environment.Probe{"readinessProbe": declared.ReadinessProbe, "livenessProbe": declared.LivenessProbe} {
		if want == nil {
			continue
		}
		get := object(object(c[probe])["httpGet"])
		if get["path"] != want.Path || toNumber(get["port"]) != float64(want.Port) {
			return violated("explicit Score probes must be kept")
		}
	}
	if err := guardResources(object(c["resources"]), declared.Resources); err != nil {
		return err
	}
	return guardEnv(list(c["env"]), bindings)
}

func toNumber(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return -1
}

// guardResources: an explicit request wins, then its same-field limit, then
// the bundle default; explicit limits are exact. Missing limits and other
// resource names are bundle policy (ADR-024).
func guardResources(actual map[string]any, declared *environment.ContainerResourceRequirements) error {
	var requests, limits environment.ComputeResources
	if declared != nil && declared.Requests != nil {
		requests = *declared.Requests
	}
	if declared != nil && declared.Limits != nil {
		limits = *declared.Limits
	}
	gotRequests, gotLimits := object(actual["requests"]), object(actual["limits"])
	for _, field := range []struct{ name, request, limit string }{{"cpu", requests.CPU, limits.CPU}, {"memory", requests.Memory, limits.Memory}} {
		want := field.request
		if want == "" {
			want = field.limit
		}
		if got, present := gotRequests[field.name]; want != "" && (!present || got != want) {
			return violated("explicit Score %s requests or same-field limits must be kept", field.name)
		}
		if field.limit != "" && gotLimits[field.name] != field.limit {
			return violated("explicit Score %s limits must be kept", field.name)
		}
	}
	return nil
}

// guardEnv requires every approved binding with its exact value or Secret
// reference (any order). Other entries are bundle policy (ADR-024). score-k8s
// omits the value of an empty plain variable; that is the same binding.
func guardEnv(env []any, bindings []kubernetes.EnvBinding) error {
	entries := map[string]map[string]any{}
	for _, raw := range env {
		entry := object(raw)
		name, _ := entry["name"].(string)
		if _, dup := entries[name]; dup {
			return violated("environment entries must be unique")
		}
		entries[name] = entry
	}
	for _, b := range bindings {
		entry, ok := entries[b.Name]
		if !ok {
			return violated("every approved environment binding must be kept")
		}
		if b.Secret {
			if _, plain := entry["value"]; plain {
				return violated("Secret bindings must stay references")
			}
			from := object(entry["valueFrom"])
			if len(from) != 1 || !sameStringMap(from["secretKeyRef"], map[string]string{"name": b.SecretRef, "key": b.SecretKey}) {
				return violated("Secret references must be the approved name and key")
			}
			continue
		}
		if _, from := entry["valueFrom"]; from {
			return violated("plain bindings must stay values")
		}
		value, present := entry["value"]
		if !present {
			value = ""
		}
		if value != b.Value {
			return violated("plain binding values must be kept")
		}
	}
	return nil
}

func guardService(req execution.RenderRequest, delivery kubernetes.Delivery, s map[string]any) error {
	spec := object(s["spec"])
	if !sameStringMap(spec["selector"], delivery.Selector) {
		return violated("the Service selector must be the platform selector")
	}
	type port struct {
		Name       string  `json:"name"`
		Port       float64 `json:"port"`
		TargetPort float64 `json:"targetPort"`
		Protocol   string  `json:"protocol"`
	}
	got := map[string]port{}
	for _, raw := range list(spec["ports"]) {
		encoded, _ := json.Marshal(object(raw))
		var p port
		if json.Unmarshal(encoded, &p) != nil {
			continue
		}
		if p.Protocol == "" {
			p.Protocol = "TCP"
		}
		if p.TargetPort == 0 {
			p.TargetPort = p.Port
		}
		got[p.Name] = p
	}
	for name, p := range req.Module.Spec.Service.Ports {
		target := p.TargetPort
		if target == 0 {
			target = p.Port
		}
		protocol := p.Protocol
		if protocol == "" {
			protocol = "TCP"
		}
		if got[name] != (port{Name: name, Port: float64(p.Port), TargetPort: float64(target), Protocol: protocol}) {
			return violated("every Score Service port must be kept")
		}
	}
	return nil
}
