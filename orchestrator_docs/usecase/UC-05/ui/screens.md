---
id: UC-05-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-12
related: UC-05
---

# UC-05 screens

## Nhãn Console tiếng Việt — T13

Trang standalone dùng nhãn **Xem trước Score**, **Thao tác**, **ID Workload**,
**ID lần chạy**, **Score trước (YAML hoặc JSON)**, **Score sau (YAML hoặc JSON)**,
**Xem trước** và **Thử lại**. Tab giữ nguyên `staging`/`production`; các lựa chọn
Thêm/Cập nhật/Xóa Workload vẫn gửi `deploy`/`update`/`remove`.
Toàn bộ hướng dẫn, aria-label, trạng thái và lỗi hiển thị dùng tiếng Việt theo
[glossary](../../../GLOSSARY.md#nhãn-web-console-vi-01). YAML/JSON keys,
placeholder, technical IDs, graph descriptors, output keys và dữ liệu planning
giữ nguyên. Chỉ đổi thông báo parser, không thay quy tắc parse Score.

Kết quả có tóm tắt Delta, thay đổi tài nguyên, thứ tự tạo tài nguyên, liên kết
thực thi, đồ thị và tài liệu Delta/Candidate Deployment Set. Nhãn giải thích
bản đã lọc dữ liệu nhạy cảm và giá trị `***redacted***`; không diễn giải marker
thành dữ liệu thực. Kết nối ngầm được gọi là **Kết nối của môi trường**.
Lỗi API chưa có mapping code đầy đủ dùng fallback Việt an toàn, không hiện
raw backend/parser error hoặc dịch bằng regex; mapping code theo contract lỗi T20.

Application home provides **Preview Score** for the selected Environment.
The standalone page visibly identifies Application, Environment and the persisted Environment
connection key/profile, provides
action, Workload ID, Run ID and labeled Score before/after JSON/YAML editors.
Action hides irrelevant editors; validation never silently changes input.
Warn users to use configuration placeholders instead of literal secrets.

**Preview** performs only the read-only API operation. Back returns to the
Application; it does not save input. The result shows base Set/version, plan
hash, add/update/remove Delta summary, resource classification, provider-first
batches, resource execution bindings, graph nodes/edges and expandable formatted Delta
and Candidate Set. An empty change is explicitly explained as no workload
change; planning artifacts may still contain the Environment graph.

No standalone Save or Deploy control. Existing pending-change panel remains
unchanged in responsibility. Long artifacts must remain readable/scrollable.

The implicit internal cluster row identifies **Environment connection** and
the selected Connection key, rather than presenting it as a criteria-matched
user Definition. Other rows continue showing their selected Resource Definition.
