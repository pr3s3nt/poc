package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// mutateSnapshot rewrites a persisted snapshot's top-level fields and returns
// the written bytes.
func mutateSnapshot(t *testing.T, path string, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	mutate(fields)
	out, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return out
}

// seededSnapshot persists a current snapshot of the normal fresh seed.
func seededSnapshot(t *testing.T) (string, seed.Options) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	opts := seed.Defaults()
	if err := seedtest.Apply(context.Background(), st, opts); err != nil {
		t.Fatal(err)
	}
	return path, opts
}

// A current snapshot reopens unchanged: the loader neither adds defaults nor
// rewrites anything, repeatedly.
func TestCurrentSnapshotReopensUnchanged(t *testing.T) {
	path, opts := seededSnapshot(t)
	first, _ := os.ReadFile(path)
	for i := 0; i < 3; i++ {
		st, err := NewWithSnapshot(path)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		if _, err := st.GetEnvironment(context.Background(), opts.ApplicationKey, opts.EnvironmentKey); err != nil {
			t.Fatal(err)
		}
		if after, _ := os.ReadFile(path); string(after) != string(first) {
			t.Fatalf("reopen %d rewrote the snapshot", i)
		}
	}
	// The seeded AWS process record keeps its explicit current metadata.
	st, _ := NewWithSnapshot(path)
	cloud, err := st.GetConnection(context.Background(), opts.OrganizationKey, opts.CloudConnectionKey)
	if err != nil || cloud.AuthenticationType != application.AuthAWSAccessKey || cloud.Name != opts.CloudConnectionKey {
		t.Fatalf("cloud connection = %+v, %v", cloud, err)
	}
}

// An empty current store writes every aggregate as an object; empty is valid.
func TestEmptyCurrentSnapshotIsValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Transact(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range requiredSnapshotKeys {
		if string(fields[key]) != "{}" {
			t.Fatalf("%s = %q, want {}", key, fields[key])
		}
	}
	if _, err := NewWithSnapshot(path); err != nil {
		t.Fatalf("empty current snapshot rejected: %v", err)
	}
}

// Every aggregate the serializer writes is required; absent or null is not
// defaulted. The rejected file is never rewritten and the error quotes nothing.
func TestMissingOrNullAggregateIsRejected(t *testing.T) {
	for _, key := range requiredSnapshotKeys {
		for _, how := range []string{"missing", "null"} {
			path, _ := seededSnapshot(t)
			raw := mutateSnapshot(t, path, func(f map[string]json.RawMessage) {
				if how == "missing" {
					delete(f, key)
				} else {
					f[key] = json.RawMessage("null")
				}
			})
			st, err := NewWithSnapshot(path)
			if st != nil || !errors.Is(err, ErrResetRequired) || strings.Contains(err.Error(), "acme") {
				t.Fatalf("%s %s: store=%v err=%v", key, how, st, err)
			}
			if after, _ := os.ReadFile(path); string(after) != string(raw) {
				t.Fatalf("%s %s: snapshot rewritten", key, how)
			}
		}
	}
}

// Retired or unknown fields fail closed with a safe reset error.
func TestRetiredOrUnknownFieldsAreRejected(t *testing.T) {
	cases := map[string]func(map[string]json.RawMessage){
		"unknown top-level": func(f map[string]json.RawMessage) { f["legacyThing"] = json.RawMessage(`{}`) },
		"application target": func(f map[string]json.RawMessage) {
			f["applications"] = json.RawMessage(`{"old":{"id":"a1","key":"old","organizationKey":"acme","name":"Old","subdomain":"old","executionProfile":"internal-k8s","version":1}}`)
		},
		"application empty retired": func(f map[string]json.RawMessage) {
			f["applications"] = json.RawMessage(`{"old":{"id":"a1","key":"old","organizationKey":"acme","name":"Old","subdomain":"old","runtimeStatus":"UNCONFIGURED","version":1}}`)
		},
		"trailing data": nil,
	}
	for name, mutate := range cases {
		path, _ := seededSnapshot(t)
		var raw []byte
		if mutate == nil {
			raw, _ = os.ReadFile(path)
			raw = append(raw, []byte(` {}`)...)
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
		} else {
			raw = mutateSnapshot(t, path, mutate)
		}
		st, err := NewWithSnapshot(path)
		if st != nil || !errors.Is(err, ErrResetRequired) || strings.Contains(err.Error(), "internal-k8s") || strings.Contains(err.Error(), "legacyThing") {
			t.Fatalf("%s: store=%v err=%v", name, st, err)
		}
		if after, _ := os.ReadFile(path); string(after) != string(raw) {
			t.Fatalf("%s: snapshot rewritten", name)
		}
	}
}

