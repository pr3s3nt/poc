---
id: VER-SCORE-MAIN-20261117
artifact: verification-record
status: current
last_reviewed: 2026-11-17
---

# Score authoring — accepted main integration, issue #83

The user accepted the feature and explicitly requested merge without another
pre-merge test run, citing the completed validation. Merge `8651a603` joins
`features/score` at `5771c44a` into main based on `cd40a792`; it is pushed.
There were no conflicts. Backend/frontend/e2e/scripts/deploy product sources
are identical to the accepted feature head. The feature branch is deleted both
locally and remotely. No pre-merge tests were rerun.

The [1072-test frontend result](2026-11-17-score-tab-sync.md),
[full backend baseline comparison and runtime](2026-11-16-score-authoring.md),
and [Keycloak managed endpoint outcome](2026-11-17-keycloak-score.md) remain
the corresponding feature evidence. Full backend remains baseline-red, with
the same failures as pinned main; no new green-backend claim is made.

## Retained kind rollout

Both runtime images were built from main and imported into the authorized
existing kind cluster as `orchestrator-backend:main-8651a603` and
`orchestrator-frontend:main-8651a603`. The retained Console was upgraded within
its existing installation; no cluster, PVC or application namespace was reset.
Preflight found no active/recovering/interrupted operations requiring a decision.

The retained writers were quiesced. A private PostgreSQL custom-format backup
was restored successfully into a task-owned verification database, then that
database was removed. Platform credential/Vault/state-key/cache storage was
backed up with Vault stopped. The owner setup Job ran before runtime and reported
that baseline/grants already existed and nothing changed. No PostgreSQL adapter,
migration or schema artifact changed in this feature. Runtime credentials stay
separate: the backend has no owner credential environment entry or mount.

Only runtime images were replaced, preserving deployment environment, mounts
and installation configuration. Backend/frontend are both Ready. The temporary
backup Pod was removed with matching run label, UID and resourceVersion
preconditions; its absence is verified. The completed owner Job is retained as
the upgrade audit record. Backups and private receipts remain outside Git.

The main Console's previous upstream forward had disconnected. A task-owned
reconnecting upstream restored the existing Tailscale Serve route, without
changing Serve configuration or closing user sessions. An initial browser
attempt during Pod replacement received 502 before login; no test Application
had been created. The stable post-rollout retry below passed.

## Post-rollout checks

Through the exact MagicDNS Tailscale Console origin, the actual browser/API
[tab synchronization scenario](../../e2e/scenarios/score-authoring/tab-sync.mjs)
passes Score/extension admission, unsaved form-to-document changes,
document-to-form variable/replica changes, immutable template/reference
preservation, invalid-text retention, the incomplete Service-row guard, explicit
Save/reopen equivalence, VI/EN and keyboard use. Its disposable Application was
deleted through the product API with terminal SUCCEEDED. No workload was
provisioned by this smoke scenario.

Before/after inventory comparison preserves all six original visible Application
identities, nine Resource Types, and 33 Definition versions/driver-input hashes.
Existing Git settings and the READY default CD binding were retained. The exact
main Applications route was also opened separately, with authenticated
same-origin API 200 and all six Applications present. An initial handoff locator
omitted the existing leading '+' in the create button; correcting that locator
required no product change.

Full suites were not repeated; post-rollout checks were scoped to the deployed
feature and data preservation. Image builds and subsequent docs/whitespace checks
pass. The user waiver applies to pre-merge retesting, not a claim that main's
existing full backend failures disappeared.

## User trial retained

The user has a live Keycloak/PostgreSQL Application managed by the separate Score
trial installation. Its configuration, Vault/secret delivery and ingress relays
are still dependencies of that running Application. Stopping or deleting that
stack as generic feature housekeeping would disrupt user-owned work and lose
its management path. It is therefore retained deliberately, with volumes/data
and private restart/ownership receipts; Keycloak/PostgreSQL remain Ready with
zero restarts observed after the main Console upgrade.

Merging code does not migrate that Application or its Secret values into the
main installation. Any later migration or removal needs its own explicit scope;
no adoption, copying of installation identities, or deletion was performed.
Exact Console/workload URLs and credential locations are only in the user/private
handoff, never this public record. The main rollout is complete and issue #83
can close after recording this result.
