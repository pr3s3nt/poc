package catalog

import (
	"context"
	"errors"
	"path"
	"sort"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

// CapabilityVariable is the public, secret-free view of one editable Driver
// Input variable (UC-03 BR-23, OC-04d).
type CapabilityVariable struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Required      bool   `json:"required"`
	HasDefault    bool   `json:"hasDefault"`
	Default       any    `json:"default,omitempty"`
	ResourceParam bool   `json:"resourceParam"`
	Preset        string `json:"preset,omitempty"`
}

// CapabilitySource is the public verified pin and optional credential-free
// HTTPS origin. It contains no credential or cache path (UC-03 GIT-05).
type CapabilitySource struct {
	SourceID      string `json:"sourceId"`
	ModuleID      string `json:"moduleId"`
	Revision      string `json:"revision"`
	Subdirectory  string `json:"subdirectory"`
	ContentSHA256 string `json:"contentSha256"`
	RepositoryURL string `json:"repositoryUrl,omitempty"`
}

// CapabilityProvider is one pinned provider requirement and its binding kind.
type CapabilityProvider struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Version string `json:"version"`
	Binding string `json:"binding"`
}

// CapabilityOutput is one module output (executor outputs have no type).
type CapabilityOutput struct {
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Sensitive bool   `json:"sensitive"`
}

// CapabilityAccount states whether the pair needs a Driver Account (UC-03
// V02-03). Type is the required account type; RegionVariable names the
// literal Driver Inputs variable that carries the AWS region.
type CapabilityAccount struct {
	Required       bool   `json:"required"`
	Type           string `json:"type,omitempty"`
	RegionVariable string `json:"regionVariable,omitempty"`
}

// CapabilityTarget is one provider binding target a Definition may use.
type CapabilityTarget struct {
	Target string `json:"target"`
}

// CapabilityBinding lists the allowed targets of one configurable provider.
type CapabilityBinding struct {
	Provider string             `json:"provider"`
	Kind     string             `json:"kind"`
	Targets  []CapabilityTarget `json:"targets"`
}

// Capability describes one registrable Resource Type/driver/module pair.
type Capability struct {
	ResourceType     string               `json:"resourceType"`
	DriverType       string               `json:"driverType"`
	Module           string               `json:"module"`
	Source           *CapabilitySource    `json:"source,omitempty"`
	TerraformVersion string               `json:"terraformVersion,omitempty"`
	Providers        []CapabilityProvider `json:"providerRequirements,omitempty"`
	Outputs          []CapabilityOutput   `json:"outputs,omitempty"`
	Variables        []CapabilityVariable `json:"variables"`
	ExecutorOwned    []string             `json:"executorOwned"`
	Account          CapabilityAccount    `json:"account"`
	Bindings         []CapabilityBinding  `json:"bindings"`
}

// ListDefinitionCapabilities returns the supported registration pairs with
// their editable variable metadata. Terraform pairs come from verified
// registry metadata only: no fetch, inspection, write, admission, execution
// or provider call. Module variables never define a Resource Type;
// resourceParam only reflects the inputs the scoped Resource Type declares.
func (s *Service) ListDefinitionCapabilities(ctx context.Context, organizationKey string) ([]Capability, error) {
	types, err := s.store.ListResourceTypes(ctx, organizationKey)
	if err != nil {
		return nil, err
	}
	byKey := map[string]resource.Type{}
	for _, typ := range types {
		byKey[typ.Key] = typ
	}
	if s.modules == nil {
		return nil, &internalError{err: errors.New("catalog: the Terraform module registry is unavailable")}
	}
	bundles, err := s.modules.List(ctx)
	if err != nil {
		return nil, &internalError{err: err}
	}
	out := []Capability{}
	for _, bundle := range bundles {
		if bundle.Contract.SourceKind == tfmodule.SourceKindZIP && bundle.Contract.SourceOrganizationKey != organizationKey {
			continue
		}
		for _, resourceType := range bundle.Contract.ResourceTypes {
			capability, err := terraformCapability(bundle, byKey[resourceType], resourceType)
			if err != nil {
				return nil, &internalError{err: err}
			}
			out = append(out, capability)
		}
	}
	out = append(out, platformCapabilities(byKey)...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ResourceType != b.ResourceType {
			return a.ResourceType < b.ResourceType
		}
		if a.DriverType != b.DriverType {
			return a.DriverType < b.DriverType
		}
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		return sourceKey(a.Source) < sourceKey(b.Source)
	})
	return out, nil
}

func sourceKey(s *CapabilitySource) string {
	if s == nil {
		return ""
	}
	return s.SourceID + "\x00" + s.Revision + "\x00" + s.Subdirectory + "\x00" + s.ContentSHA256
}

