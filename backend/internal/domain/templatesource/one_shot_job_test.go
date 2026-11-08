package templatesource

import (
	"orchestrator/internal/domain/environment"
	"testing"
)

func TestOneShotJobRequiresAdvertisedSourceCapability(t *testing.T) {
	e := &environment.WorkloadExecution{Version: 1, Kind: "Job"}
	m := Manifest{WorkloadCapabilities: []string{CustomExecutionV1}}
	if ValidateWorkloadCapabilities(m, nil, e) == nil {
		t.Fatal("old source accepted Job")
	}
	m.WorkloadCapabilities = append(m.WorkloadCapabilities, OneShotJobV1)
	if err := ValidateWorkloadCapabilities(m, nil, e); err != nil {
		t.Fatal(err)
	}
	if err := validateCapabilities(m.WorkloadCapabilities); err != nil {
		t.Fatal(err)
	}
}
