package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// RetainedSecretAnnotation marks a VaultStaticSecret kept in the Environment
// bundle only because the last successful release still uses it (ADR-042).
const RetainedSecretAnnotation = "orchestrator.io/retained-secret"

// EnvironmentRequest publishes one Deploy batch as a single Environment
// release (ADR-042). Workloads holds the complete desired manifests of every
// changed workload; Removed lists workloads leaving the Environment. Workloads
// absent from both keep their last published manifests unchanged.
type EnvironmentRequest struct {
	OrganizationKey string
	ApplicationKey  string
	Environment     environment.Environment
	ReleaseID       string
	Target          execution.Target
	Workloads       map[string][]execution.Manifest
	Removed         []string
}

// RolloutFailure names the workloads whose objects were not healthy when the
// Environment release failed its readiness check. An empty list means the
// failing workload could not be attributed.
type RolloutFailure struct {
	Workloads []string
	Cause     error
}

func (e *RolloutFailure) Error() string { return e.Cause.Error() }
func (e *RolloutFailure) Unwrap() error { return e.Cause }

// observedFailure carries the last observation of a determined rollout failure.
type observedFailure struct {
	cause       error
	observation domain.Observation
}

func (e *observedFailure) Error() string { return e.cause.Error() }
func (e *observedFailure) Unwrap() error { return e.cause }

// storedManifest is the serializable form of one desired object.
type storedManifest struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Namespace  string         `json:"namespace"`
	Object     map[string]any `json:"object"`
}

// composition is the desired manifests of a unit revision, per workload. It is
// stored next to the provider payload so the next release can replace only the
// changed workloads.
type composition struct {
	Version   int                         `json:"version"`
	Workloads map[string][]storedManifest `json:"workloads"`
}

func compositionID(revisionID string) string { return stableID("delivery-composition", revisionID) }

func toStored(ms []execution.Manifest) ([]storedManifest, error) {
	out := make([]storedManifest, 0, len(ms))
	for _, m := range ms {
		if m.Secret || m.Object == nil {
			return nil, domain.ErrUnsupported
		}
		object, err := deepCopy(m.Object)
		if err != nil {
			return nil, err
		}
		out = append(out, storedManifest{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name, Namespace: m.Namespace, Object: object})
	}
	return out, nil
}

func fromStored(ms []storedManifest) ([]execution.Manifest, error) {
	out := make([]execution.Manifest, 0, len(ms))
	for _, m := range ms {
		object, err := deepCopy(m.Object)
		if err != nil {
			return nil, err
		}
		out = append(out, execution.Manifest{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name, Namespace: m.Namespace, Object: object})
	}
	return out, nil
}

func deepCopy(object map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(object)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil, domain.ErrInvalid
	}
	return out, nil
}

func (c *Coordinator) putComposition(ctx context.Context, org, unit, revisionID string, comp composition) error {
	payload, err := json.Marshal(comp)
	if err != nil {
		return domain.ErrInvalid
	}
	id := compositionID(revisionID)
	if existing, err := c.Artifacts.Get(ctx, org, unit, id); err == nil {
		if string(existing) != string(payload) {
			return persistence.ErrImmutable
		}
		return nil
	}
	return c.Artifacts.PutWithID(ctx, org, unit, id, payload)
}

func (c *Coordinator) getComposition(ctx context.Context, e persistence.DeliveryExecution) (composition, error) {
	raw, err := c.Artifacts.Get(ctx, e.OrganizationID, e.UnitID, compositionID(e.Revision.ID))
	if err != nil {
		return composition{}, err
	}
	var out composition
	if json.Unmarshal(raw, &out) != nil || out.Version != 1 {
		return composition{}, ErrArtifact
	}
	return out, nil
}

// UsesEnvironmentBundle reports whether the Environment delivers through the
// ADR-042 Environment units: true unless a per-workload unit is still active.
func (c *Coordinator) UsesEnvironmentBundle(ctx context.Context, orgKey string, env environment.Environment) (bool, error) {
	if c == nil || c.Store == nil {
		return false, ErrUnavailable
	}
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return false, err
	}
	executions, err := c.Store.ListDeliveryExecutions(ctx, org.ID, env.ID)
	if err != nil {
		return false, err
	}
	latest := map[string]persistence.DeliveryExecution{}
	for _, e := range executions {
		if e.WorkloadID == domain.EnvironmentWorkload {
			continue
		}
		if e.Mutation.Status == domain.IntentRunning || e.Mutation.Status == domain.IntentUnknown {
			return false, nil
		}
		if e.Mutation.Status != domain.IntentAcknowledged {
			continue
		}
		if prior, ok := latest[e.UnitID]; !ok || e.CreatedAt.After(prior.CreatedAt) {
			latest[e.UnitID] = e
		}
	}
	for _, e := range latest {
		if e.Mutation.Action == domain.Submit || e.Mutation.Action == domain.Adopt {
			return false, nil
		}
	}
	return true, nil
}

