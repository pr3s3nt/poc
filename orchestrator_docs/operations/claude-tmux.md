---
id: RUNBOOK-CLAUDE-TMUX
artifact: operations-runbook
status: current
last_reviewed: 2026-10-19
---

# Điều phối Claude, phương án thay thế và kiểm chứng local

## Quy trình hiện hành — 2026-10-19

Agent trực tiếp implement/self-review/validate/runtime/commit/push theo quyền;
không coding agent phụ, không resume writer cũ. Nghiệm thu issue #4 chỉ luồng
chính theo [runbook acceptance](terraform-v2-acceptance.md), giữ secret/ownership/
cleanup và tests đã có. Không thêm test ngoại lệ để bù. Focused impact checks và
source-bound reuse; chỉ chờ thì15phút/lần, early error/completion/user request.
Quy trình writer phía dưới là lịch sử, không còn mặc định.

## Luồng lịch sử — quyết định 2026-10-17

Người dùng yêu cầu Codex điều khiển Claude local qua `tmux` và `clauded`; áp
dụng kể cả khi cloud còn quota. Codex docs/review/checks/commit/push/merge và
chạy thật; Claude chỉ viết code/test trong checkout riêng. Không tự đổi model
hoặc thay Claude bằng Codex cho task mới; ngoại lệ `gpt-6-luna` vẫn chỉ dành M04.

1. Kiểm git status/head và các tmux session hiện có; giữ nguyên phiên người dùng.
   Tạo checkout/task branch và tmux session mới, không cho writer sửa checkout
   điều phối hoặc docs/AGENTS; không cho local writer commit/push.
2. Mở interactive shell, kiểm alias `clauded` thực tế rồi chạy alias đó trong
   session task. Giao brief đầy đủ: scope, nguồn chuẩn, acceptance/checks, paths,
   private/external-state boundaries và cách bàn giao khi hoàn tất/lỗi.
3. Xác nhận model và effective permission mode từ CLI UI, ghi session ID khi có;
   không suy từ `--dangerously-skip-permissions`. Lưu checkout/branch/base/head,
   tmux target, brief path và completion/error markers trong Current handoff.
4. Theo dõi mỗi 15 phút; chỉ đọc sớm khi lỗi/completion hoặc người dùng yêu cầu.
   Completion marker chỉ hợp lệ cùng diff/handoff/checks thực tế. Nếu quota hoặc
   auth chặn, giữ worktree/session, ghi handoff và làm phần độc lập được phép.
5. Codex review diff, tự chạy PostgreSQL thật REQUIRE_POSTGRES=1, Go/frontend/docs
   checks, runtime và headed recording theo phần dưới; không chấp nhận skips
   trong checks bắt buộc. Gửi findings vào cùng session local để writer sửa.
6. Sau review/checks, Codex commit code/test trong checkout task, merge task-only
   vào nhánh điều phối và cập nhật docs/issue rồi push. Không commit công việc
   có sẵn ngoài task. Giữ evidence/cleanup và không đóng tmux của người dùng.

Nếu chuyển một task cloud đang mở sang local, gửi yêu cầu cloud dừng, kiểm nhánh
đã push và lưu session/handoff. Không coi `ok:true` là xác nhận remote đã dừng;
không merge cloud output không được review hoặc chạy hai writer trên cùng checkout.

## Ngoại lệ riêng issue #4 V04–V06 — quyết định2026-10-18

Khi Claude local hết hạn mức, người dùng cho tiếp tục bằng Codex CLI trong
checkout và tmux mới, model `gpt-6-luna`, reasoning `medium`, giữ nhịp15phút.
Tên ban đầu `gpt-6.1-luna` bị CLI/API từ chối; người dùng đã xác nhận đổi đúng
`gpt-6-luna`, không phải model tự thay. Hủy auto-continue Claude và xác minh idle
trước khi writer thay thế làm việc để tránh hai writer. Giữ session/checkout/
snapshots gốc, chuyển partial work có receipt sang checkout riêng và giao toàn
scope/gate/nguồn chuẩn/findings. Ghi actual CLI model, effort, permission mode,
session ID; không suy từ launch flags. Writer thay thế vẫn chỉ code/test/scripts,
không docs/AGENTS, commit/push hay external mutation. Codex điều phối review và
validation độc lập. Ngoại lệ này không đổi luồng mặc định cho task khác.

