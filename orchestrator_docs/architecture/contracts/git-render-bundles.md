---
id: GIT-RENDER-BUNDLE-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-28
---

# Git render bundles — version 1 contract

Required design under [ADR-021](../decisions/ADR-021-git-render-bundles.md).
Implementation status is tracked separately in IMP-035. Infrastructure execution
and legacy installed bundles keep their existing contracts. This document owns
the Git source, render engine and API shapes; use cases own observable behavior.

## RB-C01 — Directory and manifest

A directory contains all render-policy files. No remote includes, submodules,
symlinks, executable files or files outside that directory. Extra examples/docs
are allowed but count toward the digest and size limits. Required `bundle.yaml`:

```yaml
apiVersion: orchestrator.dev/render-bundle/v1
kind: RenderBundle
metadata:
  name: standard-workload
renderer:
  type: score-k8s
  version: 0.15.0
inputContract: orchestrator.dev/render-input/v1
inputsSchema: inputs.schema.json
defaults: defaults.yaml
normalize: normalize/score.tpl
provisioners:
  - provisioners/bindings.provisioners.yaml
patches:
  - patches/deployment.tpl
  - patches/service.tpl
```

All listed fields required; arrays may be empty if no corresponding stage is
needed. `metadata.name` uses the existing catalog ID syntax. Reject unknown
fields, duplicate YAML keys, unsupported API/engine versions and missing paths.
`defaults.yaml` is an object of non-secret policy constants read by templates;
it is not merged into the submitted Score. `normalize` is exactly one entrypoint.
Provisioner/patch arrays establish priority/order; never infer order from the
filesystem. Same file cannot occur twice in an array. No arbitrary command field.

Publish the usable reference at `backend/render-bundles/standard/` (new code
artifact in the implementation task). Its defaults retain current 10m/32Mi CPU/
memory fallbacks and replicas behavior. Provide a README and render fixtures
there. Runtime must consume the selected Git cache, not silently read this
checkout's reference directory or embed it into every execution.

## RB-C02 — Inspection, pinning and cache

PE/Admin may inspect public/credential-free HTTPS Git sources without a per-repo
allowlist, following the hardened Terraform Git downloader. Reuse transport/path/
ref validation, bounded fetch and redacted errors; do not reuse HCL parsing,
Terraform release identity or Terraform state. SSH/private Git authentication is
outside v1. Reject credential-bearing URLs, unsafe protocols/redirects and hooks.

Empty path becomes `.`. Otherwise accept clean relative segments; reject traversal,
absolute paths and encoded bypasses. Ref accepts branch/tag/full commit, never
option/refspec syntax. Resolve to a full 40/64 lowercase hex commit, fetch exact
commit without executing repository code, and hash the complete selected tree.
Use the Terraform registry's canonical sorted relative-path/file-bytes digest
algorithm; do not invent a second digest convention. Changes to examples also
change this digest. Re-inspecting a moving branch creates a new immutable pin.

Limits: inspect body 64 KiB; 120 s overall inspect deadline; selected tree at
most 256 files/4 MiB, individual file at most 1 MiB; reject before unbounded
allocation. Reuse downloader's stricter repository fetch limit. Inspect checks
schema, template syntax/functions and provisioner restrictions without providers,
Kubernetes calls or executing author templates. Persist verified metadata only
after atomic durable cache materialization and digest verification. It does not
create a Definition or imply a successful deployment.

Source pin includes safe origin URL, selected directory, full commit, SHA-256
content digest, manifest/input contract versions, score-k8s version and verified
binary digest, and platform engine contract version (`git-render-v1`). Bundle ID
is a server-issued UUID; metadata name is display only. Multiple Organizations
get distinct records; physical digest cache may be shared internally. Inspecting
the same pin is idempotent. Different contents for an identical source/commit/path
or immutable contract are rejected; no UPDATE in place.

Configure durable cache through `ORCHESTRATOR_RENDER_BUNDLE_CACHE_DIR`; container
default `/var/lib/orchestrator/render-bundle-cache`, own volume and UID 65532
permissions like the module cache. No cache paths in API responses. Recheck
digest/engine compatibility on use. Missing/corrupt content fails closed; explicit
re-inspection of the same source pin may restore it. No automatic Git fetch in
planner, Preview, admission or execution. No automatic bundle garbage collection
in v1; preserve pins used by Definitions/plans and restart history.

