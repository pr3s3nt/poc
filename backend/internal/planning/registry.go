package planning

import (
	"context"
	"fmt"
	"sort"
	"time"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
)

// RegistryInspector is the product ModuleInspector: it parses a Terraform
// Definition's pinned source and mappings and reads the verified bundle
// through registry metadata. It never fetches, inspects source code, runs
// Terraform or calls a provider, so planning and Preview stay pure.
type RegistryInspector struct {
	Metadata moduleregistry.Metadata
}

// metadataTimeout bounds one metadata read; bundles are immutable rows.
const metadataTimeout = 15 * time.Second

// Contract implements ModuleInspector.
func (r RegistryInspector) Contract(def resource.Definition) (ModuleContract, error) {
	if r.Metadata == nil {
		return ModuleContract{}, fmt.Errorf("the Terraform module registry is unavailable")
	}
	inputs, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil {
		return ModuleContract{}, fmt.Errorf("the Terraform driver inputs are not a verified module pin: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), metadataTimeout)
	defer cancel()
	bundle, err := r.Metadata.Bundle(ctx, inputs.Source)
	if err != nil {
		return ModuleContract{}, err
	}
	return ContractFromBundle(bundle, inputs), nil
}

// ContractFromBundle builds the planning contract of one verified bundle and
// one Definition's mappings.
func ContractFromBundle(bundle tfmodule.Bundle, inputs tfmodule.DriverInputs) ModuleContract {
	ref := bundle.Ref
	contract := ModuleContract{
		Module:             ref.ModuleID,
		Fingerprint:        "sha256:" + ref.ContentSHA256,
		Source:             &ref,
		BundleID:           bundle.ID,
		TerraformVersion:   bundle.TerraformVersion,
		ProviderLockDigest: bundle.ProviderLockDigest,
		Outputs:            bundle.OutputNames(),
		Variables:          map[string]ModuleVariable{},
		ExecutorOwned:      append([]string(nil), bundle.Contract.ExecutorOwned...),
		InputMapping:       map[string]string{},
		OutputMapping:      map[string]string{},
		SecretOutputs:      []string{},
		ProviderKinds:      map[string]string{},
	}
	for name, provider := range bundle.Providers {
		contract.ProviderKinds[name] = provider.Binding
	}
	for name, input := range bundle.Contract.Inputs {
		contract.Variables[name] = ModuleVariable{Name: name, Type: input.Type, HasDefault: input.HasDefault, Default: input.Default, Sensitive: input.Sensitive}
	}
	for variable, param := range inputs.InputMapping {
		contract.InputMapping[variable] = param
	}
	for output, target := range inputs.OutputMapping {
		contract.OutputMapping[output] = target
		if bundle.Contract.Outputs[output].Sensitive {
			contract.SecretOutputs = append(contract.SecretOutputs, target)
		}
	}
	sort.Strings(contract.SecretOutputs)
	return contract
}
