package connection_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
)

// FAKE verifiers and in-memory credential store: these tests prove V01 account
// orchestration (CAS, ordering, cleanup, target guards). The real SQL/Vault
// behaviour has its own integration suites and the headed scenario.

const (
	secondKeyID  = "AKIAFIXTUREKEY000002"
	secondSecret = "second/secret/value/00000000000000000000" // 40 chars
)

type accountEnv struct {
	st    *store.Store
	creds *credentialmemory.Store
	kube  *kubeconfigVerifier
	aws   *awsVerifier
	svc   *connection.Service
}

func newAccountEnv(t *testing.T, credStore credentialStore, st *store.Store) *accountEnv {
	t.Helper()
	ctx := context.Background()
	if st == nil {
		st = store.New()
	}
	for _, key := range []string{"acme", "other"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: key, Name: key, DefaultConnectionKey: "internal-cluster"}); err != nil {
			t.Fatal(err)
		}
	}
	env := &accountEnv{st: st, kube: &kubeconfigVerifier{}, aws: &awsVerifier{}}
	env.svc = connection.NewService(st)
	env.svc.SetKubeconfigRegistration(env.kube, credStore)
	env.svc.SetAWSRegistration(env.aws)
	return env
}

type credentialStore interface {
	Put(ctx context.Context, org, conn string, value []byte) (string, error)
	Get(ctx context.Context, org, conn, ref string) ([]byte, error)
	Delete(ctx context.Context, org, conn, ref string) error
	Durable() bool
}

