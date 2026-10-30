package tfexec

import (
	"context"
	"encoding/json"

	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformstate"
)

// Lifecycle/state versions can advance during a reviewed stable-output repair.
// Resolve the original exact acknowledged version, never today's Definition or
// variables, and compare its private output contract with the new acknowledgement.
func (s *Service) stableDependencyOutputs(ctx context.Context, org string, pin terraformstate.BaselineDependency, current tfresource.Instance, state *tfresource.StateBlob) bool {
	if state == nil || (current.Status != tfresource.InstanceReady && current.Status != tfresource.InstanceUnreferenced) {
		return false
	}
	original, err := s.deps.Baselines.FindExecutionBaselineVersion(ctx, org, pin.InstanceID, pin.Version, pin.StateVersion)
	if err != nil {
		return false
	}
	latest, err := s.deps.Baselines.LatestExecutionBaseline(ctx, org, pin.InstanceID)
	if err != nil {
		return false
	}
	if original.InstanceID != current.ID || latest.InstanceID != current.ID || original.EnvironmentID != current.Identity.EnvironmentID || latest.EnvironmentID != current.Identity.EnvironmentID || original.ModuleBundleID != latest.ModuleBundleID || latest.ModuleBundleID != current.ModuleBundleID || original.Lineage != latest.Lineage || latest.Lineage != state.Lineage || latest.InstanceVersion != current.Version || latest.StateVersion != state.Version || latest.Serial != state.Serial {
		return false
	}
	old, err := s.openExecutionBaseline(ctx, org, original)
	if err != nil {
		return false
	}
	new, err := s.openExecutionBaseline(ctx, org, latest)
	if err != nil {
		return false
	}
	if old.Request.Identity != current.Identity || new.Request.Identity != current.Identity {
		return false
	}
	return samePrivateJSON(old.Outputs, new.Outputs) && samePrivateJSON(old.Request.OutputMapping, new.Request.OutputMapping) && samePrivateJSON(old.Request.Type, new.Request.Type) && samePrivateJSON(old.Request.Pins, new.Request.Pins) && samePrivateJSON(old.Request.Targets, new.Request.Targets)
}
func (s *Service) openExecutionBaseline(ctx context.Context, org string, b terraformstate.ExecutionBaseline) (out baselinePayload, err error) {
	a, err := s.deps.Baselines.ReadBaselineArtifact(ctx, org, b.ArtifactID)
	if err != nil || a.ID != b.ArtifactID || a.OrganizationID != org || a.OwnerID != b.InstanceID || a.Purpose != "TERRAFORM_BASELINE" || a.AADVersion != 1 {
		return out, ErrBaselineUnavailable
	}
	raw, err := s.deps.Cipher.Open(a.KeyID, a.Nonce, a.Ciphertext, baselineAAD(a))
	if err != nil {
		return out, ErrBaselineUnavailable
	}
	if json.Unmarshal(raw, &out) != nil {
		return out, ErrBaselineUnavailable
	}
	return out, nil
}
func samePrivateJSON(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && string(left) == string(right)
}
