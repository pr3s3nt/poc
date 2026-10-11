---
id: VERIFY-T11-APPLICATIONS-LIST-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T11 — Danh sách ứng dụng

## Outcome và phạm vi

VI-02/04/05 trên branch `refactor_ui`, T01/T10 DONE. Working tree sạch khi
bắt đầu, không có user-owned changes. Codex chốt UC-01 UI screens/states trước
code; Claude implement bằng `clauded -p` trong tmux riêng `codex-T11-20261011`,
không sửa docs/commit/push. Codex review và kiểm chứng độc lập.

ApplicationsPage dùng tiếng Việt cho heading, hướng dẫn, cards, create CTA,
loading/empty/error/retry và aria-label. Nhãn môi trường giữ `staging`/
`production`. Tên, endpoint, thứ tự dữ liệu, payload và create/card routes giữ
nguyên. Nhãn “Sẵn sàng cấu hình” chỉ Việt hóa status presentation có sẵn,
không thêm cách suy trạng thái. Không thêm search/filter hay đổi API/backend.
Application home và code-based errors vẫn thuộc T12/T20.

App tests kiểm nhiều ứng dụng/thứ tự/card route, loading/empty/error/retry,
hai create CTA độc lập và initial deep link. Shared applications locators,
human helper và các e2e selector cũ được cập nhật; không chạy live runners.
Scenario T11 giữ assertions, marks và video cả khi fail.

## Review và validation

- Codex yêu cầu sửa test render App hai lần thành hai case độc lập; chỉ có
  một form mỗi case. Test initial deep link được đặt tên đúng coverage; browser
  kiểm reload thật. Raw-error assertions dùng substring/regex để tránh false pass.
- Codex đồng bộ lowercase staging/production trên cards theo kế hoạch.
- Claude V-FE đầu pass 323 tests; sau review pass 324 tests/20 files.
- Codex độc lập `cd frontend && npm run typecheck && npm run lint && npm test && npm run build` exit 0, 324 tests/20 files. Build có cảnh báo Vite chunk >500 kB.
  Build log tại `/tmp/poc-T11-handoff-20261011/codex-build.log`.
- `bash -n backend/test/integration/refactor-ui-local.sh` exit 0; Claude
  `node --check` mọi mjs đổi exit 0. Wrapper generic không cần sửa để dispatch T11.
- V-D/docs checker và whitespace được kiểm trước commit.

## Browser evidence và giới hạn

T11 PASS cuối tại `/tmp/poc-refactor-T11-vfix-20261011174002/`, run ID
`refactor-20261011104003-17000`. Có `T11-raw.mp4`, `T11-captioned.mp4`,
`captions.srt/ass`, `marks.json`, `assertions.txt`, `result.txt`, `run.json`,
`frames/`. 8 assertions và 9 phase marks; captioned 1440×996, 87.60 giây.
T01 regression sau review PASS tại `/tmp/poc-refactor-T01-vfix-20261011174211/`.

Codex full decode cả hai video exit 0; logs `codex-decode-T11-vfix.log` và
`codex-decode-T01-vfix.log` dưới handoff. Review frames trạng thái rỗng,
list nhiều, loading skeleton, lỗi/retry và card/home; browser/cursor rõ,
phụ đề Việt ở strip dưới không che UI, không hiện password. Review phát hiện mốc captions/marks lấy sau early-exit wait 1.5 giây của
recorder, khiến phụ đề sớm so với video. Claude thêm spawnedAtMs ở video helper,
refactor runner dùng mốc trước spawn, chuyển cue list sau click navigation và
kéo loading hold lên 4.5 giây để dễ đọc; không đổi API gọi helper hoặc product.
Codex syntax-check mọi mjs đổi exit 0, review video mới ở 15/60/64/69/75 giây
và T01 ở 34 giây: nội dung/caption khớp, cursor và các states rõ. Frame samples
lưu `/tmp/poc-T11-handoff-20261011/vfix-frames/`. Còn độ trễ ffmpeg init nhỏ
(<0.5 giây theo Claude), không ảnh hưởng các frame đã đối chiếu. Claude chạy
lại App tests 33 pass, typecheck/lint/build và hai browser scenarios sau fix;
product code không đổi sau full V-FE độc lập, chỉ chỉnh indentation test.
Video/helper fix report: `/tmp/poc-T11-handoff-20261011/video-review-report.txt`.

Adapter fake, profile test, JSON state disposable; không phải live adapter.
Ứng dụng seed và hai ứng dụng tạo bằng form đi qua backend local; POST chỉ
name/subdomain. Header create CTA được dùng tạo ứng dụng; empty CTA kiểm mở
form từ projection rỗng. API GET chỉ assertions. GET /applications được mock
một lần rỗng, một lần giữ khoảng 4.5 giây và một lần 500 để chứng minh UI
loading/empty/error/explicit retry. Không claim backend failure/empty behavior.
List giữ App state hiện có; reload đọc lại thứ tự backend. Mở card/reload
chỉ chứng minh route/identity, không chứng minh chức năng Application home T12.
Card được mở là acceptance seed: home có lỗi workload/recent-deployments do
fixture seed thiếu related data, ngoài assertion/scope T11; không claim home
healthy từ lượt quay này.

## Failure retention và cleanup

Lượt T11 01–04 fail, giữ `/tmp/poc-T11-evidence-20261011-01` đến `-04` cùng
logs dưới handoff: giả định backend chưa seed, stale list sau navigation,
alert không có accessible name, và nhầm API key với frontend id. Lượt 05 pass
trước review tại `/tmp/poc-T11-evidence-20261011-05`; T01 ban đầu pass tại
`/tmp/poc-T11-evidence-20261011-T01-01`.

Lượt review đầu fail preflight vì dist cũ hơn src:
`/tmp/poc-refactor-T11-fix-20261011173308/`, log `/tmp/t11-20261011173308.log`;
chưa có browser/video vì chưa bắt đầu scenario. Rebuild rồi chạy path mới.
Không xóa hoặc ghi đè artifacts fail. Các lượt pass trước video fix cũng giữ:
`/tmp/poc-refactor-T11-fix-20261011173325` và
`/tmp/poc-refactor-T01-reg-20261011173515`; không dùng chúng làm video cuối.

Wrapper cleanup process/state thuộc run, giữ evidence ngoài Git; không đọc
.env/credentials thật, nối retained DB/Vault/K8S-4F/AWS hoặc upload evidence.
Go/conformance/SQL/live checks không chạy vì chỉ đổi frontend/e2e và external
verification ngoài scope. Không đổi PlantUML. Handoff/logs:
`/tmp/poc-T11-handoff-20261011/`. Session task đóng sau handoff; commit/push
chỉ T11 sau review/validation.
