package deployment_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	appsvc "orchestrator/internal/application/deployment"
	domain "orchestrator/internal/domain/deployment"
)

const (
	blockedCluster   = "k8s-cluster.internal#environments.shop.dev"
	blockedNamespace = "k8s-namespace.default#environments.shop.dev"
	blockedDatabase  = "postgres.in-cluster#shared.db"
	blockedWorkload  = "workload.default#modules.backend"
)

// ADR-017: a provider that failed holds its consumers back. The view names each
// held-back consumer, the graph edge reason and a SAFE code (a persisted attempt
// code, or REJECTED_BEFORE_RUN for a provider refused before its module ran),
// derived from the persisted plan and progress only.
func TestGetDeploymentNamesConsumersBlockedByAFailedProvider(t *testing.T) {
	ctx := context.Background()
	base, opts, id := queryFixture(t)
	scope := appsvc.GetDeploymentQuery{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, DeploymentID: id}
	graph := map[string]any{
		"nodes": []any{},
		"edges": []any{
			map[string]any{"consumer": blockedNamespace, "provider": blockedCluster, "reason": "provider-binding"},
			map[string]any{"consumer": blockedDatabase, "provider": blockedNamespace, "reason": "resource-reference"},
			map[string]any{"consumer": blockedWorkload, "provider": blockedDatabase, "reason": "workload-placeholder"},
			map[string]any{"consumer": blockedWorkload, "provider": blockedNamespace, "reason": "resource-reference"},
		},
	}
	if err := base.SavePlan(ctx, id, map[string]any{"graph": graph, "planHash": "h"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	save := func(descriptor string, status domain.ResourceStatus) {
		t.Helper()
		if err := base.SaveDeploymentResource(ctx, domain.Resource{DeploymentID: id, NodeDescriptor: descriptor, Status: status, StartedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	save(blockedCluster, domain.ResourceReady)
	save(blockedNamespace, domain.ResourceReady)
	save(blockedDatabase, domain.ResourceFailed)

	// The database was refused before its module ran: no attempt exists.
	view, err := appsvc.NewQueryService(base).GetDeployment(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Blocked) != 1 || view.Blocked[0] != (appsvc.BlockedView{Consumer: blockedWorkload, Provider: blockedDatabase, Reason: "workload-placeholder", Code: appsvc.RejectedBeforeRun}) {
		t.Fatalf("blocked = %+v", view.Blocked)
	}

	// With a persisted failed attempt the safe attempt code is reported instead.
	attempts := []domain.ResourceAttempt{{Descriptor: blockedDatabase, ScopeType: "SHARED", Number: 1, Action: "CREATE", Ownership: "CREATED_BY_RUN", Stage: "OUTPUTS", Status: "FAILED", SafeErrorCode: "OUTPUT_CONTRACT", ModuleID: "postgres-k8s", ModuleRevision: "r1", StartedAt: now, UpdatedAt: now}}
	view, err = appsvc.NewQueryService(&attemptStore{countingStore: base, attempts: attempts}).GetDeployment(ctx, scope)
	if err != nil || len(view.Blocked) != 1 || view.Blocked[0].Code != "OUTPUT_CONTRACT" {
		t.Fatalf("blocked with attempt = %+v %v", view.Blocked, err)
	}
	raw, _ := json.Marshal(view.Blocked)
	for _, leak := range []string{"secretRef", "vault://", "password", "token"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("blocked view leaks %q: %s", leak, raw)
		}
	}

	// Persisted SafeErrorCode text is not trusted by either public projection.
	// The restarted query continues to show a truthful failed provider while
	// replacing credential-bearing codes everywhere in the response.
	malicious := attempts[0]
	malicious.SafeErrorCode = "AUTH:hvs.REDACTED-CANARY-DO-NOT-LEAK"
	view, err = appsvc.NewQueryService(&attemptStore{countingStore: base, attempts: []domain.ResourceAttempt{malicious}}).GetDeployment(ctx, scope)
	if err != nil || len(view.Blocked) != 1 || view.Blocked[0].Code != domain.AttemptUnknownError || view.ResourceProgress[0].SafeErrorCode != domain.AttemptUnknownError {
		t.Fatalf("unsafe persisted code projection = %+v, %v", view, err)
	}
	public, _ := json.Marshal(view)
	for _, leak := range []string{"hvs.REDACTED-CANARY-DO-NOT-LEAK", "AUTH:", "REDACTED-CANARY"} {
		if strings.Contains(string(public), leak) {
			t.Fatalf("public deployment view leaks persisted provider code %q: %s", leak, public)
		}
	}

	// A healthy deployment has an empty (not null) list.
	save(blockedDatabase, domain.ResourceReady)
	view, err = appsvc.NewQueryService(base).GetDeployment(ctx, scope)
	if err != nil || view.Blocked == nil || len(view.Blocked) != 0 {
		t.Fatalf("healthy deployment blocked = %#v %v", view.Blocked, err)
	}
}