## Quy trình cloud trước đây — chỉ dùng lại khi người dùng yêu cầu

Quyết định người dùng 2026-10-14 thay clauded/tmux bằng Claude cloud; giữ path
runbook để các liên kết lịch sử còn hợp lệ. Codex quản lý docs/review/checks/
merge và chạy thật local; Claude viết code/test, không sửa docs.

1. Push `refactor_ui` trước khi giao; mỗi task tạo phiên mới từ thư mục repo:
   `claude --cloud "<bàn giao>" --dangerously-skip-permissions`.
   CLI cần terminal tương tác (TTY); không tạo phiên bằng piped stdout/print mode.
2. Giao scope, sources, acceptance criteria, checks; yêu cầu push nhánh riêng
   `claude/m04-<task>`, không push `refactor_ui`, bàn giao ngắn trong commit cuối.
3. Lưu session ID/URL và kiểm chế độ quyền thực tế, không suy từ cờ CLI.
   Không gửi credential/kubeconfig/state/.env; mọi nhánh của repo là public.
4. Phản hồi: `claude -p "<nội dung>" --cloud <session-id> --output-format json`.
   `ok: true` chỉ nghĩa đã gửi. Fetch nhánh mỗi 15 phút (900 giây); đọc sớm khi lỗi,
   completion hoặc yêu cầu người dùng. Chờ theo giới hạn môi trường.
5. Review diff; tự chạy local `REQUIRE_POSTGRES=1 go test ./...`, build,
   frontend checks và docs checks, không chấp nhận skip. Merge khi đạt,
   cập nhật verification/Current handoff/Task order, commit/push và xóa nhánh.
6. Codex chạy thật local trên code Claude: cluster/AWS theo quyền đã giao,
   Playwright headed và video. Gửi lỗi code về cùng phiên để Claude sửa.
   Công cụ lỗi kéo dài: lưu task/bước/session/tài nguyên/việc tiếp theo trước dừng.

## Phương án thay thế khi hết hạn mức — quyết định mới nhất 2026-10-14

Người dùng yêu cầu chuyển khỏi cloud đã hết credit: fetch code đã push, giữ
ID/URL/head và các findings; không giả đã lấy được file cloud chưa push.
Dùng checkout riêng và session tmux riêng của task, chạy `clauded` (alias local
của `claude --dangerously-skip-permissions`). Có thể dùng `--teleport` để lấy
handoff/conversation của phiên cloud; kiểm trạng thái thực tế trước khi ghi
đã phục hồi. Giao brief có nguồn chuẩn, phạm vi, findings chưa sửa và checks.
Claude local chỉ sửa code/test, không đọc credential/kubeconfig/state/.env,
không chạy cluster/AWS, không sửa docs hoặc commit/push. Codex chạy thật.

Nếu Claude local báo hết hạn mức, giữ evidence và chuyển việc code/test còn
lại cho agent Codex với model **gpt-6-luna**, theo quyền người dùng đã cấp.
Codex điều phối vẫn review và chạy kiểm tra độc lập, không đổi sang model khác.
Nhịp theo dõi 15 phút, kiểm sớm khi lỗi/hoàn tất; quyết định 2026-10-16
áp dụng cả lượt chạy thật AWS/Kubernetes, supervisor vẫn chạy độc lập và
không gửi cập nhật lặp lại khi chờ; chỉ dọn session task sau
lưu handoff. Giữ nguyên source order, nghiệm thu, zero-skip và cleanup.

## Playwright recording cho task code/test có browser flow

1. Dùng hoặc mở rộng runner/scenario hiện có trong `frontend/test/e2e/` và
   `backend/test/integration/`; Claude cloud viết code/test; Codex chạy recording local.
   Quay headed browser, ưu tiên toàn cửa sổ gồm tab/address bar với runner Xvfb/
   ffmpeg hiện có. Không cần quay terminal hoặc toàn desktop chứa dữ liệu riêng.
2. Scenario phải phản ánh các bước người dùng thực hiện: sign-in, mở màn hình,
   chọn/nhập form, Save, Preview, Deploy nếu được phép, quan sát kết quả. Hiện cursor/
   click, nhập liệu với tốc độ dễ đọc, pause ngắn ở mỗi bước quan trọng. Chờ UI/API/
   readiness có assertion; pause để người xem đọc không thay thế synchronization.
   Không dùng API seed thay cho thao tác UI được tuyên bố đã chứng minh; setup API
   riêng phải được nêu rõ trong evidence.
