package deployment_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"orchestrator/internal/adapters/templatedriver"
	"orchestrator/internal/renderengine"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/fake"
	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/statecipher"
	deliveryapp "orchestrator/internal/application/delivery"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/provisioning"
	"orchestrator/internal/bootstrap"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

type cdStore interface {
	persistence.Store
	persistence.DeliveryBindingRepository
	persistence.DeliverySelectionRepository
	persistence.DeliveryExecutionRepository
	persistence.GitPublicationRepository
	persistence.OrganizationIdentityReader
	persistence.DeliveryArtifactRepository
}

type captureCD struct {
	manifests []execution.Manifest
	removed   int
}

func (*captureCD) Describe(context.Context, port.BindingVersion) (domain.Capabilities, error) {
	return domain.Capabilities{}, nil
}
func (*captureCD) Verify(context.Context, port.BindingVersion) (port.Verification, error) {
	return port.Verification{}, nil
}
func (*captureCD) Prepare(_ context.Context, i port.Intent, m []execution.Manifest) (port.Prepared, error) {
	raw, err := json.Marshal(m)
	sum := sha256.Sum256(raw)
	r := i.Revision
	r.PayloadDigest = hex.EncodeToString(sum[:])
	return port.Prepared{Revision: r, Payload: raw}, err
}
func (p *captureCD) Submit(_ context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	var m []execution.Manifest
	if err := json.Unmarshal(prepared.Payload, &m); err != nil {
		return port.Receipt{}, err
	}
	p.manifests = append(p.manifests, m...)
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: i.Revision.PayloadDigest, Handle: map[string]string{"manifests": string(prepared.Payload)}}, nil
}
func (*captureCD) Observe(_ context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	var m []execution.Manifest
	if err := json.Unmarshal([]byte(r.Handle["manifests"]), &m); err != nil {
		return domain.Observation{}, err
	}
	out := domain.Observation{Scope: i.Revision.Scope, IntendedRevision: i.Revision.ID, ObservedRevision: r.RevisionID, PayloadDigest: r.PayloadDigest, Sync: domain.InSync, Health: domain.Healthy, Drift: domain.NoDrift, Complete: true, ObservedAt: time.Now().UTC()}
	for _, obj := range m {
		out.Objects = append(out.Objects, domain.ObjectObservation{APIVersion: obj.APIVersion, Kind: obj.Kind, Name: obj.Name, Namespace: i.Target.Namespace, UID: "owned-" + obj.Name})
	}
	return out, nil
}
func (*captureCD) Reconcile(context.Context, port.Intent, port.Receipt) (port.Receipt, error) {
	return port.Receipt{}, domain.ErrUnsupported
}
func (*captureCD) Withdraw(_ context.Context, _ port.Intent, r port.Receipt) (port.WithdrawalProof, error) {
	return port.WithdrawalProof{Receipt: r, ManagementStopped: true}, nil
}
func (p *captureCD) RemoveRetainedObjects(_ context.Context, _ port.Intent, _ port.Receipt, m []port.AppliedObject) error {
	p.removed += len(m)
	return nil
}

type privateRegistry struct {
	values   []string
	revision string
}

func (p *privateRegistry) Provider(context.Context, string, string) (configport.Provider, error) {
	return p, nil
}
func (p *privateRegistry) WriteValue(_ context.Context, _, _, value string) (string, error) {
	p.values = append(p.values, value)
	return "kv2://kv/orchestrator/apps/app/envs/staging/values/" + ids.New(), nil
}
func (*privateRegistry) ReadValue(context.Context, string) (string, error) {
	panic("no extra private read expected")
}
func (*privateRegistry) CheckWorkloadAuth(context.Context, string, string) error { return nil }
func (*privateRegistry) PrepareAccess(_ context.Context, r configport.AccessRequest) (configport.WorkloadAccess, error) {
	if !r.CDOwned {
		panic("CD secret access must use a separate ServiceAccount")
	}
	return configport.WorkloadAccess{Role: "workload", Address: "http://vault.internal", ServiceAccount: "workload-vault"}, nil
}
func (p *privateRegistry) PrepareBundle(_ context.Context, r configport.BundleRequest) (configport.WorkloadBundle, error) {
	if !r.CDOwned {
		panic("CD bundle must use a separate ServiceAccount")
	}
	p.revision = r.RevisionID
	keys := map[string]map[string]string{}
	for container, entries := range r.Values {
		keys[container] = map[string]string{}
		for name, value := range entries {
			p.values = append(p.values, value)
			keys[container][name] = container + "_" + name
		}
	}
	return configport.WorkloadBundle{StoreKey: r.DeliveryStoreKey, Address: "http://vault.internal", Mount: "kv", AuthMount: "kubernetes", Path: "orchestrator/apps/app/envs/staging/values/" + r.DeploymentID, Role: "workload", ServiceAccount: "workload-vault", SecretName: "orch-" + r.DeploymentID, Keys: keys}, nil
}

