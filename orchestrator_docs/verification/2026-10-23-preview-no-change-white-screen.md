---
id: VERIFY-PREVIEW-NOCHANGE-WHITE-SCREEN-20261023
artifact: verification-report
status: current
last_reviewed: 2026-10-23
---

# Existing Application Preview — white-screen correction

## Cause and correction

User reported that clicking **Xem trước thay đổi** in an existing Application
replaced the Console with a white screen. Real Chromium reproduced it on all
six existing port 18094 Applications: Preview returned HTTP200, no pending
changes, and `review:{version:0,nodes:null,edges:null,batches:null,artifacts:null}`.
The pending service only assembles a version1 review when a last plan exists.
This zero-value response must not be treated as an executable or empty version1 plan.

DeploymentReview dereferenced `review.nodes.find` before its unsupported-version
guard. The exception `Cannot read properties of null (reading 'find')` escaped
React rendering; the resulting body was empty. This was a frontend defect, not
a failed provider operation or lost application data.

The public component now validates version and all collection shapes before
mounting the hook-owning renderer. Missing/unsupported review retains the existing
unavailable presentation. The Application page continues to show
**Không có thay đổi Workload nào để triển khai.** with Deploy disabled.
Separating the renderer also preserves hook ordering when a response changes
between unavailable and valid. No backend/schema/contract/admission or deploy
behavior was changed; the existing canonical no-change behavior remains valid.

Regression coverage includes the actual zero-value HTTP payload in
ApplicationHomePage.test.tsx, malformed/unsupported collections and review
availability transitions in DeploymentReview.test.tsx. Earlier tests omitted
the serialized review field and therefore did not exercise this crash.

## Verification and delivery

The local frontend bundle and `orchestrator-frontend:preview-kind` image were
rebuilt. Static assets were copied into the running frontend without restarting
backend/nginx or modifying stored data; prior hashed assets remain available
to in-flight clients. The port 18093 demo also serves the rebuilt local bundle.
Browser refresh loads the fixed index/bundle.

The initial after-fix audit covered ten supported Application staging pages
across ports 18094/18093: seven zero-value reviews and three valid graph reviews,
all without page exceptions or blank bodies. The one legacy dev-only Application
was kept behind its existing unsupported-scope guard. Final browser verification
passed 20 supported staging/production scenarios: 17 no-change reviews and three
valid graphs, with no page exceptions. Two unsupported legacy scopes remain
explicitly guarded. Assertions cover no-change text, disabled Deploy, valid graph
visibility and repeated Preview on the retained kind sample. The first final
observer incorrectly expected an Application heading on the legacy unsupported
page; its expected heading was corrected and that failed observer log retained.
Only read-only
queries and Preview POSTs ran; no draft/catalog writes or Deploy clicks occurred.

Checks passed: TypeScript, production build, affected ESLint, 37 focused tests,
and all **832 tests / 64 files** in the full frontend suite (160.08s).
Documentation checker and diff whitespace checks passed.
Full-directory lint reports 48 preexisting errors: the previous 46 plus two in
the already-untracked user-owned `deploy-acceptance-video.mjs`. That file is
preserved, unmodified and excluded from this task's commit. Go/backend/cloud/
cluster tests are reused because this correction changes only frontend rendering.

Private evidence: `/tmp/preview-white-20261023/` retains original Chromium errors,
before screenshots, actual review DTOs, fixed audit, final browser assertions and
validation logs. Public [fixed Console capture](artifacts/preview-white-20261023/fixed.png)
shows the retained sample's no-change notice, unavailable review and disabled
Deploy. No credential, Terraform state or raw provider error is published.
