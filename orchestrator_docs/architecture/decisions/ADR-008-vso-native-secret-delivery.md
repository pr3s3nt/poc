---
id: ADR-008
artifact: architecture-decision
status: current
last_reviewed: 2026-09-28
---

# ADR-008 — Vault Secrets Operator delivery for UC-12 on Kubernetes

Status: Accepted
Date: 2026-09-28
Supersedes: ADR-006 decision 4 and 8 (Vault Agent file delivery)

Per-Environment store selection and ordinary Variable storage now follow
[ADR-012](ADR-012-environment-stores-and-transitions.md); scoped immutable secret
bundles and Kubernetes-native delivery remain required.

## Context

The Agent Injector produces a file which the workload image must source before
starting. This prevents ordinary images from consuming UC-12 variables and
secrets as environment variables. The kind Injector webhook also uses
`failurePolicy: Ignore`, so rollout readiness alone cannot prove injection.

## Decision

1. Vault remains the source of truth for UC-12 values. For each workload and
   pinned configuration revision, the backend writes an immutable bundle of
   only that workload's referenced keys to a scoped Vault KV v2 path.
2. Vault Secrets Operator (VSO) synchronizes that bundle to a revision-specific
   Kubernetes Secret in the workload namespace. The workload receives keys via
   `env.valueFrom.secretKeyRef`; no image startup script or Agent Injector is
   required. The VSO objects contain Vault paths and Secret names, never values.
3. Before applying the workload, execution must verify that the uniquely named
   destination Secret exists with all expected keys. Deployment success still
   requires workload readiness. A new UC-12 desired revision cannot change a
   running workload before explicit Preview → Deploy. Existing Secret revisions
   remain available while old Pods may still reference them.
4. VSO authentication is scoped to the Application, Environment, workload and
   referenced revision. A workload may receive only the keys it references.
   The provider interface remains replaceable per Application.
5. On kind, VSO is installed separately from the existing Vault releases. Do
   not uninstall the existing Injector or change unrelated workloads. No raw
   Secret value may enter GitOps manifests, Score, state snapshots, Preview or
   logs.

## Consequences

- Unlike ADR-006, secret bytes now exist in namespace-local Kubernetes Secret
  objects and etcd. Production use requires encryption at rest, least-privilege
  RBAC and a policy for Secret retention/garbage collection.
- Environment variables are fixed for a running process. A changed UC-12 value
  needs a new workload revision and Pod rollout; VSO automatic restart is not
  used for the MVP because it would bypass Preview → Deploy.
- The current per-value immutable Vault objects remain; the provider adds an
  immutable per-workload bundle for VSO, so existing stored values need not be
  migrated.

## References

- [ADR-006](ADR-006-application-configuration-provider.md)
- [VSO Vault source](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/vso/sources/vault)
- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
