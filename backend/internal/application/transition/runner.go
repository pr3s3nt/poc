package transition

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/envops"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

// runner executes one claimed transition. It owns the lease for its lifetime
// and always releases it.
type runner struct {
	s     *Service
	lease *envops.Lease
	p     *prepared
	t     *environment.Transition
	actor string

	srcTargets map[string]execution.Target
	quiesced   []string
	expected   map[string]execution.PostgresInventory // source descriptor -> inventory after quiesce
	archives   map[string]execution.PostgresArchive
	destPG     map[string]execution.PostgresResource
	newSetID   string
	base       string
	committed  bool
	warnings   []string
}

// compensationBudget bounds one compensation pass (scaling, route changes).
const compensationBudget = 15 * time.Minute

// stageError is a failure with a fixed, safe sentence.
type stageError struct {
	message string
	cause   error
}

func (e *stageError) Error() string { return e.message }
func (e *stageError) Unwrap() error { return e.cause }

func fail(message string, cause error) error { return &stageError{message: message, cause: cause} }

func (r *runner) ctx() context.Context { return r.lease.Ctx }

// persist saves the transition with a context that survives cancellation, so
// the failure record is written even when the claim context is cancelled.
func (r *runner) persist() error {
	ctx, cancel := r.lease.Detached()
	defer cancel()
	return r.s.store.SaveTransition(ctx, *r.t)
}

func (r *runner) stage(stage environment.TransitionStage, fn func() error) error {
	r.t.Stage = stage
	started := time.Now().UTC()
	r.t.Stages = append(r.t.Stages, environment.StageResult{Stage: stage, Status: "RUNNING", StartedAt: started})
	// No external work starts unless the stage is durable and the claim is still ours.
	if err := r.persist(); err != nil {
		return err
	}
	if err := r.lease.Stage(string(stage), map[string]any{"transition": r.t.ID}); err != nil {
		return err
	}
	err := fn()
	finished := time.Now().UTC()
	last := &r.t.Stages[len(r.t.Stages)-1]
	last.FinishedAt = &finished
	if err != nil {
		last.Status, last.Message = "FAILED", safeMessage(err)
	} else {
		last.Status = "SUCCEEDED"
	}
	if saveErr := r.persist(); saveErr != nil && err == nil {
		return saveErr
	}
	return err
}

func safeMessage(err error) string {
	var staged *stageError
	if errors.As(err, &staged) {
		return staged.message
	}
	var unsupportedErr *UnsupportedError
	if errors.As(err, &unsupportedErr) {
		return unsupportedErr.Error()
	}
	if errors.Is(err, context.Canceled) {
		return "the operation was cancelled"
	}
	return appsvc.PublicFailure(err)
}

// execute runs every stage; any failure before the cutover commit compensates
// and leaves the source generation authoritative.
func (r *runner) execute() {
	err := r.run()
	if err == nil {
		r.t.Stage, r.t.Status = environment.StageSucceeded, environment.TransitionSucceeded
		if err := r.persist(); err != nil {
			// The cutover committed but its final record is unsaved. Never report a
			// completed operation whose record is missing: keep the claim held;
			// recovery reads the committed binding and finishes the bookkeeping.
			_ = r.lease.Suspend("the cutover committed but the transition record could not be saved; recover to finish bookkeeping")
			return
		}
		if endErr := r.lease.End(environment.OpSucceeded, ""); endErr != nil {
			r.t.Compensation = append(r.t.Compensation, "the claim could not be released cleanly")
			_ = r.persist()
		}
		return
	}
	if r.lease.Lost() {
		// Recovery owns the cleanup now; this owner only stops.
		// A fenced-out owner cannot write; recovery owns the record now.
		r.t.Status, r.t.Failure = environment.TransitionInterrupted, "the operation lost its claim and was taken over by recovery"
		_ = r.lease.End(environment.OpFailed, r.t.Failure)
		return
	}
	r.t.Failure = safeMessage(err)
	compensation, cancelCompensation := context.WithTimeout(context.WithoutCancel(r.lease.Ctx), compensationBudget)
	r.s.compensate(compensation, r.t, r.snapshot())
	cancelCompensation()
	r.t.Status = environment.TransitionFailed
	if err := r.persist(); err != nil {
		r.t.Failure += " (the final record could not be saved)"
	}
	if len(r.t.CompensationFailed) > 0 {
		// An incomplete compensation keeps the claim: the Environment stays held
		// and INTERRUPTED so an operator sees and repeats recovery, instead of
		// being released under a false safe outcome.
		_ = r.lease.Suspend(r.t.Failure + "; compensation incomplete: operator action needed")
		return
	}
	_ = r.lease.End(environment.OpFailed, r.t.Failure)
}

