package persistence

import (
	"context"
	"encoding/json"
	"orchestrator/internal/domain/resource"
)

type DefinitionHistory struct {
	DefinitionKey string                        `json:"definitionKey"`
	Generation    int64                         `json:"generation"`
	ActiveID      string                        `json:"activeId"`
	Versions      []resource.DefinitionRevision `json:"versions"`
}
type DefinitionSelection struct {
	Frozen          bool                  `json:"frozen"`
	ApplicationKey  string                `json:"applicationKey"`
	EnvironmentKey  string                `json:"environmentKey"`
	Revision        int64                 `json:"revision"`
	AppliedRevision int64                 `json:"appliedRevision"`
	CatalogPin      string                `json:"catalogPin,omitempty"`
	Baseline        []resource.Definition `json:"baseline,omitempty"`
	Desired         []resource.Definition `json:"desired,omitempty"`
	Applied         []resource.Definition `json:"applied,omitempty"`
	AppliedVerified bool                  `json:"appliedVerified"`
}

func (s DefinitionSelection) Pending() bool { return s.Revision != s.AppliedRevision }

type DefinitionVersionRepository interface {
	GetDefinitionHistory(context.Context, string, string) (DefinitionHistory, error)
	SaveDefinitionHistory(context.Context, string, int64, DefinitionHistory) error
	GetDefinitionSelection(context.Context, string, string) (DefinitionSelection, error)
	FreezeDefinitionSelection(context.Context, DefinitionSelection) error
	SelectDefinitionSelection(context.Context, string, int64, int64, DefinitionSelection) error
	ApplyDefinitionSelection(context.Context, string, string, int64, []resource.Definition, string) error
}

// ValidateDefinitionHistory allows append-only snapshots and mutable archive/active metadata.
func ValidateDefinitionHistory(old, next DefinitionHistory) error {
	if next.DefinitionKey == "" || next.Generation < 1 || len(next.Versions) == 0 || len(next.Versions) > 10000 {
		return ErrImmutable
	}
	byID := map[string]resource.DefinitionRevision{}
	numbers := map[int64]bool{}
	active := false
	for _, v := range next.Versions {
		if v.ID == "" || v.Number < 1 || v.CreatedAt.IsZero() || v.Definition.Key != next.DefinitionKey || v.Definition.RevisionID != v.ID || numbers[v.Number] {
			return ErrImmutable
		}
		if _, ok := byID[v.ID]; ok {
			return ErrImmutable
		}
		byID[v.ID] = v
		numbers[v.Number] = true
		if v.ID == next.ActiveID {
			if v.Archived {
				return ErrImmutable
			}
			active = true
		}
	}
	if !active {
		return ErrImmutable
	}
	for _, v := range old.Versions {
		n, ok := byID[v.ID]
		if !ok {
			return ErrImmutable
		}
		n.Archived = v.Archived
		a, _ := json.Marshal(n)
		b, _ := json.Marshal(v)
		if string(a) != string(b) {
			return ErrImmutable
		}
	}
	return nil
}
