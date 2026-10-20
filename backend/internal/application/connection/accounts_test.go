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
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// FAKE verifiers and in-memory credential store: these tests prove account
// orchestration (version CAS, same-target guards, attempt cleanup, retained
// prior objects, safe failure). They are not evidence that AWS or a cluster
// accepts a credential.

const (
	newKeyID  = "AKIAFIXTUREKEY000002"
	newSecret = "fixture/secret/value/1111111111111111111"
)

type accountFixture struct {
	st    *store.Store
	creds *recordingCredentials
	aws   *awsVerifier
	kube  *kubeconfigVerifier
	svc   *connection.Service
}

func newAccountFixture(t *testing.T) *accountFixture {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	for _, key := range []string{"acme", "other"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: key, Name: key}); err != nil {
			t.Fatal(err)
		}
	}
	f := &accountFixture{st: st, creds: &recordingCredentials{Store: credentialmemory.New()}, aws: &awsVerifier{}, kube: &kubeconfigVerifier{}}
	f.svc = connection.NewService(st)
	f.svc.SetAWSRegistration(f.aws)
	f.svc.SetKubeconfigRegistration(f.kube, f.creds)
	return f
}

func (f *accountFixture) awsAccount(t *testing.T) application.Connection {
	t.Helper()
	created, err := f.svc.RegisterAWSDriverAccount(context.Background(), "acme", awsCommand())
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func internalDoc(token string) string { return ct.Single("lab", "https://lab.example:6443", token) }

func (f *accountFixture) internalAccount(t *testing.T) application.Connection {
	t.Helper()
	created, err := f.svc.RegisterKubeconfig(context.Background(), "acme", connection.RegisterKubeconfigCommand{Name: "Lab", Kubeconfig: internalDoc("token-one"), Context: "lab"})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func TestNewAccountsStartAtVersionOneWithVerificationEvidence(t *testing.T) {
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	if acct.Version != 1 || strings.Join(acct.ConfigStrings("verificationRegions"), ",") != "us-east-1,eu-west-1" || acct.Verification["verifiedAt"] == "" {
		t.Fatalf("aws account: %#v", acct)
	}
	if k := f.internalAccount(t); k.Version != 1 || k.ConfigString("tlsTrust") == "" || k.ConfigString("tlsMode") != "SYSTEM_ROOTS" {
		t.Fatalf("internal account: %#v", k)
	}
}

func TestRenameKeepsIdentityTargetRefAndEvidenceAndBumpsVersionOnce(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	before := f.awsAccount(t)
	after, err := f.svc.RenameAccount(ctx, "acme", before.Key, 1, "  Tên mới  ")
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != "Tên mới" || after.Version != 2 || after.ID != before.ID || after.Key != before.Key || after.SecretRef != before.SecretRef ||
		after.ConfigString("accountId") != before.ConfigString("accountId") || after.Verification["verifiedAt"] != before.Verification["verifiedAt"] ||
		after.Kind != before.Kind || after.AuthenticationType != before.AuthenticationType {
		t.Fatalf("before %#v after %#v", before, after)
	}
	if f.aws.calls != 1 || f.creds.Len() != 1 {
		t.Fatalf("rename touched the provider (%d) or the credential store (%d)", f.aws.calls, f.creds.Len())
	}
}

func TestRenameValidationStaleAndScope(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	var field *connection.FieldError
	for _, name := range []string{"", "   ", strings.Repeat("x", 101), "bad\x00name"} {
		if _, err := f.svc.RenameAccount(ctx, "acme", acct.Key, 1, name); !errors.As(err, &field) || field.Field != "name" {
			t.Errorf("name %q: %v", name, err)
		}
	}
	if _, err := f.svc.RenameAccount(ctx, "acme", acct.Key, 0, "ok"); !errors.As(err, &field) || field.Field != "version" {
		t.Errorf("version 0: %v", err)
	}
	if _, err := f.svc.RenameAccount(ctx, "acme", acct.Key, 7, "ok"); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Errorf("stale: %v", err)
	}
	if _, err := f.svc.RenameAccount(ctx, "other", acct.Key, 1, "ok"); !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("foreign organization: %v", err)
	}
	if _, err := f.svc.GetAccount(ctx, "other", acct.Key); !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("foreign get: %v", err)
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.Name != acct.Name || got.Version != 1 {
		t.Fatalf("a rejected rename changed the record: %#v", got)
	}
}

