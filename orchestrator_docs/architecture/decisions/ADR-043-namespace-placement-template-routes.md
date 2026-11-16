---
id: ADR-043
artifact: architecture-decision
status: current
last_reviewed: 2026-11-16
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

Gateway policy is nonsecret configuration attached to the cluster Definition,
keyed by a stable Gateway configuration name. Each entry contains the endpoint
policy fields (domain, scheme, Gateway name/namespace/listener and reserved names).
An endpoint Definition selects that cluster and configuration name. Planning
pins the selected cluster Definition revision and resolved policy, and requires
the same cluster as the consuming workload's namespace. Matching criteria alone
are not evidence that two targets agree. Multiple policies per cluster allow
separate public/private listeners and domains without putting routing policy on
the namespace or requiring a new Gateway to be provisioned per application.

Existing literal endpoint Definitions remain explicit legacy configurations;
new cluster-bound configurations cannot fall back to another cluster or a legacy
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