## RB-C03 — Definition and inputs

Keep `driverType: score-k8s`, `resourceType: workload`, current matching,
`provision`, namespace and cluster semantics. Extend only workload variables:

```json
{
  "render_bundle": "<verified bundle UUID>",
  "namespace": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}",
  "cluster": "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}",
  "render_inputs": {"team": "payments", "runAsNonRoot": true}
}
```

`render_inputs` is optional (default `{}`), non-secret, literal JSON. No new
resource/context references or dependency expansion inside it. Schema is a closed
JSON Schema object subset: `type`, `properties`, `required`, `additionalProperties`,
`items`, `enum`, `default`, `title`, `description`, `minimum`, `maximum`, `minLength`,
`maxLength`, `pattern`, `minItems`, `maxItems`. Types string/boolean/integer/number/
object/array; all object schemas require `additionalProperties: false`. No `$ref`,
remote resolution, combinators, null or secret/password inputs. Unsupported
keywords fail inspect rather than being silently ignored. Pattern uses Go RE2.
Maximum schema depth 8 and serialized effective inputs 64 KiB.

Apply schema defaults only to absent keys, never to explicit false/0/empty string;
then validate types/bounds/unknown keys, with no coercion. Arrays replace, objects
recurse. Persist supplied inputs; pin effective inputs and their canonical hash
in the plan. Changing source requires revalidation; preserve old draft keyed by
bundle pin and show incompatible fields rather than silently dropping them.
Installed legacy bundles accept no nonempty `render_inputs`.

## RB-C04 — Engine stages and neutral input

Render pipeline is fixed; authors cannot reorder the stages:

1. Platform resolves existing dependencies and configuration through unchanged
   services. A secret bridge prepares approved namespace-local Secret material
   using existing delivery code. **No raw SecretEnv bytes enter author templates,
   score-k8s, cache, review artifacts or logs.** Preparing Secret objects privately
   must not call native Deployment/Service rendering.
2. Construct JSON-only `Input` v1: `workload` (deep copy of product module),
   `bindings` (per-container plain values and Secret name/key references),
   `target` (namespace only), `identity` (workload/application/environment IDs,
   required labels and selectors), `delivery` (approved pull-Secret reference,
   ServiceAccount and required Vault Agent metadata), `services` (complete
   Candidate Set Service names/ports), and `unknowns` (path/provenance entries
   for Preview). Do not expose credentials, clients, Go methods, cache paths,
   raw provider outputs or mutable stores. Resource aliases are already resolved
   by the existing planner/binding resolver.
3. Evaluate normalization template with root `.Input`, `.Inputs` (effective
   typed parameters), `.Defaults` (policy constants). It returns **one JSON object
   containing a valid Score document**, not Kubernetes objects. Validate supported
   Score syntax/identity. This step owns container/probe shape conversion,
   fallback/default choices, and packaging resolved bindings as Score resources.
4. Initialize a private score-k8s workspace, disable defaults, install listed
   provisioners, generate raw Deployment/Service with the pinned binary. Provisioner
   file priority is manifest array order, first matching entry wins. Materialize
   numbered local files or reverse CLI installation order as required by 0.15.0;
   tests must prove this, not assume CLI repeated-flag order. Provisioners are
   unchanged native score-k8s `template://` YAML with `type`, optional `class/id`,
   description and `outputs`. Forbid command, state mutation and emitted resources;
   only output bridging is supported. Upstream context (`.Params` etc.) and
   `encodeSecretRef` semantics are pinned by score-k8s version.
5. Apply `patches` sequentially using the platform's deterministic template engine.
   Context adds `.Normalized` (normalized Score) and `.Manifests` (current generated
   object array) to step 3. Each template returns a YAML/JSON array of score-k8s
   compatible **`set`/`delete` path operations**, applied before the next template.
   Empty array is a no-op. These are not RFC6902 Deployment Delta patches. Reuse
   or port the pinned score-k8s patch semantics with conformance tests. This step
   deliberately runs after CLI generation to provide explicit typed inputs;
   it is not an invented upstream `--values` option. Templates may range manifests
   by kind/name; absent Service is a no-op. No array-root insertion/deletion or
   creation of additional objects in v1.
