---
id: VERIFY-T05-SECRET-STORES-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T05 — Trang Kho bí mật

## Outcome và scope

SS-01..04, UI-01, VI-02/04/05 trên branch `refactor_ui`; T01 đã DONE.
Working tree sạch khi bắt đầu, không user-owned changes. Codex chốt UC-04 UI
screens/states/api-mapping trước code; Claude implement qua `clauded` trong
session riêng `codex-T05-20261011`, không sửa docs/commit/push.

Trang dùng tiếng Việt, mount/CA nâng cao với tóm tắt hiệu lực mặc định
`kv`/`kubernetes`. Checkbox cùng địa chỉ mặc định tắt, payload theo backend khi
bật; tắt khôi phục draft workload riêng. Local URL/mount/name/token/PEM shape/
UTF-8 byte/envelope validation chặn POST; backend vẫn parse X.509/verify thật.
Đóng nâng cao giữ dữ liệu; submit có lỗi nâng cao mở lại để thấy lỗi.
Server failure xóa token, giữ metadata; unknown outcome không tự resubmit.
List reload lỗi không phủ nhận đăng ký thành công. Danh sách có cả hai địa chỉ,
xác minh backend và Kubernetes auth riêng; READY không nghĩa deploy-ready.
Không trả/hiện token/ref, không đổi API/Developer choices/backend product.
Error status guidance cố định Việt; stable code mapping thuộc T20; shell thuộc T10.

Code/test: SecretStoresPage và tests, CSS chỉ scoped secret-store-form,
refactor scenario T05 và locators, secret-scan tests. Hai runner live cũ
(environment-stores-kind-human/k8s4f-dev-human) đổi selectors/open advanced,
chỉ syntax checked. Wrapper T05 dùng in-memory state + explicit memory
credential store, không persistent snapshot/Vault; scenarios khác giữ JSON.

## Checks

- V-FE tất cả exit 0: typecheck/lint/test/build, 258 tests/20 files.
  Logs tại `/tmp/poc-T05-handoff-20261011/logs/`: `exit-codes.txt`,
  `typecheck.log`, `lint.log`, `test.log`, `build.log`. Codex chạy độc lập cùng
  suite exit 0 (258 tests), build log `codex-build.log`.
- V-UI(T05) PASS: run `refactor-20261011034510-10361`,
  `/tmp/poc-T05-handoff-20261011/evidence-run4`, 10 assertion records/13 marks.
- T01 locator regression PASS: `evidence-T01-regression`, run
  `refactor-20261011034819-20073`; log `logs/vui-T01.log`.
- Codex bash -n wrapper, node --check refactor và hai live scripts, docs checker
  và git diff --check pass. T01 captioned video full decode exit 0.
- Go suite/conformance/SQL/kind/AWS không chạy vì không đổi backend product;
  runner build local Go binary. Không SQL opt-in hoặc external mutation.

## Evidence và review

Adapters **fake**, state và credential store memory disposable. Không đọc .env,
retained DB/cluster/cloud hoặc token thật. Product steps sign-in/form/submit/
retry/sign-out qua UI; API GET chỉ assertion. POST 422 token bị từ chối và
201 đăng ký là fake backend thật; Kho A auth CONFIGURED từ backend fake.
Playwright GET interception giả lập một lần reload 500 và Kho B auth ABSENT;
không claim verifier/Vault thật. PEM/byte boundaries, unknown metadata,
network lost-response và các HTTP statuses khác kiểm bằng unit, không video.

Run4 giữ `T05-raw.mp4` 1440×900 và `T05-captioned.mp4` 1440×996, khoảng
156 giây, captions.srt/ass, marks.json/assertions.txt/result.txt/run.json,
ffprobe.json/frames. Codex full decode raw/captioned exit 0, review frame
advanced/defaults, token-cleared error và auth list: UI/cursor rõ, caption strip
ở dưới browser không che controls. Synthetic token luôn được che; không lộ
credential thật. Review PNG ngoài Git tại handoff root: review-token-cleared,
review-summary, review-auth-list. Không upload evidence.

Ba lượt fail giữ nguyên `evidence-run1/2/3` cùng video/marks/assertions:
run1 hidden bị CSS display:grid ghi đè (sửa scoped hidden rule); run2 runner
credential store none gây 503 (T05 explicit memory/no persistent state); run3
helper chèn /v1 giữa URL (sửa scenario replace toàn URL). Không thay assertion
để hợp thức hóa failure. Wrapper cleanup chỉ process/state của run; tài nguyên
retained không bị chạm. Codex đóng tmux task sau review/validation/commit/push,
giữ sessions có sẵn của người dùng.