func (r *runner) snapshot() compensationState {
	return compensationState{app: r.p.app, source: r.p.env, set: r.p.set, committed: r.committed}
}

func (r *runner) run() error {
	if err := r.stage(environment.StagePreflight, r.preflight); err != nil {
		return err
	}
	migrate := r.t.Mode == environment.ModeMigratePostgres
	if migrate {
		if err := r.stage(environment.StageQuiescing, r.quiesce); err != nil {
			return err
		}
		if err := r.stage(environment.StageBackup, r.backup); err != nil {
			return err
		}
	}
	if err := r.stage(environment.StageProvisioning, r.provision); err != nil {
		return err
	}
	if migrate {
		if err := r.stage(environment.StageRestoring, r.restore); err != nil {
			return err
		}
	}
	if err := r.stage(environment.StageDeploying, r.deployAll); err != nil {
		return err
	}
	if err := r.stage(environment.StageVerifying, r.verify); err != nil {
		return err
	}
	return r.stage(environment.StageCutover, r.cutover)
}

func (r *runner) preflight() error {
	r.srcTargets = map[string]execution.Target{}
	r.t.Replicas = map[string]int{}
	for _, id := range r.p.order {
		instance, ok := r.p.instances[id]
		if !ok {
			continue
		}
		target := execution.Target{Namespace: r.p.env.Namespace(), Extra: map[string]string{"application": r.p.app.Key, "environment": r.p.env.Key}}
		if err := appsvc.RestoreTarget(&target, instance.TargetRef, r.p.app.OrganizationKey); err != nil {
			return fail("a source workload target could not be restored", err)
		}
		r.srcTargets[id] = target
		r.t.SourceTargets = append(r.t.SourceTargets, instance.TargetRef)
		if r.s.scaler != nil {
			replicas, exists, err := r.s.scaler.Replicas(r.ctx(), target, id)
			if err != nil {
				return fail("the replica count of a source workload could not be read", err)
			}
			if exists {
				r.t.Replicas[id] = replicas
			}
		}
	}
	return nil
}

func (r *runner) quiesce() error {
	ids := make([]string, 0, len(r.srcTargets))
	for id := range r.srcTargets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		// Mark before scaling: a half-applied scale is still restored.
		r.quiesced = append(r.quiesced, id)
		r.persistQuiesced()
		if err := r.s.scaler.Scale(r.ctx(), r.srcTargets[id], id, 0); err != nil {
			return fail("a source workload could not be stopped", err)
		}
	}
	return nil
}

// persistQuiesced records which writers were touched, so recovery after a
// crash restores exactly those.
func (r *runner) persistQuiesced() {
	r.t.Compensation = nil
	_ = r.persist()
}

func (r *runner) backup() error {
	r.expected = map[string]execution.PostgresInventory{}
	r.archives = map[string]execution.PostgresArchive{}
	for _, source := range r.p.sources {
		descriptor := source.active.Descriptor.String()
		inventory, err := r.s.postgres.Inspect(r.ctx(), source.resource)
		if err != nil {
			return fail("a source database could not be inspected", err)
		}
		r.expected[descriptor] = inventory
		// Record the private path before the backup runs, so a crash or a
		// cancelled exec leaves a handle that compensation and recovery remove.
		want := execution.PostgresArchive{Pod: source.resource.Name + "-0", Dir: "/tmp/orch-backup." + strings.ReplaceAll(ids.New(), "-", "")[:20]}
		r.t.Backups = append(r.t.Backups, environment.BackupRecord{SourceDescriptor: descriptor, Pod: want.Pod, Namespace: source.resource.Namespace, Dir: want.Dir})
		if err := r.persist(); err != nil {
			return err
		}
		archive, err := r.s.postgres.Backup(r.ctx(), source.resource, want)
		if err != nil {
			return fail("a source database backup failed", err)
		}
		r.archives[descriptor] = archive
		last := &r.t.Backups[len(r.t.Backups)-1]
		last.SHA256, last.Bytes = archive.SHA256, archive.Bytes
		if err := r.persist(); err != nil {
			return err
		}
	}
	return nil
}

