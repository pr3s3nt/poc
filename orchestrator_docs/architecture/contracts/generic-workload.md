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

## GW-06 — Container lifecycle and volumes (approved 2026-11-05)

All containers retain the same named `containers` map, bindings and resource
limits. Optional `role` is `app` (also the omitted default), `init`, or `sidecar`.
Init/sidecar declare a unique positive `initOrder` (1..999); app has no order.
At least one app is required. Init containers run to completion in order; native
sidecars occupy that same ordered init sequence with `restartPolicy: Always`.
Regular init containers cannot declare probes. Names are unique across roles.
Changing a role must not change its env bindings, image, resource or mount intent.
Native sidecar execution needs a Kubernetes target supporting that feature; the
platform never silently converts it to an ordinary long-running container.
See [Kubernetes sidecars](https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/).

Existing `workloadExecution.mounts` continues to describe retained managed PVC
resource references. New `volumes[]` defines Pod-local named sources: exactly one
of `emptyDir: {medium?: "Memory", sizeLimit?: quantity}`,
`hostPath: {path: absoluteCleanLinuxPath, type?: ""|"Directory"|"DirectoryOrCreate"|"File"|"FileOrCreate"|"Socket"|"CharDevice"|"BlockDevice"}`, or
`configMap: {files: [{name: fileName, value: literalNonsecretText}], defaultMode?: 256|288|292}`.
Default ConfigMap mode is 0444 (292). ConfigMap data is explicitly nonsecret;
values are literal, never resolved as secret/resource expressions. ConfigMaps are
owned namespaced workload inventory. `volumeMounts[]` has volume, container,
absolute path, readOnly and optional clean relative subPath. ConfigMap mounts
are read-only; file subPaths must name an existing data key. emptyDir supports
sharing the same volume among containers; its lifetime is the Pod, unlike PVC.
PVC mounts accept optional clean relative subPath too. All roles can mount all
five source kinds. Host paths use the scheduled node filesystem, not the
Console client filesystem; no provisioning, retention, deletion, placement or
permission changes are inferred. Source paths must be absolute, clean, bounded
to 1024 bytes and exclude NUL, CR/LF and backslashes. The selected node/cluster
policy owns path existence, permissions and admission. Form defaults are
Directory and read-only mounting; explicit writable intent is preserved. See
[Kubernetes hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath).
Reject duplicate/overlapping/protected paths, missing sources,
invalid sizes, duplicate names and writable ConfigMap/Secret mounts. Secret files
keep the existing scoped UC-12 reference/version delivery; never store bytes in
the workload, ConfigMap, Preview or logs. File mounts remain revision-based,
not a promise of automatic hot reload.

Custom authoring may omit the execution template pin; partial pins are rejected.
Such intent has version 1, an explicit kind and no template inputs. The matched
Definition/source still owns and pins rendering; no renderer is auto-selected.
Prepared sources advertise bounded `workloadCapabilities`: `custom-execution-v1`,
`container-lifecycle-v1`, `pod-volumes-v1`, `container-probes-v1`,
`host-path-volumes-v1` (required additionally for hostPath). Save validates syntax/preset pins;
Preview requires the exact selected source to advertise each requested capability
before provisioning, and rendering guards verify lifecycle, order, source data,
mounts and approved Secret projection without dropping intent. Existing preset
pins and their input validation remain mandatory when present. Standard source
advertises these capabilities; old published sources require explicit source
publication/selection, never silent mutation of existing pins.

## GW-07 — Complete basic runtime authoring

Container ports (name/containerPort/optional TCP, UDP or SCTP protocol), Service
protocol, optional zero Deployment replicas and Workload metadata annotations
are editable and round-trip in the form. Annotations remain template inputs,
not arbitrary Pod annotations. Duplicate annotation/port names are errors.

Probes retain the historical HTTP path/port shorthand. Optional `type` adds
`http`, `tcp` and `exec`; exec uses command argv, TCP uses port, HTTP uses path,
port and optional HTTP/HTTPS scheme. Exactly one action is represented.
Optional initialDelaySeconds >= 0, timeoutSeconds/periodSeconds/failureThreshold/
successThreshold >= 1 override source defaults. Liveness/startup successThreshold
must be one. Regular init containers have no probes. Templates must preserve
explicit action/timing; validate before Save/Preview and check again at render.

Non-HTTP probes or explicit probe timing/scheme require `container-probes-v1`
on the selected source before provisioning. Historical HTTP shorthand is unchanged.

## GW-08 — Complete runtime configuration and defaults

[Workload defaults contract](workload-defaults.md) defines optional security,
controller policy, StatefulSet claims and immutable policy selection, preserving
legacy documents and desired/applied round-trip semantics.

[ADR-039](../decisions/ADR-039-template-owned-workload-defaults.md) removes
`workloadExecution.defaults`; verified template-only profiles are frozen in
`workloadExecution.templateDefaults`. Unknown/removed policy fields are rejected.

## GW-09 — Authored variable order (issue #63)

A container may declare optional `variableOrder: string[]`: the display order the
author gave its `variables`. Entries are distinct names declared in `variables`;
names it omits follow in sorted order. It is authoring metadata stored with the
draft and the Deployment Set module, so a deployed workload reopens in the same
order. Rendering, bindings and manifests ignore it and keep their sorted order.
The editor writes it only when the order differs from the sorted order, so an
unchanged older workload keeps the same document. Import/export keep it as written.
