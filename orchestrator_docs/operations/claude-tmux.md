---
id: RUNBOOK-CLAUDE-TMUX
artifact: operations-runbook
status: current
last_reviewed: 2026-10-14
---

# Điều phối Claude, phương án thay thế và kiểm chứng local

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
Nhịp theo dõi 15 phút, kiểm sớm khi lỗi/hoàn tất; chỉ dọn session task sau
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
