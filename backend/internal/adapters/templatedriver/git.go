package templatedriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/renderregistry"
	"orchestrator/internal/renderengine"
)

// Git render failures (ADR-021) use the registry port classes so delivery
// maps them to stable codes; messages carry stage, code and file names only.
var (
	ErrBundleUnavailable = renderregistry.ErrBundleUnavailable
	ErrRenderFailed      = renderregistry.ErrRenderFailed
)

// RenderError is an ErrRenderFailed at one safe stage.
type RenderError struct {
	Stage string
	Code  string
	File  string
	Line  int
}

func (e *RenderError) Error() string {
	text := "template: render bundle " + e.Stage + " failed (" + e.Code + ")"
	if e.File != "" {
		text += " in " + e.File
		if e.Line > 0 {
			text += fmt.Sprintf(" line %d", e.Line)
		}
	}
	return text
}
func (e *RenderError) Is(target error) bool { return target == ErrRenderFailed }

func stageError(stage string, err error) error {
	var tmpl *renderengine.TemplateError
	if errors.As(err, &tmpl) {
		return &RenderError{Stage: stage, Code: tmpl.Code, File: tmpl.File, Line: tmpl.Line}
	}
	var patch *renderengine.PatchError
	if errors.As(err, &patch) {
		return &RenderError{Stage: stage, Code: "PATCH_INVALID", File: patch.File, Line: patch.Operation}
	}
	var guard *guardError
	if errors.As(err, &guard) {
		return &RenderError{Stage: stage, Code: "INVARIANT_VIOLATED"}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return &RenderError{Stage: stage, Code: "STAGE_FAILED"}
}

// BundleStore loads the verified cached content of one Git bundle pin. It
// never fetches Git: a missing or drifted cache is ErrBundleUnavailable.
type BundleStore interface {
	Load(ctx context.Context, pin resource.TemplateSource) (renderengine.Bundle, error)
}

// SetGitBundles enables Git render bundles: content comes from the verified
// cache and author templates run in the given worker executor.
func (r *Renderer) SetGitBundles(store BundleStore, executor renderengine.Executor) {
	r.git = store
	r.executor = executor
}

// validateGit checks a Git pin against the runtime and the verified cache.
func (r *Renderer) validateGit(ctx context.Context, selected resource.RenderingSelection) (renderengine.Bundle, error) {
	pin := selected.Bundle
	if r.git == nil {
		return renderengine.Bundle{}, fmt.Errorf("%w: Git render bundles are not enabled", ErrBundleUnavailable)
	}
	if selected.DriverType != resource.DriverTemplate || selected.DefinitionKey == "" || selected.DefinitionHash == "" ||
		pin.EngineContract != templatesource.EngineContract || pin.RendererVersion != Version || pin.InputContract != templatesource.InputContract ||
		pin.Source == nil || pin.Source.ContentSHA256 != pin.Digest {
		return renderengine.Bundle{}, fmt.Errorf("%w: the pinned engine contract is not supported", ErrBundleUnavailable)
	}
	if pin.BinaryDigest != r.BinaryDigest() {
		return renderengine.Bundle{}, fmt.Errorf("%w: the pinned template binary is not the runtime binary", ErrBundleUnavailable)
	}
	if err := r.CheckBinary(); err != nil {
		return renderengine.Bundle{}, err
	}
	bundle, err := r.git.Load(ctx, pin)
	if err != nil {
		return renderengine.Bundle{}, err
	}
	effective, err := templatesource.Effective(bundle.Contract.InputsSchema, selected.Inputs)
	if err != nil {
		return renderengine.Bundle{}, fmt.Errorf("%w: the pinned render inputs no longer validate", ErrBundleUnavailable)
	}
	hash, err := templatesource.InputsHash(effective)
	if err != nil || hash != selected.InputsHash {
		return renderengine.Bundle{}, fmt.Errorf("%w: the pinned render inputs do not match their hash", ErrBundleUnavailable)
	}
	return bundle, nil
}

// Input v1 (RB-C04 step 2): JSON only, no credentials, clients or methods.
func neutralInput(req execution.RenderRequest, delivery kubernetes.Delivery) (map[string]any, error) {
	workload, err := toTree(req.Module)
	if err != nil {
		return nil, err
	}
	// Bindings are authoritative; raw Workload variable templates are not input.
	if spec, ok := workload["spec"].(map[string]any); ok {
		if containers, ok := spec["containers"].(map[string]any); ok {
			for _, raw := range containers {
				if container, ok := raw.(map[string]any); ok {
					delete(container, "variables")
				}
			}
		}
	}
	bindings := map[string]any{}
	for container, entries := range delivery.Env {
		list := []any{}
		for _, b := range entries {
			if b.Secret {
				list = append(list, map[string]any{"name": b.Name, "kind": "secret", "secretName": b.SecretRef, "secretKey": b.SecretKey})
				continue
			}
			list = append(list, map[string]any{"name": b.Name, "kind": "plain", "value": b.Value})
		}
		bindings[container] = list
	}
	labels := map[string]any{}
	for k, v := range delivery.Labels {
		labels[k] = v
	}
	selector := map[string]any{}
	for k, v := range delivery.Selector {
		selector[k] = v
	}
	deliveryInput := map[string]any{}
	if delivery.ImagePullSecret != "" {
		deliveryInput["imagePullSecrets"] = []any{map[string]any{"name": delivery.ImagePullSecret}}
	}
	if delivery.ServiceAccount != "" {
		deliveryInput["serviceAccountName"] = delivery.ServiceAccount
	}
	if len(delivery.PodAnnotations) > 0 {
		annotations := map[string]any{}
		for k, v := range delivery.PodAnnotations {
			annotations[k] = v
		}
		deliveryInput["podAnnotations"] = annotations
	}
	services := map[string]any{}
	for workload, ports := range req.Services {
		entry := map[string]any{}
		for name, port := range ports {
			entry[name] = port
		}
		services[workload] = entry
	}
	unknowns := []any{}
	paths := append([]string{}, req.UnknownPaths...)
	sort.Strings(paths)
	for _, path := range paths {
		unknowns = append(unknowns, map[string]any{"path": path, "provenance": "unresolved"})
	}
	input := map[string]any{
		"apiVersion":  templatesource.InputContract,
		"workload":    workload,
		"bindings":    bindings,
		"storage":     req.Storage,
		"secretFiles": delivery.Files,
		"configMaps":  configMaps(req),
		"probes":      probeIntents(req),
		"target":      map[string]any{"namespace": req.Namespace},
		"identity": map[string]any{
			"workload":    req.WorkloadID,
			"application": req.Labels["orchestrator.io/application"],
			"environment": req.Labels["orchestrator.io/environment"],
			"deployment":  req.DeploymentID,
			"labels":      labels,
			"selector":    selector,
		},
		"delivery":  deliveryInput,
		"services":  services,
		"unknowns":  unknowns,
		"resources": req.ResourceReferences,
	}
	// One JSON round trip: the templates see only JSON values.
	out, err := toTree(input)
	return out, err
}

func toTree(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// renderGit runs the fixed Git pipeline (RB-C04 steps 1-6). It never calls the
// native Deployment/Service renderer.
func (r *Renderer) renderGit(ctx context.Context, req execution.RenderRequest) ([]execution.Manifest, error) {
	bundle, err := r.validateGit(ctx, req.Selection)
	if err != nil {
		return nil, err
	}
	if e := req.Module.Spec.Execution; req.Vault != nil && e != nil && e.AutomountServiceAccountToken != nil && !*e.AutomountServiceAccountToken {
		return nil, environment.ErrExecution
	}
	var profile *environment.RuntimeDefaults
	if e := req.Module.Spec.Execution; e.HasTemplate() {
		for _, t := range bundle.Contract.Manifest.WorkloadTemplates {
			if t.ID == e.Template.ID && t.Version == e.Template.Version && t.SHA256() == e.Template.SHA256 {
				profile = t.RuntimeDefaults
			}
		}
	}
	req.Module.Spec, _ = req.Module.Spec.ResolveDefaults(profile)
	if err := req.Module.Spec.ValidateComputePolicy(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := templatesource.ValidateWorkloadCapabilities(bundle.Contract.Manifest, req.Module.Spec.Containers, req.Module.Spec.Execution); err != nil {
		return nil, err
	}
	if err := environment.ValidateContainerRoles(req.Module.Spec.Containers); err != nil {
		return nil, err
	}
	for _, c := range req.Module.Spec.Containers {
		for _, p := range []struct {
			probe  *environment.Probe
			single bool
		}{{c.ReadinessProbe, false}, {c.LivenessProbe, true}, {c.StartupProbe, true}} {
			if err := p.probe.Validate(p.single); err != nil {
				return nil, err
			}
		}
	}
	storageKeys := map[string]bool{}
	for key := range req.Storage {
		storageKeys[key] = true
	}
	if err := req.Module.Spec.Execution.Validate(req.Module.Spec.Containers, req.Module.Spec.ReplicaCount(), req.Module.Spec.Service != nil && len(req.Module.Spec.Service.Ports) > 0, storageKeys); err != nil {
		return nil, err
	}
	delivery, err := kubernetes.PrepareDelivery(ctx, req)
	if err != nil {
		return nil, err
	}
	if e := req.Module.Spec.Execution; e != nil {
		inputs, err := templatesource.ExecutionInputs(bundle.Contract.Manifest, e)
		if err != nil {
			return nil, environment.ErrExecutionUnavailable
		}
		req.Module.Spec.Execution = e.Clone()
		req.Module.Spec.Execution.Inputs = inputs
	}
	if e := req.Module.Spec.Execution; e != nil {
		if e.ServiceAccountName != "" && e.ServiceAccountName != delivery.ServiceAccount {
			return nil, environment.ErrExecution
		}
		// Agent-based Vault auth needs the Pod token. VSO file delivery does not.
		if req.Vault != nil {
			if e.AutomountServiceAccountToken != nil && !*e.AutomountServiceAccountToken && e.Defaults == nil {
				return nil, environment.ErrExecution
			}
			// Desired explicit false is checked before inherited policy is resolved.
			token := true
			e.AutomountServiceAccountToken = &token
		}
	}
	input, err := neutralInput(req, delivery)
	if err != nil {
		return nil, stageError("input", err)
	}
	inputs, _ := templatesource.DeepCopy(req.Selection.Inputs).(map[string]any)
	if inputs == nil {
		inputs = map[string]any{}
	}
	defaults, _ := templatesource.DeepCopy(bundle.Contract.Defaults).(map[string]any)
	manifest := bundle.Contract.Manifest

	// Optional normalization supplies template defaults, never an executable generator.
	normalized := map[string]any{}
	if manifest.Normalize != "" {
		output, err := r.executor.Execute(ctx, manifest.Normalize, string(bundle.File(manifest.Normalize)), map[string]any{"Input": input, "Inputs": inputs, "Defaults": defaults})
		if err != nil {
			return nil, stageError("normalize", err)
		}
		if json.Unmarshal([]byte(output), &normalized) != nil || normalized == nil {
			return nil, &RenderError{Stage: "normalize", Code: "WORKLOAD_INVALID", File: manifest.Normalize}
		}
	}
	objects := []map[string]any{}
	// Additional objects (ADR-024), appended in manifest order so patches see them.
	for _, name := range manifest.Resources {
		current := make([]any, len(objects))
		for i, object := range objects {
			current[i] = templatesource.DeepCopy(object)
		}
		output, err := r.executor.Execute(ctx, name, string(bundle.File(name)), map[string]any{
			"Resources": input["resources"], "Input": input, "Workload": object(input["workload"])["spec"], "Inputs": inputs, "Defaults": defaults, "Normalized": templatesource.DeepCopy(normalized), "Manifests": current,
		})
		if err != nil {
			return nil, stageError("resources", err)
		}
		added, err := renderengine.ParseResourceOutput(name, output)
		if err != nil {
			return nil, stageError("resources", err)
		}
		objects = append(objects, added...)
		if len(objects) > renderengine.MaxObjects {
			return nil, &RenderError{Stage: "resources", Code: "TOO_MANY_OBJECTS", File: name}
		}
	}

	// Step 5: ordered set/delete patches.
	for _, name := range manifest.Patches {
		current := make([]any, len(objects))
		for i, object := range objects {
			current[i] = templatesource.DeepCopy(object)
		}
		output, err := r.executor.Execute(ctx, name, string(bundle.File(name)), map[string]any{
			"Resources": input["resources"], "Input": input, "Workload": object(input["workload"])["spec"], "Inputs": inputs, "Defaults": defaults, "Normalized": templatesource.DeepCopy(normalized), "Manifests": current,
		})
		if err != nil {
			return nil, stageError("patch", err)
		}
		ops, err := renderengine.ParsePatchOutput(name, output)
		if err != nil {
			return nil, stageError("patch", err)
		}
		if objects, err = renderengine.ApplyPatches(name, objects, ops); err != nil {
			return nil, stageError("patch", err)
		}
	}
	result := make([]execution.Manifest, 0, len(objects))
	for _, object := range objects {
		kind, _ := object["kind"].(string)
		api, _ := object["apiVersion"].(string)
		meta, _ := object["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		namespace, _ := meta["namespace"].(string)
		result = append(result, execution.Manifest{APIVersion: api, Kind: kind, Name: name, Namespace: namespace, Object: object})
	}
	// Step 6: semantic invariants instead of a hidden native policy render.
	if err := guard(req, delivery, result); err != nil {
		return nil, stageError("validate", err)
	}
	for i := range result {
		own(&result[i], req)
	}
	auxiliaries := make([]execution.Manifest, len(delivery.Auxiliaries))
	for i, m := range delivery.Auxiliaries {
		m.Auxiliary = true
		auxiliaries[i] = m
	}
	return append(auxiliaries, result...), nil
}

// own sets the namespace and the platform ownership labels of one rendered
// object (contract RB-C08); the guard already refused conflicting values.
func own(m *execution.Manifest, req execution.RenderRequest) {
	meta, _ := m.Object["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		m.Object["metadata"] = meta
	}
	meta["namespace"] = req.Namespace
	m.Namespace = req.Namespace
	labels, _ := meta["labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
		meta["labels"] = labels
	}
	labels["app.kubernetes.io/managed-by"] = "orchestrator"
	labels["orchestrator.io/workload"] = req.WorkloadID
	for _, key := range []string{"orchestrator.io/application", "orchestrator.io/environment", "orchestrator.io/deployment-id"} {
		if value := req.Labels[key]; value != "" {
			labels[key] = value
		}
	}
}
