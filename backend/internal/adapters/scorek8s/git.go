package scorek8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/domain/resource"
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
	text := "score-k8s: render bundle " + e.Stage + " failed (" + e.Code + ")"
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
	Load(ctx context.Context, pin resource.RenderBundle) (renderengine.Bundle, error)
}

// SetGitBundles enables Git render bundles: content comes from the verified
// cache and author templates run in the given worker executor.
func (r *Renderer) SetGitBundles(store BundleStore, executor renderengine.Executor) {
	r.git = store
	r.executor = executor
}

// BinaryDigest is the SHA-256 of the verified score-k8s binary pinned by Git
// bundle records.
func (r *Renderer) BinaryDigest() string { return r.bundle.BinaryDigest }

// CheckBinary re-hashes the configured binary; a changed binary fails closed.
func (r *Renderer) CheckBinary() error {
	digest, err := binaryDigest(r.path)
	if err != nil {
		return err
	}
	if digest != r.bundle.BinaryDigest {
		return fmt.Errorf("%w: score-k8s binary changed", ErrBundleUnavailable)
	}
	return nil
}

// validateGit checks a Git pin against the runtime and the verified cache.
func (r *Renderer) validateGit(ctx context.Context, selected resource.RenderingSelection) (renderengine.Bundle, error) {
	pin := selected.Bundle
	if r.git == nil {
		return renderengine.Bundle{}, fmt.Errorf("%w: Git render bundles are not enabled", ErrBundleUnavailable)
	}
	if selected.DriverType != resource.DriverScoreK8s || selected.DefinitionKey == "" || selected.DefinitionHash == "" ||
		pin.EngineContract != renderbundle.EngineContract || pin.RendererVersion != Version || pin.InputContract != renderbundle.InputContract ||
		pin.Source == nil || pin.Source.ContentSHA256 != pin.Digest {
		return renderengine.Bundle{}, fmt.Errorf("%w: the pinned engine contract is not supported", ErrBundleUnavailable)
	}
	if pin.BinaryDigest != r.bundle.BinaryDigest {
		return renderengine.Bundle{}, fmt.Errorf("%w: the pinned score-k8s binary is not the runtime binary", ErrBundleUnavailable)
	}
	if err := r.CheckBinary(); err != nil {
		return renderengine.Bundle{}, err
	}
	bundle, err := r.git.Load(ctx, pin)
	if err != nil {
		return renderengine.Bundle{}, err
	}
	effective, err := renderbundle.Effective(bundle.Contract.InputsSchema, selected.Inputs)
	if err != nil {
		return renderengine.Bundle{}, fmt.Errorf("%w: the pinned render inputs no longer validate", ErrBundleUnavailable)
	}
	hash, err := renderbundle.InputsHash(effective)
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
	// Bindings are authoritative; raw Score variable templates are not input.
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
		"apiVersion": renderbundle.InputContract,
		"workload":   workload,
		"bindings":   bindings,
		"target":     map[string]any{"namespace": req.Namespace},
		"identity": map[string]any{
			"workload":    req.WorkloadID,
			"application": req.Labels["orchestrator.io/application"],
			"environment": req.Labels["orchestrator.io/environment"],
			"deployment":  req.DeploymentID,
			"labels":      labels,
			"selector":    selector,
		},
		"delivery": deliveryInput,
		"services": services,
		"unknowns": unknowns,
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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	delivery, err := kubernetes.PrepareDelivery(ctx, req)
	if err != nil {
		return nil, err
	}
	input, err := neutralInput(req, delivery)
	if err != nil {
		return nil, stageError("input", err)
	}
	inputs, _ := renderbundle.DeepCopy(req.Selection.Inputs).(map[string]any)
	if inputs == nil {
		inputs = map[string]any{}
	}
	defaults, _ := renderbundle.DeepCopy(bundle.Contract.Defaults).(map[string]any)
	manifest := bundle.Contract.Manifest

	// Step 3: normalization returns one Score document.
	output, err := r.executor.Execute(ctx, manifest.Normalize, string(bundle.File(manifest.Normalize)), map[string]any{"Input": input, "Inputs": inputs, "Defaults": defaults})
	if err != nil {
		return nil, stageError("normalize", err)
	}
	normalized, err := normalizedScore(output, req)
	if err != nil {
		return nil, &RenderError{Stage: "normalize", Code: "SCORE_INVALID", File: manifest.Normalize}
	}

	// Step 4: score-k8s with only the listed output provisioners.
	generated, err := r.generate(ctx, normalized, bundle, req)
	if err != nil {
		return nil, err
	}

	// Step 5: ordered set/delete patches.
	objects := make([]map[string]any, len(generated))
	for i, m := range generated {
		objects[i] = m.Object
	}
	for _, name := range manifest.Patches {
		current := make([]any, len(objects))
		for i, object := range objects {
			current[i] = renderbundle.DeepCopy(object)
		}
		output, err := r.executor.Execute(ctx, name, string(bundle.File(name)), map[string]any{
			"Input": input, "Inputs": inputs, "Defaults": defaults, "Normalized": renderbundle.DeepCopy(normalized), "Manifests": current,
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
	return append(append([]execution.Manifest{}, delivery.Auxiliaries...), result...), nil
}

// normalizedScore parses the normalization output: exactly one JSON object
// holding a supported Score document for this workload.
func normalizedScore(output string, req execution.RenderRequest) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(output))
	var doc map[string]any
	if err := decoder.Decode(&doc); err != nil || doc == nil {
		return nil, errors.New("not a JSON object")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing output")
	}
	for key := range doc {
		switch key {
		case "apiVersion", "metadata", "containers", "service", "resources":
		default:
			return nil, errors.New("unsupported Score field")
		}
	}
	meta, _ := doc["metadata"].(map[string]any)
	if doc["apiVersion"] != "score.dev/v1b1" || meta == nil || meta["name"] != req.WorkloadID {
		return nil, errors.New("Score identity")
	}
	containers, _ := doc["containers"].(map[string]any)
	if len(containers) != len(req.Module.Spec.Containers) {
		return nil, errors.New("containers")
	}
	for name := range req.Module.Spec.Containers {
		if _, ok := containers[name].(map[string]any); !ok {
			return nil, errors.New("containers")
		}
	}
	return doc, nil
}

func (r *Renderer) generate(ctx context.Context, score map[string]any, bundle renderengine.Bundle, req execution.RenderRequest) ([]execution.Manifest, error) {
	workspace, err := os.MkdirTemp("", "orchestrator-score-k8s-git-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workspace)
	run := func(args ...string) error {
		cmd := commandContext(ctx, r.path, args...)
		cmd.Dir = workspace
		// No host tokens, kubeconfig, Terraform or Git credentials inherited.
		cmd.Env = []string{"PATH=" + filepath.Dir(r.path), "HOME=" + workspace, "TMPDIR=" + workspace, "LANG=C"}
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return &RenderError{Stage: "generate", Code: "SCORE_K8S_FAILED"}
		}
		return nil
	}
	if err := run("init", "--quiet", "--no-sample", "--no-default-provisioners"); err != nil {
		return nil, err
	}
	// score-k8s 0.15.0 loads *.provisioners.yaml of its state directory in
	// file-name order and the first matching provisioner wins; numbered names
	// make the bundle.yaml array order the priority order.
	for i, name := range bundle.Contract.Manifest.Provisioners {
		target := filepath.Join(workspace, ".score-k8s", fmt.Sprintf("%03d-bundle.provisioners.yaml", i))
		if err := os.WriteFile(target, bundle.File(name), 0o600); err != nil {
			return nil, err
		}
	}
	scoreBytes, err := json.Marshal(score)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(workspace, "score.json"), scoreBytes, 0o600); err != nil {
		return nil, err
	}
	if err := run("generate", "--quiet", "score.json", "--namespace", req.Namespace, "--output", "manifests.yaml"); err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Join(workspace, "manifests.yaml"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(io.LimitReader(file, renderengine.MaxWorkerOutput))
	generated := []execution.Manifest{}
	for {
		var object map[string]any
		err := decoder.Decode(&object)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, &RenderError{Stage: "generate", Code: "MANIFEST_INVALID"}
		}
		if len(object) == 0 {
			continue
		}
		tree, err := toTree(object)
		if err != nil {
			return nil, &RenderError{Stage: "generate", Code: "MANIFEST_INVALID"}
		}
		kind, _ := tree["kind"].(string)
		api, _ := tree["apiVersion"].(string)
		meta, _ := tree["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		namespace, _ := meta["namespace"].(string)
		// The empty status of generated objects is not policy and never applied.
		delete(tree, "status")
		generated = append(generated, execution.Manifest{APIVersion: api, Kind: kind, Name: name, Namespace: namespace, Object: tree})
	}
	return generated, nil
}
