package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

// ErrResetRequired means the snapshot is not the current complete format. It is
// never converted, defaulted or rewritten; the development state must be reset.
// The text carries no snapshot values.
var ErrResetRequired = errors.New("store: snapshot is not the current format; reset the development state (snapshot conversion is unsupported)")

func resetRequired(reason string) error { return fmt.Errorf("%w: %s", ErrResetRequired, reason) }

// requiredSnapshotKeys are every aggregate map the current serializer writes.
// Each must be present and a JSON object; an empty object is valid.
var requiredSnapshotKeys = []string{
	"organizations", "userAccounts", "sessions", "applications", "connections", "environments",
	"deploymentSets", "resourceTypes", "resourceDefinitions", "deployments", "deploymentDeltaSnapshots",
	"deploymentPlans", "deploymentResources", "activeResources", "workloadInstances", "deploymentWorkloads",
	"configurationScopes", "configurationRevisions", "workloadDrafts", "secretStores",
	"environmentOperations", "environmentTransitions", "terraformModuleBundles",
}

// loadSnapshot parses the current format only. Missing or null aggregate maps,
// unknown or retired fields and invalid current metadata are rejected without
// quoting values. No default is initialized, no Connection metadata is
// inferred and no workload history is reconstructed.
func loadSnapshot(b []byte) (*state, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil || fields == nil {
		return nil, resetRequired("snapshot is not a JSON object")
	}
	for _, key := range requiredSnapshotKeys {
		raw, ok := fields[key]
		if !ok {
			return nil, resetRequired("required aggregate " + key + " is missing")
		}
		if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
			return nil, resetRequired("aggregate " + key + " is not an object")
		}
	}
	loaded := newState()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(loaded); err != nil {
		// Decoder errors name fields and may quote values; report neither.
		return nil, resetRequired("snapshot holds unknown, retired or malformed fields")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, resetRequired("snapshot has trailing data")
	}
	if err := loaded.validateCurrent(); err != nil {
		return nil, err
	}
	return loaded, nil
}

// validateCurrent checks the required identity, ownership and pointer
// integrity of the loaded aggregates plus the current binding, authentication
// and configuration metadata. Raw plan/Score documents stay untyped. Errors
// name no values.
func (s *state) validateCurrent() error {
	for key, org := range s.Organizations {
		if org.ID == "" || org.Key == "" || org.Name == "" || key != org.Key {
			return resetRequired("an Organization record is incomplete")
		}
	}
	for key, app := range s.Applications {
		if app.ID == "" || app.Key == "" || app.Name == "" || key != app.Key {
			return resetRequired("an Application record is incomplete")
		}
		if _, ok := s.Organizations[app.OrganizationKey]; !ok {
			return resetRequired("an Application has no Organization")
		}
	}
	for key, def := range s.Definitions {
		org, _, ok := strings.Cut(key, "/")
		if !ok || def.Key == "" || catalogKey(org, def.Key) != key {
			return resetRequired("a Resource Definition record is incomplete")
		}
		if _, ok := s.Organizations[org]; !ok {
			return resetRequired("a Resource Definition has no Organization")
		}
		if resource.ValidateName(def.Name) != nil {
			return resetRequired("a Resource Definition lacks an explicit current name")
		}
		if def.Version < 1 {
			return resetRequired("a Resource Definition has no positive version")
		}
		if def.DriverAccount != "" {
			if _, ok := s.Connections[catalogKey(org, def.DriverAccount)]; !ok {
				return resetRequired("a Resource Definition names no Driver Account of its Organization")
			}
		}
	}
	for key, conn := range s.Connections {
		if conn.ID == "" || conn.Key == "" || key != catalogKey(conn.OrganizationKey, conn.Key) {
			return resetRequired("a Connection record is incomplete")
		}
		if _, ok := s.Organizations[conn.OrganizationKey]; !ok {
			return resetRequired("a Connection has no Organization")
		}
		if err := conn.Validate(); err != nil {
			return resetRequired("a Connection lacks explicit current name or authentication metadata")
		}
		if err := conn.ValidateStored(); err != nil {
			return resetRequired("a Connection has no positive version")
		}
	}
	for key, store := range s.SecretStores {
		if key != catalogKey(store.OrganizationKey, store.Key) {
			return resetRequired("a Secret Store record is misplaced")
		}
		if _, ok := s.Organizations[store.OrganizationKey]; !ok {
			return resetRequired("a Secret Store has no Organization")
		}
		if err := store.Validate(); err != nil {
			return resetRequired("a Secret Store record is invalid")
		}
	}
	for key, env := range s.Environments {
		if err := s.validEnvironment(key, env); err != nil {
			return err
		}
	}
	for key, set := range s.DeploymentSets {
		if set.ID == "" || key != set.ID {
			return resetRequired("a Deployment Set record is incomplete")
		}
	}
	for key, d := range s.Deployments {
		if d.ID == "" || key != d.ID {
			return resetRequired("a Deployment record is incomplete")
		}
	}
	for key, bundle := range s.ModuleBundles {
		if bundle.Validate() != nil || key != bundle.Ref.Pin() {
			return resetRequired("a Terraform module bundle record is invalid")
		}
	}
	for id := range s.DeltaSnapshots {
		if _, ok := s.Deployments[s.DeltaSnapshots[id].DeploymentID]; !ok {
			return resetRequired("a Deployment snapshot has no Deployment")
		}
	}
	for _, snapshot := range s.DeploymentWorkloads {
		if _, ok := s.Deployments[snapshot.DeploymentID]; !ok {
			return resetRequired("a workload history row has no Deployment")
		}
	}
	for _, w := range s.WorkloadInstances {
		if _, ok := s.Deployments[w.LastDeploymentID]; !ok {
			return resetRequired("a Workload Instance has no Deployment")
		}
	}
	for key, revision := range s.ConfigRevisions {
		if key != revision.ID {
			return resetRequired("a configuration revision is misplaced")
		}
		if err := revision.Validate(); err != nil {
			return resetRequired("a configuration revision is invalid")
		}
		if _, ok := s.Environments[envKey(revision.ApplicationKey, revision.EnvironmentKey)]; !ok {
			return resetRequired("a configuration revision has no Environment")
		}
	}
	for key, scope := range s.ConfigScopes {
		if key != envKey(scope.ApplicationKey, scope.EnvironmentKey) {
			return resetRequired("a configuration scope is misplaced")
		}
		revision, ok := s.ConfigRevisions[scope.DesiredRevisionID]
		if !ok || revision.ApplicationKey != scope.ApplicationKey || revision.EnvironmentKey != scope.EnvironmentKey || revision.Version != scope.Version {
			return resetRequired("a configuration scope does not point at its own revision")
		}
	}
	for key, op := range s.Operations {
		if op.ID == "" || key != op.ID {
			return resetRequired("an Environment operation record is incomplete")
		}
		if _, ok := s.Environments[envKey(op.ApplicationKey, op.EnvironmentKey)]; !ok {
			return resetRequired("an Environment operation has no Environment")
		}
	}
	for key, t := range s.Transitions {
		if t.ID == "" || key != t.ID {
			return resetRequired("an Environment transition record is incomplete")
		}
	}
	return nil
}

