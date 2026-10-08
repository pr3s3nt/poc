package persistencetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// OperationFixture identifies the data created by EnvironmentOperations.
type OperationFixture struct {
	OrganizationKey, ApplicationKey string
}

func newOperationApp(t *testing.T, st persistence.Store, org string) string {
	t.Helper()
	ctx := context.Background()
	appID := ids.New()
	if err := st.SaveApplication(ctx, application.Application{ID: appID, Key: appID, OrganizationKey: org, Name: "Ops " + appID[:6], Subdomain: "ops-" + appID[:8], Version: 1}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"staging", "production"} {
		set := environment.DeploymentSet{ID: ids.New(), EnvironmentKey: appID + "/" + key, Document: environment.NewDocument(), DocumentHash: "empty", CreatedAt: time.Now().UTC()}
		env := environment.Environment{ID: ids.New(), Key: key, ApplicationID: appID, ApplicationKey: appID, Name: key, Type: key, NamespaceIdentity: "app-" + appID + "-" + key, CurrentDeploymentSetID: set.ID, Version: 1}
		set.EnvironmentID = env.ID
		if err := st.Transact(ctx, func(ctx context.Context) error {
			if err := st.SaveEnvironment(ctx, env); err != nil {
				return err
			}
			return st.SaveDeploymentSet(ctx, set)
		}); err != nil {
			t.Fatal(err)
		}
	}
	return appID
}

func ownerOf(op environment.Operation) persistence.Owner {
	return persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
}

func testStore(org, key string) secretstore.Store {
	return secretstore.Store{
		Key: key, OrganizationKey: org, Name: "Store " + key, Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
		BackendAddress: "http://vault.example:8200", WorkloadAddress: "http://vault.vault.svc:8200", Mount: "kv", AuthMount: "kubernetes",
		CredentialRef: "kv2://kv/orchestrator/connections/" + org + "/ss-" + key + "/credentials/" + ids.New(),
	}
}

