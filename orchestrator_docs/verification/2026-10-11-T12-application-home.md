---
id: VERIFY-T12-APPLICATION-HOME-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T12 — Trang ứng dụng

## Outcome và phạm vi

VI-02/04/05 trên branch `refactor_ui`; T01/T10 DONE, working tree sạch khi
bắt đầu. Codex sửa canonical UC-01 UI screens/states, UC-07/16 UI screens
trước code. Claude implement qua `clauded -p` trong session task riêng
`codex-T12-20261011`, không sửa docs/commit/push. Codex review độc lập.

ApplicationHomePage, RecentDeployments và shared ApplicationTarget dùng nhãn
Việt cho điều hướng, trạng thái Workload, pending Preview/Deploy, hành động,
xác nhận Xóa/Hoàn tác, tóm tắt đích và kết quả từng Workload. Tab và payload giữ
`staging`/`production`; technical IDs, tên, URLs, graph và API semantics giữ
nguyên. Preview chỉ đọc, Deploy cần click và token hiện tại. Stale bỏ preview
và tải lại danh sách; partial giữ hướng dẫn xem trước mới để thử lại, không
rollback tự động. Không hiển thị raw API/provider hoặc per-workload error.

Missing-key metadata chưa có trong API client (chỉ error/field); T12 dùng
fallback Việt an toàn, không parse chuỗi tiếng Anh hoặc hiện value. Full stable
code mapping thuộc T20. Shared target cũng xuất hiện ở Score Preview; chỉ target
và assertion liên quan thay đổi, không Việt hóa trang T13/14/15 ngoài scope.
UC-16 UI summary cũ “hai môi trường cùng đích” được chỉnh khớp UC-01 BR-04/05.

## Review và checks

- Review yêu cầu lowercase technical tabs, Vùng/bản nháp/đường dẫn công khai,
  own-key label lookups; sửa test late Deploy từ unmount/absent DOM thành
  rerender cùng component sang Application khác rồi resolve trong act và kiểm
  context mới không bị thay đổi.
- RecentDeployments tests kiểm error/retry, tối đa năm mục theo server order,
  đúng detail link và bỏ response cũ khi đổi Environment.
- V-FE cuối của Claude exit 0; Codex độc lập chạy
  `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
  exit 0: 336 tests/20 files. Build còn cảnh báo chunk >500 kB.
- `bash -n backend/test/integration/refactor-ui-local.sh` và whitespace pass.
  Claude syntax-check e2e mjs; Codex kiểm lại các mjs thuộc diff trước commit.
- Audit runner cũ sửa home result/422/target labels còn sót và giữ heading
  tiếng Anh của detail/history chưa Việt hóa (T14/T15). Các runner external
  chỉ syntax-check; không claim live regression từ những sửa selector này.
- V-D/docs checker pass trước commit; không đổi PlantUML.

## Browser evidence và giới hạn

Primary: `/tmp/poc-refactor-T12-final/`, run ID
`refactor-20261011112419-22244`; headed, fake adapters/state JSON disposable.
Có `T12-raw.mp4`, `T12-captioned.mp4`, captions.srt/ass, marks.json,
assertions.txt/result.txt/run.json/mocks.json và dialog still frames.
14 assertions PASS, 19 marks gồm bốn dialog-visible marks. Captioned video
1440×996, 192.6 giây. Codex full decode raw/captioned exit 0; review frame
25/111/118/140/161/184 giây: created/preview/partial/dialog/stale/scope đúng,
cursor rõ, phụ đề strip dưới không che UI và không lộ secret. Native browser
Cancel/OK là browser chrome, confirmation product dùng tiếng Việt.

Product UI qua backend fake local: sign-in/create app, chọn connection và Save,
hai Workload draft, unconfigured Preview 422, pending Preview, explicit Deploy,
recent history, no-op, Xóa hủy/xác nhận và Hoàn tác. Native confirmation dùng
pattern UC-07: giữ dialog visible, pause rồi X11 Escape/Return; không bypass
bằng dialog.accept/dismiss. Preview assertions so workload list trước/sau và
đếm Deploy requests bằng zero cho tới click triển khai.

Projection mocks có ghi caption/assertion/mocks.json: deploy PARTIAL với chuỗi
lỗi giống bí mật; deploy 409; DELETE 409; giữ Preview 3.5 giây rồi forward
backend để kiểm late scope. PARTIAL không mutate backend, nên list reload vẫn
có hai draft; lượt xem trước mới/Deploy tiếp theo qua fake deliverer thành công
cả hai. Không claim live partial execution hoặc selective failed-only retry từ
projection này. BUSY có unit coverage, không có running operation trong browser.
Missing-key value không render; browser không tạo thêm key lifecycle scenario.

T09 regression sau review PASS: `/tmp/poc-refactor-T09-T12final/`.
T01/T03/T08/T09/T11 regression trước review PASS, paths
`/tmp/poc-refactor-Txx-T12reg/` (thay Txx bằng task thực); không claim là rerun
sau minor review. Lượt T12 đầu pass giữ `/tmp/poc-refactor-T12-r1/`, không dùng
làm primary vì auto-answer confirmation được sửa ở lượt cuối. Evidence ngoài
Git, không upload hoặc ghi đè.

## External, cleanup và handoff

Không đọc .env/credentials thật hoặc mutate retained DB/Vault/K8S-4F/AWS.
Go/conformance/SQL/live checks không chạy vì frontend/e2e-only; các external
runners chỉ sửa selectors và syntax-check, không chạy. Runner cleanup process/
state thuộc run, giữ evidence. Handoff/logs/decode/frame samples:
`/tmp/poc-T12-handoff-20261011/`. Không có user-owned changes ban đầu.
Task chỉ commit/push sau review/validation; session task đóng sau handoff.
