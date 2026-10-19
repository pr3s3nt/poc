package planning

import (
	"sort"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/canon"
)

// Resource intents (create/retain/bind) shown by Preview.
const (
	IntentCreate = "CREATE"
	IntentRetain = "RETAIN"
	// IntentUpdate: the identity exists and its authored Definition, module
	// pin, provider targets or params differ from the last executed plan.
	IntentUpdate = "UPDATE"
	// IntentUnverified: the identity exists but no persisted plan can prove
	// it unchanged; a possible update is never shown as "retain".
	IntentUnverified = "UNVERIFIED"
)

// Ownership values of a previewed resource: every Terraform-managed node is
// owned by the orchestrator within its scope; an existing cluster is only reached.
const (
	OwnerOrchestrator = "ORCHESTRATOR"
	// OwnerPreexisting marks a cluster the orchestrator reaches but never
	// creates or destroys (ADR-016).
	OwnerPreexisting = "PREEXISTING"
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
	Descriptor    string `json:"descriptor"`
	ResourceType  string `json:"resourceType"`
	Class         string `json:"class"`
	Scope         string `json:"scope"`
	Ownership     string `json:"ownership"`
	Intent        string `json:"intent"`
	DefinitionKey string `json:"definitionKey"`
	DriverType    string `json:"driverType"`
	// DriverAccount and AccountVersion name the safe Driver Account key a
	// node's Definition pinned (an Internal account for existing-cluster).
	DriverAccount  string           `json:"driverAccount,omitempty"`
	AccountVersion int64            `json:"accountVersion,omitempty"`
	Module         *IntentModule    `json:"module,omitempty"`
	Providers      []IntentProvider `json:"providers,omitempty"`
}

// intentFingerprints hashes, per node, the complete authored and pinned
// execution intent an execution would act on: Definition and driver, explicit
// Connection/binding, authored params, the verified bundle (source pin, bundle
// ID, release pins, contract fingerprint), the module input table (fallback
// variables and input-mapping sources, kept raw so no secret is resolved), the
// output mapping and the resolved provider targets. It is compared between two
// plans only; it is never displayed, returned or persisted. A Terraform node
// without contract evidence has no fingerprint, so it reads as unverified.
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
		match := p.Matches[node.Descriptor]
		contract, hasContract := contracts[node.Descriptor]
		if match.DriverType == resource.DriverTerraform && (!hasContract || contract.Fingerprint == "") {
			continue
		}
		evidence, hasEvidence := p.IntentEvidence[node.Descriptor]
		if !hasEvidence {
			continue // no authored-content evidence (for example an older persisted plan): unverified
		}
		hash, err := canon.Hash(map[string]any{
			"definition": match.DefinitionKey, "driver": match.DriverType, "account": match.DriverAccount, "accountVersion": match.AccountVersion,
			"type": node.ResourceType, "class": node.Class, "params": node.Params, "bindings": node.Bindings,
			"authored": evidence, "fingerprint": contract.Fingerprint, "bundle": contract.BundleID, "terraform": contract.TerraformVersion, "lock": contract.ProviderLockDigest,
			"source": contract.Source, "inputs": contract.Inputs, "outputs": contract.OutputMapping, "secretOutputs": contract.SecretOutputs, "providers": contract.ProviderBindings,
		})
		if err == nil {
			out[node.Descriptor+"|"+node.Scope.Key()] = hash
		}
	}
	return out
}

// intentEvidence digests the authored content of each matched Definition. The
// implicit Environment cluster binding has no stored Definition and no entry.
func intentEvidence(matches map[string]Match, matcher *definitionMatcher) map[string]string {
	out := map[string]string{}
	for descriptor, match := range matches {
		def, ok := matcher.definition(match.DefinitionKey)
		if !ok {
			continue
		}
		hash, err := canon.Hash(map[string]any{"driver": def.DriverType, "account": def.DriverAccount, "inputs": def.DriverInputs, "provision": def.Provision})
		if err == nil {
			out[descriptor] = hash
		}
	}
	return out
}

// PlacementList returns the plan's placements ordered by node.
func (p *Plan) PlacementList() []Placement {
	out := make([]Placement, 0, len(p.Placements))
	for _, node := range sortedKeys(p.Placements) {
		out = append(out, p.Placements[node])
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
		if match.DriverType == resource.DriverExistingCluster {
			intent.DriverAccount, intent.AccountVersion = match.DriverAccount, match.AccountVersion
			intent.Ownership = OwnerPreexisting
		}
		if isNew[node.Descriptor] {
			intent.Intent = IntentCreate
		} else {
			intent.Intent = IntentUnverified
			if previous != nil {
				if old := previous(node); old != nil {
					key := node.Descriptor + "|" + node.Scope.Key()
					before, hadBefore := old.intentFingerprints()[key]
					now, hasNow := current[key]
					if hadBefore && hasNow {
						intent.Intent = IntentUpdate
						if before == now {
							intent.Intent = IntentRetain
						}
					}
				}
			}
		}
		if contract, ok := contracts[node.Descriptor]; ok {
			if contract.Source != nil {
				intent.Module = &IntentModule{Module: contract.Module, SourceID: contract.Source.SourceID, Revision: contract.Source.Revision, Subdirectory: contract.Source.Subdirectory, ContentSHA256: contract.Source.ContentSHA256}
			}
			for _, pin := range contract.ProviderBindings {
				connection := pin.ConnectionKey
				if connection == "" {
					connection = pin.ClusterAccount // an EKS target the plan itself creates: the producing AWS account
				}
				intent.Providers = append(intent.Providers, IntentProvider{Name: pin.Name, Kind: pin.Kind, Target: pin.Target, ConnectionKey: connection, Region: pin.Region, Provider: pin.Provider})
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
