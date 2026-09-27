---
id: VERIFICATION-UC12-VAULT-KIND-2026-09-26
artifact: verification-record
status: evidence
last_reviewed: 2026-09-26
---

# UC-12 persistent Vault install on kind — 2026-09-26

## Scope

Installed a second Vault release in the existing `kind-idp-internal` cluster,
namespace `vault`, without changing the pre-existing `vault` release or its
Agent Injector. Values: [kind Vault configuration](../../deploy/kind/vault-uc12-values.yaml).

## Observations

- `helm install vault-uc12 hashicorp/vault --version 0.34.0` returned deployed.
- `vault-uc12-0` Pod reached `Running`; its readiness was false while sealed.
- PVC `data-vault-uc12-0` was `Bound`, 1 GiB, StorageClass `standard`.
- Service `vault-uc12` was created in namespace `vault`.
- `vault status -format=json` reported `storage_type: file`,
  `initialized: false`, `sealed: true` (exit code 2 is expected here).
- One Injector mutating webhook remained: `vault-agent-injector-cfg`.
- No unseal keys or root token were generated; no workload injection was run.

## Follow-up

Initialize/unseal only after operator key custody is arranged, then configure
KV v2 and scoped auth/roles before testing an injected workload. This record
does not establish that UC-12 API, adapter or Preview → Deploy is working.
