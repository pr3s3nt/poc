package delivery

import (
	"encoding/json"
	"strings"
	"testing"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/execution"
)

func TestIntentRejectsUnscopedOrForeignExecution(t *testing.T) {
	revision := domain.Revision{ID: "revision", Scope: domain.Scope{OrganizationID: "org", EnvironmentID: "env", UnitKey: "rollout"}, Phase: domain.Rollout, ArtifactRef: "private", PayloadDigest: strings.Repeat("a", 64), Selection: domain.Selection{Mode: domain.CD, BindingID: "binding", BindingVersion: 1}}
	i := Intent{ID: "intent", Owner: Owner{OperationID: "op", Instance: "worker", Fence: 1}, Revision: revision, Target: execution.Target{Organization: "org", Connection: "account"}, Binding: BindingVersion{ID: "binding", Version: 1, OrganizationID: "org", Capabilities: domain.Capabilities{ContractVersion: "v1", ExactRevision: true, PhasedDelivery: true, RetainOnWithdraw: true, GuardedPrune: true}}}
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Intent){
		"host context":          func(i *Intent) { i.Target = execution.Target{Context: "host-default"} },
		"wrong binding version": func(i *Intent) { i.Binding.Version = 2 },
		"foreign binding":       func(i *Intent) { i.Binding.OrganizationID = "other" },
		"no owner":              func(i *Intent) { i.Owner = Owner{} },
	} {
		t.Run(name, func(t *testing.T) {
			copy := i
			mutate(&copy)
			if copy.Validate() == nil {
				t.Fatal("unsafe intent admitted")
			}
		})
	}
	b, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "private") {
		t.Fatal("artifact ref exposed in public revision")
	}
}

type providerStub struct{ Provider }

func TestRegistryDoesNotFallbackToAnotherProvider(t *testing.T) {
	r := NewRegistry()
	fleet := &providerStub{}
	second := &providerStub{}
	if err := r.Register("fleet", fleet); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("future", second); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("fleet", second); err == nil {
		t.Fatal("silently replaced provider")
	}
	if got, err := r.Resolve("future"); err != nil || got != second {
		t.Fatal("wrong provider", err)
	}
	if _, err := r.Resolve("uninstalled"); err == nil {
		t.Fatal("fell back to installed provider")
	}
}
