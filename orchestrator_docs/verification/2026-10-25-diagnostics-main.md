---
id: VERIFY-DIAGNOSTICS-MAIN-20261025
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-25
---

# Account diagnostics: merged main acceptance

[PR #17](https://github.com/pr3s3nt/poc/pull/17) merged on2026-10-25 at04:50UTC
as `7f3886bf8c87ac017e168ab4f0b86c8b17437c50`. Codex independently ran the
complete real browser acceptance again from a clean checkout of that exact main
commit, after [37/37 pre-merge acceptance](2026-10-25-account-diagnostics.md).

Result: **37/37 PASS**, zero browser page errors, `CLEANUP-PROVEN`.
Command: `bash backend/test/integration/diagnostics-e2e.sh --evidence NEW_PATH`.
This second run created its own fresh kind cluster and real
PostgreSQL/Vault/backend/built-nginx Console. No provider or product HTTP response
was mocked. The merged product tree matches the reviewed PR head; subsequent
handoff changes are documentation only.

The run reverified real account onboarding, General facts against kubectl,
never-deployed desired-state requirements, exact missing StorageClass FAIL and
dependent database BLOCKED, native repair and UI rerun, real provisioner failure,
exact-class PVC/readypod execution and verified cleanup, cancellation with native
resources present, purpose/account isolation, safe versioned export, reload and
backend-restart history, SIGKILL interruption/recovery and Developer403. No new
product defect was found after merge.

## Evidence and cleanup

Evidence is retained outside Git at
`/home/thanhnt1/projects/diagnose/evidence/diagnose-coordinator-20261025/postmerge-7f3886b`.
Run/cluster/Compose project: `cdiag-261025045028-17904`. See `owned.json`, `run.json`,
`assertions.json`, screenshots, native fixture logs, `result.txt` and `cleanup.txt`.
The runner verified absence of its containers, volumes, network, images and
cluster before deleting its private bootstrap credentials.

Coordinator fixtures were also removed with identity checks and native absence:
reviewer Compose project `diagnose-review-1025a`, its images, PostgreSQL integration
container `diagnose-pg-check-20261025`, reviewer cluster `diagnose-review-1025a`
and Claude's earlier owned dev cluster `cdiag-dev-063648`. The coordinator's
private copied credentials/kubeconfigs were removed only after this verification.
No task-owned `cdiag-*` image tags or clusters remained. Existing user
`idp-internal`/`k8s-4f` clusters, retained Console deployments and tmux sessions
were preserved. The coordinator checkout is on main.

## Practical limits

The [pre-merge record](2026-10-25-account-diagnostics.md) records independent
909-test frontend, Go/PostgreSQL/race, full PR CI and real read-only AWS checks.
Post-merge runtime evidence has the same boundaries: primitive pod/PVC execution
is not an Application image/SQL/cache transaction test; workload-side Vault login
is not proven; AWS quota and paid execution remain UNKNOWN and IAM simulation is
policy evidence. Retained databases were neither reset nor silently migrated.