6. Validate resulting objects against semantic invariants below, join only
   approved private secret-delivery auxiliaries, and return unchanged manifests
   to existing review/digest/apply/readiness. Remove workspace on all exits.

Normalization and patch template engine: Go `text/template`, `missingkey=error`,
standard built-ins plus exactly `dict`, `list`, `append`, `get`, `hasKey`, `set`,
`unset`, `merge`, `deepCopy`, `keys`, `sortAlpha`, `toJson`, `fromJson`, `quote`,
`default`, `required`, `fail`, `escapeScore`. Deterministic functions only; no env,
file, network, shell, clock, random or template includes outside the parsed bundle.
`default` means absent/nil only, never false/0/empty string. `merge` recursively
overlays later objects, replaces arrays, and never mutates inputs; sort map keys
when emitting ordered arrays. `escapeScore` escapes literal `$` exactly once.
`required` rejects absent/nil only; explicit zero/false/empty are schema decisions.
Other helper semantics follow their conventional Go/Sprig counterparts with
errors returned rather than panics. Version these helpers with `git-render-v1`.

Execute author templates in a killable Go worker subprocess of the same product
binary with bounded JSON stdin/stdout and no inherited credentials; no extra
language/runtime. Whole render deadline 30 s (Preview 10 s), total generated output
8 MiB, input 4 MiB, at most 128 patches/file. Kill/reap child on cancel/timeout;
do not rely on a leaked goroutine to enforce template execution time. Public
errors contain stage/code and safe relative filename, not template values/output.

## RB-C05 — Policy freedom and invariant validation

Git rendering must not call `kubernetes.NewRenderer().Render` and compare its
entire result to author output. That legacy path may remain for installed pins.
The reference Git bundle carries current normalization and policy explicitly.
Pure semantic validation enforces:

- Exactly one `apps/v1 Deployment` named for the workload in the resolved namespace;
  exactly one `v1 Service` iff Score declares service ports. Names/kinds/namespace
  and protected identity labels/selectors must match supplied identity. No cluster
  object, arbitrary Secret, extra container, sidecar or initContainer.
- Preserve explicit Score image, command/args, ports, variables and probe meaning.
  For CPU/memory each explicit request wins, then its same-field limit, then
  the bundle default. Explicit limits remain exact; do not invent limits. Bundle
  may change missing-value fallbacks; reference defaults remain 10m/32Mi. Explicit
  replicas remain exact; a missing value uses the bundle default. Persisted Score
  and Candidate Set remain unmodified.
- Authors may add non-reserved labels/annotations, scheduling fields (nodeSelector,
  affinity, tolerations), securityContext, rollout strategy and missing-value
  defaults. Reject conflicts with explicit Score intent or identity rather than
  quietly overwriting them after templates. Kubernetes API validation still runs.
- Allowed environment bindings and Secret/pull/service-account references must
  equal those supplied by the platform. No arbitrary `envFrom`, new volume Secret
  references, token mount, command credential access or changed Vault injection.
  Shared secret delivery owns required auxiliaries and Secret bytes. Bundle files
  own formatting of permitted references/required labels; validators enforce
  their values, not a full Go-rendered policy object.
- No Namespace/PVC/database/cloud objects. Terraform output-only bindings do not
  select/provision infrastructure. PublicRouteManager still owns Environment
  Ingress. Existing Deployment readiness and workload object removal apply.

## RB-C06 — Preview, admission and history

Planner reads verified metadata from the scoped snapshot without Git/render I/O.
Plan pins Definition version/hash, source pin, engine/binary, effective inputs and
placement. Pending compares against each workload's applied plan; bundle or input
changes alone produce UPDATE, not a no-op or infrastructure replacement.