// validEnvironment checks one Environment row: identity, ownership by an
// existing Application, current binding metadata and every pointer it holds.
func (s *state) validEnvironment(key string, env environment.Environment) error {
	if env.ID == "" || env.Key == "" || env.NamespaceIdentity == "" || key != envKey(env.ApplicationKey, env.Key) {
		return resetRequired("an Environment record is incomplete")
	}
	app, ok := s.Applications[env.ApplicationKey]
	if !ok || (env.ApplicationID != "" && env.ApplicationID != app.ID) {
		return resetRequired("an Environment has no owning Application")
	}
	if env.InfrastructureScope != environment.ScopeEnvironment {
		return fmt.Errorf("%w: an Environment lacks the ENVIRONMENT infrastructure scope: %w", ErrResetRequired, environment.ErrInvalidInfrastructureScope)
	}
	if err := validEnvironmentBinding(env); err != nil {
		return err
	}
	if env.ConnectionKey != "" {
		if _, ok := s.Connections[catalogKey(app.OrganizationKey, env.ConnectionKey)]; !ok {
			return resetRequired("an Environment binding names no Connection of its Organization")
		}
	}
	if env.SecretStoreKey != "" {
		if _, ok := s.SecretStores[catalogKey(app.OrganizationKey, env.SecretStoreKey)]; !ok {
			return resetRequired("an Environment selects no Secret Store of its Organization")
		}
	}
	if env.CurrentDeploymentSetID != "" {
		if _, ok := s.DeploymentSets[env.CurrentDeploymentSetID]; !ok {
			return resetRequired("an Environment points at a missing Deployment Set")
		}
	}
	if env.ActiveOperationID != "" {
		op, ok := s.Operations[env.ActiveOperationID]
		if !ok || !op.Status.Holds() || envKey(op.ApplicationKey, op.EnvironmentKey) != key {
			return resetRequired("an Environment claim does not match a holding operation")
		}
	}
	return nil
}

// validEnvironmentBinding mirrors the SQL binding consistency constraint.
func validEnvironmentBinding(env environment.Environment) error {
	if env.TargetGeneration < 0 || env.GenerationHigh < 0 {
		return resetRequired("an Environment has a negative target generation")
	}
	if env.ConnectionKey == "" {
		if env.RuntimeStatus != application.RuntimeUnconfigured || env.Profile != "" || env.Region != "" {
			return resetRequired("an unconfigured Environment carries binding data")
		}
		return nil
	}
	return resetRequired("an Environment carries a retired deployment binding")
}
