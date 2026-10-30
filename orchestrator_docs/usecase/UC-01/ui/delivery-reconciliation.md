---
id: UC-01-UI-DELIVERY-RECONCILIATION
artifact: use-case-ui-design
status: current
last_reviewed: 2026-10-28
related: [ADR-030, UC-01, UC-05, UC-09]
---

# Optional Environment CD

Required interaction design; the Settings panel is implemented on the feature branch. Reuse
[shared UI](../../../architecture/ui/README.md), actual AppShell,
SettingsPage, existing Environment tabs, section cards, inline-check, Button,
Dialog and status/error patterns. The current product Settings page was rendered
in Chromium with fake configuration data before drafting this extension; this
was a reference inspection, not Fleet acceptance or a new UI prototype.

## Settings panel

Within the selected staging/production tab, add one **Triển khai** section before
Secret Store. No new sidebar navigation and no infrastructure target selector.

- Checkbox **Dùng CD provider**, unchecked by default.
- When checked, show **CD provider** select of verified eligible bindings, e.g.
  `Fleet · Internal platform`. Provider kind/name are safe labels; credentials,
  Git paths, management topology and capability JSON stay out of this form.
- Checkbox **Tự sửa sai lệch**, unchecked by default, only when the binding
  supports the required continuous-object lifecycle. Help explains that admitted
  revisions are restored and Jobs/data resources are not automatically recreated.
- **Lưu thay đổi** saves desired selection. One confirmation: **Đã lưu. Xem trước
  để áp dụng.** Link to the existing Application Preview flow.
- Compact applied value **Đang dùng: Trực tiếp** or **Đang dùng: Fleet**; show
  **Chờ áp dụng** only when desired differs. PE/Admin edits; Developer reads.

Unchecking hides provider fields locally but retains the last selected draft
until Save/cancel. Cancel restores server desired selection. Switching Environment
discards unsaved local selection only with the existing unsaved-edit protection;
never leaks staging choice into production. Do not auto-select a binding when
multiple exist. A sole eligible binding may be a local suggestion, never a save.

| State | Presentation / action |
|---|---|
| Loading | Disable save; no false DIRECT applied summary before data loads |
| No binding | Inline **Chưa có CD provider sẵn sàng**; link PE/Admin to Cấu hình CD; Developer sees read-only guidance |
| Unverified/incompatible | Safe reason near selector; keep draft, block save/deploy as appropriate |
| Saved pending | Applied and desired labels; Preview link; no optimistic runtime success |
| Busy/deleting/transition held | Read-only, existing OperationBanner and progress; preserve unsaved text |
| Stale | Reload/review authoritative selection, do not automatically resubmit |
| Unknown mutation result | Reload operation by request ID; no blind repeated transition |
| API error | Stable localized code, retry read; raw provider output never rendered |

## Preview and transfer

Reuse current Preview component and confirm action. A delivery-only action shows
source → destination manager and affected workload/route list. Infrastructure is
listed as retained, with **Không đổi cluster hoặc dữ liệu** where the decision is
made. Show blockers for missing baseline or simultaneous pending workload/config
changes. Deploy starts transfer; ticking/saving does not.

Persisted stages map to **Kiểm tra**, **Bàn giao**, **Tiếp nhận**, **Xác minh**,
**Hoàn tất**. A partial handoff reports affected units and **Cần xử lý**, retains
the busy hold and offers authorized **Thử lại** only when server recovery permits.
Do not label the destination as applied until commit. Disabling CD uses the same
review/progress flow; there is no destructive “delete workloads and recreate” shortcut.

## Reconciliation status integration (UC-09)

In existing Environment workload/resource details, add a compact summary with
**Đồng bộ**, **Sức khỏe**, **Sai lệch**, observation time and stale indicator.
Show exact revisions and changed field paths only on expansion. Keep deployment
history and runtime observations separate. An unavailable observer is not a green
status or zero drift. **Đồng bộ lại** targets only the admitted revision and is
disabled when unsupported/busy. Infrastructure uses **Kiểm tra sai lệch**, then
PE/Admin **Xem phương án sửa** → **Xác nhận sửa**; never reuse the pure Score
Preview call to run Terraform. Show selected instances/actions, blockers and
expiry in the existing review/dialog patterns; no secret before/after values.

## Content and visual review

Reference inspected at 1440px: current dark sidebar, light page, Environment tabs,
white bordered Settings cards, buttons and inline status. Preserve these and
existing settings content. Move definitions of drift, Git/revision internals and
provider setup to expandable help; do not repeat explanatory paragraphs in each
card. VI/EN catalog keys are required; no hard-coded bilingual copy.

The implemented panel was rendered in Chromium using product AppShell/Settings
with fixture API data. Browser checks proved staging-only save, unsaved-edit
confirmation and production remaining DIRECT. Visual inspection found cramped
unstyled controls; the panel now reuses the existing inline editor/grid form
styles. Final visual recheck follows that fix. Definitions remain under expandable
help; there is no provider configuration/topology text in the normal form. This
is UI fixture evidence, not real Fleet deployment acceptance. Provider-binding authoring uses the platform CD configuration page in UC-04.

API/error/role mapping: [CDR-08](../../../architecture/contracts/delivery-reconciliation.md#cdr-08--public-api)
and [INF-C04](../../../architecture/contracts/infrastructure-drift.md#inf-c04--http-and-rbac).
