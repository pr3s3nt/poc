// Package deploymentreview builds bounded, sanitized, non-executable review
// data. It has no executor, provider, credential or mutable catalog port.
package deploymentreview

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"gopkg.in/yaml.v3"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/ports/execution"
)

const MaxArtifacts = 128
const MaxSideBytes = 256 << 10
const MaxTotalBytes = 2 << 20

var namespaceLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

type Side struct {
	Availability  string            `json:"availability"`
	Text          string            `json:"text,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	Provenance    map[string]string `json:"provenance,omitempty"`
	RedactedPaths []string          `json:"redactedPaths"`
	UnknownPaths  []string          `json:"unknownPaths"`
}
type Artifact struct {
	ID       string `json:"id"`
	NodeID   string `json:"nodeId"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Language string `json:"language"`
	Before   Side   `json:"before"`
	After    Side   `json:"after"`
}
type Node struct {
	Properties        []Property             `json:"properties,omitempty"`
	ID                string                 `json:"id"`
	Kind              string                 `json:"kind"`
	Name              string                 `json:"name"`
	ResourceType      string                 `json:"resourceType"`
	Class             string                 `json:"class"`
	DefinitionKey     string                 `json:"definitionKey,omitempty"`
	DefinitionVersion int64                  `json:"definitionVersion,omitempty"`
	Account           string                 `json:"account,omitempty"`
	Region            string                 `json:"region,omitempty"`
	Scope             string                 `json:"scope,omitempty"`
	Ownership         string                 `json:"ownership,omitempty"`
	Intent            string                 `json:"intent,omitempty"`
	Module            *planning.IntentModule `json:"module,omitempty"`
	Placement         *planning.Placement    `json:"placement,omitempty"`
	Renderer          string                 `json:"renderer,omitempty"`
	RendererDigest    string                 `json:"rendererDigest,omitempty"`
	// RenderSource is the Git render bundle pin of a workload (ADR-021).
	RenderSource *RenderSource `json:"renderSource,omitempty"`
}

// RenderSource is the safe provenance of a Git render bundle pin.
type RenderSource struct {
	URL        string `json:"url"`
	Path       string `json:"path"`
	Revision   string `json:"revision"`
	Version    string `json:"version"`
	InputsHash string `json:"inputsHash,omitempty"`
}

func renderSource(selection resource.RenderingSelection) *RenderSource {
	if !selection.Bundle.Git() {
		return nil
	}
	src := selection.Bundle.Source
	return &RenderSource{URL: src.URL, Path: src.Path, Revision: src.Revision, Version: selection.Bundle.Version, InputsHash: selection.InputsHash}
}

// renderProvenance is the manifest artifact provenance of a selection.
func renderProvenance(planHash string, selection resource.RenderingSelection) map[string]string {
	out := map[string]string{"planHash": planHash, "rendererBundle": selection.Bundle.ID, "rendererDigest": selection.Bundle.Digest}
	if src := renderSource(selection); src != nil {
		out["renderSourceUrl"], out["renderSourcePath"], out["renderRevision"], out["renderVersion"] = src.URL, src.Path, src.Revision, src.Version
	}
	return out
}

type Review struct {
	Version   int             `json:"version"`
	PlanHash  string          `json:"planHash"`
	Nodes     []Node          `json:"nodes"`
	Edges     []planning.Edge `json:"edges"`
	Batches   [][]string      `json:"batches"`
	Artifacts []Artifact      `json:"artifacts"`
}
type SourceReader interface {
	ReadPinned(context.Context, tfmodule.SourceRef) (map[string][]byte, error)
}
type Renderer interface {
	Render(context.Context, execution.RenderRequest) ([]execution.Manifest, error)
}
type Assembler struct {
	Sources  SourceReader
	Renderer Renderer
}

