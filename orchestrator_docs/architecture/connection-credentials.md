---
id: CONNECTION-CREDENTIAL-DESIGN
artifact: shared-design
status: current
last_reviewed: 2026-10-06
---

# Connection credentials and execution identity

## Authority and delivery

Behavior is owned by [UC-04 specification](../usecase/UC-04/specification.md).
This shared design separates infrastructure destination from authentication.
Kubernetes upload is the first delivery; AWS provisioning remains deferred under
[ADR-009](decisions/ADR-009-aws-access-key-storage.md). An AWS Connection supplies
identity for VPC/EKS/Aurora/Terraform state, not merely an EKS login.

## Connection model

Common fields: Organization, stable key, display name, kind, authentication type,
non-secret config, opaque secret reference, status and verification metadata.
Kinds stay `KUBERNETES` / `AWS`. Authentication types:

| Type | Kind | Resolution |
|---|---|---|
| `HOST_CONTEXT` | Kubernetes legacy | Existing host context, explicit compatibility path |
| `KUBECONFIG` | Kubernetes new | Normalized selected-context config from scoped credential store |
| `AWS_ACCESS_KEY` | AWS future | Access key pair in Vault according to ADR-009 |

Legacy missing display name reads as key; missing auth type is inferred only for
known legacy metadata/reference shape. Unknown type/reference fails closed.
AWS seeded records are legacy process configuration, not proof of new AWS
credential flow. No generic arbitrary command authentication provider exists.

Kubernetes config contains cluster/context/endpoint (and non-secret TLS metadata
if needed). AWS config contains verified account, allowed regions and backend
configuration. Secret byte values belong to the credential store only.
Schema changes and compatibility defaults: [schema](database/schema.md).

## Dedicated credential lifecycle boundary

Introduce an outbound port under `ports`, distinct from UC-12 Provider and
UC-08 output-secret store. Contract has Put/Get/Delete with explicit
Organization, Connection key, and immutable object reference. Validate complete
reference scope before any remote read/delete. Reject path traversal, arbitrary
URLs and cross-Organization/Connection refs. Values never have a public read API.

Durable adapter uses Vault KV v2, namespace
`orchestrator/connections/<org>/<connection>/credentials/<object-id>`.
Use create-only CAS; delete attempt metadata/versions on rollback so failed
registration retains no recoverable credential. Separate this from
`orchestrator/apps/...` and workload ACLs. Use dedicated scoped-token file
configuration (`-connection-vault-token-file`); do not print it or persist the token in product state.
An explicit local/test in-memory adapter supports deterministic tests; it is
not durable and must not be selected silently for real Kubernetes/persistent
use. Without a configured durable store, new registration fails safely rather
than saving a READY record whose secret will disappear on process restart.

Bootstrap selects `-connection-credential-store none|vault|memory` (default
`none`); Vault uses dedicated `-connection-vault-address`,
`-connection-vault-token-file` and `-connection-vault-mount` flags, independent
of UC-12 configuration. `memory` is explicit fake/no-persistent-state only.
Bootstrap injects the same store/resolver into registration and all adapters.
Existing UC-12 configuration provider remains independently wired.

## Kubeconfig boundary

Use structured YAML/JSON parsing with duplicate keys/references rejected;
bounded request/file size (1 MiB document, JSON envelope limit allowing escaping).
Only supported API shape, valid references, non-empty token or certificate+key,
embedded CA/TLS configuration and HTTP(S) server URL can be accepted. URL must
not contain userinfo credential. Preserve supplied TLS verification choices;
never silently disable TLS. Detect external CA/client cert/key files, tokenFile,
auth-provider and exec in selected material; never execute uploaded commands.
Inspect exposes safe names/endpoints only; register revalidates independently.
Normalize to a single selected context, cluster and user; drop unrelated entries
and extension data before storage or kubectl. Raw parser/provider stderr must
not be returned or logged. Credential input is cleared after success and must
be masked/hidden in review recordings with real material.

## Adapter execution and persistence boundary

Opaque execution identity includes Organization and Connection key/reference;
no credential bytes, temporary kubeconfig paths or AWS env enter Target JSON,
ActiveResource outputs/state, Preview or plans. At each kubectl call, resolver
creates a private mode-0600 file and supplies its path via scoped invocation;
cleanup runs on success/error/cancellation. Retain scope identity through
existing-cluster resource output, dependency target resolution and deployment
Target restoration. All namespace/provision/deploy/readiness/remove/Ingress/VSO
calls must obey this identity. Fail closed on missing secret; never use host
credentials for KUBECONFIG references. Legacy context targets continue to work.

Future AWS resolver yields account credential only to the selected Terraform
subprocess environment, never global env, variables, arguments or workload.
Existing Terraform local-state/seed behavior is not reclassified as durable AWS
onboarding. Additional AWS methods can implement this boundary without changing
Definition references or Connection ownership.

## Diagram

[Credential collaborators](domain/design-class-diagram.puml) are part of the
shared classes; [Connection ERD](database/erd.puml) contains scalar identity.

## Consumption and compatibility choices

UC-01 Environment Settings selects a READY Connection once, without default
fallback. Each Environment owns profile/region and credential identity (ADR-011).
A new Connection also requires registering an
`existing-cluster` Resource Definition with its Connection key and matching
criteria for the desired Application/Environment (UC-03). Existing seed
Definition remains untouched. For uploaded records, the selected context in
Connection is authoritative; Definition kubeContext/name driver values cannot
redirect the credential to another context or host. For legacy records the
existing input contract is preserved. Reused ActiveResource restores opaque
identity from its persisted Connection key, not transient Target output alone.
Lookup failure/not-READY must fail before execution, not silently become an
empty Connection. New target identity survives cached resources and workload
TargetRef for removal/route reconciliation after restart.

Fleet GitRepo delivery is scoped to the fixed cluster by ADR-007. It must fail
closed if a credential-backed target is not supported, never silently use its
configured seed context. Expanding Fleet multi-cluster delivery is not in this
change. Direct delivery supports new Connection targets.

Endpoint selection supports user-supplied cluster URLs because internal and
local cluster addresses are valid product targets. Network access controls are
operator responsibility; this change must reject URL userinfo and unsafe
schemes and must not introduce redirects to unvalidated credential destinations.
Kubeconfig parser validation must sanitize attacker-controlled names/endpoints
so inspection cannot echo credential fields disguised as metadata in errors.

## Workload secret-store registration and selection

[ADR-012](decisions/ADR-012-environment-stores-and-transitions.md) adds separate
SecretStoreConnection metadata and Environment selection. Vault access tokens are
stored in the existing platform credential vault under Organization/store/attempt
scope, never recursively inside the workload's selected store. Resolver validates
scope/provider/READY and returns private client credentials; public choices exclude
refs/tokens. VSO bundle metadata pins store addresses/mount/auth and store-specific
object names; old deployments do not follow a changed Settings selection.
