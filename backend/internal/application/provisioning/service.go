// Package provisioning implements UC-08: execute resource-only batches in
// provider-first order, validate outputs and persist Active Resource state.
package provisioning

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
var errConnectionMismatch = errors.New("the Definition connection differs from the Environment connection")

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
	// Instances maps a descriptor to its durable Terraform instance ID.
	Instances map[string]string
	// Secrets marks, per descriptor, the output names whose EFFECTIVE
	// classification is secret: Resource Type, module mapping or a runtime
	// sensitive flag. Raw values stay in Outputs only for this run's
	// resolution; consumers are checked against this map before any input is
	// persisted or any consumer runs.
	Secrets map[string]map[string]bool
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
	result := &Result{Outputs: map[string]map[string]any{}, Targets: map[string]execution.Target{}, Instances: map[string]string{}, Secrets: map[string]map[string]bool{}}
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
	if err := planning.ValidateBuiltinMatch(req.Context, node, match); err != nil {
		return err
	}
	def, ok := req.Definitions[match.DefinitionKey]
	if match.DefinitionKey == planning.BuiltinClusterKey {
		// ADR-013: the effective definition is trusted code, never a stored copy.
		def, ok = planning.BuiltinClusterDefinition(), true
		if err := EnsureBuiltinCluster(ctx, s.store, req.Context.OrganizationKey); err != nil {
			return err
		}
	}
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
	if match.DefinitionKey == planning.BuiltinClusterKey {
		nodeParams = nil // the Environment Connection is the only input source
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

	// Defense in depth for UC-08 BR-07: a target Definition (internal cluster,
	// Kubernetes, or aws-eks Terraform VPC/EKS) must not retarget the Environment's saved Connection.
	if planning.ConnectionMismatch(req.Context.Env, def) {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return fmt.Errorf("provisioning: %s: %w", descriptor, errConnectionMismatch)
	}

	// A missing or non-READY Connection fails before execution instead of
	// becoming an empty Connection (UC-04 BR-04/BR-10).
	connectionKey := connectionKeyFor(def, req.Context)
	if selfVerifying {
		// Every v2 provider binding targets the Environment Connection (AWS
		// account/region, internal cluster) or a cluster this plan creates.
		connectionKey = req.Context.Env.ConnectionKey
	}
	var connection appdomain.Connection
	err = errors.New("connection key is empty")
	if connectionKey != "" {
		connection, err = s.store.GetConnection(ctx, req.Context.OrganizationKey, connectionKey)
	}
	if err == nil && (connection.Status != appdomain.ConnectionReady || connection.OrganizationKey != req.Context.OrganizationKey) {
		err = fmt.Errorf("connection is not READY in this organization")
	}
	if err == nil && match.DefinitionKey == planning.BuiltinClusterKey &&
		(connection.Kind != appdomain.ConnectionKubernetes || connection.Key != req.Context.Env.ConnectionKey) {
		err = fmt.Errorf("connection is not the Kubernetes Environment connection")
	}
	if err == nil && def.DriverType == resource.DriverTerraform && !selfVerifying && connection.Kind != appdomain.ConnectionAWS {
		err = fmt.Errorf("connection is not an AWS connection")
	}
	if err == nil && selfVerifying && connection.Kind != expectedConnectionKind(req.Context.Env.Profile) {
		err = fmt.Errorf("connection does not match the Environment execution profile")
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
		if pinErr := checkPinsAgainstPlan(req.Plan, descriptor, pins, req.Context); pinErr != nil {
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
		execReq.Source, execReq.Identity, execReq.OutputMapping, execReq.Type = &source, identity, terraformInputs.OutputMapping, typ
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

	if err := typ.ValidateOutputs(execResult.Outputs); err != nil {
		progress.Status = deployment.ResourceFailed
		finished := s.clock.Now()
		progress.FinishedAt = &finished
		_ = s.store.SaveDeploymentResource(ctx, progress)
		return err
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

// connectionKeyFor returns the effective connection identity of a node.
// Terraform VPC/EKS always use the pinned Environment connection (UC-08 BR-07);
// other Terraform resources such as Aurora keep their explicit Driver Account
// and never inherit the Environment account, so a missing key stays empty and
// fails closed. Other drivers fall back to the Environment connection.
func connectionKeyFor(def resource.Definition, ctx planning.Context) string {
	if def.DriverType == resource.DriverTerraform {
		switch def.ResourceTypeKey {
		case planning.TypeVPC, planning.TypeCluster:
			return ctx.Env.ConnectionKey
		}
		// A v2 AWS provider binding targets the Environment AWS Connection
		// (UC-03 BR-08); no Definition-level account override exists.
		if inputs, err := tfmodule.ParseDriverInputs(def.DriverValues()); err == nil {
			for _, binding := range inputs.ProviderBindings {
				if binding.Target == tfmodule.TargetEnvironment && def.ConnectionKey == "" {
					return ctx.Env.ConnectionKey
				}
			}
		}
		return def.ConnectionKey
	}
	if def.ConnectionKey != "" {
		return def.ConnectionKey
	}
	return ctx.Env.ConnectionKey
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
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

// expectedConnectionKind is the Connection kind of an Environment profile.
func expectedConnectionKind(profile appdomain.ExecutionProfile) appdomain.ConnectionKind {
	if profile == appdomain.ProfileAWSEKS {
		return appdomain.ConnectionAWS
	}
	return appdomain.ConnectionKubernetes
}

// checkPinsAgainstPlan refuses a node whose pinned bindings no longer match
// the Environment it executes in: a pin for another connection, region or
// profile (stale), or a provider cluster without its graph edge (foreign).
// This runs before any state write or provider call.
func checkPinsAgainstPlan(plan *planning.Plan, descriptor string, pins []tfmodule.ProviderPin, ctx planning.Context) error {
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
			if pin.ConnectionKey != env.ConnectionKey || pin.Region != env.Region {
				return errPinStale
			}
		case tfmodule.BindingKubernetes:
			if pin.Provider == "" || !providers[pin.Provider] {
				return errPinForeign
			}
			if pin.ConnectionKey != "" && pin.ConnectionKey != env.ConnectionKey {
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