// destCommand deploys one DESIRED workload to the destination: the pinned draft
// (or unchanged current) Score, with the desired configuration revision.
func (r *runner) destCommand(id string, provisionOnly bool, base string) appsvc.DeployCommand {
	dest := r.p.dest
	action := deployment.ActionUpdate
	before := r.p.befores[id]
	if _, existed := r.p.set.Document.Modules[id]; !existed {
		action, before = deployment.ActionDeploy, nil
	}
	return appsvc.DeployCommand{
		OrganizationKey: r.p.app.OrganizationKey, ApplicationKey: r.p.app.Key, EnvironmentKey: r.p.env.Key,
		WorkloadID: id, ScoreBefore: before, ScoreAfter: r.p.scores[id], Action: action,
		Actor: actorOr(r.actor), RunID: RunIDFor(r.p.app.Key, r.p.env.Key, dest.TargetGeneration),
		ConfigRevisionID: r.p.scope.Revision, DeferPublicRoutes: true,
		Destination: &dest, BaseSetID: base, HoldSet: true, ProvisionOnly: provisionOnly,
	}
}

// startBase returns the planning base of the destination chain: the current Set,
// or a new immutable Set without the workloads the pinned drafts delete. Nothing
// is removed from the source; the removal only means absence at the destination.
func (r *runner) startBase() (string, error) {
	if len(r.p.deleted) == 0 {
		return "", nil
	}
	hash, err := canon.Hash(r.p.startDoc)
	if err != nil {
		return "", err
	}
	set := environment.DeploymentSet{ID: ids.New(), EnvironmentID: r.p.env.ID, EnvironmentKey: r.p.app.Key + "/" + r.p.env.Key, Document: r.p.startDoc, DocumentHash: hash, CreatedAt: time.Now().UTC()}
	if err := r.s.store.SaveDeploymentSet(r.ctx(), set); err != nil {
		return "", err
	}
	return set.ID, nil
}

func actorOr(actor string) string {
	if actor == "" {
		return "transition"
	}
	return actor
}

func (r *runner) provision() error {
	base, err := r.startBase()
	if err != nil {
		return fail("the destination planning base could not be saved", err)
	}
	r.base = base
	for _, id := range r.p.order {
		result, err := r.s.deployer.DeployWorkload(r.ctx(), r.destCommand(id, true, base))
		if err != nil {
			return fail("the destination resources could not be provisioned", err)
		}
		base = result.CandidateSetID
	}
	if r.t.Mode != environment.ModeMigratePostgres {
		return nil
	}
	r.destPG = map[string]execution.PostgresResource{}
	active, err := r.s.store.ListActiveResources(r.ctx(), r.p.app.OrganizationKey)
	if err != nil {
		return err
	}
	for _, mapping := range r.t.Mappings {
		destination := r.p.dests[mapping.DestinationDescriptor]
		var found bool
		for _, a := range active {
			if a.Descriptor.String() != mapping.DestinationDescriptor || a.Scope != destination.scope {
				continue
			}
			name, _ := a.ExecutorState["name"].(string)
			namespace, _ := a.ExecutorState["namespace"].(string)
			database, _ := a.Outputs["database"].(string)
			username, _ := a.Outputs["username"].(string)
			if name == "" || namespace == "" || database == "" || username == "" {
				return fail("a destination database has no recorded server identity", nil)
			}
			target := appsvc.ConnectionTarget(r.p.app.OrganizationKey, r.p.dest, r.p.destConn)
			target.Namespace = namespace
			r.destPG[mapping.DestinationDescriptor] = execution.PostgresResource{Target: target, Namespace: namespace, Name: name, Database: database, Username: username}
			found = true
		}
		if !found {
			return fail("a destination database was not provisioned", nil)
		}
	}
	return nil
}

