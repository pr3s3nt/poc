package connection_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

// insertBarrier holds the first CreateConnection of two racers until both
// arrived, so both have already chosen the same free key and only the
// PostgreSQL unique constraint decides the winner.
type insertBarrier struct {
	persistence.Store
	mu      sync.Mutex
	arrived int
	release chan struct{}
}

func (b *insertBarrier) CreateConnection(ctx context.Context, conn application.Connection) error {
	b.mu.Lock()
	b.arrived++
	first := b.arrived <= 2
	if b.arrived == 2 {
		close(b.release)
	}
	b.mu.Unlock()
	if first {
		select {
		case <-b.release:
		case <-time.After(10 * time.Second):
			return errors.New("test barrier: the second racer never reached CreateConnection")
		}
	}
	return b.Store.CreateConnection(ctx, conn)
}

// TestPostgresKubeconfigRegistrationRaceAndReopen races two uploads with the
// same name against PostgreSQL. The loser's insert hits the unique key, its
// credential object is removed and it retries with the next suffix. The
// Organization default never changes and every value survives a reopen with
// no credential material in the rows.
func TestPostgresKubeconfigRegistrationRaceAndReopen(t *testing.T) {
	ctx := context.Background()
	url := persistencetest.FreshPostgresDatabase(t)
	st, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetOrganization(ctx, opts.OrganizationKey)
	if err != nil || before.DefaultConnectionKey == "" {
		t.Fatalf("seeded default: %#v %v", before, err)
	}
	creds := credentialmemory.New()
	barrier := &insertBarrier{Store: st, release: make(chan struct{})}
	svc := connection.NewService(barrier, nil)
	svc.SetKubeconfigRegistration(&kubeconfigVerifier{}, creds)
	doc := ct.Document(ct.TokenContext("lab", "https://lab.example", secretToken))
	results := make([]application.Connection, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.RegisterKubeconfig(ctx, opts.OrganizationKey, connection.RegisterKubeconfigCommand{Name: "Lab", Kubeconfig: doc, Context: "lab"})
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
	if keys[0] != "lab" || keys[1] != "lab-2" {
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
	for _, created := range results {
		stored, err := reopened.GetConnection(ctx, opts.OrganizationKey, created.Key)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Name != "Lab" || stored.AuthenticationType != application.AuthKubeconfig || stored.Status != application.ConnectionReady ||
			stored.SecretRef != created.SecretRef || stored.ConfigString("kubeContext") != "lab" {
			t.Fatalf("reopened %s: %#v", created.Key, stored)
		}
		value, err := creds.Get(ctx, opts.OrganizationKey, created.Key, stored.SecretRef)
		if err != nil || !strings.Contains(string(value), secretToken) {
			t.Fatalf("credential of %s: %v", created.Key, err)
		}
		row, _ := json.Marshal(stored)
		if strings.Contains(string(row), secretToken) || strings.Contains(string(row), "clusters") {
			t.Fatalf("row holds credential material: %s", row)
		}
	}
	seeded, err := reopened.GetConnection(ctx, opts.OrganizationKey, opts.ConnectionKey)
	if err != nil || seeded.AuthenticationType != application.AuthHostContext || seeded.Name == "" {
		t.Fatalf("seeded connection after reopen: %#v %v", seeded, err)
	}
}
