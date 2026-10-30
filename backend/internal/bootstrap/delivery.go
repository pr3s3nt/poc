package bootstrap

import (
	"fmt"

	directadapter "orchestrator/internal/adapters/delivery/direct"
	fleetadapter "orchestrator/internal/adapters/delivery/fleet"
	gitadapter "orchestrator/internal/adapters/gitpublication"
	k8s "orchestrator/internal/adapters/kubernetes"
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
	directDeployer := k8s.NewDeployer(opts.KubectlPath)
	directDeployer.Credentials = source
	direct := &directadapter.Provider{Deployer: directDeployer, KubectlPath: opts.KubectlPath, Credentials: source, Guard: coordinator.Guard, RecoveryGuard: coordinator.GuardRecovery}
	if err := registry.Register("direct", direct); err != nil {
		return nil, nil, err
	}
	provider := &fleetadapter.Provider{Manager: &fleetadapter.Manager{Verifier: &fleetadapter.Verifier{KubectlPath: opts.KubectlPath, Credentials: source, Targets: appdelivery.Targets{Store: st, Identity: repo}}, Guard: coordinator.Guard, RecoveryGuard: coordinator.GuardRecovery}, Publishers: publishers}
	if err := registry.Register("fleet", provider); err != nil {
		return nil, nil, err
	}
	return service, coordinator, nil
}