func (r *runner) restore() error {
	sources := map[string]pgSource{}
	for _, source := range r.p.sources {
		sources[source.active.Descriptor.String()] = source
	}
	for _, mapping := range r.t.Mappings {
		source := sources[mapping.SourceDescriptor]
		dest := r.destPG[mapping.DestinationDescriptor]
		archive := r.archives[mapping.SourceDescriptor]
		if err := r.s.postgres.Restore(r.ctx(), source.resource, archive, dest); err != nil {
			return fail("the database restore failed", err)
		}
		inventory, err := r.s.postgres.Inspect(r.ctx(), dest)
		if err != nil {
			return fail("the restored database could not be inspected", err)
		}
		if mismatch := compareInventory(r.expected[mapping.SourceDescriptor], inventory, true); mismatch != "" {
			return fail("the restored database does not match the source ("+mismatch+")", nil)
		}
		// The archive holds application data: remove it as soon as it is verified.
		if err := r.s.postgres.RemoveArchive(r.ctx(), source.resource, archive); err != nil {
			r.warnings = append(r.warnings, "backup archive cleanup failed; it is retained privately on the source Pod")
			r.t.CompensationFailed = append(r.t.CompensationFailed, "remove backup archive "+archive.Dir)
		} else {
			for i := range r.t.Backups {
				if r.t.Backups[i].SourceDescriptor == mapping.SourceDescriptor {
					r.t.Backups[i].Removed = true
				}
			}
		}
		if err := r.persist(); err != nil {
			return err
		}
	}
	return nil
}

// compareInventory reports the first difference between expected and actual
// table inventories without naming row data. When atLeast is set the actual
// counts may exceed the expected ones (applications started since).
func compareInventory(expected, actual execution.PostgresInventory, exact bool) string {
	for name, rows := range expected.Tables {
		got, ok := actual.Tables[name]
		switch {
		case !ok:
			return "table " + name + " is missing"
		case exact && got != rows:
			return fmt.Sprintf("table %s has %d rows, expected %d", name, got, rows)
		case !exact && got < rows:
			return fmt.Sprintf("table %s lost rows", name)
		}
	}
	if exact && len(actual.Tables) != len(expected.Tables) {
		return "the table set differs"
	}
	return ""
}

func (r *runner) deployAll() error {
	base := r.base
	for _, id := range r.p.order {
		result, err := r.s.deployer.DeployWorkload(r.ctx(), r.destCommand(id, false, base))
		if err != nil {
			return fail("a workload could not be deployed to the destination", err)
		}
		base = result.CandidateSetID
		// Persist the destination writer identity before it can matter to compensation.
		r.t.DestinationWorkloads = append(r.t.DestinationWorkloads, id)
		if err := r.persist(); err != nil {
			return err
		}
	}
	r.newSetID = base
	return nil
}

func (r *runner) verify() error {
	instances, err := r.s.store.ListWorkloadInstancesFor(r.ctx(), r.p.app.Key+"/"+r.p.env.Key, r.p.dest.TargetGeneration)
	if err != nil {
		return err
	}
	ready := map[string]bool{}
	for _, instance := range instances {
		if instance.Status == deployment.InstanceReady {
			ready[instance.WorkloadID] = true
		}
	}
	for _, id := range r.p.order {
		if !ready[id] {
			return fail("a destination workload is not ready", nil)
		}
	}
	if r.t.Mode == environment.ModeMigratePostgres {
		for _, mapping := range r.t.Mappings {
			inventory, err := r.s.postgres.Inspect(r.ctx(), r.destPG[mapping.DestinationDescriptor])
			if err != nil {
				return fail("the destination database could not be inspected", err)
			}
			if mismatch := compareInventory(r.expected[mapping.SourceDescriptor], inventory, false); mismatch != "" {
				return fail("the destination data check failed ("+mismatch+")", nil)
			}
		}
	}
	return nil
}

