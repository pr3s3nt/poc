package provisioning

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// RepairBindings resolves only the immutable plan which originally supplied
// this instance's acknowledged inputs. Current catalog edits and drafts are
// intentionally absent from this path.
type RepairBindings struct {
	Provisioning *Service
	Executor     *tfexec.Service
}

func (b *RepairBindings) ResolveRepairInputs(ctx context.Context, original tfexec.ExecuteRequest) (tfexec.ExecuteRequest, error) {
	s := b.Provisioning
	raw, err := s.store.GetPlan(ctx, original.DeploymentID)
	if err != nil {
		return original, tfexec.ErrBaselineUnavailable
	}
	var plan planning.Plan
	encoded, _ := json.Marshal(raw)
	if json.Unmarshal(encoded, &plan) != nil {
		return original, tfexec.ErrBaselineUnavailable
	}
	node, found := plan.Graph.Node(original.Descriptor)
	if !found || node.Scope != original.Scope {
		return original, tfexec.ErrBaselineUnavailable
	}
	match, found := plan.Matches[original.Descriptor]
	if !found {
		return original, tfexec.ErrBaselineUnavailable
	}
	var definition resource.Definition
	for _, d := range plan.DefinitionSnapshots {
		if d.Key == match.DefinitionKey {
			definition = d
			break
		}
	}
	if definition.DriverType != resource.DriverTerraform {
		return original, tfexec.ErrBaselineUnavailable
	}
	driver, err := parseRepairDriver(definition, original)
	if err != nil {
		return original, err
	}
	app, err := s.store.GetApplication(ctx, original.Identity.ApplicationID)
	if err != nil { // Application repository is keyed by public key, not UUID.
		apps, listErr := s.store.ListApplications(ctx)
		if listErr != nil {
			return original, tfexec.ErrBaselineUnavailable
		}
		for _, candidate := range apps {
			if candidate.ID == original.Identity.ApplicationID {
				app = candidate
				err = nil
				break
			}
		}
	}
	if err != nil {
		return original, tfexec.ErrBaselineUnavailable
	}
	var envKey string
	for _, key := range []string{"staging", "production"} {
		env, e := s.store.GetEnvironment(ctx, app.Key, key)
		if e == nil && env.ID == original.Identity.EnvironmentID {
			envKey = key
			break
		}
	}
	env, err := s.store.GetEnvironment(ctx, app.Key, envKey)
	if err != nil || app.OrganizationKey != original.OrganizationKey || env.TargetGeneration != original.Identity.TargetGeneration {
		return original, tfexec.ErrBaselineUnavailable
	}
	result, err := b.LoadResult(ctx, original.OrganizationKey, &plan, original.Identity.EnvironmentID)
	if err != nil {
		return original, err
	}
	pc := planning.Context{OrganizationKey: original.OrganizationKey, App: app, Env: env, RunID: original.RunID}
	expand := func(raw map[string]any) (map[string]any, error) {
		out := map[string]any{}
		for _, key := range sortedNames(raw) {
			resolver := &runResolver{ctx: pc, node: node, bindings: node.Bindings, outputs: result.Outputs, secrets: result.Secrets}
			value, err := placeholder.ExpandTree(raw[key], resolver)
			if err != nil || len(resolver.secretUsed) > 0 {
				return nil, tfexec.ErrBaselineUnavailable
			}
			out[key] = value
		}
		return out, nil
	}
	inputs, err := expand(driver.Variables)
	if err != nil {
		return original, err
	}
	params, err := expand(node.Params)
	if err != nil {
		return original, err
	}
	for variable, param := range driver.InputMapping {
		if v, exists := params[param]; exists {
			inputs[variable] = v
		}
	}
	original.Inputs = inputs
	original.Targets = providerTargets(original.Pins, result)
	return original, nil
}

func parseRepairDriver(def resource.Definition, original tfexec.ExecuteRequest) (tfmodule.DriverInputs, error) {
	driver, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil || driver.Source != original.Source || !sameJSON(driver.OutputMapping, original.OutputMapping) {
		return driver, tfexec.ErrBaselineUnavailable
	}
	return driver, nil
}
func sameJSON(a, b any) bool {
	x, e := json.Marshal(a)
	y, f := json.Marshal(b)
	return e == nil && f == nil && string(x) == string(y)
}