func terraformCapability(bundle tfmodule.Bundle, typ resource.Type, resourceType string) (Capability, error) {
	ref := bundle.Ref
	capability := Capability{
		ResourceType: resourceType, DriverType: string(resource.DriverTerraform), Module: ref.ModuleID,
		Source:           &CapabilitySource{SourceID: ref.SourceID, ModuleID: ref.ModuleID, Revision: ref.Revision, Subdirectory: ref.Subdirectory, ContentSHA256: ref.ContentSHA256, RepositoryURL: bundle.Contract.SourceURL},
		TerraformVersion: bundle.TerraformVersion,
		Providers:        []CapabilityProvider{},
		Outputs:          []CapabilityOutput{},
		Variables:        []CapabilityVariable{},
		ExecutorOwned:    append([]string{}, bundle.Contract.ExecutorOwned...),
		Bindings:         []CapabilityBinding{},
	}
	if bundle.Contract.SourceURL != "" {
		capability.Module = path.Base(ref.Subdirectory)
		if capability.Module == "." {
			capability.Module = resourceType
		}
	}
	sort.Strings(capability.ExecutorOwned)
	for _, name := range sortedKeys(bundle.Providers) {
		p := bundle.Providers[name]
		capability.Providers = append(capability.Providers, CapabilityProvider{Name: name, Source: p.Source, Version: p.Version, Binding: p.Binding})
		switch p.Binding {
		case tfmodule.BindingAWS:
			capability.Account = CapabilityAccount{Required: true, Type: "AWS", RegionVariable: "region"}
			capability.Bindings = append(capability.Bindings, CapabilityBinding{Provider: name, Kind: p.Binding, Targets: []CapabilityTarget{{Target: tfmodule.TargetAccount}}})
		case tfmodule.BindingKubernetes:
			capability.Bindings = append(capability.Bindings, CapabilityBinding{Provider: name, Kind: p.Binding, Targets: []CapabilityTarget{{Target: tfmodule.TargetResource}}})
		}
	}
	for _, name := range sortedKeys(bundle.Contract.Outputs) {
		o := bundle.Contract.Outputs[name]
		capability.Outputs = append(capability.Outputs, CapabilityOutput{Name: name, Type: o.Type, Sensitive: o.Sensitive})
	}
	for _, name := range bundle.Contract.ExecutorOutputs {
		capability.Outputs = append(capability.Outputs, CapabilityOutput{Name: name})
	}
	owned := map[string]bool{}
	for _, name := range bundle.Contract.ExecutorOwned {
		owned[name] = true
	}
	presets := map[string]string{}
	if bundle.Contract.SourceKind == tfmodule.SourceKindPackaged {
		presets = terraformPresets[ref.ModuleID]
	}
	for _, name := range sortedKeys(bundle.Contract.Inputs) {
		declared := bundle.Contract.Inputs[name]
		if owned[name] || declared.Sensitive || credentialVariables[name] {
			continue // executor-owned, sensitive and credential inputs have no author control
		}
		parsed, err := tfmodule.ParseType(declared.Type)
		if err != nil {
			return Capability{}, err
		}
		_, param := typ.Input(name)
		entry := CapabilityVariable{Name: name, Type: parsed.String(), Required: declared.Required, HasDefault: declared.HasDefault, ResourceParam: param, Preset: presets[name]}
		if declared.HasDefault && declared.Default != nil {
			entry.Default = declared.Default
		}
		capability.Variables = append(capability.Variables, entry)
	}
	return capability, nil
}

// Presets of the typed placement references a workload Definition declares
// (ADR-016). They are suggestions the author applies explicitly.
const (
	NamespaceReferencePreset = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	ClusterReferencePreset   = "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"
)

// platformCapabilities lists the non-module registration pairs: a public
// existing-cluster Definition with its own Internal Driver Account, and the
// score-k8s workload with its typed namespace/cluster references only (no
// other workload variable is consumed at execution). Metadata only; nothing is executed.
func platformCapabilities(types map[string]resource.Type) []Capability {
	out := []Capability{}
	if typ, ok := types[resource.ClusterTargetType]; ok {
		if field, declared := typ.Output(resource.ClusterTargetOutput); declared && !field.Secret {
			out = append(out, Capability{
				ResourceType: typ.Key, DriverType: string(resource.DriverExistingCluster),
				Variables: []CapabilityVariable{}, ExecutorOwned: []string{}, Bindings: []CapabilityBinding{},
				Account: CapabilityAccount{Required: true, Type: "INTERNAL"},
				Outputs: []CapabilityOutput{{Name: resource.ClusterTargetOutput, Type: "any"}},
			})
		}
	}
	if typ, ok := types["workload"]; ok {
		variables := []CapabilityVariable{
			{Name: resource.VariableRenderBundle, Type: "string", Required: true},
			{Name: resource.VariableNamespace, Type: "string", Required: true, Preset: NamespaceReferencePreset},
			{Name: resource.VariableCluster, Type: "string", Required: true, Preset: ClusterReferencePreset},
		}
		out = append(out, Capability{
			ResourceType: typ.Key, DriverType: string(resource.DriverScoreK8s),
			Variables: variables, ExecutorOwned: []string{}, Bindings: []CapabilityBinding{}, Account: CapabilityAccount{},
		})
	}
	return out
}