func TestConcurrentRenamesLetExactlyOneWinnerWin(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	const racers = 12
	var wg sync.WaitGroup
	results := make(chan error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.RenameAccount(ctx, "acme", acct.Key, 1, "Racer")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	wins, stale := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, persistence.ErrVersionConflict):
			stale++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	if wins != 1 || stale != racers-1 {
		t.Fatalf("wins %d stale %d", wins, stale)
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.Version != 2 {
		t.Fatalf("version %d", got.Version)
	}
}

func TestVerifyAWSPublishesOnlyOnSuccessAndKeepsPriorRecordOnFailure(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	f.aws.mu.Lock()
	f.aws.err = connection.ErrAuthentication
	f.aws.mu.Unlock()
	if _, err := f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 1}); !errors.Is(err, connection.ErrVerification) || !errors.Is(err, connection.ErrAuthentication) {
		t.Fatalf("failure: %v", err)
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.Version != 1 || got.Status != application.ConnectionReady || got.SecretRef != acct.SecretRef || got.Verification["verifiedAt"] != acct.Verification["verifiedAt"] {
		t.Fatalf("a failed check changed the record: %#v", got)
	}
	f.aws.mu.Lock()
	f.aws.err = nil
	f.aws.mu.Unlock()
	f.svc = withClock(f, time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	updated, err := f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 1, VerificationRegions: []string{"ap-southeast-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Verification["verifiedAt"] != "2026-10-17T09:00:00Z" || strings.Join(updated.ConfigStrings("verificationRegions"), ",") != "ap-southeast-1" {
		t.Fatalf("updated: %#v", updated)
	}
	// The legacy allowed-region policy is untouched by the new evidence list.
	if strings.Join(updated.AllowedRegions(), ",") != "us-east-1,eu-west-1" {
		t.Fatalf("legacy allowed regions changed: %v", updated.AllowedRegions())
	}
	// Omitting regions rechecks the stored evidence list.
	if _, err := f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 2}); err != nil {
		t.Fatal(err)
	}
	last := f.aws.asked[len(f.aws.asked)-1]
	if strings.Join(last, ",") != "ap-southeast-1" {
		t.Fatalf("asked %v", last)
	}
	if _, err := f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 2}); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale verify: %v", err)
	}
}

func withClock(f *accountFixture, at time.Time) *connection.Service {
	svc := connection.NewService(f.st)
	svc.SetAWSRegistration(f.aws)
	svc.SetKubeconfigRegistration(f.kube, f.creds)
	svc.SetClock(func() time.Time { return at })
	return svc
}

func TestVerifyRejectsAnotherAWSAccountAndInternalRegions(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	f.aws.mu.Lock()
	f.aws.result = connection.AWSVerification{AccountID: "999999999999", PrincipalARN: "arn:aws:iam::999999999999:user/x", Regions: []string{"us-east-1", "eu-west-1"}, Capabilities: []string{"Aurora", "EKS", "ElastiCache", "VPC"}}
	f.aws.mu.Unlock()
	if _, err := f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 1}); !errors.Is(err, connection.ErrAccountIncompatible) {
		t.Fatalf("other account verify: %v", err)
	}
	k := f.internalAccount(t)
	var field *connection.FieldError
	if _, err := f.svc.VerifyAccount(ctx, "acme", k.Key, connection.VerifyAccountCommand{Version: 1, VerificationRegions: []string{"us-east-1"}}); !errors.As(err, &field) || field.Field != "verificationRegions" {
		t.Fatalf("internal with regions: %v", err)
	}
	if _, err := f.svc.VerifyAccount(ctx, "acme", k.Key, connection.VerifyAccountCommand{Version: 1}); err != nil {
		t.Fatal(err)
	}
	f.kube.err = connection.ErrClusterUnreachable
	if _, err := f.svc.VerifyAccount(ctx, "acme", k.Key, connection.VerifyAccountCommand{Version: 2}); !errors.Is(err, connection.ErrVerification) || !errors.Is(err, connection.ErrClusterUnreachable) {
		t.Fatalf("unreachable: %v", err)
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", k.Key); got.Version != 2 {
		t.Fatalf("failed check bumped the version: %d", got.Version)
	}
}