// LoadResult restores secret values privately and typed provider targets. It
// never provisions resources or writes a secret during Preview.
func (b *RepairBindings) LoadResult(ctx context.Context, org string, plan *planning.Plan, environmentID ...string) (*Result, error) {
	result := &Result{Outputs: map[string]map[string]any{}, Targets: map[string]execution.Target{}, Instances: map[string]string{}, Secrets: map[string]map[string]bool{}, NodeClusters: map[string]execution.ClusterRef{}}
	for _, node := range plan.Graph.Nodes {
		if node.Kind == planning.NodeWorkload {
			continue
		}
		desc, err := resource.ParseDescriptor(node.Descriptor)
		if err != nil {
			return nil, tfexec.ErrBaselineUnavailable
		}
		active, err := b.Provisioning.store.FindByLogicalIdentity(ctx, org, desc, node.Scope)
		if errors.Is(err, persistence.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result.Outputs[node.Descriptor] = map[string]any{}
		result.Secrets[node.Descriptor] = map[string]bool{}
		if id, ok := active.ExecutorState["instanceId"].(string); ok && id != "" {
			req, snapshot, err := b.Executor.AcknowledgedRepairResult(ctx, id)
			if err != nil || req.OrganizationKey != org || req.Descriptor != node.Descriptor || req.Scope != node.Scope || (len(environmentID) > 0 && req.Identity.EnvironmentID != environmentID[0]) {
				return nil, tfexec.ErrBaselineUnavailable
			}
			for output, value := range snapshot.Outputs {
				name := req.OutputMapping[output]
				if name != "" {
					result.Outputs[node.Descriptor][name] = value
					result.Secrets[node.Descriptor][name] = snapshot.Sensitive[output]
				}
			}
			result.Instances[node.Descriptor] = id
			if snapshot.Target != nil {
				result.Targets[node.Descriptor] = *snapshot.Target
			}
		} else {
			for key, value := range active.Outputs {
				if object, ok := value.(map[string]any); ok {
					if ref, ok := object["secretRef"].(string); ok {
						secret, found := b.Provisioning.secrets.Get(ctx, ref)
						if !found {
							return nil, tfexec.ErrBaselineUnavailable
						}
						value = secret
						result.Secrets[node.Descriptor][key] = true
					}
				}
				result.Outputs[node.Descriptor][key] = value
			}
		}
		if value, exists := result.Outputs[node.Descriptor][resource.ClusterTargetOutput]; exists {
			cluster, err := execution.ParseClusterOutput(value)
			if err != nil {
				return nil, tfexec.ErrBaselineUnavailable
			}
			result.Targets[node.Descriptor] = cluster.Target
		}
	}
	for _, node := range plan.Graph.Nodes {
		for _, pin := range pinsOf(plan, node.Descriptor) {
			if pin.Kind == tfmodule.BindingKubernetes && pin.Provider != "" {
				if target, exists := result.Targets[pin.Provider]; exists {
					result.NodeClusters[node.Descriptor] = execution.ClusterRef{Target: target, Descriptor: pin.Provider, AccountVersion: pin.AccountVersion}
				}
			}
		}
	}
	return result, nil
}

func (b *RepairBindings) PublishRepairOutputs(ctx context.Context, req tfexec.ExecuteRequest, result tfexec.Result) error {
	desc, err := resource.ParseDescriptor(req.Descriptor)
	if err != nil {
		return err
	}
	active, err := b.Provisioning.store.FindByLogicalIdentity(ctx, req.OrganizationKey, desc, req.Scope)
	if err != nil {
		return err
	}
	if active.ExecutorState["instanceId"] != result.InstanceID || active.LastDeploymentID != req.DeploymentID {
		return persistence.ErrVersionConflict
	}
	mapped := map[string]any{}
	sensitive := map[string]bool{}
	for name, target := range req.OutputMapping {
		if value, exists := result.Outputs[name]; exists {
			mapped[target] = value
			sensitive[target] = result.Sensitive[name]
		}
	}
	active.Outputs, err = b.Provisioning.redactOutputs(ctx, req.Type, req.Descriptor, mapped, sensitive)
	if err != nil {
		return err
	}
	active.InputFingerprint, err = canon.Hash(req.Inputs)
	if err != nil {
		return err
	}
	// LastDeploymentID remains the source of the original applied configuration.
	_, err = b.Provisioning.store.UpsertActiveResource(ctx, active)
	return err
}
