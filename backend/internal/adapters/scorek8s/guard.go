package scorek8s

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

// guardError is one violated Git render invariant (contract RB-C05). The
// reason is for tests and internal diagnosis; delivery never shows it.
type guardError struct{ reason string }

func (e *guardError) Error() string { return "score-k8s: render invariant violated: " + e.reason }

func violated(format string, args ...any) error {
	return &guardError{reason: fmt.Sprintf(format, args...)}
}

// Allowed fields. Anything else (volumes, initContainers, host namespaces,
// envFrom, volume mounts, lifecycle hooks, other kinds) is rejected.
var (
	allowedMetadata   = set("name", "namespace", "labels", "annotations")
	allowedTemplateMD = set("labels", "annotations")
	allowedDeployment = set("replicas", "selector", "template", "strategy", "minReadySeconds", "revisionHistoryLimit", "progressDeadlineSeconds")
	allowedPod        = set("containers", "imagePullSecrets", "serviceAccountName", "nodeSelector", "affinity", "tolerations", "securityContext",
		"topologySpreadConstraints", "terminationGracePeriodSeconds", "priorityClassName", "restartPolicy", "dnsPolicy", "enableServiceLinks")
	allowedContainer = set("name", "image", "imagePullPolicy", "command", "args", "env", "ports", "resources", "readinessProbe", "livenessProbe",
		"startupProbe", "securityContext", "workingDir", "terminationMessagePath", "terminationMessagePolicy")
	allowedService = set("type", "selector", "ports", "sessionAffinity")
)

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

// guard validates Git-rendered objects against the platform-supplied intent:
// object envelope and identity, explicit Score intent, approved bindings and
// references. It does not compare with a native policy render.
func guard(req execution.RenderRequest, delivery kubernetes.Delivery, objects []execution.Manifest) error {
	var deployment, service map[string]any
	for _, m := range objects {
		switch {
		case m.Kind == "Deployment" && m.APIVersion == "apps/v1" && deployment == nil:
			deployment = m.Object
		case m.Kind == "Service" && m.APIVersion == "v1" && service == nil:
			service = m.Object
		default:
			return violated("only one apps/v1 Deployment and one v1 Service are allowed")
		}
		if m.Name != req.WorkloadID || m.Namespace != req.Namespace {
			return violated("object name and namespace must be the workload identity")
		}
	}
	if deployment == nil {
		return violated("the Deployment is missing")
	}
	wantService := req.Module.Spec.Service != nil && len(req.Module.Spec.Service.Ports) > 0
	if (service != nil) != wantService {
		return violated("a Service is required exactly when the Score declares service ports")
	}
	if err := guardDeployment(req, delivery, deployment); err != nil {
		return err
	}
	if service != nil {
		return guardService(req, delivery, service)
	}
	return nil
}