func TestVerifyMissingStoredCredentialIsAccountCredentialError(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	if err := f.creds.Store.Delete(ctx, "acme", acct.Key, acct.SecretRef); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 1}); !errors.Is(err, connection.ErrAccountCredential) {
		t.Fatalf("missing object: %v", err)
	}
}

func TestReplaceAWSCredentialSameAccountKeepsPriorObjectAndPublishesAtomically(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	updated, err := f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, connection.ReplaceAWSCommand{Version: 1, AccessKeyID: newKeyID, SecretAccessKey: newSecret, VerificationRegions: []string{"us-east-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.SecretRef == acct.SecretRef || updated.ID != acct.ID || updated.ConfigString("accountId") != acct.ConfigString("accountId") {
		t.Fatalf("updated: %#v", updated)
	}
	// Both objects remain: persisted targets may pin the prior one.
	if _, err := f.creds.Get(ctx, "acme", acct.Key, acct.SecretRef); err != nil {
		t.Fatalf("prior object deleted: %v", err)
	}
	value, err := f.creds.Get(ctx, "acme", acct.Key, updated.SecretRef)
	if err != nil || !strings.Contains(string(value), newKeyID) {
		t.Fatalf("new object: %v", err)
	}
	if len(f.creds.deletes) != 0 {
		t.Fatalf("a successful replacement deleted %v", f.creds.deletes)
	}
	// The resolver (legacy consumer path) reads the new key.
	got, err := connection.NewCredentialResolver(f.st, f.creds).ResolveAWS(ctx, "acme", acct.Key)
	if err != nil || got.AccessKeyID != newKeyID || !got.AllowsRegion("eu-west-1") {
		t.Fatalf("resolved %+v: %v", got, err)
	}
	if strings.Contains(toJSON(t, updated), newSecret) {
		t.Fatal("record holds the secret")
	}
}

func TestReplaceAWSRejectsWrongTargetVerificationFailureAndBadInputBeforeWriting(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	cmd := connection.ReplaceAWSCommand{Version: 1, AccessKeyID: newKeyID, SecretAccessKey: newSecret, VerificationRegions: []string{"us-east-1"}}

	f.aws.mu.Lock()
	f.aws.result = connection.AWSVerification{AccountID: "999999999999", PrincipalARN: "arn:aws:iam::999999999999:user/x", Regions: []string{"us-east-1", "eu-west-1"}, Capabilities: []string{"Aurora", "EKS", "ElastiCache", "VPC"}}
	f.aws.mu.Unlock()
	if _, err := f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, cmd); !errors.Is(err, connection.ErrAccountIncompatible) {
		t.Fatalf("redirected account: %v", err)
	}
	f.aws.mu.Lock()
	f.aws.result = connection.AWSVerification{}
	f.aws.err = connection.ErrPermissionDenied
	f.aws.mu.Unlock()
	if _, err := f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, cmd); !errors.Is(err, connection.ErrVerification) {
		t.Fatalf("verification failure: %v", err)
	}
	f.aws.mu.Lock()
	f.aws.err = nil
	f.aws.mu.Unlock()
	var field *connection.FieldError
	for name, bad := range map[string]connection.ReplaceAWSCommand{
		"accessKeyId":         {Version: 1, AccessKeyID: "ASIAFIXTUREKEY000002", SecretAccessKey: newSecret, VerificationRegions: []string{"us-east-1"}},
		"secretAccessKey":     {Version: 1, AccessKeyID: newKeyID, SecretAccessKey: "short", VerificationRegions: []string{"us-east-1"}},
		"verificationRegions": {Version: 1, AccessKeyID: newKeyID, SecretAccessKey: newSecret},
	} {
		if _, err := f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, bad); !errors.As(err, &field) || field.Field != name {
			t.Errorf("%s: %v", name, err)
		}
	}
	if f.creds.Len() != 1 || len(f.creds.puts) != 1 {
		t.Fatalf("a rejected replacement wrote an object: %d objects, %d puts", f.creds.Len(), len(f.creds.puts))
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.Version != 1 || got.SecretRef != acct.SecretRef {
		t.Fatalf("record changed: %#v", got)
	}
	if _, err := f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, connection.ReplaceAWSCommand{Version: 1}); err == nil {
		t.Fatal("empty command accepted")
	}
}

