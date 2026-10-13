package connection_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
)

// FAKE verifier: these tests prove registration orchestration (validation,
// ordering, credential lifecycle, rollback, concurrency, redaction). They are
// not evidence that AWS accepts a key; see the awsaccess verifier tests and
// the real-AWS acceptance for that.

const (
	fixtureKeyID  = "AKIAFIXTUREKEY000001"
	fixtureSecret = "fixture/secret/value/0000000000000000000" // 40 chars
)

type awsVerifier struct {
	mu     sync.Mutex
	err    error
	result connection.AWSVerification
	calls  int
	keys   []connection.AWSAccessKey
	asked  [][]string
}

func okVerification(regions ...string) connection.AWSVerification {
	return connection.AWSVerification{
		AccountID: "123456789012", PrincipalARN: "arn:aws:iam::123456789012:user/orch",
		Regions: regions, Capabilities: []string{"Aurora", "EKS", "ElastiCache", "VPC"},
	}
}

func (v *awsVerifier) VerifyAWS(_ context.Context, key connection.AWSAccessKey, regions []string) (connection.AWSVerification, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls++
	v.keys = append(v.keys, key)
	v.asked = append(v.asked, append([]string(nil), regions...))
	if v.err != nil {
		return connection.AWSVerification{}, v.err
	}
	if v.result.AccountID != "" {
		return v.result, nil
	}
	return okVerification(regions...), nil
}

func awsFixture(t *testing.T) (*store.Store, *credentialmemory.Store, *awsVerifier, *connection.Service) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	for _, key := range []string{"acme", "other"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: key, Name: key, DefaultConnectionKey: "internal-cluster"}); err != nil {
			t.Fatal(err)
		}
	}
	creds := credentialmemory.New()
	v := &awsVerifier{}
	svc := connection.NewService(st)
	svc.SetAWSRegistration(v)
	svc.SetKubeconfigRegistration(nil, creds)
	return st, creds, v, svc
}

func awsCommand() connection.RegisterAWSCommand {
	return connection.RegisterAWSCommand{Name: "Tài khoản AWS lab", AccessKeyID: fixtureKeyID, SecretAccessKey: fixtureSecret, AllowedRegions: []string{"us-east-1", "eu-west-1"}}
}

func TestRegisterAWSStoresKeyOnlyInTheScopedStoreAndResolvesIt(t *testing.T) {
	ctx := context.Background()
	st, creds, v, svc := awsFixture(t)
	created, err := svc.RegisterAWS(ctx, "acme", awsCommand())
	if err != nil {
		t.Fatal(err)
	}
	if created.Key != "tai-khoan-aws-lab" || created.Kind != application.ConnectionAWS || created.AuthenticationType != application.AuthAWSAccessKey || created.Status != application.ConnectionReady {
		t.Fatalf("connection: %#v", created)
	}
	if created.ConfigString("accountId") != "123456789012" || created.ConfigString("region") != "us-east-1" ||
		strings.Join(created.AllowedRegions(), ",") != "us-east-1,eu-west-1" {
		t.Fatalf("config: %#v", created.Config)
	}
	if created.Verification["identity"] != "VERIFIED" || created.Verification["regionCheck"] != "VERIFIED" || created.Verification["capabilityCheck"] != "VERIFIED" {
		t.Fatalf("verification: %#v", created.Verification)
	}
	if v.calls != 1 || v.keys[0].AccessKeyID != fixtureKeyID || strings.Join(v.asked[0], ",") != "us-east-1,eu-west-1" {
		t.Fatalf("verifier saw %+v", v)
	}
	stored, err := st.GetConnection(ctx, "acme", created.Key)
	if err != nil {
		t.Fatal(err)
	}
	record := toJSON(t, stored)
	if strings.Contains(record, fixtureKeyID) || strings.Contains(record, fixtureSecret) || strings.Contains(record, "AKIA") {
		t.Fatalf("record holds credential material: %s", record)
	}
	if stored.SecretRef == "" || !strings.Contains(stored.SecretRef, "/acme/"+created.Key+"/credentials/") {
		t.Fatalf("secret reference is not scoped: %q", stored.SecretRef)
	}
	if org, _ := st.GetOrganization(ctx, "acme"); org.DefaultConnectionKey != "internal-cluster" {
		t.Fatalf("registration changed the organization default: %#v", org)
	}

	resolver := connection.NewCredentialResolver(st, creds)
	got, err := resolver.ResolveAWS(ctx, "acme", created.Key)
	if err != nil || got.AccessKeyID != fixtureKeyID || got.SecretAccessKey != fixtureSecret || got.AccountID != "123456789012" ||
		got.Region != "us-east-1" || !got.AllowsRegion("eu-west-1") || got.AllowsRegion("ap-south-1") {
		t.Fatalf("resolved %+v: %v", got, err)
	}
	if _, err := resolver.ResolveAWS(ctx, "other", created.Key); !errors.Is(err, connection.ErrCredentialUnavailable) {
		t.Fatalf("another organization resolved the key: %v", err)
	}
	if _, err := creds.Get(ctx, "other", created.Key, stored.SecretRef); err == nil {
		t.Fatal("credential readable from another organization")
	}
}

