package connection_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed/seedtest"
)

// Real disposable PostgreSQL, fake AWS/Kubernetes verifiers, in-memory
// credential store. The race and rollback guards under test are the SQL CAS
// and the attempt-object cleanup; reopen proves the committed version, name
// and reference are what the restarted service reads.
func TestPostgresAccountLifecycleRaceCleanupAndReopen(t *testing.T) {
	ctx := context.Background()
	url := postgrestest.Fresh(t).Console
	st, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	creds := &gatedCredentials{Store: credentialmemory.New()}
	aws := &awsVerifier{}
	kube := &kubeconfigVerifier{}
	svc := connection.NewService(st)
	svc.SetKubeconfigRegistration(kube, creds)
	svc.SetAWSRegistration(&barrierVerifier{awsVerifier: aws, ready: make(chan struct{}), n: 2})

	// A barrier verifier blocks until two verifies run: register through the
	// plain verifier first.
	plain := connection.NewService(st)
	plain.SetKubeconfigRegistration(kube, creds)
	plain.SetAWSRegistration(aws)
	acc, err := plain.CreateAWSAccount(ctx, opts.OrganizationKey, awsCommand())
	if err != nil || acc.Version != 1 {
		t.Fatalf("create: %v %+v", err, acc)
	}
	internal, err := plain.RegisterKubeconfig(ctx, opts.OrganizationKey, connection.RegisterKubeconfigCommand{Name: "Cụm lab", Kubeconfig: ct.Single("lab", "https://lab.example:6443", secretToken), Context: "lab"})
	if err != nil || internal.Version != 1 || internal.ID == "" {
		t.Fatalf("create internal: %v %+v", err, internal)
	}

	// Two replacements race for the same expected version.
	keys := []connection.RegisterAWSCommand{
		{AccessKeyID: secondKeyID, SecretAccessKey: secondSecret, AllowedRegions: []string{"us-east-1"}},
		{AccessKeyID: "AKIAFIXTUREKEY000003", SecretAccessKey: "third/secret/value/000000000000000000000", AllowedRegions: []string{"us-east-1"}},
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.ReplaceAWSCredentials(ctx, opts.OrganizationKey, acc.Key, acc.Version, keys[i])
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, persistence.ErrVersionConflict) {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if wins != 1 || len(creds.deletes) != 1 {
		t.Fatalf("wins=%d deletes=%v", wins, creds.deletes)
	}
	current, err := st.GetConnection(ctx, opts.OrganizationKey, acc.Key)
	if err != nil || current.Version != 2 || creds.deletes[0] == current.SecretRef || creds.deletes[0] == acc.SecretRef {
		t.Fatalf("winner record %+v err %v deletes %v", current, err, creds.deletes)
	}

	// Rename and reverify, then a stale rename.
	renamed, err := plain.RenameAccount(ctx, opts.OrganizationKey, acc.Key, 2, "Tên sau restart")
	if err != nil || renamed.Version != 3 {
		t.Fatalf("rename: %v %+v", err, renamed)
	}
	if _, err := plain.RenameAccount(ctx, opts.OrganizationKey, acc.Key, 2, "Stale"); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale rename: %v", err)
	}
	verified, err := plain.VerifyAccount(ctx, opts.OrganizationKey, internal.Key, 1, nil)
	if err != nil || verified.Version != 2 {
		t.Fatalf("verify internal: %v %+v", err, verified)
	}
	st.Close()

	reopened, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	again := connection.NewService(reopened)
	again.SetKubeconfigRegistration(kube, creds)
	again.SetAWSRegistration(aws)
	got, err := again.GetAccount(ctx, opts.OrganizationKey, acc.Key)
	if err != nil || got.Version != 3 || got.Name != "Tên sau restart" || got.SecretRef != current.SecretRef {
		t.Fatalf("after reopen: %+v %v", got, err)
	}
	if _, err := again.RenameAccount(ctx, opts.OrganizationKey, acc.Key, 2, "Stale after restart"); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale after reopen: %v", err)
	}
	resolver := connection.NewCredentialResolver(reopened, creds)
	resolved, err := resolver.ResolveAWS(ctx, opts.OrganizationKey, acc.Key)
	if err != nil || resolved.AccessKeyID == fixtureKeyID {
		t.Fatalf("resolver must read the committed replacement after reopen: %v %q", err, resolved.AccessKeyID)
	}
	// The record never holds the key; foreign Organization sees nothing.
	record := toJSON(t, got)
	for _, secret := range []string{fixtureSecret, secondSecret, fixtureKeyID, secondKeyID} {
		if strings.Contains(record, secret) {
			t.Fatalf("record holds credential material: %s", record)
		}
	}
	if _, err := again.GetAccount(ctx, "no-such-org", acc.Key); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization: %v", err)
	}
}