// hookCredentials wraps the credential store: a callback right after the
// attempt object is written (where a concurrent writer or a cancellation
// interleaves) and an optional failing Delete.
type hookCredentials struct {
	*recordingCredentials
	afterPut   func()
	deleteFail bool
}

func (s *hookCredentials) Put(ctx context.Context, org, conn string, value []byte) (string, error) {
	ref, err := s.recordingCredentials.Put(ctx, org, conn, value)
	if err == nil && s.afterPut != nil {
		hook := s.afterPut
		s.afterPut = nil
		hook()
	}
	return ref, err
}

func (s *hookCredentials) Delete(ctx context.Context, org, conn, ref string) error {
	if s.deleteFail {
		s.recordingCredentials.deletes = append(s.recordingCredentials.deletes, deleteCall{org: org, connection: conn, ref: ref})
		return errors.New("vault down")
	}
	return s.recordingCredentials.Delete(ctx, org, conn, ref)
}

// ctxStore refuses a transaction of a cancelled request, like a real database.
type ctxStore struct{ *store.Store }

func (s ctxStore) Transact(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Store.Transact(ctx, fn)
}

func replaceCommand() connection.ReplaceAWSCommand {
	return connection.ReplaceAWSCommand{Version: 1, AccessKeyID: newKeyID, SecretAccessKey: newSecret, VerificationRegions: []string{"us-east-1"}}
}

func (f *accountFixture) serviceWith(creds *hookCredentials, st interface {
	persistence.Store
}) *connection.Service {
	svc := connection.NewService(st)
	svc.SetAWSRegistration(f.aws)
	svc.SetKubeconfigRegistration(f.kube, creds)
	return svc
}

func TestReplaceLosingTheRaceDeletesOnlyItsOwnAttemptObject(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	hooked := &hookCredentials{recordingCredentials: f.creds}
	svc := f.serviceWith(hooked, f.st)
	hooked.afterPut = func() {
		if _, err := f.svc.RenameAccount(ctx, "acme", acct.Key, 1, "Winner"); err != nil {
			t.Error(err)
		}
	}
	if _, err := svc.ReplaceAWSCredential(ctx, "acme", acct.Key, replaceCommand()); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("loser: %v", err)
	}
	if len(f.creds.puts) != 2 || len(f.creds.deletes) != 1 || f.creds.deletes[0].ref != f.creds.puts[1] {
		t.Fatalf("puts %v deletes %+v", f.creds.puts, f.creds.deletes)
	}
	if f.creds.Len() != 1 {
		t.Fatalf("objects: %d", f.creds.Len())
	}
	if _, err := f.creds.Get(ctx, "acme", acct.Key, acct.SecretRef); err != nil {
		t.Fatalf("committed object lost: %v", err)
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.Name != "Winner" || got.Version != 2 || got.SecretRef != acct.SecretRef {
		t.Fatalf("winner overwritten: %#v", got)
	}
}

func TestReplaceCleanupSurvivesRequestCancellation(t *testing.T) {
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	ctx, cancel := context.WithCancel(context.Background())
	hooked := &hookCredentials{recordingCredentials: f.creds, afterPut: cancel}
	svc := f.serviceWith(hooked, ctxStore{f.st})
	if _, err := svc.ReplaceAWSCredential(ctx, "acme", acct.Key, replaceCommand()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled replacement: %v", err)
	}
	if len(f.creds.deletes) != 1 || f.creds.deletes[0].ctxErr != nil || !f.creds.deletes[0].hasDeadline || f.creds.deletes[0].remaining > 16*time.Second {
		t.Fatalf("cleanup context: %+v", f.creds.deletes)
	}
	if f.creds.Len() != 1 {
		t.Fatalf("attempt object left behind: %d", f.creds.Len())
	}
	if _, err := f.creds.Get(context.Background(), "acme", acct.Key, acct.SecretRef); err != nil {
		t.Fatalf("committed object lost: %v", err)
	}
}

