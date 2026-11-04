---
id: VERIFY-DIRECT-BOOTSTRAP-20261104
artifact: verification-record
status: current
last_reviewed: 2026-11-04
---

# Direct Definition bootstrap — issue #33

Scope: [approved operational flow](../implementation/direct-bootstrap.md),
[bootstrap procedure](../../scripts/bootstrap/README.md),
[Generic Workload contract](../architecture/contracts/generic-workload.md).
The user approved environment-specific input, whole-script idempotency and
acceptance deployment at the end. Actual runtime was restricted to kind k8s-4f.
Initial Git working tree was clean at `48035f75`; implementation used branch
`feat/issue-33-direct-bootstrap`. No existing user files were reverted or staged.

## Environment and mutations

Used the retained dedicated `workload-template-final-20261104` Compose stack
(Console 3040, backend 8100), not the shared ports 3001/3031. Before mutation,
read-only PostgreSQL inspection returned `orchestrator-schema-v8` from
`schema_baseline.baseline`. No DB migration/reset/marker change was performed.
Backend startup and Generic Workload/template capabilities are required by preflight.

Preserved the existing Kubernetes Connection/account and verified CD binding,
including its separate existing Git Connection and platform default. Registered
missing Types and platform Git settings, prepared an immutable Git template source,
registered five direct Definitions and a scoped workload Vault store. No Catalog
source/release selection was created by the direct bootstrap.

Template source `1cbc647d-76f3-42c9-8de2-93cd8f15543e` pins repository
`https://github.com/pr3s3nt/poc.git`, directory `backend/template-sources/standard`,
commit `48035f75af4b3fe7ce1fafd9ebaf41f8b4356d10`, tree SHA-256
`fa4df6f2e54046fe85cefe58c30782663b899cee63ec480047df636be1f55c18`.
An initial input mistakenly used the embedded source digest for the Git tree;
pin validation blocked it. The declared input was corrected after independent
file-tree hashing; the already prepared exact source was reused, not duplicated.
Other rehearsal failures were runner locator/receipt serialization defects; they
were corrected without weakening product guards or changing existing resources.

Definitions `direct-dev-{cluster,k8s-namespace,postgres,redis,workload}` all retain
editor version 1 and revision generation 1. Terraform uses the declared verified
packaged namespace/PostgreSQL/Redis pins; workload uses the Git template source.
Criteria, input/output mappings, namespace/cluster references and explicit Workload
intent were submitted through the actual Console forms with review/read-back.
Existing app-scoped `generic-*` Definitions and unrelated Applications were kept.

## Acceptance and rerun

New Application `acceptance-direct-dev`, key
`a318a733-a7af-46ac-a241-15159265d519`, Environment `staging`, has no desired Catalog.
Its namespace UID is `73410f86-39dd-49b6-af07-eb2de902b66e`.
The script imported the existing three Generic Workload fixtures, then performed
Preview → Deploy through the product. It created only application-labelled
Role/RoleBinding `bootstrap-fleet-access` after the product namespace existed;
no Namespace/workload direct apply, shared controller installation or cluster RBAC.

The three workload deployment IDs remain:

- `9603815d-2f71-4a1b-9485-9af47ec5f962`
- `cb1d1273-ad2d-4eb8-a971-1fc6d3dd07e8`
- `f03a8a49-065e-4815-8566-81d09a800232`

All three Workloads are ready. All six Fleet units have GitRepo commit equal to
the requested 40-character SHA and exactly one Bundle ready 1/1 at that SHA.
Applied/desired Definition revision IDs equal the registered active revisions;
no promotion or new Definition revision occurred. PostgreSQL/Redis StatefulSets
and PVC UIDs, resource IDs, source ID/pins and job ID 1 were retained.
The real browser app reports PASS for backend connection, environment, secret,
database and cache; the configured persistent job is DONE exactly once.

The tracked E2E scenario runs platform-only with application/developer/verification
input removed, resumes full acceptance using the same stateDir, then reruns the
same full input. Server snapshots compare source IDs/pins, Definition histories,
account/CD identities, environment/config/workloads, selections, resources and
deployment count. These checks passed. Initial interrupted setup also resumed
from missing-only registration into acceptance; no data was deleted to force a
fresh platform. Platform-only BOOTSTRAPPED and full SUCCEEDED are separate reports.

## Checks and limits

- Bootstrap package: 49 tests passed, 0 skipped, including preserved Catalog tests,
  config/source pins, reuse/conflict, unknown creation, receipt serialization,
  platform-only → full resume, cross-target isolation, active revision IDs,
  selection guards, source origin, semantic acceptance input hashes and unknown
  registration-response journals (no replay until exact server reconciliation).
- Changed/new JavaScript syntax, documentation checker and Git whitespace checks
  passed. No Go/frontend product code, schema or shell was changed, so their
  whole-module suites/builds were not run for this script-only change.
- Real dev/kind Console/CD/Fleet/app verification and unchanged rerun passed.
  This is a fresh direct-Definition/acceptance slice on a retained dedicated
  schema-8 installation, not a claim of rebuilding an empty installation.
- Runtime used Git template preparation and installed verified Terraform sources.
  ZIP/Inline and external Git Terraform preparation are supported by the runner
  but were not deployed in this runtime scenario. No staging, production, AWS,
  real Rancher or issue-30 upgrade matrix was run. No intentional failure runtime
  matrix was added; conflict/error guards have focused automated tests.

## Browser handoff and retained data

Real Console:
[acceptance-direct-dev / staging](http://desktop-bdjp2rq-1.tail342836.ts.net:3040/ui/applications/a318a733-a7af-46ac-a241-15159265d519?environment=staging).
Real workload:
[acceptance application](http://desktop-bdjp2rq-1.tail342836.ts.net:3041/).
Both exact MagicDNS URLs were opened in Chromium from this host. The
[Resource Types](http://desktop-bdjp2rq-1.tail342836.ts.net:3040/ui/platform/resource-types)
and [Definition list](http://desktop-bdjp2rq-1.tail342836.ts.net:3040/ui/platform/resource-definitions)
were also opened and verified with all seven Types/five direct Definitions. Console loaded
all three ready Workloads and synchronized healthy delivery units; its real
same-origin API returned ready data. Workload displayed all five PASS checks and
one DONE job. This does not assert another device's tailnet ACL.

Existing UI reference was the current Resource Definitions editor and Application
home on Console 3040; no UI/shell/style redesign was made. Visual inspection of
the handoff screenshots confirmed product pages and ready data. No explanatory
product content was added by this script change.

Private config/state/logs/screenshots/reports are under
`~/.local/state/issue33/`; credentials are referenced from private files outside Git.
Application, PostgreSQL/Redis/PVC/job data, Git/CD history, reusable platform
configuration and the port-forward are intentionally retained for review/resume.
No user-owned services/tmux sessions or existing Serve routes were stopped.

Restart the existing Console using the command in the
[original hosting handoff](2026-11-04-generic-workload.md#console-handoff).
Resume the application/forward with the same reviewed private config:

```sh
node scripts/bootstrap/run.mjs --config /home/thanhnt1/.local/state/issue33/config.json
sudo tailscale serve --bg --http=3041 http://127.0.0.1:3041
```

Cleanup is separate: verify the recorded Application key and namespace UID, then
use product removal/Preview/Deploy and Application deletion; remove scoped access
objects only with ownership and UID preconditions. Do not delete shared/preexisting
configuration to make the platform empty.