// editRecords decodes one aggregate of a snapshot as generic objects, lets fn
// change them and stores them back.
func editRecords(t *testing.T, f map[string]json.RawMessage, aggregate string, fn func(map[string]map[string]any)) {
	t.Helper()
	var records map[string]map[string]any
	if err := json.Unmarshal(f[aggregate], &records); err != nil {
		t.Fatal(err)
	}
	fn(records)
	f[aggregate], _ = json.Marshal(records)
}

// editOne changes one record of an aggregate, selected by pick (nil: any).
func editOne(t *testing.T, aggregate string, pick func(map[string]any) bool, fn func(map[string]any)) func(map[string]json.RawMessage) {
	return func(f map[string]json.RawMessage) {
		editRecords(t, f, aggregate, func(records map[string]map[string]any) {
			for _, record := range records {
				if pick == nil || pick(record) {
					fn(record)
					return
				}
			}
			t.Fatalf("no %s record matched", aggregate)
		})
	}
}

func put(t *testing.T, aggregate, key string, record map[string]any) func(map[string]json.RawMessage) {
	return func(f map[string]json.RawMessage) {
		editRecords(t, f, aggregate, func(records map[string]map[string]any) { records[key] = record })
	}
}

// A syntactically valid snapshot whose current records are incomplete,
// misowned, inconsistent or dangling is rejected before serving: nothing is
// defaulted, inferred or repaired, the file is not rewritten and no value is
// quoted in the error.
func TestIncompleteOrDanglingCurrentRecordsAreRejected(t *testing.T) {
	opts := seed.Defaults()
	kubernetes := func(r map[string]any) bool { return r["kind"] == "KUBERNETES" }
	env := func(fn func(map[string]any)) func(map[string]json.RawMessage) {
		return editOne(t, "environments", nil, fn)
	}
	cases := map[string]func(map[string]json.RawMessage){
		"empty application record":         put(t, "applications", "ghost", map[string]any{}),
		"application without organization": editOne(t, "applications", nil, func(r map[string]any) { r["organizationKey"] = "nobody-org" }),
		"application without name":         editOne(t, "applications", nil, func(r map[string]any) { r["name"] = "" }),
		"organization misplaced": func(f map[string]json.RawMessage) {
			editRecords(t, f, "organizations", func(r map[string]map[string]any) {
				for k, v := range r {
					r["other"] = v
					delete(r, k)
					return
				}
			})
		},
		"connection without name":                  editOne(t, "connections", nil, func(r map[string]any) { delete(r, "name") }),
		"connection without version":               editOne(t, "connections", nil, func(r map[string]any) { delete(r, "version") }),
		"connection with zero version":             editOne(t, "connections", nil, func(r map[string]any) { r["version"] = 0 }),
		"connection host context":                  editOne(t, "connections", kubernetes, func(r map[string]any) { r["authenticationType"] = "HOST_CONTEXT" }),
		"connection without authentication type":   editOne(t, "connections", kubernetes, func(r map[string]any) { delete(r, "authenticationType") }),
		"connection of a missing organization":     editOne(t, "connections", nil, func(r map[string]any) { r["organizationKey"] = "nobody-org" }),
		"environment without scope":                env(func(r map[string]any) { delete(r, "infrastructureScope") }),
		"retired Organization default choice":      editOne(t, "organizations", nil, func(r map[string]any) { r["defaultConnectionKey"] = "retired-choice-do-not-leak" }),
		"environment without infrastructure scope": env(func(r map[string]any) { delete(r, "infrastructureScope") }),
		"environment without application":          env(func(r map[string]any) { r["applicationKey"] = "nobody-app" }),
		"environment without namespace":            env(func(r map[string]any) { r["namespaceIdentity"] = "" }),
		"unconfigured with profile":                env(func(r map[string]any) { r["executionProfile"] = "aws-eks" }),
		"bound to a missing connection": env(func(r map[string]any) {
			r["connectionKey"], r["executionProfile"], r["runtimeStatus"] = "nowhere", "internal-k8s", "READY"
		}),
		"bound without profile": env(func(r map[string]any) {
			delete(r, "executionProfile")
			r["connectionKey"], r["runtimeStatus"] = opts.ConnectionKey, "READY"
		}),
		"aws without region": env(func(r map[string]any) {
			r["connectionKey"], r["executionProfile"], r["runtimeStatus"] = opts.CloudConnectionKey, "aws-eks", "READY"
		}),
		"missing secret store":                    env(func(r map[string]any) { r["secretStoreKey"] = "vault-gone" }),
		"dangling active operation":               env(func(r map[string]any) { r["activeOperationId"] = "op-gone" }),
		"dangling deployment set":                 env(func(r map[string]any) { r["currentDeploymentSetId"] = "set-gone" }),
		"negative generation":                     env(func(r map[string]any) { r["targetGeneration"] = -1 }),
		"configuration scope without revision":    put(t, "configurationScopes", opts.ApplicationKey+"/"+opts.EnvironmentKey, map[string]any{"applicationKey": opts.ApplicationKey, "environmentKey": opts.EnvironmentKey, "desiredRevisionId": "rev-gone", "version": 1}),
		"revision of a missing environment":       put(t, "configurationRevisions", "r1", map[string]any{"id": "r1", "applicationKey": "nobody-app", "environmentKey": "dev", "version": 1, "entries": map[string]any{}}),
		"revision with old store-backed variable": put(t, "configurationRevisions", "r2", map[string]any{"id": "r2", "applicationKey": opts.ApplicationKey, "environmentKey": opts.EnvironmentKey, "version": 1, "entries": map[string]any{"K": map[string]any{"kind": "VARIABLE", "valueRef": "kv2://x", "storeKey": "s"}}}),
		"workload instance without deployment":    put(t, "workloadInstances", "env/api", map[string]any{"id": "w1", "environmentKey": opts.ApplicationKey + "/" + opts.EnvironmentKey, "workloadId": "api", "lastDeploymentId": "dep-gone", "targetRef": map[string]any{}, "manifestDigest": "d", "status": "READY"}),
		"workload history without deployment":     put(t, "deploymentWorkloads", "dep-gone/api", map[string]any{"deploymentId": "dep-gone", "workloadId": "api"}),
		"operation without environment":           put(t, "environmentOperations", "op1", map[string]any{"id": "op1", "applicationKey": "nobody-app", "environmentKey": "dev", "status": "ACTIVE"}),
		"deployment record without id":            put(t, "deployments", "dep1", map[string]any{}),
	}
	for name, mutate := range cases {
		path, _ := seededSnapshot(t)
		raw := mutateSnapshot(t, path, mutate)
		st, err := NewWithSnapshot(path)
		if st != nil || !errors.Is(err, ErrResetRequired) {
			t.Errorf("%s: store=%v err=%v", name, st, err)
			continue
		}
		for _, leak := range []string{"nobody", "nowhere", "vault-gone", "op-gone", "set-gone", "kv2://", "HOST_CONTEXT", "dep-gone"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: error leaks %q: %v", name, leak, err)
			}
		}
		if after, _ := os.ReadFile(path); string(after) != string(raw) {
			t.Errorf("%s: snapshot rewritten", name)
		}
	}
}

