package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	pgstore "orchestrator/internal/adapters/postgres"
	"orchestrator/internal/application/runcleanup"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/tfresource"
)

// selectFailedInstances narrows an already-owned private operator run; it never
// authorizes from a descriptor alone or changes the public user cleanup API.
func selectFailedInstances(ctx context.Context, app *bootstrap.App, target runcleanup.Target, stateURL, file string) ([]string, error) {
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return nil, errors.New("private descriptor handle required")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var names []string
	if json.Unmarshal(raw, &names) != nil || len(names) == 0 {
		return nil, errors.New("empty selection")
	}
	env, err := app.Store.GetEnvironment(ctx, target.ApplicationKey, target.EnvironmentKey)
	if err != nil || env.TargetGeneration != target.Generation {
		return nil, errors.New("environment differs")
	}
	state, err := pgstore.OpenState(ctx, stateURL)
	if err != nil {
		return nil, err
	}
	defer state.Close()
	instances, err := state.ListInstances(ctx, env.ID)
	if err != nil {
		return nil, err
	}
	owned := map[string]bool{}
	for _, run := range target.RunIDs {
		owned[run] = true
	}
	preApply := map[string]bool{}
	for _, inst := range instances {
		if inst.Status != tfresource.InstancePending {
			continue
		}
		attempts, err := state.ListAttempts(ctx, inst.ID)
		if err != nil {
			return nil, err
		}
		preApply[inst.ID] = provenPreApplyFailure(inst, attempts, owned)
	}
	return selectFailedIDs(instances, names, owned, target.Generation, preApply)
}
func selectFailedIDs(instances []tfresource.Instance, names []string, owned map[string]bool, generation int64, preApply ...map[string]bool) ([]string, error) {
	selected := map[string]bool{}
	ids := []string{}
	for _, name := range names {
		if name == "" || selected[name] {
			return nil, errors.New("duplicate or empty descriptor")
		}
		selected[name] = true
		found := ""
		for _, inst := range instances {
			if inst.Identity.Descriptor != name {
				continue
			}
			if found != "" || inst.Identity.TargetGeneration != generation || (inst.Status != tfresource.InstanceFailed && !(inst.Status == tfresource.InstancePending && len(preApply) == 1 && preApply[0][inst.ID])) || !owned[inst.CreatedByRunID] {
				return nil, errors.New("descriptor ownership/state differs")
			}
			found = inst.ID
		}
		if found == "" {
			return nil, errors.New("descriptor missing")
		}
		ids = append(ids, found)
	}
	if len(ids) == 0 {
		return nil, errors.New("empty selection")
	}
	return ids, nil
}

// A pre-apply rejection preserves PENDING; the failed attempt is the proof.
// This metadata admission never substitutes for the engine's state/history gate.
func provenPreApplyFailure(inst tfresource.Instance, attempts []tfresource.Attempt, owned map[string]bool) bool {
	if inst.Status != tfresource.InstancePending || !owned[inst.CreatedByRunID] || len(attempts) == 0 {
		return false
	}
	for _, a := range attempts {
		if a.InstanceID != inst.ID || !owned[a.RunID] || a.Action != tfresource.ActionCreate || a.Ownership != tfresource.CreatedByRun || a.Status != tfresource.AttemptFailed || a.Stage != "PLANNING" || len(a.ObservedPhysicalIDs) != 0 || a.StateSerial != nil || a.RecoveryHandle != "" {
			return false
		}
	}
	return true
}
