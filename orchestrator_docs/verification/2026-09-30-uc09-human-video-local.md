---
id: VER-2026-09-30-UC09-HUMAN-VIDEO-LOCAL
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-30
---

# UC-09 human-paced review video — 2026-09-30

Claude wrote and ran the local Playwright recording; the coordinator reviewed
the script, requested safety fixes, inspected sampled frames and decoded the
whole final file independently before publishing it.

- Command: `bash backend/test/integration/uc09-video-local.sh` after building
  the Web Console.
- Run: `uc09-video-20260930043252-720`, exit status `0`.
- Video: [MP4 release asset](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc09-human-local-20260930-043252-720.mp4).
  H.264, 1280x900, 15 fps, 134.13 seconds, 1,059,140 bytes.
- SHA-256: `8180c9ba59ef15403e20e543b936d9a30b03fc5ef76c500f39d696a349ef49e6`.
- Local evidence: `/tmp/uc09-video-20260930043252-720.a6e6bb/`, including
  `marks.json`, `ffprobe.json`, logs and 19 sampled frames. Temporary evidence
  is not tracked source; the release asset is the durable attachment.

## Recorded actions

The recording captures a headed Chromium window on an isolated Xvfb display,
including the real tab strip and address bar. A visible pointer follows mouse
movement and flashes on clicks. Username/password are typed sequentially;
the password remains masked. Native X11 keyboard events choose status-filter
options and type a URL into Chromium's real address bar. Important views pause
for 2–4 seconds. There is no URL overlay or hidden API substitute for recorded
UI actions; the pointer overlay is confined to the page, not browser chrome.

| Approximate time | Observation |
|---|---|
| 0:02–0:18 | Sign in, select Application and view staging recent deployments. |
| 0:25–0:33 | Select production and view empty recent/history states. |
| 0:43–1:06 | Staging history, then click/keyboard failed and succeeded filters. |
| 1:17–1:22 | Planning-failure reason; no fabricated workload status or plan. |
| 1:30–1:42 | Successful workload digest, redacted outputs, graph and batches. |
| 1:49 | Older Deployment displays its own digest, not the redeploy's digest. |
| 2:00–2:03 | Type an out-of-scope production URL; show scoped not-found. |
| 2:10 | Sign out and return to the sign-in form. |

Playwright assertions passed for all these states, newest-first ordering,
observed server-side filter requests, distinct workload digests, secret-output
redaction, absence of resolved inputs in the displayed detail and no retry
button on not-found.

## Fixture and execution boundary

Before recording, a separate API request context creates an Application and
three history records: successful backend deployment, planning failure and a
successful redeploy with a changed manifest. These are test fixtures on a
fake-adapter backend with temporary JSON state, not recorded UI create/deploy
steps. This video demonstrates UC-09 UI/read-model behavior, not Kubernetes or
cloud execution. Restart persistence has separate
[JSON/PostgreSQL verification](2026-09-30-uc09-local-observability.md).

Only seeded local test-account credentials and fake workload data are used.
No host kubeconfig, cloud credential, real secret value or desktop application
appears in the recording. The published asset is in the repository's public
release, not a private attachment.

## Validation and cleanup

- Frontend typecheck, lint, 11 test files/36 tests and build passed independently.
- Shell syntax, Node syntax, documentation checker and `git diff --check` passed.
- The runner rejects existing/symlink output directories rather than overwriting
  prior evidence; the coding agent verified this refusal.
- ffmpeg startup/exit failures fail the run; the final MP4 decoded completely
  without errors. The runner checked codec, dimensions, duration, last mark and
  non-blank sampled frames. Coordinator inspected masked sign-in, status filter,
  failure, redaction, native URL typing and not-found frames.
- Owned backend, browser, recorder and Xvfb processes stopped; temporary JSON
  state was removed. Video/logs/frames remain for review. Dependencies for native
  X11 input were extracted only into the private work directory, not installed
  system-wide. Docker, kind and AWS were not used.

No product code or shared human-input helper was changed for this recording.