// Assemble keeps the planner result untouched. Callers must finish their read
// snapshot before invoking this method, because the selected renderer runs a
// local subprocess. Previous contains only exact successful safe artifacts.
func (a Assembler) Assemble(ctx context.Context, plan *planning.Plan, intents []planning.ResourceIntent, previous map[string][]Artifact) Review {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	review := Review{Version: 1, PlanHash: plan.PlanHash, Nodes: []Node{}, Edges: append([]planning.Edge{}, plan.Graph.Edges...), Batches: [][]string{}, Artifacts: []Artifact{}}
	for _, b := range plan.Batches {
		review.Batches = append(review.Batches, append([]string{}, b...))
	}
	intentByID := map[string]planning.ResourceIntent{}
	for _, i := range intents {
		intentByID[i.Descriptor] = i
	}
	total := 0
	overflow := false
	appendArtifact := func(art Artifact) {
		if len(review.Artifacts) >= MaxArtifacts {
			overflow = true
			return
		}
		for _, side := range []*Side{&art.Before, &art.After} {
			if len(side.Text) > MaxSideBytes || total+len(side.Text) > MaxTotalBytes {
				*side = unavailable("LIMIT_EXCEEDED")
			} else {
				total += len(side.Text)
			}
		}
		review.Artifacts = append(review.Artifacts, art)
	}
	for _, node := range plan.Graph.Nodes {
		n := Node{ID: node.Descriptor, Kind: string(node.Kind), Name: node.Descriptor, ResourceType: node.ResourceType, Class: node.Class}
		if match, ok := plan.Matches[node.Descriptor]; ok {
			n.DefinitionKey, n.DefinitionVersion = match.DefinitionKey, match.DefinitionVersion
			n.Account = match.DriverAccount
		}
		if place, ok := plan.Placements[node.Descriptor]; ok {
			copy := place
			n.Placement = &copy
		}
		if i, ok := intentByID[node.Descriptor]; ok {
			n.Ownership, n.Intent, n.Module = i.Ownership, i.Intent, i.Module
			n.Scope = i.Scope
			if i.Ownership == planning.OwnerPreexisting && i.Intent == planning.IntentCreate {
				n.Intent = "BIND"
			}
			if n.Account == "" {
				n.Account = i.DriverAccount
			}
			for _, provider := range i.Providers {
				if provider.Region != "" {
					n.Region = provider.Region
				}
				if n.Account == "" {
					n.Account = provider.ConnectionKey
				}
			}
		}
		if node.Kind == planning.NodeWorkload {
			n.Name = node.WorkloadID
			before, existed := plan.BaseSet.Modules[node.WorkloadID]
			if !existed {
				n.Intent = "CREATE"
			} else if reflect.DeepEqual(before, plan.CandidateSet.Modules[node.WorkloadID]) {
				n.Intent = "RETAIN"
			} else {
				n.Intent = "UPDATE"
			}
			if plan.WorkloadID == node.WorkloadID && string(plan.Action) == "UPDATE" {
				n.Intent = "UPDATE"
			}
			if selected, ok := plan.Rendering[node.WorkloadID]; ok {
				n.Renderer = selected.Bundle.ID + "@" + selected.Bundle.Version
				n.RendererDigest = selected.Bundle.Digest
				n.DefinitionKey = selected.DefinitionKey
				n.RenderSource = renderSource(selected)
			}
			for _, art := range a.manifests(ctx, plan, node.Descriptor, node.WorkloadID, previous[node.Descriptor]) {
				appendArtifact(art)
			}
		} else if i, ok := intentByID[node.Descriptor]; ok && i.Module != nil {
			for _, art := range a.sources(ctx, plan.PlanHash, i, previous[node.Descriptor]) {
				appendArtifact(art)
			}
			for _, contract := range plan.Terraform {
				if contract.Descriptor == node.Descriptor {
					n.Properties = a.properties(ctx, node, contract)
					appendArtifact(inputsArtifact(plan.PlanHash, i, contract, previous[node.Descriptor]))
					break
				}
			}
		}
		review.Nodes = append(review.Nodes, n)
	}
	// Removed workloads no longer belong to the desired DAG, but must remain visible for review.
	for id, module := range plan.BaseSet.Modules {
		if _, exists := plan.CandidateSet.Modules[id]; exists {
			continue
		}
		descriptor, err := planning.WorkloadDescriptorFor(id, module.WorkloadClass())
		if err != nil {
			continue
		}
		key := descriptor.String()
		review.Nodes = append(review.Nodes, Node{ID: key, Name: id, Kind: "workload", ResourceType: "workload", Class: module.WorkloadClass(), Intent: "REMOVE"})
		old := previous[key]
		if len(old) == 0 {
			appendArtifact(Artifact{ID: key + ":removed", NodeID: key, Kind: "kubernetes-manifest", Path: "Deployment/" + id, Language: "yaml", Before: unavailable("BASELINE_MISSING"), After: absent()})
		}
		for _, art := range old {
			if art.Kind == "kubernetes-manifest" {
				art.Before = art.After
				art.After = absent()
				appendArtifact(art)
			}
		}
	}
	if overflow {
		nodeID := review.Artifacts[MaxArtifacts-1].NodeID
		review.Artifacts[MaxArtifacts-1] = Artifact{ID: "review:limit", NodeID: nodeID, Kind: "binding", Path: "additional artifacts", Language: "text", Before: unavailable("LIMIT_EXCEEDED"), After: unavailable("LIMIT_EXCEEDED")}
	}
	sort.Slice(review.Nodes, func(i, j int) bool { return review.Nodes[i].ID < review.Nodes[j].ID })
	sort.Slice(review.Artifacts, func(i, j int) bool {
		if review.Artifacts[i].NodeID != review.Artifacts[j].NodeID {
			return review.Artifacts[i].NodeID < review.Artifacts[j].NodeID
		}
		return review.Artifacts[i].Path < review.Artifacts[j].Path
	})
	return review
}