3. Ưu tiên phụ đề tiếng Việt ở vùng dưới video theo phase marks: số bước, mục đích,
   thao tác và kết quả thật. Mỗi phụ đề ngắn, đủ thời gian đọc, không che nút/form/
   kết quả. Ưu tiên caption trong bản video xem lại; có thể kèm SRT/VTT đồng bộ.
   Overlay/caption chỉ phục vụ recording, không thêm vào UI sản phẩm. Không đưa
   token, password, kubeconfig hoặc secret values vào phụ đề. Nếu runner chưa hỗ
   trợ caption, Claude bổ sung trong task code/test liên quan hoặc handoff nêu rõ
   recording chưa có phụ đề; tài liệu này không tuyên bố helper đã implement.
4. Giữ recording và phase marks trên cả success/failure, cùng kết quả assertion,
   run ID, adapter mode và cleanup. Video là bằng chứng bổ sung; chỉ pass khi
   assertions/checks tương ứng pass. Fake-adapter video không chứng minh live deploy.
   Test chỉ có unit/CLI thì ghi rõ không có browser flow, không mở rộng scope chỉ
   để quay một video không liên quan.
5. Codex review các frame/đoạn chính và đối chiếu kết quả sau khi chạy local hoàn tất,
   không xem liên tục khi quay. Với runner MP4 hiện có, kiểm tra full decode,
   resolution, phase marks và frame không trống; không cố kéo dài video để đủ
   thời lượng nếu scenario đã đổi. Kiểm tra caption khớp bước/kết quả, không lộ
   credential và không che UI. Evidence ở ngoài Git; handoff link/path video,
   captions, checks và giới hạn. Không tự publish/upload recording khi chưa được
   yêu cầu. Cleanup chỉ các process/session/resource của task theo authorization.

## Runner refactor Console local

Sau khi build frontend, chạy tại repository root, với evidence path mới ở
ngoài repository cho mỗi lần chạy:

```bash
bash backend/test/integration/refactor-ui-local.sh --scenario T01 --headed --captions vi --evidence /tmp/poc-refactor-T01-20261010-01
```

`--scenario` dispatch task đã có scenario; task chưa đăng ký bị từ chối trước
khi khởi động. `--headed` và `--captions vi` là bắt buộc. Wrapper chuyển
`--evidence` sang `ORCH_VIDEO_DIR`; không ghi đè thư mục đã tồn tại. Dùng chung
`video-lib.sh`, helpers human input/recording/captions và fake backend với JSON
state disposable. Không đọc `.env`, không kết nối retained DB/cluster/cloud.
Các thao tác product đi qua UI; API read chỉ phục vụ assertions.

Evidence giữ raw/captioned MP4, captions, marks, assertions và result, kể cả
scenario fail. Phụ đề nằm trong strip dưới cửa sổ browser. Cleanup chỉ process
thuộc run và state tạm; không xóa evidence, không upload. Locator role/label
của trang chưa Việt hóa dùng nhãn hiện tại tới task chuyển trang đó. Khi thêm
scenario, chạy `bash -n` runner và checks frontend tương ứng; review decode,
frame, captions và assertions theo procedure ở trên.

T05 dùng state và credential store `memory` không bền vững, vì memory credential
store chỉ được phép với fake adapters và không có persistent state. Các scenario
khác giữ JSON state disposable. T05 không ghi token ra snapshot hoặc dùng Vault;
registration/verification qua fake backend, projection ABSENT và lỗi GET dùng
mock được ghi riêng trong assertions/evidence.


## Issue4 V04–V06 — direct continuation, 2026-10-19 Bangkok

Latest user instruction explicitly stops the coding agent and assigns Codex
direct implementation through completion, followed by self-review. This task
exception supersedes the Claude/Luna writer split above; it does not change the
default workflow for other tasks. Luna conversation interrupted and background
terminals stopped with CLI acknowledgement; keep its tmux/session, checkout,
partial staged work and provenance. No more writer/model calls. Codex directly
finishes code/tests in isolated task sources, reviews the full result, performs
independent validation/runtime/video/cleanup and commits only complete gates.
All source-of-truth, ownership, external authorization, real PostgreSQL/zero-skip,
recording and staged-removal requirements remain. No user-owned changes or
existing tmux sessions may be removed.
