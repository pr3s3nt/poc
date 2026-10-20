package catalog

import (
	"context"
	"errors"
	"sort"
	"strings"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
)

// Matching preview statuses (UC-03 V02-05, OC-04c). PreviewConnectionConflict
// keeps its established wire name for a winner whose Driver Account or binding
// is unusable.
const (
	PreviewIncomplete         = "INCOMPLETE"
	PreviewMatched            = "MATCHED"
	PreviewNoMatch            = "NO_MATCH"
	PreviewAmbiguous          = "AMBIGUOUS"
	PreviewConnectionConflict = "CONNECTION_CONFLICT"
)

// Safe winner-account conflict reasons. They are fixed codes, never text from
// the account or its credentials.
const (
	ConflictAccountMissing      = "ACCOUNT_MISSING"
	ConflictAccountNotFound     = "ACCOUNT_NOT_FOUND"
	ConflictAccountKindMismatch = "ACCOUNT_KIND_MISMATCH"
	ConflictAccountNotReady     = "ACCOUNT_NOT_READY"
	ConflictRegionInvalid       = "REGION_INVALID"
)

// Cluster source kinds shown for a Kubernetes resource binding.
const (
	ClusterSourceResource = "RESOURCE"
)

// Bounds keep one preview request cheap and its response small.
const (
	maxPreviewCriteria    = 64
	maxPreviewContextText = 253
)

// ErrPreviewNotFound reports an Application or Environment outside the
// Organization (HTTP 404).
var ErrPreviewNotFound = errors.New("catalog: application or environment not found")

// MatchingContext is the explicit client context. Organization and the
// environment type always come from the server snapshot.
type MatchingContext struct {
	ApplicationID string `json:"applicationId"`
	EnvironmentID string `json:"environmentId"`
	ResourceID    string `json:"resourceId"`
	Class         string `json:"class"`
}

// DefinitionDraft is the strict public authoring document of a Definition
// (create, edit and matching preview). The version, source fingerprint and any
// retired field are not part of it.
type DefinitionDraft struct {
	Key             string                            `json:"key"`
	Name            string                            `json:"name"`
	ResourceTypeKey string                            `json:"resourceType"`
	DriverType      resource.DriverType               `json:"driverType"`
	DriverAccount   string                            `json:"driverAccount,omitempty"`
	DriverInputs    map[string]any                    `json:"driverInputs,omitempty"`
	Provision       map[string]resource.ProvisionRule `json:"provision,omitempty"`
	Criteria        []resource.Criterion              `json:"criteria"`
}

// Definition converts the draft to a Definition without a version.
func (d DefinitionDraft) Definition() resource.Definition {
	return resource.Definition{Key: d.Key, Name: d.Name, ResourceTypeKey: d.ResourceTypeKey, DriverType: d.DriverType,
		DriverAccount: d.DriverAccount, DriverInputs: d.DriverInputs, Provision: d.Provision, Criteria: d.Criteria}
}

// DefinitionUpdate is the strict edit document: the draft plus the expected
// stored version (UC-03 V02-06).
type DefinitionUpdate struct {
	DefinitionDraft
	Version int64 `json:"version"`
}

// MatchingPreviewQuery is one unsaved candidate plus its context.
type MatchingPreviewQuery struct {
	Definition DefinitionDraft `json:"definition"`
	Context    MatchingContext `json:"context"`
}

// MatchingCandidate is the safe per-Definition result.
type MatchingCandidate struct {
	Key           string              `json:"key"`
	Name          string              `json:"name"`
	Matched       bool                `json:"matched"`
	BestCriterion *resource.Criterion `json:"bestCriterion,omitempty"`
	Score         *int                `json:"score,omitempty"`
}

// MatchingAccount is the safe identity of a selected Driver Account.
type MatchingAccount struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Version int64  `json:"version"`
}

// MatchingClusterSource names where a Kubernetes binding inherits its cluster.
type MatchingClusterSource struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference,omitempty"`
}

// MatchingWinner identifies the winning Definition and what it will run with.
type MatchingWinner struct {
	Key           string                 `json:"key"`
	Name          string                 `json:"name"`
	Version       int64                  `json:"version,omitempty"`
	DriverType    string                 `json:"driverType"`
	Score         int                    `json:"score"`
	Account       *MatchingAccount       `json:"account,omitempty"`
	Region        string                 `json:"region,omitempty"`
	ClusterSource *MatchingClusterSource `json:"clusterSource,omitempty"`
}

