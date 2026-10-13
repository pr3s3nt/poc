package catalog

import (
	"context"
	"errors"
	"sort"
	"strings"

	"orchestrator/internal/application/target"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
)

// Matching preview statuses (UC-03 BR-20..22, OC-04c).
const (
	PreviewIncomplete         = "INCOMPLETE"
	PreviewMatched            = "MATCHED"
	PreviewNoMatch            = "NO_MATCH"
	PreviewAmbiguous          = "AMBIGUOUS"
	PreviewConnectionConflict = "CONNECTION_CONFLICT"
	PreviewSystemBinding      = "SYSTEM_BINDING"
)

// Bounds keep one preview request cheap and its response small.
const (
	maxPreviewCriteria    = 64
	maxPreviewContextText = 253
)

// ErrTargetUnavailable reports an Environment without a usable execution
// binding (HTTP 422).
var ErrTargetUnavailable = errors.New("catalog: environment execution target is unavailable")

// ErrPreviewNotFound reports an Application or Environment outside the
// Organization (HTTP 404).
var ErrPreviewNotFound = errors.New("catalog: application or environment not found")

// MatchingContext is the explicit client context. Organization, profile,
// environment type and Connection always come from the server snapshot.
type MatchingContext struct {
	ApplicationID string `json:"applicationId"`
	EnvironmentID string `json:"environmentId"`
	ResourceID    string `json:"resourceId"`
	Class         string `json:"class"`
}

// MatchingPreviewQuery is one unsaved candidate plus its context.
type MatchingPreviewQuery struct {
	Definition resource.Definition `json:"definition"`
	Context    MatchingContext     `json:"context"`
}

// MatchingCandidate is the safe per-Definition result.
type MatchingCandidate struct {
	Key           string              `json:"key"`
	Eligible      bool                `json:"eligible"`
	Matched       bool                `json:"matched"`
	BestCriterion *resource.Criterion `json:"bestCriterion,omitempty"`
	Score         *int                `json:"score,omitempty"`
}

// MatchingWinner identifies the winning Definition.
type MatchingWinner struct {
	Key   string `json:"key"`
	Score int    `json:"score"`
}

// MatchingConflict reports a winner whose Connection differs from the target.
type MatchingConflict struct {
	DefinitionKey string `json:"definitionKey"`
	ConnectionKey string `json:"connectionKey"`
	TargetKey     string `json:"targetConnectionKey"`
}

// MatchingSnapshot identifies what the result reflects; it is not a token.
type MatchingSnapshot struct {
	Marker             string `json:"marker"`
	EnvironmentVersion int64  `json:"environmentVersion"`
}

// MatchingPreview is the 200 response of OC-04c.
type MatchingPreview struct {
	Status             string              `json:"status"`
	MissingFields      []string            `json:"missingFields"`
	Candidates         []MatchingCandidate `json:"candidates"`
	Winner             *MatchingWinner     `json:"winner,omitempty"`
	Ties               []string            `json:"ties,omitempty"`
	ConnectionConflict *MatchingConflict   `json:"connectionConflict,omitempty"`
	Snapshot           MatchingSnapshot    `json:"snapshot"`
}

// PreviewInvalidError is a validation failure with a fixed message: it names
// the field path only and never echoes a submitted value.
type PreviewInvalidError struct{ Field string }

func (e *PreviewInvalidError) Error() string {
	return "catalog: invalid matching preview document: " + e.Field
}
func (e *PreviewInvalidError) Is(target error) bool { return target == ErrInvalid }

func invalid(field string) error { return &PreviewInvalidError{Field: field} }

