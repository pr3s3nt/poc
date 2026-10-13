package connection_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/seed/seedtest"
)

// TestPostgresAWSRegistrationRaceReopenAndResolution runs AWS registration
// against a real disposable PostgreSQL (fake AWS verifier, in-memory
// credential store). Two same-name registrations choose the same key and only
// the PostgreSQL unique constraint decides. The loser removes only its own
// credential object and retries. After a reopen the rows hold no key, the
// allowed-region list survived JSON storage, and the resolver reads each
// Connection's own scoped credential.
func TestPostgresAWSRegistrationRaceReopenAndResolution(t *testing.T) {
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
	before, err := st.GetOrganization(ctx, opts.OrganizationKey)
	if err != nil || before.DefaultConnectionKey == "" {
		t.Fatalf("seeded default: %#v %v", before, err)
	}
	creds := credentialmemory.New()
	barrier := &insertBarrier{Store: st, release: make(chan struct{})}
	svc := connection.NewService(barrier)
	svc.SetKubeconfigRegistration(nil, creds)
	svc.SetAWSRegistration(&awsVerifier{})

	cmd := awsCommand()
	cmd.Name = "Lab AWS"
	results := make([]application.Connection, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.RegisterAWS(ctx, opts.OrganizationKey, cmd)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	keys := []string{results[0].Key, results[1].Key}
	sort.Strings(keys)
	if keys[0] != "lab-aws" || keys[1] != "lab-aws-2" {
		t.Fatalf("keys: %v", keys)
	}
	if barrier.arrived != 3 {
		t.Fatalf("the loser did not retry after the unique conflict: %d inserts", barrier.arrived)
	}
	if creds.Len() != 2 {
		t.Fatalf("credential objects after the race: %d (the losing attempt must be removed)", creds.Len())
	}
	st.Close()

	reopened, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err := reopened.GetOrganization(ctx, opts.OrganizationKey)
	if err != nil || after.DefaultConnectionKey != before.DefaultConnectionKey {
		t.Fatalf("default connection changed: %q -> %q (%v)", before.DefaultConnectionKey, after.DefaultConnectionKey, err)
	}
	resolver := connection.NewCredentialResolver(reopened, creds)
	for _, created := range results {
		stored, err := reopened.GetConnection(ctx, opts.OrganizationKey, created.Key)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Kind != application.ConnectionAWS || stored.AuthenticationType != application.AuthAWSAccessKey || stored.Status != application.ConnectionReady ||
			stored.SecretRef != created.SecretRef || strings.Join(stored.AllowedRegions(), ",") != "us-east-1,eu-west-1" ||
			stored.ConfigString("region") != "us-east-1" || stored.ConfigString("accountId") != "123456789012" {
			t.Fatalf("reopened %s: %#v", created.Key, stored)
		}
		row, _ := json.Marshal(stored)
		if strings.Contains(string(row), fixtureKeyID) || strings.Contains(string(row), fixtureSecret) {
			t.Fatalf("row holds the key: %s", row)
		}
		got, err := resolver.ResolveAWS(ctx, opts.OrganizationKey, created.Key)
		if err != nil || got.AccessKeyID != fixtureKeyID || got.SecretAccessKey != fixtureSecret || got.ConnectionKey != created.Key || !got.AllowsRegion("eu-west-1") {
			t.Fatalf("resolve %s: %+v %v", created.Key, got, err)
		}
		// Another organization's scope never reads this reference.
		if _, err := creds.Get(ctx, "other-org", created.Key, stored.SecretRef); err == nil {
			t.Fatal("credential readable through another scope")
		}
	}
	// The seeded Kubernetes default is untouched by AWS registration.
	seeded, err := reopened.GetConnection(ctx, opts.OrganizationKey, opts.ConnectionKey)
	if err != nil || seeded.AuthenticationType != application.AuthKubeconfig {
		t.Fatalf("seeded connection after reopen: %#v %v", seeded, err)
	}
}

// TestPostgresAWSInsertFailureLeavesNoRowAndNoCredential proves against real SQL
// that an unknown organization writes nothing and that an existing key is
// never overwritten: the unique constraint rejects a duplicate and registration
// takes the next free key.
func TestPostgresAWSInsertFailureLeavesNoRowAndNoCredential(t *testing.T) {
	ctx := context.Background()
	url := postgrestest.Fresh(t).Console
	st, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	creds := &recordingCredentials{Store: credentialmemory.New()}
	svc := connection.NewService(st)
	svc.SetKubeconfigRegistration(nil, creds)
	svc.SetAWSRegistration(&awsVerifier{})
	// An unknown organization is refused before any verification or write.
	_, err = svc.RegisterAWS(ctx, "no-such-org", awsCommand())
	if err == nil || strings.Contains(err.Error(), fixtureSecret) || strings.Contains(err.Error(), fixtureKeyID) {
		t.Fatalf("got %v", err)
	}
	if len(creds.puts) != 0 {
		t.Fatalf("a credential was written for an unknown organization: %v", creds.puts)
	}
	before, _ := st.ListConnections(ctx, opts.OrganizationKey)
	// A duplicate key inserted behind the service's back must make the real
	// SQL unique constraint reject and roll back.
	winner := application.Connection{ID: ids.New(), Key: "tai-khoan-aws-lab", Name: "Winner", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionAWS,
		AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady, SecretRef: "kv2://kv/winner", Config: map[string]any{"accountId": "999999999999", "region": "us-east-1"}}
	if err := st.CreateConnection(ctx, winner); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateConnection(ctx, winner); err == nil {
		t.Fatal("the unique (organization, key) constraint did not reject a duplicate")
	}
	got, err := svc.RegisterAWS(ctx, opts.OrganizationKey, awsCommand())
	if err != nil || got.Key != "tai-khoan-aws-lab-2" {
		t.Fatalf("registration next to an existing key: %v %#v", err, got)
	}
	after, _ := st.ListConnections(ctx, opts.OrganizationKey)
	if len(after) != len(before)+2 {
		t.Fatalf("connections: %d -> %d", len(before), len(after))
	}
	untouched, err := st.GetConnection(ctx, opts.OrganizationKey, "tai-khoan-aws-lab")
	if err != nil || untouched.SecretRef != "kv2://kv/winner" || untouched.ConfigString("accountId") != "999999999999" {
		t.Fatalf("existing connection changed: %#v %v", untouched, err)
	}
}
