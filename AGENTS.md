# Project instructions for AI agents

## Purpose and scope

File này quy định cách AI agent làm việc trong toàn repository. Yêu cầu cụ thể
của người dùng quyết định mục tiêu và phạm vi của từng task; tài liệu này không
tự mở rộng phạm vi sang hạng mục roadmap khác.

Tài liệu này không sở hữu product requirement, architecture, project status hay
runbook. Dùng [documentation index](orchestrator_docs/INDEX.md) để tìm nguồn
chuẩn tương ứng.

## Starting a task

Trước khi sửa file:

1. Phân loại task:
   - review/explanation/status: chỉ đọc và báo cáo, không sửa nếu người dùng
     không yêu cầu;
   - diagnosis: xác định nguyên nhân, chỉ sửa khi remediation thuộc yêu cầu;
   - change/build: implement, validate và hand off trong phạm vi được yêu cầu.
2. Chạy `git status --short --branch`; coi mọi thay đổi và file untracked có sẵn
   là công việc của người dùng.
3. Đọc `orchestrator_docs/INDEX.md`, chọn route phù hợp và chỉ nạp bộ tài liệu
   nhỏ nhất đủ cho task.
4. Với thay đổi theo use case, bắt đầu từ
   `orchestrator_docs/usecase/UC-xx/README.md`, rồi đọc specification và artifact
   liên quan.
5. Đọc tài liệu để hiểu intent trước; sau đó đọc code và test để hiểu trạng thái
   thực tế. Không suy product requirement chỉ từ code hiện tại.
6. Dùng change-impact map bên dưới để xác định artifact cần kiểm tra.
7. Nếu một quyết định còn thiếu có thể thay đổi behavior, architecture, data,
   security hoặc scope, dừng phần phụ thuộc và hỏi người dùng.

Chỉ khi người dùng yêu cầu “tiếp tục roadmap” mới đọc active iteration trong
`orchestrator_docs/iterations/`. Không tự động bỏ qua yêu cầu hiện tại để làm
hạng mục roadmap kế tiếp.

## UI, mock và prototype

Mọi task UI, kể cả mock/prototype phục vụ duyệt, phải đọc
[shared UI design](orchestrator_docs/architecture/ui/README.md) trước khi sửa.
Đọc shell, styles, shared components và màn hình sản phẩm tương tự; mở giao diện
đó trên browser trước khi thiết kế. Có thể dùng fixture khi không có backend,
nhưng phải nêu rõ đó là UI sản phẩm với dữ liệu giả. Mock cũ không mặc nhiên là
nguồn chuẩn. Nếu không thể xem browser, ghi rõ giới hạn, không coi chỉ đọc code
là đã đối chiếu trực quan.

Tái sử dụng UI hiện hành; không tự đổi nhận diện, shell, navigation hoặc pattern
chung ngoài phạm vi được yêu cầu. Tuân thủ quy tắc nội dung ngắn gọn trong shared
UI design. Trước bàn giao, đối chiếu trực quan với màn hình tham chiếu và hỏi:
“Nội dung nào có thể bỏ hoặc chuyển vào trợ giúp mà người dùng vẫn hoàn thành
tác vụ?” Ghi nguồn UI đã dùng và kết quả đối chiếu trong handoff.

Quy tắc áp dụng cho UI mới và phần UI đang sửa; không tự mở rộng task sang việc
thiết kế lại hoặc tinh giản mọi màn hình cũ.

## Canonical change order

| Change | Required sequence |
|---|---|
| Required product behavior | Specification → realization/diagram → shared design → code/test → traceability |
| Shared architecture/domain | Architecture/ADR → affected use cases → code/test |
| Database design | `architecture/database/schema.md` → ERD → adapter/migration/test |
| Code defect against correct docs | Giữ canonical docs; sửa code và test |
| Designed but unimplemented behavior | Xác nhận docs đầy đủ → implement → cập nhật current state/code map |
| Behavior-preserving refactor | Chỉ cập nhật code map/path/runbook bị ảnh hưởng |
| Unexplained design/code mismatch | Ghi vào `orchestrator_docs/implementation/deviations.md` trước khi giải quyết |
| Chưa thể quyết định | Ghi backlog nếu task cho phép; không biến option thành requirement |

Không sửa canonical documentation chỉ để hợp thức hóa code hiện tại. Khi intent
thay đổi, sửa nguồn chuẩn trước rồi mới sửa dependent artifacts.

## Source and reference boundaries