// MatchingConflict reports a winner whose Driver Account or binding is unusable.
type MatchingConflict struct {
	DefinitionKey string `json:"definitionKey"`
	DriverAccount string `json:"driverAccount,omitempty"`
	Reason        string `json:"reason"`
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
// driver contract, account, bundle or placeholder inspection. Errors name the
// field and never echo submitted values.
func validatePreviewCandidate(def resource.Definition, types map[string]resource.Type) error {
	if def.Key == planning.BuiltinClusterKey {
		return invalid("definition.key")
	}
	if validatePublicID("resource definition", def.Key) != nil || len(def.Key) > maxPreviewContextText {
		return invalid("definition.key")
	}
	// A partial draft may still be unnamed; a given Name follows the same
	// Unicode bound and control rule as registration.
	if strings.TrimSpace(def.Name) != "" && resource.ValidateName(def.Name) != nil {
		return invalid("definition.name")
	}
	if _, ok := types[def.ResourceTypeKey]; !ok {
		return invalid("definition.resourceType")
	}
	switch def.DriverType {
	case resource.DriverTerraform, resource.DriverScoreK8s, resource.DriverExistingCluster:
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
	if !boundedText(def.DriverAccount) {
		return invalid("definition.driverAccount")
	}
	return nil
}

// PreviewMatching computes the single-context matching of one unsaved
// Definition against a consistent catalog snapshot. It never writes and does
// not need a configured Environment target: the five-field ranking uses only
// the scoped Environment type and the explicit context.
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
		accountList, err := view.ListConnections(ctx, organizationKey)
		if err != nil {
			return err
		}
		accounts := map[string]application.Connection{}
		safeAccounts := make([]map[string]any, 0, len(accountList))
		for _, a := range accountList {
			accounts[a.Key] = a
			// Non-secret identity only; never configuration or credentials.
			safeAccounts = append(safeAccounts, map[string]any{"key": a.Key, "name": a.Name, "kind": a.Kind, "status": a.Status, "version": a.Version})
		}
		sort.Slice(safeAccounts, func(i, j int) bool { return safeAccounts[i]["key"].(string) < safeAccounts[j]["key"].(string) })
		// Marker covers the catalog, accounts and context before candidate
		// replacement. Sorted copies keep the hash independent of list order.
		sortedDefs := append([]resource.Definition(nil), stored...)
		resource.SortDefinitions(sortedDefs)
		sortedTypes := append([]resource.Type(nil), typeList...)
		sort.Slice(sortedTypes, func(i, j int) bool { return sortedTypes[i].Key < sortedTypes[j].Key })
		marker, err := canon.Hash(map[string]any{
			"definitions": sortedDefs, "types": sortedTypes, "accounts": safeAccounts,
			"environment": map[string]any{
				"app": env.ApplicationKey, "key": env.Key, "type": env.Type,
				"generation": env.TargetGeneration, "version": env.Version,
			},
		})
		if err != nil {
			return err
		}
		candidate := q.Definition.Definition()
		candidate.Version = 0
		if err := validatePreviewCandidate(candidate, types); err != nil {
			return err
		}
		out = s.previewResult(env, candidate, stored, accounts, c)
		out.Snapshot = MatchingSnapshot{Marker: marker, EnvironmentVersion: env.Version}
		return nil
	})
	if err != nil {
		return MatchingPreview{}, err
	}
	return out, nil
}

func (s *Service) previewResult(env environment.Environment, candidate resource.Definition, stored []resource.Definition, accounts map[string]application.Connection, c MatchingContext) MatchingPreview {
	out := MatchingPreview{MissingFields: []string{}, Candidates: []MatchingCandidate{}}
	if strings.TrimSpace(c.ResourceID) == "" {
		out.MissingFields = append(out.MissingFields, "resourceId")
	}
	if strings.TrimSpace(c.Class) == "" {
		out.MissingFields = append(out.MissingFields, "class")
	}
	typeKey := candidate.ResourceTypeKey

	// Replace only the same-key entry, in memory. The candidate keeps the
	// stored version of the entry it replaces so the winner reports it.
	defs := make([]resource.Definition, 0, len(stored)+1)
	replaced := false
	for _, d := range stored {
		if d.Key == planning.BuiltinClusterKey {
			continue // system admission copy never takes part in matching
		}
		if d.Key == candidate.Key {
			candidate.Version = d.Version
			d, replaced = candidate, true
		}
		defs = append(defs, d)
	}
	if !replaced {
		defs = append(defs, candidate)
	}

	mctx := resource.MatchContext{EnvironmentType: env.Type, ApplicationID: c.ApplicationID, EnvironmentID: c.EnvironmentID, ResourceID: c.ResourceID, Class: c.Class}
	complete := len(out.MissingFields) == 0
	// Workload ranking only includes score-k8s renderers, as selectRendering.
	include := func(d resource.Definition) bool {
		return typeKey != "workload" || d.DriverType == resource.DriverScoreK8s
	}
	ranking := resource.Rank(defs, typeKey, mctx, include)
	byKey := map[string]resource.Ranked{}
	for _, r := range ranking.Matched {
		byKey[r.Definition.Key] = r
	}
	ordered := append([]resource.Definition(nil), defs...)
	resource.SortDefinitions(ordered)
	for _, d := range ordered {
		if d.ResourceTypeKey != typeKey || !include(d) {
			continue
		}
		cand := MatchingCandidate{Key: d.Key, Name: d.Name}
		if r, ok := byKey[d.Key]; ok && complete {
			crit, score := r.Criterion, r.Score
			cand.Matched, cand.BestCriterion, cand.Score = true, &crit, &score
		}
		out.Candidates = append(out.Candidates, cand)
	}

	if !complete {
		out.Status = PreviewIncomplete
		return out
	}
	switch len(ranking.Top) {
	case 0:
		// A Workload with no score-k8s match is NO_MATCH like any other type:
		// there is no native/default renderer to fall back to (UC-03 V02-02).
		out.Status = PreviewNoMatch
	case 1:
		win := ranking.Top[0]
		out.Status = PreviewMatched
		out.Winner = winnerOf(win, accounts)
		if conflict := accountConflict(win.Definition, accounts); conflict != nil {
			out.Status, out.ConnectionConflict = PreviewConnectionConflict, conflict
		}
	default:
		out.Status = PreviewAmbiguous
		for _, t := range ranking.Top {
			out.Ties = append(out.Ties, t.Definition.Key)
		}
		sort.Strings(out.Ties)
	}
	return out
}

