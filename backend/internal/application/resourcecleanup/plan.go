package resourcecleanup

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// snapshot is one consistent read of everything a deletion decision needs. It
// is built from the Console's safe projections only.
type snapshot struct {
	scope    Scope
	appID    string
	env      environment.Environment
	records  map[string]tfresource.ResourceRecord
	order    []string
	edges    []tfresource.Dependency
	active   map[string]resource.ActiveResource
	types    map[string]resource.Type
	doc      environment.Document
	drafts   []environment.WorkloadDraft
	cleanups []tfresource.CleanupRecord
}

// loadSnapshot reads the scope through st/rd. Inside a transaction (the
// admission) both read that transaction; otherwise the caller wraps the call
// in ReadSnapshot. A foreign or missing Application/Environment is
// persistence.ErrNotFound.
func loadSnapshot(ctx context.Context, st persistence.Store, rd terraformstate.ResourceReader, sc Scope) (*snapshot, error) {
	app, err := st.GetApplication(ctx, sc.ApplicationKey)
	if err != nil {
		return nil, err
	}
	if sc.OrganizationKey == "" || app.OrganizationKey != sc.OrganizationKey {
		return nil, persistence.ErrNotFound
	}
	env, err := st.GetEnvironment(ctx, sc.ApplicationKey, sc.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	snap := &snapshot{scope: sc, appID: app.ID, env: env, records: map[string]tfresource.ResourceRecord{}, active: map[string]resource.ActiveResource{}, types: map[string]resource.Type{}, doc: environment.NewDocument()}
	records, err := rd.ListResourceRecords(ctx, sc.ApplicationKey, sc.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	for _, rec := range records {
		snap.records[rec.Instance.ID] = rec
		snap.order = append(snap.order, rec.Instance.ID)
	}
	if snap.edges, err = rd.ListResourceEdges(ctx, sc.ApplicationKey, sc.EnvironmentKey); err != nil {
		return nil, err
	}
	if snap.cleanups, err = rd.ListCleanupRecords(ctx, sc.ApplicationKey, sc.EnvironmentKey, 50); err != nil {
		return nil, err
	}
	activeList, err := st.ListActiveResources(ctx, sc.OrganizationKey)
	if err != nil {
		return nil, err
	}
	for _, a := range activeList {
		snap.active[a.ID] = a
	}
	types, err := st.ListResourceTypes(ctx, sc.OrganizationKey)
	if err != nil {
		return nil, err
	}
	for _, t := range types {
		snap.types[t.Key] = t
	}
	if env.CurrentDeploymentSetID != "" {
		set, err := st.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
		if err != nil {
			return nil, err
		}
		snap.doc = set.Document
	}
	if snap.drafts, err = st.ListWorkloadDrafts(ctx, sc.ApplicationKey, sc.EnvironmentKey); err != nil {
		return nil, err
	}
	return snap, nil
}

// activeOf returns the linked Active Resource of a record, if any.
func (s *snapshot) activeOf(rec tfresource.ResourceRecord) (resource.ActiveResource, bool) {
	if rec.Instance.ActiveResourceID == "" {
		return resource.ActiveResource{}, false
	}
	a, ok := s.active[rec.Instance.ActiveResourceID]
	return a, ok
}

// platformResource reports a resource the Environment does not own: the
// Application-wide scope (shared by every Environment) and the registered
// cluster of the Connection are platform, never deletable from here.
func platformResource(rec tfresource.ResourceRecord) bool {
	id := rec.Instance.Identity
	if id.Scope.Type == resource.ScopeApplication {
		return true
	}
	d, err := resource.ParseDescriptor(id.Descriptor)
	if err != nil {
		return true
	}
	return d.Type == "k8s-cluster" && d.Class == "internal" || strings.HasPrefix(d.ID, "connections.")
}

// staticReasons are the blockers that depend on the item alone (not on what
// else is selected), in a stable order.
func (s *snapshot) staticReasons(rec tfresource.ResourceRecord, consumers []ConsumerView, external bool) []string {
	reasons := []string{}
	if platformResource(rec) {
		reasons = append(reasons, ReasonPlatformResource)
	}
	if rec.Instance.Status != tfresource.InstanceUnreferenced {
		reasons = append(reasons, ReasonNotUnreferenced)
	} else if a, ok := s.activeOf(rec); ok && a.Status != resource.StatusUnreferenced {
		reasons = append(reasons, ReasonNotUnreferenced)
	}
	if rec.Attempts == 0 && rec.Instance.Status != tfresource.InstancePending {
		reasons = append(reasons, ReasonOwnershipUnproven)
	}
	if external {
		reasons = append(reasons, ReasonHasConsumers)
	}
	return reasons
}

// workloadConsumers lists the workloads and Deployment Set entries that still
// use a resource: the current Deployment Set and the pending (draft) desired
// graph. Environment infrastructure (namespace, VPC, cluster) is used by every
// workload, so any workload or pending upsert is a consumer of it.
func (s *snapshot) workloadConsumers(rec tfresource.ResourceRecord) []ConsumerView {
	d, err := resource.ParseDescriptor(rec.Instance.Identity.Descriptor)
	if err != nil {
		return []ConsumerView{{Kind: KindDeploymentSet, Name: "unknown"}}
	}
	out := []ConsumerView{}
	pendingUpserts := []string{}
	for _, draft := range s.drafts {
		if draft.State == environment.DraftUpsert {
			pendingUpserts = append(pendingUpserts, draft.WorkloadID)
		}
	}
	sort.Strings(pendingUpserts)
	switch {
	case strings.HasPrefix(d.ID, "modules."):
		rest := strings.TrimPrefix(d.ID, "modules.")
		workload, name, _ := strings.Cut(rest, ".externals.")
		if m, ok := s.doc.Modules[workload]; ok {
			if _, declared := m.Externals[name]; declared && name != "" {
				out = append(out, ConsumerView{Kind: KindWorkload, Name: workload})
			}
		}
		for _, draft := range s.drafts {
			if draft.WorkloadID == workload && draft.State == environment.DraftUpsert && draftDeclares(draft.Score, "", name, d) {
				out = append(out, ConsumerView{Kind: KindPendingWorkload, Name: workload})
			}
		}
	case strings.HasPrefix(d.ID, "shared."):
		sid := strings.TrimPrefix(d.ID, "shared.")
		if _, ok := s.doc.Shared[sid]; ok {
			out = append(out, ConsumerView{Kind: KindDeploymentSet, Name: d.ID})
		}
		for _, id := range s.doc.ModuleIDs() {
			if moduleMentions(s.doc.Modules[id], "shared."+sid) {
				out = append(out, ConsumerView{Kind: KindWorkload, Name: id})
			}
		}
		for _, draft := range s.drafts {
			if draft.State == environment.DraftUpsert && draftDeclares(draft.Score, sid, "", d) {
				out = append(out, ConsumerView{Kind: KindPendingWorkload, Name: draft.WorkloadID})
			}
		}
	default:
		// Environment, Application and any unrecognised path: infrastructure
		// every workload uses. Unknown shapes fail closed the same way.
		for _, id := range s.doc.ModuleIDs() {
			out = append(out, ConsumerView{Kind: KindWorkload, Name: id})
		}
		for _, id := range pendingUpserts {
			out = append(out, ConsumerView{Kind: KindPendingWorkload, Name: id})
		}
	}
	return dedupeConsumers(out)
}

func dedupeConsumers(in []ConsumerView) []ConsumerView {
	seen := map[string]bool{}
	out := []ConsumerView{}
	for _, c := range in {
		key := c.Kind + "|" + c.Name + "|" + c.ResourceID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func moduleMentions(m environment.Module, needle string) bool {
	b, err := json.Marshal(m)
	return err == nil && strings.Contains(string(b), needle)
}

// draftDeclares reports whether a pending Score declares the resource: a
// shared one by its id, or a private one by its alias, with the same type and
// class as the descriptor.
func draftDeclares(score map[string]any, sharedID, alias string, d resource.Descriptor) bool {
	resources, _ := score["resources"].(map[string]any)
	for name, raw := range resources {
		entry, _ := raw.(map[string]any)
		typ, _ := entry["type"].(string)
		class, _ := entry["class"].(string)
		id, _ := entry["id"].(string)
		if class == "" {
			class = "default"
		}
		if typ != d.Type || class != d.Class {
			continue
		}
		if sharedID != "" && id == sharedID {
			return true
		}
		if sharedID == "" && id == "" && name == alias {
			return true
		}
	}
	return false
}

// plan is the decision for one root: its dependency universe and what each
// item blocks or requires.
type plan struct {
	snap     *snapshot
	root     string
	items    map[string]*planItem
	order    []string // root first, then candidates sorted by descriptor
	busy     bool
	activeOp string
}

type planItem struct {
	rec        tfresource.ResourceRecord
	reasons    []string
	consumers  []ConsumerView
	requiredBy []string // consumer instance IDs inside the universe
}

func (p *plan) item(id string) *planItem { return p.items[id] }

// build computes the plan of a root. The universe is the root and every
// resource it (transitively) depends on in the same Environment/generation;
// platform resources are not part of it.
func buildPlan(snap *snapshot, rootID string) (*plan, error) {
	root, ok := snap.records[rootID]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	generation := root.Instance.Identity.TargetGeneration
	providersOf := map[string][]string{}
	for _, e := range snap.edges {
		providersOf[e.ConsumerID] = append(providersOf[e.ConsumerID], e.ProviderID)
	}
	universe := map[string]bool{rootID: true}
	queue := []string{rootID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, prov := range providersOf[id] {
			rec, ok := snap.records[prov]
			if !ok || universe[prov] || rec.Instance.Identity.TargetGeneration != generation || platformResource(rec) {
				continue
			}
			universe[prov] = true
			queue = append(queue, prov)
		}
	}
	p := &plan{snap: snap, root: rootID, items: map[string]*planItem{}, busy: snap.env.ActiveOperationID != "", activeOp: snap.env.ActiveOperationID}
	ids := make([]string, 0, len(universe))
	for id := range universe {
		if id != rootID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return snap.records[ids[i]].Instance.Identity.Descriptor < snap.records[ids[j]].Instance.Identity.Descriptor
	})
	p.order = append([]string{rootID}, ids...)
	for _, id := range p.order {
		rec := snap.records[id]
		it := &planItem{rec: rec}
		external := false
		for _, e := range snap.edges {
			if e.ProviderID != id || !e.Holds() {
				continue
			}
			consumer, known := snap.records[e.ConsumerID]
			view := ConsumerView{Kind: KindResource, ResourceID: e.ConsumerID, Status: string(e.ConsumerStatus)}
			if known {
				view.Name = consumer.Instance.Identity.Descriptor
			} else {
				view.Name = "unknown"
			}
			it.consumers = append(it.consumers, view)
			if universe[e.ConsumerID] && id != rootID {
				it.requiredBy = append(it.requiredBy, e.ConsumerID)
			} else {
				external = true
			}
		}
		workload := snap.workloadConsumers(rec)
		if len(workload) > 0 {
			external = true
			it.consumers = append(it.consumers, workload...)
		}
		it.consumers = dedupeConsumers(it.consumers)
		sort.Strings(it.requiredBy)
		it.reasons = snap.staticReasons(rec, it.consumers, external)
		p.items[id] = it
	}
	return p, nil
}

// rootRefusal returns the allowlisted refusal of an ineligible root, or "".
func (p *plan) rootRefusal() string {
	if p.busy {
		return ReasonEnvironmentBusy
	}
	if r := p.items[p.root].reasons; len(r) > 0 {
		return r[0]
	}
	return ""
}

// eligibility is the snapshot decision of the root for the detail view.
func (p *plan) eligibility() Eligibility {
	reasons := append([]string{}, p.items[p.root].reasons...)
	if p.busy {
		reasons = append([]string{ReasonEnvironmentBusy}, reasons...)
	}
	return Eligibility{Eligible: len(reasons) == 0, Reasons: reasons}
}

// validate checks a selected set against the plan as a whole. Nothing is
// selected, added or removed on the caller's behalf: a structurally wrong set
// is ErrInvalid, a set that breaks a rule is a RefusedError, and either rejects
// the entire request.
func (p *plan) validate(selected []string) error {
	chosen := map[string]bool{}
	for _, id := range selected {
		if id == "" || chosen[id] {
			return invalid("selectedResourceIds")
		}
		chosen[id] = true
	}
	if !chosen[p.root] {
		return invalid("selectedResourceIds")
	}
	for id := range chosen {
		if _, ok := p.items[id]; !ok {
			return invalid("selectedResourceIds")
		}
	}
	if r := p.rootRefusal(); r != "" {
		return &RefusedError{Reason: r}
	}
	for id := range chosen {
		it := p.items[id]
		if len(it.reasons) > 0 {
			return &RefusedError{Reason: it.reasons[0]}
		}
		for _, consumer := range it.requiredBy {
			if !chosen[consumer] {
				return &RefusedError{Reason: ReasonHasConsumers}
			}
		}
	}
	return nil
}

// tokenBody is everything a preview pins. The token is a hash of it, so any
// change of any pinned item, of the Environment, the current Set, the drafts
// or the actor makes a replay of the token stale.
type tokenBody struct {
	Version     int    `json:"v"`
	Actor       string `json:"actor"`
	Org         string `json:"org"`
	Application string `json:"application"`
	Environment string `json:"environment"`
	EnvID       string `json:"envId"`
	Generation  int64  `json:"generation"`
	EnvVersion  int64  `json:"envVersion"`
	DraftVer    int64  `json:"draftVersion"`
	SetID       string `json:"setId"`
	Busy        bool   `json:"busy"`
	Root        string `json:"root"`
	// Expires is the issue deadline: it is an input of the digest, so a token
	// whose expiry was rewritten no longer matches the state it was issued for.
	Expires int64 `json:"expires"`
	// Nonce makes every preview token unique, so two previews of the same state are
	// two requests and a replay is always the same token.
	Nonce string      `json:"nonce"`
	Items []tokenItem `json:"items"`
}

type tokenItem struct {
	ID         string         `json:"id"`
	Descriptor string         `json:"descriptor"`
	Status     string         `json:"status"`
	Version    int64          `json:"version"`
	StateKey   string         `json:"stateKey"`
	Module     string         `json:"module"`
	ActiveID   string         `json:"activeId"`
	ActiveStat string         `json:"activeStatus"`
	ActiveVer  int64          `json:"activeVersion"`
	Attempts   int            `json:"attempts"`
	Reasons    []string       `json:"reasons"`
	RequiredBy []string       `json:"requiredBy"`
	Consumers  []ConsumerView `json:"consumers"`
	// Target is a one-way digest of the immutable original target reference;
	// the reference itself never enters the token or any response.
	Target string `json:"target"`
}

func (p *plan) tokenHash(actor string, targets map[string]string, expires int64, nonce string) (string, error) {
	snap := p.snap
	body := tokenBody{Version: 1, Actor: actor, Org: snap.scope.OrganizationKey, Application: snap.scope.ApplicationKey, Environment: snap.scope.EnvironmentKey,
		EnvID: snap.env.ID, Generation: snap.records[p.root].Instance.Identity.TargetGeneration, EnvVersion: snap.env.Version, DraftVer: snap.env.DraftVersion,
		SetID: snap.env.CurrentDeploymentSetID, Busy: p.busy, Root: p.root, Expires: expires, Nonce: nonce}
	for _, id := range p.order {
		it := p.items[id]
		ti := tokenItem{ID: id, Descriptor: it.rec.Instance.Identity.Descriptor, Status: string(it.rec.Instance.Status), Version: it.rec.Instance.Version, StateKey: it.rec.Instance.StateKey,
			Module: it.rec.ModuleID + "@" + it.rec.ModuleRevision + "/" + it.rec.Instance.ModuleBundleID, ActiveID: it.rec.Instance.ActiveResourceID, Attempts: it.rec.Attempts,
			Reasons: it.reasons, RequiredBy: it.requiredBy, Consumers: it.consumers, Target: targets[id]}
		if a, ok := snap.activeOf(it.rec); ok {
			ti.ActiveStat, ti.ActiveVer = string(a.Status), a.Version
		}
		if ti.Reasons == nil {
			ti.Reasons = []string{}
		}
		if ti.RequiredBy == nil {
			ti.RequiredBy = []string{}
		}
		if ti.Consumers == nil {
			ti.Consumers = []ConsumerView{}
		}
		body.Items = append(body.Items, ti)
	}
	return canon.Hash(body)
}