// EnvironmentOperations is the adapter-neutral contract of secret-store
// records, persisted Environment claims, owner-checked writes, target
// generations and transition records (ADR-012).
func EnvironmentOperations(t *testing.T, st persistence.Store) OperationFixture {
	t.Helper()
	ctx := context.Background()
	org := "opsorg"
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: org, Name: "Ops", DefaultConnectionKey: "lab"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "otherorg", Name: "Other", DefaultConnectionKey: "lab"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"lab", "lab2"} {
		if err := st.SaveConnection(ctx, application.Connection{ID: ids.New(), Key: key, Name: key, OrganizationKey: org, Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthHostContext, Status: application.ConnectionReady, Config: map[string]any{}, SecretRef: "host-kube-context://" + key, Verification: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}

	// Secret stores: insert-only, Organization scoped.
	one, two := testStore(org, "vault-a"), testStore(org, "vault-b")
	for _, store := range []secretstore.Store{one, two} {
		if err := st.CreateSecretStore(ctx, store); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.CreateSecretStore(ctx, one); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("duplicate store: %v", err)
	}
	if err := st.CreateSecretStore(ctx, testStore("missing-org", "vault-a")); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("store in missing organization: %v", err)
	}
	if err := st.CreateSecretStore(ctx, testStore("otherorg", "vault-a")); err != nil {
		t.Fatalf("same key in another organization: %v", err)
	}
	if got, err := st.GetSecretStore(ctx, org, "vault-a"); err != nil || got.Mount != "kv" || got.CredentialRef != one.CredentialRef || got.OrganizationKey != org {
		t.Fatalf("get store: %+v %v", got, err)
	}
	if _, err := st.GetSecretStore(ctx, "otherorg", "vault-b"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign store must be invisible: %v", err)
	}
	if list, err := st.ListSecretStores(ctx, org); err != nil || len(list) != 2 || list[0].Key != "vault-a" {
		t.Fatalf("list stores: %+v %v", list, err)
	}

	appID := newOperationApp(t, st, org)
	pins := environment.OperationPins{CheckEnvVersion: true, EnvVersion: 1, CheckDraftVersion: true, DraftVersion: 0, CheckConfigVersion: true, ConfigVersion: 0}
	claim := func(env string, p environment.OperationPins, owner string) persistence.OperationClaim {
		return persistence.OperationClaim{ID: ids.New(), ApplicationKey: appID, EnvironmentKey: env, Owner: owner, Kind: environment.OpDeploy, Pins: p, Deadline: time.Now().Add(time.Hour)}
	}

	// Stale pins reject without claiming.
	stale := pins
	stale.EnvVersion = 7
	if _, err := st.ClaimEnvironment(ctx, claim("staging", stale, "p1")); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale claim: %v", err)
	}
	if env, _ := st.GetEnvironment(ctx, appID, "staging"); env.Busy() {
		t.Fatal("stale claim must not hold the environment")
	}

	// Concurrent claims: exactly one winner, all others busy.
	var wg sync.WaitGroup
	type result struct {
		op  environment.Operation
		err error
	}
	results := make(chan result, 8)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op, err := st.ClaimEnvironment(ctx, claim("staging", pins, "proc-"+ids.New()[:4]))
			results <- result{op, err}
		}()
	}
	wg.Wait()
	close(results)
	var held environment.Operation
	wins := 0
	for r := range results {
		switch {
		case r.err == nil:
			wins++
			held = r.op
		case errors.Is(r.err, persistence.ErrEnvironmentBusy):
		default:
			t.Fatalf("concurrent claim: %v", r.err)
		}
	}
	if wins != 1 {
		t.Fatalf("claim winners = %d", wins)
	}
	env, _ := st.GetEnvironment(ctx, appID, "staging")
	if env.ActiveOperationID != held.ID || held.Status != environment.OpActive {
		t.Fatalf("environment claim not visible: %+v %+v", env, held)
	}
	if active, ok, err := st.ActiveOperation(ctx, appID, "staging"); err != nil || !ok || active.ID != held.ID {
		t.Fatalf("active operation: %+v %v %v", active, ok, err)
	}

	// A different Environment stays independent.
	other, err := st.ClaimEnvironment(ctx, claim("production", pins, "procX"))
	if err != nil {
		t.Fatalf("independent environment: %v", err)
	}
	if err := st.ReleaseOperation(ctx, ownerOf(other), environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}

	// Every foreign write is rejected as busy while claimed.
	if err := st.SaveWorkloadDraft(ctx, 0, environment.WorkloadDraft{ApplicationKey: appID, EnvironmentKey: "staging", WorkloadID: "web", State: environment.DraftUpsert, Score: map[string]any{"apiVersion": "score.dev/v1b1"}}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("draft while busy: %v", err)
	}
	if err := st.CommitConfigurationRevision(ctx, 0, configuration.Revision{ID: ids.New(), ApplicationKey: appID, EnvironmentKey: "staging", Version: 1, Entries: map[string]configuration.Entry{"A": {Kind: configuration.Variable, Value: "1"}}}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("configuration while busy: %v", err)
	}
	if _, err := st.BindEnvironment(ctx, bind(appID, "staging", "lab", application.ProfileInternalK8s, "", application.RuntimeReady, 1)); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("bind while busy: %v", err)
	}
	if _, err := st.SelectSecretStore(ctx, persistence.SecretStoreSelection{ApplicationKey: appID, EnvironmentKey: "staging", StoreKey: "vault-a", ExpectedVersion: 1}); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("store selection while busy: %v", err)
	}
	if err := st.SetPublicRoutesPending(ctx, appID, "staging", true); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("routes flag while busy: %v", err)
	}
	if err := st.CompareVersionAndSetCurrent(ctx, appID, "staging", 1, env.CurrentDeploymentSetID); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("set pointer while busy: %v", err)
	}
	if _, err := st.AllocateGeneration(ctx, appID, "staging"); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("generation without owner: %v", err)
	}
	if got, _ := st.GetEnvironment(ctx, appID, "staging"); got.Version != 1 || got.Configured() || got.PublicRoutesPending {
		t.Fatalf("rejected writes mutated: %+v", got)
	}

	// The owner may write; ctx carries the operation.
	own := persistence.WithOwner(ctx, ownerOf(held))
	if err := st.SetPublicRoutesPending(own, appID, "staging", true); err != nil {
		t.Fatalf("owner routes flag: %v", err)
	}
	if err := st.SaveWorkloadDraft(own, 0, environment.WorkloadDraft{ApplicationKey: appID, EnvironmentKey: "staging", WorkloadID: "web", State: environment.DraftUpsert, Score: map[string]any{"apiVersion": "score.dev/v1b1"}}); err != nil {
		t.Fatalf("owner draft: %v", err)
	}
	generation, err := st.AllocateGeneration(own, appID, "staging")
	if err != nil || generation != 1 {
		t.Fatalf("generation = %d %v", generation, err)
	}
	if again, err := st.AllocateGeneration(own, appID, "staging"); err != nil || again != 2 {
		t.Fatalf("second generation = %d %v", again, err)
	}
	// Bind to generation 1 as the owner; generation 5 was never allocated.
	badGen := bind(appID, "staging", "lab", application.ProfileInternalK8s, "", application.RuntimeReady, 1)
	badGen.Generation = 5
	if _, err := st.BindEnvironment(own, badGen); err == nil {
		t.Fatal("binding an unallocated generation must fail")
	}
	good := bind(appID, "staging", "lab", application.ProfileInternalK8s, "", application.RuntimeReady, 1)
	good.Generation = 1
	if bound, err := st.BindEnvironment(own, good); err != nil || bound.TargetGeneration != 1 || bound.GenerationHigh != 2 || bound.Version != 2 {
		t.Fatalf("owner bind: %+v %v", bound, err)
	}

	// Instances of different generations coexist.
	deploymentID := ids.New()
	if err := st.SaveDeployment(own, deployment.Deployment{ID: deploymentID, EnvironmentID: env.ID, OrganizationKey: org, ApplicationKey: appID, EnvironmentKey: "staging", ExecutionProfile: "internal-k8s", Action: deployment.ActionDeploy, WorkloadID: "web", ActorRef: "t", Status: deployment.StatusPlanning, StartedAt: time.Now().UTC(), ConnectionKey: "lab", TargetGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	for _, generation := range []int64{0, 1} {
		w := deployment.WorkloadInstance{EnvironmentKey: appID + "/staging", Generation: generation, WorkloadID: "web", LastDeploymentID: deploymentID, TargetRef: map[string]any{"namespace": environment.NamespaceFor(env.NamespaceIdentity, generation)}, ManifestDigest: "d", Status: deployment.InstanceApplying, ObservedAt: time.Now().UTC()}
		if err := st.UpsertWorkloadInstance(own, w); err != nil {
			t.Fatalf("instance generation %d: %v", generation, err)
		}
	}
	current, _ := st.ListWorkloadInstances(ctx, appID+"/staging")
	oldOne, _ := st.ListWorkloadInstancesFor(ctx, appID+"/staging", 0)
	if len(current) != 1 || current[0].Generation != 1 || len(oldOne) != 1 || oldOne[0].Generation != 0 || oldOne[0].TargetRef["namespace"] == current[0].TargetRef["namespace"] {
		t.Fatalf("generations overwrote each other: current=%+v old=%+v", current, oldOne)
	}
	if got, _ := st.GetDeployment(ctx, deploymentID); got.ConnectionKey != "lab" || got.TargetGeneration != 1 {
		t.Fatalf("deployment target not recorded: %+v", got)
	}

	// Heartbeat / interruption / recovery never release the claim by time.
	if err := st.HeartbeatOperation(ctx, persistence.Owner{OperationID: held.ID, Owner: "someone-else", Fence: held.Fence}, "x", nil); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("foreign heartbeat: %v", err)
	}
	if err := st.HeartbeatOperation(ctx, ownerOf(held), "STAGE", map[string]any{"n": float64(1)}); err != nil {
		t.Fatalf("owner heartbeat: %v", err)
	}
	if n, err := st.MarkInterrupted(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("fresh claim must not be interrupted: %d %v", n, err)
	}
	if n, err := st.MarkInterrupted(ctx, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("stale claim interrupted: %d %v", n, err)
	}
	if got, _ := st.GetOperation(ctx, held.ID); got.Status != environment.OpInterrupted || got.Stage != "STAGE" {
		t.Fatalf("interrupted state: %+v", got)
	}
	if _, err := st.ClaimEnvironment(ctx, claim("staging", environment.OperationPins{}, "p9")); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("interrupted claim must keep the environment busy: %v", err)
	}
	if err := st.HeartbeatOperation(ctx, ownerOf(held), "late", nil); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("interrupted owner must lose its claim: %v", err)
	}
	// Stale heartbeat alone never authorizes recovery: confirmation is required.
	if _, err := st.BeginRecovery(ctx, held.ID, "recoverer", ""); !errors.Is(err, persistence.ErrRecoveryUnconfirmed) {
		t.Fatalf("unconfirmed recovery: %v", err)
	}
	recovering, err := st.BeginRecovery(ctx, held.ID, "recoverer", "operator")
	if err != nil || recovering.Status != environment.OpRecovering || recovering.Owner != "recoverer" || recovering.Fence != held.Fence+1 || recovering.Detail["stoppedConfirmedBy"] != "operator" {
		t.Fatalf("begin recovery: %+v %v", recovering, err)
	}
	if _, err := st.BeginRecovery(ctx, held.ID, "recoverer-2", "operator"); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("double recovery: %v", err)
	}
	// The paused former owner is fenced out of release, heartbeat and writes
	// even though it still presents the same operation ID.
	if err := st.ReleaseOperation(ctx, ownerOf(held), environment.OpFailed, "stale release"); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("stale owner release: %v", err)
	}
	if err := st.HeartbeatOperation(ctx, ownerOf(held), "x", nil); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("stale owner heartbeat: %v", err)
	}
	if err := st.SetPublicRoutesPending(own, appID, "staging", false); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("stale owner write: %v", err)
	}
	if err := st.SaveTransition(own, environment.Transition{ID: ids.New(), OperationID: held.ID, ApplicationKey: appID, EnvironmentKey: "staging", Status: environment.TransitionRunning, Stage: environment.StagePreflight, Mode: environment.ModeDeployNew}); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("stale owner transition write: %v", err)
	}
	if err := st.UpsertWorkloadInstance(own, deployment.WorkloadInstance{EnvironmentKey: appID + "/staging", WorkloadID: "late", LastDeploymentID: deploymentID, Status: deployment.InstanceReady, ObservedAt: time.Now()}); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatalf("stale owner instance write: %v", err)
	}
	if got, _ := st.GetOperation(ctx, held.ID); got.Status != environment.OpRecovering {
		t.Fatalf("stale release changed the operation: %+v", got)
	}
	newOwner := persistence.WithOwner(ctx, ownerOf(recovering))
	if err := st.SetPublicRoutesPending(newOwner, appID, "staging", false); err != nil {
		t.Fatalf("recovery owner write: %v", err)
	}
	// A duplicate operation ID never overwrites another claim.
	dup := claim("production", pins, "px")
	dup.ID = held.ID
	if _, err := st.ClaimEnvironment(ctx, dup); err == nil {
		t.Fatal("duplicate operation id accepted")
	}
	if got, _ := st.GetOperation(ctx, held.ID); got.EnvironmentKey != "staging" || got.Status != environment.OpRecovering {
		t.Fatalf("duplicate id overwrote the claim: %+v", got)
	}
	if err := st.ReleaseOperation(ctx, ownerOf(recovering), environment.OpRecovered, "interrupted deploy recovered"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetEnvironment(ctx, appID, "staging"); got.Busy() {
		t.Fatal("release must free the environment")
	}
	if ops, err := st.ListOperations(ctx, appID, "staging", 10); err != nil || len(ops) != 1 || ops[0].Status != environment.OpRecovered {
		t.Fatalf("operation history: %+v %v", ops, err)
	}

	// Store selection and legacy backfill.
	sel, err := st.SelectSecretStore(ctx, persistence.SecretStoreSelection{ApplicationKey: appID, EnvironmentKey: "staging", StoreKey: "vault-a", ExpectedVersion: 2})
	if err != nil || sel.SecretStoreKey != "vault-a" || sel.Version != 3 {
		t.Fatalf("select store: %+v %v", sel, err)
	}
	if _, err := st.SelectSecretStore(ctx, persistence.SecretStoreSelection{ApplicationKey: appID, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: 2}); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale store selection: %v", err)
	}
	if _, err := st.SelectSecretStore(ctx, persistence.SecretStoreSelection{ApplicationKey: appID, EnvironmentKey: "staging", StoreKey: "nope", ExpectedVersion: 3}); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unknown store: %v", err)
	}
	// A store of another Organization is invisible to this Environment.
	if _, err := st.SelectSecretStore(ctx, persistence.SecretStoreSelection{ApplicationKey: appID, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: 3}); err != nil {
		t.Fatalf("switch store: %v", err)
	}

	legacyApp := newOperationApp(t, st, org)
	legacyRef := "kv2://kv/orchestrator/apps/" + legacyApp + "/envs/staging/values/" + ids.New()
	legacyRev := configuration.Revision{ID: ids.New(), ApplicationKey: legacyApp, EnvironmentKey: "staging", Version: 1, Entries: map[string]configuration.Entry{
		"OLD_VAR": {Kind: configuration.Variable, ValueRef: legacyRef},
		"OLD_SEC": {Kind: configuration.Secret, ValueRef: legacyRef + "s"},
		"NEW_VAR": {Kind: configuration.Variable, Value: "plain"},
	}}
	if err := st.CommitConfigurationRevision(ctx, 0, legacyRev); err != nil {
		t.Fatal(err)
	}
	legacyStore := testStore(org, "platform-vault")
	legacyStore.Legacy = true
	if err := st.CreateSecretStore(ctx, legacyStore); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := st.BackfillLegacySecretStore(ctx, org, "platform-vault"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.GetConfigurationRevision(ctx, legacyRev.ID)
	if err != nil || got.Entries["OLD_VAR"].StoreKey != "platform-vault" || got.Entries["OLD_SEC"].StoreKey != "platform-vault" || got.Entries["OLD_VAR"].ValueRef != legacyRef || got.Entries["NEW_VAR"].StoreKey != "" || got.Entries["NEW_VAR"].Value != "plain" {
		t.Fatalf("legacy backfill: %+v %v", got.Entries, err)
	}
	if e, _ := st.GetEnvironment(ctx, legacyApp, "staging"); e.SecretStoreKey != "platform-vault" || e.Version != 1 {
		t.Fatalf("backfill selection: %+v", e)
	}
	if e, _ := st.GetEnvironment(ctx, legacyApp, "production"); e.SecretStoreKey != "" {
		t.Fatalf("environment without configuration must stay unselected: %+v", e)
	}
	assertManagedAdmission(t, st, org, legacyApp, legacyRev, legacyRef)

	// New Applications never auto-select the legacy store.
	if fresh := newOperationApp(t, st, org); true {
		if e, _ := st.GetEnvironment(ctx, fresh, "staging"); e.SecretStoreKey != "" {
			t.Fatalf("new environment auto-selected a store: %+v", e)
		}
	}

	// Transition records round-trip with their detail.
	tr := environment.Transition{ID: ids.New(), OperationID: ids.New(), ApplicationKey: appID, EnvironmentKey: "staging", Mode: environment.ModeMigratePostgres, Stage: environment.StageBackup, Status: environment.TransitionRunning,
		Source: environment.Binding{ConnectionKey: "lab", Profile: application.ProfileInternalK8s, Scope: environment.ScopeEnvironment}, Destination: environment.Binding{ConnectionKey: "lab2", Profile: application.ProfileInternalK8s, Scope: environment.ScopeEnvironment, Generation: 2},
		OldSetID: "set", Replicas: map[string]int{"api": 2}, SourceState: environment.SourceAuthoritative, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.SaveTransition(ctx, tr); err != nil {
		t.Fatal(err)
	}
	tr.Stage, tr.Status = environment.StageRestoring, environment.TransitionFailed
	if err := st.SaveTransition(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if gotTr, err := st.GetTransition(ctx, tr.ID); err != nil || gotTr.Stage != environment.StageRestoring || gotTr.Replicas["api"] != 2 || gotTr.Destination.Generation != 2 || gotTr.Status != environment.TransitionFailed {
		t.Fatalf("transition: %+v %v", gotTr, err)
	}
	if list, err := st.ListTransitions(ctx, appID, "staging"); err != nil || len(list) != 1 {
		t.Fatalf("transitions: %+v %v", list, err)
	}
	return OperationFixture{OrganizationKey: org, ApplicationKey: appID}
}

// AssertOperationsReloaded checks persisted claims, selections and generations
// after the adapter was reopened.
func AssertOperationsReloaded(t *testing.T, st persistence.Store, fx OperationFixture) {
	t.Helper()
	ctx := context.Background()
	env, err := st.GetEnvironment(ctx, fx.ApplicationKey, "staging")
	if err != nil || env.SecretStoreKey != "vault-b" || env.TargetGeneration != 1 || env.GenerationHigh != 2 || env.Busy() || env.Version != 4 {
		t.Fatalf("environment after reopen: %+v %v", env, err)
	}
	if managed, err := st.GetSecretStore(ctx, fx.OrganizationKey, "platform-vault"); err != nil || managed.Legacy || managed.CredentialRef != managedRef2 || managed.Verification["bootstrap"] != "compose" {
		t.Fatalf("managed store after reopen: %+v %v", managed, err)
	}
	if ops, err := st.ListOperations(ctx, fx.ApplicationKey, "staging", 5); err != nil || len(ops) != 1 || ops[0].Status != environment.OpRecovered {
		t.Fatalf("operations after reopen: %+v %v", ops, err)
	}
	if old, _ := st.ListWorkloadInstancesFor(ctx, fx.ApplicationKey+"/staging", 0); len(old) != 1 {
		t.Fatalf("generation 0 instance lost after reopen: %+v", old)
	}
}

const (
	managedRef1 = "kv2://kv/orchestrator/connections/opsorg/ss-platform-vault/credentials/00000000-0000-4000-8000-000000000001"
	managedRef2 = "kv2://kv/orchestrator/connections/opsorg/ss-platform-vault/credentials/00000000-0000-4000-8000-000000000002"
)

// assertManagedAdmission is the contract of the Compose bootstrap admission:
// compare-and-set conversion/refresh that keeps the row identity and every
// reference, and refuses stale, duplicate and mismatched admissions unchanged.
func assertManagedAdmission(t *testing.T, st persistence.Store, org, legacyApp string, legacyRev configuration.Revision, legacyRef string) {
	t.Helper()
	ctx := context.Background()
	prior, err := st.GetSecretStore(ctx, org, "platform-vault")
	if err != nil || !prior.Legacy || prior.ID == "" {
		t.Fatalf("legacy prior: %+v %v", prior, err)
	}
	next := prior
	next.Name, next.Legacy, next.CredentialRef, next.Verification = "Platform Vault", false, managedRef1, map[string]any{"bootstrap": "compose", "verified": true}
	admit := func(s secretstore.Store, id string, legacy bool, ref string) (secretstore.Store, error) {
		return st.AdmitManagedSecretStore(ctx, persistence.ManagedStoreAdmission{Store: s, ExpectedID: id, ExpectedLegacy: legacy, ExpectedCredentialRef: ref})
	}
	unchanged := func(label string) {
		t.Helper()
		got, err := st.GetSecretStore(ctx, org, "platform-vault")
		if err != nil || !got.Legacy || got.CredentialRef != prior.CredentialRef || got.Name != prior.Name || got.ID != prior.ID {
			t.Fatalf("%s changed the record: %+v %v", label, got, err)
		}
	}
	if _, err := admit(next, "", false, ""); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("create over an existing record: %v", err)
	}
	unchanged("duplicate create")
	for label, args := range map[string]struct {
		id     string
		legacy bool
		ref    string
	}{
		"stale id":          {ids.New(), true, prior.CredentialRef},
		"wrong legacy flag": {prior.ID, false, prior.CredentialRef},
		"wrong credential":  {prior.ID, true, managedRef2},
	} {
		if _, err := admit(next, args.id, args.legacy, args.ref); !errors.Is(err, persistence.ErrVersionConflict) {
			t.Fatalf("%s: %v", label, err)
		}
		unchanged(label)
	}
	for label, mutate := range map[string]func(*secretstore.Store){
		"backend":    func(s *secretstore.Store) { s.BackendAddress = "http://other.example:8200" },
		"workload":   func(s *secretstore.Store) { s.WorkloadAddress = "http://other.svc:8200" },
		"mount":      func(s *secretstore.Store) { s.Mount = "other" },
		"auth mount": func(s *secretstore.Store) { s.AuthMount = "other" },
	} {
		mismatch := next
		mutate(&mismatch)
		if _, err := admit(mismatch, prior.ID, true, prior.CredentialRef); !errors.Is(err, persistence.ErrImmutable) {
			t.Fatalf("%s identity mismatch: %v", label, err)
		}
		unchanged(label)
	}
	missingOrg := next
	missingOrg.OrganizationKey = "missing-org"
	if _, err := admit(missingOrg, "", false, ""); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing organization: %v", err)
	}

	converted, err := admit(next, prior.ID, true, prior.CredentialRef)
	if err != nil || converted.ID != prior.ID || converted.Legacy || converted.CredentialRef != managedRef1 || converted.Name != "Platform Vault" || !converted.CreatedAt.Equal(prior.CreatedAt) || converted.Status != secretstore.StatusReady {
		t.Fatalf("conversion: %+v %v", converted, err)
	}
	if _, err := admit(next, prior.ID, true, prior.CredentialRef); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("replayed conversion must lose the compare-and-set: %v", err)
	}
	got, err := st.GetConfigurationRevision(ctx, legacyRev.ID)
	if err != nil || got.Entries["OLD_VAR"].StoreKey != "platform-vault" || got.Entries["OLD_VAR"].ValueRef != legacyRef || got.Entries["OLD_SEC"].StoreKey != "platform-vault" {
		t.Fatalf("conversion changed value references: %+v %v", got.Entries, err)
	}
	if e, _ := st.GetEnvironment(ctx, legacyApp, "staging"); e.SecretStoreKey != "platform-vault" || e.Version != 1 {
		t.Fatalf("conversion changed the Environment selection: %+v", e)
	}

	refreshed := next
	refreshed.CredentialRef = managedRef2
	if out, err := admit(refreshed, prior.ID, false, managedRef1); err != nil || out.CredentialRef != managedRef2 || out.ID != prior.ID {
		t.Fatalf("refresh: %+v %v", out, err)
	}
	if _, err := admit(refreshed, prior.ID, false, managedRef1); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale refresh: %v", err)
	}

	// A missing record is created insert-style.
	fresh := testStore("otherorg", "platform-vault")
	fresh.Name = "Platform Vault"
	if out, err := admit(fresh, "", false, ""); err != nil || out.ID == "" || out.CredentialRef != fresh.CredentialRef || out.Legacy {
		t.Fatalf("create: %+v %v", out, err)
	}
	if _, err := admit(fresh, "", false, ""); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("second create: %v", err)
	}
	if _, err := admit(fresh, ids.New(), false, fresh.CredentialRef); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale id: %v", err)
	}

	// Concurrent refreshes of one prior state: exactly one wins, the rest lose
	// the compare-and-set and leave the winner's record intact.
	created, _ := st.GetSecretStore(ctx, "otherorg", "platform-vault")
	const racers = 8
	results := make(chan error, racers)
	refs := make([]string, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		refs[i] = fmt.Sprintf("kv2://kv/orchestrator/connections/otherorg/ss-platform-vault/credentials/00000000-0000-4000-8000-0000000001%02d", i)
		wg.Add(1)
		go func(ref string) {
			defer wg.Done()
			attempt := fresh
			attempt.CredentialRef = ref
			_, err := admit(attempt, created.ID, false, created.CredentialRef)
			results <- err
		}(refs[i])
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, persistence.ErrVersionConflict):
			t.Fatalf("racer lost with an unexpected error: %v", err)
		}
	}
	winner, _ := st.GetSecretStore(ctx, "otherorg", "platform-vault")
	known := false
	for _, ref := range refs {
		known = known || ref == winner.CredentialRef
	}
	if wins != 1 || !known || winner.ID != created.ID {
		t.Fatalf("concurrent admission: wins=%d winner=%+v", wins, winner)
	}
}
