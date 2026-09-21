// Package provisioning implements UC-08: execute resource-only batches in
// provider-first order, validate outputs and persist Active Resource state.
package provisioning

import (
	"context"
	"fmt"
	"sort"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// Request is the input of one UC-08 run.
type Request struct {
	DeploymentID string
	RunID        string
	Context      planning.Context
	Plan         *planning.Plan
	Types        map[string]resource.Type
	Definitions  map[string]resource.Definition
}

// Result maps every executed descriptor to its outputs and resolved target.
type Result struct {
	Outputs map[string]map[string]any
	Targets map[string]execution.Target
}

// OutputsFor returns the outputs of one descriptor.
func (r Result) OutputsFor(descriptor string) map[string]any { return r.Outputs[descriptor] }

// Service executes the resource batches of a Deployment Plan.
type Service struct {
	store    persistence.Store
	registry execution.ExecutorRegistry
	secrets  execution.SecretStore
	clock    clock.Clock
}

// NewService wires UC-08 with its ports.
func NewService(store persistence.Store, registry execution.ExecutorRegistry, secrets execution.SecretStore, c clock.Clock) *Service {
	return &Service{store: store, registry: registry, secrets: secrets, clock: c}
}

// Provision runs every batch in order and returns the collected outputs (OC-10).
func (s *Service) Provision(ctx context.Context, req Request) (*Result, error) {
	if req.Plan == nil {
		return nil, fmt.Errorf("provisioning: plan is required")
	}
	result := &Result{Outputs: map[string]map[string]any{}, Targets: map[string]execution.Target{}}
	for batchIndex, batch := range req.Plan.Batches {
		for _, descriptor := range batch {
			if err := s.provisionNode(ctx, req, result, descriptor, batchIndex); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (s *Service) provisionNode(ctx context.Context, req Request, result *Result, descriptor string, batchIndex int) error {
	node, ok := req.Plan.Graph.Node(descriptor)
	if !ok {
		return fmt.Errorf("provisioning: node %q is not in the plan graph", descriptor)
	}
	match, ok := req.Plan.Matches[descriptor]
	if !ok {
		return fmt.Errorf("provisioning: node %q has no matched definition", descriptor)
	}
	def, ok := req.Definitions[match.DefinitionKey]
	if !ok {
		return fmt.Errorf("provisioning: definition %q is not registered", match.DefinitionKey)
	}
	typ, ok := req.Types[node.ResourceType]
	if !ok {
		return fmt.Errorf("provisioning: resource type %q is not registered", node.ResourceType)
	}
	parsed, err := resource.ParseDescriptor(descriptor)
	if err != nil {
		return err
	}

	resolver := runResolver{ctx: req.Context, node: node, bindings: node.Bindings, outputs: result.Outputs}
	variables, err := placeholder.ExpandTree(mapOrEmpty(def.Variables()), resolver)
	if err != nil {
		return fmt.Errorf("provisioning: resolve driver variables of %s: %w", descriptor, err)
	}
	params, err := placeholder.ExpandTree(mapOrEmpty(node.Params), resolver)
	if err != nil {
		return fmt.Errorf("provisioning: resolve params of %s: %w", descriptor, err)
	}
	inputMap := map[string]any{}
	if values, ok := variables.(map[string]any); ok {
		for k, v := range values {
			inputMap[k] = v
		}
	}
	// Resource params come from the Score document and win over driver defaults.
	if values, ok := params.(map[string]any); ok {
		for k, v := range values {
			inputMap[k] = v
		}
	}

	target := ResolveTarget(req.Plan.Graph, result, descriptor)

	prior, err := s.store.FindByLogicalIdentity(ctx, req.Context.OrganizationKey, parsed, node.Scope)
	priorState := map[string]any{}
	if err == nil {
		priorState = prior.ExecutorState
	}

	started := s.clock.Now()
	progress := deployment.Resource{
		DeploymentID:    req.DeploymentID,
		NodeDescriptor:  descriptor,
		DefinitionKey:   match.DefinitionKey,
		ResourceTypeKey: node.ResourceType,
		Status:          deployment.ResourceRunning,
		BatchIndex:      batchIndex,
		ResolvedInputs:  redactInputs(inputMap),
		StartedAt:       started,
	}
	if err := s.store.SaveDeploymentResource(ctx, progress); err != nil {
		return err
	}

	executor, err := s.registry.Resolve(match.DriverType)
	if err != nil {
		return err
	}
	connection, _ := s.store.GetConnection(ctx, connectionKeyFor(def, req.Context))

	execResult, err := executor.Provision(ctx, execution.ProvisionRequest{
		DeploymentID:    req.DeploymentID,
		RunID:           req.RunID,
		OrganizationKey: req.Context.OrganizationKey,
		ApplicationKey:  req.Context.App.Key,
		EnvironmentKey:  req.Context.Env.Key,
		Descriptor:      descriptor,
		ResourceType:    node.ResourceType,
		Class:           node.Class,
		Scope:           node.Scope,
		DefinitionKey:   match.DefinitionKey,
		DriverType:      match.DriverType,
		Module:          moduleName(def),
		Inputs:          mapOrEmpty(inputMap),
		PriorState:      priorState,
		Connection:      connection,
		Target:          target,
	})
	if err != nil {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return fmt.Errorf("provisioning: %s: %w", descriptor, err)
	}

	if err := typ.ValidateOutputs(execResult.Outputs); err != nil {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return err
	}

	storable, err := s.redactOutputs(ctx, typ, descriptor, execResult.Outputs)
	if err != nil {
		return err
	}

	fingerprint, err := canon.Hash(inputMap)
	if err != nil {
		return err
	}

	active := resource.ActiveResource{
		OrganizationKey:  req.Context.OrganizationKey,
		Descriptor:       parsed,
		Scope:            node.Scope,
		DefinitionKey:    match.DefinitionKey,
		ConnectionKey:    connection.Key,
		Status:           resource.StatusReady,
		ExecutorState:    execResult.State,
		Outputs:          storable,
		InputFingerprint: fingerprint,
		LastDeploymentID: req.DeploymentID,
	}

	finished := s.clock.Now()
	progress.Status = deployment.ResourceReady
	progress.OutputSnapshot = storable
	progress.FinishedAt = &finished

	// Short transaction per node: Active Resource upsert plus node progress (OC-10).
	if err := s.store.Transact(ctx, func(ctx context.Context) error {
		saved, err := s.store.UpsertActiveResource(ctx, active)
		if err != nil {
			return err
		}
		progress.ActiveResourceID = saved.ID
		return s.store.SaveDeploymentResource(ctx, progress)
	}); err != nil {
		return err
	}

	result.Outputs[descriptor] = execResult.Outputs
	if execResult.Target != nil {
		result.Targets[descriptor] = *execResult.Target
	}
	return nil
}

// redactOutputs replaces secret outputs with opaque Secret Store references so the
// state store never holds credential values.
func (s *Service) redactOutputs(ctx context.Context, typ resource.Type, descriptor string, outputs map[string]any) (map[string]any, error) {
	storable := map[string]any{}
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field, _ := typ.Output(name)
		if !field.Secret {
			storable[name] = outputs[name]
			continue
		}
		ref, err := s.secrets.Put(ctx, descriptor+"/"+name, placeholder.Stringify(outputs[name]))
		if err != nil {
			return nil, err
		}
		storable[name] = map[string]any{"secretRef": ref}
	}
	return storable, nil
}

func redactInputs(inputs map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range inputs {
		if isSecretInputKey(k) {
			out[k] = "***"
			continue
		}
		out[k] = v
	}
	return out
}

func isSecretInputKey(key string) bool {
	switch key {
	case "password", "secret", "token", "credentials", "masterPassword":
		return true
	}
	return false
}

func connectionKeyFor(def resource.Definition, ctx planning.Context) string {
	if def.ConnectionKey != "" {
		return def.ConnectionKey
	}
	return ctx.App.ConnectionKey
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func moduleName(def resource.Definition) string {
	name, _ := def.Source()["module"].(string)
	return name
}

// runResolver resolves placeholders against the outputs collected in this run.
type runResolver struct {
	ctx      planning.Context
	node     planning.Node
	bindings map[string]string
	outputs  map[string]map[string]any
}

// ResolveResource returns the output of a completed provider (UC-08 MS-03).
func (r runResolver) ResolveResource(binding, outputKey string) (any, error) {
	descriptor, ok := r.bindings[binding]
	if !ok {
		return nil, fmt.Errorf("provisioning: unknown resource binding %q", binding)
	}
	outputs, ok := r.outputs[descriptor]
	if !ok {
		return nil, fmt.Errorf("provisioning: provider %q has no outputs yet", descriptor)
	}
	value, ok := outputs[outputKey]
	if !ok {
		return nil, fmt.Errorf("provisioning: provider %q has no output %q", descriptor, outputKey)
	}
	return value, nil
}

// ResolveContext returns a non-secret context value. `res.*` describes the node
// currently being provisioned.
func (r runResolver) ResolveContext(path string) (any, error) {
	switch path {
	case "res.id":
		return descriptorID(r.node.Descriptor), nil
	case "res.class":
		return r.node.Class, nil
	case "res.type":
		return r.node.ResourceType, nil
	}
	return r.ctx.ResolveContext(path)
}

func descriptorID(descriptor string) string {
	d, err := resource.ParseDescriptor(descriptor)
	if err != nil {
		return ""
	}
	return d.ID
}

// ResolveTarget derives the Kubernetes target of a node from its provider outputs.
func ResolveTarget(graph planning.Graph, result *Result, descriptor string) execution.Target {
	target := execution.Target{Kind: "kubernetes"}
	visited := map[string]bool{}
	var walk func(string)
	walk = func(current string) {
		if visited[current] {
			return
		}
		visited[current] = true
		for _, provider := range graph.Providers(current) {
			node, ok := graph.Node(provider)
			if !ok {
				continue
			}
			outputs := result.Outputs[provider]
			switch node.ResourceType {
			case planning.TypeCluster:
				target.ClusterName = stringOutput(outputs, "name")
				target.Context = stringOutput(outputs, "kubeContext")
				target.Kubeconfig = stringOutput(outputs, "kubeconfig")
			case planning.TypeNamespace:
				if target.Namespace == "" {
					target.Namespace = stringOutput(outputs, "name")
				}
			}
			walk(provider)
		}
	}
	walk(descriptor)
	return target
}

func stringOutput(outputs map[string]any, key string) string {
	if outputs == nil {
		return ""
	}
	if v, ok := outputs[key].(string); ok {
		return v
	}
	return ""
}