// An explicit non-ENVIRONMENT scope keeps the typed scope error and never
// quotes the stored value.
func TestRetiredInfrastructureScopeIsRejected(t *testing.T) {
	for name, scope := range map[string]string{"retired": "LEGACY_APPLICATION", "unknown": "x-hostile-scope"} {
		path, _ := seededSnapshot(t)
		raw := mutateSnapshot(t, path, editOne(t, "environments", nil, func(r map[string]any) { r["infrastructureScope"] = scope }))
		st, err := NewWithSnapshot(path)
		if err == nil || st != nil || !errors.Is(err, environment.ErrInvalidInfrastructureScope) || !errors.Is(err, ErrResetRequired) || strings.Contains(err.Error(), scope) {
			t.Fatalf("%s: store=%v err=%v", name, st, err)
		}
		if after, _ := os.ReadFile(path); string(after) != string(raw) {
			t.Fatalf("%s: snapshot was rewritten", name)
		}
	}
}

// Current generation bindings, operation history and historical Deployment
// targets (an older Connection key and generation) reopen exactly: the file is
// byte-stable across load and persist, and the loader never rewrites identities.
func TestCurrentGenerationAndHistoricalTargetsReopenUnchanged(t *testing.T) {
	ctx := context.Background()
	for name, build := range map[string]func(*testing.T, *Store) func(*testing.T, persistence.Store){
		"historical deployment targets": func(t *testing.T, st *Store) func(*testing.T, persistence.Store) {
			fixture := persistencetest.WorkloadSnapshots(t, st)
			return func(t *testing.T, reopened persistence.Store) { persistencetest.AssertReloaded(t, reopened, fixture) }
		},
	} {
		path := filepath.Join(t.TempDir(), "state.json")
		first, err := NewWithSnapshot(path)
		if err != nil {
			t.Fatal(err)
		}
		assertReloaded := build(t, first)
		written, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := NewWithSnapshot(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		assertReloaded(t, reopened)
		// Persisting the reopened state again reproduces the same bytes.
		if err := reopened.Transact(ctx, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if again, _ := os.ReadFile(path); string(again) != string(written) {
			t.Fatalf("%s: reopen and persist changed the snapshot", name)
		}
	}
}

// UC-03 V02-08: a persisted Resource Definition needs an explicit current Name
// and positive version and no retired profile/Connection field. Nothing is
// inferred from the ID, defaulted or migrated; the file is never rewritten.
func TestOldOrIncompleteDefinitionStateRequiresReset(t *testing.T) {
	cases := map[string]func(map[string]any){
		"no name":               func(r map[string]any) { delete(r, "name") },
		"blank name":            func(r map[string]any) { r["name"] = "  " },
		"no version":            func(r map[string]any) { delete(r, "version") },
		"zero version":          func(r map[string]any) { r["version"] = 0 },
		"retired profile":       func(r map[string]any) { r["executionProfile"] = "aws-eks" },
		"retired connectionKey": func(r map[string]any) { r["connectionKey"] = "aws-account" },
		"foreign account":       func(r map[string]any) { r["driverAccount"] = "nowhere" },
	}
	for name, change := range cases {
		path, _ := seededSnapshot(t)
		raw := mutateSnapshot(t, path, editOne(t, "resourceDefinitions", nil, change))
		st, err := NewWithSnapshot(path)
		if st != nil || !errors.Is(err, ErrResetRequired) || strings.Contains(err.Error(), "aws-account") || strings.Contains(err.Error(), "nowhere") {
			t.Fatalf("%s: store=%v err=%v", name, st, err)
		}
		if after, _ := os.ReadFile(path); string(after) != string(raw) {
			t.Fatalf("%s: snapshot rewritten", name)
		}
	}
}

// A saved edit survives reopen with its Name, version, account and criteria.
func TestDefinitionEditSurvivesReopenInTheJSONSnapshot(t *testing.T) {
	path, opts := seededSnapshot(t)
	ctx := context.Background()
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defs, _ := st.ListResourceDefinitions(ctx, opts.OrganizationKey)
	var target = defs[0]
	for _, d := range defs {
		if d.Key == "postgres-internal-statefulset" {
			target = d
		}
	}
	target.Name = "Đã đổi tên"
	target.Criteria = append(target.Criteria, resource.Criterion{Class: "extra"})
	saved, err := st.UpdateResourceDefinition(ctx, opts.OrganizationKey, target.Version, target)
	if err != nil || saved.Version != target.Version+1 {
		t.Fatalf("update: %+v %v", saved, err)
	}
	reopened, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	after, _ := reopened.ListResourceDefinitions(ctx, opts.OrganizationKey)
	for _, d := range after {
		if d.Key == target.Key && (d.Name != "Đã đổi tên" || d.Version != saved.Version || len(d.Criteria) != 2) {
			t.Fatalf("reopened = %+v", d)
		}
	}
}