- `backend/` chứa Go product module, tests và acceptance workloads.
- `frontend/` là Orchestrator Web Console.
- `e2e/` là nơi lưu script E2E mới, setup và fixtures đi xuyên frontend/backend;
  đọc [hướng dẫn E2E](e2e/README.md) trước khi thêm kịch bản. Dùng `setup/`,
  `scenarios/<scenario-name>/`, `fixtures/`, `lib/` theo mục đích. Unit/component
  tests và Go integration helpers vẫn nằm trong module tương ứng.
- `backend/examples/acceptance-app/` là workload kiểm chứng, không phải
  Web Console.
- `orchestrator_docs/` chứa documentation; không chứa runtime product code.
- `orchestrator_reference/` là reference tracked, read-only. Không sửa hoặc copy
  nguyên domain/code vào sản phẩm. Các local duplicate repository ở root đã
  được retire theo ADR-005 và không được tạo lại.
- Planner, API, executor, CLI, acceptance workload và backend tests dùng Go.
  Web Console dùng React + TypeScript strict. Không thêm Python runtime vào sản
  phẩm.

## End-to-end scripts

Kịch bản E2E mới phải đặt trong `e2e/`, không tiếp tục rải runner mới vào
`frontend/test/e2e/` hoặc `backend/test/integration/`. Mỗi scenario có README
ghi mục tiêu, prerequisites, cách chạy, assertions, side effects và cleanup;
link tới specification/runbook chuẩn. Setup chỉ chuẩn bị platform phải nằm
trong `e2e/setup/`, không được báo là đã nghiệm thu deploy ứng dụng.

Runner cũ giữ nguyên path cho tới task di chuyển riêng hoặc refactor liên quan;
không tự migrate hàng loạt, không copy thành hai bản. Khi di chuyển phải cập
nhật imports, dependencies, lệnh chạy, CI và runbooks liên quan rồi kiểm tra.
Credential, kubeconfig, state, session browser, logs, screenshot và video của
lượt chạy lưu ngoài Git. Có script không đồng nghĩa được phép chạy mutation
lên staging/Kubernetes/cloud; tiếp tục áp dụng quyền external và ownership cleanup.

## Change-impact map

“Inspect” không đồng nghĩa với “edit”; chỉ sửa artifact thực sự bị ảnh hưởng.

| Change | At minimum, inspect |
|---|---|
| Use-case flow/business rule | Specification, realization, sequence, contracts, traceability, tests |
| Operation/participant | Realization, sequence, VOPC, shared classes, contracts |
| Domain object/relationship | Domain model, schema, persistence, tests |
| State/transition | Specification, state machine, contracts, schema, code, tests |
| API/UI | Specification, realization, handler/client, shared UI design, UI states, tests; với UI/mock: shell, styles, components và browser reference |
| Planner contract | UC-05/06/07/08, planner reference, conformance tests, traceability |
| External provider | ADR/architecture, adapter, configuration, runbook, verification |
| Source path/module name | Imports, build tools, runbooks, code map, links |
| Implemented scope | `CURRENT_STATE.md`, code map/deviations, tests, verification |
| Documentation structure | `INDEX.md`, this file, `DOCUMENTATION_RULES.md`, checker/CI |

## Repository and external-state safety

1. Không sửa, xóa, revert hoặc commit thay đổi có sẵn ngoài task.
2. Không đọc/hiển thị credential, secret, Terraform state hoặc local `.env` nếu
   task không thực sự cần.
3. Thay đổi code không tự cấp quyền deploy hoặc mutate cluster/cloud.
4. Chỉ chạy kind/AWS integration khi người dùng đặt external verification vào
   scope. AWS run phải có preflight, run ID/tag, cost estimate, cleanup trap và
   xác minh cleanup theo `orchestrator_docs/operations/aws.md`.
5. Không commit `node_modules`, `dist`, coverage, binary, kubeconfig, local
   snapshot hoặc Terraform state.
6. Commit/push theo authorization của task hoặc quy trình Codex/Claude bên dưới; không commit thay đổi ngoài task.

## Validation

Chạy validation tương ứng với phạm vi thay đổi:

| Scope | Minimum validation |
|---|---|
| Documentation | `python3 scripts/check_docs.py` và `git diff --check` |
| Go backend/product | `cd backend && go test ./... && go build ./...` |
| Web Console | `cd frontend && npm run typecheck && npm run lint && npm test && npm run build` |
| Shell | `bash -n <changed scripts>` |
| PlantUML | checker tài liệu; CI yêu cầu PlantUML |
| Renamed path/ID | Search toàn repository để tìm stale references |

