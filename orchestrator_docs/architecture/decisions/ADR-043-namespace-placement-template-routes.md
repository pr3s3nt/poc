---
id: ADR-043
artifact: architecture-decision
status: current
last_reviewed: 2026-11-17
related: [ADR-016, ADR-017, ADR-035, ADR-037, ADR-041]
---

# Namespace-owned placement and template-owned HTTPRoute

Approved scope: issue #84, user conversation 2026-11-16. This changes the
authoring and rendering boundaries of ADR-016/017/037; it does not authorize
changes to shared Gateway, Traefik, DNS, TLS or cloud resources.

## Placement

A Kubernetes Terraform provider may reference a namespace resource with
`{target: resource, resource_ref: k8s-namespace.default#environments.@app.@env}`.
The namespace itself binds explicitly to a cluster. The consumer depends on
the namespace, inherits its cluster account/version and pins the resolved
cluster for invocation. Its namespace variable must reference that same
namespace's `name` output; literal or conflicting namespace inputs fail before
execution. Namespace creation and deletion order follows the resource graph.
No credential or connection is obtained from a namespace name alone.

Existing explicit cluster bindings remain readable/executable with their
original state and pins. They are not silently rewritten. AWS database
Definitions retain account/region/network dependencies; no namespace is imposed
on them. No automatic cross-cluster move, multi-cluster networking or state reset.

## Existing Gateway configuration

User refinement (2026-11-16): Gateway is a distinct `k8s-gateway` Resource Type
with its own Definition, not a map in the cluster Definition.
The Gateway Definition holds a whole cluster target reference, Gateway
name/namespace/listener and nonsecret domain/scheme/reserved-name policy. It
references the shared preexisting Gateway and never creates, adopts, updates or
destroys it. Cluster Definitions retain their existing identity-only contract.

An endpoint Definition references the Gateway resource's nonsecret `gateway`
output. The graph is endpoint → Gateway → cluster, with workload → namespace →
cluster. Planning resolves and pins the selected Gateway Definition policy and
requires the same cluster as the consuming workload's namespace. The pre-render target verifier reads Gateway/listener readiness and allowed
namespace without mutating shared infrastructure. Multiple Gateway Definitions permit distinct listeners
or domains without duplicating Gateway objects.

Existing literal endpoint Definitions remain explicit legacy configurations;
new Gateway-bound configurations cannot fall back to another cluster or a legacy
policy when missing. Developer inputs stay name, Service port and path.

## Rendering

The platform resolves and reserves hostname/path, supplies a nonsecret endpoint
intent in the neutral template input, and validates the rendered route against
that intent. The versioned Git template constructs HTTPRoute. It must use the
actual rendered Service and numeric port, selected Gateway/listener, hostname,
prefix path, workload namespace and platform ownership labels. Missing,
ambiguous, duplicate or contradictory routes/Services fail before publication.
Gateway, Ingress and unrelated routes are not an alternative endpoint publisher.

Template support is explicit, versioned capability metadata. Older immutable
template/endpoint revisions retain the legacy backend-rendering path during
transition; the new path never silently generates a missing route in Go.
New capability/pins require explicit inspection and selection. Existing applied
workloads, renderer pins, state and inventories are not mutated by registration.

HTTPRoute joins normal workload inventory and CD publication; updates/removal
retain ownership/UID guards. Hostname/path claims remain platform-owned and are
released only after acknowledged replacement/pruning or removal. No template
access to reservations, credentials, raw secrets or provider clients.

## Verification and rollout

Check matching, graph order, inherited target pins, conflicting namespaces,
Gateway/workload cluster agreement, rendered route intent and unchanged cloud
bindings. Real acceptance uses the existing kind Gateway and new task-owned
Application/namespace/Definitions, exercising Score and native workload authoring,
Preview/Deploy, update/restart/reuse and owned cleanup. Run full checks on kind
and distinguish existing main failures from candidate regressions. Never infer
live availability from template inspection or an HTML shell response.

Implementation and live evidence: [issue #84 handoff](../../implementation/namespace-template-routes.md).

## Approved refinement — 2026-11-17

The user approved separating Gateway metadata from Kubernetes verification. New
Gateway Definitions use the generic `echo` executor: `values.variables` contains
exactly `gateway`, an object holding the cluster reference and existing policy
fields above. The executor returns expanded inputs unchanged as outputs; it has
no provider client, manifests, physical instance ID, target or destroy operation.
Public Echo registration in this slice serves only the Gateway contract; other
Resource Types and credentials/secrets are not opened by this amendment.

Catalog/planning validate the Gateway Type/output contract, no developer inputs
or account, supported metadata driver, policy and whole cluster reference, rather
than requiring only the `existing-gateway` name. Graph/pins and same-cluster rules
remain mandatory. Gateway ownership remains PREEXISTING for both supported
representations, without reclassifying arbitrary Echo resources. Cluster identity
and current account version remain verified before metadata execution; the Echo
executor receives neither Connection nor execution Target. Existing active
Gateway provenance is preserved across an explicitly selected Definition upgrade.

Before render/CD publication the Kubernetes target verifier checks readiness,
listener/HTTPRoute support, hostname and namespace admission. Metadata success
alone never authorizes publishing. Legacy `existing-gateway` Definitions keep
their flat input layout and adapter, including stored pins and live read checks;
no revision is rewritten or silently migrated. The Console defaults new Gateway
Definitions to Echo and preserves the stored driver/layout on legacy edits.

Existing clusters retain their dedicated `existing-cluster` adapter and Driver
Account. Review/reuse both planning/pin and real API readiness checks; cluster
authentication is not delegated to Echo. Namespace inheritance, cloud database
Definitions, Git-owned HTTPRoute and cleanup guards are unchanged.