After closing the snapshot, Preview executes the same cached pipeline with safe
symbolic bindings. Carry current unknown-path provenance/redaction into generated
manifest review; do not expose symbolic stand-ins as real values. Fields whose
values depend on unresolved inputs stay unknown; if a template cannot evaluate
symbolically, report a clearly unavailable artifact rather than inventing output.
Extend the existing safe manifest projection to show accepted scheduling/security/
rollout fields and non-secret bundle annotations/defaults, preserving size/redaction
limits. No unrestricted template stdout or Secret object projection.

Before any UC-08 action, validate pin/cache/engine/effective inputs and existing
admission versions. Changed Definition/input invalidates old Preview. Once admitted,
execution uses the immutable pin, never rereads branch HEAD or mutable Definition.
Render only after required outputs; persist sanitized manifest review before Apply.
Refresh/restart retains source pin, review, and applied before-side. Re-selecting
an older verified pin is an explicit Definition edit followed by Preview/Deploy;
it re-renders against current inputs/outputs, not a replay of old Secret bytes.

## RB-C07 — HTTP operations and errors

All paths below are under `/api/v1`, authenticated Organization scoped. PE/Admin
only for catalog/inspect/Definition mutations; Developer consumes safe provenance
through existing Preview/Deployment APIs. Existing Definition CRUD/version-CAS
endpoints, criteria and matching preview remain unchanged.

| Operation | Request / result |
|---|---|
| `POST /render-bundles/inspect` | `{url,path,ref}` → `200 {renderBundle: <metadata>}`; inspection only |
| `GET /render-bundles` | Existing `{renderBundles: [...]}` extended with same-org verified Git entries; existing installed entries retained |
| `GET /render-bundles/{id}` | One same-org verified Git metadata record; foreign/missing ID 404; no source body/cache path |
| Definition create/update | Existing payload with `values.variables.render_bundle` UUID and optional `render_inputs`; validate prepared pin and schema; existing CAS transaction |

Git metadata has `id`, `name`, `version` (short commit display), `status: AVAILABLE`,
`source: {kind: git,url,path,revision,contentSha256}`, `inputContract`,
`engineContract`, `renderer: {type,version}`, `inputsSchema`. Status becomes
`UNAVAILABLE` in safe catalog projection when cache/runtime validation fails;
record remains immutable/verified. Binary digest stays internal to persisted pin.
Installed records retain the legacy `id/version/status` shape; frontend accepts
both. Definition edit hydrates saved source/inputs without reinspection.

Strict JSON and request-size checks; no client digest/binary/contract overrides.
Errors follow existing T20 envelope `{code,field,params,correlationId}` (plus
existing envelope fields); safe fields only. New codes: `RENDER_SOURCE_INVALID`
(400), `RENDER_BUNDLE_INVALID` (400), `RENDER_INPUT_INVALID` (400),
`RENDER_SOURCE_UNAVAILABLE` (503), `RENDER_BUNDLE_UNAVAILABLE` (503),
`RENDER_BUNDLE_DRIFT` (409), `RENDER_CONTRACT_UNSUPPORTED` (400),
`RENDER_FAILED` (422). Preserve existing auth, size, CAS/stale and COMMIT_UNCERTAIN
codes. Inspect response never auto-saves; unknown mutation outcome requires read/
review, not replay. Invalid legacy/Git source combinations are rejected explicitly.

## RB-C08 — Open object set (ADR-024)

[ADR-024](../decisions/ADR-024-open-workload-objects.md) replaces the object and
field rules of RB-C04 step 5 and RB-C05 for Git bundles. Unchanged: RB-C01..C03,
the worker/helper/limit rules of RB-C04, Secret handling and RB-C06/C07.

Manifest: optional `resources` array of template paths (same rules as
`patches`; omitted means empty). Each template sees `.Input`, `.Inputs`,
`.Defaults`, `.Normalized` and `.Manifests` and returns a YAML/JSON array of
Kubernetes objects (`[]` is a no-op). Pipeline order: normalize → score-k8s
generate → `resources` (appended in array order) → `patches` → validate. At most
128 objects per workload. `.Input.workload.annotations` holds Score
`metadata.annotations` (string map; at most 64 keys, Kubernetes annotation key
syntax, 64 KiB total).

Validation after patches:

