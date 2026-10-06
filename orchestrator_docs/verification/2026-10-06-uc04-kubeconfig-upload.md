---
id: VERIFY-20261006-UC04-KUBECONFIG-UPLOAD
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-06
---

# UC-04 kubeconfig onboarding: implementation review and recording

## Outcome and reviewed video

The approved [UC-04 specification](../usecase/UC-04/specification.md) is
implemented for Kubernetes upload/paste onboarding. AWS Connection identity for
VPC/EKS/Aurora provisioning remains future delivery. Registration preserves
existing host-context connections and the Organization default.

[Review video: Kubernetes upload to READY](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc04-kubeconfig-kind-20261006-035937-11374.mp4)
shows the real Web Console against an isolated backend with actual read-only
Kubernetes verification and a run-owned local Vault test service. Workload
executor adapters are fake; no cluster resource is created by the recording.

- Run: `uc04-kubeconfig-kind-video-20261006035937-11374`.
- Command: `bash backend/test/integration/uc04-kubeconfig-video-local.sh --kind`.
- Connection: `kind-internal-711374`, `KUBERNETES`, `KUBECONFIG`, `READY`.
- Selected context: `kind-idp-internal`; verified server version `v1.36.1`.
- Runner exit 0; MP4 H.264, 1440x900, 110.266667 seconds, 885,519 bytes.
- Eleven marks: sign-in, list, invalid document, unsupported auth, single-context
  auto-selection, multiple contexts, reviewed destination, successful save,
  READY list, credential input cleared and sign-out.
- SHA-256: `b3c9f23d9ada4c9e90e2b03c54b65db88a4b5bff73a528566468e8da0a3307ab`.

Codex inspected all eleven settled frames and independently decoded the full
video. The address bar and cursor are visible; invalid pasted content is masked,
and real kubeconfig is uploaded from a private file without displaying content.
The new asset was uploaded to the existing `acceptance-recordings` prerelease
without replacing other assets, then downloaded and compared byte-for-byte.
No code commit/push was performed.

## Registration and storage assertions

All product mutations in the recording happen through browser UI, without route
mocks or API-created Connection fixtures. Inspection and registration responses
are asserted and scanned for credential values. The UI clears submitted input
only after successful save; the new READY row includes selected endpoint.

The runner additionally confirmed no embedded credential in page responses,
backend/Playwright logs, run/mark metadata or logical JSON state. Vault contains
exactly one immutable credential object with one version. Its base64 value
decodes to normalized selected-context config; the synthetic second context is
absent and the actual selected credential is present. The scoped demo token
cannot overwrite the existing object (403). Only booleans/counts are printed.

Vault is an isolated `hashicorp/vault:1.20` dev container with synthetic test
root token and a separate scoped backend token. This verifies the adapter against
Vault KV v2 APIs, not production Vault durability or policy installation. The
existing platform Vault and its policies were not changed.

## Claude coding and independent review

Codex completed realization, diagrams, shared credential design, schema/ERD,
contracts, UI states and traceability before authorizing coding. Claude performed
code/tests/scripts in tmux `claude-uc04-20261006`; Codex owns documentation and
review. Progress was checked at approximately four-minute intervals. Hash audits
confirmed Claude did not alter documentation or the user-approved spec.

First review required corrections to:

- Reject missing `apiVersion`/`kind` before verification or persistence.
- Remove raw kubectl stderr/resolver messages from credential-backed errors,
  keeping typed safe categories and legacy diagnostics.
- Exercise a real snapshot reopen/new backend instance for remove/routes;
  replace a racing cancellation test with deterministic bounded cleanup checks.
- Update the old catalog recording's obsolete Connection form selectors.
- Mark newly direct Go dependencies without unrelated tidy changes.
- Validate migration/concurrent upload against isolated PostgreSQL.

The corrected code was independently reviewed; Codex ran Go tests/build and all
frontend gates successfully. Credential-backed deploy/apply/readiness/remove,
Ingress and VSO use per-operation private kubeconfig files with cleanup. Opaque
Organization/Connection identity survives persisted targets. AWS credential
execution is not implemented by this delivery; Fleet fixed-cluster mode fails
closed for credential-backed targets.

## Validation

- Codex: `cd backend && go test ./... && go build ./...` passed.
- Codex: frontend typecheck/lint/test/build passed: 14 files, 111 tests.
- Claude: Go vet, uncached full tests/build and race checks for connection,
  deployment, Kubernetes and HTTP/e2e passed.
- Claude: disposable PostgreSQL 17.11 container run covered adapter, connection,
  catalog, deployment, preview and HTTP tests: 109 PASS, zero SKIP, exit 0.
  Includes migration 5 backfill/kind constraint/default preservation, concurrent
  upload collision/credential cleanup and reopen. Upload HTTP contract uses
  memory; upload-to-PostgreSQL is covered at service level.
- Script syntax/lint checked; recording assertions, full MP4 decode and all
  settled frames passed. Documentation checker and diff check are final gates.
- The updated local UC-02/03/04 catalog recording passed on its second smoke
  attempt, then again after the lint-only assertion rewrite. Final run:
  `uc02-04-video-20261006041437-32269`, exit 0, 19 marks, 319.5 seconds.
  It confirms inspect succeeds and registration with no
  credential store fails closed without saving a Connection, alongside the
  catalog and Developer Preview flows. Its video remains local.

The restart test rebuilds logical state and resolver from snapshot, retaining a
memory credential store as a test substitute for external Vault. It verifies
credential resolution with stub kubectl, not a live workload restart. Repeated
deploy reuses logical Active Resources but re-runs executors; execution-skipping
cache behavior is not claimed.

## Recording corrections and cleanup

Two incomplete upload recordings stayed local: an ambiguous file-input label
selected the radio control, and a context selector matched the uploaded filename.
Claude corrected selectors and added single-context/response/storage assertions;
the third run passed completely. No incomplete video was published. Recording
corrections did not change product runtime code.

The first catalog smoke attempt incorrectly treated retained masked textarea
content as visible credential output. Claude corrected the assertion to scan
outside the masked field; the second attempt passed. Both recording scripts
passed independent ESLint and syntax checks afterward.
The default simulated upload runner also passed (9 marks, 102 seconds).

Run-owned Vault containers, backend/browser/Xvfb processes, private credential
files and temporary logical state are cleaned by runner traps. Codex separately
confirmed the successful run's container and private/state files were absent,
and the kind node remained present. The disposable PostgreSQL container was
removed and its absence verified. Existing backend/platform containers were
preserved; no AWS or other cluster context was used.

Video/log/frame evidence stays outside tracked source. Test Docker image cache
and owner-only failed recording artifacts remain local. No raw credential,
state file or video was added to Git. The only pre-task change was the approved
UC-04 specification, preserved byte-for-byte throughout coding and review.