func environmentUnit(req EnvironmentRequest, orgID, key string) (domain.Scope, string) {
	scope := domain.Scope{OrganizationID: orgID, EnvironmentID: req.Environment.ID, TargetGeneration: req.Environment.TargetGeneration, Epoch: req.Environment.DeliveryEpoch, UnitKey: key}
	return scope, stableID("delivery-unit", scope.OrganizationID, scope.EnvironmentID, fmt.Sprint(scope.TargetGeneration), fmt.Sprint(scope.Epoch), scope.UnitKey)
}

func published(e persistence.DeliveryExecution) bool {
	return e.Receipt.IntentID == e.Mutation.ID && e.Receipt.RevisionID == e.Revision.ID
}

// unitHistory returns the unit's newest published revision (the base of the
// next release) and its newest successful one. A withdrawn unit has neither.
// An unknown outcome holds the Environment.
func unitHistory(executions []persistence.DeliveryExecution, unit string) (base, success *persistence.DeliveryExecution, err error) {
	records := []persistence.DeliveryExecution{}
	for _, e := range executions {
		if e.UnitID == unit {
			records = append(records, e)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })
	for index := range records {
		e := &records[index]
		if e.Mutation.Status == domain.IntentRunning || e.Mutation.Status == domain.IntentUnknown {
			return nil, nil, domain.ErrOutcomeUnknown
		}
		if e.Mutation.Action == domain.Withdraw {
			if e.Mutation.Status == domain.IntentAcknowledged {
				return base, success, nil
			}
			continue
		}
		if e.Mutation.Action != domain.Submit && e.Mutation.Action != domain.Adopt {
			continue
		}
		if base == nil && (e.Mutation.Status == domain.IntentAcknowledged || (e.Mutation.Status == domain.IntentFailed && published(*e))) {
			base = e
		}
		if e.Mutation.Status == domain.IntentAcknowledged {
			return base, e, nil
		}
	}
	return base, nil, nil
}

func objectKey(m execution.Manifest) string { return m.APIVersion + "/" + m.Kind + "/" + m.Name }

// jobName gives a Job a content-derived name: an unchanged Job keeps its name
// and is never re-run, a changed Job becomes a new object (ADR-042).
func jobName(m execution.Manifest) (string, error) {
	spec, err := json.Marshal(m.Object["spec"])
	if err != nil {
		return "", domain.ErrInvalid
	}
	sum := sha256.Sum256(spec)
	base := m.Name
	if len(base) > 54 {
		base = strings.TrimRight(base[:54], "-.")
	}
	return base + "-" + hex.EncodeToString(sum[:4]), nil
}

func renameJob(m execution.Manifest) (execution.Manifest, error) {
	name, err := jobName(m)
	if err != nil {
		return m, err
	}
	object, err := deepCopy(m.Object)
	if err != nil {
		return m, err
	}
	md, _ := object["metadata"].(map[string]any)
	if md == nil {
		return m, domain.ErrInvalid
	}
	md["name"] = name
	// The deployment label changes on every render; an unchanged Job must stay
	// byte-identical so the migrate unit is not republished for it.
	if labels, ok := md["labels"].(map[string]any); ok {
		delete(labels, "orchestrator.io/deployment-id")
	}
	m.Object, m.Name = object, name
	return m, nil
}

// flatten merges per-workload manifests into one unit. Shared objects must be
// identical; it returns the merged list and each object's workload.
func flatten(workloads map[string][]execution.Manifest) ([]execution.Manifest, map[string]string, error) {
	names := make([]string, 0, len(workloads))
	for name := range workloads {
		names = append(names, name)
	}
	sort.Strings(names)
	out := []execution.Manifest{}
	owner := map[string]string{}
	seen := map[string]execution.Manifest{}
	for _, name := range names {
		for _, m := range workloads[name] {
			key := objectKey(m)
			if prior, ok := seen[key]; ok {
				if !reflect.DeepEqual(prior.Object, m.Object) {
					return nil, nil, fmt.Errorf("%w: workloads declare different %s objects named %s", domain.ErrInvalid, m.Kind, m.Name)
				}
				continue
			}
			seen[key] = m
			owner[key] = name
			out = append(out, m)
		}
	}
	return out, owner, nil
}

