---
id: VERIFY-20261118-SETUP-KUBERNETES-MAIN
artifact: verification-record
status: current
last_reviewed: 2026-11-18
---

# Accepted main integration — first-run setup and Kubernetes package, #89/#91

User accepted both features and requested merge. Main merge `8a357d2e` includes
`features/setup`; `d772495f` includes `features/kubernetes-manifests`. Two documentation
conflicts retained all setup and Kubernetes material; the resulting full tree
matched the accepted feature `81b1a5c6`. Main is pushed, both feature branches are
deleted locally/remotely, and the second checkout remains at detached HEAD.

## Retained main rollout

Accepted backend/frontend images were reused by identical source trees and tagged
`main-d772495f` on kind. A task-owned image tagger namespace was cleaned with its
original label/UID and fresh resourceVersion. This is verified image reuse, not a
new image build. Only retained backend/frontend image fields changed; installation,
Organization, users, connections, Applications, Types/Definitions, PVC UIDs and Secret
UIDs/data match before/after. Existing SSO and its designated personal local Admin
remain; no trial account, Git catalog, credential or setup token was imported.

No active/recovering/interrupted operation existed. Writers/frontend were stopped,
private custom-format PostgreSQL backup and a consistent platform-storage archive
were taken, with Vault stopped for its data archive. The live PostgreSQL data path
was excluded from the platform archive. Archive listings validated. An isolated
emptyDir PostgreSQL Pod restored the dump and matched all captured database/domain
identities before the owner Job ran. This main run did not rehearse Vault restoration;
that separate check passed on the feature installation in the
[package verification](2026-11-18-kubernetes-installations.md).

The main dump exposed an early-exit stdin issue: `pg_restore --list` can finish after
the TOC while kubectl still waits to transmit the remaining dump. The interrupted
listing was stopped; matching database/PVC/Secret snapshots were rechecked. A full
stdin-consuming listing validated the retained dump before backup/restore/setup
continued. Fix `57aa3a5e`, merged as `969f50e9`, drains the remaining input and preserves
the pg_restore exit status. Seven installer tests pass, including 16 MiB input and
failure propagation, plus an actual kind listing of the retained database dump.
No backend/frontend/e2e source changed in this correction.

Vault resumed using its existing configuration. Fresh owner Job
`db-setup-main-d772495f` completed before backend/frontend restarted Ready. No released
schema, migration, checksum or marker was edited. Temporary backup/restore Pods were
removed with UID/ownership checks and absence verified. Owner Job remains as audit.

## Browser and checks

Exact main MagicDNS Tailscale URL from Serve was opened with Playwright using the
existing designated personal local Admin and its matching private password file.
Organization claim is closed; platform guide and real setup-progress/Application
APIs work. An initial login check used the general seeded password file, which does
not authenticate this designated Admin; the configured private Admin credential
succeeded. The seeded employee's local login is still denied by existing SSO policy.
No auth policy or account was changed to make the check pass. URL and private files
are recorded outside Git; the final user report carries the access link.

Documentation, whitespace and Node syntax checks pass; PlantUML remains unavailable,
with no changed diagram. Full product results are reused only for identical product
source trees from the accepted kind run: frontend 117 files/1104 tests, typecheck,
lint/build pass; both Go builds pass. Candidate Go 1610 pass/104 fail/8 skip versus
main baseline 1603 pass/103 fail/8 skip. All 103 common failures/eight skips and the
extra intermittent unchanged Git cancellation test remain disclosed in
[full-suite evidence](2026-11-18-kubernetes-installations.md#focused-checks-and-full-suite-limits).
No clean full backend claim or user waiver of testing is made.

Docker setup/fresh trials are stopped with containers/volumes retained. The owned
kind manifest trial has its four Deployments scaled to zero; namespace, PVCs and
operator-held recovery material remain. Only that trial's dedicated forwards were
stopped; historical trial URLs are retired. Main Console/Serve and unrelated shared
services stay running. No staging/production cluster was accessed.