Test làm thay đổi database, Kubernetes hoặc cloud chỉ chạy khi môi trường có sẵn
và external mutation đã được cho phép.

## Definition of done and handoff

Task chỉ hoàn thành khi outcome đã tồn tại, canonical docs/code/test/traceability
nhất quán hoặc deviation còn lại được ghi rõ, validation phù hợp đã pass, và
không làm mất công việc có sẵn của người dùng.

Handoff phải nêu ngắn gọn: outcome, vùng thay đổi, checks đã chạy, checks bỏ qua
và lý do, cùng mọi user-owned change còn lại.

## Quy trình hiện hành — quyết định người dùng 2026-10-19

Agent nhận task tự code/test/scripts/docs, tự review/check/runtime và commit/push
theo quyền của task. Không khởi tạo hoặc giao coding agent phụ trừ khi người dùng
yêu cầu. Giữ work/session cũ, không tự đóng tmux người dùng.

Issue #4 V04–V06 nghiệm thu thật chỉ luồng chính: đăng ký → Preview → deploy →
app/jobs/PostgreSQL/Redis → valid update/restart/reuse → cleanup. Bỏ intentional
negative paths khỏi runtime/gate; giữ tests đã có và yêu cầu pass, không thêm
tests ngoại lệ để thay thế. Giữ product guards, secret không lộ, ownership/UID/API
cleanup bảo vệ shared/preexisting resources. Focused checks theo impact, reuse
nguồn/config/environment/assertions không đổi; không fullsuite checkpoint.
Khi chỉ chờ runtime quan sát15phút/lần, sớm khi error/completion/user request.
Quyết định bổ sung19-10 cho issue4: nghiệm thu bằng browser E2E chạy nhanh,
không bắt buộc thao tác chậm/cursor/pause mô phỏng người, recording, captions,
video review/upload/player/download. Giữ assertions nghiệp vụ, technical reads,
secret protection, automated checks theo impact và ownership/API cleanup.
Lượt đã khởi chạy theo chế độ cũ được hoàn tất/tái sử dụng; không rerun chỉ để
đổi chế độ evidence.
Các quy trình writer dưới đây là lịch sử, không ghi đè quyết định này.

## Codex điều phối và Claude thực hiện code — lịch sử

### Quyết định lịch sử — 2026-10-17: Claude local qua tmux

Người dùng yêu cầu từ nay Codex điều khiển Claude trên máy này qua `tmux` và
`clauded`, sau đó xác nhận tiếp tục local cho V01. Đây là luồng mặc định mới,
không phụ thuộc việc cloud có còn quota hay không. `clauded` là alias local;
kiểm tra alias thực tế, không tự thay bằng model hoặc công cụ khác.

Mỗi task dùng checkout và session tmux riêng. Claude local chỉ sửa code/test
trong checkout task, không sửa docs/AGENTS, không commit/push hoặc sửa checkout
điều phối. Codex sửa docs, giao nguồn chuẩn/scope/checks, review diff và tự chạy
validation/runtime, rồi commit/push/merge task-only. Không đóng session tmux có
sẵn của người dùng. Lưu session ID, model và mode quyền thực tế từ CLI, nhánh,
base/head và handoff; không suy mode từ launch flag. Theo dõi bình thường mỗi
15 phút, đọc sớm khi lỗi/hoàn tất hoặc người dùng yêu cầu. Giữ toàn bộ yêu cầu
PostgreSQL thật, zero-skip, recording, quyền external và cleanup bên dưới.

Quy trình cloud trong phần lịch sử bên dưới không còn là mặc định và không
cấp quyền chạy đồng thời writer cloud/local. Cloud chỉ dùng lại khi người dùng
yêu cầu. Quyền thay Claude bằng Codex `gpt-6-luna` trước đây vẫn chỉ dành M04,
không tự mở rộng sang V01 hoặc task mới.

### Các quyết định và quy trình cloud M04 — lịch sử, giữ provenance

### Quyết định M04 mới nhất — 2026-10-15

Người dùng cho tiếp tục đến hết I04-12, không cần duyệt giữa chặng hoặc duyệt
cuối; hoàn tất gửi báo cáo. Cho dọn ngoại lệ đúng namespace
`orch-v2-test-a01-20261014-r2`, UID
`a76ad236-5a87-44a2-b4bc-8c0040e5ba4e`, sau khi xác minh lại. Quyền ngoại lệ
này chỉ dành cho namespace/UID đó, không cấp quyền bypass guard nói chung.