func TestRegisterAWSValidationRejectsBeforeAnyEffect(t *testing.T) {
	ctx := context.Background()
	long := make([]string, connection.MaxAllowedRegions+1)
	for i := range long {
		long[i] = fmt.Sprintf("us-east-%d", i+1)
	}
	mutate := func(f func(*connection.RegisterAWSCommand)) connection.RegisterAWSCommand {
		c := awsCommand()
		f(&c)
		return c
	}
	cases := map[string]struct {
		cmd   connection.RegisterAWSCommand
		field string
	}{
		"empty name":          {mutate(func(c *connection.RegisterAWSCommand) { c.Name = " " }), "name"},
		"control name":        {mutate(func(c *connection.RegisterAWSCommand) { c.Name = "a\nb" }), "name"},
		"short key id":        {mutate(func(c *connection.RegisterAWSCommand) { c.AccessKeyID = "AKIA123" }), "accessKeyId"},
		"temporary key id":    {mutate(func(c *connection.RegisterAWSCommand) { c.AccessKeyID = "ASIAFIXTUREKEY000001" }), "accessKeyId"},
		"lower-case key id":   {mutate(func(c *connection.RegisterAWSCommand) { c.AccessKeyID = strings.ToLower(fixtureKeyID) }), "accessKeyId"},
		"secret too short":    {mutate(func(c *connection.RegisterAWSCommand) { c.SecretAccessKey = "short" }), "secretAccessKey"},
		"secret with space":   {mutate(func(c *connection.RegisterAWSCommand) { c.SecretAccessKey = strings.Repeat("a", 39) + " " }), "secretAccessKey"},
		"secret with newline": {mutate(func(c *connection.RegisterAWSCommand) { c.SecretAccessKey = strings.Repeat("a", 39) + "\n" }), "secretAccessKey"},
		"no regions":          {mutate(func(c *connection.RegisterAWSCommand) { c.AllowedRegions = nil }), "allowedRegions"},
		"too many regions":    {mutate(func(c *connection.RegisterAWSCommand) { c.AllowedRegions = long }), "allowedRegions"},
		"duplicate region":    {mutate(func(c *connection.RegisterAWSCommand) { c.AllowedRegions = []string{"us-east-1", "us-east-1"} }), "allowedRegions"},
		"malformed region":    {mutate(func(c *connection.RegisterAWSCommand) { c.AllowedRegions = []string{"us east 1"} }), "allowedRegions"},
		"path-like region":    {mutate(func(c *connection.RegisterAWSCommand) { c.AllowedRegions = []string{"../us-east-1"} }), "allowedRegions"},
		"upper-case region":   {mutate(func(c *connection.RegisterAWSCommand) { c.AllowedRegions = []string{"US-EAST-1"} }), "allowedRegions"},
	}
	for name, tc := range cases {
		st, creds, v, svc := awsFixture(t)
		_, err := svc.RegisterAWS(ctx, "acme", tc.cmd)
		var field *connection.FieldError
		if !errors.Is(err, connection.ErrInvalid) || !errors.As(err, &field) || field.Field != tc.field {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, secret := range []string{fixtureKeyID, fixtureSecret} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("%s: error leaks the key", name)
			}
		}
		if list, _ := st.ListConnections(ctx, "acme"); v.calls != 0 || creds.Len() != 0 || len(list) != 0 {
			t.Errorf("%s: verifier %d, credentials %d, connections %d", name, v.calls, creds.Len(), len(list))
		}
	}
}

