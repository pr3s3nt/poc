---
id: VERIFICATION-UC12-VAULT-INIT-2026-09-26
artifact: verification-record
status: evidence
last_reviewed: 2026-09-26
---

# UC-12 Vault initialization and unseal — 2026-09-26

## Scope and safety

Target: existing `kind-idp-internal` cluster, `vault` namespace, new
`vault-uc12-0` Pod only. The original `vault` release and Injector were not
changed. Before initialization, `vault operator init -status` reported not
initialized and `data-vault-uc12-0` was Bound.

Initialization used one key share and threshold one for this local kind
instance. JSON output was redirected directly to a private file outside the
repository; neither the key nor root token was printed to agent output. The
file and its directory were verified as owner-only modes `0600` and `0700`.

## Result

- Vault `/v1/sys/unseal` accepted the stored key through a local-only
  port-forward and stdin, without putting the key in a process argument.
- `vault status -format=json` reported `initialized: true`, `sealed: false`,
  `storage_type: file`, Vault version `2.0.3`.
- StatefulSet `vault-uc12` reported Ready `1/1`; Pod `vault-uc12-0` was
  Running and Ready.
- The temporary localhost port-forward was stopped.
- No KV engine, policy, role, product adapter or workload injection was
  configured or tested during this run.

The key file is a sensitive local operational artifact, not a repository
artifact. It should be backed up securely by its owner.
