package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// The fixture topology (FAKE: terraform_data objects arranged by this launcher;
// the Application itself is made through the product API):
//
//	ROOT1 postgres  shared.orders-db        consumes ORDERS-NET, SHARED-NET, SESSIONS   (all UNREFERENCED)
//	ORDERS-NET vpc  environments.orders-net consumed only by ROOT1                      (valid dependency)
//	SHARED-NET vpc  environments.shared-net also consumed by LIVE-CACHE                 (locked: retained consumer)
//	SESSIONS redis  shared.session-store    consumed only by ROOT1                      (valid, left unticked)
//	LIVE-CACHE redis shared.live-cache      READY, consumes SHARED-NET                  (outside the set)
//	ROOT2 postgres  shared.audit-db         consumes AUDIT-NET; its destroy FAILS       (partial-failure scenario)
//	AUDIT-NET vpc   environments.audit-net  consumed only by ROOT2
type fixtureResource struct {
	Key, Descriptor, Type string
	ScopeType             resource.ScopeType
	DestroyFails          bool
	Unreferenced          bool
	Providers             []string
}

var topology = []fixtureResource{
	{Key: "ordersNet", Descriptor: "vpc.default#environments.payments.orders-net", Type: "vpc", ScopeType: resource.ScopeEnvironment, Unreferenced: true},
	{Key: "sharedNet", Descriptor: "vpc.default#environments.payments.shared-net", Type: "vpc", ScopeType: resource.ScopeEnvironment, Unreferenced: true},
	{Key: "sessions", Descriptor: "redis.default#shared.session-store", Type: "redis", ScopeType: resource.ScopeShared, Unreferenced: true},
	{Key: "root1", Descriptor: "postgres.default#shared.orders-db", Type: "postgres", ScopeType: resource.ScopeShared, Unreferenced: true, Providers: []string{"ordersNet", "sharedNet", "sessions"}},
	{Key: "liveCache", Descriptor: "redis.default#shared.live-cache", Type: "redis", ScopeType: resource.ScopeShared, Providers: []string{"sharedNet"}},
	{Key: "auditNet", Descriptor: "vpc.default#environments.payments.audit-net", Type: "vpc", ScopeType: resource.ScopeEnvironment, Unreferenced: true},
	{Key: "root2", Descriptor: "postgres.default#shared.audit-db", Type: "postgres", ScopeType: resource.ScopeShared, DestroyFails: true, Unreferenced: true, Providers: []string{"auditNet"}},
}

// seedTopology creates the Application through the product API (as a Platform
// Engineer), then CREATEs every fixture resource through the real executor
// under one Environment claim and marks the unreferenced ones, the way a
// successful Remove leaves them. The IDs are written to scenarioFile.
func seedTopology(ctx context.Context, app *bootstrap.App, eng *engine, scenarioFile string) error {
	cookie, err := signIn(app, "platform-engineer", seed.FixedTestPassword)
	if err != nil {
		return err
	}
	appKey, err := createApplication(app, cookie, "Payments", "payments")
	if err != nil {
		return err
	}
	opts := seed.Defaults()
	org, err := app.Store.GetOrganization(ctx, opts.OrganizationKey)
	if err != nil {
		return err
	}
	env, err := app.Store.GetEnvironment(ctx, appKey, "staging")
	if err != nil {
		return err
	}
	op, err := app.Store.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: appKey, EnvironmentKey: "staging", Owner: "fixture-seed", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		return fmt.Errorf("claim the Environment for the fixture: %w", err)
	}
	owner := persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
	ownerCtx := persistence.WithOwner(ctx, owner)
	release := func(status environment.OperationStatus) error {
		return app.Store.ReleaseOperation(ctx, owner, status, "")
	}
	ids := map[string]string{}
	for _, res := range topology {
		d, err := resource.ParseDescriptor(res.Descriptor)
		if err != nil {
			_ = release(environment.OpFailed)
			return err
		}
		scope := resource.Scope{Type: res.ScopeType, ID: "payments.staging"}
		var providers []string
		for _, key := range res.Providers {
			providers = append(providers, ids[key])
		}
		result, err := eng.Execute(ownerCtx, tfexec.ExecuteRequest{
			RunID: "fixture-seed", Descriptor: res.Descriptor, Scope: scope, Generation: 0,
			Identity: tfresource.Identity{OrganizationID: org.ID, ApplicationID: env.ApplicationID, EnvironmentID: env.ID, TargetGeneration: 0, Descriptor: res.Descriptor, Scope: scope},
			Source:   fixtureBundle(eng.bundleID).Ref, Type: fixtureType(d.Type),
			Inputs: map[string]any{"note": d.ID, "stable": d.ID, "destroy_fails": res.DestroyFails}, OutputMapping: identityMapping(),
			ProviderIDs: providers, OrganizationKey: opts.OrganizationKey,
		})
		if err != nil {
			_ = release(environment.OpFailed)
			return fmt.Errorf("create the fixture resource %s: %w", d.ID, err)
		}
		ids[res.Key] = result.InstanceID
	}
	for _, res := range topology {
		if !res.Unreferenced {
			continue
		}
		inst, err := eng.state.GetInstance(ownerCtx, ids[res.Key])
		if err != nil {
			_ = release(environment.OpFailed)
			return err
		}
		if _, err := eng.state.SetInstanceStatus(ownerCtx, inst.ID, inst.Version, tfresource.InstanceUnreferenced); err != nil {
			_ = release(environment.OpFailed)
			return err
		}
	}
	if err := release(environment.OpSucceeded); err != nil {
		return err
	}
	out := map[string]any{"applicationId": appKey, "environment": "staging", "resources": ids, "descriptors": map[string]string{}, "terraform": "terraform_data fixture (FAKE objects)"}
	descriptors := map[string]string{}
	for _, res := range topology {
		descriptors[res.Key] = res.Descriptor
	}
	out["descriptors"] = descriptors
	encoded, _ := json.MarshalIndent(out, "", "  ")
	if scenarioFile == "" {
		return errors.New("-scenario-file is required with -seed")
	}
	return os.WriteFile(scenarioFile, encoded, 0o600)
}

func signIn(app *bootstrap.App, username, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	app.Server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return "", fmt.Errorf("fixture sign-in failed (%d)", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		return c.Name + "=" + c.Value, nil
	}
	return "", errors.New("fixture sign-in returned no session")
}

func createApplication(app *bootstrap.App, cookie, name, subdomain string) (string, error) {
	body, _ := json.Marshal(map[string]string{"name": name, "subdomain": subdomain})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	app.Server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		return "", fmt.Errorf("fixture application creation failed (%d)", rec.Code)
	}
	var out struct {
		Application struct {
			Key string `json:"key"`
		} `json:"application"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Application.Key == "" {
		return "", errors.New("fixture application response was not understood")
	}
	return out.Application.Key, nil
}