func TestRegisterAWSVerificationFailuresSaveNothing(t *testing.T) {
	ctx := context.Background()
	for name, cause := range map[string]error{
		"auth":         connection.ErrAuthentication,
		"permission":   connection.ErrPermissionDenied,
		"region":       connection.ErrRegionUnavailable,
		"unverifiable": connection.ErrCapabilityUnverifiable,
		"principal":    connection.ErrUnsupportedPrincipal,
		"unreachable":  connection.ErrClusterUnreachable,
		"raw":          errors.New("AccessDenied: SECRET-AWS-DETAIL " + fixtureSecret),
	} {
		st, creds, v, svc := awsFixture(t)
		v.err = cause
		_, err := svc.RegisterAWS(ctx, "acme", awsCommand())
		if !errors.Is(err, connection.ErrVerification) {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if name != "raw" && !errors.Is(err, cause) {
			t.Errorf("%s: category lost: %v", name, err)
		}
		if strings.Contains(err.Error(), "SECRET-AWS-DETAIL") || strings.Contains(err.Error(), fixtureSecret) {
			t.Errorf("%s: raw provider text reached the caller: %v", name, err)
		}
		if list, _ := st.ListConnections(ctx, "acme"); creds.Len() != 0 || len(list) != 0 {
			t.Errorf("%s: stored state after a failed verification", name)
		}
	}
}

func TestRegisterAWSIdentityAloneIsNeverReady(t *testing.T) {
	ctx := context.Background()
	for name, result := range map[string]connection.AWSVerification{
		"no capabilities": {AccountID: "123456789012", Regions: []string{"us-east-1", "eu-west-1"}},
		"missing region":  {AccountID: "123456789012", Regions: []string{"us-east-1"}, Capabilities: []string{"VPC"}},
	} {
		st, creds, v, svc := awsFixture(t)
		v.result = result
		if _, err := svc.RegisterAWS(ctx, "acme", awsCommand()); !errors.Is(err, connection.ErrVerification) {
			t.Errorf("%s: %v", name, err)
		}
		if list, _ := st.ListConnections(ctx, "acme"); creds.Len() != 0 || len(list) != 0 {
			t.Errorf("%s: a READY connection or credential exists", name)
		}
	}
	st, creds, _, _ := awsFixture(t)
	svc := connection.NewService(st)
	svc.SetAWSRegistration(emptyAccountVerifier{})
	svc.SetKubeconfigRegistration(nil, creds)
	if _, err := svc.RegisterAWS(ctx, "acme", awsCommand()); !errors.Is(err, connection.ErrVerification) {
		t.Errorf("no account: %v", err)
	}
	if list, _ := st.ListConnections(ctx, "acme"); creds.Len() != 0 || len(list) != 0 {
		t.Error("no account: a READY connection or credential exists")
	}
}

type emptyAccountVerifier struct{}

func (emptyAccountVerifier) VerifyAWS(_ context.Context, _ connection.AWSAccessKey, regions []string) (connection.AWSVerification, error) {
	return connection.AWSVerification{Regions: regions, Capabilities: []string{"VPC"}}, nil
}

func TestRegisterAWSFailsClosedWithoutVerifierOrStore(t *testing.T) {
	ctx := context.Background()
	st, creds, _, _ := awsFixture(t)
	noVerifier := connection.NewService(st)
	noVerifier.SetKubeconfigRegistration(nil, creds)
	if _, err := noVerifier.RegisterAWS(ctx, "acme", awsCommand()); !errors.Is(err, connection.ErrVerification) {
		t.Fatalf("no verifier: %v", err)
	}
	v := &awsVerifier{}
	noStore := connection.NewService(st)
	noStore.SetAWSRegistration(v)
	if _, err := noStore.RegisterAWS(ctx, "acme", awsCommand()); !errors.Is(err, connection.ErrCredentialStore) || v.calls != 0 {
		t.Fatalf("no store: %v (verifier calls %d)", err, v.calls)
	}
	_, _, _, svc := awsFixture(t)
	if _, err := svc.RegisterAWS(ctx, "ghost-org", awsCommand()); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unknown organization: %v", err)
	}
}

