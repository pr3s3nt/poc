---
id: VERIFY-WORKLOAD-GIT-SOURCE-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Retained issue #37 Console uses a Git template source

User requested converting the retained test Console from Inline to Git. Starting
commit `b6ab5bd4`, clean `features/bo_sung_default`. This is an operational source/
Definition configuration change, with no product code, schema or default changes.
Source semantics follow [ADR-035](../architecture/decisions/ADR-035-generic-workload-template-driver.md)
and [ADR-039](../architecture/decisions/ADR-039-template-owned-workload-defaults.md).

## Selected source and scope

- Repository: `https://github.com/pr3s3nt/poc.git`.
- Directory: `backend/template-sources/standard`.
- Immutable commit: `b6ab5bd4ce18ce6fb881345a98daff682282a7d9`, confirmed on remote
  branch `features/bo_sung_default` before registration.
- Source kind `git`, status `AVAILABLE`, ID
  `b89b1beb-dcdd-475f-adda-6149bd58c896`.
- Verified source content digest:
  `146792c8414161e71f86d09dd2875e3dfc0535ec27604c88db9c6012ca5814fb`.
- Definition `issue37-5c7bc93a-workload-git`, version 1, template source set to
  that Git record, criteria restricted to Application
  `5c7bc93a-edd3-4ee2-a618-f3a4a60d9853` (`issue37-template-authoring`).
- Both staging and production explicitly select Definition revision
  `f200ea7afe69b4bad07421d7ee939d9516b090ba7f5ce523feab18ee9cab769a`,
  selection revision 1. Other Definitions and their criteria were not changed.

The Git bundle was downloaded and verified through the actual Console Git
inspection form. The Definition was registered through the authenticated API;
version-controlled Environment selections and read-back confirm the selected
source. Immutable historical Inline records remain; they are not the selected
render source of this Application's new workload Definition. The authoring
catalog may deduplicate identical template metadata across those source records.

## Verification and retained Console

[Verified Console workload editor](http://desktop-bdjp2rq-1.tail342836.ts.net:18339/ui/applications/5c7bc93a-edd3-4ee2-a618-f3a4a60d9853/environments/staging/workloads/web)
was opened through the exact Tailscale Serve origin at `2026-11-06T09:35:27Z`.
The saved Definition editor shows Git, the repository URL, source directory and
pinned commit. Same-origin API read-back confirms kind, commit, availability and
selected Environment revisions. No mocked API responses were used.

The existing `web` desired document and template hash are unchanged. Pure resolver
checks return template CPU requests staging 100m/production 200m; the actual editor
shows an empty CPU override with the muted 100m hint and 128Mi memory hint. Browser
errors were empty. No Preview/Deploy, Kubernetes mutation or Git publication was
performed. Configuration and the draft remain in the existing isolated database
and source cache volumes; no service restart was needed.

Private evidence under `/home/thanhnt1/.local/state/issue37`:
`git-source-before.json`, `git-source-registration.json`,
`git-source-selected.json`, `git-source-definition.png`,
`git-source-workload.png`. A first browser read-back locator expected the visible
Edit text rather than its accessible label and timed out; the configuration had
already saved. Idempotent read-back with the correct label then passed.

The URL was checked from this host through Tailscale, not from every tailnet
ACL/device. Compose `issue37-defaults`, kind and Serve 18339 remain running;
restart/data scope follows the [retained stack instructions](2026-11-06-template-only-defaults.md#console-handoff).

Documentation and diff checks pass. No Go/frontend build or runtime suite was
repeated because product code did not change; validation used real Git inspection,
API read-back, source/Definition pin checks and the actual Console. New Git content
requires inspection and explicit revision selection; rendering never follows a
mutable branch automatically.