Quyết định bổ sung 2026-10-15: khi kiểm tra hoặc scenario thất bại, Codex phải
tìm nguyên nhân, giao writer sửa code/test, review và kiểm tra độc lập rồi chạy
lại sau khi cleanup lượt trước được xác minh sạch. Bỏ giới hạn một lần retry
scenario; không dừng chỉ vì lượt retry thất bại. Không lặp lại lỗi chưa sửa,
giảm assertions hoặc vượt ownership/quyền external để có kết quả pass.
Nếu thiếu quyết định/quyền, công cụ lỗi kéo dài, cleanup chưa chứng minh sạch
hoặc sắp hết hạn mức thì ghi handoff và dừng phần phụ thuộc; vẫn làm chẩn đoán,
sửa lỗi và các kiểm tra độc lập có thể thực hiện an toàn.

Khi Claude cloud hết hạn mức, lấy code đã push về checkout riêng và lưu
handoff/session cloud. Tiếp tục code/test bằng Claude local qua tmux với
`clauded`; Codex giữ vai trò docs/review/checks/merge và chạy thật. Nếu Claude
local tiếp tục hết hạn mức, Codex được giao code/test cho agent Codex dùng
model `gpt-6-luna`. Đây là quyền thay thế do người dùng cấp cho M04, ưu tiên
hơn các câu cấm thay Claude bằng Codex bên dưới. Không tự đổi model khác.
Theo dõi tiến độ mỗi 15 phút; chỉ đọc sớm khi lỗi/hoàn tất hoặc người dùng yêu
cầu. Quyết định 2026-10-16 áp dụng nhịp này cả cho lượt chạy thật AWS/Kubernetes;
supervisor vẫn chạy độc lập, không gửi cập nhật lặp lại khi chỉ đang chờ.
Giữ mọi yêu cầu validation, không skip, quyền external và cleanup.
Claude local/agent thay thế chỉ sửa code/test trong checkout task riêng,
không sửa docs/commit/push nhánh điều phối; Codex commit sau review và checks.
Không đóng tmux của người dùng, không ghi credential vào handoff hoặc Git.


Codex sửa canonical documentation và điều phối; Claude cloud viết code/test.
Codex không tự viết code sản phẩm/test. Mỗi task tạo phiên cloud mới từ repo
sau khi push nhánh điều phối; giao phạm vi, nguồn chuẩn, tiêu chí nghiệm thu và
checks. Claude được commit/push nhánh riêng `claude/m04-<task>`, không sửa docs
hoặc push `refactor_ui`; commit cuối chứa bàn giao ngắn. Không gửi credential,
kubeconfig, Terraform state hoặc `.env` vào cloud; repo và các nhánh đều public.

Codex lưu session ID/URL và chế độ quyền thực tế vào Current handoff, fetch
nhánh Claude mỗi **15 phút (900 giây)** khi đang chạy bình thường. Phản hồi gửi cùng phiên;
`ok: true` chỉ xác nhận gửi, không xác nhận hoàn tất. Codex review diff, tự chạy
lại checks local với PostgreSQL thật (`REQUIRE_POSTGRES=1`), build, frontend và
docs checks; không chấp nhận skip. Khi đạt, merge vào `refactor_ui`, cập nhật
docs, commit/push task-only rồi xóa nhánh Claude. Không force-push hoặc stage
công việc có sẵn ngoài task. Nếu cloud lỗi, báo và tiếp tục phần độc lập;
không thay Claude bằng Codex coding.

Codex tự chạy deploy k8s-4f, tạo/xóa AWS và Playwright headed/quay video local
trên code Claude viết khi được giao quyền. Lỗi code được gửi lại Claude cloud.
Browser flow có cursor/click rõ, nhập liệu và pause đủ đọc, phụ đề Việt dưới
video, phase marks/assertions; giữ evidence cả khi fail và phân biệt thật/fake.
Không lộ secret. Unit/CLI không có browser scenario thì ghi rõ lý do.

Với change/build, Codex commit/push thay đổi thuộc task sau review/validation
thành công, trừ chỉ dẫn riêng. Review/explanation không phát sinh commit.
Lưu bàn giao nếu bị chặn/hủy; không đóng phiên tmux của người dùng. Quy trình
cloud và recording tại [runbook Claude](orchestrator_docs/operations/claude-tmux.md).