func TestCDOnlyDependencySecretsAndAuxiliaryCleanup(t *testing.T) {
	for _, vso := range []bool{false, true} {
		t.Run(map[bool]string{false: "Vault Agent", true: "VSO"}[vso], func(t *testing.T) {
			ctx := context.Background()
			app, opts := newApp(t)
			repo := app.Store.(cdStore)
			org, err := repo.GetOrganization(ctx, opts.OrganizationKey)
			if err != nil {
				t.Fatal(err)
			}
			account, err := repo.GetConnection(ctx, opts.OrganizationKey, opts.ConnectionKey)
			if err != nil {
				t.Fatal(err)
			}
			b := domain.BindingVersion{ID: ids.New(), Version: 1, OrganizationID: org.ID, Name: "Test CD", ProviderKind: "test-cd", ContractVersion: "v1", ManagementAccountID: account.ID, ManagementAccountVersion: account.Version, Config: map[string]string{}, Status: domain.Unverified, Revision: 1, CreatedAt: time.Now().UTC()}
			if err := repo.CreateDeliveryBinding(ctx, b); err != nil {
				t.Fatal(err)
			}
			caps := domain.Capabilities{ContractVersion: "v1", ExactRevision: true, PhasedDelivery: true, RetainOnWithdraw: true, GuardedPrune: true}
			if _, err := repo.VerifyDeliveryBinding(ctx, org.ID, b.ID, 1, 1, domain.BindingReady, caps, time.Now().UTC(), ""); err != nil {
				t.Fatal(err)
			}
			env, err := repo.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateSecretStore(ctx, secretstore.Store{Key: "private-store", OrganizationKey: opts.OrganizationKey, Name: "Private store", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady, BackendAddress: "https://vault.invalid", WorkloadAddress: "http://vault.internal", Mount: "kv", AuthMount: "kubernetes", CredentialRef: "synthetic"}); err != nil {
				t.Fatal(err)
			}
			env, err = repo.SelectSecretStore(ctx, persistence.SecretStoreSelection{ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, StoreKey: "private-store", ExpectedVersion: env.Version})
			if err != nil {
				t.Fatal(err)
			}
			env, err = repo.SelectDelivery(ctx, persistence.DeliverySelectionCommand{OrganizationID: org.ID, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, ExpectedEnvironmentVersion: env.Version, ExpectedConfigurationRevision: env.DeliveryConfigRevision, Selection: domain.Selection{Mode: domain.CD, BindingID: b.ID, BindingVersion: 1}})
			if err != nil {
				t.Fatal(err)
			}
			cipher, err := statecipher.New("test", make([]byte, 32))
			if err != nil {
				t.Fatal(err)
			}
			registry := port.NewRegistry()
			cd := &captureCD{}
			if err := registry.Register("test-cd", cd); err != nil {
				t.Fatal(err)
			}
			coordinator := &deliveryapp.Coordinator{Store: repo, Registry: registry, Artifacts: &deliveryapp.Artifacts{Store: repo, Cipher: cipher}}
			prov := provisioning.NewService(app.Store, fake.Registry{Executor: app.FakeExec}, app.Secrets, clock.System{})
			prov.SetModuleVerifier(app.Modules)
			templateRenderer := templatedriver.New()
			templateRenderer.SetGitBundles(app.RenderRegistry, renderengine.Executor{})
			svc := appsvc.NewService(app.Store, planning.NewService(bootstrap.FakeTemplateSources()), prov, templateRenderer, &rejectDirectApply{}, planning.RegistryInspector{Metadata: app.Modules}, clock.System{})
			svc.SetDelivery(coordinator)
			private := &privateRegistry{}
			svc.SetStoreRegistry(private)
			if vso {
				svc.SetConfigSecretSynchronizer(&k8s.VSOSynchronizer{KubectlPath: "/must-not-run"})
			}
			scores := seed.AcceptanceWorkloads(opts)
			score := scores["backend"]
			result, err := svc.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", WorkloadAfter: score, Actor: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if len(private.values) == 0 {
				t.Fatal("dependency secret did not reach the private store")
			}
			raw, _ := json.Marshal(cd.manifests)
			for _, value := range private.values {
				if strings.Contains(string(raw), value) {
					t.Fatal("dependency secret entered a CD artifact")
				}
			}
			if vso && private.revision != result.DeploymentID {
				t.Fatal("dependency-only secret delivery lost its immutable execution revision")
			}
			foundSA, foundVSO := false, false
			for _, m := range cd.manifests {
				if m.Kind == "Secret" {
					t.Fatal("direct Secret manifest survived CD conversion")
				}
				foundSA = foundSA || m.Kind == "ServiceAccount"
				foundVSO = foundVSO || m.Kind == "VaultStaticSecret"
			}
			if !foundSA || foundVSO != vso {
				t.Fatal("CD does not own complete secret delivery declarations")
			}
			_, err = svc.RemoveDelivered(ctx, opts.OrganizationKey, opts.ApplicationKey, opts.EnvironmentKey, env.TargetGeneration)
			if err != nil || cd.removed != len(cd.manifests) {
				t.Fatalf("CD inventory did not own complete cleanup: %v", err)
			}
		})
	}
}
