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
