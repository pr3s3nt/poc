---
id: VERIFY-TEMPLATE-IMAGE-RESOLUTION-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Generic template authoring waits for its required image

User-reported defect on the retained Console, branch `features/bo_sung_default`,
starting at `70b383a6`; working tree was clean. The `backend@1.0.0` template
publishes no runtime image. On initial open the form submitted `containers.main: {}`
to the pure resolver, received HTTP 400 INVALID_WORKLOAD, and misleadingly presented
an unavailable/invalid configuration error with Retry before any image was entered.

The editor now distinguishes incomplete required image input from resolver failure.
For a pinned template without a declared/frozen image, it asks for image input and
waits; it does not issue an invalid resolution request or invent an image. Once
entered, normal backend resolution resumes. Save still requires valid image input
and backend admission. Published image profiles continue inheriting their image
without copying it into desired overrides; Init/Sidecar still need their own images.
Backend policy values, template hashes, API admission and persistence are unchanged.
Canonical image requirements already matched this behavior, so no product requirement
was rewritten to accommodate the defect.

Checks: the focused defaults tests cover missing-image wait/no request, no Save
without image, successful resolution/Save after input, real failure/retry and
preserved template-provided image omission. Typecheck, lint and build pass; the
existing PlatformSettingsPage ref warning and chunk-size warning remain.

Full frontend suite (`npm test -- --maxWorkers=2`) completed with 969/971 tests
passing, including all 134 workload tests. Only the same two baseline E2E source
assertion failures remain: terraform-v2-labels and v04-internal-full. Documentation
and diff checks pass. Backend/Go/SQL/Kubernetes checks were not repeated because
no corresponding source or runtime contract changed.

At `2026-11-06T07:16:19Z`, authenticated Chromium checks on the actual Tailscale
origin passed in VI and EN: no resolver call or error on initial missing image,
HTTP 200 plus 100m/128Mi effective requests after entering an image, and inherited
Nginx image left omitted from the input. Workload list/draft version matched before
and after; no Save, Preview or Deploy was invoked, and browser errors were empty.
The pre-change live page and shared shell/components were the visual reference;
error/Retry was replaced with a single short required-input hint, while the compute
summary stays unconfigured until resolution can run. No shell/navigation changed.

The final single-hint build was rechecked at `2026-11-06T07:21:21Z`: exactly one
input hint while empty, no initial resolver call/error, and HTTP 200 after input.

The exact user URL is the real Tailscale Console page:
[Backend API authoring](http://desktop-bdjp2rq-1.tail342836.ts.net:18339/ui/applications/b1b983d7-cea1-4e4d-9a9e-63638919db9c/environments/staging/workloads/new?template=backend%401.0.0%3A9f59b711a3afb669fbd9f6b707e15c21dfdf01a8be1f8e086b29d9b1a9b57818).

Private evidence and image build inputs remain in
`/home/thanhnt1/.local/state/issue37`; source-check logs are
`/tmp/issue37-template-*.log`. Only the dedicated frontend image/service was updated.
The unchanged backend, kind/Fleet and application resources were not deployed or
mutated. The Console and Serve port 18339 remain running; restart/data instructions
are in [the original handoff](2026-11-06-workload-defaults.md).
