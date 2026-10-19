// Package provisioning implements UC-08: execute resource-only batches in
// provider-first order, validate outputs and persist Active Resource state.
package provisioning

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
)

var errConnectionUnavailable = errors.New("the Connection must exist and be READY")
var errPriorConnectionMismatch = errors.New("the existing resource belongs to another connection and cannot be reused")
var errAccountNotApplicable = errors.New("the Definition names a Driver Account its driver does not use")

// Request is the input of one UC-08 run.
type Request struct {
	DeploymentID string
	RunID        string
	Context      planning.Context
	Plan         *planning.Plan
	Types        map[string]resource.Type
	Definitions  map[string]resource.Definition
	// Select restricts execution to a resource-only subset (UC-06 owns the combined
	// lifecycle order: resources that depend on a workload run after it). Nil runs all.
	Select func(descriptor string) bool
	// Prior carries the outputs of an earlier subset so later subsets resolve against them.
	Prior *Result
}

// Result maps every executed descriptor to its outputs and resolved target.
type Result struct {
	Outputs map[string]map[string]any
	Targets map[string]execution.Target
	// Instances maps a descriptor to its durable Terraform instance ID.
	Instances map[string]string
	// Secrets marks, per descriptor, the output names whose EFFECTIVE
	// classification is secret: Resource Type, module mapping or a runtime
	// sensitive flag. Raw values stay in Outputs only for this run's
	// resolution; consumers are checked against this map before any input is
	// persisted or any consumer runs.
	Secrets map[string]map[string]bool
	// NodeClusters records, per Kubernetes-bound resource, the cluster target
	// its provider binding actually used (ADR-016); consumers compare it with
	// their own typed cluster reference before any call.
	NodeClusters map[string]execution.ClusterRef
}

// ErrSecretInput means a secret output would reach a nonsecret Terraform
// input (planning.ErrSecretInput at execution time).
var ErrSecretInput = planning.ErrSecretInput

// OutputsFor returns the outputs of one descriptor.
func (r Result) OutputsFor(descriptor string) map[string]any { return r.Outputs[descriptor] }

// Service executes the resource batches of a Deployment Plan.
type Service struct {
	store    persistence.Store
	registry execution.ExecutorRegistry
	secrets  execution.SecretStore
	clock    clock.Clock
	modules  moduleregistry.ExecutionVerifier
	// clusters proves a completed cluster target is usable (API auth/authorisation, network, Ready node
	// capacity) BEFORE any consumer runs (ADR-017). Nil disables the proof (fake adapters).
	clusters       execution.ClusterReadiness
	clusterTimeout time.Duration
}

// SetClusterReadiness wires the cluster usability proof run after a cluster node completes.
func (s *Service) SetClusterReadiness(checker execution.ClusterReadiness, timeout time.Duration) {
	s.clusters, s.clusterTimeout = checker, timeout
}

// SetModuleVerifier wires the module registry check that runs immediately
// before a Terraform module executes. Without it, an executor that runs
// module code is refused.
func (s *Service) SetModuleVerifier(modules moduleregistry.ExecutionVerifier) { s.modules = modules }

// NewService wires UC-08 with its ports.
func NewService(store persistence.Store, registry execution.ExecutorRegistry, secrets execution.SecretStore, c clock.Clock) *Service {
	return &Service{store: store, registry: registry, secrets: secrets, clock: c}
}

