package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestSecretStoreRegistrationIsPlatformOnlyAndNeverLeaksTheTokenOrRefs(t *testing.T) {
	server, _ := newServer(t, "")
	developer := authenticatedClient(t, server.URL)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	body := map[string]any{"name": "Team Vault", "backendAddress": "http://vault.example:8200", "workloadAddress": "http://vault.vault.svc:8200", "token": "very-secret-token-value"}
	if status, _ := requestJSON(t, developer, http.MethodPost, server.URL+"/api/v1/secret-stores", body); status != http.StatusForbidden {
		t.Fatalf("developer registration = %d", status)
	}
	if status, _ := requestJSON(t, developer, http.MethodGet, server.URL+"/api/v1/secret-stores", nil); status != http.StatusForbidden {
		t.Fatalf("developer list = %d", status)
	}
	status, created := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/secret-stores", body)
	if status != http.StatusCreated || created["status"] != "READY" || created["mount"] != "kv" || created["authMount"] != "kubernetes" {
		t.Fatalf("register: %d %v", status, created)
	}
	// Bad token, unknown field, bad address and an attacker-style name stay safe.
	status, bad := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/secret-stores", map[string]any{"name": "Bad", "backendAddress": "http://v:8200", "workloadAddress": "http://v:8200", "token": "bad-token"})
	if status != http.StatusUnprocessableEntity || strings.Contains(toJSON(t, bad), "bad-token") || !strings.Contains(toJSON(t, bad), "rejected the token") {
		t.Fatalf("bad token: %d %v", status, bad)
	}
	for name, payload := range map[string]map[string]any{
		"unknown field": {"name": "X", "backendAddress": "http://v:8200", "workloadAddress": "http://v:8200", "token": "t", "extra": 1},
		"user info":     {"name": "X", "backendAddress": "http://u:p@v:8200", "workloadAddress": "http://v:8200", "token": "t"},
		"bad mount":     {"name": "X", "backendAddress": "http://v:8200", "workloadAddress": "http://v:8200", "token": "t", "mount": "../kv"},
		"blank token":   {"name": "X", "backendAddress": "http://v:8200", "workloadAddress": "http://v:8200", "token": " "},
		"not-a-cert CA": {"name": "X", "backendAddress": "http://v:8200", "workloadAddress": "http://v:8200", "token": "t", "tlsCaPem": "nonsense"},
		"file address":  {"name": "X", "backendAddress": "file:///etc/passwd", "workloadAddress": "http://v:8200", "token": "t"},
	} {
		if status, out := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/secret-stores", payload); status != http.StatusBadRequest {
			t.Fatalf("%s = %d %v", name, status, out)
		}
	}
	// Two registrations with one name get distinct generated keys.
	status, second := requestJSON(t, pe, http.MethodPost, server.URL+"/api/v1/secret-stores", body)
	if status != http.StatusCreated || second["key"] == created["key"] {
		t.Fatalf("second registration: %d %v", status, second)
	}
	status, list := requestJSON(t, pe, http.MethodGet, server.URL+"/api/v1/secret-stores", nil)
	status2, choices := requestJSON(t, developer, http.MethodGet, server.URL+"/api/v1/secret-store-choices", nil)
	if status != http.StatusOK || status2 != http.StatusOK || len(list["secretStores"].([]any)) != 2 || len(choices["secretStores"].([]any)) != 2 {
		t.Fatalf("list %d %v choices %d %v", status, list, status2, choices)
	}
	for _, payload := range []string{toJSON(t, created), toJSON(t, list), toJSON(t, choices), toJSON(t, bad)} {
		for _, forbidden := range []string{"very-secret-token-value", "credentialRef", "kv2://", "orchestrator/connections", "secretRef", "\"token\""} {
			if strings.Contains(payload, forbidden) {
				t.Fatalf("public payload leaks %q: %s", forbidden, payload)
			}
		}
	}
	if strings.Contains(toJSON(t, choices), "backendAddress") {
		t.Fatalf("Developer choices must be minimal: %v", choices)
	}
}

