# Complex application UI acceptance — issue #50

Goal: validate GW-08 first-class Job completion prerequisites through actual
Console authoring and one Environment batch, then use that capability for the
GitLab mapping recorded in orchestrator_docs/implementation/complex-application-ui.md.
This scenario is in progress; having these scripts is not deployment acceptance.
References: UC-03/05/06/12/16, generic-workload GW-08, e2e/README.md and kind/Tailscale
runbooks. No response interception, API seed or direct workload apply.

Prerequisites: e2e and scripts/bootstrap dependencies, Chromium, isolated feature
Console with Job code, Git credentials, uploaded kubeconfig and an existing kind
with Fleet. Shared login variables ORCH_PLATFORM_URL/USERNAME/PASSWORD_FILE and
HEADLESS apply. ORCH_TEST_RUN_DIR must be outside Git and contain config.json:
Git scope/username/token file, kubeconfig file/context/endpoint, module repository/
ref/path and template repository/ref/path. File contents are private input, never
committed or printed. The platform/config helpers use UI mutations and technical
GETs; bootstrap creates only installation/admin prerequisites separately.

From root: `node e2e/scenarios/complex-application/setup.mjs platform`, then
`node e2e/scenarios/complex-application/setup.mjs definitions`. State and receipts
are saved after acknowledged UI changes to permit inspecting/resuming failures.
Platform creates Git settings, scoped cluster account and verified/default Fleet
binding in the isolated Console. Definitions registers cluster, namespace and
Job-capable workload Git definitions. No application Deploy is performed by setup.

After setup, run `node e2e/scenarios/complex-application/job-proof.mjs author`,
then `preview`, then `deploy`. It creates its own Application, authors both
Job/consumer workloads before one Preview/Deploy, and records the exact attempt
before submission. `status` observes the recorded initial attempt without
resubmitting. `update` edits/deploys only the consumer after the complete initial
batch succeeded, checks unchanged Job UID/completion timestamp and ready consumer.
Never rerun deploy/update when a recorded attempt exists; inspect the receipt and
operation first. The host clusterKubeconfigFile in config.json is required for
technical reads and namespace-scoped Fleet Role/RoleBinding preparation using the
existing bootstrap helper. That operator preparation is separate evidence; no
workload manifests are applied manually. GitLab acceptance is a separate later
stage, not implied by this prerequisite test.

Side effects: Console platform records, remote Git access verification and later
explicit app deployment; never cloud. Do not run against shared/staging Console
without the issue authorization and exact config. Evidence/screenshots/kubeconfig/
credentials/state stay outside Git. Preserve failed run evidence. Cleanup removes
only scenario-owned Applications through UI and verifies namespace/GitRepo absence
with recorded ownership/UID; retained storage needs separate explicit cleanup.
Other stacks, cluster controllers and credentials are preserved. A live workload
is retained for handoff only when recorded explicitly; no implicit volume deletion.

GitLab continuation (in progress): `gitlab-profile.mjs` derives form input JSON
from the checksum-verified `gitlab-rendered.yaml` in the private run directory.
It does not apply manifests or carry secret bytes. `gitlab.mjs prepare` creates
its own app/store/configuration through UI using private `gitlab-input.json`
(secretFiles and endpoint routing policy); `endpoint` registers the scoped
endpoint Definition. `author` saves all nine workloads; `attach-endpoint` attaches
webservice HTTP access. `storage` is the approved pre-deploy MinIO-to-SeaweedFS
conversion of the two existing drafts, recorded per workload. Do not run that
conversion against a new profile already authored with SeaweedFS. `preview`
checks the complete saved draft set, nine changes and Job ordering without Deploy.
`inspect` only reads store readiness. The store registration requires
ORCH_VAULT_WORKLOAD_ADDRESS and a private vault-application-token input; operator
Kubernetes authentication setup is a separate prerequisite. Drafts, receipts and
rendered files stay outside Git. Preview is not runtime acceptance; do not claim
that these stages deploy GitLab or that all workload images have been pulled.

Recorded deployment continuation adds `deploy` (first batch), `status` (observe
only) and explicit `retry` after a diagnosed failed/recovered attempt. Every
attempt records its reviewRequestId before observation and validates history;
never invoke deploy to resume an unknown attempt. The Deploy click starts the
observer immediately rather than waiting for the long-running HTTP response,
so scoped namespace access can be prepared as soon as the platform creates it.
`namespace-access` only invokes that guarded operator helper; `refresh-source`
re-inspects the configured exact Git ref after an executable change and explicitly
selects a new Definition revision in this app. `recover` uses Settings' existing
recovery control and records the result, without overriding guards. It may remain
incomplete for a failed Job; do not interpret running it as successful recovery.

The current retry checker is intentionally limited to the recorded nine-change
initial batch. A later partial-success retry or new Job identity needs explicit
scenario updates and assertions. `updateGitLabConnectionFiles` is prepared for
form corrections to the literal files, but has not yet been exercised. Service
references resolve to complete HTTP URLs: use the full S3 URL directly; Rails
config must extract hostname/authority rather than add another URL scheme. The
initial bucket Job used a double scheme and failed; its immutable identity must
not be rewritten/replayed. Keep failed attempt records and use an explicitly new
Job identity if the supported lifecycle allows it.

Approved option 2 adds bounded failed-Job recovery/removal on the current app.
`recover` must pass the product proof; failure remains blocked and is not a
success. After verified recovery and regenerating the corrected offline profile,
`refresh-source` explicitly selects the inspected renderer revision, then
`correct-failed-job` edits connection ConfigMaps, authors `object-init-v2`, changes
the migration completion reference and marks the old Job for removal through UI.
It asserts that deletion remains PENDING_DELETE rather than silently discarding a
published draft. `retry` previews the entire replacement/removal batch and clicks
Environment Deploy once; history and previous attempt receipts remain retained.

`recover` now observes the recorded operation asynchronously after its single UI
click, preserving the request if recovery outlasts a short browser response wait.
For the diagnosed multiline Settings-input mistake, `correct-migration` uses
privately prepared YAML-compatible single-line JSON, asserts the input preserves
its bytes, replaces S3/Rails secrets through Settings, authors explicit new
bucket/migration Jobs, repoints consumers and retains three retirement drafts.
Preview asserts the complete 12-change batch. This does not replay a completed Job.

For initial Settings-based preparation, S3_CONNECTION and RAILS_SECRETS must be
valid YAML-compatible JSON on a single line. A multiline file must use the actual
file importer rather than filling a single-line password field.
`gitlab-web.mjs` opens the actual configured HTTPS origin only after the recorded
batch is SUCCEEDED, and keeps browser inspection evidence private. Inspection
alone is not login/project/Git/upload/retention acceptance.

`correct-postgres` handles the observed schema lock-table exhaustion by editing
only this test PostgreSQL's startup argument to max_locks_per_transaction=4096,
retaining data and authoring explicit replacement Jobs/references. The profile
generator includes the same sizing for fresh runs. The next complete Preview
asserts thirteen changes; no SQL reset, direct config edit or individual Deploy.