func storedWorkloads(comp composition) (map[string][]execution.Manifest, error) {
	out := map[string][]execution.Manifest{}
	for name, ms := range comp.Workloads {
		converted, err := fromStored(ms)
		if err != nil {
			return nil, err
		}
		out[name] = converted
	}
	return out, nil
}

func toComposition(workloads map[string][]execution.Manifest) (composition, error) {
	comp := composition{Version: 1, Workloads: map[string][]storedManifest{}}
	for name, ms := range workloads {
		stored, err := toStored(ms)
		if err != nil {
			return comp, err
		}
		comp.Workloads[name] = stored
	}
	return comp, nil
}

// DeployEnvironment publishes the changed workloads as one Environment
// release: Jobs of changed workloads first (waiting for completion), then all
// workloads in one app unit. It never falls back to per-workload units.
func (c *Coordinator) DeployEnvironment(ctx context.Context, req EnvironmentRequest) error {
	if c == nil || c.Store == nil || c.Registry == nil || c.Artifacts == nil {
		return ErrUnavailable
	}
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	if req.ReleaseID == "" {
		req.ReleaseID = owner.OperationID
	}
	b, provider, err := c.binding(ctx, req.OrganizationKey, req.Environment)
	if err != nil {
		return err
	}
	org, err := c.Store.GetOrganization(ctx, req.OrganizationKey)
	if err != nil {
		return err
	}
	target := req.Target
	target.Extra = map[string]string{}
	for k, v := range req.Target.Extra {
		if k != "deployment" && k != "workload" {
			target.Extra[k] = v
		}
	}
	if len(target.Extra) == 0 {
		target.Extra = nil
	}
	req.Target = target
	executions, err := c.Store.ListDeliveryExecutions(ctx, org.ID, req.Environment.ID)
	if err != nil {
		return err
	}
	_, appUnit := environmentUnit(req, org.ID, domain.EnvironmentAppUnit)
	_, migrateUnit := environmentUnit(req, org.ID, domain.EnvironmentMigrateUnit)
	appBase, appSuccess, err := unitHistory(executions, appUnit)
	if err != nil {
		persistence.MarkHold(ctx)
		return err
	}
	migrateBase, _, err := unitHistory(executions, migrateUnit)
	if err != nil {
		persistence.MarkHold(ctx)
		return err
	}
	load := func(e *persistence.DeliveryExecution) (map[string][]execution.Manifest, error) {
		if e == nil {
			return map[string][]execution.Manifest{}, nil
		}
		comp, err := c.getComposition(ctx, *e)
		if err != nil {
			return nil, err
		}
		return storedWorkloads(comp)
	}
	apps, err := load(appBase)
	if err != nil {
		return err
	}
	baseApps, err := load(appBase)
	if err != nil {
		return err
	}
	jobs, err := load(migrateBase)
	if err != nil {
		return err
	}
	previousJobs, err := toComposition(jobs)
	if err != nil {
		return err
	}
	removed := map[string]bool{}
	for _, name := range req.Removed {
		removed[name] = true
		delete(apps, name)
		delete(jobs, name)
	}
	for name, manifests := range req.Workloads {
		if removed[name] || name == "" || strings.HasPrefix(name, "_") {
			return domain.ErrInvalid
		}
		appPart, jobPart := []execution.Manifest{}, []execution.Manifest{}
		for _, m := range manifests {
			if m.Secret || (m.Kind == "Secret" && !execution.PublicCertificateSecret(m.Object)) {
				return domain.ErrUnsupported
			}
			if m.Namespace != req.Target.Namespace {
				return fmt.Errorf("%w: every workload of an Environment bundle must use namespace %s", domain.ErrUnsupported, req.Target.Namespace)
			}
			if m.Kind == "Job" {
				renamed, err := renameJob(m)
				if err != nil {
					return err
				}
				jobPart = append(jobPart, renamed)
				continue
			}
			appPart = append(appPart, m)
		}
		apps[name] = appPart
		if len(jobPart) > 0 {
			jobs[name] = jobPart
		} else {
			delete(jobs, name)
		}
	}
	// Desired composition (without retained secrets) is what the next release
	// starts from.
	desired, err := toComposition(apps)
	if err != nil {
		return err
	}
	// ADR-042 decision 6: keep the VaultStaticSecrets of the last successful
	// release that the new desired state no longer uses.
	var lastSuccess map[string][]execution.Manifest
	if appSuccess != nil {
		if lastSuccess, err = load(appSuccess); err != nil {
			return err
		}
	}
	withRetained, err := retainSecrets(apps, lastSuccess)
	if err != nil {
		return err
	}
	appManifests, appOwners, err := flatten(withRetained)
	if err != nil {
		return err
	}
	jobManifests, _, err := flatten(jobs)
	if err != nil {
		return err
	}
	jobComposition, err := toComposition(jobs)
	if err != nil {
		return err
	}
	// Validate both proposed artifacts before the first external write.
	appIntent, _ := c.environmentIntent(req, org.ID, owner, b, domain.EnvironmentAppUnit, domain.Rollout)
	if len(appManifests) > 0 {
		if _, err := provider.Prepare(ctx, appIntent, appManifests); err != nil {
			return err
		}
	}
	migrateChanged := !reflect.DeepEqual(previousJobs.Workloads, jobComposition.Workloads)
	if len(jobManifests) > 0 && migrateChanged {
		// A changed Job can need a Secret the published app unit does not
		// declare yet (first deploy or a new value version). Publish only the
		// secret delivery objects first, so the Job can start (ADR-042).
		if prepare, needed := secretPreparation(baseApps, req.Workloads, jobManifests); needed {
			prepared, err := retainSecrets(prepare, lastSuccess)
			if err != nil {
				return err
			}
			manifests, _, err := flatten(prepared)
			if err != nil {
				return err
			}
			comp, err := toComposition(prepare)
			if err != nil {
				return err
			}
			prepReq := req
			prepReq.ReleaseID = req.ReleaseID + "/secrets"
			intent, _ := c.environmentIntent(prepReq, org.ID, owner, b, domain.EnvironmentAppUnit, domain.Rollout)
			added := addedSecretObjects(baseApps, prepare)
			if err := c.runEnvironmentUnitAccepting(ctx, provider, intent, appUnit, executions, manifests, comp, secretsApplied(intent.Revision.ID, added)); err != nil {
				return err
			}
			// The preparation is now the published base of the final app revision.
			if executions, err = c.Store.ListDeliveryExecutions(ctx, org.ID, req.Environment.ID); err != nil {
				return err
			}
		}
		intent, _ := c.environmentIntent(req, org.ID, owner, b, domain.EnvironmentMigrateUnit, domain.Job)
		if _, err := provider.Prepare(ctx, intent, jobManifests); err != nil {
			return err
		}
		if err := c.runEnvironmentUnit(ctx, provider, intent, migrateUnit, executions, jobManifests, jobComposition); err != nil {
			return err
		}
	}
	if len(appManifests) > 0 {
		if err := c.runEnvironmentUnit(ctx, provider, appIntent, appUnit, executions, appManifests, desired); err != nil {
			var observed *observedFailure
			if errors.As(err, &observed) {
				return &RolloutFailure{Workloads: failingWorkloads(observed.observation, appOwners), Cause: err}
			}
			return err
		}
	} else if appBase != nil {
		active, err := activeExecutions(executions, domain.EnvironmentWorkload)
		if err != nil {
			persistence.MarkHold(ctx)
			return err
		}
		for _, e := range active {
			if e.UnitID == appUnit {
				if err := c.removeExecution(ctx, req.OrganizationKey, e); err != nil {
					return err
				}
			}
		}
	}
	// Completed Jobs of workloads that no longer have one are cleaned up only
	// after the release succeeded.
	if len(jobManifests) == 0 && migrateBase != nil {
		active, err := activeExecutions(executions, domain.EnvironmentWorkload)
		if err != nil {
			return err
		}
		for _, e := range active {
			if e.UnitID == migrateUnit {
				if err := c.removeExecution(ctx, req.OrganizationKey, e); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *Coordinator) environmentIntent(req EnvironmentRequest, orgID string, owner persistence.Owner, b domain.BindingVersion, key string, phase domain.Phase) (port.Intent, string) {
	selection := req.Environment.DesiredDelivery.Effective()
	if phase == domain.Job {
		selection.SelfHeal = false
	}
	scope, unit := environmentUnit(req, orgID, key)
	revision := domain.Revision{ID: stableID("delivery-revision", owner.OperationID, req.ReleaseID, unit), Scope: scope, Phase: phase, Selection: selection, PayloadDigest: strings.Repeat("0", 64), ArtifactRef: "preparing"}
	return port.Intent{ID: stableID("delivery-intent", revision.ID, string(domain.Submit)), OrganizationKey: req.OrganizationKey, Owner: port.Owner{OperationID: owner.OperationID, Instance: owner.Owner, Fence: owner.Fence}, Binding: b, Revision: revision, Target: req.Target}, unit
}

// runEnvironmentUnit stores the composition and runs the unit revision with
// the same receipt/lease/readiness protocol as a per-workload phase.
func (c *Coordinator) runEnvironmentUnit(ctx context.Context, provider port.Provider, intent port.Intent, unit string, executions []persistence.DeliveryExecution, manifests []execution.Manifest, comp composition) error {
	return c.runEnvironmentUnitAccepting(ctx, provider, intent, unit, executions, manifests, comp, nil)
}

func (c *Coordinator) runEnvironmentUnitAccepting(ctx context.Context, provider port.Provider, intent port.Intent, unit string, executions []persistence.DeliveryExecution, manifests []execution.Manifest, comp composition, accept func(domain.Observation) bool) error {
	if err := c.putComposition(ctx, intent.Revision.Scope.OrganizationID, unit, intent.Revision.ID, comp); err != nil {
		return err
	}
	active, err := activeExecutions(executions, domain.EnvironmentWorkload)
	if err != nil {
		persistence.MarkHold(ctx)
		return err
	}
	for _, e := range active {
		if e.UnitID == unit {
			intent.ExpectedRevision = e.Revision.ID
			intent.SourceInventory = append([]port.AppliedObject(nil), e.Inventory...)
		}
	}
	if failed := publishedFailure(executions, unit, active); failed != nil {
		intent.ExpectedRevision = failed.Revision.ID
	}
	return c.runPhaseAccepting(ctx, provider, intent, unit, domain.EnvironmentWorkload, manifests, accept)
}

// addedSecretObjects lists the kind/name keys of the secret delivery objects
// that the preparation adds to the published composition.
func addedSecretObjects(base, prepare map[string][]execution.Manifest) map[string]bool {
	present := map[string]bool{}
	for _, ms := range base {
		for _, m := range ms {
			present[m.Kind+"/"+m.Name] = true
		}
	}
	out := map[string]bool{}
	for _, ms := range prepare {
		for _, m := range ms {
			if secretDelivery(m) && !present[m.Kind+"/"+m.Name] {
				out[m.Kind+"/"+m.Name] = true
			}
		}
	}
	return out
}

// secretsApplied accepts a secret preparation once its revision is applied
// and every object it adds is healthy, even when an object that was already
// published (for example a broken workload being fixed by this Deploy) is
// not Ready. A failure of an added object still fails the preparation.
func secretsApplied(revision string, added map[string]bool) func(domain.Observation) bool {
	return func(o domain.Observation) bool {
		healthy := map[string]bool{}
		for _, object := range o.Objects {
			key := object.Kind + "/" + object.Name
			if added[key] && object.Health != domain.Healthy && object.Health != "" {
				return false
			}
			if object.Health == domain.Healthy {
				healthy[key] = true
			}
		}
		if o.Complete && o.ObservedRevision == revision {
			// Full object-level observation of this revision.
			for key := range added {
				if !healthy[key] {
					return false
				}
			}
			return true
		}
		// Fleet's aggregate apply failure for this deployment lists only the
		// objects that blocked it; Helm applied everything else.
		return o.Health == domain.Degraded && o.Sync == domain.OutOfSync && len(o.Objects) > 0
	}
}

// failingWorkloads attributes unhealthy observed objects to their workloads.
func failingWorkloads(observation domain.Observation, owners map[string]string) []string {
	set := map[string]bool{}
	byKindName := map[string]string{}
	for key, name := range owners {
		parts := strings.Split(key, "/")
		if len(parts) >= 2 {
			byKindName[parts[len(parts)-2]+"/"+parts[len(parts)-1]] = name
		}
	}
	for _, o := range observation.Objects {
		if o.Health == domain.Degraded || o.Health == domain.Progressing {
			name := owners[o.APIVersion+"/"+o.Kind+"/"+o.Name]
			if name == "" {
				// Fleet's apply message names kind and name only.
				name = byKindName[o.Kind+"/"+o.Name]
			}
			if name != "" {
				set[name] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// retainSecrets adds, per remaining workload, the VaultStaticSecrets of the
// last successful release that the desired manifests no longer declare.
func retainSecrets(apps, lastSuccess map[string][]execution.Manifest) (map[string][]execution.Manifest, error) {
	out := map[string][]execution.Manifest{}
	for name, ms := range apps {
		out[name] = append([]execution.Manifest(nil), ms...)
	}
	for name, ms := range lastSuccess {
		current, keep := out[name]
		if !keep {
			continue
		}
		names := map[string]bool{}
		for _, m := range current {
			names[objectKey(m)] = true
		}
		for _, m := range ms {
			if m.Kind != "VaultStaticSecret" || names[objectKey(m)] {
				continue
			}
			object, err := deepCopy(m.Object)
			if err != nil {
				return nil, err
			}
			md, _ := object["metadata"].(map[string]any)
			if md == nil {
				return nil, domain.ErrInvalid
			}
			annotations, _ := md["annotations"].(map[string]any)
			if annotations == nil {
				annotations = map[string]any{}
			}
			annotations[RetainedSecretAnnotation] = "true"
			md["annotations"] = annotations
			m.Object = object
			out[name] = append(out[name], m)
		}
	}
	return out, nil
}

// secretDelivery reports a platform secret-delivery object of a workload.
func secretDelivery(m execution.Manifest) bool {
	switch {
	case strings.HasPrefix(m.APIVersion, "secrets.hashicorp.com/"):
		return true
	case m.APIVersion == "v1" && m.Kind == "ServiceAccount" && strings.HasPrefix(m.Name, "orch-"):
		return true
	case m.APIVersion == "v1" && m.Kind == "Secret" && execution.PublicCertificateSecret(m.Object):
		return true
	}
	return false
}

// jobSecretNames lists the Secrets the pods of the Jobs reference.
func jobSecretNames(jobs []execution.Manifest) map[string]bool {
	out := map[string]bool{}
	for _, job := range jobs {
		spec, _ := job.Object["spec"].(map[string]any)
		template, _ := spec["template"].(map[string]any)
		pod, _ := template["spec"].(map[string]any)
		containers, _ := pod["containers"].([]any)
		if initContainers, ok := pod["initContainers"].([]any); ok {
			containers = append(containers, initContainers...)
		}
		for _, raw := range containers {
			container, _ := raw.(map[string]any)
			env, _ := container["env"].([]any)
			for _, rawEnv := range env {
				entry, _ := rawEnv.(map[string]any)
				from, _ := entry["valueFrom"].(map[string]any)
				ref, _ := from["secretKeyRef"].(map[string]any)
				if name, _ := ref["name"].(string); name != "" {
					out[name] = true
				}
			}
			envFrom, _ := container["envFrom"].([]any)
			for _, rawFrom := range envFrom {
				entry, _ := rawFrom.(map[string]any)
				ref, _ := entry["secretRef"].(map[string]any)
				if name, _ := ref["name"].(string); name != "" {
					out[name] = true
				}
			}
		}
		volumes, _ := pod["volumes"].([]any)
		for _, raw := range volumes {
			volume, _ := raw.(map[string]any)
			secret, _ := volume["secret"].(map[string]any)
			if name, _ := secret["secretName"].(string); name != "" {
				out[name] = true
			}
		}
	}
	return out
}

// secretPreparation returns the published app composition extended with the
// secret delivery objects of the changed workloads, when a changed Job needs
// a Secret that the published composition does not declare.
func secretPreparation(base, changed map[string][]execution.Manifest, jobs []execution.Manifest) (map[string][]execution.Manifest, bool) {
	declared := map[string]bool{}
	for _, ms := range base {
		for _, m := range ms {
			if m.Kind == "VaultStaticSecret" {
				declared[m.Name] = true
			}
		}
	}
	missing := false
	for name := range jobSecretNames(jobs) {
		missing = missing || !declared[name]
	}
	if !missing {
		return nil, false
	}
	out := map[string][]execution.Manifest{}
	for name, ms := range base {
		out[name] = append([]execution.Manifest(nil), ms...)
	}
	for name, ms := range changed {
		current := out[name]
		present := map[string]bool{}
		for _, m := range current {
			present[objectKey(m)] = true
		}
		for _, m := range ms {
			if secretDelivery(m) && !present[objectKey(m)] {
				current = append(current, m)
			}
		}
		out[name] = current
	}
	return out, true
}
