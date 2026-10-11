---
id: VERIFY-T14-HISTORY-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T14 — Lịch sử triển khai

## Outcome và phạm vi

VI-02/04/05 trên branch `refactor_ui`; T01/T10 DONE, working tree sạch khi bắt
đầu. Codex cập nhật UC-09 UI screens/states trước code. Claude implement qua
`clauded` rồi `clauded -p` trong session `codex-T14-20261011`, không sửa docs,
commit/push. Phản hồi review ban đầu chưa được giao diện tương tác nhận; Codex
chuyển sang print mode trong cùng session để hoàn tất review.

DeploymentHistoryPage dùng tiếng Việt cho tiêu đề, filters, status/action,
loading/empty/error/retry, actor thiếu và locale thời gian vi-VN. Enum query,
IDs, staging/production, dữ liệu và thứ tự server giữ nguyên. GET scoped theo
Application/Environment; mở detail giữ đúng ID. Không đổi backend/API hoặc
trang detail (T15), không thêm so sánh/live refresh. Code map và requirements
hiện có vẫn đúng; không có design deviation hoặc thay đổi implemented scope.

## Checks và review

- Claude V-FE exit 0: typecheck/lint/356 tests (20 files)/build; logs ở
  `/tmp/poc-T14-handoff-20261011/`. File `vfe2-status-final.txt` ghi lint 1 từ
  lượt trước sửa; report ghi lượt sửa exit 0. Codex độc lập xác nhận lint cuối
  exit 0, full V-FE trước review (354 tests) exit 0, rồi 23 Deployment tests
  cuối exit 0. Build có cảnh báo chunk >500 kB hiện hữu.
- Tests kiểm label/enum, time/actor, thứ tự, encoded detail ID, safe raw-error
  suppression, retry/GET-only và late filter/Application/Environment replies
  cùng late errors. Claude thử bỏ cancellation guards: 4 tests fail, sau đó
  khôi phục guards; Codex review final diff và tests.
- Codex syntax-check mjs/wrapper, docs checker và whitespace exit 0.
  Không sửa Go/SQL/PlantUML nên không chạy Go/conformance/SQL/live checks.
- Review yêu cầu thay timer bằng explicit gate: FAILED chỉ được nhả sau ALL
  tải xong, chờ response finished trước assertions; không dùng sleep thay sync.
  Claude từng sửa nhầm anchor T02 trong vòng review, đã khôi phục từ HEAD;
  final e2e diff chỉ import previewAndDeploy và scenario T14.

## Browser evidence

Primary `/tmp/poc-T14-20261011200321`, result PASS; 12 assertions, 15 marks,
4 mock activations; raw/captioned MP4, SRT/ASS, assertions, marks, mocks/run JSON
và frames được giữ. Captioned video 1440×996, 214.6 giây. Codex full decode
raw/captioned exit 0; xem frames history-real/all-statuses/loading và frame
captioned tại 175 giây: cursor rõ, UI không trống, lỗi an toàn, phụ đề Việt ở
strip dưới không che UI, không thấy credential.

Product UI dùng backend local với fake adapters/fake deliverer: đăng nhập,
tạo app, thêm api/web, Preview/Deploy tạo hai Deployment SUCCEEDED, history
đúng thứ tự, server filters SUCCEEDED/FAILED, mở detail đúng ID, production
rỗng. Sau Deploy, history/filter/reload/detail không gửi non-GET requests và
backend vẫn có hai Deployments. API reads chỉ phục vụ assertions.

Projection mocks cho đủ năm trạng thái/ba hành động/actor thiếu, 500 một lần
với secret-like text giả (retry quay về local backend), explicit-gate FAILED
late response và DEPLOYING empty. Mocks không persist. “Backend thật” trong
caption nghĩa là HTTP local với fake adapters, không phải providers thật.
Browser không kiểm nội dung detail tiếng Việt; thuộc T15.

T01 regression cuối PASS tại `/tmp/poc-T14-20261011200746-T01`.
Run đầu `/tmp/poc-T14-20261011194349` FAIL vì assertion nhầm actor username
thay actorRef ID; giữ evidence. Run hai `/tmp/poc-T14-20261011194633` PASS nhưng
late-response dùng timer; giữ và thay bằng primary sau review. Không xóa lượt
fail. Logs/report ngoài Git: `/tmp/poc-T14-handoff-20261011/`.

## External và handoff

Không đọc .env/credential thật hoặc mutate retained DB/Vault/K8S-4F/AWS.
Runner cleanup process/state disposable của run; evidence không upload.
Không có user-owned changes ban đầu. Codex commit/push task với prefix T14
và đóng session sở hữu sau validation; tra SHA bằng `git log --grep '^T14:'`.