- **Primary workload** `<workload>`: `apps/v1` Deployment or StatefulSet in the
  target namespace; platform identity labels on metadata and pod template;
  `selector.matchLabels` equals the platform selector; each Score container
  keeps image, command/args, probes, explicit CPU/memory requests/limits
  (missing request falls back to the same-field limit, then the bundle) and
  exactly the approved value/Secret reference for every approved variable.
  Extra containers, environment entries, limits and pod fields are allowed.
  Explicit Score replicas are kept. Platform image pull Secret is present; when
  Vault Agent delivery applies, its ServiceAccount and annotations are kept.
- **Primary Service** `<workload>`: present iff the Score declares ports;
  platform selector; includes every Score port (name, port, target, protocol).
- **Other objects**: namespaced name (DNS subdomain), namespace absent or the
  target namespace (the platform sets it), unique `kind/name`; not `v1 Secret`,
  `v1 Namespace`, any `secrets.hashicorp.com` kind or a known cluster-scoped kind.
- **Every object**: reserved labels (`orchestrator.io/*`,
  `app.kubernetes.io/managed-by`) and annotations (`orchestrator.io/*`,
  `vault.hashicorp.com/*`) are platform-owned. The platform then sets
  `app.kubernetes.io/managed-by=orchestrator`, `orchestrator.io/application`,
  `orchestrator.io/environment`, `orchestrator.io/deployment-id` and
  `orchestrator.io/workload` on each object's metadata.

## RB-C09 — Inventory, apply order and removal (ADR-024)

- **Discovery check:** before any write the deployer resolves each
  `apiVersion/kind` through API discovery; unknown or cluster-scoped kinds fail
  the deploy (`RENDER_FAILED` class at apply, no partial write).
- **Ownership check:** each non-Secret object must be absent or carry
  `managed-by=orchestrator`, the same application and environment and the same
  workload (`orchestrator.io/workload`, or `app.kubernetes.io/name` for objects
  applied before ADR-024).
- **Inventory:** `workload_instances.target_ref.objects` is a list of
  `{apiVersion, kind, name, owners}` (owners = deployment IDs whose labels may be
  on the object). Platform auxiliaries (`<workload>-env` Secret, VSO objects)
  are not inventory; they keep their own removal. Before apply the inventory is
  the union of the previous and new sets; after readiness, objects only in the
  previous set are deleted (UID precondition, ownership verified) and the
  inventory becomes the new set.
- **Order/readiness:** with Jobs, apply non-rollout objects, wait for every Job
  `Complete` (`Failed` fails), then apply rollout objects (Deployment,
  StatefulSet, DaemonSet) and wait for rollout status. Without Jobs one apply.
- **Removal:** delete every inventory object (ownership + UID), then the fixed
  Deployment/Service/`-env` Secret removal; absent objects are success.
- **Review:** Deployment/StatefulSet and Services keep the safe projection;
  other kinds show metadata and a sanitized `spec` with environment values,
  ConfigMap data and Secret-like fields redacted.

Public routes (`-public-route-kind httproute`): one owned
`gateway.networking.k8s.io/v1 HTTPRoute orch-public` per Environment namespace,
`parentRefs` from `-public-gateway namespace/name[/sectionName]`, `hostnames`
= Environment host, one `PathPrefix` rule per Score public route with the
Service name and numeric port. An owned object of the other route kind is
removed when the kind changes. See [public routes](../../operations/public-routes.md).

## ZIP upload — ADR-025

`POST /api/v1/render-bundles/upload` accepts multipart `file` (exactly one ZIP),
`path` (optional, default `.`).
Unknown or repeated fields are rejected. Return `200 {renderBundle: ...}` using
existing metadata projection; source identity and archive bounds follow
[ADR-025](../decisions/ADR-025-zip-source-upload.md). Malformed requests return
INVALID_REQUEST, oversized requests REQUEST_TOO_LARGE (413); invalid content
uses existing module/RENDER_BUNDLE_INVALID classifications. ZIP uses the same
verified cache, inspector, input schema and execution guards as Git. No mutable
ref or Git URL is required. Rendering engine `git-render-v1` is unchanged for ZIP.