func inputsArtifact(planHash string, intent planning.ResourceIntent, contract planning.TerraformContract, old []Artifact) Artifact {
	items := append([]planning.ContractInput{}, contract.Inputs...)
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	var text strings.Builder
	paths := []string{}
	allowed := map[string]bool{"driver-input": true, "resource-input": true, "terraform-default": true, "executor": true}
	for _, input := range items {
		if !safeLabel(input.Name) {
			continue
		}
		source := input.Source
		if !allowed[source] {
			source = "unknown"
		}
		text.WriteString(input.Name + " (" + input.Type + ") <- " + source + "; value hidden\n")
		paths = append(paths, input.Name)
	}
	side := Side{Availability: "PARTIAL", Text: text.String(), Provenance: map[string]string{"planHash": planHash, "module": contract.Module}, RedactedPaths: paths, UnknownPaths: []string{}}
	if text.Len() > MaxSideBytes {
		side = unavailable("LIMIT_EXCEEDED")
	}
	return Artifact{ID: intent.Descriptor + ":inputs", NodeID: intent.Descriptor, Kind: "terraform-inputs", Path: "inputs", Language: "text", Before: beforeFor(old, "terraform-inputs", "inputs", intent.Intent == planning.IntentCreate), After: side}
}

func unavailable(reason string) Side {
	return Side{Availability: "UNAVAILABLE", Reason: reason, RedactedPaths: []string{}, UnknownPaths: []string{}}
}
func absent() Side {
	return Side{Availability: "ABSENT", RedactedPaths: []string{}, UnknownPaths: []string{}}
}
func beforeFor(old []Artifact, kind, path string, created bool) Side {
	if created {
		return absent()
	}
	for _, a := range old {
		if a.Kind == kind && a.Path == path && (a.After.Availability == "AVAILABLE" || a.After.Availability == "PARTIAL") {
			return a.After
		}
	}
	return unavailable("BASELINE_MISSING")
}