func (e *accountEnv) awsAccount(t *testing.T) application.Connection {
	t.Helper()
	acc, err := e.svc.CreateAWSAccount(context.Background(), "acme", awsCommand())
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func (e *accountEnv) internalAccount(t *testing.T) application.Connection {
	t.Helper()
	doc := ct.Single("lab", "https://lab.example:6443", secretToken)
	acc, err := e.svc.RegisterKubeconfig(context.Background(), "acme", connection.RegisterKubeconfigCommand{Name: "Cụm lab", Kubeconfig: doc, Context: "lab"})
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func TestCreateAccountsAreVersionOneWithEvidenceRegions(t *testing.T) {
	creds := credentialmemory.New()
	env := newAccountEnv(t, creds, nil)
	aws := env.awsAccount(t)
	if aws.Version != 1 || connection.AccountType(aws.Kind) != "AWS" || aws.AuthenticationType != application.AuthAWSAccessKey {
		t.Fatalf("aws account: %#v", aws)
	}
	if got := connection.AccountRegions(aws); len(got) != 2 || got[0] != "us-east-1" {
		t.Fatalf("verification regions: %v", got)
	}
	if aws.ConfigString("accountId") != "123456789012" || len(aws.AllowedRegions()) != 2 {
		t.Fatalf("legacy consumer metadata must be kept: %#v", aws.Config)
	}
	internal := env.internalAccount(t)
	if internal.Version != 1 || connection.AccountType(internal.Kind) != "INTERNAL" || internal.ConfigString("tlsVerification") != "ENABLED" {
		t.Fatalf("internal account: %#v", internal)
	}
	// Field errors for the new form name verificationRegions, not allowedRegions.
	bad := awsCommand()
	bad.AllowedRegions = nil
	_, err := env.svc.CreateAWSAccount(context.Background(), "acme", bad)
	var fe *connection.FieldError
	if !errors.As(err, &fe) || fe.Field != "verificationRegions" {
		t.Fatalf("want verificationRegions field error, got %v", err)
	}
}

func TestRenameKeepsIdentityTargetRefAndEvidence(t *testing.T) {
	ctx := context.Background()
	env := newAccountEnv(t, credentialmemory.New(), nil)
	before := env.awsAccount(t)
	after, err := env.svc.RenameAccount(ctx, "acme", before.Key, before.Version, "  Tên mới  ")
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != "Tên mới" || after.Version != before.Version+1 {
		t.Fatalf("rename: %#v", after)
	}
	if after.ID != before.ID || after.Key != before.Key || after.SecretRef != before.SecretRef || after.Kind != before.Kind ||
		after.AuthenticationType != before.AuthenticationType || after.ConfigString("accountId") != before.ConfigString("accountId") ||
		after.Verification["verifiedAt"] != before.Verification["verifiedAt"] || after.Status != before.Status || before.ID == "" {
		t.Fatalf("rename changed more than the name:\nbefore %#v\nafter  %#v", before, after)
	}
	// Stale: the same expected version again loses without changing the winner.
	if _, err := env.svc.RenameAccount(ctx, "acme", before.Key, before.Version, "Stale"); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale rename: %v", err)
	}
	current, _ := env.st.GetConnection(ctx, "acme", before.Key)
	if current.Name != "Tên mới" || current.Version != 2 {
		t.Fatalf("winner overwritten: %#v", current)
	}
	for _, name := range []string{"", "   ", strings.Repeat("x", 101), "a\x07b"} {
		if _, err := env.svc.RenameAccount(ctx, "acme", before.Key, 2, name); !errors.Is(err, connection.ErrInvalid) {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	if _, err := env.svc.RenameAccount(ctx, "other", before.Key, 2, "X"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization must be not found: %v", err)
	}
}

func TestVerifySuccessPublishesOnceAndFailureKeepsCommittedRecord(t *testing.T) {
	ctx := context.Background()
	creds := credentialmemory.New()
	env := newAccountEnv(t, creds, nil)
	clock := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	env.svc.SetClock(func() time.Time { return clock })
	acc := env.awsAccount(t)

	clock = clock.Add(time.Hour)
	verified, err := env.svc.VerifyAccount(ctx, "acme", acc.Key, acc.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Version != 2 || verified.Verification["verifiedAt"] != "2026-10-17T09:00:00Z" || verified.Status != application.ConnectionReady {
		t.Fatalf("verified: %#v", verified)
	}
	last := env.aws.keys[len(env.aws.keys)-1]
	if last.AccessKeyID != fixtureKeyID {
		t.Fatalf("verify did not use the stored key")
	}
	if got := env.aws.asked[len(env.aws.asked)-1]; len(got) != 2 || got[0] != "us-east-1" {
		t.Fatalf("stored evidence regions were not rechecked: %v", got)
	}
	// Explicit regions replace the evidence list.
	again, err := env.svc.VerifyAccount(ctx, "acme", acc.Key, 2, []string{"ap-southeast-1"})
	if err != nil || again.Version != 3 || len(connection.AccountRegions(again)) != 1 {
		t.Fatalf("explicit regions: %v %#v", err, again)
	}
	if again.ConfigString("region") != "us-east-1" || len(again.AllowedRegions()) != 2 {
		t.Fatalf("legacy allowed-region policy must not move on reverify: %#v", again.Config)
	}

	// Failure: committed record, ref and version stay; the error is a safe category.
	env.aws.err = connection.ErrAuthentication
	_, err = env.svc.VerifyAccount(ctx, "acme", acc.Key, 3, nil)
	if !errors.Is(err, connection.ErrVerification) || !errors.Is(err, connection.ErrAuthentication) {
		t.Fatalf("failure class: %v", err)
	}
	if strings.Contains(err.Error(), fixtureSecret) || strings.Contains(err.Error(), fixtureKeyID) {
		t.Fatalf("error leaks the credential: %v", err)
	}
	current, _ := env.st.GetConnection(ctx, "acme", acc.Key)
	if current.Version != 3 || current.SecretRef != acc.SecretRef || current.Status != application.ConnectionReady || current.Verification["verifiedAt"] != again.Verification["verifiedAt"] {
		t.Fatalf("failed verify changed the record: %#v", current)
	}
	// Stale version is rejected before any provider call.
	env.aws.err = nil
	calls := env.aws.calls
	if _, err := env.svc.VerifyAccount(ctx, "acme", acc.Key, 1, nil); !errors.Is(err, persistence.ErrVersionConflict) || env.aws.calls != calls {
		t.Fatalf("stale verify: %v calls %d->%d", err, calls, env.aws.calls)
	}
}

func TestVerifyInternalRejectsRegionsAndChecksStoredTarget(t *testing.T) {
	ctx := context.Background()
	env := newAccountEnv(t, credentialmemory.New(), nil)
	acc := env.internalAccount(t)
	if _, err := env.svc.VerifyAccount(ctx, "acme", acc.Key, acc.Version, []string{"us-east-1"}); !errors.Is(err, connection.ErrInvalid) {
		t.Fatalf("AWS-only field on Internal: %v", err)
	}
	updated, err := env.svc.VerifyAccount(ctx, "acme", acc.Key, acc.Version, nil)
	if err != nil || updated.Version != 2 {
		t.Fatalf("verify: %v %#v", err, updated)
	}
	if n := len(env.kube.selected); n != 2 || env.kube.selected[1].Context.Endpoint != "https://lab.example:6443" {
		t.Fatalf("verifier input: %d", n)
	}
	env.kube.err = connection.ErrPermissionDenied
	_, err = env.svc.VerifyAccount(ctx, "acme", acc.Key, 2, nil)
	if !errors.Is(err, connection.ErrVerification) || !errors.Is(err, connection.ErrPermissionDenied) {
		t.Fatalf("permission failure: %v", err)
	}
	current, _ := env.st.GetConnection(ctx, "acme", acc.Key)
	if current.Version != 2 {
		t.Fatalf("failed verify advanced the version: %d", current.Version)
	}
}

func TestVerifyWithoutStoredCredentialOrStoreFailsSafely(t *testing.T) {
	ctx := context.Background()
	creds := credentialmemory.New()
	env := newAccountEnv(t, creds, nil)
	seed := application.Connection{Key: "seed", Name: "seed", OrganizationKey: "acme", Version: 1, Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Config: map[string]any{"cluster": "c", "kubeContext": "x"}, Status: application.ConnectionVerifying, Verification: map[string]any{"verified": false}}
	if err := env.st.SaveConnection(ctx, seed); err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.VerifyAccount(ctx, "acme", "seed", 1, nil); !errors.Is(err, connection.ErrVerification) {
		t.Fatalf("seed without credential must not become READY: %v", err)
	}
	if _, err := env.svc.ReplaceInternalCredentials(ctx, "acme", "seed", 1, ct.Single("x", "https://a.example", secretToken), "x"); !errors.Is(err, connection.ErrTargetMismatch) {
		t.Fatalf("seed without a committed target cannot be replaced: %v", err)
	}
	acc := env.awsAccount(t)
	if err := creds.Delete(ctx, "acme", acc.Key, acc.SecretRef); err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.VerifyAccount(ctx, "acme", acc.Key, acc.Version, nil); !errors.Is(err, connection.ErrCredentialStore) {
		t.Fatalf("missing object: %v", err)
	}
}

func TestReplaceAWSSameAccountPublishesAtomicallyAndKeepsPriorObject(t *testing.T) {
	ctx := context.Background()
	creds := &recordingCredentials{Store: credentialmemory.New()}
	env := newAccountEnv(t, creds, nil)
	acc := env.awsAccount(t)
	cmd := connection.RegisterAWSCommand{AccessKeyID: secondKeyID, SecretAccessKey: secondSecret, AllowedRegions: []string{"eu-west-1"}}
	next, err := env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, acc.Version, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if next.Version != 2 || next.SecretRef == acc.SecretRef || next.ID != acc.ID || next.Key != acc.Key || next.Name != acc.Name {
		t.Fatalf("replacement: %#v", next)
	}
	if got := connection.AccountRegions(next); len(got) != 1 || got[0] != "eu-west-1" {
		t.Fatalf("evidence regions: %v", got)
	}
	// The prior object stays resolvable for persisted targets that pin it.
	if value, err := creds.Get(ctx, "acme", acc.Key, acc.SecretRef); err != nil || !strings.Contains(string(value), fixtureKeyID) {
		t.Fatalf("prior object must be kept: %v", err)
	}
	if value, err := creds.Get(ctx, "acme", acc.Key, next.SecretRef); err != nil || !strings.Contains(string(value), secondKeyID) {
		t.Fatalf("new object: %v", err)
	}
	if len(creds.deletes) != 0 {
		t.Fatalf("a successful replacement must delete nothing: %v", creds.deletes)
	}
	// The resolver used by execution reads the new key.
	resolver := connection.NewCredentialResolver(env.st, creds)
	resolved, err := resolver.ResolveAWS(ctx, "acme", acc.Key)
	if err != nil || resolved.AccessKeyID != secondKeyID {
		t.Fatalf("resolver after replacement: %v %#v", err, resolved.AccessKeyID)
	}
}

func TestReplaceAWSRejectsOtherAccountAndFailedVerificationWithoutWriting(t *testing.T) {
	ctx := context.Background()
	creds := &recordingCredentials{Store: credentialmemory.New()}
	env := newAccountEnv(t, creds, nil)
	acc := env.awsAccount(t)
	cmd := connection.RegisterAWSCommand{AccessKeyID: secondKeyID, SecretAccessKey: secondSecret, AllowedRegions: []string{"us-east-1"}}

	other := okVerification("us-east-1")
	other.AccountID, other.PrincipalARN = "999999999999", "arn:aws:iam::999999999999:user/orch"
	env.aws.result = other
	if _, err := env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, acc.Version, cmd); !errors.Is(err, connection.ErrTargetMismatch) {
		t.Fatalf("other account must be refused: %v", err)
	}
	env.aws.result = connection.AWSVerification{}
	env.aws.err = connection.ErrPermissionDenied
	if _, err := env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, acc.Version, cmd); !errors.Is(err, connection.ErrPermissionDenied) {
		t.Fatalf("verification failure: %v", err)
	}
	if len(creds.puts) != 1 {
		t.Fatalf("nothing may be written before verification and target proof passed: %v", creds.puts)
	}
	current, _ := env.st.GetConnection(ctx, "acme", acc.Key)
	if current.Version != 1 || current.SecretRef != acc.SecretRef {
		t.Fatalf("committed record changed: %#v", current)
	}
	// Wrong shape.
	bad := cmd
	bad.AccessKeyID = "nope"
	var fe *connection.FieldError
	if _, err := env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, 1, bad); !errors.As(err, &fe) || fe.Field != "accessKeyId" {
		t.Fatalf("bad key id: %v", err)
	}
	// Wrong account type: AWS replacement on an Internal account.
	internal := env.internalAccount(t)
	if _, err := env.svc.ReplaceAWSCredentials(ctx, "acme", internal.Key, internal.Version, cmd); !errors.Is(err, connection.ErrWrongKind) {
		t.Fatalf("wrong kind: %v", err)
	}
}

// failAfterCommit fails the first UpdateConnection after committing it (a
// lost commit response); failBefore fails without committing.
type updateInterceptor struct {
	*store.Store
	mu          sync.Mutex
	failBefore  error
	afterCommit error
}

func (s *updateInterceptor) UpdateConnection(ctx context.Context, u persistence.ConnectionUpdate) (application.Connection, error) {
	s.mu.Lock()
	before, after := s.failBefore, s.afterCommit
	s.failBefore, s.afterCommit = nil, nil
	s.mu.Unlock()
	if before != nil {
		return application.Connection{}, before
	}
	conn, err := s.Store.UpdateConnection(ctx, u)
	if err == nil && after != nil {
		return application.Connection{}, after
	}
	return conn, err
}

func TestReplaceAttemptCleanupOnlyTouchesTheAttemptObject(t *testing.T) {
	ctx := context.Background()
	creds := &recordingCredentials{Store: credentialmemory.New()}
	inner := store.New()
	wrapped := &updateInterceptor{Store: inner}
	env := newAccountEnv(t, creds, inner)
	env.svc = connection.NewService(wrapped)
	env.svc.SetKubeconfigRegistration(env.kube, creds)
	env.svc.SetAWSRegistration(env.aws)
	acc := env.awsAccount(t)
	cmd := connection.RegisterAWSCommand{AccessKeyID: secondKeyID, SecretAccessKey: secondSecret, AllowedRegions: []string{"us-east-1"}}

	// A database failure after the attempt object was written.
	wrapped.failBefore = errors.New("database down")
	if _, err := env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, acc.Version, cmd); err == nil {
		t.Fatal("expected failure")
	}
	if len(creds.puts) != 2 || len(creds.deletes) != 1 || creds.deletes[0].ref != creds.puts[1] || creds.deletes[0].ref == acc.SecretRef {
		t.Fatalf("only the attempt object may be deleted: puts=%v deletes=%v", creds.puts, creds.deletes)
	}
	if _, err := creds.Get(ctx, "acme", acc.Key, acc.SecretRef); err != nil {
		t.Fatalf("prior object lost: %v", err)
	}
	current, _ := inner.GetConnection(ctx, "acme", acc.Key)
	if current.Version != 1 || current.SecretRef != acc.SecretRef {
		t.Fatalf("record changed: %#v", current)
	}

	// Lost commit response: the attempt is the committed ref and must survive.
	creds.deletes = nil
	wrapped.afterCommit = errors.New("connection reset")
	committed, err := env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, 1, cmd)
	if err != nil {
		t.Fatalf("a commit that landed is a success: %v", err)
	}
	if len(creds.deletes) != 0 {
		t.Fatalf("committed object deleted: %v", creds.deletes)
	}
	if _, err := creds.Get(ctx, "acme", acc.Key, committed.SecretRef); err != nil || committed.Version != 2 {
		t.Fatalf("committed object: %v %#v", err, committed)
	}
}