func TestRegisterAWSStoreWriteFailureSavesNothing(t *testing.T) {
	ctx := context.Background()
	st, creds, _, svc := awsFixture(t)
	creds.FailPut = errors.New("vault down")
	if _, err := svc.RegisterAWS(ctx, "acme", awsCommand()); !errors.Is(err, connection.ErrCredentialStore) {
		t.Fatalf("got %v", err)
	}
	if list, _ := st.ListConnections(ctx, "acme"); len(list) != 0 || creds.Len() != 0 {
		t.Fatal("READY metadata without a credential, or an orphan credential")
	}
}

func TestRegisterAWSInsertFailureDeletesOnlyItsOwnAttempt(t *testing.T) {
	ctx := context.Background()
	base, mem, v, _ := awsFixture(t)
	unrelated, err := mem.Put(ctx, "acme", "keep", []byte(`{"accessKeyId":"AKIAOTHEROTHEROTHER01","secretAccessKey":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	creds := &recordingCredentials{Store: mem}
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	st := &failingStore{Store: base, createErr: errors.New("db down"), onCreate: cancel}
	svc := connection.NewService(st)
	svc.SetAWSRegistration(v)
	svc.SetKubeconfigRegistration(nil, creds)
	if _, err := svc.RegisterAWS(cancelled, "acme", awsCommand()); err == nil {
		t.Fatal("insert failure reported success")
	}
	if len(creds.puts) != 1 || len(creds.deletes) != 1 || creds.deletes[0].ref != creds.puts[0] || creds.deletes[0].ctxErr != nil || !creds.deletes[0].hasDeadline {
		t.Fatalf("puts %v deletes %#v", creds.puts, creds.deletes)
	}
	if mem.Len() != 1 {
		t.Fatalf("objects after rollback: %d (only the unrelated one may remain)", mem.Len())
	}
	if _, err := mem.Get(ctx, "acme", "keep", unrelated); err != nil {
		t.Fatal("rollback removed another object")
	}
	if list, _ := base.ListConnections(ctx, "acme"); len(list) != 0 {
		t.Fatal("READY metadata after a failed insert")
	}
}

func TestRegisterAWSCleanupFailureIsObservableWithoutTheKey(t *testing.T) {
	ctx := context.Background()
	base, _, v, _ := awsFixture(t)
	st := &failingStore{Store: base, createErr: errors.New("db down")}
	creds := &deleteFailing{Store: credentialmemory.New()}
	svc := connection.NewService(st)
	svc.SetAWSRegistration(v)
	svc.SetKubeconfigRegistration(nil, creds)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	_, err := svc.RegisterAWS(ctx, "acme", awsCommand())
	var cleanup *connection.CleanupError
	if !errors.As(err, &cleanup) || cleanup.Reference == "" || creds.deletes != 1 {
		t.Fatalf("got %v (deletes %d)", err, creds.deletes)
	}
	for _, text := range []string{err.Error(), logs.String()} {
		if strings.Contains(text, fixtureKeyID) || strings.Contains(text, fixtureSecret) {
			t.Fatalf("key material in operational output: %s", text)
		}
	}
	if !strings.Contains(logs.String(), cleanup.Reference) {
		t.Fatalf("the operational record lacks the safe reference: %q", logs.String())
	}
}

// winnerStore lets a competing registration commit the same generated key
// right after this attempt chose it (after its key listing, before its
// insert), so the unique constraint decides and this attempt loses.
type winnerStore struct {
	*store.Store
	once   sync.Once
	winner application.Connection
}

func (s *winnerStore) ListConnections(ctx context.Context, org string) ([]application.Connection, error) {
	list, err := s.Store.ListConnections(ctx, org)
	s.once.Do(func() { _ = s.Store.CreateConnection(ctx, s.winner) })
	return list, err
}

func TestRegisterAWSLosingAConcurrentWinnerNeverTouchesIt(t *testing.T) {
	ctx := context.Background()
	base, mem, v, _ := awsFixture(t)
	creds := &recordingCredentials{Store: mem}
	winnerRef, err := mem.Put(ctx, "acme", "tai-khoan-aws-lab", []byte(`{"accessKeyId":"AKIAWINNERWINNERWIN1","secretAccessKey":"winner-secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	st := &winnerStore{Store: base, winner: application.Connection{
		Key: "tai-khoan-aws-lab", Name: "Winner", OrganizationKey: "acme", Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSAccessKey,
		Config: map[string]any{"accountId": "999999999999", "region": "us-east-1"}, SecretRef: winnerRef, Status: application.ConnectionReady,
	}}
	svc := connection.NewService(st)
	svc.SetAWSRegistration(v)
	svc.SetKubeconfigRegistration(nil, creds)
	got, err := svc.RegisterAWS(ctx, "acme", awsCommand())
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "tai-khoan-aws-lab-2" {
		t.Fatalf("the loser must take the next free key, got %q", got.Key)
	}
	winner, err := base.GetConnection(ctx, "acme", "tai-khoan-aws-lab")
	if err != nil || winner.SecretRef != winnerRef || winner.ConfigString("accountId") != "999999999999" {
		t.Fatalf("the winner changed: %#v %v", winner, err)
	}
	if value, err := mem.Get(ctx, "acme", "tai-khoan-aws-lab", winnerRef); err != nil || !strings.Contains(string(value), "winner-secret") {
		t.Fatalf("the winner's credential was touched: %v", err)
	}
	if len(creds.puts) != 2 || len(creds.deletes) != 1 || creds.deletes[0].ref != creds.puts[0] || creds.deletes[0].ref == winnerRef {
		t.Fatalf("puts %v deletes %#v", creds.puts, creds.deletes)
	}
	if mem.Len() != 2 {
		t.Fatalf("objects: %d (winner + this registration)", mem.Len())
	}
}

func TestRegisterAWSConcurrentSameNameGetsDistinctKeysAndCredentials(t *testing.T) {
	ctx := context.Background()
	st, creds, _, svc := awsFixture(t)
	var wg sync.WaitGroup
	results := make([]application.Connection, 8)
	errs := make([]error, len(results))
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.RegisterAWS(ctx, "acme", awsCommand())
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	refs := map[string]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
		if seen[results[i].Key] || refs[results[i].SecretRef] {
			t.Fatalf("duplicate key or reference: %q", results[i].Key)
		}
		seen[results[i].Key], refs[results[i].SecretRef] = true, true
	}
	list, _ := st.ListConnections(ctx, "acme")
	if len(list) != len(results) || creds.Len() != len(results) {
		t.Fatalf("connections %d, credentials %d", len(list), creds.Len())
	}
	// Every Connection resolves to its own object.
	resolver := connection.NewCredentialResolver(st, creds)
	for _, c := range results {
		if got, err := resolver.ResolveAWS(ctx, "acme", c.Key); err != nil || got.ConnectionKey != c.Key || got.AccessKeyID != fixtureKeyID {
			t.Fatalf("resolve %s: %+v %v", c.Key, got, err)
		}
	}
}

