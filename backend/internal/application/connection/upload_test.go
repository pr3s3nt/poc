package connection_test

import (
	"context"
	"encoding/json"
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
)

type kubeconfigVerifier struct {
	mu       sync.Mutex
	err      error
	selected []connection.SelectedKubeconfig
}

func (v *kubeconfigVerifier) VerifyKubeconfig(_ context.Context, selected connection.SelectedKubeconfig) (connection.KubernetesVerification, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.selected = append(v.selected, selected)
	return connection.KubernetesVerification{Endpoint: selected.Context.Endpoint, Version: "v1.34.0"}, v.err
}

// failingStore fails CreateConnection to exercise ERR-08 rollback.
type failingStore struct {
	*store.Store
	createErr error
	// onCreate runs inside CreateConnection before it fails.
	onCreate func()
}

func (s *failingStore) CreateConnection(ctx context.Context, conn application.Connection) error {
	if s.onCreate != nil {
		s.onCreate()
	}
	if s.createErr != nil {
		return s.createErr
	}
	return s.Store.CreateConnection(ctx, conn)
}

// deleteFailing makes rollback fail as well.
type deleteFailing struct {
	*credentialmemory.Store
	deletes int
}

func (s *deleteFailing) Delete(context.Context, string, string, string) error {
	s.deletes++
	return errors.New("vault down")
}

// recordingCredentials records Put references and the context each Delete
// receives.
type recordingCredentials struct {
	*credentialmemory.Store
	puts    []string
	deletes []deleteCall
}

type deleteCall struct {
	org, connection, ref string
	ctxErr               error
	hasDeadline          bool
	remaining            time.Duration
}

func (s *recordingCredentials) Put(ctx context.Context, org, conn string, value []byte) (string, error) {
	ref, err := s.Store.Put(ctx, org, conn, value)
	s.puts = append(s.puts, ref)
	return ref, err
}

func (s *recordingCredentials) Delete(ctx context.Context, org, conn, ref string) error {
	deadline, ok := ctx.Deadline()
	s.deletes = append(s.deletes, deleteCall{org: org, connection: conn, ref: ref, ctxErr: ctx.Err(), hasDeadline: ok, remaining: time.Until(deadline)})
	return s.Store.Delete(ctx, org, conn, ref)
}

func uploadFixture(t *testing.T) (*store.Store, *credentialmemory.Store, *kubeconfigVerifier, *connection.Service) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	for _, key := range []string{"acme", "other"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: key, Name: key, DefaultConnectionKey: "internal-cluster"}); err != nil {
			t.Fatal(err)
		}
	}
	creds := credentialmemory.New()
	v := &kubeconfigVerifier{}
	svc := connection.NewService(st, nil)
	svc.SetKubeconfigRegistration(v, creds)
	return st, creds, v, svc
}