func (a Assembler) sources(ctx context.Context, planHash string, i planning.ResourceIntent, old []Artifact) []Artifact {
	pin := tfmodule.SourceRef{ModuleID: i.Module.Module, SourceID: i.Module.SourceID, Revision: i.Module.Revision, Subdirectory: i.Module.Subdirectory, ContentSHA256: i.Module.ContentSHA256}
	provenance := map[string]string{"planHash": planHash, "module": pin.ModuleID, "sourceId": pin.SourceID, "revision": pin.Revision, "subdirectory": pin.Subdirectory, "contentSha256": pin.ContentSHA256}
	if a.Sources == nil {
		return []Artifact{{ID: i.Descriptor + ":source", NodeID: i.Descriptor, Kind: "terraform-source", Path: "module.tf", Language: "hcl", Before: beforeFor(old, "terraform-source", "module.tf", i.Intent == planning.IntentCreate), After: unavailable("CONTENT_UNAVAILABLE")}}
	}
	files, err := a.Sources.ReadPinned(ctx, pin)
	if err != nil || len(files) == 0 {
		return []Artifact{{ID: i.Descriptor + ":source", NodeID: i.Descriptor, Kind: "terraform-source", Path: "module.tf", Language: "hcl", Before: beforeFor(old, "terraform-source", "module.tf", i.Intent == planning.IntentCreate), After: unavailable("CONTENT_UNAVAILABLE")}}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	out := []Artifact{}
	for _, name := range names {
		after := unavailable("REDACTION_REQUIRED")
		if len(files[name]) <= MaxSideBytes {
			if safe, paths, ok := ProjectHCL(files[name]); ok {
				after = Side{Availability: "PARTIAL", Text: safe, Provenance: provenance, RedactedPaths: paths, UnknownPaths: []string{}}
			}
		} else {
			after = unavailable("LIMIT_EXCEEDED")
		}
		out = append(out, Artifact{ID: i.Descriptor + ":" + name, NodeID: i.Descriptor, Kind: "terraform-source", Path: name, Language: "hcl", Before: beforeFor(old, "terraform-source", name, i.Intent == planning.IntentCreate), After: after})
	}
	// A file present in the successful baseline but no longer in the pinned module is a removal.
	seen := map[string]bool{}
	for _, art := range out {
		seen[art.Path] = true
	}
	for _, art := range old {
		if art.Kind == "terraform-source" && !seen[art.Path] {
			art.Before = art.After
			art.After = absent()
			out = append(out, art)
		}
	}
	return out
}

// ProjectHCL parses configuration first and emits only block/attribute
// structure. Expressions and comments, including nested defaults and output
// expressions, are never copied from source. This is intentionally non-runnable.
func ProjectHCL(input []byte) (string, []string, bool) {
	if len(input) > MaxSideBytes || !utf8Valid(input) {
		return "", nil, false
	}
	file, diags := hclsyntax.ParseConfig(input, "source.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return "", nil, false
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return "", nil, false
	}
	var b strings.Builder
	paths := []string{}
	var walk func(*hclsyntax.Body, string, int)
	walk = func(body *hclsyntax.Body, prefix string, depth int) {
		indent := strings.Repeat("  ", depth)
		keys := make([]string, 0, len(body.Attributes))
		for key := range body.Attributes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteString(indent + key + " = <redacted>\n")
			paths = append(paths, prefix+key)
		}
		for _, block := range body.Blocks {
			b.WriteString(indent + block.Type)
			for _, label := range block.Labels {
				if safeLabel(label) {
					b.WriteString(" " + fmt.Sprintf("%q", label))
				} else {
					b.WriteString(" \"<redacted>\"")
					paths = append(paths, prefix+block.Type+".label")
				}
			}
			b.WriteString(" {\n")
			walk(block.Body, prefix+block.Type+".", depth+1)
			b.WriteString(indent + "}\n")
		}
	}
	walk(body, "", 0)
	return b.String(), paths, true
}

func safeLabel(v string) bool {
	if len(v) > 128 || len(v) == 0 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}
func utf8Valid(b []byte) bool { return utf8.Valid(b) && !strings.ContainsRune(string(b), '\x00') }

// reviewedKind reports objects with the detailed workload/Service projection.
func reviewedKind(kind string) bool {
	return kind == "Deployment" || kind == "StatefulSet" || kind == "Service"
}

// ProjectManifests emits review data of the rendered objects. The execution
// manifests remain untouched. Platform delivery objects and credential-named
// annotations, Secret bodies and container literal environment values are
// excluded; accepted Git policy fields are shown. Other bundle objects
// (ADR-024) show metadata and a sanitized spec.
func ProjectManifests(manifests []execution.Manifest) map[string]Side {
	out := map[string]Side{}
	for _, m := range manifests {
		if m.Auxiliary || m.Secret || m.Kind == "Secret" {
			continue
		}
		if !reviewedKind(m.Kind) {
			encoded, err := yaml.Marshal(sanitizeExtra(m))
			side := Side{Availability: "PARTIAL", Text: string(encoded), RedactedPaths: []string{"metadata.annotations", "**.env[*].value", "data", "binaryData", "stringData"}, UnknownPaths: []string{}}
			if err != nil {
				side = unavailable("REDACTION_REQUIRED")
			} else if len(encoded) > MaxSideBytes {
				side = unavailable("LIMIT_EXCEEDED")
			}
			out[m.Kind+"/"+m.Name] = side
			continue
		}
		obj := sanitizeManifest(m)
		encoded, err := yaml.Marshal(obj)
		if err != nil {
			out[m.Kind+"/"+m.Name] = unavailable("REDACTION_REQUIRED")
			continue
		}
		side := Side{Availability: "PARTIAL", Text: string(encoded), RedactedPaths: []string{"metadata.annotations", "spec.template.metadata.annotations[vault.hashicorp.com/*]", "spec.template.spec.containers[*].env[*].value"}, UnknownPaths: []string{}}
		if len(encoded) > MaxSideBytes {
			side = unavailable("LIMIT_EXCEEDED")
		}
		out[m.Kind+"/"+m.Name] = side
	}
	return out
}

