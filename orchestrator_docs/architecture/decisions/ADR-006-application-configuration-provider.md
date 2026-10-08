---
id: ADR-006
artifact: architecture-decision
status: current
last_reviewed: 2026-09-27
---

# ADR-006 — Per-Application configuration provider and Vault Agent delivery

Status: Accepted
Date: 2026-09-26

Vault Agent delivery in decision 4 and 8 is superseded by [ADR-008](ADR-008-vso-native-secret-delivery.md); the provider/revision decisions remain accepted.

Application-level provider selection, Variable storage and deferred provider migration
are superseded by [ADR-012](ADR-012-environment-stores-and-transitions.md).
Immutable revisions and secret-redaction requirements remain.

## Context

UC-12 manages Variables & Secrets for the two Environments of each Application.
An Application may use Vault while another uses a different provider. Saving a
value must not change the running workload before Preview and Deploy. The
current kind cluster already has a Vault Agent Injector, but its existing Vault
server uses in-memory storage and has no PVC.

## Decision

1. Each Application binds to one configuration provider through a provider
   interface. The interface is independent of workload/Score syntax and keeps
   the Application and Environment scope explicit. Provider selection is not a
   per-workload choice.
2. The first provider adapter uses Vault KV for both Variables and Secrets.
   Separate paths and read policies isolate Applications and their `staging`
   and `production` Environments. Application API reads may return Variable
   values but must never return Secret values after write.
3. Desired and applied configuration revisions are distinct. Saving a change
   writes a new immutable revision. Preview compares the pending revision with
   the applied one and identifies affected workloads. The Vault path referenced
   by a running Pod stays fixed until its workload is redeployed. A key rename
   or deletion never rewrites workload references automatically.
4. On Kubernetes, the existing Vault Agent Injector supplies the applied
   revision as files in the Pod. A workload startup script safely imports the
   rendered values into process environment variables and executes the
   application. Template output must be shell-safe; never interpolate arbitrary
   secret bytes as executable shell text. Rotation of process environment
   values requires a new Pod; the Injector alone cannot change an already
   running process environment.
5. For kind, install a separate persistent Vault release backed by a PVC on
   the cluster's `standard` local-path StorageClass. Keep the existing in-memory
   Vault untouched and reuse the installed Injector; a second webhook is not
   required. The local-path PVC survives Pod restarts but is not a backup or a
   production-grade HA store. Do not initialize Vault or persist unseal keys in
   the repository/Kubernetes without an explicit key-custody decision.
6. Application keys share one namespace within each Environment, including
   Variables and Secrets. UC-16 uses Score's `${resources.env.KEY}` reference
   with an `env` resource of type `environment`; the platform resolves the
   dynamic output and its secret classification from UC-12, without
   provisioning the virtual resource.
7. Preview pins both the UC-12 desired revision and UC-16 draft version.
   Deploy rejects stale previews. On a multi-workload failure the applied
   revision is tracked per workload; partial success is reported and remains
   retryable instead of pretending an atomic cluster-wide commit.
8. The Injector renders a workload-specific file with only the keys that
   workload uses. The platform must shell-escape its content; the workload's
   startup script sources it and then executes the application. Application
   key metadata and revision pointers use the existing state store in the MVP;
   secret values remain in Vault.

## Consequences

- Orchestrator stores provider binding, key metadata and revision pointers; it
  must not store raw secret values in its state snapshots, logs or preview.
- A backend write identity and a narrower per-workload read identity are
  required. Vault policies must limit workloads to their Application,
  Environment and applied revision.
- Deploy must pin the revision used by every affected workload and handle
  partial failure without exposing a pending revision to unrelated Pods.
- Provider migration remains deferred. The exact HTTP operations, persistence
  shape, startup-file format and recovery procedure are specified in dependent
  contracts before the feature is executable.

## References

- [Vault Agent Injector](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/injector)
- [Vault Helm modes](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/helm/run)
- [Vault in-memory storage](https://developer.hashicorp.com/vault/docs/configuration/storage/in-memory)
