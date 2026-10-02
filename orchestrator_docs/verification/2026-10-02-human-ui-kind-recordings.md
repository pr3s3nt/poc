---
id: VERIFY-20261002-HUMAN-UI-KIND-RECORDINGS
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-02
---

# Human-paced Developer and Platform Engineer recordings on kind

## Outcome and review videos

Both scenarios completed with external runner exit 0. Every product mutation
was performed through the browser UI; no API fixtures/mocks seeded the flow.
Headed Chromium was recorded through private Xvfb/ffmpeg, with actual tabs and
address bar, visible cursor, native select popups, 85 ms character typing and
read pauses. Video codec is H.264, 1440x900 at 15 fps. Synthetic secret and test
account password inputs were masked. Recordings contain no actual Vault token,
kubeconfig, database URL or terminal output.

| Scenario | Video | Duration / size / marks |
|---|---|---|
| Developer deploys acceptance app | [MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/acceptance-human-kind-20261002-113328-20230.mp4) | 325.2 s / 2,578,093 bytes / 10 marks |
| Platform Engineer catalog and real connection verification | [MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/platform-engineer-kind-20261002-115245-26032.mp4) | 299.666667 s / 2,456,114 bytes / 19 marks |

The reviewed files were uploaded as new assets to the existing
`acceptance-recordings` GitHub prerelease, without replacing earlier assets.
Both were downloaded back and compared byte-for-byte with the reviewed files.
SHA-256:

- Developer: `f4e059c855529d4bd8677be432e9e2a3f69f5d42f4c5ec88891e1ed172935948`.
- Platform: `596140068e9cb7b381edae51b4bf2b73dc122f4c0946e3d016f197247b6e8c8e`.

## Developer deployment

Command: `bash backend/test/integration/acceptance-playwright-kind.sh --human`.
Run: `acceptance-20261002113328-20230`; Application
`6bd50186-abe2-436e-840c-d92a2b05c410`; namespace
`app-6bd50186-abe2-436e-840c-d92a2b05c410-staging`.
Context `kind-idp-internal`, Ready node Kubernetes `v1.36.1`, existing Vault
unsealed and VSO ready. The scoped token was consumed by the backend by file
path; the runner used its own dynamically allocated Vault-forward port.

Developer created Application and UC-12 variables/secret through Settings.
Backend workload selected existing keys using UC-16 checklists, with same-name
references and no unnecessary aliases. PostgreSQL inputs and five resource
outputs used Other sources. The saved Score was asserted exactly, contained
no copied secret, and reopening Edit restored checklist selection and bindings.
Frontend used the backend Service reference and declared public path `/`.
Read-only inspection confirmed stored drafts matched the submitted references.

Preview reported two affected workloads; both Deploy results succeeded. The
script opened the deployed frontend through its own Service port-forward by
typing the address in browser chrome. Backend connection, environment, secret
and database rows all showed PASS; read-only diagnostic JSON confirmed true
checks without secret disclosure. One submitted job was stored and listed.
No worker was deployed, so its PENDING state is expected and does not prove
job processing. Public Ingress configuration was submitted, but this recording
checks the app through Service forwarding, not external DNS/Ingress delivery.

Useful video offsets: 3:23 restored selections, 4:26 Preview, 4:49 Deploy result,
4:59 app opened, 5:21 job submitted. Codex inspected these settled frames plus
Settings secret typing and workload form frames. The secret field was masked.

## Platform Engineer

Command: `bash backend/test/integration/uc02-04-video-local.sh --kind`.
Run: `uc02-04-kind-video-20261002115245-26032`; Application
`47a312d0-2df7-4965-b35b-76f34666e589`; connection `kind-24526032`.
The backend used temporary catalog state and fake execution adapters, while
its independently wired real Kubernetes ConnectionVerifier checked the host
context, reachable API and required create permissions read-only. No workload
was deployed by this second scenario and no Kubernetes object was created.

Platform Engineer registered `cache`, exercised duplicate ID and duplicate
input rejection, and registered PostgreSQL Definition `postgres-fast` for
class `fast`. Invalid Definition ID, unsupported Driver Input `replicas`, wrong
`storage` literal type and duplicate Definition ID were rejected with form
content retained. Invalid connection ID and missing host context were rejected.
The run-unique valid connection returned 201, verified true, READY, expected
cluster/context and server version `v1.36.1`.

After sign-out, Developer created a new Application and standalone Score
Preview selected `postgres-fast`, demonstrating catalog consumption without
restart. Final sign-out completed. Useful offsets: 0:41 Type registered,
2:15 unsupported Driver Input rejected, 2:49 Definition registered,
3:47 verified READY connection, 4:47 matching Preview, 4:55 signed out.
Codex inspected settled frames for registration, validation, READY, matching
and signed-out screens.

## Delegation, fixes and independent validation

Codex updated the runbook first, then delegated code/scripts only to Claude
through `clauded` in tmux `codex-ui-video-20261002`, polling approximately every
four minutes. Claude reported documentation changes for Codex to apply.
Documentation hash audits found no Claude modifications. No product runtime
code or canonical product requirement was changed.

Review fixed stale binding selectors, native headed input, awaited Settings
save completion, exact reference assertions, slower JSON/Score typing,
dynamic Vault forwarding and trustworthy namespace absence checks.
One invocation initially failed because the runner was not executable; calling
it with Bash resolved that. A Developer rerun fixed a test's assumption about
map ordering; it now verifies the full binding set independent of order.

A Platform run was interrupted when a `clauded -p` session ended while its
background task was still active. It had only 14 marks and was explicitly
classified incomplete, not successful. A persistent coordinator rerun exposed
an assertion expecting lowercase `kubernetes` where the UI renders enum
`KUBERNETES`; actual connection verification had succeeded. Claude corrected
that assertion and added HUP/INT/TERM traps to report interruption as nonzero
while retaining cleanup. The final Platform run used a durable tmux window,
so agent exit could not kill it. All failed/incomplete videos stay local and
were not published. No product defect was indicated by these failures.

Codex independently ran frontend typecheck/lint/test/build: 14 files, 107 tests
passed. After the final script corrections, affected `.mjs` lint and syntax,
all changed shell syntax and whitespace checks passed. Both actual recording
runners passed full MP4 decoding, dimensions/duration and settled-frame checks
(10 and 19 frames respectively). Signal demonstrations produced exit
129/130/143 and cleaned their own child. Documentation checker passed.
Shellcheck was unavailable. Go test gates were not repeated because Go product
code was unchanged; both runners built their temporary backend successfully.

## Cleanup and limits

The successful Developer namespace and the failed Developer attempt namespace
were independently confirmed absent via successful `--ignore-not-found`
Kubernetes lookups. The kind cluster remained Ready, current context was not
changed, temporary state files were removed, and no run browser/Xvfb processes
remained. Platform registration state existed only in the temporary backend.

Existing lifecycle limits remain: run-specific Vault KV revisions and ACL/auth
roles, kind image cache and local Docker acceptance images are retained.
No persistent platform service was replaced and no AWS resources were touched.
Local seeded accounts do not prove production RBAC. No live PostgreSQL system
store or cloud verification was added. Prompts/logs/videos/frames live outside
tracked source; scripts and this evidence record are the delivery artifacts.
The repository was clean at task start (`bd004d1`); no user-owned changes were
present. Publication was explicitly requested by the user.