// Provision runs every batch in order and returns the collected outputs (OC-10).
func (s *Service) Provision(ctx context.Context, req Request) (*Result, error) {
	if req.Plan == nil {
		return nil, fmt.Errorf("provisioning: plan is required")
	}
	result := req.Prior
	if result == nil {
		result = &Result{Outputs: map[string]map[string]any{}, Targets: map[string]execution.Target{}, Instances: map[string]string{}, Secrets: map[string]map[string]bool{}, NodeClusters: map[string]execution.ClusterRef{}}
	}
	for batchIndex, batch := range req.Plan.Batches {
		for _, descriptor := range batch {
			if req.Select != nil && !req.Select(descriptor) {
				continue
			}
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

	isTerraform := def.DriverType == resource.DriverTerraform
	// Every variable and param is expanded on its own so a secret source is
	// attributed to the exact input it would feed. For Terraform consumers any
	// secret-derived value (Type, module or runtime classification, nested or
	// concatenated) is refused before inputs are persisted or a runner opens.
	expand := func(raw map[string]any, kind string) (map[string]any, error) {
		out := make(map[string]any, len(raw))
		for _, name := range sortedNames(raw) {
			tracker := &runResolver{ctx: req.Context, node: node, bindings: node.Bindings, outputs: result.Outputs, secrets: result.Secrets}
			value, err := placeholder.ExpandTree(raw[name], tracker)
			if err != nil {
				return nil, fmt.Errorf("provisioning: resolve driver %s of %s: %w", kind, descriptor, err)
			}
			if isTerraform && len(tracker.secretUsed) > 0 {
				return nil, fmt.Errorf("%w: %s %q of %s uses the secret output %s", ErrSecretInput, kind, name, descriptor, tracker.secretUsed[0])
			}
			out[name] = value
		}
		return out, nil
	}
	variables, err := expand(mapOrEmpty(def.Variables()), "variable")
	if err != nil {
		return err
	}
	nodeParams := node.Params
	if match.DriverType == resource.DriverExistingCluster && len(nodeParams) > 0 {
		return fmt.Errorf("provisioning: %s: an existing cluster accepts no params; its identity comes from the driver account", descriptor)
	}
	params, err := expand(mapOrEmpty(nodeParams), "param")
	if err != nil {
		return err
	}
	inputMap := map[string]any{}
	for k, v := range variables {
		inputMap[k] = v
	}
	var terraformInputs *tfmodule.DriverInputs
	if def.DriverType == resource.DriverTerraform {
		parsed, err := tfmodule.ParseDriverInputs(def.DriverValues())
		if err != nil {
			return fmt.Errorf("provisioning: %s: the Terraform definition is not a verified module pin: %w", descriptor, err)
		}
		terraformInputs = &parsed
	}
	// Resource params come from the Score document and win over driver
	// defaults. Terraform params reach module variables only through the
	// Definition input mapping.
	if values := params; values != nil {
		if terraformInputs != nil {
			for variable, param := range terraformInputs.InputMapping {
				if v, ok := values[param]; ok {
					inputMap[variable] = v
				}
			}
		} else {
			for k, v := range values {
				inputMap[k] = v
			}
		}
	}

	target := ResolveTarget(req.Plan.Graph, result, descriptor)

	// A Terraform v2 executor verifies the pinned module itself and resolves
	// the plan's provider pins; it never receives a pre-verified artifact.
	selfVerifying := false
	var pins []tfmodule.ProviderPin
	if def.DriverType == resource.DriverTerraform {
		if mv, ok := s.registry.(execution.ModuleVerifyingRegistry); ok && mv.VerifiesModules(match.DriverType) {
			selfVerifying = true
			pins = pinsOf(req.Plan, descriptor)
		}
	}

	prior, err := s.store.FindByLogicalIdentity(ctx, req.Context.OrganizationKey, parsed, node.Scope)
	priorState := map[string]any{}
	hasPrior := false
	switch {
	case err == nil:
		priorState, hasPrior = prior.ExecutorState, true
	case !errors.Is(err, persistence.ErrNotFound):
		return fmt.Errorf("provisioning: %s: load prior resource: %w", descriptor, err)
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

	if def.DriverType != resource.DriverTerraform && def.DriverType != resource.DriverExistingCluster && def.DriverAccount != "" {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return fmt.Errorf("provisioning: %s: %w", descriptor, errAccountNotApplicable)
	}

	// A missing, foreign or non-READY Connection fails before execution
	// instead of becoming an empty Connection (UC-04 BR-04/BR-10). An AWS node
	// runs with the Driver Account its winning Definition pinned; the
	// Environment account is never a substitute (UC-03 V02-09).
	connectionKey, wantKind := effectiveConnection(def, pinsOf(req.Plan, descriptor), selfVerifying, target)
	var connection appdomain.Connection
	err = errors.New("connection key is empty")
	if connectionKey != "" {
		connection, err = s.store.GetConnection(ctx, req.Context.OrganizationKey, connectionKey)
	}
	if err == nil && (connection.Status != appdomain.ConnectionReady || connection.OrganizationKey != req.Context.OrganizationKey) {
		err = fmt.Errorf("connection is not READY in this organization")
	}
	if err == nil && wantKind != "" && connection.Kind != wantKind {
		err = fmt.Errorf("connection kind does not match the node binding")
	}
	if err == nil && match.DriverType == resource.DriverExistingCluster && (connection.Key != match.DriverAccount || connection.Version != match.AccountVersion) {
		err = fmt.Errorf("the driver account changed after the plan; plan again")
	}
	if err != nil {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return fmt.Errorf("provisioning: %s: connection %q is unavailable: %w", descriptor, connectionKey, errConnectionUnavailable)
	}

	// UC-08 BR-07: prior state is reused only under the same effective
	// connection identity; another account/target never inherits it.
	if hasPrior && prior.ConnectionKey != connection.Key {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return fmt.Errorf("provisioning: %s: %w", descriptor, errPriorConnectionMismatch)
	}

	if selfVerifying {
		// Stale or foreign provider pins fail before any state write or
		// provider call.
		if pinErr := checkPinsAgainstPlan(req.Plan, descriptor, pins, def, connection, req.Context); pinErr != nil {
			progress.Status = deployment.ResourceFailed
			finished := s.clock.Now()
			progress.FinishedAt = &finished
			_ = s.store.SaveDeploymentResource(ctx, progress)
			return fmt.Errorf("provisioning: %s: %w", descriptor, pinErr)
		}
	}

	executor, err := s.registry.Resolve(match.DriverType)
	if err != nil {
		return err
	}
	var artifact *tfmodule.VerifiedArtifact
	if verified, ok := executor.(execution.VerifiedModuleExecutor); ok && verified.RequiresVerifiedModule() && !selfVerifying {
		// Re-verify the pinned bundle (content digests, release pins and
		// Terraform binary) immediately before execution; drift or a missing
		// artifact stops the node before any provider call.
		if terraformInputs == nil || s.modules == nil {
			err = errors.New("the Terraform module registry is unavailable")
		} else {
			var verifiedArtifact tfmodule.VerifiedArtifact
			verifiedArtifact, err = s.modules.VerifyForExecution(ctx, terraformInputs.Source)
			artifact = &verifiedArtifact
		}
		if err != nil {
			progress.Status = deployment.ResourceFailed
			finished := s.clock.Now()
			progress.FinishedAt = &finished
			_ = s.store.SaveDeploymentResource(ctx, progress)
			return fmt.Errorf("provisioning: %s: verify module artifact: %w", descriptor, err)
		}
	}

	execReq := execution.ProvisionRequest{
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
		Artifact:        artifact,
		Inputs:          mapOrEmpty(inputMap),
		Generation:      req.Context.Env.TargetGeneration,
		PriorState:      priorState,
		Connection:      connection,
		Target:          target,
	}
	if selfVerifying && terraformInputs != nil {
		identity, identityErr := s.identityOf(ctx, req, parsed, node)
		if identityErr != nil {
			progress.Status = deployment.ResourceFailed
			finished := s.clock.Now()
			progress.FinishedAt = &finished
			_ = s.store.SaveDeploymentResource(ctx, progress)
			return fmt.Errorf("provisioning: %s: %w", descriptor, identityErr)
		}
		source := terraformInputs.Source
		execReq.Source, execReq.Identity, execReq.OutputMapping, execReq.Type = &source, identity, terraformInputs.OutputMapping, moduleOutputContract(typ, source.ModuleID)
		execReq.ProviderPins = pins
		execReq.ProviderTargets = providerTargets(pins, result)
		execReq.ProviderInstanceIDs = providerInstances(req.Plan.Graph, result, descriptor)
	}
	execResult, err := executor.Provision(ctx, execReq)
	if err != nil {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return fmt.Errorf("provisioning: %s: %w", descriptor, err)
	}

	secretOutputs := map[string]bool{}
	if terraformInputs != nil {
		// Module outputs reach the Resource Type only through the output
		// mapping; unmapped internal outputs are never published. A module
		// sensitive output stays secret whatever the Type says.
		mapped := map[string]any{}
		for output, target := range terraformInputs.OutputMapping {
			if v, ok := execResult.Outputs[output]; ok {
				mapped[target] = v
			}
		}
		if artifact != nil {
			for output, target := range terraformInputs.OutputMapping {
				if artifact.Bundle.Contract.Outputs[output].Sensitive {
					secretOutputs[target] = true
				}
			}
		}
		// A runtime sensitive flag reported by the executor is never lost.
		for output, target := range terraformInputs.OutputMapping {
			if execResult.Sensitive[output] {
				secretOutputs[target] = true
			}
		}
		execResult.Outputs = mapped
	}

	if def.DriverType == resource.DriverExistingCluster {
		// Only the safe outputs the Resource Type declares are published; the
		// typed `cluster` target is mandatory (ADR-016).
		kept := map[string]any{}
		for name, value := range execResult.Outputs {
			if _, declared := typ.Output(name); declared {
				kept[name] = value
			}
		}
		execResult.Outputs = kept
		if _, ok := kept[resource.ClusterTargetOutput]; !ok {
			progress.Status = deployment.ResourceFailed
			finished := s.clock.Now()
			progress.FinishedAt = &finished
			_ = s.store.SaveDeploymentResource(ctx, progress)
			return fmt.Errorf("provisioning: %s: the cluster resource type does not declare the %q output", descriptor, resource.ClusterTargetOutput)
		}
	}
	if def.DriverType == resource.DriverTerraform && node.ResourceType == planning.TypeCluster && execResult.Target != nil {
		// ADR-017: the platform, never the module, publishes the typed target of a cluster it created.
		if _, declared := typ.Output(resource.ClusterTargetOutput); declared {
			if err := completeEKSTarget(descriptor, pinsOf(req.Plan, descriptor), &execResult); err != nil {
				progress.Status = deployment.ResourceFailed
				finished := s.clock.Now()
				progress.FinishedAt = &finished
				_ = s.store.SaveDeploymentResource(ctx, progress)
				return fmt.Errorf("provisioning: %s: %w", descriptor, err)
			}
		}
	}
	if err := typ.ValidateOutputs(execResult.Outputs); err != nil {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return err
	}

	if node.ResourceType == planning.TypeCluster && execResult.Target != nil && s.clusters != nil {
		// ACTIVE is not enough: authenticated + authorised API access, executor network and usable Ready
		// node capacity must hold before the namespace, database, cache or workload runs.
		timeout := s.clusterTimeout
		if timeout <= 0 {
			timeout = 15 * time.Minute
		}
		waitCtx, cancel := context.WithTimeout(ctx, timeout)
		err := s.clusters.WaitClusterReady(waitCtx, *execResult.Target)
		cancel()
		if err != nil {
			progress.Status = deployment.ResourceFailed
			finished := s.clock.Now()
			progress.FinishedAt = &finished
			_ = s.store.SaveDeploymentResource(ctx, progress)
			return fmt.Errorf("provisioning: %s: %w", descriptor, publicClusterError(err))
		}
	}

	storable, err := s.redactOutputs(ctx, typ, descriptor, execResult.Outputs, secretOutputs)
	if err != nil {
		return err
	}
	effective := map[string]bool{}
	for name := range execResult.Outputs {
		field, _ := typ.Output(name)
		if tfmodule.EffectiveSecret(field.Secret, secretOutputs[name]) {
			effective[name] = true
		}
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

	for _, pin := range pinsOf(req.Plan, descriptor) {
		if pin.Kind != tfmodule.BindingKubernetes || pin.Provider == "" {
			continue
		}
		if provider, ok := result.Targets[pin.Provider]; ok {
			result.NodeClusters[descriptor] = execution.ClusterRef{Target: provider, Descriptor: pin.Provider, AccountVersion: pin.AccountVersion}
		}
	}
	result.Outputs[descriptor] = execResult.Outputs
	if result.Secrets == nil {
		result.Secrets = map[string]map[string]bool{}
	}
	result.Secrets[descriptor] = effective
	if execResult.InstanceID != "" {
		result.Instances[descriptor] = execResult.InstanceID
	}
	if execResult.Target != nil {
		result.Targets[descriptor] = *execResult.Target
	}
	return nil
}

// redactOutputs replaces secret outputs with opaque Secret Store references so the
// state store never holds credential values.
func (s *Service) redactOutputs(ctx context.Context, typ resource.Type, descriptor string, outputs map[string]any, secret map[string]bool) (map[string]any, error) {
	storable := map[string]any{}
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field, _ := typ.Output(name)
		if !tfmodule.EffectiveSecret(field.Secret, secret[name]) {
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

// effectiveConnection returns the Connection identity and kind a node executes with (ADR-016, ADR-017). A self-verifying Terraform
// node uses the account of its AWS provider pin; a Kubernetes-bound node inherits its cluster: the Internal account of an existing
// cluster, or the original AWS account of a cluster the plan itself created. No Environment connection, profile or account is ever a
// fallback: a node without any of them yields no account and fails as unavailable.
func effectiveConnection(def resource.Definition, pins []tfmodule.ProviderPin, selfVerifying bool, target execution.Target) (string, appdomain.ConnectionKind) {
	switch def.DriverType {
	case resource.DriverExistingCluster:
		return def.DriverAccount, appdomain.ConnectionKubernetes
	case resource.DriverTerraform:
		for _, pin := range pins {
			if pin.Kind == tfmodule.BindingKubernetes && pin.ConnectionKey != "" {
				return pin.ConnectionKey, appdomain.ConnectionKubernetes
			}
		}
		if target.Provider == "eks" && target.Connection != "" {
			// A node bound to a cluster the plan itself created runs with the original AWS account of that cluster.
			for _, pin := range pins {
				if pin.Kind == tfmodule.BindingKubernetes && pin.ClusterProvider == planning.ClusterEKS {
					return target.Connection, appdomain.ConnectionAWS
				}
			}
		}
		if !selfVerifying {
			return def.DriverAccount, appdomain.ConnectionAWS
		}
		for _, pin := range pins {
			if pin.Kind == tfmodule.BindingAWS {
				return pin.ConnectionKey, appdomain.ConnectionAWS
			}
		}
		if target.Provider == "eks" && target.Connection != "" {
			return target.Connection, appdomain.ConnectionAWS
		}
		return "", ""
	}
	// A non-Terraform Kubernetes driver runs with the account of the cluster its references resolved to.
	return target.Connection, appdomain.ConnectionKubernetes
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// moduleOutputContract keeps platform-completed EKS identity out of the earlier
// module-output check. The unchanged full Type is validated after completeEKSTarget.
func moduleOutputContract(typ resource.Type, module string) resource.Type {
	if typ.Key != resource.ClusterTargetType || module != "eks" {
		return typ
	}
	outputs := make([]resource.OutputField, 0, len(typ.Outputs))
	for _, field := range typ.Outputs {
		if field.Name != resource.ClusterTargetOutput {
			outputs = append(outputs, field)
		}
	}
	typ.Outputs = outputs
	return typ
}

func moduleName(def resource.Definition) string {
	name, _ := def.Source()["module_id"].(string)
	return name
}

// runResolver resolves placeholders against the outputs collected in this run.
type runResolver struct {
	ctx      planning.Context
	node     planning.Node
	bindings map[string]string
	outputs  map[string]map[string]any
	secrets  map[string]map[string]bool
	// secretUsed lists "descriptor.output" names of secret outputs resolved.
	secretUsed []string
}

func sortedNames(m map[string]any) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveResource returns the output of a completed provider (UC-08 MS-03).
func (r *runResolver) ResolveResource(binding, outputKey string) (any, error) {
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
	if r.secrets[descriptor][outputKey] {
		r.secretUsed = append(r.secretUsed, descriptor+"."+outputKey)
	}
	return value, nil
}

// ResolveContext returns a non-secret context value. `res.*` describes the node
// currently being provisioned.
func (r *runResolver) ResolveContext(path string) (any, error) {
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
				// Credential-backed clusters carry only their opaque
				// Organization/Connection identity, never credential bytes.
				if resolved, ok := result.Targets[provider]; ok && resolved.CredentialBacked() {
					target.Organization, target.Connection = resolved.Organization, resolved.Connection
					target.Provider, target.Region = resolved.Provider, resolved.Region
					if resolved.Provider != "" {
						target.ClusterName = resolved.ClusterName
						target.Context = ""
					}
					target.Kubeconfig = ""
				}
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

// pinsOf returns the plan-pinned provider bindings of one Terraform node.
func pinsOf(plan *planning.Plan, descriptor string) []tfmodule.ProviderPin {
	for _, contract := range plan.Terraform {
		if contract.Descriptor == descriptor {
			return append([]tfmodule.ProviderPin(nil), contract.ProviderBindings...)
		}
	}
	return nil
}

// checkPinsAgainstPlan refuses a node whose pinned bindings no longer match
// the Environment it executes in: a pin for another connection, region or
// profile (stale), or a provider cluster without its graph edge (foreign).
// This runs before any state write or provider call.
func checkPinsAgainstPlan(plan *planning.Plan, descriptor string, pins []tfmodule.ProviderPin, def resource.Definition, account appdomain.Connection, ctx planning.Context) error {
	env := ctx.Env
	providers := map[string]bool{}
	for _, p := range plan.Graph.Providers(descriptor) {
		providers[p] = true
	}
	for _, pin := range pins {
		if pin.Profile != string(env.Profile) || pin.Generation != env.TargetGeneration {
			return errPinStale
		}
		switch pin.Kind {
		case tfmodule.BindingAWS:
			// The pin must still describe the Definition's own Driver Account,
			// literal region and the account version the plan saw.
			region, _ := def.DefinitionRegion()
			if pin.ConnectionKey == "" || pin.ConnectionKey != def.DriverAccount || pin.ConnectionKey != account.Key ||
				pin.Region == "" || pin.Region != region || pin.AccountVersion != account.Version {
				return errPinStale
			}
		case tfmodule.BindingKubernetes:
			if pin.Provider == "" || !providers[pin.Provider] {
				return errPinForeign
			}
			if pin.ClusterProvider == planning.ClusterEKS {
				// A cluster the plan itself creates (ADR-017): no Kubernetes Connection exists. The node
				// runs with the original AWS account of the producing cluster, at the version the plan saw.
				if pin.ConnectionKey != "" || pin.ClusterAccount == "" || pin.ClusterAccount != account.Key || pin.AccountVersion != account.Version || account.Kind != appdomain.ConnectionAWS || pin.Region == "" {
					return errPinStale
				}
			} else if pin.ConnectionKey == "" || pin.ConnectionKey != account.Key || pin.AccountVersion != account.Version {
				// The pin must describe the READY Internal account this node runs with, at the version the plan saw.
				return errPinStale
			}
		}
	}
	return nil
}

var (
	// ErrProviderPin is returned for a provider binding pin that is stale
	// (another connection, region, profile or generation) or foreign (a
	// cluster that is not a provider of the node).
	ErrProviderPin = errors.New("the provider binding does not match the plan pin")
	errPinStale    = fmt.Errorf("%w: it differs from the Environment target; plan again", ErrProviderPin)
	errPinForeign  = fmt.Errorf("%w: it names a cluster that is not a provider of this node", ErrProviderPin)
)

// providerTargets returns the completed opaque target of each cluster node a
// pin names; a cluster that produced none is absent and fails the binding.
func providerTargets(pins []tfmodule.ProviderPin, result *Result) map[string]execution.Target {
	out := map[string]execution.Target{}
	for _, pin := range pins {
		if pin.Provider == "" {
			continue
		}
		if target, ok := result.Targets[pin.Provider]; ok {
			out[pin.Provider] = target
		}
	}
	return out
}

// providerInstances lists the Terraform instances of this node's providers.
func providerInstances(graph planning.Graph, result *Result, descriptor string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, provider := range graph.Providers(descriptor) {
		if id := result.Instances[provider]; id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// identityOf builds the canonical instance identity of a node: the stable
// Organization, Application and Environment IDs, target generation, descriptor
// and scope. It never contains a run ID.
func (s *Service) identityOf(ctx context.Context, req Request, descriptor resource.Descriptor, node planning.Node) (tfresource.Identity, error) {
	org, err := s.store.GetOrganization(ctx, req.Context.OrganizationKey)
	if err != nil {
		return tfresource.Identity{}, fmt.Errorf("the Organization is unavailable")
	}
	identity := tfresource.Identity{
		OrganizationID: org.ID, ApplicationID: req.Context.Env.ApplicationID, EnvironmentID: req.Context.Env.ID,
		TargetGeneration: req.Context.Env.TargetGeneration, Descriptor: descriptor.String(), Scope: node.Scope,
	}
	if err := identity.Validate(); err != nil {
		return tfresource.Identity{}, fmt.Errorf("the instance identity is incomplete")
	}
	return identity, nil
}

// ErrWorkloadTarget reports a workload whose typed cluster/namespace
// references do not resolve to one agreeing, account-pinned target.
var ErrWorkloadTarget = errors.New("provisioning: the workload target does not match its pinned placement")

// ResolveWorkloadTarget builds the execution target of a workload from its
// own typed namespace and cluster references (ADR-016), never from the last
// cluster in the graph. The cluster output must equal the plan's pinned
// placement and the namespace must have been created on that same cluster.
func ResolveWorkloadTarget(plan *planning.Plan, result *Result, workload, organizationKey string) (execution.Target, error) {
	placement, ok := plan.Placements[workload]
	if !ok || placement.Namespace == "" {
		return execution.Target{}, fmt.Errorf("%w: no pinned placement", ErrWorkloadTarget)
	}
	raw, ok := result.Outputs[placement.Cluster]["cluster"]
	if !ok {
		return execution.Target{}, fmt.Errorf("%w: the cluster produced no target", ErrWorkloadTarget)
	}
	ref, err := execution.ParseClusterOutput(raw)
	if err != nil {
		return execution.Target{}, fmt.Errorf("%w: %v", ErrWorkloadTarget, err)
	}
	wantProvider := ""
	if placement.Provider == planning.ClusterEKS {
		wantProvider = resource.K8sTargetProviderEKS
	}
	if ref.Target.Provider != wantProvider || ref.Target.Region != placement.Region {
		return execution.Target{}, fmt.Errorf("%w: the cluster provider or region differs from the plan pin", ErrWorkloadTarget)
	}
	if ref.Descriptor != placement.Cluster || ref.Target.Connection != placement.Account || ref.AccountVersion != placement.AccountVersion || ref.Target.Organization != organizationKey {
		return execution.Target{}, fmt.Errorf("%w: the resolved cluster differs from the plan pin", ErrWorkloadTarget)
	}
	nsCluster, ok := result.NodeClusters[placement.Namespace]
	if !ok || nsCluster.Descriptor != placement.Cluster || nsCluster.Target.Connection != ref.Target.Connection ||
		nsCluster.Target.ClusterName != ref.Target.ClusterName || nsCluster.Target.Organization != ref.Target.Organization ||
		nsCluster.Target.Provider != ref.Target.Provider || nsCluster.Target.Region != ref.Target.Region {
		return execution.Target{}, fmt.Errorf("%w: the namespace was not created on the workload cluster", ErrWorkloadTarget)
	}
	namespace, _ := result.Outputs[placement.Namespace]["name"].(string)
	if namespace == "" {
		return execution.Target{}, fmt.Errorf("%w: the namespace resolved no name", ErrWorkloadTarget)
	}
	target := ref.Target
	target.Namespace = namespace
	return target, nil
}

// ErrClusterTarget reports a completed cluster whose identity does not match the plan pin.
var ErrClusterTarget = errors.New("the created cluster does not match the planned account, region or name")

// completeEKSTarget publishes the typed `cluster` output of a cluster created by a verified EKS module from
// the completed opaque target, after proving it still equals the plan's AWS pin (original account and literal
// region). It carries no endpoint, token, kubeconfig or path.
func completeEKSTarget(descriptor string, pins []tfmodule.ProviderPin, result *execution.ProvisionResult) error {
	target := *result.Target
	if target.Provider != resource.K8sTargetProviderEKS {
		return nil // an existing cluster publishes its own target
	}
	var aws *tfmodule.ProviderPin
	for i := range pins {
		if pins[i].Kind == tfmodule.BindingAWS {
			aws = &pins[i]
		}
	}
	if aws == nil || target.Connection != aws.ConnectionKey || target.Region != aws.Region || target.ClusterName == "" || aws.AccountVersion < 1 {
		return ErrClusterTarget
	}
	if result.Outputs == nil {
		result.Outputs = map[string]any{}
	}
	result.Outputs[resource.ClusterTargetOutput] = target.ClusterOutput(descriptor, aws.AccountVersion)
	return nil
}

// ErrClusterNotUsable is the fixed public failure of a cluster that exists but cannot serve consumers.
var ErrClusterNotUsable = errors.New("the cluster is not usable yet: API authentication, authorisation, network access or Ready node capacity failed")

// publicClusterError keeps the fixed category and drops everything else.
func publicClusterError(err error) error {
	for _, category := range []error{execution.ErrClusterUnreachable, execution.ErrClusterAuth, execution.ErrClusterForbidden, execution.ErrClusterNoCapacity, execution.ErrClusterNotReady} {
		if errors.Is(err, category) {
			return fmt.Errorf("%w: %w", ErrClusterNotUsable, category)
		}
	}
	return ErrClusterNotUsable
}