func boundedText(s string) bool {
	if len(s) > maxPreviewContextText {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// validatePreviewCandidate applies structural matching validation only: no
// driver contract, connection, bundle or placeholder inspection. Errors name
// the field and never echo submitted values.
func validatePreviewCandidate(def resource.Definition, types map[string]resource.Type) error {
	if def.Key == planning.BuiltinClusterKey || def.DriverType == resource.DriverExistingCluster {
		return invalid("definition.key")
	}
	if validatePublicID("resource definition", def.Key) != nil || len(def.Key) > maxPreviewContextText {
		return invalid("definition.key")
	}
	if _, ok := types[def.ResourceTypeKey]; !ok {
		return invalid("definition.resourceType")
	}
	if def.ExecutionProfile != "" && def.ExecutionProfile != "internal-k8s" && def.ExecutionProfile != "aws-eks" {
		return invalid("definition.executionProfile")
	}
	switch def.DriverType {
	case resource.DriverTerraform, resource.DriverKubernetes, resource.DriverEcho, resource.DriverScoreK8s:
	default:
		return invalid("definition.driverType")
	}
	if len(def.Criteria) == 0 || len(def.Criteria) > maxPreviewCriteria {
		return invalid("definition.criteria")
	}
	for _, c := range def.Criteria {
		for _, v := range []string{c.EnvironmentType, c.ApplicationID, c.EnvironmentID, c.ResourceID, c.Class} {
			if !boundedText(v) {
				return invalid("definition.criteria")
			}
		}
	}
	if !boundedText(def.ConnectionKey) {
		return invalid("definition.connectionKey")
	}
	return nil
}

// PreviewMatching computes the single-context matching of one unsaved
// Definition against a consistent catalog snapshot. It never writes.
func (s *Service) PreviewMatching(ctx context.Context, organizationKey string, q MatchingPreviewQuery) (MatchingPreview, error) {
	c := q.Context
	for field, v := range map[string]string{"context.applicationId": c.ApplicationID, "context.environmentId": c.EnvironmentID, "context.resourceId": c.ResourceID, "context.class": c.Class} {
		if !boundedText(v) {
			return MatchingPreview{}, invalid(field)
		}
	}
	if strings.TrimSpace(c.ApplicationID) == "" {
		return MatchingPreview{}, invalid("context.applicationId")
	}
	if strings.TrimSpace(c.EnvironmentID) == "" {
		return MatchingPreview{}, invalid("context.environmentId")
	}
	var out MatchingPreview
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		app, err := view.GetApplication(ctx, c.ApplicationID)
		if errors.Is(err, persistence.ErrNotFound) || (err == nil && app.OrganizationKey != organizationKey) {
			return ErrPreviewNotFound
		}
		if err != nil {
			return err
		}
		env, err := view.GetEnvironment(ctx, c.ApplicationID, c.EnvironmentID)
		if errors.Is(err, persistence.ErrNotFound) {
			return ErrPreviewNotFound
		}
		if err != nil {
			return err
		}
		// Same snapshot: READY Connection of the Organization, kind/region/scope consistency.
		conn, err := target.Resolve(ctx, view, organizationKey, env)
		if err != nil {
			if errors.Is(err, target.ErrUnconfigured) || errors.Is(err, target.ErrInconsistent) || errors.Is(err, target.ErrConnectionNotReady) || errors.Is(err, persistence.ErrNotFound) {
				return ErrTargetUnavailable
			}
			return err
		}
		typeList, err := view.ListResourceTypes(ctx, organizationKey)
		if err != nil {
			return err
		}
		types := map[string]resource.Type{}
		for _, t := range typeList {
			types[t.Key] = t
		}
		stored, err := view.ListResourceDefinitions(ctx, organizationKey)
		if err != nil {
			return err
		}
		// Marker covers the catalog and target before candidate replacement.
		// Sorted copies keep the hash independent of repository list order.
		sortedDefs := append([]resource.Definition(nil), stored...)
		resource.SortDefinitions(sortedDefs)
		sortedTypes := append([]resource.Type(nil), typeList...)
		sort.Slice(sortedTypes, func(i, j int) bool { return sortedTypes[i].Key < sortedTypes[j].Key })
		marker, err := canon.Hash(map[string]any{
			"definitions": sortedDefs, "types": sortedTypes,
			"environment": map[string]any{
				"app": env.ApplicationKey, "key": env.Key, "type": env.Type, "profile": env.Profile,
				"connection": env.ConnectionKey, "scope": env.Scope(), "generation": env.TargetGeneration,
				"version": env.Version, "region": env.Region, "runtimeStatus": env.RuntimeStatus,
			},
			// Non-secret identity of the resolved Connection; never its config or credentials.
			"connection": map[string]any{"key": conn.Key, "kind": conn.Kind, "status": conn.Status, "region": conn.ConfigString("region")},
		})
		if err != nil {
			return err
		}
		candidate := q.Definition
		if err := validatePreviewCandidate(candidate, types); err != nil {
			return err
		}
		candidate.SourceFingerpr = ""
		out = s.previewResult(env, candidate, stored, c)
		out.Snapshot = MatchingSnapshot{Marker: marker, EnvironmentVersion: env.Version}
		return nil
	})
	if err != nil {
		return MatchingPreview{}, err
	}
	return out, nil
}

