---
id: DELIVERY-RECONCILIATION-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-28
related: [ADR-027, UC-01, UC-04, UC-05, UC-06, UC-07, UC-09, UC-18]
---

# Optional workload CD and reconciliation

Required design, not an available API. Behavior belongs to the linked use-case
specifications; persistence belongs to [schema](../database/schema.md#delivery-and-drift--adr-027-required-design).
Architecture: [ADR-027](../decisions/ADR-027-delivery-reconciliation.md).

## CDR-01 — Selection, identity and permissions

Environment `desiredDelivery` and `appliedDelivery` are separate:

```json
{"mode":"DIRECT","bindingId":null,"bindingVersion":null,"selfHeal":false}
```

`mode=CD` requires a verified Organization-owned binding/version. Binding contains
provider kind, adapter contract version, capability digest, management-account
identity, provider-specific validated configuration, publication destination and
target registration. Binding versions are immutable; rename/reverify creates a
new version without rewriting existing applied receipts. Credentials resolve
privately through stable account/credential refs at each call; credential rotation
does not replace original cluster identity. Deleted/revoked credentials fail closed.

PE/Admin creates/verifies bindings, changes delivery selection and self-heal
policy. Developers can read safe choices/status and deploy the already selected
binding through existing application permissions. This is a delivery selection,
not a reintroduction of an Environment deployment Connection/profile.

Selection change bumps Environment version and delivery configuration revision.
With no deployed inventory, next Deploy applies it. Otherwise Preview classifies
`DELIVERY_TRANSITION` even if Score is unchanged; unrelated pending changes must
be deployed/undone first. Cancel before Deploy discards desired selection without
runtime calls. Disabling CD on an occupied Environment also requires transition.
Changing policy only uses the same admission/revision guards; it is not a move
of ownership and does not rerun Jobs or change Pod templates.

## CDR-02 — Provider-neutral ports

Proposed Go signatures (typed domain records, not runtime implementation):

```go
type DeliveryProvider interface {
    Describe(ctx context.Context, binding BindingVersion) (Capabilities, error)
    Verify(ctx context.Context, binding BindingVersion) (Verification, error)
    Prepare(ctx context.Context, req PrepareRequest) (PreparedDelivery, error)
    Submit(ctx context.Context, intent DeliveryIntent) (Receipt, error)
    Observe(ctx context.Context, ref DeliveryRef) (Observation, error)
    Reconcile(ctx context.Context, req ReconcileRequest) (Receipt, error)
    Withdraw(ctx context.Context, req WithdrawRequest) (WithdrawalProof, error)
}
type GitPublisher interface {
    Publish(ctx context.Context, intent PublicationIntent) (PublicationReceipt, error)
    ObservePublication(ctx context.Context, key PublicationKey) (PublicationReceipt, error)
}
type OwnershipTransfer interface {
    PrepareTransfer(ctx context.Context, req TransferRequest) (TransferPlan, error)
    Release(ctx context.Context, req TransferRequest) (WithdrawalProof, error)
    Adopt(ctx context.Context, req TransferRequest) (Receipt, error)
}
```

Registry selects adapter only at composition boundary. `Prepare` is local,
deterministic packaging/validation after rendering; `Verify` is an explicit
read-only external operation, never called by pure Preview. `Submit`/`Reconcile`
take operation ID, fence, intent ID and expected revision; replay is idempotent.
`Withdraw` stops management and retains objects; it never means “delete by name”.
Shared `OwnedObjectRemover` deletes the confirmed inventory with UID preconditions
after proof. Direct adapter delegates current Kubernetes apply/readiness/removal;
it returns no autonomous self-heal capability. Its observation compares only
platform-owned desired fields, excluding status/defaulted/controller-owned fields.

Capabilities are versioned structured data: accepted object/lifecycle classes,
exact revision observation, continuous drift/self-heal, delegated fields,
phased delivery, retain-on-withdraw, guarded prune, ownership export/adoption,
and reconcile action semantics (`RESYNC` or unsupported). Unknown capability
is unsupported. Do not coerce unsupported operations into direct execution.
Argo/Flux may implement publication differently; the core does not require Git
for every possible adapter. Raw provider status and arbitrary JSON patches are
private adapter data, not public error messages or lifecycle decisions.

## CDR-03 — Approved intent, receipts and publication

Identity: Organization + Environment + target generation + delivery epoch + unit.
An epoch changes only on ownership transfer, never Terraform target generation.
Units: workload phases and Environment routes. Immutable revision identifies the
admitted deployment, normalized payload digest, binding version, policy and
render pins. Transport Git commit and Fleet deployment ID are separate receipts.

Before any publish, persist artifact plus intent under Environment owner/fence.
Artifact has full non-secret manifests, inventory, phase graph and dependency
pins; redacted review is not replayable. Secret bytes, credentials, VSO objects
and auxiliary Secret/Agent objects never enter Git. Existing secret bridge runs
against the actual scoped target before dependent units are submitted.

Use dedicated generated paths:
`org/<org-id>/env/<env-id>/generation/<n>/epoch/<n>/<unit-id>/`.
Only Orchestrator writes them; PE-authored render templates remain a different
repository/source. Git path traversal, symlinks, external generators, unexpected
files and embedded transport credentials are refused. Artifact digests include
provider-generated lifecycle metadata so review/provenance can explain it.

Git publisher uses a durable lease per repository/branch plus ordinary remote
fast-forward compare-and-swap. Persist base commit, intended tree digest, operation
marker and resulting commit. On conflict, refetch and recompute only owned paths;
never force-push. On lost acknowledgement, look up marker and exact tree, not
blindly commit again. Bounded three publication retries, then actionable conflict.
Never stage foreign paths or print Git stderr/URLs containing credentials.

Fleet receives an explicit commit pin for each unit GitRepo. Pushing unrelated
paths or manually changing the branch cannot activate an unapproved revision.
Before updating the provider pointer, verify current owner/fence and expected
provider resourceVersion. Lost database acknowledgement after provider submission
is `OUTCOME_UNKNOWN`: observe exact persisted intent/handle before any replay.

Database fencing cannot revoke an already issued Git/Kubernetes request. Never
take over a mutating intent from lease expiry alone: confirm the old worker and
outstanding calls have terminated and observe the provider before changing owner.
For pointer updates use conditional resourceVersion writes; for Git use remote
fast-forward checks. These guards prevent a delayed stale writer from choosing an
older revision after a new owner has proceeded; unresolved external writes retain
the Environment hold. Read-only observer leases may expire independently.

During deployment, the admitted pending revision is authoritative for that unit;
last-successful is retained for history only. After failure, preserve pending
receipt and union inventory, mark attention, require explicit retry/review.
Background workers observe but never choose the old revision as a rollback.
No current-Set advance until all required phases/readiness/cleanup pass, preserving
the existing per-workload pending-batch commit semantics and route-only retry.

## CDR-04 — Observation and reconciliation state

| Dimension | Values / meaning |
|---|---|
| Lifecycle | `PREPARED`, `PUBLISHING`, `SUBMITTED`, `VERIFYING`, `ACTIVE`, `WITHDRAWING`, `REMOVED`, `FAILED`, `NEEDS_ATTENTION` |
| Sync | `IN_SYNC`, `OUT_OF_SYNC`, `PENDING`, `UNKNOWN`, `NOT_SUPPORTED` |
| Health | `HEALTHY`, `PROGRESSING`, `DEGRADED`, `UNKNOWN`, `NOT_APPLICABLE` |
| Drift | `NONE`, `DETECTED`, `UNKNOWN`, `NOT_SUPPORTED` |
| Policy | `DEPLOY_ONLY` or `SELF_HEAL`; desired policy and acknowledged policy distinct |

Observation includes intended/observed revision, observed generation, time,
safe reason code, object summaries, bounded field paths and completeness.
`IN_SYNC` requires exact unit payload/revision and complete relevant observation;
Git commit seen or ready Pods alone are insufficient. Disconnection/auth failure
is `UNKNOWN`, not `NONE`. Health can be HEALTHY while drift is DETECTED.
Staleness after three configured poll intervals makes effective status UNKNOWN;
retain timestamp/last-known result. Historical Deployment status stays immutable.

Default observation interval 30 seconds with jitter; retry transient reads with
bounded backoff up to five minutes. Use per-target concurrency limit 2 and global
limit 8, configurable. Workers use leased rows/fences; stale writes are rejected.
No cluster/network call inside DB transactions. Status GET reads stored data only.
Reconcile request is an explicit idempotent operation on the already admitted
revision, never render latest templates, fetch newer source or consume draft data.

Autonomous self-heal is executed by the CD controller against its pinned revision.
It can continue during Orchestrator outage; changing DB policy alone cannot stop
it. Stop/change requires provider acknowledgement. If a controller cannot prove
withdrawal, no competing writer or destructive cleanup is enabled.

## CDR-05 — Object lifecycle and ownership

| Object class | Handling |
|---|---|
| Deployment/StatefulSet/DaemonSet and continuous config | CD-managed after owned-target preflight; readiness for every rollout object |
| Job | Separate execution unit, self-heal off; completion receipt keyed by UID and spec digest; immutable update rejected, no force replace |
| CronJob | Reconcile parent spec only; child Jobs/Pods are controller-owned, not Git inventory; no “catch up” or manual run action added |
| HPA | Owns replicas within declared bounds; adapter must prove ignore **and** apply semantics; explicit fixed Score replicas conflicting with HPA is rejected |
| PVC | Owned inventory and guarded delete; no forced replacement/self-heal recreation that could change data identity |
| Operator CR | API discovery plus adapter capability; unknown readiness/side-effect safety means observe-only or reject requested self-heal, never assume safe |
| Terraform namespace/database/cache; VSO/Agent/Secret | Remain outside CD inventory and under their existing managers |

Preserve ADR-024 stage order: non-rollout prerequisites → all Jobs complete →
rollout objects Ready → public routes. Split units so a Job update does not rerun
because a continuous ConfigMap drifts. A completed Job deleted by TTL is recorded
as expected absence, not recreated; first-delivery TTL cleanup before completion
was observed gives UNKNOWN. Reject TTL configuration that cannot be supported by
the pinned adapter's completion tracking. No Helm hooks, force recreation or
automatic rollback is allowed in generated packages.

Delete/prune: capture expected UID and labels from acknowledged owned inventory;
remove desired ownership from provider and prove it cannot recreate the object;
then GET/check labels and delete with UID precondition. Missing is idempotent;
foreign/replaced/unproven is NEEDS_ATTENTION. Update keeps union inventory until
removed objects are absent. Provider selection never adopts an object merely
because its name matches. New UID after self-heal requires a new observed creation
receipt attributable to the exact owned release/intent; ambiguous UID remains
protected, never overwrite the previous proof with a fresh GET.

## CDR-06 — Fleet adapter v1

Target compatibility baseline: Fleet 0.16 API, with exact controller, agent and
embedded Helm patch versions/digests recorded by verification and acceptance.
Documentation is a capability reference, not proof of installed behavior.

Binding config: scoped management Kubernetes account, workspace namespace,
registered Cluster name **and UID**, exact destination identity verified via the
target account, Git repository/branch/root, opaque writer credential ref,
preinstalled Fleet read-credential Secret name, deployment service account and
timeouts. Internal Kubernetes and already registered EKS are supported targets;
the same namespace name or context name does not prove cluster equivalence.

The platform operator preinstalls Fleet/agent, scoped credentials and service
account/RBAC. The adapter creates only owned unit GitRepos in the configured
workspace. Each selects exactly the verified Cluster, pins a commit and path,
uses a stable unique Helm release name and an explicit target namespace.
Read credentials and writer credentials are separate. No host context/helper,
Harbor-only restriction, default fleet-local assumption or wildcard target.

Generated controls: `helm.takeOwnership=false`, `helm.force=false`,
`helm.atomic=false`, `correctDrift.force=false`, `deleteNamespace=false`;
`correctDrift.enabled` only for approved continuous self-heal units.
Use `helm.waitForJobs` in execution units, native target readiness plus Fleet
BundleDeployment observation. No arbitrary Helm valuesFrom/remote charts/plugins.
Preserve literal manifest data through Fleet's Helm conversion; verify braces,
ConfigMap payload and explicit Score fields in conformance tests.

`GitRepo.status.commit` is transport evidence. Correlate owned Bundle and exact
target BundleDeployment, its spec/applied deployment IDs and observed generations,
then verify live revision/inventory/readiness. Missing mapping is UNKNOWN.
Do not use maximum aggregate readiness across unrelated bundles as success.

For UID-safe cleanup, emit `helm.sh/resource-policy: keep` on inventory objects
and retain-on-withdraw controls. Successful withdrawal must remove the controller's
ability to manage/recreate these objects and its owned release/control metadata,
while leaving workload objects for the shared remover. `keepResources=true`
alone is not proof that a retained Helm release has been retired. Verify actual
uninstall/release behavior of the pinned implementation; if necessary perform a
scoped retained uninstall after controller withdrawal. Never delete broad Helm
Secret selectors. Update prune requires proof that the old object is absent from
the acknowledged new release manifest before UID removal. Prune is coordinated
cleanup, not an unrestricted native Fleet deletion option.

Provider metadata (GitRepo/BundleDeployment/release storage) receives its own
owned UID inventory so namespace cleanup can account for it. Keep controller
installation, registrations, read-credential Secrets and service accounts that
were preexisting. Withdrawal failure blocks namespace/Terraform cleanup.

Official references: [Fleet configuration](https://fleet.rancher.io/reference/ref-fleet-yaml),
[CRDs](https://fleet.rancher.io/reference/ref-crds),
[GitRepo](https://fleet.rancher.io/reference/ref-gitrepo),
[diff behavior](https://fleet.rancher.io/how-tos-for-users/bundle-diffs),
[Helm retained resources](https://helm.sh/docs/howto/charts_tips_and_tricks/).
The retained-withdrawal protocol above is this project's design, not a claim that
Fleet supplies a native UID-guarded delete API.

## CDR-07 — Delivery transition

Preview pins applied/desired selections, Environment/current Set/config/draft
versions, all unit receipts/inventory digests and target identity. Transition
does not change Terraform state identity, secrets, manifests' business fields
or target generation. All admitted objects including routes must be covered.
Missing historical exact artifacts/UID proof yields `DELIVERY_BASELINE_REQUIRED`;
do a normal direct redeploy to establish baseline first, never infer from live.

Stages: `CHECKING → RELEASING → ADOPTING → VERIFYING → COMMITTING`.
Acquire Environment operation and delivery epoch fence; no normal deploy/scan
remediation/cleanup/config write interleaves. Revalidate live ownership and absence
of foreign Helm ownership before any handoff.

Direct → Fleet: direct has no background writer, but fence its commands. For
each owned object, patch only Helm ownership metadata using UID/resourceVersion
tests and the planned release name/namespace; keep business spec and Pod template
unchanged. Import retained artifacts into Fleet with self-heal off. Do not use
takeOwnership. Observe exact inventory/revision; completed Jobs are imported as
completion records and excluded from execution, missing completed Jobs stay absent.
Enable requested continuous policy only after all units are verified.

Fleet → direct: withdraw every source unit with retain proof; establish no active
controller/Helm operation remains; remove only the source's ownership metadata
with UID/resourceVersion tests. Register direct receipts for the same artifacts,
verify health and ownership, then commit applied selection/epoch. No force apply,
Pod restart, re-provision or data-copy is implied by changing delivery manager.

Persist per-unit source release and destination adoption checkpoints. A timeout
or lost ack can leave a mixture of owners: keep the whole Environment held and
show NEEDS_ATTENTION with unit progress. Explicit Retry observes checkpoints and
continues forward without rerunning Jobs. No automatic rollback/release on lease
expiry. A successful transition commits all applied selection metadata together;
the existing workloads keep their Deployment history. Future provider transitions
must implement the same export/adoption protocol or return UNSUPPORTED.

## CDR-08 — Public API

All routes are under the existing authenticated `/api/v1` boundary. Environment
prefix `E=/applications/{app}/environments/{env}`; Organization comes from session.
Foreign IDs return 404. Mutations use strict JSON, CSRF/session rules, role gates,
`requestId` idempotency and expected versions. IDs below are opaque, never credentials.

| Method/path | Request / result |
|---|---|
| GET `/delivery-providers` | Safe installed provider descriptors/capabilities; no unimplemented Argo/Flux choices |
| POST `/delivery-bindings` | PE/Admin config plus private credential inputs; stores immutable version, returns safe metadata, initially UNVERIFIED |
| POST `/delivery-bindings/{id}/verify` | PE/Admin expectedVersion; read-only verify, safe facts, READY/REJECTED and time |
| GET `/delivery-bindings` | PE/Admin full safe metadata; Developer choices only eligible ID/name/provider/version/capabilities |
| POST `/delivery-bindings/{id}/versions` | PE/Admin immutable revised config; old versions retained for historical consumers |
| GET `E/delivery` | desired/applied, configurationRevision, choices, pending change, unit summary and operation |
| PUT `E/delivery` | PE/Admin expectedEnvironmentVersion, expectedConfigurationRevision, mode, bindingId/version, selfHeal; saves intent, no external writes |
| POST existing `E/preview` and `E/deploy` | Extend existing pending handlers, not parallel deployment APIs; selection/transition included in token |
| GET `E/reconciliation` | Stored summaries with cursor/limit, default 50/max 200; target-filtered units and infra observations |
| POST `E/delivery/reconcile` | Existing app deploy role; expected revision, unit IDs, requestId; 202 operation, current approved payload only |
| POST `E/delivery/transitions/{operationId}/retry` | PE/Admin expected revision/requestId, source outcome proven; 202 forward recovery |

Secret writer inputs are write-only and stored via the existing secret-store
boundary, never returned by GET, diff or operation history. Binding changes do
not repoint applied records. Deletion of a referenced version is refused; logical
disable stops new selections but retains original refs for cleanup.

Successful PUT returns 200 with authoritative revisions; async commands return
202 `{operationId,status}`. Repeated requestId/same body returns the same result;
different body 409. 400 invalid config, 401 unauthenticated, 403 wrong role,
404 foreign/missing, 409 BUSY/STALE/OWNERSHIP_CONFLICT/BASELINE_REQUIRED,
422 UNSUPPORTED_CAPABILITY/TARGET_MISMATCH, 503 PROVIDER_UNAVAILABLE.
Safe codes distinguish Git publish conflict, controller disconnected, pending
revision, ownership conflict and unknown outcome. Never return provider stderr.

## CDR-09 — Extension acceptance

A fake second provider must run the same core orchestration tests with different
provider handles and publication mechanics. No planner edits, provider type
assertions, string switches in deployment/pending services or Fleet statuses in
HTTP DTOs. New provider work is adapter + descriptor/configuration validator +
bootstrap registration + conformance/live tests; optional configuration fields
may need a UI editor, not a new deployment flow.
