---
id: VERIFY-T13-SCORE-PREVIEW-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T13 — Xem trước Score standalone

## Outcome và phạm vi

VI-02/04/05 trên branch `refactor_ui`; T01/T10 DONE, working tree sạch khi bắt
đầu. Codex cập nhật UC-05 UI screens/states trước code; Claude implement qua
`clauded -p` trong session riêng `codex-T13-20261011`, không sửa docs/commit/push.

ScorePreviewPage dùng tiếng Việt cho form, tab aria-label, hướng dẫn, validation,
loading, kết quả, bindings, graph, tài liệu và lỗi/retry. Tab giữ đúng
`staging`/`production`; enum `deploy`/`update`/`remove`, YAML/JSON keys,
placeholder, ID, graph descriptors/kinds/reasons, JSON-patch ops và dữ liệu
Delta/Candidate/matches/batches giữ nguyên. Parser chỉ đổi thông báo, bỏ raw
YAML error text; quy tắc một document/object, alias limit và JSON round-trip
không đổi. API lỗi dùng fallback Việt an toàn theo status/field, không echo
raw backend text hoặc giá trị gửi lên; mapping chi tiết theo code thuộc T20.
Preview không lưu draft/Deployment/provision; input/scope generations giữ nguyên.

## Review và checks

- Codex review code/parser/tests và scope, yêu cầu lowercase tabs cùng assertion,
  sửa hint và kết thúc video bằng đăng xuất thật sau mock hết phiên.
- Frontend của Claude cuối exit 0: typecheck/lint/344 tests (20 files)/build,
  log `/tmp/poc-T13-handoff-20261011/final-fe.log`. Codex độc lập chạy full
  frontend trước chỉnh tab (343 tests, exit 0), rồi 20 ScorePreviewPage tests
  sau chỉnh tab (exit 0). Build còn cảnh báo chunk >500 kB.
- Tests kiểm payload add/update/remove, editor ẩn đúng action, safe errors
  400/413/404/422/network/retry, stale/late input/scope, parser alias/object/
  document invariants và không có nút lưu/triển khai.
- Runner selectors UC-05/application-connection/UC-02..04 được cập nhật theo
  trang đã Việt hóa; không sửa backend product hoặc shared validator.
- Codex độc lập syntax-check wrapper và bốn mjs, V-D/docs checker và whitespace
  exit 0. Review cuối khôi phục accidental phase mark của T10 về HEAD; không
  còn đổi scenario khác. Không đổi PlantUML.
- Broad UC-02..04 video regression fail trước Preview ở locator
  `Điều kiện 1 Class`: runner không chọn criteria advanced. Đối chiếu HEAD thấy
  cùng lời gọi cũ và CriteriaEditor chỉ render field trong advanced; phần này
  không đổi trong T13. Không claim broad runner pass; preview selectors của
  runner này chỉ syntax-check. Evidence fail được giữ ngoài Git.

## Browser evidence và giới hạn

Primary: `/tmp/poc-T13-evidence-20261011-final-t13/`, run ID
`refactor-20261011121213-9757`, exit 0, 16 PASS assertions và 18 phase marks.
Raw/captioned MP4, captions.srt/ass, marks.json, assertions.txt/result.txt,
run.json/mocks.json và frames được giữ. Captioned video 1440×996, 229.7 giây.
Codex full decode raw/captioned exit 0; review frames 30/55/69/125/129/132/146/
190/227 giây: form/tab, parse/unconfigured errors, result/graph/docs, retry,
scope và final sign-out đúng; cursor rõ, captions strip dưới không che UI và
không thấy credential. Frame samples ở handoff ngoài Git.

Product qua UI với backend fake local/state JSON disposable: Developer sign-in,
role navigation (ẩn mục nền tảng, không có role gate riêng của Preview), tạo ứng
dụng, nhập form trống/YAML sai, unconfigured 422, chọn/lưu connection ở Settings,
server 400 ID không khớp, xem graph/Delta/Candidate/batches/bindings/documents.
API reads kiểm zero drafts/deployments; request ngoài score-preview sau setup
chỉ là PUT connection qua UI. Remove kiểm payload, không claim successful remove
vì Workload chưa deploy. Update/no-op có unit và UC-05 regression coverage.

Projection mocks ghi trong captions/assertions/mocks.json: 413, 404, 500 một lần
(retry nhận backend local response), giữ request 3.5 giây rồi forward để kiểm
late scope, và 401 expired session. Caption “backend thật” nghĩa là HTTP backend
local với fake adapters, không phải live providers. Đăng nhập lại/đăng xuất cuối
qua UI thật. No-mutation proof giới hạn ở UI request/API reads; backend read-only
invariants hiện có không đổi, không claim live cluster/cloud verification.

Lượt T13 đầu `/tmp/poc-T13-evidence-20261011-a` pass 15 assertions nhưng fail
video validator vì không có final signed-out mark; giữ nguyên. Lượt hai `-b`
pass nhưng mark hết phiên được đặt tên signed-out; sau review sửa thành
session-expired rồi sign-in/sign-out thật và chạy lại primary. UC-05 lượt đầu
fail stale selector `/^New/`, giữ `-uc05-fail1`; lượt sửa selectors pass ở `-uc05`.
Broad UC-02..04 fail giữ `-uc0204`. Không xóa/ghi đè các run.

Final regressions exit 0, ghi status/timestamp tại
`/tmp/poc-T13-handoff-20261011/final-status.txt`: T01 ở
`/tmp/poc-T13-evidence-20261011-final-t01`; UC-05 ở
`/tmp/poc-T13-evidence-20261011-final-uc05` (20 marks, 3 previews, no-op update
của Workload đã deploy bằng fake deliverer, 237.7 giây). Application-connection
local create/restart pass, run ID `conn-20261011122158-15593`, log
`/tmp/poc-T13-handoff-20261011/final-appconn.log`; không claim live binding.

## External và handoff

Không đọc .env/credentials thật, không mutate retained DB/Vault/K8S-4F/AWS.
Go/conformance/SQL/live checks không chạy vì frontend/e2e-only; fake runners có
build backend local nhưng không sửa product Go. Cleanup chỉ process/state thuộc
run, evidence ngoài Git không upload. Logs/handoff:
`/tmp/poc-T13-handoff-20261011/`. Không có user-owned changes ban đầu.