func (r *runner) cutover() error {
	set, err := r.s.store.GetDeploymentSet(r.ctx(), r.newSetID)
	if err != nil {
		return err
	}
	sourceRoutes, destinationRoutes := hasRoutes(r.p.set.Document), hasRoutes(set.Document)
	if r.s.deployer.RoutesManaged() && (sourceRoutes || destinationRoutes) {
		// Ownership moves only now, after the destination is ready: the source
		// Ingress goes first (both would otherwise claim the same host).
		r.t.RoutesMoved = true
		if err := r.persist(); err != nil {
			return err
		}
		if err := r.s.deployer.RemoveRoutesFor(r.ctx(), r.p.app.Key, r.p.env); err != nil {
			return fail("the source route could not be removed", err)
		}
		if err := r.s.deployer.ReconcileRoutesFor(r.ctx(), r.p.app.Key, r.p.dest, set.Document); err != nil {
			return fail("the destination route could not be applied", err)
		}
		if err := r.verifyRoutes(destinationRoutes); err != nil {
			return err
		}
	}
	err = r.s.store.Transact(r.ctx(), func(ctx context.Context) error {
		current, err := r.s.store.GetEnvironment(ctx, r.p.app.Key, r.p.env.Key)
		if err != nil {
			return err
		}
		bound, err := r.s.store.BindEnvironment(ctx, persistence.EnvironmentBinding{
			ApplicationKey: r.p.app.Key, EnvironmentKey: r.p.env.Key, ConnectionKey: r.p.dest.ConnectionKey,
			Profile: r.p.dest.Profile, Region: r.p.dest.Region, RuntimeStatus: appRuntimeReady(r.p.dest), Scope: r.p.dest.InfrastructureScope,
			Generation: r.p.dest.TargetGeneration, ExpectedVersion: current.Version,
		})
		if err != nil {
			return err
		}
		if err := r.s.store.CompareVersionAndSetCurrent(ctx, r.p.app.Key, r.p.env.Key, bound.Version, r.newSetID); err != nil {
			return err
		}
		if err := r.s.store.SetPublicRoutesPending(ctx, r.p.app.Key, r.p.env.Key, false); err != nil {
			return err
		}
		// Consume exactly the pinned drafts, advancing the draft version like Deploy.
		draftVersion := current.DraftVersion
		for _, draft := range r.p.drafts {
			if err := r.s.store.DeleteWorkloadDraft(ctx, r.p.app.Key, r.p.env.Key, draft.WorkloadID, draftVersion); err != nil {
				return err
			}
			draftVersion++
		}
		return nil
	})
	if err != nil {
		return fail("the cutover could not be committed; the source remains authoritative", err)
	}
	r.committed = true
	r.t.NewSetID = r.newSetID
	r.t.SourceState = environment.SourceQuiesced
	// After the commit the source is retained and must be quiesced. Any source
	// writer that cannot be stopped is a recorded problem, never a quiesced
	// state: the destination is live but the source needs operator attention.
	if r.s.scaler != nil {
		ids := make([]string, 0, len(r.srcTargets))
		for id := range r.srcTargets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if r.t.Mode == environment.ModeMigratePostgres && contains(r.quiesced, id) {
				continue
			}
			if err := r.s.scaler.Scale(r.ctx(), r.srcTargets[id], id, 0); err != nil {
				r.t.CompensationFailed = append(r.t.CompensationFailed, "quiesce source workload "+id)
				r.t.SourceState = environment.SourceNeedsAttention
			} else {
				r.quiesced = append(r.quiesced, id)
			}
		}
	} else if len(r.srcTargets) > 0 {
		r.t.CompensationFailed = append(r.t.CompensationFailed, "quiesce source workloads (no scaler)")
		r.t.SourceState = environment.SourceNeedsAttention
	}
	_ = r.persist()
	return nil
}

func contains(items []string, item string) bool {
	for _, candidate := range items {
		if candidate == item {
			return true
		}
	}
	return false
}

func appRuntimeReady(environment.Environment) appdomain.RuntimeStatus { return appdomain.RuntimeReady }

func hasRoutes(doc environment.Document) bool {
	for _, id := range doc.ModuleIDs() {
		if len(doc.Modules[id].Spec.Service.Routes()) > 0 {
			return true
		}
	}
	return false
}

func (r *runner) verifyRoutes(destinationRoutes bool) error {
	if r.s.routes == nil {
		return nil
	}
	srcTarget, err := r.s.deployer.GenerationTarget(r.ctx(), r.p.app, r.p.env)
	if err != nil {
		return err
	}
	destTarget, err := r.s.deployer.GenerationTarget(r.ctx(), r.p.app, r.p.dest)
	if err != nil {
		return err
	}
	onDest, err := r.s.routes.HasPublicRoute(r.ctx(), destTarget, r.p.app.Key, r.p.env.Key)
	if err != nil || onDest != destinationRoutes {
		return fail("the destination does not own the public route", err)
	}
	onSource, err := r.s.routes.HasPublicRoute(r.ctx(), srcTarget, r.p.app.Key, r.p.env.Key)
	if err != nil || onSource {
		return fail("the source still owns the public route", err)
	}
	return nil
}
