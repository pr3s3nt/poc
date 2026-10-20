---
id: ADR-017
artifact: architecture-decision
status: current
last_reviewed: 2026-10-18
related: [ADR-014, ADR-015, ADR-016, UC-03, UC-05, UC-06, UC-08]
---

# ADR-017 — Definition dependencies and deferred cluster targets

## Authority and delivery boundary

Issue #4 and the user's 2026-10-18 V04–V06 instruction authorize the replacement
of profile-driven execution by Definition/matching/references. This decision
extends ADR-016 from existing clusters to clusters created in the same execution.
It preserves ADR-014 state, immutable artifacts, fencing, secrets and ownership.
V04, V05 and V06 remain separate acceptance/removal/commit gates. Multi-cluster
execution and PE-authored workload patches (issue #5) remain outside scope.
V06.06 requires planner verification that no unrequested EKS is inserted.
Its no-workload runtime verification is conditional on an already defined public
entry/contract. No such entry is defined; adding one is outside this delivery.
This is the issue condition applied, not a newly approved product feature/decision.

## Graph and references

Use the existing resource-output reference grammar and explicit Kubernetes
provider binding `target: resource, resource_ref: <descriptor>`. A provider
binding selects the cluster and adds consumer→cluster ordering; it is not a
second account selector. A whole typed namespace/cluster output reference in
workload inputs denotes placement and adds its graph edge. Other supported
references, including nested/interpolated resource params, add their existing
edges. Output declarations alone add none. Do not introduce another dependency
field to duplicate these relationships or derive graph edges from Terraform
module-internal `depends_on`.

Expand selected workload and resource Definitions to a deterministic fixed point,
including late consumers; deduplicate by scoped descriptor and reject conflicting
class/params/scope, missing outputs and cycles over the full graph before calls.
UC-06 BR-08 flags retain their meanings: an empty rule adds a node but invents
no ordering edge; `is_dependent` adds child→parent; `match_dependents` adds every
parent consumer→child. A selection-only relation must not be shown as an ordering
edge. Full graph scheduling cannot erase workload-provider dependencies.

## Account and target binding

Every AWS Terraform node uses its winning Definition's READY Organization AWS
Driver Account and literal Driver Inputs region, with invocation-private resolved
credentials. Verification regions are probe evidence, not deployment restrictions.
No Environment identity/profile/region, host default chain or alternate winner may
supply infrastructure or a target. Different resource accounts/regions are not
rejected just because they differ; actual references/network/provider contracts
must still be valid. The final V06.08 removal also deletes the now-unused
Organization default-choice marker belonging to the retired selector; it is
not an account source at any gate. Organization ownership of Driver Accounts
and their scoped verification/credentials remain unchanged. Existing-cluster Definitions select Internal accounts.

Workload and namespace bind one explicit cluster descriptor. The cluster winner
may be an existing-cluster driver or verified Terraform EKS module; consumer
placement pins the cluster Definition/account/version and namespace identity
before output values exist. Preview represents unresolved runtime outputs with
provenance and their declared types, not fabricated endpoint/Ready values.
One resolved cluster per Candidate Set remains the current capability boundary,
with placement stored per workload/resource for future expansion.

After successful EKS provision, the adapter supplies typed `cluster` output with
safe cluster identity, Organization, original AWS account/version, region and
`provider: eks`, tied to the exact producing descriptor. Internal targets retain
the existing kubeconfig-account representation. Discriminated validation must
reject mixed Internal/EKS forms, unknown fields, missing identity/region and
credential/path-bearing values. Public Preview/status project only safe metadata;
no kubeconfig, token, private secret reference or endpoint credential is output.
EKS identity is reconstructed into the existing opaque execution Target at each
invocation; short-lived authentication derives only from its original scoped AWS
account. Refresh auth per invocation, never persist tokens or temporary files.

EKS ACTIVE alone is insufficient. Before consumers, verify the resolved cluster's
API authentication/authorization, allowed executor network access and usable
Ready node capacity. Failures block namespace/workload and remain truthful in UI.
Managed db/cache depend on the referenced network inputs they actually require;
they do not acquire an EKS edge simply because they share an Environment.

## Runtime, persistence and cleanup

Outputs are type/required/secret checked before consumers. Persist original
Definition/module/account/region/cluster/namespace/state/ownership pins and safe
outputs or opaque secret references. Reopen/redeploy preserves logical resource
identity and data; catalog edits never retarget an existing state implicitly.
No duplicate cluster/database/cache after restart/reuse. Missing state, uncertain
outcome or ownership mismatch holds rather than recreating/adopting resources.

Remove workload delivery before reverse-graph resource cleanup through original
module/state/target. A shared resource with remaining consumers stays; preexisting
cluster and unrelated objects stay. Cleanup requires ownership/UID and service-API
absence checks, not only Terraform success or tag-index absence. Keep recovery
material until clean is independently proven. Errors retain failed evidence.

## Replacement gates

Acceptance-scope override19Oct2026: browser main flow only, with secret and
ownership/UID/native-API cleanup checks; intentional negative scenarios and
video/human pacing are no longer gate requirements. Existing negative automated
tests remain required. Negative/video/writer wording below preserves the original
gate provenance; current execution follows the user decision in issue4 and
[the acceptance runbook](../../operations/terraform-v2-acceptance.md).

V04 proves the entire acceptance app on existing k8s-4f with real shared PostgreSQL
and Redis, browser jobs, persistence, cache TTL/refill and truthful negatives.
V05 proves EKS created by the authored graph in the same execution, readiness,
app lifecycle, negatives and API-proven cleanup before V05.10 removes AWS profile
fallback. V06 runs managed A02 then clean then A03, validates isolation/reuse and
only then applies V06.08 removal of executionProfile/API/UI/seed/runtime remnants.
Writer prepares separate removal patches; Codex applies only after the relevant
replacement gate. Historical evidence remains explicitly historical. Final source
must pass impacted checks/runtime after removals before milestone commit/push.