func TestEnvironmentSettingsStoreSelectionConflictsAndBusyStateOverHTTP(t *testing.T) {
	server, opts, app := newServerApp(t, "")
	developer := authenticatedClient(t, server.URL)
	storeKey := registerSecretStore(t, server.URL, "Settings Vault")
	otherKey := registerSecretStore(t, server.URL, "Other Vault")
	created := createConfiguredApplication(t, developer, server.URL, "Settings Test", "settings-test", "internal-cluster")
	appKey := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + appKey + "/environments/staging"
	env := func() map[string]any {
		_, out := requestJSON(t, developer, http.MethodGet, server.URL+"/api/v1/applications/"+appKey, nil)
		for _, raw := range out["application"].(map[string]any)["environments"].([]any) {
			if view := raw.(map[string]any); view["key"] == "staging" {
				return view
			}
		}
		return nil
	}
	put := func(key string, version, config any) (int, map[string]any) {
		return requestJSON(t, developer, http.MethodPut, base+"/secret-store", map[string]any{"secretStoreKey": key, "expectedVersion": version, "expectedConfigVersion": config})
	}
	view := env()
	if view["secretStoreKey"] != "" {
		t.Fatalf("a new environment must not auto-select a store: %v", view)
	}
	status, out := put(storeKey, view["version"], 0)
	if status != http.StatusOK || out["changed"] != true || out["environment"].(map[string]any)["secretStoreKey"] != storeKey {
		t.Fatalf("select store: %d %v", status, out)
	}
	// Stale version, unknown store and same-store no-op.
	if status, out := put(otherKey, view["version"], 0); status != http.StatusConflict || out["code"] != "STALE_VERSION" {
		t.Fatalf("stale: %d %v", status, out)
	}
	current := env()
	if status, out := put("missing", current["version"], 0); status != http.StatusUnprocessableEntity || out["field"] != "secretStoreKey" || strings.Contains(toJSON(t, out), "missing") {
		t.Fatalf("unknown store: %d %v", status, out)
	}
	if status, out := put(storeKey, current["version"], 0); status != http.StatusOK || out["changed"] != false {
		t.Fatalf("same store: %d %v", status, out)
	}
	for body, label := range map[string]string{`{"secretStoreKey":"","expectedVersion":1,"expectedConfigVersion":0}`: "blank", `{"secretStoreKey":"x","expectedVersion":0,"expectedConfigVersion":0}`: "zero version", `{"secretStoreKey":"x","expectedVersion":1}`: "missing config version"} {
		req, _ := http.NewRequest(http.MethodPut, base+"/secret-store", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := developer.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s = %d", label, resp.StatusCode)
		}
	}

	// A held claim blocks every write with ENVIRONMENT_BUSY and shows the owner.
	op, err := app.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ApplicationKey: appKey, EnvironmentKey: "staging", Owner: "other-backend", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	current = env()
	if current["activeOperation"] == nil || current["activeOperation"].(map[string]any)["kind"] != "DEPLOY" {
		t.Fatalf("busy state not visible: %v", current)
	}
	busy := func(label string, status int, out map[string]any) {
		t.Helper()
		if status != http.StatusConflict || out["code"] != "ENVIRONMENT_BUSY" || out["operation"] == nil {
			t.Fatalf("%s while busy = %d %v", label, status, out)
		}
	}
	s, o := requestJSON(t, developer, http.MethodPut, base+"/connection", map[string]any{"connectionKey": "aws-account", "expectedVersion": current["version"]})
	busy("connection", s, o)
	s, o = requestJSON(t, developer, http.MethodPut, base+"/configuration/keys/MODE", map[string]any{"kind": "VARIABLE", "value": "x", "version": 0})
	busy("config", s, o)
	s, o = requestJSON(t, developer, http.MethodPut, base+"/workloads/web", map[string]any{"score": extraScore("web"), "version": 0})
	busy("draft", s, o)
	s, o = put(otherKey, current["version"], 0)
	busy("store", s, o)
	if status, ops := requestJSON(t, developer, http.MethodGet, base+"/operations", nil); status != http.StatusOK || len(ops["operations"].([]any)) != 2 {
		t.Fatalf("operations: %d %v", status, ops)
	}
	// Different environments stay independent.
	if status, _ := requestJSON(t, developer, http.MethodPut, strings.Replace(base, "/staging", "/production", 1)+"/configuration/keys/MODE", map[string]any{"kind": "VARIABLE", "value": "x", "version": 0}); status != http.StatusOK {
		t.Fatalf("production while staging is busy = %d", status)
	}
	owner := persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
	if err := app.Store.ReleaseOperation(context.Background(), owner, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	if status, _ := requestJSON(t, developer, http.MethodPut, base+"/configuration/keys/MODE", map[string]any{"kind": "VARIABLE", "value": "x", "version": 0}); status != http.StatusOK {
		t.Fatalf("after release = %d", status)
	}
	_ = opts
}

func extraScore(name string) map[string]any {
	return map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": name}, "containers": map[string]any{"main": map[string]any{"image": "example.invalid/" + name + ":1"}}}
}

func TestTransitionOverHTTPPreviewExecuteProgressAndCleanup(t *testing.T) {
	server, opts, app := newServerApp(t, "")
	app.Transitions.SetAsync(false)
	developer := authenticatedClient(t, server.URL)
	ctx := context.Background()
	conn := appdomain.Connection{ID: ids.New(), Key: "lab2", Name: "Second", OrganizationKey: opts.OrganizationKey, Kind: appdomain.ConnectionKubernetes, AuthenticationType: appdomain.AuthHostContext, Status: appdomain.ConnectionReady,
		Config: map[string]any{"cluster": "second", "kubeContext": "second-context", "endpoint": "https://second.invalid"}, SecretRef: "host-kube-context://second-context", Verification: map[string]any{}}
	if err := app.Store.SaveConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}
	def := resource.Definition{Key: "cluster-internal-lab2", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, ConnectionKey: "lab2",
		DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:     []resource.Criterion{{Class: "internal", ResourceID: "connections.lab2"}}}
	if err := app.Store.SaveResourceDefinition(ctx, opts.OrganizationKey, def); err != nil {
		t.Fatal(err)
	}
	created := createConfiguredApplication(t, developer, server.URL, "Transition Test", "transition-test", "internal-cluster")
	appKey := created["application"].(map[string]any)["key"].(string)
	base := server.URL + "/api/v1/applications/" + appKey + "/environments/staging"

	// No runtime yet: a transition is refused and a plain Save is how it changes.
	if status, out := requestJSON(t, developer, http.MethodPost, base+"/connection-transition/preview", map[string]any{"destinationKey": "lab2", "mode": "DEPLOY_NEW"}); status != http.StatusConflict || out["code"] != "NO_RUNTIME" {
		t.Fatalf("no runtime: %d %v", status, out)
	}
	if status, _ := requestJSON(t, developer, http.MethodPut, base+"/workloads/web", map[string]any{"score": extraScore("web"), "version": 0}); status != http.StatusOK {
		t.Fatal("draft")
	}
	_, preview := requestJSON(t, developer, http.MethodPost, base+"/preview", map[string]any{})
	if status, out := requestJSON(t, developer, http.MethodPost, base+"/deploy", map[string]any{"token": preview["token"]}); status != http.StatusOK || out["status"] != "SUCCEEDED" {
		t.Fatalf("deploy: %d %v", status, out)
	}
	// The Settings Save now needs the explicit transition.
	_, envs := requestJSON(t, developer, http.MethodGet, server.URL+"/api/v1/applications/"+appKey, nil)
	var version any
	for _, raw := range envs["application"].(map[string]any)["environments"].([]any) {
		if view := raw.(map[string]any); view["key"] == "staging" {
			version = view["version"]
			if view["runtimeExists"] != true {
				t.Fatalf("runtimeExists must be reported: %v", view)
			}
		}
	}
	if status, out := requestJSON(t, developer, http.MethodPut, base+"/connection", map[string]any{"connectionKey": "lab2", "expectedVersion": version}); status != http.StatusConflict || out["code"] != "RUNTIME_EXISTS" {
		t.Fatalf("save with runtime: %d %v", status, out)
	}
	if status, out := requestJSON(t, developer, http.MethodPost, base+"/connection-transition/preview", map[string]any{"destinationKey": "internal-cluster", "mode": "DEPLOY_NEW"}); status != http.StatusUnprocessableEntity || out["field"] != "destinationKey" {
		t.Fatalf("same destination: %d %v", status, out)
	}
	if status, out := requestJSON(t, developer, http.MethodPost, base+"/connection-transition/preview", map[string]any{"destinationKey": "lab2", "mode": "MIGRATE_POSTGRES"}); status != http.StatusUnprocessableEntity || out["code"] != "TRANSITION_UNSUPPORTED" {
		t.Fatalf("no database to migrate must be unsupported: %d %v", status, out)
	}
	status, pv := requestJSON(t, developer, http.MethodPost, base+"/connection-transition/preview", map[string]any{"destinationKey": "lab2", "mode": "DEPLOY_NEW"})
	token := pv["preview"].(map[string]any)["token"]
	if status != http.StatusOK || token == "" {
		t.Fatalf("preview: %d %v", status, pv)
	}
	if status, out := requestJSON(t, developer, http.MethodPost, base+"/connection-transitions", map[string]any{"destinationKey": "lab2", "mode": "DEPLOY_NEW", "token": "forged"}); status != http.StatusConflict || out["code"] != "STALE_PREVIEW" {
		t.Fatalf("forged token: %d %v", status, out)
	}
	status, executed := requestJSON(t, developer, http.MethodPost, base+"/connection-transitions", map[string]any{"destinationKey": "lab2", "mode": "DEPLOY_NEW", "token": token})
	detail := executed["transition"].(map[string]any)
	if status != http.StatusAccepted || detail["status"] != "SUCCEEDED" || detail["canCleanupSource"] != true {
		t.Fatalf("execute: %d %v", status, executed)
	}
	id := detail["id"].(string)
	if status, got := requestJSON(t, developer, http.MethodGet, base+"/connection-transitions/"+id, nil); status != http.StatusOK || got["transition"].(map[string]any)["destination"].(map[string]any)["connectionKey"] != "lab2" {
		t.Fatalf("get: %d %v", status, got)
	}
	if status, list := requestJSON(t, developer, http.MethodGet, base+"/connection-transitions", nil); status != http.StatusOK || len(list["transitions"].([]any)) != 1 {
		t.Fatalf("list: %d %v", status, list)
	}
	// The retained source can be cleaned only explicitly; the first request deletes it.
	status, cleaned := requestJSON(t, developer, http.MethodPost, base+"/connection-transitions/"+id+"/cleanup-source", map[string]any{})
	if status != http.StatusOK || cleaned["transition"].(map[string]any)["sourceState"] != "CLEANED" {
		t.Fatalf("cleanup: %d %v", status, cleaned)
	}
	if status, out := requestJSON(t, developer, http.MethodPost, base+"/connection-transitions/"+id+"/cleanup-source", map[string]any{}); status != http.StatusConflict || out["code"] != "CLEANUP_REFUSED" {
		t.Fatalf("second cleanup: %d %v", status, out)
	}
	// Recovery needs the explicit confirmation and an interrupted operation.
	if status, out := requestJSON(t, developer, http.MethodPost, base+"/operations/"+ids.New()+"/recover", map[string]any{"priorExecutionStopped": false}); status != http.StatusUnprocessableEntity || out["field"] != "priorExecutionStopped" {
		t.Fatalf("recover without confirmation: %d %v", status, out)
	}
	if status, _ := requestJSON(t, developer, http.MethodPost, base+"/operations/"+ids.New()+"/recover", map[string]any{"priorExecutionStopped": true}); status != http.StatusNotFound {
		t.Fatalf("recover unknown: %d", status)
	}
}