func TestReplaceCleanupFailureIsObservableAndSafe(t *testing.T) {
	ctx := context.Background()
	creds := &deleteFailing{Store: credentialmemory.New()}
	inner := store.New()
	wrapped := &updateInterceptor{Store: inner}
	env := newAccountEnv(t, creds, inner)
	env.svc = connection.NewService(wrapped)
	env.svc.SetKubeconfigRegistration(env.kube, creds)
	env.svc.SetAWSRegistration(env.aws)
	acc := env.awsAccount(t)
	wrapped.failBefore = errors.New("database down")
	_, err := env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, acc.Version, connection.RegisterAWSCommand{AccessKeyID: secondKeyID, SecretAccessKey: secondSecret, AllowedRegions: []string{"us-east-1"}})
	var cleanup *connection.CleanupError
	if !errors.As(err, &cleanup) || cleanup.Reference == "" || strings.Contains(err.Error(), secondSecret) || strings.Contains(err.Error(), "vault down") {
		t.Fatalf("cleanup failure must be a safe reference: %v", err)
	}
	if creds.deletes != 1 {
		t.Fatalf("deletes: %d", creds.deletes)
	}
}

func TestCancelledRequestStillCleansTheAttemptWithBoundedContext(t *testing.T) {
	creds := &recordingCredentials{Store: credentialmemory.New()}
	inner := store.New()
	wrapped := &updateInterceptor{Store: inner}
	env := newAccountEnv(t, creds, inner)
	env.svc = connection.NewService(wrapped)
	env.svc.SetKubeconfigRegistration(env.kube, creds)
	env.svc.SetAWSRegistration(env.aws)
	acc := env.awsAccount(t)
	ctx, cancel := context.WithCancel(context.Background())
	wrapped.failBefore = context.Canceled
	cancel()
	// A request cancelled after the attempt object was written: Put ignores
	// the cancelled context in the fake, the CAS fails, cleanup must run.
	_, err := env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, acc.Version, connection.RegisterAWSCommand{AccessKeyID: secondKeyID, SecretAccessKey: secondSecret, AllowedRegions: []string{"us-east-1"}})
	if err == nil {
		t.Fatal("expected failure")
	}
	if len(creds.deletes) != 1 || creds.deletes[0].ctxErr != nil || !creds.deletes[0].hasDeadline {
		t.Fatalf("cleanup must use a live bounded context: %#v", creds.deletes)
	}
}

