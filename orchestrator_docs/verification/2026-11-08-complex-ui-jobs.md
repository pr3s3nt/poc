---
id: VERIFY-2026-11-08-COMPLEX-UI-JOBS
artifact: verification
status: current
last_reviewed: 2026-11-08
---

# Complex application prerequisite — first-class Job through UI, issue #50

This is an implementation checkpoint and real Job capability proof, not GitLab
acceptance or permission to merge main. User chose A (first-class Job). Product
code checkpoint is `a8d38ec5` on `features/complex_application_ui`.

## Implemented slice and checks

GW-08 owns typed Job policy and explicit same-Environment waitForJobs completion
prerequisites. Existing Job phase/acknowledgement is reused, with immutable
completed execution, no implicit replay and source capability admission. No SQL
schema change or new rollback/recovery behavior. Standard source adds Job 1.0.0
without changing existing preset metadata. Source/code/test mappings are recorded
in the implementation handoff and traceability matrix.

Affected environment/template-source/workloadspec package tests, focused planning/
renderer/pending/delivery checks and Go build pass. Coverage includes explicit
zero/null handling, bad target/kind/removal, mixed cycles, exact Job policy and
renderer tampering. Console focused checks pass: an initial four-file run has 41
passing tests; the later editor/JobFields run has 27 including added Job/consumer
Save regressions. Typecheck/lint/build pass with the existing PlatformSettings
warning and bundle-size advisory. These are focused phase checks; the issue's
full gate is not run because implementation/GitLab acceptance is still in progress.
Documentation/whitespace and new Node script syntax checks pass; local PlantUML
is unavailable, so changed diagram notes have not had a local PlantUML compile.

## Isolated Console setup

Dedicated Docker backend/frontend/PostgreSQL/Vault and owned volumes are created;
the main Console and other user stacks are unchanged. Official backend Dockerfile
and a frontend runtime image containing the production bundle are used. Bootstrap
only initialized the installation and first Admin; it did not create application
workloads or replace UI actions. Initial root Admin bootstrap created cache
subdirectories with root ownership; only the new stack's cache/recovery/Git volumes
were corrected to its runtime UID. An initial browser request during router
startup returned 404; subsequent authenticated browser/API checks pass. These are
setup failures, not hidden successful application deployments.

All following platform mutations use actual Console controls and normal response
assertions: platform Git settings, uploaded/verified cluster account, Fleet CD
binding/default, Git-backed cluster/namespace/workload Definitions and new
Application/Environment. The template source is the exact public task commit;
namespace module is an existing immutable Git source. No API seed or response
interception. The isolated Console holds actual new records, not fixtures.

## Real Job batch — passed

The scenario `e2e/scenarios/complex-application/` authored **both** workloads via
forms before any application Deploy: `z-prepare` (Job with explicit zero retry,
300-second deadline) and `a-consumer` (Deployment waiting for z-prepare). Names
intentionally oppose alphabetical order. Preview orders z-prepare before
consumer. First Deploy is one click/one recorded request for the whole Environment.
The exact recorded Operation completes **SUCCEEDED**.

Independent Kubernetes reads prove Job succeeded 1/1 and its completion timestamp
precedes or equals the consumer Deployment creation timestamp. Consumer is Ready.
There was no direct workload apply or manual migration invocation.

The existing scoped Fleet access helper created a Role and RoleBinding only after
checking the product-created namespace ownership/UID; its subject was discovered
from the verified binding. This is explicitly **operator infrastructure setup**,
not evidence that the platform automatically grants namespace permissions or that
end-to-end deployment needs no prerequisites. It did not create/adopt the Namespace
or grant cluster-wide rights. No operator GitRepo resync, Job rewrite, native
workload apply or ownership/pin bypass was used.

## Consumer update / no replay — passed

After the entire initial batch succeeded, UI edited and saved the consumer's
command arguments. Preview contains only the consumer UPDATE; a second recorded
single-click Deploy Operation succeeds. The Job has the **same UID and completion
timestamp** and succeeded count 1; the consumer is Ready. This is an explicit
follow-up update test. It does not stand up the application workload-by-workload.
The UI shows the Job task completed/healthy and both workload records ready.
The observability panel still reports incomplete metrics availability; metrics
acceptance is not claimed by this Job proof.

The workload namespace, Application, immutable Git repo, feature stack and access
forwards are retained for comparison while issue #50 proceeds. Namespace/Job UID,
operation IDs, API read-backs, screenshots, credential files and logs remain
private outside Git. No cleanup or main merge is claimed. Later cleanup must use
these exact recorded owners/UIDs and preserve other applications.

## Next decision — HTTPS for GitLab

