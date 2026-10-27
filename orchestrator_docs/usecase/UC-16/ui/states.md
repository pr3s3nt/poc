---
id: UC-16-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-24
related: UC-16, UC-12
---

# UC-16 UI states

## Bảng Biến môi trường — DEVEX-ENV

| State | UI behavior |
|---|---|
| New/empty | Không có dòng tự chọn; + Thêm biến tạo dòng Giá trị nhập. |
| Literal | Chuỗi rỗng hợp lệ; giữ whitespace/escaping; báo NUL/giá trị không hợp lệ mà không echo payload. |
| Existing/imported | Khôi phục nguồn, tên/alias, nhiều dòng cùng key; giữ advanced fields. |
| Key/output secret | Chỉ tên và nhãn Bí mật/trạng thái; không reveal/copy value. |
| Missing source | Giữ target/name cũ, lỗi ngay dòng; sửa hoặc xóa trước lưu. |
| Catalog loading/error | Phân biệt empty; retry GET giữ edits; khóa source/save cần metadata đó, không bỏ các dòng chưa validate. |
| Duplicate/invalid name | Lỗi container/dòng trước serialize; không overwrite trong map. |
| Source changed | Giữ tên, xóa payload source cũ khỏi row; không chuyển secret thành literal; không xóa key/dependency. |
| Source removed | Xóa mapping dòng đó; giữ key, dependency và các mappings khác. |
| Save busy/stale/failure | Khóa conflicting submit, giữ draft; reload/review cùng scope không replay mutation. |
| Scope changed | Bỏ late catalog/import/save/dialog replies; không mang edits hoặc secret input sang app/env/workload khác. |
| Legacy unavailable | Không mở editor bằng literal chưa được xác minh, không xóa dữ liệu; hiển thị lỗi an toàn. |

## Form con tạo key hoặc tài nguyên

| State | UI behavior |
|---|---|
| Open | Giữ parent mounted, app/env rõ; focus field đầu, không lộ secret cũ. |
| Cancel before submit | Không tạo key/dependency; xóa input secret, về dòng gốc. |
| Create key success | Dùng response/version mới, chọn key; clear input secret; key đã lưu, workload chưa lưu. |
| Create key duplicate | Không upsert; giữ form, GET/review rồi chọn key cùng loại hoặc đổi tên. |
| Missing store | Cho chọn/lưu kho READY ngay form; nếu không có kho, retry/hủy, không block chỉnh sửa các dòng khác. |
| Store success/key failure | Hiện kho đã lưu; key chưa thành công; không reset selection. |
| Create key validation/BUSY/stale | Giữ nonsecret edits/input che trong form; hướng dẫn sửa/đợi/reload, không tự retry. |
| Mutation outcome unknown | GET configuration để review; không báo chưa lưu hoặc replay create. Cùng tên không chứng minh đúng secret value; Dùng key hiện có cần thao tác tường minh. |
| Key committed/workload save fails or cancels | Key tồn tại trong Settings; giữ draft khi lỗi, không tự delete key. |
| Add dependency success | Chỉ sửa parent draft, chọn alias; focus output còn trống. |
| Add dependency invalid | Giữ subform; type/params/alias/class errors hiện theo field; không thêm partial dependency. |
| Parent row gone/scope changed | Không gắn kết quả vào dòng khác; committed key vẫn thuộc scope gốc, không auto rollback. |

## Các trạng thái hiện hành giữ nguyên

Loading/empty/error Application home; pending add/update/delete và Undo; Service
không hợp lệ/self/pending delete; Type params bắt buộc; public route trùng/sai
port; import invalid; no-op Preview; route-only retry; Save success chỉ pending.
Form và import kiểm mọi dependency, kể cả không dùng ở biến. Không gọi deploy
khi tạo key/dependency hoặc Save. Thay draft/config làm Preview cũ hết hiệu lực.

## T08 — Trạng thái nâng cao và cổng công khai

| State | Behavior |
|---|---|
| Basic/new | Phần nâng cao đóng; tóm tắt request `10m`/`32Mi`, limit chưa đặt, class `default` và target port fallback; không ghi defaults vào capacity fields. |
| Advanced edited/imported | Overrides giữ nguyên khi mở/đóng hoặc sửa trường cơ bản; tóm tắt phản ánh giá trị đang có. |
| Add public route | Chỉ chọn cổng khi thao tác thêm và có đúng một cổng hợp lệ; không tự bật route khi thêm/sửa cổng. |
| Public target invalid | Giữ lựa chọn đích cũ khi port đổi/xóa/không hợp lệ; hiện lỗi tiếng Việt và chặn Save, không chọn đích thay thế. |
| Import/save validation | Giữ nội dung và aliases; params number/bool serialize đúng kiểu; không advance draft sau failure. |
| BUSY/stale/scope | Giữ guards hiện hành và pending edits; không tự gửi lại Save hoặc dùng response của scope cũ. |
| Key catalog | Không tự chọn keys/secrets mới; thiếu key/loading/error khác empty và không lộ giá trị. |

Tất cả nhãn và validation frontend dùng catalog ADR-020 (Việt/Anh);
backend error envelope/mapping theo contract lỗi T20. Mapping và literal policy theo OC-16 DEVEX-ENV; các guards ngoài phạm vi giữ nguyên.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.

## T25 — Field route đã loại bỏ

Import hoặc draft có `service.publicPort` (kể cả rỗng/null hoặc cùng publicRoutes)
bị từ chối với thông báo Việt an toàn; không chuyển sang form/lưu thành công
và không tạo route mặc định. Giữ nội dung import/edits để sửa. Routes hợp lệ
round-trip đầy đủ; đổi/xóa cổng vẫn chặn lưu theo T08, không retarget.

## T30 renamed desired references

Draft được UC-12 rename opt-in sửa giữ container aliases và advanced fields;
editor mở lại hiển thị key mới. Editor còn giữ draft version cũ cần reload/review
khi Save trả STALE_VERSION; không ghi đè draft rename. Pending delete không được
hồi sinh. Lossless reconstruction failure giữ nguyên toàn bộ trạng thái và dùng
REFERENCE_REWRITE_UNSAFE với hướng dẫn Việt an toàn. Preview/Deploy vẫn tường minh.


## Metadata và tiến trình tài nguyên

Module/class metadata đang tải, lỗi hoặc unavailable chặn thao tác chưa kiểm
chứng; retry giữ edits, không thay bằng default hay replay mutation. Known
input/provider/integrity error dùng code/field an toàn. BUSY giữ dữ liệu đang
sửa; stale yêu cầu review/Preview mới. Progress lấy persisted stage/attempt,
không raw CLI/state/secret; interruption không được coi là cleanup thành công.

## ENV-UX — direct entry and on-demand source search

A new row is an editable name/value pair. Opening/cancelling search preserves
the draft and returns focus to the invoking row. Selecting a reference keeps
the name, clears old payload and returns focus to the row's source button.
Inline key/resource forms replace the picker; no nested Dialogs. Resource
creation reopens output search; the user explicitly chooses an output.
Missing catalog metadata is a loading/error state, with the existing editor
retry; unrelated literals stay editable. Missing references remain on their
original rows and block Save; search offers only currently valid candidates.
The Secret checkbox invokes UC-12 creation without changing the row until
creation succeeds; Cancel retains its previous ordinary value. References
show authoritative secret metadata read-only. No local variable reference chain.
