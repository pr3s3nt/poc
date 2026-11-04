---
id: GENERIC-WORKLOAD-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-11-04
---

# Generic Workload and Template Driver

## GW-01 — Workload

The versioned Generic Workload contract is independent of Kubernetes manifests.
Its canonical expanded form has apiVersion `orchestrator.dev/workload/v1`, metadata.name,
class/annotations, containers, replicas, optional Service, resource declarations
and execution intent. Resource aliases, shared IDs and output expressions reuse
the existing dependency model. Authoring, import, persistence, Preview and Deploy
must round-trip the same normalized model without dropping advanced fields.
Single-container shorthand image/env/ports normalizes to this model on admission.
Absent replicas differs from explicit zero. Secret bytes never enter the document.

## GW-02 — Definition and sources

`resourceType: workload`, `driverType: template` uses the existing Definition CRUD,
criteria, immutable version publication, explicit Environment revision selection
and companion resolution. Driver configuration names a verified template source
and typed inputs. Inline/Git/ZIP/Catalog prepare the same immutable source contract.
External mutable refs resolve at preparation; rendering never fetches a branch.
Template source metadata is Organization scoped. Referenced files must be bounded,
relative, duplicate-free and executable-free. Template sources own manifest
templates, input schema/defaults and optional authoring capabilities.

## GW-03 — Render

Render receives JSON-only workload intent, effective Definition inputs, resolved
dependency bindings, target and authorized delivery references. Explicit workload
intent wins over defaults. Templates emit bounded namespaced object arrays;
Service presence follows explicit desired Service intent. Rendering uses the
existing closed Go helper set in a child worker with bounded time/input/output
and no inherited credentials. No shell, network, dynamic plugin or Score stage.
Validate image/replica/port/env/probe/storage/runtime intent, reserved identity,
namespace, permitted Secret references and object identity before delivery.

## GW-04 — Revision lifecycle

Preview pins workload snapshot, Definition revision/hash, template source digest,
input hash and dependency identities/versions. Unknown outputs remain symbolic.
Deploy verifies pins before provisioning and records actual nonsecret values and
the resulting artifact/digest after outputs resolve. A source/Definition/dependency
change requires a new Preview/revision; old snapshots cannot be silently repinned.
Secret snapshots contain references/versions, never bytes. Existing delivery
artifacts, stale-token admission, inventory and cleanup remain authoritative.

## GW-05 — Removal and regression boundary

Remove Score-specific contracts/adapters, score-k8s runtime, Render Bundle APIs
and the separate Rendering Templates page. Remove only obsolete experimental
workload configuration; never erase Terraform Definitions, accounts, connections
or unrelated domain/state identities. Schema changes follow ADR-027. Terraform
and CD/Fleet interfaces keep their existing behavior.

## Example and template author interface

```yaml
name: order-api
image: registry.example.com/order-api:1.0
replicas: 2
ports:
  - name: http
    containerPort: 8080
service:
  enabled: true
  port: 80
env:
  LOG_LEVEL: info
```

Admission expands this shorthand to the same multi-container document used by
Console authoring. It does not persist Kubernetes objects in the Workload.
A prepared source's `bundle.yaml` names `resources: [deployment.tpl, service.tpl]`,
input schema/defaults and the versioned engine/input contracts. Each resources
file emits a YAML/JSON array of objects, or empty output for a conditional resource.
`.Workload` exposes the normalized ModuleSpec, `.Input` the complete authorized
render context, `.Inputs` effective Definition inputs, `.Resources` resolved
nonsecret references and `.Defaults` source defaults. Optional normalization and
ordered patches reuse the bounded worker; they do not execute a generator.

For example, a Definition's `resource_references.namespaceName` may contain
`${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}`.
Its template reads `.Resources.namespaceName`. This uses existing descriptors,
graph dependencies and bindings; it does not introduce another reference syntax.
The renderer validates explicit Workload intent before handing manifests to CD.
