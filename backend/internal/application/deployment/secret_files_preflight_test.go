package deployment

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/persistence"
)

type fileRevisionStore struct {
	persistence.Store
	revision configuration.Revision
}

func (s fileRevisionStore) GetConfigurationRevision(context.Context, string) (configuration.Revision, error) {
	return s.revision, nil
}

func TestFileOnlySecretConsumerFailsPreflightWithoutDeliveryOrPinnedRevision(t *testing.T) {
	plan := &planning.Plan{CandidateSet: environment.NewDocument()}
	plan.CandidateSet.Modules["app"] = environment.Module{Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{SecretFiles: []environment.SecretFile{{Ref: "${context.uc12.GPG}", Container: "main", Path: "/keys/private", Mode: 0400}}}}}
	svc := &Service{store: fileRevisionStore{revision: configuration.Revision{Entries: map[string]configuration.Entry{"GPG": {Kind: configuration.Secret, StoreKey: "vault", ValueRef: "opaque"}}}}}
	// No env variable references are present; file-only intent must be sufficient.
	if err := svc.preflightSecretDelivery(context.Background(), DeployCommand{}, environment.Environment{}, plan, "app", true); !errors.Is(err, configport.ErrNoStore) {
		t.Fatalf("missing pinned revision admitted: %v", err)
	}
	if err := svc.preflightSecretDelivery(context.Background(), DeployCommand{ConfigRevisionID: "revision"}, environment.Environment{}, plan, "app", true); !errors.Is(err, environment.ErrExecutionUnavailable) {
		t.Fatalf("missing VSO delivery admitted: %v", err)
	}
}