func TestRegisterKubeconfig_StoresSelectedCredentialOutsideRecord(t *testing.T) {
	ctx := context.Background()
	st, creds, v, svc := uploadFixture(t)
	doc := ct.Document(ct.TokenContext("dev", "https://dev.example:6443", secretToken), ct.TokenContext("prod", "https://prod.example", "prod-token-value"))
	created, err := svc.RegisterKubeconfig(ctx, "acme", connection.RegisterKubeconfigCommand{Name: "  Cụm Nội bộ Lab ", Kubeconfig: doc, Context: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Key != "cum-noi-bo-lab" || created.Name != "Cụm Nội bộ Lab" || created.AuthenticationType != application.AuthKubeconfig || created.Status != application.ConnectionReady {
		t.Fatalf("connection: %#v", created)
	}
	if created.ConfigString("kubeContext") != "prod" || created.ConfigString("cluster") != "prod-cluster" || created.ConfigString("endpoint") != "https://prod.example" {
		t.Fatalf("config: %#v", created.Config)
	}
	if len(v.selected) != 1 || v.selected[0].Context.Name != "prod" || strings.Contains(string(v.selected[0].Document), "dev.example") {
		t.Fatalf("verifier did not receive only the selected context: %#v", v.selected)
	}
	stored, err := st.GetConnection(ctx, "acme", created.Key)
	if err != nil {
		t.Fatal(err)
	}
	record := toJSON(t, stored)
	for _, secret := range []string{"prod-token-value", secretToken, "clusters"} {
		if strings.Contains(record, secret) {
			t.Fatalf("record holds credential material %q: %s", secret, record)
		}
	}
	value, err := creds.Get(ctx, "acme", created.Key, stored.SecretRef)
	if err != nil || !strings.Contains(string(value), "prod-token-value") || strings.Contains(string(value), secretToken) {
		t.Fatalf("stored credential: %v %s", err, value)
	}
	if _, err := creds.Get(ctx, "other", created.Key, stored.SecretRef); err == nil {
		t.Fatal("credential readable from another organization")
	}
	org, _ := st.GetOrganization(ctx, "acme")
	if org.DefaultConnectionKey != "internal-cluster" {
		t.Fatalf("registration changed the organization default: %#v", org)
	}

	// Same name: a collision suffix, never an overwrite.
	again, err := svc.RegisterKubeconfig(ctx, "acme", connection.RegisterKubeconfigCommand{Name: "Cụm nội bộ lab", Kubeconfig: doc, Context: "dev"})
	if err != nil || again.Key != "cum-noi-bo-lab-2" {
		t.Fatalf("collision: %v %#v", err, again)
	}
	first, _ := st.GetConnection(ctx, "acme", created.Key)
	if first.SecretRef != stored.SecretRef || first.ConfigString("kubeContext") != "prod" {
		t.Fatalf("existing connection changed: %#v", first)
	}
	if creds.Len() != 2 {
		t.Fatalf("credential objects: %d", creds.Len())
	}
}

func TestRegisterKubeconfig_ConcurrentSameNameGetsDistinctKeys(t *testing.T) {
	ctx := context.Background()
	st, creds, _, svc := uploadFixture(t)
	doc := ct.Single("ctx", "https://c.example", secretToken)
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.RegisterKubeconfig(ctx, "acme", connection.RegisterKubeconfigCommand{Name: "lab", Kubeconfig: doc, Context: "ctx"})
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	list, _ := st.ListConnections(ctx, "acme")
	if len(list) != 6 || creds.Len() != 6 {
		t.Fatalf("connections %d, credentials %d", len(list), creds.Len())
	}
}

func TestRegisterKubeconfig_FailuresSaveNothingReady(t *testing.T) {
	ctx := context.Background()
	doc := ct.Single("ctx", "https://c.example", secretToken)
	cmd := connection.RegisterKubeconfigCommand{Name: "lab", Kubeconfig: doc, Context: "ctx"}

	t.Run("verification", func(t *testing.T) {
		st, creds, v, svc := uploadFixture(t)
		v.err = connection.ErrPermissionDenied
		_, err := svc.RegisterKubeconfig(ctx, "acme", cmd)
		if !errors.Is(err, connection.ErrVerification) || !strings.Contains(err.Error(), "permission") {
			t.Fatalf("got %v", err)
		}
		if list, _ := st.ListConnections(ctx, "acme"); len(list) != 0 || creds.Len() != 0 {
			t.Fatal("verification failure stored state")
		}
	})
	t.Run("no store", func(t *testing.T) {
		st, _, v, _ := uploadFixture(t)
		svc := connection.NewService(st, nil)
		svc.SetKubeconfigRegistration(v, nil)
		if _, err := svc.RegisterKubeconfig(ctx, "acme", cmd); !errors.Is(err, connection.ErrCredentialStore) {
			t.Fatalf("got %v", err)
		}
		if len(v.selected) != 0 {
			t.Fatal("verified without a credential store")
		}
	})
	t.Run("store write", func(t *testing.T) {
		st, creds, _, svc := uploadFixture(t)
		creds.FailPut = errors.New("boom")
		if _, err := svc.RegisterKubeconfig(ctx, "acme", cmd); !errors.Is(err, connection.ErrCredentialStore) {
			t.Fatalf("got %v", err)
		}
		if list, _ := st.ListConnections(ctx, "acme"); len(list) != 0 {
			t.Fatal("READY connection without credential")
		}
	})
	t.Run("incomplete kubeconfig shape touches no store or cluster", func(t *testing.T) {
		for name, document := range map[string]string{
			"apiVersion missing": strings.Replace(doc, "apiVersion: v1\n", "", 1),
			"apiVersion null":    strings.Replace(doc, "apiVersion: v1\n", "apiVersion: null\n", 1),
			"kind missing":       strings.Replace(doc, "kind: Config\n", "", 1),
			"kind null":          strings.Replace(doc, "kind: Config\n", "kind: null\n", 1),
		} {
			st, creds, v, svc := uploadFixture(t)
			_, err := svc.RegisterKubeconfig(ctx, "acme", connection.RegisterKubeconfigCommand{Name: "lab", Kubeconfig: document, Context: "ctx"})
			if !errors.Is(err, connection.ErrKubeconfigInvalid) || strings.Contains(err.Error(), secretToken) {
				t.Fatalf("%s: got %v", name, err)
			}
			if len(v.selected) != 0 || creds.Len() != 0 {
				t.Fatalf("%s: verifier calls %d, credential objects %d", name, len(v.selected), creds.Len())
			}
			if list, _ := st.ListConnections(ctx, "acme"); len(list) != 0 {
				t.Fatalf("%s: connection saved", name)
			}
		}
	})
	t.Run("insert failure deletes own attempt even when cancelled", func(t *testing.T) {
		base, mem, v, _ := uploadFixture(t)
		other, err := mem.Put(ctx, "acme", "keep", []byte("unrelated"))
		if err != nil {
			t.Fatal(err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		defer cancel()
		// The request is cancelled deterministically while the insert fails.
		st := &failingStore{Store: base, createErr: errors.New("db down"), onCreate: cancel}
		creds := &recordingCredentials{Store: mem}
		svc := connection.NewService(st, nil)
		svc.SetKubeconfigRegistration(v, creds)
		if _, err := svc.RegisterKubeconfig(cancelled, "acme", cmd); err == nil {
			t.Fatal("insert failure reported success")
		}
		if cancelled.Err() == nil {
			t.Fatal("request context was not cancelled during the insert")
		}
		if len(creds.puts) != 1 || len(creds.deletes) != 1 {
			t.Fatalf("puts %v, deletes %v", creds.puts, creds.deletes)
		}
		del := creds.deletes[0]
		if del.ref != creds.puts[0] || del.org != "acme" || del.connection != "lab" {
			t.Fatalf("cleanup deleted %#v, attempt was %s", del, creds.puts[0])
		}
		if del.ctxErr != nil || !del.hasDeadline || del.remaining <= 0 || del.remaining > time.Minute {
			t.Fatalf("cleanup context not live and bounded: %#v", del)
		}
		if mem.Len() != 1 {
			t.Fatalf("attempt credential not cleaned: %d objects", mem.Len())
		}
		if _, err := mem.Get(ctx, "acme", "keep", other); err != nil {
			t.Fatal("cleanup removed another object")
		}
	})
	t.Run("cleanup failure is observable", func(t *testing.T) {
		base, _, v, _ := uploadFixture(t)
		st := &failingStore{Store: base, createErr: errors.New("db down")}
		creds := &deleteFailing{Store: credentialmemory.New()}
		svc := connection.NewService(st, nil)
		svc.SetKubeconfigRegistration(v, creds)
		_, err := svc.RegisterKubeconfig(ctx, "acme", cmd)
		var cleanup *connection.CleanupError
		if !errors.As(err, &cleanup) || cleanup.Reference == "" || creds.deletes != 1 || strings.Contains(err.Error(), secretToken) {
			t.Fatalf("got %v (deletes %d)", err, creds.deletes)
		}
	})
	t.Run("invalid name", func(t *testing.T) {
		_, _, _, svc := uploadFixture(t)
		for _, name := range []string{"", "  ", strings.Repeat("x", 101), "bad\nname"} {
			if _, err := svc.RegisterKubeconfig(ctx, "acme", connection.RegisterKubeconfigCommand{Name: name, Kubeconfig: doc, Context: "ctx"}); !errors.Is(err, connection.ErrInvalid) {
				t.Fatalf("name %q: %v", name, err)
			}
		}
	})
}

func TestCredentialResolver_FailsClosed(t *testing.T) {
	ctx := context.Background()
	st, creds, _, svc := uploadFixture(t)
	created, err := svc.RegisterKubeconfig(ctx, "acme", connection.RegisterKubeconfigCommand{Name: "lab", Kubeconfig: ct.Single("ctx", "https://c.example", secretToken), Context: "ctx"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConnection(ctx, application.Connection{Key: "legacy", OrganizationKey: "acme", Kind: application.ConnectionKubernetes, Config: map[string]any{"kubeContext": "ctx"}, SecretRef: "host-kube-context://ctx", Status: application.ConnectionReady}); err != nil {
		t.Fatal(err)
	}
	notReady := created
	notReady.Key, notReady.Status = "pending", application.ConnectionVerifying
	if err := st.SaveConnection(ctx, notReady); err != nil {
		t.Fatal(err)
	}
	resolver := connection.NewCredentialResolver(st, creds)
	value, err := resolver.ResolveKubeconfig(ctx, execution.Target{Organization: "acme", Connection: created.Key, Context: "ctx"})
	if err != nil || !strings.Contains(string(value), secretToken) {
		t.Fatalf("resolve: %v", err)
	}
	for name, target := range map[string]execution.Target{
		"other organization": {Organization: "other", Connection: created.Key},
		"missing":            {Organization: "acme", Connection: "ghost"},
		"host context":       {Organization: "acme", Connection: "legacy"},
		"not READY":          {Organization: "acme", Connection: "pending"},
		"redirected context": {Organization: "acme", Connection: created.Key, Context: "kind-idp-internal"},
		"no identity":        {Context: "ctx"},
	} {
		if _, err := resolver.ResolveKubeconfig(ctx, target); !errors.Is(err, connection.ErrCredentialUnavailable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := connection.NewCredentialResolver(st, nil).ResolveKubeconfig(ctx, execution.Target{Organization: "acme", Connection: created.Key}); !errors.Is(err, connection.ErrCredentialUnavailable) {
		t.Fatalf("nil store: %v", err)
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
