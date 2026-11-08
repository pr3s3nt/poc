package pending

import (
	"context"
	"encoding/json"
	"fmt"

	deliveryapp "orchestrator/internal/application/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
)

// ReleaseSnapshot is private execution metadata. Operation HTTP projections must
// never expose its Definition inputs or secret/configuration references.
// Terminal operation details are immutable; the referenced Deployment Set is too.
type ReleaseSnapshot struct {
	Digest           string                 `json:"digest"`
	Version          int                    `json:"version"`
	OperationID      string                 `json:"operationId"`
	DeploymentSetID  string                 `json:"deploymentSetId"`
	DocumentHash     string                 `json:"documentHash"`
	ConfigRevisionID string                 `json:"configRevisionId,omitempty"`
	Definitions      []resource.Definition  `json:"definitions"`
	CatalogPin       string                 `json:"catalogPin,omitempty"`
	Delivery         deliveryapp.PreviewPin `json:"delivery"`
	Binding          environment.Binding    `json:"binding"`
	SecretStoreKey   string                 `json:"secretStoreKey,omitempty"`
}

// LastSuccessfulRelease reads only completed whole-Environment Deploy batches.
// Per-workload successes and interrupted/partial batches are never promoted.
func (s *Service) LastSuccessfulRelease(ctx context.Context, appKey, envKey string) (ReleaseSnapshot, error) {
	var result ReleaseSnapshot
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, st persistence.Store) error {
		ops, err := st.ListOperations(ctx, appKey, envKey, 1000)
		if err != nil {
			return err
		}
		env, err := st.GetEnvironment(ctx, appKey, envKey)
		if err != nil {
			return err
		}
		for _, op := range ops {
			if op.ApplicationKey != appKey || op.EnvironmentKey != envKey || op.Kind != environment.OpDeploy || op.Status != environment.OpSucceeded {
				continue
			}
			raw, ok := op.Detail["releaseSnapshot"]
			if !ok {
				return fmt.Errorf("%w: successful deployment has no Environment release snapshot", ErrInvalid)
			}
			data, err := json.Marshal(raw)
			if err != nil {
				return fmt.Errorf("%w: invalid release metadata", ErrInvalid)
			}
			var snapshot ReleaseSnapshot
			if err := json.Unmarshal(data, &snapshot); err != nil || snapshot.Version != 1 || snapshot.OperationID != op.ID || snapshot.DeploymentSetID == "" || snapshot.DocumentHash == "" {
				return fmt.Errorf("%w: invalid release metadata", ErrInvalid)
			}
			wantDigest := snapshot.Digest
			snapshot.Digest = ""
			digest, err := canon.Hash(snapshot)
			if err != nil || wantDigest == "" || digest != wantDigest {
				return fmt.Errorf("%w: release metadata digest mismatch", ErrInvalid)
			}
			snapshot.Digest = wantDigest
			set, err := st.GetDeploymentSet(ctx, snapshot.DeploymentSetID)
			if err != nil {
				return err
			}
			hash, err := canon.Hash(set.Document)
			if err != nil {
				return err
			}
			if set.EnvironmentID != env.ID || hash != snapshot.DocumentHash {
				return fmt.Errorf("%w: release snapshot identity mismatch", ErrInvalid)
			}
			result = snapshot
			return nil
		}
		return persistence.ErrNotFound
	})
	return result, err
}

func (s *Service) captureRelease(ctx context.Context, operationID string, preview Preview) (ReleaseSnapshot, error) {
	env, err := s.store.GetEnvironment(ctx, preview.ApplicationKey, preview.EnvironmentKey)
	if err != nil {
		return ReleaseSnapshot{}, err
	}
	set, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil {
		return ReleaseSnapshot{}, err
	}
	if set.EnvironmentID != env.ID {
		return ReleaseSnapshot{}, ErrInvalid
	}
	hash, err := canon.Hash(set.Document)
	if err != nil {
		return ReleaseSnapshot{}, err
	}
	snapshot := ReleaseSnapshot{Version: 1, OperationID: operationID,
		DeploymentSetID: set.ID, DocumentHash: hash, ConfigRevisionID: preview.ConfigRevisionID,
		Definitions: preview.DefinitionSnapshots, CatalogPin: preview.CatalogPin,
		Delivery: preview.DeliveryBindingPin, Binding: preview.Binding, SecretStoreKey: preview.SecretStoreKey}
	snapshot.Digest, err = canon.Hash(snapshot)
	return snapshot, err
}
