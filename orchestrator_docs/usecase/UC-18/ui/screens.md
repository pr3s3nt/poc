---
id: UC-18-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-27
related: UC-18, UC-01, UC-07
---

# UC-18 screens

Reuse the existing Settings page shell and the Xóa tài nguyên confirmation
pattern ([UC-07 screens](../../UC-07/ui/screens.md#xóa-tài-nguyên-tại-resource-detail)).
Follow [shared UI design](../../../architecture/ui/README.md); no new navigation.

## Vùng nguy hiểm trong Cài đặt

Below the Environment tabs, one Application-level section **Xóa ứng dụng**
with one short sentence and a danger button. Visible to every role with
Application access. Disabled with a reason while any Environment is busy.

## Hộp thoại xác nhận

1. On open, call deletion preview; show loading, then the scope. No provider call.
2. Two groups, `staging` and `production`: workloads to remove, resources to
   destroy (technical name, Type, generation). Collapsed by default with counts.
3. One group **Được giữ lại**: PREEXISTING cluster, Connection, Secret Store,
   catalog. Warning when matching criteria target this Application.
4. Blockers replace the confirmation with reasons and links (operation, resource
   detail). Confirmation is not possible while blockers exist.
5. Warning **Xóa vĩnh viễn dữ liệu, không thể hoàn tác**; field
   “Nhập tên ứng dụng” must equal the server name exactly. Confirm button stays
   disabled until it matches and the preview is loaded.
6. Cancel calls nothing. Confirm sends the token and name once.

## Tiến trình

After `202`, the Application home shows a banner “Đang xóa ứng dụng” and the
read-only state on every page of the Application. A progress panel shows per
Environment the stage (Gỡ workload → Xóa tài nguyên → Xác minh → Xóa secret)
and per-resource results. On failure show stage, remaining IDs and **Thử lại**,
which reopens the dialog with a fresh preview of the remainder. On success
navigate to the Applications list with a short notice. No undo.