// gatedCredentials records deletions; barrierVerifier makes racing
// replacements all pass their guards and verification before any CAS runs.
type gatedCredentials struct {
	*credentialmemory.Store
	mu      sync.Mutex
	deletes []string
}

func (s *gatedCredentials) Delete(ctx context.Context, org, conn, ref string) error {
	s.mu.Lock()
	s.deletes = append(s.deletes, ref)
	s.mu.Unlock()
	return s.Store.Delete(ctx, org, conn, ref)
}

type barrierVerifier struct {
	*awsVerifier
	ready chan struct{}
	n     int
	mu    sync.Mutex
	seen  int
}

func (v *barrierVerifier) VerifyAWS(ctx context.Context, key connection.AWSAccessKey, regions []string) (connection.AWSVerification, error) {
	out, err := v.awsVerifier.VerifyAWS(ctx, key, regions)
	v.mu.Lock()
	v.seen++
	if v.seen == v.n {
		close(v.ready)
	}
	v.mu.Unlock()
	<-v.ready
	return out, err
}

func TestConcurrentReplacementsHaveOneWinnerAndOneCleanedLoser(t *testing.T) {
	ctx := context.Background()
	creds := &gatedCredentials{Store: credentialmemory.New()}
	env := newAccountEnv(t, creds, nil)
	acc := env.awsAccount(t)
	env.svc.SetAWSRegistration(&barrierVerifier{awsVerifier: env.aws, ready: make(chan struct{}), n: 2})
	keys := []connection.RegisterAWSCommand{
		{AccessKeyID: secondKeyID, SecretAccessKey: secondSecret, AllowedRegions: []string{"us-east-1"}},
		{AccessKeyID: "AKIAFIXTUREKEY000003", SecretAccessKey: "third/secret/value/000000000000000000000", AllowedRegions: []string{"us-east-1"}},
	}
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = env.svc.ReplaceAWSCredentials(ctx, "acme", acc.Key, acc.Version, keys[i])
		}()
	}
	wg.Wait()
	wins, stale := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, persistence.ErrVersionConflict):
			stale++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	if wins != 1 || stale != 1 {
		t.Fatalf("wins=%d stale=%d", wins, stale)
	}
	current, _ := env.st.GetConnection(ctx, "acme", acc.Key)
	if current.Version != 2 || current.SecretRef == acc.SecretRef {
		t.Fatalf("winner record: %#v", current)
	}
	if len(creds.deletes) != 1 || creds.deletes[0] == current.SecretRef || creds.deletes[0] == acc.SecretRef {
		t.Fatalf("the loser must clean only its own attempt: %v", creds.deletes)
	}
	if creds.Len() != 2 {
		t.Fatalf("expected prior + winner objects, got %d", creds.Len())
	}
	if _, err := creds.Get(ctx, "acme", acc.Key, current.SecretRef); err != nil {
		t.Fatalf("winner's object missing: %v", err)
	}
}