func TestReplaceCleanupFailureIsObservableAndNeverSuccess(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	hooked := &hookCredentials{recordingCredentials: f.creds, deleteFail: true}
	svc := f.serviceWith(hooked, f.st)
	hooked.afterPut = func() {
		if _, err := f.svc.RenameAccount(ctx, "acme", acct.Key, 1, "Winner"); err != nil {
			t.Error(err)
		}
	}
	_, err := svc.ReplaceAWSCredential(ctx, "acme", acct.Key, replaceCommand())
	var cleanup *connection.CleanupError
	if !errors.As(err, &cleanup) || cleanup.Reference == "" {
		t.Fatalf("cleanup failure: %v", err)
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.SecretRef != acct.SecretRef {
		t.Fatalf("a failed replacement published: %#v", got)
	}
}

func TestReplaceKubeconfigRequiresTheSameClusterContextEndpointAndTrust(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.internalAccount(t)
	verifiedBefore := len(f.kube.selected)
	replace := func(doc, context_ string) error {
		_, err := f.svc.ReplaceKubeconfigCredential(ctx, "acme", acct.Key, connection.ReplaceKubeconfigCommand{Version: 1, Kubeconfig: doc, Context: context_})
		return err
	}
	insecure := ct.Context{Name: "lab", Cluster: "lab-cluster", User: "lab-user", Server: "https://lab.example:6443", ClusterExtra: []string{"insecure-skip-tls-verify: true"}, UserExtra: []string{"token: token-two"}}
	for name, doc := range map[string]string{
		"other endpoint": ct.Single("lab", "https://elsewhere.example:6443", "token-two"),
		"other context":  ct.Single("lab2", "https://lab.example:6443", "token-two"),
		"other CA":       ct.Document(func() ct.Context { c := ct.CertificateContext("lab", "https://lab.example:6443"); return c }()),
		"insecure TLS":   ct.Document(insecure),
	} {
		ctxName := "lab"
		if name == "other context" {
			ctxName = "lab2"
		}
		if err := replace(doc, ctxName); !errors.Is(err, connection.ErrAccountIncompatible) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(f.kube.selected) != verifiedBefore || f.creds.Len() != 1 {
		t.Fatalf("a redirected replacement reached the verifier (%d) or the store (%d)", len(f.kube.selected), f.creds.Len())
	}
	if err := replace(internalDoc("token-two"), "lab"); err != nil {
		t.Fatalf("same target: %v", err)
	}
	got, _ := f.svc.GetAccount(ctx, "acme", acct.Key)
	if got.Version != 2 || got.SecretRef == acct.SecretRef {
		t.Fatalf("not published: %#v", got)
	}
	value, err := connection.NewCredentialResolver(f.st, f.creds).ResolveKubeconfig(ctx, executionTarget("acme", acct.Key))
	if err != nil || !strings.Contains(string(value), "token-two") {
		t.Fatalf("resolved: %v", err)
	}
	if _, err := f.creds.Get(ctx, "acme", acct.Key, acct.SecretRef); err != nil {
		t.Fatalf("prior kubeconfig object deleted: %v", err)
	}
	f.kube.err = connection.ErrAuthentication
	if _, err := f.svc.ReplaceKubeconfigCredential(ctx, "acme", acct.Key, connection.ReplaceKubeconfigCommand{Version: 2, Kubeconfig: internalDoc("token-three"), Context: "lab"}); !errors.Is(err, connection.ErrVerification) {
		t.Fatalf("verification failure: %v", err)
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.Version != 2 {
		t.Fatalf("failed replacement bumped version")
	}
}

func TestReplaceKubeconfigDerivesTrustFromTheStoredCredentialForOlderRecords(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.internalAccount(t)
	delete(acct.Config, "tlsTrust")
	if err := f.st.SaveConnection(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ReplaceKubeconfigCredential(ctx, "acme", acct.Key, connection.ReplaceKubeconfigCommand{Version: 1, Kubeconfig: internalDoc("token-two"), Context: "lab"}); err != nil {
		t.Fatalf("same target: %v", err)
	}
	g := ct.Context{Name: "lab", Cluster: "lab-cluster", User: "lab-user", Server: "https://lab.example:6443", ClusterExtra: []string{"insecure-skip-tls-verify: true"}, UserExtra: []string{"token: x"}}
	if _, err := f.svc.ReplaceKubeconfigCredential(ctx, "acme", acct.Key, connection.ReplaceKubeconfigCommand{Version: 2, Kubeconfig: ct.Document(g), Context: "lab"}); !errors.Is(err, connection.ErrAccountIncompatible) {
		t.Fatalf("trust change: %v", err)
	}
}

func TestStoreFailureDuringPutNeverPublishesAndCleansItsReference(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	f.creds.Store.FailPut = errors.New("vault down")
	_, err := f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, connection.ReplaceAWSCommand{Version: 1, AccessKeyID: newKeyID, SecretAccessKey: newSecret, VerificationRegions: []string{"us-east-1"}})
	if !errors.Is(err, connection.ErrCredentialStore) {
		t.Fatalf("put failure: %v", err)
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.Version != 1 || got.SecretRef != acct.SecretRef {
		t.Fatalf("record changed: %#v", got)
	}
}

func executionTarget(org, key string) execution.Target {
	return execution.Target{Kind: "kubernetes", Organization: org, Connection: key}
}

func TestReplaceVerifiesEveryRetainedLegacyRegionBeforePublishing(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t) // legacy allowed regions: us-east-1, eu-west-1
	cmd := replaceCommand() // new evidence regions: us-east-1 only

	f.aws.mu.Lock()
	f.aws.denyRegion = "eu-west-1"
	f.aws.mu.Unlock()
	if _, err := f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, cmd); !errors.Is(err, connection.ErrVerification) || !errors.Is(err, connection.ErrRegionUnavailable) {
		t.Fatalf("new key unusable in a retained legacy region: %v", err)
	}
	if f.creds.Len() != 1 || len(f.creds.puts) != 1 {
		t.Fatalf("a refused replacement wrote an object: %d objects", f.creds.Len())
	}
	if got, _ := f.svc.GetAccount(ctx, "acme", acct.Key); got.Version != 1 || got.SecretRef != acct.SecretRef || toJSON(t, got.Verification) != toJSON(t, acct.Verification) {
		t.Fatalf("old ref/version/evidence changed: %#v", got)
	}
	if got, err := connection.NewCredentialResolver(f.st, f.creds).ResolveAWS(ctx, "acme", acct.Key); err != nil || got.AccessKeyID != fixtureKeyID {
		t.Fatalf("the old credential is no longer the one legacy consumers resolve: %v", err)
	}

	f.aws.mu.Lock()
	f.aws.denyRegion = ""
	f.aws.mu.Unlock()
	updated, err := f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.aws.asked[len(f.aws.asked)-1]; strings.Join(got, ",") != "us-east-1,eu-west-1" {
		t.Fatalf("checked regions %v", got)
	}
	// Public evidence list is what was requested; legacy policy is untouched;
	// the recorded verification belongs to the new key and covers every region.
	if strings.Join(updated.ConfigStrings("verificationRegions"), ",") != "us-east-1" || strings.Join(updated.AllowedRegions(), ",") != "us-east-1,eu-west-1" ||
		strings.Join(updated.Config["allowedRegions"].([]string), ",") != "us-east-1,eu-west-1" || updated.ConfigString("region") != "us-east-1" {
		t.Fatalf("config: %#v", updated.Config)
	}
	if regions, _ := updated.Verification["regions"].([]string); strings.Join(regions, ",") != "us-east-1,eu-west-1" {
		t.Fatalf("verification evidence: %#v", updated.Verification)
	}
}

// uncertainStore commits the transaction and then reports an unknown commit
// outcome, like a lost COMMIT acknowledgement.
type uncertainStore struct {
	*store.Store
	armed bool
}

func (s *uncertainStore) Transact(ctx context.Context, fn func(context.Context) error) error {
	err := s.Store.Transact(ctx, fn)
	if err == nil && s.armed {
		return persistence.ErrCommitUncertain
	}
	return err
}

// rollbackStore fails the transaction without running it: a known rollback.
type rollbackStore struct {
	*store.Store
	fail error
}

func (s *rollbackStore) Transact(ctx context.Context, fn func(context.Context) error) error {
	if s.fail != nil {
		return s.fail
	}
	return s.Store.Transact(ctx, fn)
}

func TestUncertainCommitKeepsEveryObjectForCreateAndReplaceAndNeverRetries(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	unc := &uncertainStore{Store: f.st}
	svc := f.serviceWith(&hookCredentials{recordingCredentials: f.creds}, unc)

	// Replace: committed, response uncertain. Active and prior objects stay readable.
	unc.armed = true
	_, err := svc.ReplaceAWSCredential(ctx, "acme", acct.Key, replaceCommand())
	if !errors.Is(err, connection.ErrOutcomeUncertain) {
		t.Fatalf("replace: %v", err)
	}
	got, _ := f.svc.GetAccount(ctx, "acme", acct.Key)
	if got.Version != 2 || got.SecretRef == acct.SecretRef {
		t.Fatalf("the fault store did commit: %#v", got)
	}
	if len(f.creds.deletes) != 0 || f.creds.Len() != 2 {
		t.Fatalf("an uncertain replacement deleted or lost an object: deletes %v, objects %d", f.creds.deletes, f.creds.Len())
	}
	for name, ref := range map[string]string{"prior": acct.SecretRef, "active": got.SecretRef} {
		if _, err := f.creds.Get(ctx, "acme", acct.Key, ref); err != nil {
			t.Fatalf("%s credential unreadable: %v", name, err)
		}
	}
	if r, err := connection.NewCredentialResolver(f.st, f.creds).ResolveAWS(ctx, "acme", acct.Key); err != nil || r.AccessKeyID != newKeyID {
		t.Fatalf("legacy resolve of the committed credential: %v", err)
	}
	if len(f.creds.puts) != 2 {
		t.Fatalf("the mutation was retried: %d puts", len(f.creds.puts))
	}

	// Create (AWS driver account and kubeconfig): the row exists, its object is kept, nothing is retried.
	putsBefore := len(f.creds.puts)
	_, err = svc.RegisterAWSDriverAccount(ctx, "acme", connection.RegisterAWSCommand{Name: "Another", AccessKeyID: fixtureKeyID, SecretAccessKey: fixtureSecret, AllowedRegions: []string{"us-east-1"}})
	if !errors.Is(err, connection.ErrOutcomeUncertain) {
		t.Fatalf("create aws: %v", err)
	}
	_, err = svc.RegisterKubeconfig(ctx, "acme", connection.RegisterKubeconfigCommand{Name: "Kube", Kubeconfig: internalDoc("tok"), Context: "lab"})
	if !errors.Is(err, connection.ErrOutcomeUncertain) {
		t.Fatalf("create kubeconfig: %v", err)
	}
	if len(f.creds.puts)-putsBefore != 2 || len(f.creds.deletes) != 0 {
		t.Fatalf("create retried or deleted: puts +%d deletes %v", len(f.creds.puts)-putsBefore, f.creds.deletes)
	}
	for _, key := range []string{"another", "kube"} {
		rec, err := f.svc.GetAccount(ctx, "acme", key)
		if err != nil {
			t.Fatalf("committed %s account missing: %v", key, err)
		}
		if _, err := f.creds.Get(ctx, "acme", key, rec.SecretRef); err != nil {
			t.Fatalf("%s credential unreadable: %v", key, err)
		}
	}

	// A rename with an unknown outcome is also reported as uncertain.
	if _, err := svc.RenameAccount(ctx, "acme", acct.Key, 2, "Renamed"); !errors.Is(err, connection.ErrOutcomeUncertain) {
		t.Fatalf("rename: %v", err)
	}
}

func TestKnownRollbackStillCleansOnlyTheAttemptObject(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	acct := f.awsAccount(t)
	rb := &rollbackStore{Store: f.st, fail: errors.New("tx rolled back")}
	svc := f.serviceWith(&hookCredentials{recordingCredentials: f.creds}, rb)
	if _, err := svc.ReplaceAWSCredential(ctx, "acme", acct.Key, replaceCommand()); err == nil || errors.Is(err, connection.ErrOutcomeUncertain) {
		t.Fatalf("replace: %v", err)
	}
	if len(f.creds.deletes) != 1 || f.creds.Len() != 1 {
		t.Fatalf("known rollback: deletes %v objects %d", f.creds.deletes, f.creds.Len())
	}
	if _, err := f.creds.Get(ctx, "acme", acct.Key, acct.SecretRef); err != nil {
		t.Fatalf("committed object lost: %v", err)
	}
	// Creation: the rolled-back attempt's object is removed and not retried forever.
	before := f.creds.Len()
	if _, err := svc.RegisterAWSDriverAccount(ctx, "acme", connection.RegisterAWSCommand{Name: "Other", AccessKeyID: fixtureKeyID, SecretAccessKey: fixtureSecret, AllowedRegions: []string{"us-east-1"}}); err == nil || errors.Is(err, connection.ErrOutcomeUncertain) {
		t.Fatalf("create: %v", err)
	}
	if f.creds.Len() != before {
		t.Fatalf("a rolled-back create left an object: %d -> %d", before, f.creds.Len())
	}
}
