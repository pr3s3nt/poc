package bootstrap

import (
	"context"
	"fmt"
	"orchestrator/internal/application/gitaccess"
	"strings"

	fleetadapter "orchestrator/internal/adapters/delivery/fleet"
	gitadapter "orchestrator/internal/adapters/gitpublication"
	githubhost "orchestrator/internal/adapters/repositoryhosting/github"
	appdelivery "orchestrator/internal/application/delivery"
	"orchestrator/internal/ports/credentials"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

type deliveryStore interface {
	persistence.Store
	persistence.DeliveryBindingRepository
	persistence.DeliverySelectionRepository
	persistence.DeliveryPlatformRepository
	persistence.DeliveryExecutionRepository
	persistence.GitPublicationRepository
	persistence.OrganizationIdentityReader
}

func setupDelivery(opts Options, st persistence.Store, source execution.KubeconfigSource, creds credentials.Store, tf *terraformRuntime) (*appdelivery.Bindings, *appdelivery.Coordinator, error) {
	repo, ok := st.(deliveryStore)
	if !ok {
		return nil, nil, nil
	}
	registry := port.NewRegistry()
	service := appdelivery.NewBindings(repo, registry)
	service.SetCredentials(creds)
	if opts.DeliveryGitRoot == "" {
		return service, nil, nil
	}
	if tf == nil || tf.repo == nil || tf.cipher == nil || source == nil || creds == nil || !creds.Durable() {
		return nil, nil, fmt.Errorf("bootstrap: optional CD delivery requires durable state, encryption, scoped Kubernetes and credential storage")
	}
	publishers, err := gitadapter.NewFactory(opts.DeliveryGitRoot, creds, repo)
	if err != nil {
		return nil, nil, fmt.Errorf("bootstrap: delivery Git storage unavailable")
	}
	coordinator := &appdelivery.Coordinator{Store: repo, Registry: registry, Artifacts: &appdelivery.Artifacts{Store: tf.repo, Cipher: tf.cipher}}
	hosts := &githubhost.Factory{Credentials: creds, Organizations: repo}
	coordinator.Repositories = &appdelivery.Repositories{Store: repo, Hosts: hosts}
	provider := &fleetadapter.Provider{Manager: &fleetadapter.Manager{Verifier: &fleetadapter.Verifier{KubectlPath: opts.KubectlPath, Credentials: source, Targets: appdelivery.Targets{Store: st, Identity: repo}}, Guard: coordinator.Guard, RecoveryGuard: coordinator.GuardRecovery}, Publishers: publishers, RepositoryHosts: hosts, StuckThreshold: opts.StuckPodThreshold}
	provider.Manager.ReaderCredentials = func(ctx context.Context, b port.BindingVersion) ([]byte, error) {
		org, err := repo.GetOrganizationByID(ctx, b.OrganizationID)
		if err != nil {
			return nil, err
		}
		if b.WriterCredentialRef == gitaccess.PlatformWriterReference {
			settings, ok := st.(persistence.GitSettingsRepository)
			if !ok {
				return nil, gitaccess.ErrUnavailable
			}
			raw, _, err := gitaccess.Writer(ctx, settings, creds, org.Key, b.Config["repositoryURL"])
			return raw, err
		}
		if strings.HasPrefix(b.WriterCredentialRef, gitaccess.ConnectionWriterPrefix) {
			return gitaccess.ConnectionCredential(ctx, st, creds, org.Key, b.WriterCredentialRef, b.Config["repositoryURL"])
		}
		return creds.Get(ctx, org.Key, b.ID, b.WriterCredentialRef)
	}
	if err := registry.Register("fleet", provider); err != nil {
		return nil, nil, err
	}
	return service, coordinator, nil
}
