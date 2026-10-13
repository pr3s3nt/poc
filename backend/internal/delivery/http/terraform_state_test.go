package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformstate"
)

// stateRepoStub authorizes every capability for one state key and holds no
// state. Any other repository method panics through the nil embedded
// interface. It counts the transactions the handler opens.
type stateRepoStub struct {
	terraformstate.Repository
	key          string
	transactions int
}

func (s *stateRepoStub) Transact(ctx context.Context, fn func(ctx context.Context) error) error {
	s.transactions++
	return fn(ctx)
}

func (s *stateRepoStub) AuthorizeForUpdate(_ context.Context, _, stateKey string, now time.Time) (terraformstate.Invocation, error) {
	if stateKey != s.key {
		return terraformstate.Invocation{}, terraformstate.ErrForbidden
	}
	return terraformstate.Invocation{InstanceID: "inst-1", OrganizationID: "org-1", StateKey: stateKey, Action: tfresource.CapabilityCreate, ExpiresAt: now.Add(time.Hour)}, nil
}

func (s *stateRepoStub) StateForUpdate(context.Context, string) (*tfresource.StateBlob, error) {
	return nil, nil
}

func newStateHandlerForTest(t *testing.T, key string) (*TerraformStateHandler, *stateRepoStub) {
	t.Helper()
	repo := &stateRepoStub{key: key}
	cipher, err := statecipher.New("key-1", bytes.Repeat([]byte{1}, statecipher.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := statebackend.New(repo, cipher)
	if err != nil {
		t.Fatal(err)
	}
	return NewTerraformStateHandler(svc), repo
}

func serveState(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(`{"ID":"lock-1"}`))
	req.SetBasicAuth("executor", "token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestTerraformStateMalformedKeyIsNeverNoState: Terraform reads GET 404 (and
// 204) as "no state yet", so a malformed key inside the state collection must
// fail with 400 before any repository access, for every method.
func TestTerraformStateMalformedKeyIsNeverNoState(t *testing.T) {
	valid := strings.Repeat("ab", 32)
	h, repo := newStateHandlerForTest(t, valid)
	malformed := map[string]string{
		"empty key":            TerraformStatePrefix,
		"collection no slash":  strings.TrimSuffix(TerraformStatePrefix, "/"),
		"short key":            TerraformStatePrefix + "abc",
		"upper-case hex":       TerraformStatePrefix + strings.ToUpper(valid),
		"one char too long":    TerraformStatePrefix + valid + "a",
		"trailing slash":       TerraformStatePrefix + valid + "/",
		"nested path":          TerraformStatePrefix + valid + "/extra",
		"escaped slash":        TerraformStatePrefix + valid[:32] + "%2F" + valid[32:],
		"dot segment":          TerraformStatePrefix + "../" + valid,
		"non-hex of right len": TerraformStatePrefix + strings.Repeat("zz", 32),
	}
	for name, path := range malformed {
		for _, method := range []string{http.MethodGet, http.MethodPost, "LOCK", "UNLOCK", http.MethodDelete} {
			rec := serveState(h, method, path)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_STATE_KEY") {
				t.Errorf("%s %s: got %d %s, want 400 INVALID_STATE_KEY", name, method, rec.Code, rec.Body.String())
			}
		}
	}
	if repo.transactions != 0 {
		t.Fatalf("malformed keys reached the repository %d times", repo.transactions)
	}

	// A route outside the state collection is an ordinary 404.
	for _, path := range []string{"/", "/healthz", "/internal/terraform/statesX/" + valid, "/internal/terraform/" + valid} {
		if rec := serveState(h, http.MethodGet, path); rec.Code != http.StatusNotFound {
			t.Errorf("unrelated route %s: %d", path, rec.Code)
		}
	}

	// A well-formed key the capability does not cover is 403, not no-state.
	if rec := serveState(h, http.MethodGet, TerraformStatePrefix+strings.Repeat("cd", 32)); rec.Code != http.StatusForbidden {
		t.Errorf("out-of-scope key GET: %d", rec.Code)
	}
	// The authorized key without any state is the protocol no-state answer.
	if rec := serveState(h, http.MethodGet, TerraformStatePrefix+valid); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("scoped missing state GET: %d %q", rec.Code, rec.Body.String())
	}
}
