package planning

import (
	"sort"

	"orchestrator/internal/platform/canon"
)

// Resource intents (create/retain/bind) shown by Preview.
const (
	IntentCreate = "CREATE"
	IntentRetain = "RETAIN"
	IntentBind   = "BIND"
	// IntentUpdate: the identity exists and its authored Definition, module
	// pin, provider targets or params differ from the last executed plan.
	IntentUpdate = "UPDATE"
	// IntentUnverified: the identity exists but no persisted plan can prove
	// it unchanged; a possible update is never shown as "retain".
	IntentUnverified = "UNVERIFIED"
)

// Ownership values of a previewed resource: the implicit Environment cluster
// binding is a platform resource (ADR-013); every other node is owned by the
// orchestrator within its scope.
const (
	OwnerPlatform     = "PLATFORM"
	OwnerOrchestrator = "ORCHESTRATOR"
)

// IntentModule is the public pin of the verified module a node will run.
type IntentModule struct {
	Module        string `json:"module"`
	SourceID      string `json:"sourceId"`
	Revision      string `json:"revision"`
	Subdirectory  string `json:"subdirectory"`
	ContentSHA256 string `json:"contentSha256"`
}

// IntentProvider is one resolved provider target of a node, without any
// credential, endpoint or state reference.
type IntentProvider struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Target        string `json:"target"`
	ConnectionKey string `json:"connectionKey,omitempty"`
	Region        string `json:"region,omitempty"`
	Provider      string `json:"provider,omitempty"`
}

// ResourceIntent is the safe Preview projection of one resource node (UC-05
// BR-10): type/class, Definition/module/revision, provider targets,
// ownership and create/retain intent. It carries no params, resolved inputs,
// secret or state reference and does not claim a final Terraform plan.
type ResourceIntent struct {
	Descriptor    string           `json:"descriptor"`
	ResourceType  string           `json:"resourceType"`
	Class         string           `json:"class"`
	Scope         string           `json:"scope"`
	Ownership     string           `json:"ownership"`
	Intent        string           `json:"intent"`
	DefinitionKey string           `json:"definitionKey"`
	DriverType    string           `json:"driverType"`
	Module        *IntentModule    `json:"module,omitempty"`
	Providers     []IntentProvider `json:"providers,omitempty"`
}

// intentFingerprints hashes, per node, the authored intent that an execution
// would act on: Definition, module pin, provider targets and authored params.
// It is compared between two plans and never displayed or persisted.
func (p *Plan) intentFingerprints() map[string]string {
	contracts := map[string]TerraformContract{}
	for _, contract := range p.Terraform {
		contracts[contract.Descriptor] = contract
	}
	out := map[string]string{}
	for _, node := range p.Graph.Nodes {
		if node.Kind != NodeResource {
			continue
		}
		contract := contracts[node.Descriptor]
		hash, err := canon.Hash(map[string]any{
			"definition": p.Matches[node.Descriptor].DefinitionKey, "driver": p.Matches[node.Descriptor].DriverType,
			"params": node.Params, "source": contract.Source, "providers": contract.ProviderBindings, "mapping": contract.OutputMapping,
		})
		if err == nil {
			out[node.Descriptor+"|"+node.Scope.Key()] = hash
		}
	}
	return out
}

// ResourceIntents projects the resource nodes of the plan, ordered by
// descriptor then scope. The Workload node is shown through the renderer
// selection, not here. previous returns the plan an existing node was last
// executed under (nil when unknown); it is read from persisted plan snapshots
// by the caller, so no provider, CLI or secret call is made.
func (p *Plan) ResourceIntents(previous func(Node) *Plan) []ResourceIntent {
	current := p.intentFingerprints()
	isNew := map[string]bool{}
	for _, descriptor := range p.Classification.New {
		isNew[descriptor] = true
	}
	contracts := map[string]TerraformContract{}
	for _, contract := range p.Terraform {
		contracts[contract.Descriptor] = contract
	}
	out := []ResourceIntent{}
	for _, node := range p.Graph.Nodes {
		if node.Kind != NodeResource {
			continue
		}
		match := p.Matches[node.Descriptor]
		intent := ResourceIntent{
			Descriptor: node.Descriptor, ResourceType: node.ResourceType, Class: node.Class,
			Scope: node.Scope.Key(), Ownership: OwnerOrchestrator,
			DefinitionKey: match.DefinitionKey, DriverType: string(match.DriverType),
		}
		if isNew[node.Descriptor] {
			intent.Intent = IntentCreate
		} else {
			intent.Intent = IntentUnverified
			if previous != nil {
				if old := previous(node); old != nil {
					if before, ok := old.intentFingerprints()[node.Descriptor+"|"+node.Scope.Key()]; ok {
						intent.Intent = IntentUpdate
						if before == current[node.Descriptor+"|"+node.Scope.Key()] {
							intent.Intent = IntentRetain
						}
					}
				}
			}
		}
		if match.Binding == BindingEnvironmentConnection {
			intent.Ownership, intent.Intent = OwnerPlatform, IntentBind
		}
		if contract, ok := contracts[node.Descriptor]; ok {
			if contract.Source != nil {
				intent.Module = &IntentModule{Module: contract.Module, SourceID: contract.Source.SourceID, Revision: contract.Source.Revision, Subdirectory: contract.Source.Subdirectory, ContentSHA256: contract.Source.ContentSHA256}
			}
			for _, pin := range contract.ProviderBindings {
				intent.Providers = append(intent.Providers, IntentProvider{Name: pin.Name, Kind: pin.Kind, Target: pin.Target, ConnectionKey: pin.ConnectionKey, Region: pin.Region, Provider: pin.Provider})
			}
		}
		out = append(out, intent)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Descriptor != out[j].Descriptor {
			return out[i].Descriptor < out[j].Descriptor
		}
		return out[i].Scope < out[j].Scope
	})
	return out
}
