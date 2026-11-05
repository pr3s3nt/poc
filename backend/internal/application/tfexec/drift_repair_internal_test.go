package tfexec

import (
	"orchestrator/internal/domain/drift"
	"orchestrator/internal/ports/terraformexec"
	"testing"
)

func TestReviewedInventoryPermitsOnlyDeclaredPhysicalChanges(t *testing.T) {
	old := []terraformexec.StateResource{{Address: "example.a", Type: "example", ID: "old", UID: "old-uid"}}
	replaced := []terraformexec.StateResource{{Address: "example.a", Type: "example", ID: "new", UID: "new-uid"}}
	report := drift.Report{Complete: true, Proposed: []drift.Finding{{Address: "example.a", Actions: []string{"delete", "create"}}}}
	if !reviewedInventory(old, replaced, report) {
		t.Fatal("reviewed replacement rejected")
	}
	report.Proposed[0].Actions = []string{"update"}
	if reviewedInventory(old, replaced, report) {
		t.Fatal("identity changed on update")
	}
	report.Proposed[0].Actions = []string{"delete"}
	if !reviewedInventory(old, nil, report) || reviewedInventory(old, replaced, report) {
		t.Fatal("delete result not verified")
	}
	report.Proposed[0].Actions = []string{"create"}
	if !reviewedInventory(nil, replaced, report) || reviewedInventory(nil, nil, report) {
		t.Fatal("creation result not verified")
	}
	if reviewedInventory(old, append(old, terraformexec.StateResource{Address: "example.foreign", Type: "example", ID: "foreign"}), drift.Report{}) {
		t.Fatal("unreviewed resource adopted")
	}
	if reviewedInventory(old, nil, drift.Report{}) {
		t.Fatal("unreviewed deletion admitted")
	}
}