// sanitizeExtra projects a bundle object of any other kind: metadata labels,
// non-platform annotations and the spec through safeTree, with environment
// values and data payloads redacted.
func sanitizeExtra(m execution.Manifest) map[string]any {
	metadata := map[string]any{"name": m.Name, "namespace": m.Namespace}
	if source, ok := m.Object["metadata"].(map[string]any); ok {
		if labels := selectKeyed(source["labels"]); len(labels) > 0 {
			metadata["labels"] = labels
		}
		if annotations := selectAnnotations(source["annotations"]); len(annotations) > 0 {
			metadata["annotations"] = annotations
		}
	}
	result := map[string]any{"apiVersion": m.APIVersion, "kind": m.Kind, "metadata": metadata}
	for _, field := range []string{"data", "binaryData", "stringData"} {
		if values, ok := m.Object[field].(map[string]any); ok {
			keys := map[string]any{}
			for key := range values {
				if safeKey.MatchString(key) || safeLabel(key) {
					keys[key] = "<redacted>"
				}
			}
			result[field] = keys
		}
	}
	if spec := safeTree(redactEnv(m.Object["spec"], 0), 0); spec != nil {
		result["spec"] = spec
	}
	return result
}

// redactEnv copies a tree with every env[].value replaced by "<redacted>".
func redactEnv(v any, depth int) any {
	if depth > 24 {
		return nil
	}
	switch value := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			if key == "env" {
				if entries, ok := item.([]any); ok {
					safe := make([]any, 0, len(entries))
					for _, entry := range entries {
						e, ok := entry.(map[string]any)
						if !ok {
							continue
						}
						copied := map[string]any{}
						for k, v := range e {
							copied[k] = v
						}
						if _, plain := copied["value"]; plain {
							copied["value"] = "<redacted>"
						}
						safe = append(safe, copied)
					}
					out[key] = safe
					continue
				}
			}
			out[key] = redactEnv(item, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = redactEnv(item, depth+1)
		}
		return out
	default:
		return v
	}
}