GitLab's external hostname and HTTPS URL must be chosen before entering its
configuration and accepting clone/push over HTTPS. Current Tailscale status
reports no CertDomains and current Serve entries have no HTTPS listener; neither
tailnet HTTPS nor shared Serve settings were changed by this task. This is read
preflight evidence, not a failed attempt to install GitLab.

Proposed options requiring user selection:

1. Enable HTTPS Certificates for Tailscale and use a trusted MagicDNS certificate.
   This is a tailnet-level setting; issuing the certificate publishes its device
   FQDN in Certificate Transparency. Network access can remain tailnet-only; this
   does not require Funnel/public exposure.
2. Use a user-provided domain/certificate with an agreed DNS/routing setup.
3. Use a private test CA/certificate, keeping trust scoped to the test clients;
   browsers/Git need that CA to avoid certificate errors. Do not silently disable
   TLS verification or install a system-wide CA.

Sources: [Tailscale HTTPS](https://tailscale.com/docs/how-to/set-up-https-certificates),
[GitLab chart TLS](https://docs.gitlab.com/charts/installation/tls/).
The choice changes external DNS/certificate trust and possibly tailnet settings;
it is outside a routine form implementation detail. User input is needed before
the dependent GitLab configuration/endpoint work. At this checkpoint no TLS
certificate was requested or issued and no tailnet setting was mutated.

## Tailscale HTTPS selected — account action required

The user subsequently selected option 1 and authorized the Tailscale HTTPS path.
Read-only preflight still reports no CertDomains. The first certificate command
was denied for the non-operator WSL user; running it as root avoids changing the
shared Tailscale operator setting. Tailscale then returned HTTP 500 with
"your Tailscale account does not support getting TLS certs". No certificate was
issued. Existing Serve configuration was backed up privately and not changed.

The actual admin DNS page was opened with a browser and redirected to Tailscale
login. There is no authenticated admin session available to perform the account
change. The account owner needs to open the admin DNS page and enable HTTPS
Certificates, acknowledging the previously approved CT disclosure. This is an
account-access prerequisite, not another request to approve the chosen option.
After confirmation, retry issuance and continue the GitLab configuration through
UI. The login screenshot and command diagnostics remain outside Git. GitLab is
not deployed and no HTTPS endpoint is claimed ready.

## HTTPS enabled and GitLab drafts/Preview — passed, not deployment

After the owner enabled HTTPS, Tailscale issued a certificate. An HTTPS Serve
listener was added for the existing Gateway; comparison against the private
snapshot proves every previous listener/routing entry unchanged. The exact
MagicDNS URL completes TLS verification without ignoring errors. Before a matching
route exists its HTTP response is 404, so this is transport evidence only and is
not a working-GitLab handoff. Certificate/key and hostname remain private.

The isolated feature Vault now has Kubernetes authentication configured using the
existing reviewer credential; no new cluster-wide role/binding or shared Vault
change. A verified GitLab test store is registered through UI. Application,
Environment secrets, endpoint Definition and all nine workloads are entered through
forms: PostgreSQL 17, Redis 7.2, S3 objectstore, bucket initialization Job, Gitaly,
migration Job, Webservice/Workhorse, Sidekiq and Toolbox. Secrets stay in the store;
ConfigMaps contain nonsecret configuration/scripts. Stateful components have
retained claim templates. Every draft exists before any application Deploy.

Manifest preflight rejected the selected MinIO server/client tags. The user then
approved SeaweedFS over S3. The two drafts were edited through forms to use
SeaweedFS 4.52 and AWS CLI 2.31.20, preserving the Service identity/port; both
replacement manifests are available. This is not yet compatibility acceptance.
Source: [SeaweedFS release](https://github.com/seaweedfs/seaweedfs/releases/tag/4.52),
[MinIO archived repository](https://github.com/minio/minio).

Initial whole-app Preview returned PLANNING_REJECTED at graph construction.
Graph discovery was parsing shell/reference-looking text inside ConfigMap files,
contrary to GW-06's literal contract. The bounded fix excludes only file values
from graph/shared reference discovery, preserves authored text and still rejects
invalid real container references. The regression and all planning package tests
pass, as does Go build. The isolated backend was rebuilt/restarted. Repeated real
UI Preview now passes with all nine DEPLOY changes; bucket Job precedes migration,
and migration precedes Sidekiq/Toolbox/Webservice. No application Deploy was
submitted during these failed/successful Previews. Earlier authoring failures were
harness selectors, corrected using the actual form; no API mutation substituted.

New GitLab authoring/profile scripts are in `e2e/scenarios/complex-application/`.
Node syntax and documentation/whitespace checks pass. Frontend code is unchanged
in this checkpoint, so its earlier focused checks are reused. Full gate, GitLab
runtime/Git operations and cleanup remain pending; no main merge.