func TestSubmittedKeyIsRedactedWhenFormatted(t *testing.T) {
	cmd := awsCommand()
	key := connection.AWSAccessKey{AccessKeyID: fixtureKeyID, SecretAccessKey: fixtureSecret}
	for _, text := range []string{
		fmt.Sprintf("%v %+v %#v %s", cmd, cmd, cmd, cmd), fmt.Sprintf("%v %+v %#v %s", key, key, key, key),
		fmt.Sprint(&cmd), fmt.Sprint([]connection.AWSAccessKey{key}),
	} {
		if strings.Contains(text, fixtureKeyID) || strings.Contains(text, fixtureSecret) {
			t.Fatalf("formatting leaks the key: %s", text)
		}
	}
}

func TestResolveAWSAcceptsSessionTokenAndRejectsRegionlessRecords(t *testing.T) {
	ctx := context.Background()
	mem := credentialmemory.New()
	ref, _ := mem.Put(ctx, "acme", "acct", []byte(`{"accessKeyId":"AKID","secretAccessKey":"S","sessionToken":"T"}`))
	conn := awsConnection(ref)
	conn.Config = map[string]any{"accountId": "123456789012", "region": "us-east-1", "allowedRegions": []any{"us-east-1", "eu-west-1"}}
	got, err := connection.NewCredentialResolver(connections{"acme/acct": conn}, mem).ResolveAWS(ctx, "acme", "acct")
	if err != nil || got.SessionToken != "T" || strings.Join(got.AllowedRegions, ",") != "us-east-1,eu-west-1" {
		t.Fatalf("%+v %v", got, err)
	}
	conn.Config = map[string]any{"accountId": "123456789012"}
	if _, err := connection.NewCredentialResolver(connections{"acme/acct": conn}, mem).ResolveAWS(ctx, "acme", "acct"); !errors.Is(err, connection.ErrCredentialUnavailable) {
		t.Fatalf("regionless record resolved: %v", err)
	}
}