func sanitizeManifest(m execution.Manifest) map[string]any {
	metadata := map[string]any{"name": m.Name, "namespace": m.Namespace}
	if source, ok := m.Object["metadata"].(map[string]any); ok {
		if labels := selectKeyed(source["labels"]); len(labels) > 0 {
			metadata["labels"] = labels
		}
	}
	result := map[string]any{"apiVersion": m.APIVersion, "kind": m.Kind, "metadata": metadata}
	spec, _ := m.Object["spec"].(map[string]any)
	if m.Kind == "Service" {
		safe := map[string]any{}
		if value, ok := spec["type"].(string); ok {
			safe["type"] = value
		}
		if ports, ok := spec["ports"].([]any); ok {
			safe["ports"] = selectObjectList(ports, "name", "port", "targetPort", "protocol")
		}
		if labels, ok := spec["selector"].(map[string]any); ok {
			safe["selector"] = selectLabels(labels)
		}
		result["spec"] = safe
		return result
	}
	safe := map[string]any{}
	switch replicas := spec["replicas"].(type) {
	case int, int32, int64, float64:
		safe["replicas"] = replicas
	}
	if selector, ok := spec["selector"].(map[string]any); ok {
		selected := map[string]any{}
		if labels, ok := selector["matchLabels"].(map[string]any); ok {
			selected["matchLabels"] = selectLabels(labels)
		}
		safe["selector"] = selected
	}
	// Accepted rollout fields of Git render bundles (ADR-021 RB-C06).
	for _, field := range []string{"strategy"} {
		if value := safeTree(spec[field], 0); value != nil {
			if object, ok := value.(map[string]any); !ok || len(object) > 0 {
				safe[field] = value
			}
		}
	}
	for _, field := range []string{"minReadySeconds", "revisionHistoryLimit", "progressDeadlineSeconds"} {
		switch value := spec[field].(type) {
		case int, int32, int64, float64:
			safe[field] = value
		}
	}
	tpl, _ := spec["template"].(map[string]any)
	pod, _ := tpl["spec"].(map[string]any)
	if source, ok := tpl["metadata"].(map[string]any); ok {
		templateMetadata := map[string]any{}
		if labels := selectKeyed(source["labels"]); len(labels) > 0 {
			templateMetadata["labels"] = labels
		}
		// Platform delivery annotations (Vault Agent) stay redacted.
		if annotations := selectAnnotations(source["annotations"]); len(annotations) > 0 {
			templateMetadata["annotations"] = annotations
		}
		if len(templateMetadata) > 0 {
			safe["template"] = map[string]any{"metadata": templateMetadata}
		}
	}
	podSafe := map[string]any{}
	if refs, ok := pod["imagePullSecrets"].([]any); ok {
		podSafe["imagePullSecrets"] = selectObjectList(refs, "name")
	}
	if account, ok := pod["serviceAccountName"].(string); ok && safeLabel(account) {
		podSafe["serviceAccountName"] = account
	}
	for _, field := range []string{"nodeSelector", "affinity", "tolerations", "securityContext", "topologySpreadConstraints"} {
		if value := safeTree(pod[field], 0); value != nil {
			podSafe[field] = value
		}
	}
	containers, _ := pod["containers"].([]any)
	list := []any{}
	for _, entry := range containers {
		c, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		clean := map[string]any{}
		for _, field := range []string{"name", "image", "imagePullPolicy"} {
			if value, ok := c[field].(string); ok {
				clean[field] = value
			}
		}
		if ports, ok := c["ports"].([]any); ok {
			clean["ports"] = selectObjectList(ports, "name", "containerPort", "protocol")
		}
		if value := safeTree(c["securityContext"], 0); value != nil {
			clean["securityContext"] = value
		}
		if resources, ok := c["resources"].(map[string]any); ok {
			selected := map[string]any{}
			for _, class := range []string{"requests", "limits"} {
				if values, ok := resources[class].(map[string]any); ok {
					selected[class] = selectObject(values, "cpu", "memory")
				}
			}
			clean["resources"] = selected
		}
		for _, field := range []string{"readinessProbe", "livenessProbe", "startupProbe"} {
			if probe, ok := c[field].(map[string]any); ok {
				selected := selectObject(probe, "initialDelaySeconds", "periodSeconds", "timeoutSeconds", "failureThreshold", "successThreshold")
				if get, ok := probe["httpGet"].(map[string]any); ok {
					selected["httpGet"] = selectObject(get, "path", "port", "scheme")
				}
				clean[field] = selected
			}
		}
		if env, ok := c["env"].([]any); ok {
			safeEnv := []any{}
			for _, e := range env {
				v, ok := e.(map[string]any)
				if !ok {
					continue
				}
				name, valid := v["name"].(string)
				if !valid || !safeLabel(name) {
					continue
				}
				item := map[string]any{"name": name}
				if ref, ok := v["valueFrom"].(map[string]any); ok {
					if secret, ok := ref["secretKeyRef"].(map[string]any); ok {
						item["valueFrom"] = map[string]any{"secretKeyRef": selectObject(secret, "name", "key", "optional")}
					}
				} else {
					item["value"] = "<redacted>"
				}
				safeEnv = append(safeEnv, item)
			}
			clean["env"] = safeEnv
		}
		list = append(list, clean)
	}
	podSafe["containers"] = list
	template, _ := safe["template"].(map[string]any)
	if template == nil {
		template = map[string]any{}
	}
	template["spec"] = podSafe
	safe["template"] = template
	result["spec"] = safe
	return result
}

