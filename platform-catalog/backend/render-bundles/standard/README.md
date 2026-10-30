# Standard workload render bundle

Reference Git render bundle (ADR-021, contract
[`git-render-bundles.md`](https://github.com/pr3s3nt/poc/blob/155ccf46afbb3739ce6dd07497bf9e47dbe62e83/orchestrator_docs/architecture/contracts/git-render-bundles.md)).
It reproduces the platform's previous Kubernetes manifest policy with files only:
the same identity labels and selector, `IfNotPresent` pull policy, probe timing,
container ports on the first container, explicit Score resources and the
10m CPU / 32Mi memory fallbacks for missing requests, `replicas` 1 by default and
the platform's Secret, VSO and Vault Agent references.

The platform never reads this checkout at runtime. Publish a copy (or this
directory) in any credential-free HTTPS Git repository, then in the Console open
**Mẫu dựng ứng dụng → Thêm từ Git**, enter the URL, this directory and a tag,
branch or full commit, and press **Kiểm tra bundle**. A branch or tag is resolved
once to a full commit; a later commit is a new version you select explicitly.

## Files

| File | Stage | Purpose |
|---|---|---|
| `bundle.yaml` | manifest | Version 1 manifest. Arrays set priority/order. |
| `inputs.schema.json` | inputs | Typed literal `render_inputs` (closed JSON Schema subset). |
| `defaults.yaml` | defaults | Non-secret policy constants (`.Defaults`). Not merged into Score. |
| `normalize/score.tpl` | normalize | Neutral input → one Score JSON document. |
| `provisioners/bindings.provisioners.yaml` | provision | score-k8s `template://` output-only bridge of resolved bindings. |
| `patches/deployment.tpl`, `patches/service.tpl` | patch | Ordered score-k8s `set`/`delete` operations. |
| `examples/*/request.json`, `expected.yaml` | test | Executable examples; `TestStandardBundleExamples` renders and compares them. |

Every file in the directory counts toward the content digest and the limits
(256 files, 4 MiB, 1 MiB per file). Symlinks, executables, submodules and paths
outside the directory are rejected at inspection.

## Template data

Normalization sees `.Input`, `.Inputs` and `.Defaults`; patches also see
`.Normalized` (the Score document) and `.Manifests` (the current objects).

`.Input` (`orchestrator.dev/render-input/v1`):

- `workload`: the workload module (`spec.containers`, `spec.service`, `spec.replicas`),
  without raw Score variables.
- `bindings.<container>`: ordered list of `{name, kind: plain, value}` or
  `{name, kind: secret, secretName, secretKey}`. Secret bytes are never present.
- `target.namespace`, `identity` (`workload`, `application`, `environment`,
  `deployment`, `labels`, `selector`), `delivery` (`imagePullSecrets`,
  `serviceAccountName`, `podAnnotations` of Vault Agent), `services`
  (Candidate Set Service ports) and `unknowns` (Preview-only unresolved paths).

Templates use Go `text/template` with `missingkey=error` and only these helpers:
`dict list append get hasKey set unset merge deepCopy keys sortAlpha toJson fromJson
quote default required fail escapeScore`. `default` and `required` treat only an
absent/nil value as missing; `false`, `0` and `""` are values. `get` returns nil
for an absent key. `merge` overlays later objects into a new object. `escapeScore`
doubles `$` for Score substitution. There is no environment, file, network,
clock or randomness access; templates run in a killable worker with a 30 s
render deadline (10 s in Preview).

Patch output is a YAML/JSON array of `{op: set|delete, path, value}` with
score-k8s 0.15.0 (`sjson`) paths such as `0.spec.replicas` or
`0.metadata.labels.example\.com/team`. A path must start with an existing
manifest index; objects are never added or removed at the array root.

## What the platform still enforces

The bundle owns formatting and missing-value policy. After the patches the
platform validates (it never re-renders a hidden policy manifest):

- one `apps/v1` Deployment named for the workload in its namespace, one `v1`
  ClusterIP Service exactly when the Score declares service ports, nothing else;
- platform identity labels and selector, no reserved `orchestrator.io/*`,
  `app.kubernetes.io/name|managed-by` or `vault.hashicorp.com/*` changes;
- explicit Score image, command, args, probes, service ports and replicas;
  each CPU/memory request is the explicit request, else the same-field limit,
  else your default; limits are only the explicit ones;
- environment entries are exactly the approved values and Secret references;
  no `envFrom`, volumes, init/sidecar containers, host namespaces or privileged
  containers; image pull Secrets and the Agent ServiceAccount are platform-owned.

Allowed additions include non-reserved labels and annotations, `securityContext`,
`nodeSelector`, `affinity`, `tolerations`, `topologySpreadConstraints`, rollout
strategy fields and missing-value defaults. Kubernetes API validation still runs.

## Inputs of this bundle

| Input | Type | Default | Effect |
|---|---|---|---|
| `team` | string | none | `example.com/team` label on Deployment, Pods and Service |
| `defaultReplicas` | integer 1-20 | 1 | Replicas when the Score declares none |
| `runAsNonRoot` | boolean | false | Pod `securityContext.runAsNonRoot` |

Change `defaults.yaml` (for example `resources.requests.cpu`, `podAnnotations`
or `podSecurityContext`) to change policy without a platform release. Run
`UPDATE_RENDER_FIXTURES=1 go test ./internal/adapters/scorek8s -run Examples`
from `backend/` to refresh `examples/*/expected.yaml` after an intended change.

## PVC Deployment rollout

A Deployment with one or more `workloadExecution.mounts` uses
`spec.strategy: {type: Recreate}`. The patch replaces the whole strategy object,
so `rollingUpdate` settings are absent. Old Pods stop before new Pods start on
updates; downtime is accepted to avoid waiting for an RWO claim attached to a
different node. This is provider-independent and applies to read-only persistent
mounts too; the bundle does not receive claim access-mode metadata.

Deployments without persistent mounts (including secret-file-only mounts) keep
the existing rollout policy. StatefulSet and CronJob retain their own controller
semantics. No application source or Score schema change is required.

Publish/select a new verified Git bundle revision and Preview/Deploy explicitly
to apply this policy to a running workload. Editing this checkout or restarting
the backend does not change already pinned/cached bundle revisions.
