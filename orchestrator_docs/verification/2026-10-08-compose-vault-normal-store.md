---
id: VERIFY-20261008-COMPOSE-VAULT-NORMAL-STORE
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-08
---

# Ordinary Compose Vault store verification

Scope: UC-04 SS-07..08, [ADR-012](../architecture/decisions/ADR-012-environment-stores-and-transitions.md)
and [Compose runbook](../operations/docker-local.md). Coordinator updated design,
reviewed code from local `clauded` in tmux `vault_normal_coding` at roughly
four-minute intervals and required three review corrections. Claude implemented
product changes and regression tests in an isolated worktree; coordinator ran
independent final Go/Docker gates. Historical legacy-bootstrap evidence remains
unchanged.

## Code review and local gates

Shared registration validation, actual Vault verifier and platform credential
persistence are reused. Managed admission performs CAS on prior ID/legacy/ref;
PostgreSQL uses transaction/row locking and memory/snapshot uses a transaction
that propagates persistence errors and restores state. Interactive registration
remains insert-only. The runtime registry gets the token through CredentialRef,
with no legacy configuration in Compose.

Review corrections: include already-converted managed records in other
Organizations when refreshing a bootstrap token; select the real verifier for
explicit bootstrap even when workload adapters are fake/empty; propagate snapshot
write failure with rollback/attempt-only credential cleanup. Regression cases were
shown to fail without the fixes. Tests cover fresh seed, legacy conversion,
idempotent restart, conflicts, failed verification/persistence, token replacement,
concurrent admission and loser cleanup.

Independent `go test -race -count=1 ./...` ran with an owned disposable
PostgreSQL 16 container and passed, including PostgreSQL repository contracts.
`go vet ./...` and `go build ./...` passed. Frontend product source did not change;
its Docker build uses the current console. Full frontend unit/lint suites were
not repeated. Compose config, shell syntax, PlantUML and documentation validation
are checked before handoff.

## Real Docker observations

Owned projects `vault-normal-review` (ports 18081/13002) and `vault-normal-fresh`
(18082/13003) use actual PostgreSQL and persistent Vault; no fake Vault or workload
adapter is used for acceptance. The review database was booted using the previous
Compose release, then given an application and synthetic secret through real API
before upgrade. Conversion retained store ID, Environment selections/versions and
immutable references; the old Vault value remained readable. The store became
`Platform Vault`, legacy=false, verified=true, probe=REMOVED, kubernetesAuth=ABSENT.

Fresh startup from empty volumes also created exactly one ordinary READY store
with an opaque credential reference. Reading that credential with the scoped
platform token recovered the actual bootstrap token from Vault; neither raw token
nor its base64 appeared in PostgreSQL. Conflicting workload-address configuration
failed startup and preserved the existing record. Fresh full down/up retained the
single normal record.

## Runtime recording and handoff

Claude executed the headed, human-paced Playwright flow against the two real
Vault containers. All five observations passed: ordinary seeded store visible
without legacy label; PE registers the second Vault; existing application still
selects the same store with its configured secret; a new variable/secret survives
reload through the ordinary credential resolver; another application selects the
second Vault and saves a secret. Structural secret scans allowed only the active
masked password field, and found no other exposure. Coordinator read all three
synthetic values directly from their actual Vault containers and compared them;
PostgreSQL contained none of those secret/token plaintexts.

Coordinator disabled bootstrap and configured an absent bootstrap-token path,
recreated backend and successfully wrote another secret through the real API.
Runtime therefore used the stored credential reference rather than a legacy
or token-file fallback. Next, with backend stopped, the application token was
revoked in the owned Vault. Restart bootstrap created a replacement; backend
verified it and refreshed the credential reference while preserving store ID and
secret references. The old credential object was removed; exactly one managed
credential remained and secrets were still readable.

The reviewed [human MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/compose-vault-normal-20261008-082200.mp4)
is H.264, 1440×900, 157.133333 seconds, 1,717,580 bytes. Full decode succeeded and
sampled frames showed the store form, preserved Environment selection and the
second store's configured secret. GitHub upload metadata size/digest matched:
`sha256:a551c8395f6df60957b5e16180b53dac000960ddc547b23fff208872721fad99`.
Only the video was uploaded, not tokens, logs, PostgreSQL dumps or private input.

The final review project also passed full down/up after token replacement:
store identity/references and the one managed credential were retained. The
default `orchestrator-local` stack was rebuilt/upgraded with existing volumes
preserved; all four services healthy, frontend proxy health HTTP 200, and its
existing `platform-vault` became ordinary `Platform Vault` / verified READY.
The default stack remains running at localhost:3001. Owned review/fresh projects
and their volumes are removed after verification; operator containers/data are
preserved. No product-code changes followed the passing final Go/runtime gates.

## Limits

Vault Kubernetes auth remains ABSENT in these local containers. The tests prove
local credential/secret storage and registration, not cluster reachability,
workload delivery or production Vault security/HA. No Kubernetes/AWS resources
are mutated. Private synthetic input, browser orchestration and logs are kept
outside Git under `/tmp/poc-vault-normal-review` and not uploaded.