func TestReplaceInternalRequiresExactSameTarget(t *testing.T) {
	ctx := context.Background()
	creds := &recordingCredentials{Store: credentialmemory.New()}
	env := newAccountEnv(t, creds, nil)
	acc := env.internalAccount(t)
	puts := len(creds.puts)
	for name, doc := range map[string]struct{ document, context string }{
		"other endpoint": {ct.Single("lab", "https://other.example:6443", "another-token-value-01"), "lab"},
		"other context":  {ct.Single("lab2", "https://lab.example:6443", "another-token-value-01"), "lab2"},
		"other trust": {ct.Document(ct.Context{Name: "lab", Cluster: "lab-cluster", User: "lab-user", Server: "https://lab.example:6443",
			ClusterExtra: []string{"insecure-skip-tls-verify: true"}, UserExtra: []string{"token: another-token-value-01"}}), "lab"},
	} {
		if _, err := env.svc.ReplaceInternalCredentials(ctx, "acme", acc.Key, acc.Version, doc.document, doc.context); !errors.Is(err, connection.ErrTargetMismatch) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if len(creds.puts) != puts || len(env.kube.selected) != 1 {
		t.Fatalf("a redirected target must not be verified or written: puts %d, verifies %d", len(creds.puts)-puts, len(env.kube.selected)-1)
	}
	// Same cluster/context/endpoint/trust with a rotated token is accepted.
	next, err := env.svc.ReplaceInternalCredentials(ctx, "acme", acc.Key, acc.Version, ct.Single("lab", "https://lab.example:6443", "rotated-token-value-02"), "lab")
	if err != nil || next.Version != 2 || next.SecretRef == acc.SecretRef {
		t.Fatalf("same-target replacement: %v %#v", err, next)
	}
	value, err := creds.Get(ctx, "acme", acc.Key, next.SecretRef)
	if err != nil || !strings.Contains(string(value), "rotated-token-value-02") {
		t.Fatalf("stored replacement: %v", err)
	}
	if _, err := creds.Get(ctx, "acme", acc.Key, acc.SecretRef); err != nil {
		t.Fatalf("prior object must remain: %v", err)
	}
	// An AWS account cannot take a kubeconfig, and the failure leaves it intact.
	aws := env.awsAccount(t)
	if _, err := env.svc.ReplaceInternalCredentials(ctx, "acme", aws.Key, aws.Version, ct.Single("lab", "https://lab.example:6443", secretToken), "lab"); !errors.Is(err, connection.ErrWrongKind) {
		t.Fatalf("wrong kind: %v", err)
	}
	// The resolver used by execution reads the replacement.
	resolver := connection.NewCredentialResolver(env.st, creds)
	doc, _, err := resolver.ResolveKubernetes(ctx, "acme", acc.Key)
	if err != nil || !strings.Contains(string(doc), "rotated-token-value-02") {
		t.Fatalf("resolver: %v", err)
	}
}