// safeKey accepts label/annotation keys: an optional DNS prefix and a name.
var safeKey = regexp.MustCompile(`^([a-z0-9]([-a-z0-9.]*[a-z0-9])?/)?[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)

// safeText is a bounded printable non-secret display value.
func safeText(v string) bool {
	if len(v) > 256 || !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// selectKeyed keeps string labels with safe keys and values.
func selectKeyed(raw any) map[string]any {
	input, _ := raw.(map[string]any)
	out := map[string]any{}
	for key, value := range input {
		if text, ok := value.(string); ok && len(key) <= 253 && safeKey.MatchString(key) && safeText(text) {
			out[key] = text
		}
	}
	return out
}

// credentialKey names annotation keys that may carry credentials.
var credentialKey = regexp.MustCompile(`(?i)(password|passwd|secret|token|credential|private.?key|auth)`)

// selectAnnotations keeps non-platform policy annotations (Git render bundle
// defaults, ADR-021). Vault Agent delivery, orchestrator.io metadata and
// credential-named keys are excluded from the review projection.
func selectAnnotations(raw any) map[string]any {
	out := selectKeyed(raw)
	for key := range out {
		if strings.HasPrefix(key, "vault.hashicorp.com/") || strings.HasPrefix(key, "orchestrator.io/") || credentialKey.MatchString(key) {
			delete(out, key)
		}
	}
	return out
}

// safeTree copies a bounded JSON tree of scheduling/security fields keeping
// only scalars and safe strings; it returns nil for nothing safe.
func safeTree(v any, depth int) any {
	if depth > 6 {
		return nil
	}
	switch value := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, item := range value {
			if !safeKey.MatchString(key) {
				continue
			}
			if child := safeTree(item, depth+1); child != nil {
				out[key] = child
			}
		}
		if len(out) == 0 && len(value) > 0 {
			return nil
		}
		return out
	case []any:
		out := []any{}
		for _, item := range value {
			if child := safeTree(item, depth+1); child != nil {
				out = append(out, child)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		if safeText(value) {
			return value
		}
	case bool, int, int32, int64, float64:
		return value
	}
	return nil
}

func selectObject(input map[string]any, fields ...string) map[string]any {
	out := map[string]any{}
	for _, field := range fields {
		switch value := input[field].(type) {
		case string, bool, int, int64, float64:
			out[field] = value
		}
	}
	return out
}

func selectObjectList(input []any, fields ...string) []any {
	out := []any{}
	for _, entry := range input {
		if obj, ok := entry.(map[string]any); ok {
			out = append(out, selectObject(obj, fields...))
		}
	}
	return out
}

func selectLabels(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		if safeLabel(key) {
			if label, ok := value.(string); ok && safeLabel(label) {
				out[key] = label
			}
		}
	}
	return out
}

func (a Assembler) manifests(ctx context.Context, plan *planning.Plan, descriptor, workloadID string, old []Artifact) []Artifact {
	module, ok := plan.CandidateSet.Modules[workloadID]
	if !ok {
		return nil
	}
	_, existed := plan.BaseSet.Modules[workloadID]
	selection, ok := plan.Rendering[workloadID]
	if !ok {
		return nil
	}
	place := plan.Placements[descriptor]
	namespace := place.Namespace
	namespaceUnknown := len(namespace) > 63 || !namespaceLabel.MatchString(namespace)
	if namespaceUnknown {
		namespace = "preview-namespace"
	}
	if a.Renderer == nil {
		return []Artifact{{ID: descriptor + ":manifest", NodeID: descriptor, Kind: "kubernetes-manifest", Path: "Deployment/" + workloadID, Language: "yaml", Before: beforeFor(old, "kubernetes-manifest", "Deployment/"+workloadID, !existed), After: unavailable("RENDER_UNAVAILABLE")}}
	}
	plain, secret, unknown := symbolicEnv(module)
	symbolicModule := module
	symbolicModule.Spec.Containers = map[string]environment.Container{}
	for name, container := range module.Spec.Containers {
		container.Variables = map[string]string{}
		symbolicModule.Spec.Containers[name] = container
	}
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	manifests, err := a.Renderer.Render(deadline, execution.RenderRequest{Selection: selection, WorkloadID: workloadID, Module: symbolicModule, Namespace: namespace, PlainEnv: plain, SecretEnv: secret, Labels: map[string]string{},
		Services: execution.ServicePortsOf(plan.CandidateSet.Modules), UnknownPaths: unknown})
	if err != nil {
		return []Artifact{{ID: descriptor + ":manifest", NodeID: descriptor, Kind: "kubernetes-manifest", Path: "Deployment/" + workloadID, Language: "yaml", Before: beforeFor(old, "kubernetes-manifest", "Deployment/"+workloadID, !existed), After: unavailable("RENDER_UNAVAILABLE")}}
	}
	projections := ProjectManifests(manifests)
	unknownSet := map[string]bool{}
	for _, path := range unknown {
		unknownSet[path] = true
	}
	for _, manifest := range manifests {
		if !reviewedKind(manifest.Kind) || manifest.Auxiliary {
			continue
		}
		obj := sanitizeManifest(manifest)
		if namespaceUnknown {
			metadata, _ := obj["metadata"].(map[string]any)
			metadata["namespace"] = "<namespace: chờ cấp phát>"
		}
		spec, _ := obj["spec"].(map[string]any)
		tpl, _ := spec["template"].(map[string]any)
		pod, _ := tpl["spec"].(map[string]any)
		containers, _ := pod["containers"].([]any)
		for _, raw := range containers {
			c, _ := raw.(map[string]any)
			name, _ := c["name"].(string)
			env, _ := c["env"].([]any)
			for _, entry := range env {
				e, _ := entry.(map[string]any)
				key, _ := e["name"].(string)
				if unknownSet["spec.template.spec.containers."+name+".env."+key] {
					if _, exists := e["value"]; exists {
						e["value"] = "<output: chờ cấp phát>"
					}
				}
			}
		}
		if encoded, err := yaml.Marshal(obj); err == nil && len(encoded) <= MaxSideBytes {
			side := projections[manifest.Kind+"/"+manifest.Name]
			side.Text = string(encoded)
			projections[manifest.Kind+"/"+manifest.Name] = side
		}
	}
	keys := make([]string, 0, len(projections))
	for key := range projections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := []Artifact{}
	for _, key := range keys {
		side := projections[key]
		side.UnknownPaths = append(side.UnknownPaths, unknown...)
		if namespaceUnknown {
			side.UnknownPaths = append(side.UnknownPaths, "metadata.namespace")
		}

		side.Provenance = renderProvenance(plan.PlanHash, selection)
		out = append(out, Artifact{ID: descriptor + ":" + key, NodeID: descriptor, Kind: "kubernetes-manifest", Path: key, Language: "yaml", Before: beforeFor(old, "kubernetes-manifest", key, !existed), After: side})
	}
	return out
}

func symbolicEnv(module environment.Module) (map[string]map[string]string, map[string]map[string]string, []string) {
	plain, secret := map[string]map[string]string{}, map[string]map[string]string{}
	unknown := []string{}
	for containerName, container := range module.Spec.Containers {
		plain[containerName] = map[string]string{}
		secret[containerName] = map[string]string{}
		for name, value := range container.Variables {
			path := "spec.template.spec.containers." + containerName + ".env." + name
			if strings.HasPrefix(value, "${context.uc12.") {
				secret[containerName][name] = "orchestrator-preview-secret"
				unknown = append(unknown, path)
				continue
			}
			refs, err := placeholder.Refs(value)
			if err != nil {
				unknown = append(unknown, path)
				plain[containerName][name] = "orchestrator-preview-unknown"
				continue
			}
			if len(refs) > 0 {
				unknown = append(unknown, path)
			}
			// Never pass original literals, references or credentials to the
			// renderer subprocess. This sentinel changes only an env value.
			plain[containerName][name] = "orchestrator-preview-unknown"
		}
	}
	sort.Strings(unknown)
	return plain, secret, unknown
}

// RenderedSnapshot captures sanitized intent from the exact execution render.
// Call after validation and before Apply; status on the owning workload row
// decides when it is an applied baseline.
func RenderedSnapshot(plan *planning.Plan, workloadID string, manifests []execution.Manifest) map[string]any {
	descriptor := ""
	for _, node := range plan.Graph.Nodes {
		if node.Kind == planning.NodeWorkload && node.WorkloadID == workloadID {
			descriptor = node.Descriptor
			break
		}
	}
	if descriptor == "" {
		return nil
	}
	projected := ProjectManifests(manifests)
	keys := make([]string, 0, len(projected))
	for key := range projected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	artifacts := []Artifact{}
	var desired *Review
	if value, ok := plan.Review.(Review); ok {
		desired = &value
	}
	for _, key := range keys {
		side := projected[key]
		side.Provenance = map[string]string{"planHash": plan.PlanHash}
		if selection, ok := plan.Rendering[workloadID]; ok && selection.Bundle.Git() {
			side.Provenance = renderProvenance(plan.PlanHash, selection)
		}
		before := unavailable("BASELINE_MISSING")
		if desired != nil {
			for _, art := range desired.Artifacts {
				if art.NodeID == descriptor && art.Kind == "kubernetes-manifest" && art.Path == key {
					before = art.Before
					break
				}
			}
		}
		artifacts = append(artifacts, Artifact{ID: descriptor + ":" + key, NodeID: descriptor, Kind: "kubernetes-manifest", Path: key, Language: "yaml", Before: before, After: side})
	}
	raw, err := json.Marshal(map[string]any{"version": 1, "planHash": plan.PlanHash, "artifacts": artifacts})
	if err != nil {
		return nil
	}
	var snapshot map[string]any
	if json.Unmarshal(raw, &snapshot) != nil {
		return nil
	}
	return snapshot
}