// bindingTargets reads the provider binding targets of a Terraform Definition
// without failing: structural problems leave the result empty and are reported
// by registration and planning instead.
func bindingTargets(def resource.Definition) []tfmodule.Binding {
	if def.DriverType != resource.DriverTerraform {
		return nil
	}
	raw, _ := def.DriverValues()["provider_bindings"].(map[string]any)
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]tfmodule.Binding, 0, len(names))
	for _, name := range names {
		object, _ := raw[name].(map[string]any)
		target, _ := object["target"].(string)
		ref, _ := object["resource_ref"].(string)
		out = append(out, tfmodule.Binding{Target: target, ResourceRef: ref})
	}
	return out
}

// needsAccount reports whether def binds an AWS provider to its Driver Account.
func needsAccount(def resource.Definition) bool {
	if def.DriverType == resource.DriverExistingCluster {
		return true // the Definition's own Internal Driver Account (ADR-016)
	}
	for _, b := range bindingTargets(def) {
		if b.Target == tfmodule.TargetAccount {
			return true
		}
	}
	return false
}

func accountType(kind application.ConnectionKind) string {
	if kind == application.ConnectionKubernetes {
		return "INTERNAL"
	}
	return "AWS"
}

// winnerOf projects the safe public view of the winner: Name, selected account
// identity and effective region, or the inherited cluster source. It carries
// no credential, secret reference, endpoint, raw configuration or diagnostic.
func winnerOf(r resource.Ranked, accounts map[string]application.Connection) *MatchingWinner {
	def := r.Definition
	w := &MatchingWinner{Key: def.Key, Name: def.Name, Version: def.Version, DriverType: string(def.DriverType), Score: r.Score}
	if needsAccount(def) {
		if acct, ok := accounts[def.DriverAccount]; ok {
			w.Account = &MatchingAccount{Key: acct.Key, Name: acct.Name, Type: accountType(acct.Kind), Version: acct.Version}
		}
		if region, ok := def.DefinitionRegion(); ok && resource.ValidAWSRegion(region) && def.DriverType != resource.DriverExistingCluster {
			w.Region = region
		}
	}
	for _, b := range bindingTargets(def) {
		switch b.Target {
		case tfmodule.TargetResource:
			w.ClusterSource = &MatchingClusterSource{Kind: ClusterSourceResource, Reference: b.ResourceRef}
		}
	}
	return w
}

// accountConflict validates the unique winner's Driver Account and region. A
// conflict is reported as is; no other candidate replaces the winner.
func accountConflict(def resource.Definition, accounts map[string]application.Connection) *MatchingConflict {
	if !needsAccount(def) {
		return nil
	}
	conflict := func(reason string) *MatchingConflict {
		return &MatchingConflict{DefinitionKey: def.Key, DriverAccount: def.DriverAccount, Reason: reason}
	}
	if def.DriverAccount == "" {
		return conflict(ConflictAccountMissing)
	}
	acct, ok := accounts[def.DriverAccount]
	switch {
	case !ok:
		return conflict(ConflictAccountNotFound)
	case acct.Kind != wantedKind(def):
		return conflict(ConflictAccountKindMismatch)
	case acct.Status != application.ConnectionReady:
		return conflict(ConflictAccountNotReady)
	}
	if def.DriverType == resource.DriverExistingCluster {
		return nil
	}
	if region, ok := def.DefinitionRegion(); !ok || !resource.ValidAWSRegion(region) {
		return conflict(ConflictRegionInvalid)
	}
	return nil
}

// wantedKind is the Driver Account kind a Definition's driver needs.
func wantedKind(def resource.Definition) application.ConnectionKind {
	if def.DriverType == resource.DriverExistingCluster {
		return application.ConnectionKubernetes
	}
	return application.ConnectionAWS
}
