package deployment

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/canon"
)

// JSON Patch operations a Deployment Delta may contain.
const (
	PatchAdd     = "add"
	PatchRemove  = "remove"
	PatchReplace = "replace"
)

// JSONPatchOperation is one RFC 6902 operation. Its path is relative to the
// object the patch is attached to: one module for `modules.update.<id>`, the
// `shared` object for `shared` (UC-05 BR-05).
type JSONPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// MarshalJSON always writes `value` for add and replace, even when it is null,
// and never writes it for remove.
func (o JSONPatchOperation) MarshalJSON() ([]byte, error) {
	if o.Op == PatchRemove {
		return json.Marshal(struct {
			Op   string `json:"op"`
			Path string `json:"path"`
		}{o.Op, o.Path})
	}
	return json.Marshal(struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	}{o.Op, o.Path, o.Value})
}

// Validate reports an operation the Delta contract does not allow.
func (o JSONPatchOperation) Validate() error {
	switch o.Op {
	case PatchAdd, PatchRemove, PatchReplace:
	default:
		return fmt.Errorf("deployment: unsupported JSON Patch op %q", o.Op)
	}
	if o.Path != "" && !strings.HasPrefix(o.Path, "/") {
		return fmt.Errorf("deployment: JSON Patch path %q must be empty or start with /", o.Path)
	}
	return nil
}

// ModuleDelta groups module changes: full modules to add, sorted workload IDs
// to remove and module-relative patches to update.
type ModuleDelta struct {
	Add    map[string]environment.Module   `json:"add,omitempty"`
	Remove []string                        `json:"remove,omitempty"`
	Update map[string][]JSONPatchOperation `json:"update,omitempty"`
}

// IsEmpty reports whether no module changes.
func (m *ModuleDelta) IsEmpty() bool {
	return m == nil || (len(m.Add) == 0 && len(m.Remove) == 0 && len(m.Update) == 0)
}

// DeltaDocument is the Humanitec-shaped Deployment Delta content. Empty
// branches are omitted, so a no-op Delta serialises as `{}`.
type DeltaDocument struct {
	Modules *ModuleDelta         `json:"modules,omitempty"`
	Shared  []JSONPatchOperation `json:"shared,omitempty"`
}

// IsEmpty reports whether the Delta changes nothing.
func (d DeltaDocument) IsEmpty() bool { return d.Modules.IsEmpty() && len(d.Shared) == 0 }

// Validate checks the structural rules of the Delta document.
func (d DeltaDocument) Validate() error {
	if d.Modules != nil {
		touched := map[string]string{}
		claim := func(id, branch string) error {
			if id == "" {
				return fmt.Errorf("deployment: modules.%s has an empty workload ID", branch)
			}
			if other, ok := touched[id]; ok {
				return fmt.Errorf("deployment: workload %q appears in modules.%s and modules.%s", id, other, branch)
			}
			touched[id] = branch
			return nil
		}
		for id := range d.Modules.Add {
			if err := claim(id, "add"); err != nil {
				return err
			}
		}
		if !sort.StringsAreSorted(d.Modules.Remove) {
			return fmt.Errorf("deployment: modules.remove must be sorted")
		}
		for _, id := range d.Modules.Remove {
			if err := claim(id, "remove"); err != nil {
				return err
			}
		}
		for id, ops := range d.Modules.Update {
			if err := claim(id, "update"); err != nil {
				return err
			}
			if len(ops) == 0 {
				return fmt.Errorf("deployment: modules.update.%s is empty", id)
			}
			for _, op := range ops {
				if err := op.Validate(); err != nil {
					return err
				}
			}
		}
	}
	for _, op := range d.Shared {
		if err := op.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Normalized drops empty branches so the document has one canonical form.
func (d DeltaDocument) Normalized() DeltaDocument {
	out := DeltaDocument{}
	if !d.Modules.IsEmpty() {
		m := &ModuleDelta{}
		if len(d.Modules.Add) > 0 {
			m.Add = d.Modules.Add
		}
		if len(d.Modules.Remove) > 0 {
			m.Remove = d.Modules.Remove
		}
		if len(d.Modules.Update) > 0 {
			m.Update = d.Modules.Update
		}
		out.Modules = m
	}
	if len(d.Shared) > 0 {
		out.Shared = d.Shared
	}
	return out
}

// DeltaSnapshotMetadata records who produced a Snapshot and why. It never
// carries secrets.
type DeltaSnapshotMetadata struct {
	ActorRef   string `json:"actorRef,omitempty"`
	Action     Action `json:"action"`
	WorkloadID string `json:"workloadId"`
}

// DeploymentDeltaSnapshot is the immutable Delta persisted for exactly one
// Deployment (architecture/domain/domain-objects.md). It is not the mutable
// Humanitec Delta entity; that lifecycle is deferred at D05.
type DeploymentDeltaSnapshot struct {
	ID             string                `json:"id"`
	ApplicationKey string                `json:"applicationKey"`
	Document       DeltaDocument         `json:"document"`
	DocumentHash   string                `json:"documentHash"`
	Metadata       DeltaSnapshotMetadata `json:"metadata"`
	CreatedAt      time.Time             `json:"createdAt"`
}

// NewDeploymentDeltaSnapshot validates the document and fingerprints it.
func NewDeploymentDeltaSnapshot(id, applicationKey string, doc DeltaDocument, metadata DeltaSnapshotMetadata, createdAt time.Time) (DeploymentDeltaSnapshot, error) {
	if id == "" || applicationKey == "" {
		return DeploymentDeltaSnapshot{}, fmt.Errorf("deployment: delta snapshot needs an id and an application key")
	}
	doc = doc.Normalized()
	if err := doc.Validate(); err != nil {
		return DeploymentDeltaSnapshot{}, err
	}
	hash, err := canon.Hash(doc)
	if err != nil {
		return DeploymentDeltaSnapshot{}, err
	}
	return DeploymentDeltaSnapshot{
		ID:             id,
		ApplicationKey: applicationKey,
		Document:       doc,
		DocumentHash:   hash,
		Metadata:       metadata,
		CreatedAt:      createdAt,
	}, nil
}

// Validate checks identity, document rules and the content fingerprint.
func (s DeploymentDeltaSnapshot) Validate() error {
	if s.ID == "" || s.ApplicationKey == "" {
		return fmt.Errorf("deployment: delta snapshot needs an id and an application key")
	}
	if err := s.Document.Validate(); err != nil {
		return err
	}
	hash, err := canon.Hash(s.Document)
	if err != nil {
		return err
	}
	if hash != s.DocumentHash {
		return fmt.Errorf("deployment: delta snapshot %q document hash does not match its content", s.ID)
	}
	return nil
}