func (s *Service) previewResult(env environment.Environment, candidate resource.Definition, stored []resource.Definition, c MatchingContext) MatchingPreview {
	out := MatchingPreview{MissingFields: []string{}, Candidates: []MatchingCandidate{}}
	if strings.TrimSpace(c.ResourceID) == "" {
		out.MissingFields = append(out.MissingFields, "resourceId")
	}
	if strings.TrimSpace(c.Class) == "" {
		out.MissingFields = append(out.MissingFields, "class")
	}
	typeKey := candidate.ResourceTypeKey

	// Replace only the same-key entry, in memory.
	defs := make([]resource.Definition, 0, len(stored)+1)
	replaced := false
	for _, d := range stored {
		if d.Key == planning.BuiltinClusterKey {
			continue // system admission copy never takes part in matching
		}
		if d.Key == candidate.Key {
			d, replaced = candidate, true
		}
		defs = append(defs, d)
	}
	if !replaced {
		defs = append(defs, candidate)
	}
	resource.SortDefinitions(defs)

	mctx := resource.MatchContext{EnvironmentType: env.Type, ApplicationID: c.ApplicationID, EnvironmentID: c.EnvironmentID, ResourceID: c.ResourceID, Class: c.Class}
	complete := len(out.MissingFields) == 0

	type scored struct {
		key   string
		def   resource.Definition
		score int
	}
	var best []scored
	bestScore := -1
	for _, d := range defs {
		if d.ResourceTypeKey != typeKey {
			continue
		}
		// Workload ranking only includes score-k8s renderers, as selectRendering.
		if typeKey == "workload" && d.DriverType != resource.DriverScoreK8s {
			continue
		}
		cand := MatchingCandidate{Key: d.Key, Eligible: d.ExecutionProfile == "" || d.ExecutionProfile == string(env.Profile)}
		if cand.Eligible && complete {
			if crit, score, ok := d.BestCriterion(mctx); ok {
				cand.Matched = true
				crit := crit
				score := score
				cand.BestCriterion, cand.Score = &crit, &score
				if score > bestScore {
					bestScore, best = score, nil
				}
				if score == bestScore {
					best = append(best, scored{key: d.Key, def: d, score: score})
				}
			}
		}
		out.Candidates = append(out.Candidates, cand)
	}

	if !complete {
		out.Status = PreviewIncomplete
		return out
	}
	// Only the exact implicit internal cluster node is a system binding.
	if env.Profile == application.ProfileInternalK8s && typeKey == planning.TypeCluster && c.Class == planning.ClassInternal {
		if want, err := planning.InternalClusterDescriptor(env.ConnectionKey); err == nil && c.ResourceID == want.ID {
			// The system binding bypasses authored Definitions: no candidate is shown as matched.
			for i := range out.Candidates {
				out.Candidates[i].Matched, out.Candidates[i].BestCriterion, out.Candidates[i].Score = false, nil, nil
			}
			out.Status = PreviewSystemBinding
			return out
		}
	}
	switch len(best) {
	case 0:
		// A Workload with no score-k8s match is NO_MATCH like any other type:
		// there is no native/default renderer to fall back to (UC-03 BR-21).
		out.Status = PreviewNoMatch
	case 1:
		out.Winner = &MatchingWinner{Key: best[0].key, Score: best[0].score}
		out.Status = PreviewMatched
		if planning.ConnectionMismatch(env, best[0].def) {
			out.Status = PreviewConnectionConflict
			out.ConnectionConflict = &MatchingConflict{DefinitionKey: best[0].key, ConnectionKey: best[0].def.ConnectionKey, TargetKey: env.ConnectionKey}
		}
	default:
		out.Status = PreviewAmbiguous
		for _, b := range best {
			out.Ties = append(out.Ties, b.key)
		}
		sort.Strings(out.Ties)
	}
	return out
}