func fields(object map[string]any, allowed map[string]bool, what string) error {
	for key := range object {
		if !allowed[key] {
			return violated("%s field %s is not allowed", what, key)
		}
	}
	return nil
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func list(v any) []any            { l, _ := v.([]any); return l }

func guardMetadata(meta map[string]any, allowed map[string]bool, delivery kubernetes.Delivery, annotations map[string]string, what string) error {
	if meta == nil {
		return violated("%s metadata is missing", what)
	}
	if err := fields(meta, allowed, what+" metadata"); err != nil {
		return err
	}
	labels := object(meta["labels"])
	if _, present := meta["labels"]; present && labels == nil {
		return violated("%s labels must be an object", what)
	}
	for key, value := range delivery.Labels {
		if labels[key] != value {
			return violated("%s must carry the platform identity labels", what)
		}
	}
	for key, value := range labels {
		if _, text := value.(string); !text {
			return violated("%s label values must be strings", what)
		}
		if reservedLabel(key) {
			if _, platform := delivery.Labels[key]; !platform {
				return violated("%s uses a reserved label", what)
			}
		}
	}
	present := object(meta["annotations"])
	if _, ok := meta["annotations"]; ok && present == nil {
		return violated("%s annotations must be an object", what)
	}
	for key, value := range present {
		text, isText := value.(string)
		if !isText {
			return violated("%s annotation values must be strings", what)
		}
		if reservedAnnotation(key) && annotations[key] != text {
			return violated("%s uses a reserved annotation", what)
		}
	}
	for key, value := range annotations {
		if present[key] != value {
			return violated("%s must carry the platform delivery annotations", what)
		}
	}
	return nil
}

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

func guardDeployment(req execution.RenderRequest, delivery kubernetes.Delivery, d map[string]any) error {
	if err := fields(d, set("apiVersion", "kind", "metadata", "spec"), "Deployment"); err != nil {
		return err
	}
	if err := guardMetadata(object(d["metadata"]), allowedMetadata, delivery, nil, "Deployment"); err != nil {
		return err
	}
	spec := object(d["spec"])
	if err := fields(spec, allowedDeployment, "Deployment spec"); err != nil {
		return err
	}
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
	if err := fields(selector, set("matchLabels"), "Deployment selector"); err != nil {
		return err
	}
	if !sameStringMap(selector["matchLabels"], delivery.Selector) {
		return violated("the selector must be the platform selector")
	}
	template := object(spec["template"])
	if err := fields(template, set("metadata", "spec"), "pod template"); err != nil {
		return err
	}
	if err := guardMetadata(object(template["metadata"]), allowedTemplateMD, delivery, delivery.PodAnnotations, "pod template"); err != nil {
		return err
	}
	pod := object(template["spec"])
	if err := fields(pod, allowedPod, "pod spec"); err != nil {
		return err
	}
	if delivery.ImagePullSecret != "" {
		refs := list(pod["imagePullSecrets"])
		if len(refs) != 1 || !sameStringMap(refs[0], map[string]string{"name": delivery.ImagePullSecret}) {
			return violated("image pull Secrets must be the platform reference")
		}
	} else if _, present := pod["imagePullSecrets"]; present {
		return violated("image pull Secrets are platform-owned")
	}
	if account, present := pod["serviceAccountName"]; present || delivery.ServiceAccount != "" {
		if account != delivery.ServiceAccount {
			return violated("the ServiceAccount is platform-owned")
		}
	}
	if err := guardPodSecurity(pod); err != nil {
		return err
	}
	containers := list(pod["containers"])
	if len(containers) != len(req.Module.Spec.Containers) {
		return violated("containers must be exactly the Score containers")
	}
	targets := map[float64]bool{}
	if req.Module.Spec.Service != nil {
		for _, port := range req.Module.Spec.Service.Ports {
			target := port.TargetPort
			if target == 0 {
				target = port.Port
			}
			targets[float64(target)] = true
		}
	}
	seen := map[string]bool{}
	for _, raw := range containers {
		c := object(raw)
		name, _ := c["name"].(string)
		declared, ok := req.Module.Spec.Containers[name]
		if !ok || seen[name] {
			return violated("containers must be exactly the Score containers")
		}
		seen[name] = true
		if err := guardContainer(c, declared, delivery.Env[name], targets); err != nil {
			return err
		}
	}
	return nil
}

func guardPodSecurity(pod map[string]any) error {
	for _, key := range []string{"hostNetwork", "hostPID", "hostIPC"} {
		if _, present := pod[key]; present {
			return violated("host namespaces are not allowed")
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

func guardContainer(c map[string]any, declared environment.Container, bindings []kubernetes.EnvBinding, targets map[float64]bool) error {
	if err := fields(c, allowedContainer, "container"); err != nil {
		return err
	}
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
	if security := object(c["securityContext"]); security != nil {
		if security["privileged"] == true {
			return violated("privileged containers are not allowed")
		}
	}
	for _, raw := range list(c["ports"]) {
		port := object(raw)
		if !targets[toNumber(port["containerPort"])] {
			return violated("container ports must be Score service target ports")
		}
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
// the bundle default; explicit limits are exact and no limit is invented.
func guardResources(actual map[string]any, declared *environment.ContainerResourceRequirements) error {
	if err := fields(actual, set("requests", "limits"), "container resources"); err != nil {
		return err
	}
	var requests, limits environment.ComputeResources
	if declared != nil && declared.Requests != nil {
		requests = *declared.Requests
	}
	if declared != nil && declared.Limits != nil {
		limits = *declared.Limits
	}
	gotRequests, gotLimits := object(actual["requests"]), object(actual["limits"])
	if err := fields(gotRequests, set("cpu", "memory"), "resource requests"); err != nil {
		return err
	}
	if err := fields(gotLimits, set("cpu", "memory"), "resource limits"); err != nil {
		return err
	}
	for _, field := range []struct{ name, request, limit string }{{"cpu", requests.CPU, limits.CPU}, {"memory", requests.Memory, limits.Memory}} {
		want := field.request
		if want == "" {
			want = field.limit
		}
		if got, present := gotRequests[field.name]; want != "" && (!present || got != want) {
			return violated("explicit Score %s requests or same-field limits must be kept", field.name)
		} else if present {
			if _, text := got.(string); !text {
				return violated("resource quantities must be strings")
			}
		}
		got, present := gotLimits[field.name]
		if field.limit != "" && got != field.limit {
			return violated("explicit Score %s limits must be kept", field.name)
		}
		if field.limit == "" && present {
			return violated("no %s limit may be invented", field.name)
		}
	}
	return nil
}

// guardEnv requires exactly the approved bindings (any order). score-k8s
// omits the value of an empty plain variable; that is the same binding.
func guardEnv(env []any, bindings []kubernetes.EnvBinding) error {
	want := map[string]kubernetes.EnvBinding{}
	for _, b := range bindings {
		want[b.Name] = b
	}
	if len(env) != len(want) {
		return violated("environment bindings must be exactly the approved bindings")
	}
	seen := map[string]bool{}
	for _, raw := range env {
		entry := object(raw)
		name, _ := entry["name"].(string)
		b, ok := want[name]
		if !ok || seen[name] {
			return violated("environment bindings must be exactly the approved bindings")
		}
		seen[name] = true
		for key := range entry {
			if key != "name" && key != "value" && key != "valueFrom" {
				return violated("environment entries may only name a value or Secret reference")
			}
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
	if err := fields(s, set("apiVersion", "kind", "metadata", "spec"), "Service"); err != nil {
		return err
	}
	if err := guardMetadata(object(s["metadata"]), allowedMetadata, delivery, nil, "Service"); err != nil {
		return err
	}
	spec := object(s["spec"])
	if err := fields(spec, allowedService, "Service spec"); err != nil {
		return err
	}
	if typ, present := spec["type"]; present && typ != "ClusterIP" {
		return violated("only ClusterIP Services are allowed; PublicRouteManager owns exposure")
	}
	if !sameStringMap(spec["selector"], delivery.Selector) {
		return violated("the Service selector must be the platform selector")
	}
	type port struct {
		Name       string  `json:"name"`
		Port       float64 `json:"port"`
		TargetPort float64 `json:"targetPort"`
		Protocol   string  `json:"protocol"`
	}
	want := []port{}
	for name, p := range req.Module.Spec.Service.Ports {
		target := p.TargetPort
		if target == 0 {
			target = p.Port
		}
		protocol := p.Protocol
		if protocol == "" {
			protocol = "TCP"
		}
		want = append(want, port{Name: name, Port: float64(p.Port), TargetPort: float64(target), Protocol: protocol})
	}
	got := []port{}
	for _, raw := range list(spec["ports"]) {
		entry := object(raw)
		for key := range entry {
			if key != "name" && key != "port" && key != "targetPort" && key != "protocol" {
				return violated("Service ports may only name name, port, targetPort and protocol")
			}
		}
		encoded, _ := json.Marshal(entry)
		var p port
		if json.Unmarshal(encoded, &p) != nil {
			return violated("Service ports must be the Score ports")
		}
		if p.Protocol == "" {
			p.Protocol = "TCP"
		}
		if p.TargetPort == 0 {
			p.TargetPort = p.Port
		}
		got = append(got, p)
	}
	sort.Slice(want, func(i, j int) bool { return want[i].Name < want[j].Name })
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	a, _ := json.Marshal(want)
	b, _ := json.Marshal(got)
	if string(a) != string(b) {
		return violated("Service ports must be the Score ports")
	}
	return nil
}
