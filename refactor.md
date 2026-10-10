# Kế hoạch giảm thao tác, Việt hóa Console và dọn legacy

Ngày kiểm tra nguồn: 2026-10-10. Branch chuẩn bị: `refactor_ui`.

## 0. Cách dùng file này

Codex mới bắt đầu phải đọc `AGENTS.md`, rồi đọc toàn bộ file này. Chọn task
đầu tiên có trạng thái TODO trong mục 9 mà mọi phụ thuộc đã DONE. Đọc nguồn
chuẩn theo INDEX trước khi sửa docs/code. Nếu gặp quyết định chưa có trong kế
hoạch, dừng phần phụ thuộc và hỏi người dùng theo AGENTS.md; không tự mở rộng.

Làm trên branch `refactor_ui`, không làm trên `main`. Mỗi task có một commit;
task L có thể có vài commit khi tách nội bộ. Mọi commit của task có message
bắt đầu bằng ID task, ví dụ `T05: ...`, kể cả khi task tách nhiều commit.
Commit hoàn tất cập nhật DONE và evidence path trong cùng commit đó.
Push lên `origin refactor_ui`,
không force-push và không merge vào `main` nếu người dùng chưa yêu cầu.
Commit `refactor.md` cùng commit đầu tiên của T01. Quy định này dành cho việc
triển khai sau khi kế hoạch được chốt; vòng sửa kế hoạch hiện tại không commit/push.

Prompt khởi động mẫu: “Thực hiện task kế tiếp trong refactor.md theo mục 0”.
Codex cập nhật tiến độ theo mục 9 và dùng mẫu handoff ở mục 8.
Nếu working tree có thay đổi không thuộc task, coi đó là công việc của người
dùng và không stage/commit chúng theo AGENTS.md; nếu chúng chặn task, hỏi người dùng.

## 1. Mục đích và bối cảnh

Dự án đang dev, chưa có người dùng thật hoặc dữ liệu production. Kế hoạch giảm
nhập liệu lặp, trình bày form theo mục đích, Việt hóa toàn Web Console và bỏ
compatibility không còn cần thiết. Không giữ tương thích dữ liệu, API hoặc
Definition cũ. Ngoại lệ duy nhất là semantics của Humanitec planner fixtures
trong `orchestrator_reference/`: read-only; adapter compatibility nằm ở harness.

Đây là kế hoạch thực thi, không thay specification hoặc ADR. Codex cập nhật
canonical docs trước khi giao code cho Claude. Người đọc không cần ngữ cảnh
cuộc trò chuyện. Các quyết định và acceptance criteria dưới đây là phạm vi task.

Deployment dev K8S-4F của commit `a0ef186` đang được giữ. Application
`k8s4f-dev-final`, ID `3c5e4c33-b724-4032-80b1-d270e5c32a35`, staging dùng
Connection `k8s-4f-2`, Store `k8s-4f-vault`, scope `ENVIRONMENT`, generation `0`.
Connection dùng `KUBECONFIG`; Store không legacy. Bản `k8s4f-dev` trước đó cũng
được giữ. Nguồn: `orchestrator_docs/operations/k8s4f-dev.md:29` và
`orchestrator_docs/verification/2026-10-09-k8s4f-dev-retained.md:24`; metadata
safe của API đã được đối chiếu khi lập kế hoạch. Phải kiểm tra lại trước reset.

Ước lượng S/M/L là tương đối, gồm checks và review. Phân loại lợi ích:
① hệ thống biết chắc; ② gợi ý cần xác nhận; ③ UX, không phải suy ra.
Không cộng số thao tác giảm của các mục trùng nhau.

## 2. Quyết định đã chốt và ngoài phạm vi

1. **RD-01 giữ profile trên Definition.** Terraform tự điền `aws-eks` và khóa
   field. Kubernetes chỉ có `internal-k8s` và rỗng, nhãn “Dùng chung”; người dùng
   quyết định. Cảnh báo shared có thể ambiguous với Definition cùng type/profile
   eligibility và specificity. `score-k8s` ngầm `internal-k8s`. Không suy profile
   ở server, không đổi matching, thêm tie-break hoặc migrate seed vì RD-01.
2. **UI-02:** form tạo resource chỉ còn Terraform/Kubernetes. Type có một cách
   tạo hợp lệ thì tự chọn driver/module. Bỏ `existing-cluster` khỏi form và API
   đăng ký; giữ driver/executor và `builtin-existing-cluster` nội bộ.
3. `score-k8s` chuyển sang mục ngang hàng “Mẫu dựng ứng dụng”; tên dùng `key`/ID
   kỹ thuật hiện có, không thêm friendly name. Vẫn lưu bằng Resource Definition
   API. Giữ ADR-010, matching hiện tại, default renderer khi không có mẫu khớp;
   render lỗi sau khi đã chọn mẫu không được fallback âm thầm.
4. Cách chọn mẫu sau này, tự động theo criteria hay Developer chọn theo
   Environment, còn chờ người dùng. Kế hoạch chỉ giữ behavior hiện tại, không
   thiết kế selector hay binding mới cho Environment.
5. **RD-02 được thực hiện:** VPC/EKS Terraform luôn dùng Connection của Environment;
   không bắt account riêng. Aurora vẫn bắt account AWS riêng. Canonical UC-03
   BR-08 và UC-06 BR-20/backend phải đổi trước phần UI này. Definition VPC/EKS không nhận
   ConnectionKey khác rỗng, kể cả bằng Connection của Environment. Khi
   Preview/Deploy, hệ thống luôn dùng Connection của Environment; không retarget.
6. **RD-06:** làm modes/dropdown/copy/phạm vi preview/cảnh báo nguy cơ ở frontend;
   matching-preview chính xác là endpoint tính toán chỉ đọc ở đợt backend.
7. **RD-08:** endpoint liệt kê bundle đã cài; một bundle tự chọn, không có thì
   không cho đăng ký mẫu. Không suy installed bundles từ ID hard-code.
8. **VI:** hard-code tiếng Việt toàn Web Console, không giữ giao diện tiếng Anh,
   không thêm i18n/library/language switcher. Giữ nguyên `staging`/`production`,
   technical IDs, values và payload. Trang được sửa phải Việt hóa trong cùng
   task; trang còn lại có task riêng. VI-03 mapping lỗi BE làm ở đợt backend.
9. **LEG-01..09 được dọn hết**, gồm HOST_CONTEXT execution. Chấp nhận reset IDP
   DB và baseline sạch, dựng lại trạng thái K8S-4F. T29 phải xin xác nhận ngay
   trước external mutation, sau khi backup/cleanup plan đã cụ thể và review được.
10. CFG-01 và RD-05 làm ở đợt cuối. Không mở rộng sang executor/provider mới.

Ngoài phạm vi thực thi: RD-09 shared condition sets, LEG-10 VSO-only, cách chọn
mẫu mới, friendly name mẫu, AWS cloud run, clone/promotion Environment, rollback
hoặc migration dữ liệu workload tự động. Không xóa fixtures/evidence lịch sử để
hợp thức hóa kết quả. Không đổi `match.go` specificity/ties trong bất kỳ task nào
của kế hoạch. Không đổi seed profile trong RD-01; cleanup seed identity/template
ở T27 là LEG, không phải policy chọn resource mới.

## 3. Thuật ngữ tiếng Việt — VI-01

T01 đã chốt nhãn chuẩn trong [glossary](orchestrator_docs/GLOSSARY.md#nhãn-web-console-vi-01);
bảng kế hoạch dưới đây ghi scope đã chốt, không thay canonical owner.

| Khái niệm/giá trị | Nhãn Console | Quy tắc |
|---|---|---|
| Application | Ứng dụng | Name/ID người dùng giữ nguyên. |
| Environment | Môi trường | Tab và giá trị vẫn `staging`, `production`. |
| Resource Type | Loại tài nguyên | Kèm Resource Type trong trợ giúp nếu cần tra contract. |
| Resource Definition | Cấu hình tài nguyên | Mô tả: cách tạo tài nguyên và điều kiện áp dụng. |
| Connection | Kết nối | Settings dùng “Kết nối triển khai”; account riêng dùng “Kết nối AWS”. |
| Secret Store | Kho bí mật | Không gọi là kho mật khẩu. |
| Execution profile | Phạm vi triển khai | Phân biệt với Environment type và criteria. |
| `internal-k8s` | Cluster nội bộ | Tooltip: cluster Kubernetes đã kết nối; không suy vị trí vật lý. |
| `aws-eks` | AWS | Có thể thêm “tạo EKS” trong hướng dẫn về Environment. |
| profile rỗng | Dùng chung | Giải thích: xét ở mọi profile, vẫn phải thỏa điều kiện áp dụng. |
| Terraform | Tạo trên AWS (Terraform) | Payload driver `terraform` giữ nguyên. |
| Kubernetes | Tạo trong cluster (Kubernetes) | Payload driver `kubernetes` giữ nguyên. |
| score-k8s UI entry | Mẫu dựng ứng dụng | Technical driver/bundle ID giữ nguyên. |
| Matching criteria | Điều kiện áp dụng | Modes: Mọi nơi; Theo loại môi trường; Theo ứng dụng; Tùy chỉnh nâng cao. |
| Driver variables | Tham số cấu hình | Không dịch JSON keys hoặc placeholder. |
| Provision rules | Quy tắc tạo tài nguyên liên quan | Không gọi chung là phụ thuộc vì hướng dependency có thể khác. |
| Workload / Container / Service | Workload / Container / Service | Giữ thuật ngữ kỹ thuật; hướng dẫn tiếng Việt. |
| Preview / Deploy | Xem trước / Triển khai | Không tự deploy khi lưu draft. |
| Variable / Secret | Biến / Bí mật | Giá trị Secret không được hiển thị lại. |
| READY / PENDING / FAILED / SUCCEEDED | Sẵn sàng / Đang chờ / Thất bại / Thành công | Chỉ dịch nhãn; enum/API giữ nguyên, ID nhỏ có thể hiện bên cạnh. |
| UNCONFIGURED / UNREFERENCED | Chưa cấu hình / Không còn được tham chiếu | Không diễn giải UNREFERENCED là đã xóa. |
| Platform Engineer / Developer / Admin | Kỹ sư nền tảng / Nhà phát triển / Quản trị viên | Role IDs không đổi. |

Text tĩnh, validation FE, aria-label, empty/loading/error states, notification và
thông báo trợ giúp đều dùng tiếng Việt. Tên do người dùng đặt, image, descriptor,
module, key, driver ID, JSON, URLs và giá trị API không dịch. Không dùng regex
thay thế từ trong raw backend errors để Việt hóa.

## 4. Quy trình, validation và evidence

Áp dụng [AGENTS.md](AGENTS.md) và
[runbook Claude/tmux](orchestrator_docs/operations/claude-tmux.md), đặc biệt
canonical change order và change-impact map. Codex sửa docs, giao scope/checks,
review diff và kiểm chứng độc lập; Claude làm code/tests bằng `clauded` trong
tmux riêng, không sửa docs/commit/push. Không tự thay Claude bằng Codex coding
khi công cụ lỗi. Poll Claude bình thường theo nhịp 240 giây, không đọc pane/log
liên tục. Ngoại lệ là completion/error/blocker hoặc user steering.

Task D/BE: specification/ADR → realization/diagrams/shared contracts/schema →
code/tests → traceability/current state/code-map/verification. Không đổi docs để
hợp thức hóa code sai. FE-only sửa UI docs trước code; không đổi API semantics.
Sau review/checks, Codex commit/push đúng task theo AGENTS.md. Quyền này áp dụng
cho các task triển khai tương lai; lần lập file kế hoạch này không commit/push.

Mọi task có browser product flow dùng headed Playwright, cursor/click rõ, nhập
liệu và pause đủ đọc, phụ đề Việt dưới video; assertions, phase marks và video
giữ cả khi fail. Dùng UI cho bước product; setup riêng phải khai rõ. Không hiện
token/kubeconfig/password/secret. Evidence ngoài Git, không tự upload. T01 chuẩn
bị runner local fake; các task FE không mutate K8S-4F. Video fake không chứng minh
real adapters. T29 là scenario thật, có approval riêng.

Các bộ lệnh dưới đây được tham chiếu nguyên vẹn trong từng task:

**V-D — docs và whitespace**, chạy từ root:

```bash
python3 scripts/check_docs.py
git diff --check
```

**V-FE — toàn frontend**, chạy từ root:

```bash
cd frontend && npm run typecheck && npm run lint && npm test && npm run build
```

**V-BE — toàn Go, không tự opt-in external integration**, chạy từ root:

```bash
cd backend && env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY go test ./... && go build ./...
cd backend && env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY go test ./test/conformance/...
```

Chạy mỗi dòng trong shell mới tại root; không chạy dòng thứ hai từ cwd backend
của dòng thứ nhất. Dòng conformance được nêu riêng để handoff ghi rõ kết quả;
nếu full suite đã chứng minh nó chạy/pass thì không cần chạy lặp vô ích.
Không suy tests PostgreSQL opt-in đã pass chỉ từ `go test ./...`.

**V-UI(Txx) — runner dự kiến tạo ở T01**, chạy từ root:

```bash
bash backend/test/integration/refactor-ui-local.sh --scenario Txx --headed --captions vi --evidence /tmp/poc-refactor-Txx
```

T01 phải tạo interface CLI `--scenario`, `--headed`, `--captions`, `--evidence`
đúng như lệnh trên; đây chưa phải interface có sẵn. Wrapper chuyển evidence
path sang `ORCH_VIDEO_DIR` của pattern hiện tại. Path phải mới cho mỗi lần chạy,
không ghi đè evidence cũ. Mọi V-UI(Txx), gồm T02B, dùng cùng interface này.
Runner dùng backend fake adapters/state disposable ngoài repo, không đọc `.env`,
không kết nối DB/cluster/cloud đang giữ. Khi task thêm scenario phải chạy
`bash -n backend/test/integration/refactor-ui-local.sh`. Không coi lệnh dự kiến
này là runner đã tồn tại. PostgreSQL mutation tests chỉ chạy trên DB disposable
khi môi trường/hành động đã được cho phép; mọi task cần SQL ghi rõ gate này.
PlantUML thay đổi: checker và regenerate PNG; CI phải kiểm PlantUML. Rename/path
removal: `rg` toàn repository tìm stale references, trừ reference read-only và
evidence lịch sử được đánh dấu đúng.

## 5. Tasks theo đợt

Mỗi Txx là một PR/commit có outcome riêng. Nếu một task L còn quá lớn, tách
implementation bên trong với acceptance không đổi, không gộp thêm scope. Các
path “mới” là artifact dự kiến được tạo khi task chạy, không phải code hiện có.

Quy ước đường dẫn cho các danh sách rút gọn dưới đây: `UC-xx` là
`orchestrator_docs/usecase/UC-xx/`; `architecture/`, `implementation/`,
`operations/` là các thư mục cùng tên trong `orchestrator_docs/`. Tên UI file
không ghi root nằm trong `frontend/src/features/<nhóm trang>/`; `app/`,
`shared/` nằm trong `frontend/src/`. Package Go không ghi root nằm trong
`backend/internal/`; conformance luôn nằm trong `backend/test/conformance/`.
`.test.tsx` và `states.md` viết liền sau file đầy đủ dùng cùng thư mục file đó.
Các bằng chứng `file:line` dùng đường dẫn đầy đủ tính từ root repository.

### Đợt 0 — thuật ngữ và nền test

#### T01 — Chốt thuật ngữ và runner/locator dùng chung

- **Mục tiêu/mã:** VI-01; VI-04 nền; VI-05 nền. Loại FE+D, M–L; lợi ích ③,
  chưa giảm bước product trực tiếp.
- **Docs Codex trước:** `orchestrator_docs/architecture/ui/README.md`,
  `orchestrator_docs/GLOSSARY.md` (phân biệt UI label với domain term),
  `orchestrator_docs/operations/claude-tmux.md` chỉ khi cần bổ sung runner usage.
  Không thêm i18n hoặc đổi architecture ADR-004.
- **Code/test Claude:** `frontend/test/e2e/human.mjs`, `captions.mjs`,
  `frontend/test/e2e/refactor-local.mjs` (mới),
  `backend/test/integration/refactor-ui-local.sh` (mới); helper locator mới dưới
  `frontend/test/e2e/`; tái sử dụng các runner/library/stubs nêu dưới đây.
  Bằng chứng pattern: `backend/test/integration/video-lib.sh:1`,
  `backend/test/integration/uc05-video-local.sh:2`,
  `backend/test/integration/uc02-04-video-local.sh:52`.
  Bằng chứng text selectors: `frontend/test/e2e/k8s4f-dev-human.mjs:98`.
- **Giao Claude:**
  - Claude phải tái sử dụng/mở rộng `backend/test/integration/video-lib.sh`,
    pattern `uc05-video-local.sh`, `onboarding-playwright-local.sh`,
    `uc02-04-video-local.sh` và `backend/test/integration/stubs/`;
    không viết lại cơ chế recording, fake backend hoặc cleanup từ đầu.
  - Wrapper phải tạo interface CLI đã chốt trong V-UI và dùng `ORCH_VIDEO_DIR`
    khi gọi helpers. Claude chuẩn hóa locator theo role/label tiếng Việt,
    dispatch scenario Txx và ownership của process/evidence.
  - Claude không đổi label mọi trang ở T01, không sửa backend product hoặc chạy
    retained cluster. Scenario chưa Việt hóa dùng locator hiện tại đến task của
    trang đó; không tạo chế độ tiếng Anh của sản phẩm.
- **Chấp nhận:**
  - Helpers không thay assertions bằng sleep. Runner hỗ trợ headed/captions, giữ
    video/marks/assertions khi fail và chỉ cleanup process do nó tạo.
  - Smoke hiện có chạy bằng UI và không lộ credentials. Runtime không thêm dependency
    i18n.
- **Validation:** V-D, V-FE; `bash -n backend/test/integration/refactor-ui-local.sh`;
  V-UI(T01), smoke sign-in/navigation fake. Codex review decode/frame/captions.
- **Phụ thuộc/rủi ro/external:** không phụ thuộc task khác; helper không bỏ
  accessibility coverage. Runner có thể cần mở rộng fake responses cho các
  trang chưa có scenario; vì vậy ước lượng M–L, không cam kết nền hiện có đủ
  mọi trang. Không external mutation; chỉ dùng setup local disposable.

### Đợt 1 — frontend theo trang, Việt hóa ngay trong task

#### T02 — Form tạo Cấu hình tài nguyên

- **Mục tiêu/mã:** RD-01; RD-02 Kubernetes; RD-03; RD-07 form; UI-01;
  UI-02 form resource; VI-02/04/05. FE, M. ① tự điền profile/driver hợp lệ;
  ③ dropdown/layout; bớt 1 profile choice, 1 key Connection nhập và lượt tra key.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-03/ui/screens.md`,
  `states.md`, `api-mapping.md`; không đổi matching hoặc contract.
- **Code/test:** `frontend/src/features/platform/ResourceDefinitionsPage.tsx`,
  `.test.tsx`, `RegistrationStates.test.tsx`; scenario T02. Bằng chứng form:
  `frontend/src/features/platform/ResourceDefinitionsPage.tsx:68`; restriction:
  `backend/internal/application/catalog/service.go:161` và `:168`.
- **Giao Claude:**
  - Form chỉ cho chọn Terraform/Kubernetes; workload chuyển sang trang T03.
    Form không cho đăng ký existing-cluster.
  - VPC/cluster tự chọn Terraform và module vpc/eks; namespace tự chọn Kubernetes.
    Postgres vẫn cần người dùng chọn cách tạo. Đổi driver cập nhật controls hợp lệ,
    không âm thầm xóa cấu hình người dùng.
  - Terraform tự điền và khóa `aws-eks`; Kubernetes cho chọn `internal-k8s`
    hoặc “Dùng chung” (rỗng). Giữ Connection bắt buộc Terraform đến T16.
  - Claude Việt hóa trang và sắp xếp phần cơ bản/nâng cao. Editor criteria
    hiện tại được giữ đến T02B; task này không làm RD-06.
- **Chấp nhận:**
  - Payload giữ contract hiện tại; profile shared gửi `""`, Terraform gửi `aws-eks`.
    UI cảnh báo shared có thể ambiguous; không hứa giải quyết mọi overlap.
  - Dropdown Connection dùng GET `/connections`, chỉ lấy READY đúng kind/org,
    gửi key kỹ thuật. Ô Connection Kubernetes mặc định trống để dùng Connection
    của Environment; ô chọn Connection riêng nằm trong phần nâng cao.
  - Form giữ metadata khi loading hoặc submit lỗi; blank/duplicate ID có lỗi rõ.
    List resource không hiển thị workload mẫu. Entry existing-cluster hệ thống
    nếu hiển thị có nhãn hệ thống và không có thao tác tạo/sửa.
  - Mọi text và aria-label của trang dùng tiếng Việt; ID/payload giữ nguyên.
- **Validation:** V-D, V-FE, V-UI(T02): driver/module/profile controls, shared
  warning, Connection dropdown/advanced override, failed submit/retry. Backend
  fake; không provision hoặc thay catalog thật.
- **Phụ thuộc/rủi ro/external:** T01. T03 là task phía sau, cần phối hợp route. Không ghi defaults
  mới vào backend hoặc migrate seed. Không external mutation.

#### T02B — Editor Điều kiện áp dụng dùng chung

- **Mục tiêu/mã:** RD-06 frontend; VI-02/04/05. FE, M; ③ giảm 3–5 ô gõ tay
  ở trường hợp phổ biến và lượt tra app/environment IDs.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-03/ui/screens.md`,
  `states.md`, `api-mapping.md`; không thay criteria contract hoặc matcher.
- **Code/test:** editor và tests mới `frontend/src/features/platform/CriteriaEditor.tsx`,
  `CriteriaEditor.test.tsx`; tích hợp `ResourceDefinitionsPage.tsx/.test.tsx`;
  scenario T02B. T03 dùng lại editor này. Bằng chứng form 5 ô:
  `frontend/src/features/platform/ResourceDefinitionsPage.tsx:70`;
  criterion contract `backend/internal/domain/resource/definition.go:40`.
- **Giao Claude:**
  - Tạo modes “Mọi nơi”, “Theo loại môi trường”, “Theo ứng dụng (+ môi trường)”
    và “Tùy chỉnh nâng cao”. Dropdown dùng GET `/api/v1/applications`.
  - Chỉ hiện ô cần dùng; advanced giữ đủ 5 fields và nhiều criterion rows.
    Cho sao chép criteria từ Definition khác, không tạo liên kết sống.
  - Hiển thị phạm vi app/environment và cảnh báo nguy cơ chồng lấn cùng type.
    Không gọi đây là kết quả matching chính xác; endpoint đó thuộc T19.
- **Chấp nhận:**
  - “Mọi nơi” gửi `[{}]`; mode env type gửi `env_type`; mode app/env gửi IDs
    thực. Chuyển mode không âm thầm làm mất criteria advanced không biểu diễn được.
  - Copy tạo bản độc lập. Loading/error/late replies không ghi đè edits.
  - Preview phân biệt profile và env_type. Khi thiếu class/res_id, UI báo cần
    thêm context; cảnh báo overlap không kết luận ambiguous nếu chưa biết winning
    specificity. Không persist/provision khi xem trước.
  - Editor dùng tiếng Việt và dùng chung được cho form resource lẫn renderer.
- **Validation:** V-D, V-FE, V-UI(T02B): 4 modes, nhiều rows, app/env dropdown,
  copy, advanced round-trip, incomplete context và overlap warning với backend fake.
- **Phụ thuộc/rủi ro/external:** T01/T02; T03 tích hợp sau đó, T19 thêm preview
  authoritative. Không external mutation; không thêm endpoint ở task này.

#### T03 — Trang Mẫu dựng ứng dụng

- **Mục tiêu/mã:** UI-02 phần renderer; RD-08 phần UI ban đầu;
  VI-02/04/05. FE, M; ① type/driver/profile ngầm, bớt 3 technical fields.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-03/ui/screens.md`,
  `states.md`, `api-mapping.md`, `orchestrator_docs/architecture/ui/README.md`.
  Giữ ADR-010; không thiết kế cách chọn mẫu mới.
- **Code/test:** `frontend/src/app/routes.ts`, `App.tsx`, `AppShell.tsx`,
  `App.test.tsx`; `frontend/src/features/platform/RenderingTemplatesPage.tsx`
  và `.test.tsx` (mới); tách khỏi ResourceDefinitionsPage. Bằng chứng payload
  `frontend/src/features/platform/ResourceDefinitionsPage.tsx:47`, `:56`; contract `backend/internal/domain/resource/rendering.go:38`.
- **Giao Claude:**
  - Claude dùng technical ID hợp lệ làm tên mẫu; danh sách chỉ lấy workload Definitions và
    form dùng editor T02B.
  - Bản frontend ban đầu cho nhập bundle ID theo API hiện tại và giải thích đây chưa phải
    selector từ danh sách bundle đã cài. T18 sẽ thay field bằng API list.
  - Claude không thêm friendly name, Environment binding hoặc runtime template upload.
- **Chấp nhận:**
  - Trang nằm ngang hàng trong sidebar và có PE/Admin gates như catalog tài nguyên.
  - POST `/resource-definitions` gửi workload/score-k8s/internal-k8s; không gửi override
    hoặc provision; variables chỉ chứa render_bundle. Form không hiện trường provision
    resource.
  - Hướng dẫn nói rõ mẫu là tùy chọn và chỉ dùng cho cluster nội bộ. Không có mẫu khớp thì
    dùng renderer mặc định; mẫu đã chọn render lỗi phải báo lỗi, không fallback.
  - Hệ thống vẫn chọn mẫu tự động theo criteria hiện tại. Mọi text và aria-label dùng
    tiếng Việt.
- **Validation:** V-D, V-FE, V-UI(T03): tạo/list mẫu với ID hợp lệ, giữ lỗi bundle,
  không thấy infrastructure fields. Preview với backend fake phải chứng minh:
  không có mẫu khớp thì dùng renderer mặc định; có mẫu khớp thì chọn đúng mẫu;
  mẫu đã chọn render lỗi thì báo lỗi, không chuyển sang renderer mặc định.
  Không chạy score-k8s/cluster thật để chứng minh provisioning.
- **Phụ thuộc/rủi ro/external:** T01/T02/T02B; RD-08 đầy đủ chưa done đến T18.
  Chỉ UI extraction, không external mutation.

#### T04 — Trang Loại tài nguyên

- **Mục tiêu/mã:** RT-01..04; UI-01; VI-02/04/05. FE, M. ② copy schema cần
  sửa/xác nhận, ③ details/link; bớt n lần Add khi dùng mẫu, 1 lượt tra contract,
  1 lượt điều hướng và chọn lại type.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-02/ui/screens.md`,
  `states.md`, `api-mapping.md`; UC-03 UI liên kết tạo Definition.
- **Code/test:** `ResourceTypesPage.tsx`, `.test.tsx`, `RegistrationStates.test.tsx`,
  `frontend/src/app/routes.ts` cho prefill điều hướng. Bằng chứng list/rows
  `frontend/src/features/platform/ResourceTypesPage.tsx:37`, `:50`; type/runtime whitelist `backend/internal/application/catalog/service.go:157`.
- **Giao Claude:**
  - Claude hiển thị inputs/outputs từ GET và cho sao chép schema sang Type có ID mới.
  - Claude không suy required/secret từ tên và không thêm version/edit/delete schema.
- **Chấp nhận:**
  - Schema sao chép giữ nguyên flags/types; ID trùng không ghi đè dữ liệu.
  - Nhãn “Có cách tạo hỗ trợ” dựa đúng whitelist. Custom ID chỉ có schema hiển thị “Chỉ có
    contract”; postgres-ha không được quảng bá là deploy được.
  - CTA tạo Definition chỉ hiện cho Type có implementation hợp lệ và prefill type ở T02.
    Trang dùng tiếng Việt và giữ form khi submit lỗi.
- **Validation:** V-D, V-FE, V-UI(T04): details, copy/sửa, create duplicate fail,
  supported/custom CTA và prefill. Fake registration.
- **Phụ thuộc/rủi ro/external:** T01/T02; metadata authority thay allowlist ở T31.
  Không mở rộng executor; không external mutation.

#### T05 — Trang Kho bí mật

- **Mục tiêu/mã:** SS-01..04; UI-01; VI-02/04/05. FE, M; ② same-address;
  ③ layout/validation. Bớt 1 URL nhập và 3 ô cần đọc ở form thường.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-04/ui/screens.md`,
  `states.md`, `api-mapping.md`.
- **Code/test:** `SecretStoresPage.tsx`, `.test.tsx`; scenario T05. Bằng chứng
  defaults `frontend/src/features/platform/SecretStoresPage.tsx:7`, failure `:33`, fields/list `:50`, `:56`;
  backend DTO `backend/internal/delivery/http/stores.go:22`.
- **Giao Claude:**
  - Claude đưa mount/CA vào phần nâng cao và hiển thị defaults trong phần tóm tắt.
  - Checkbox dùng cùng địa chỉ là opt-in. Khi tắt, UI không ghi đè workload address riêng.
  - Claude kiểm URL/mount/PEM shape/limits trước request; không coi kiểm shape là network
    hoặc cert trust validation.
- **Chấp nhận:**
  - Form gửi defaults kv/kubernetes. Checkbox dùng cùng địa chỉ đồng bộ payload; khi không
    chọn, hai address độc lập.
  - Dữ liệu không hợp lệ tại frontend không tạo POST. Khi server fail, form xóa token
    nhưng giữ metadata.
  - List hiển thị workload address, backend verification và Kubernetes auth riêng. READY
    không được diễn giải là deploy-ready. Trang không hiện token/credential ref và dùng
    tiếng Việt.
- **Validation:** V-D, V-FE, V-UI(T05): advanced, same/different URL, local error,
  verification failure/token clear, auth absent/configured list. Fake verifier.
- **Phụ thuộc/rủi ro/external:** T01; không mở rộng Developer choices DTO.
  Không external mutation hoặc đọc token thật.

#### T06 — Trang Kết nối

- **Mục tiêu/mã:** CN-01/02; UI-01; VI-02/04/05. FE, M; ① inspect/single
  context; ② friendly name. Bớt 1 click inspect và 1 tên nhập nếu nhận gợi ý.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-04/ui/screens.md`,
  `states.md`, `api-mapping.md`.
- **Code/test:** `ConnectionsPage.tsx`, `.test.tsx`; e2e T06. Bằng chứng inspect
  `frontend/src/features/platform/ConnectionsPage.tsx:76`, single context `:86`, name `:125`.
- **Giao Claude:**
  - Claude tự inspect khi upload; khi paste, chỉ inspect sau khi nội dung ổn định và có
    nút retry.
  - UI gợi ý name từ cluster/context đã chọn, không ghi đè tên người dùng đã sửa. Hành vi
    tự chọn single context hiện có được giữ.
- **Chấp nhận:**
  - Khi document đổi, summary và phản hồi pending cũ mất hiệu lực; phản hồi đến muộn không
    sửa context/name của document mới.
  - Với nhiều context, UI không đoán theo current-context. Check/save vẫn cần thao tác
    tường minh; Inspect không tự save.
  - Metadata/credentials không bị lộ. Các trạng thái loading/failure/retry và text/aria-
    label dùng tiếng Việt.
- **Validation:** V-D, V-FE, V-UI(T06): upload/paste, một/nhiều contexts, đổi file
  khi pending, tên gợi ý/custom, masked content và failed verification.
- **Phụ thuộc/rủi ro/external:** T01; fake verifier, synthetic kubeconfig.
  Không verify cluster thật trong task FE. HOST_CONTEXT cleanup thuộc T22.

#### T07 — Trang Environment Settings

- **Mục tiêu/mã:** ENV-01; VI-02/04/05; labels CFG-01 chưa đổi behavior.
  Chỉ Việt hóa các nhãn cấu hình liên quan CFG-01, chưa đổi hành vi rename.
  FE, M; ② lựa chọn gợi ý chưa lưu, bớt 1 dropdown choice mỗi selector khi chỉ có 1 đích.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-01/ui/screens.md`,
  `states.md`, `orchestrator_docs/usecase/UC-12/ui/screens.md`, `states.md`.
  Giải thích một lựa chọn draft gợi ý không phải Organization default/fallback;
  persistence vẫn explicit Save đúng ES-02/03.
- **Code/test:** `configuration/SettingsPage.tsx`, `.test.tsx`,
  `EnvironmentConnection.tsx`, `.test.tsx`; `environment/SecretStoreSelection.tsx`,
  `TransitionPanel.tsx`, `OperationBanner.tsx`, `environment.test.tsx`.
  Bằng chứng Connection select `frontend/src/features/configuration/EnvironmentConnection.tsx:103`; store change
  `frontend/src/features/environment/SecretStoreSelection.tsx:40`; key forms `frontend/src/features/configuration/SettingsPage.tsx:103`.
- **Giao Claude:**
  - Claude chỉ gợi ý lựa chọn chưa lưu nếu chưa có binding và chỉ có một entry hợp lệ. UI
    không tự persist hoặc đổi target đã cấu hình.
  - Claude Việt hóa dialogs/configuration/transition components của trang; không rewrite
    references khi rename và không bỏ confirmation downtime/recovery.
- **Chấp nhận:**
  - Chỉ Save mới đổi binding; khi có nhiều lựa chọn, UI không tự chọn. Refresh hoặc phản
    hồi đến muộn không ghi đè pending edits.
  - Profile/region do backend suy ra và chỉ đọc; UI không thêm controls. Nhãn
    staging/production giữ nguyên.
  - Các trạng thái BUSY/stale/version/copy failure vẫn xử lý đúng. Chọn lại cùng target
    không copy/migrate; secrets chỉ cho nhập, không đọc lại.
- **Validation:** V-D, V-FE, V-UI(T07): một/nhiều/no READY choices, Save, variable/
  secret forms, stale/BUSY, store-copy và transition UI với fake adapters.
- **Phụ thuộc/rủi ro/external:** T01; VI-03 lỗi server đầy đủ ở T20.
  Không chạy copy/migration trên retained store/cluster.

#### T08 — Trang Workload editor

- **Mục tiêu/mã:** WL-01/02; VI-02/04/05. FE, M; ① defaults/unique port,
  ③ progressive disclosure; bớt 4 ô resource/container, class/target-port thường.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-16/ui/screens.md`,
  `states.md`, `README.md`; không đổi Score semantics.
- **Code/test:** `workloads/WorkloadEditorPage.tsx`, `WorkloadEditorPage.test.tsx`,
  `WorkloadEditorKeys.test.tsx`, `ApplicationKeyPicker.tsx`, `bindings.ts`;
  scenario T08. Bằng chứng fallback `frontend/src/features/workloads/WorkloadEditorPage.tsx:91`, controls `:261`,
  routes `:275`, key alias default `frontend/src/features/workloads/bindings.ts:10`.
- **Giao Claude:**
  - Claude đưa class/resources/target-port override vào phần nâng cao và hiển thị defaults
    có hiệu lực; không tự điền capacity.
  - Container main và key-name defaults đã có; Claude giữ hành vi này, không làm lại. UI
    chỉ tự chọn port khi có đúng một port hợp lệ sau Add public route.
- **Chấp nhận:**
  - Score payload giữ kiểu số, quy tắc bỏ qua defaults và targetPort fallback hiện tại.
    Collapse không làm mất imported/edited overrides.
  - CPU/memory request thiếu vẫn lấy theo limit hoặc defaults 10m/32Mi hiện có; UI không
    tự thêm limit. Class rỗng vẫn dùng default.
  - UI không tự bật public route. Port đổi/xóa làm route invalid, không âm thầm đổi đích.
    Key picker không tự chọn secrets/new keys; staging/production và technical values giữ
    nguyên.
- **Validation:** V-D, V-FE, V-UI(T08): basic/advanced, typed params, imported
  overrides, unique/multiple ports, key aliases, save/import validation/BUSY.
- **Phụ thuộc/rủi ro/external:** T01; T25 bỏ publicPort sau này; T30 rename drafts.
  Fake Preview/Deploy nếu scenario cần; không cluster mutation.

#### T09 — Trang Tạo ứng dụng

- **Mục tiêu/mã:** APP-01; VI-02/04/05. FE, S; ② subdomain suggestion,
  bớt 1 ô nhập khi chấp nhận.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-01/ui/screens.md`,
  `states.md`.
- **Code/test:** `applications/CreateApplicationPage.tsx`, `.test.tsx`; T09.
  Bằng chứng name/subdomain state `frontend/src/features/applications/CreateApplicationPage.tsx:23`.
- **Giao Claude:**
  - Claude gợi ý slug từ name đến khi người dùng sửa subdomain. Create payload không thêm
    target/provider; ID service và uniqueness giữ nguyên.
- **Chấp nhận:**
  - Khi name đổi, UI không ghi đè subdomain người dùng đã sửa. Empty/invalid/duplicate có
    lỗi rõ; endpoint preview staging/production giữ contract.
  - Submit chỉ gửi name/subdomain. Text và aria-label dùng tiếng Việt.
- **Validation:** V-D, V-FE, V-UI(T09): suggestion/custom/duplicate failure,
  tạo ứng dụng fake và navigation.
- **Phụ thuộc/rủi ro/external:** T01; không external mutation.

#### T10 — Đăng nhập và shared shell

- **Mục tiêu/mã:** VI-02/04/05 cho SignIn và navigation/shared UI; FE, S,
  lợi ích ③, 0 bước trực tiếp.
- **Docs trước:** chỉ UI docs `orchestrator_docs/usecase/UC-00/ui/screens.md`,
  `states.md`, `orchestrator_docs/architecture/ui/README.md`.
- **Code/test:** `auth/SignInPage.tsx`, `.test.tsx`, `app/AppShell.tsx`, `App.tsx`,
  `App.test.tsx`, reusable labels dưới `shared/ui/`. Bằng chứng nav `frontend/src/app/AppShell.tsx:15`.
- **Giao Claude:**
  - Claude dịch tiêu đề/role/nút/session notices, giữ auth/session behavior và technical
    values. Helper login dùng nhãn Việt.
- **Chấp nhận:**
  - Ô tài khoản/mật khẩu có nhãn tiếng Việt accessible; mật khẩu được che. Sign-out và
    expired session vẫn hoạt động đúng.
  - Sidebar theo role và link T03 đúng. Unknown-page/retry messages dùng tiếng Việt; không
    thêm language switcher hoặc i18n.
- **Validation:** V-D, V-FE, V-UI(T10): sign-in fail/success, session restore,
  sign-out và role navigation fake.
- **Phụ thuộc/rủi ro/external:** T01; nên chạy đầu đợt 1 để login helper ổn định.
  Không external mutation.

#### T11 — Danh sách ứng dụng

- **Mục tiêu/mã:** VI-02/04/05; FE, S, ③/0 bước.
- **Docs trước:** chỉ UI docs UC-01 `ui/screens.md`, `ui/states.md`.
- **Code/test:** `applications/ApplicationsPage.tsx`, app list tests trong
  `app/App.test.tsx`; scenario T11.
- **Giao Claude:**
  - Claude chỉ dịch text của trang/cards và các trạng thái rỗng/đang tải/lỗi.
    Data/name/URL/sorting/navigation giữ nguyên; không thêm search/filter.
- **Chấp nhận:**
  - Mọi nhãn của trang dùng tiếng Việt; staging/production giữ nguyên. Empty/loading/retry
    accessible; tạo ứng dụng và deep links hoạt động.
- **Validation:** V-D, V-FE, V-UI(T11): list nhiều/empty/error, open app fake.
- **Phụ thuộc/rủi ro/external:** T01/T10; không external mutation.

#### T12 — Application home

- **Mục tiêu/mã:** VI-02/04/05; FE, M, ③/0 bước.
- **Docs trước:** chỉ UI docs UC-01 `ui/screens.md`, `ui/states.md`,
  UC-07 `ui/screens.md`, UC-16 `ui/screens.md` cho entry points.
- **Code/test:** `applications/ApplicationHomePage.tsx`, `.test.tsx`,
  `deployments/RecentDeployments.tsx`; scenario T12.
- **Giao Claude:**
  - Claude dịch workload list và các thao tác pending Preview/Deploy/remove/undo/status.
    Stale token, confirmations và per-workload retry giữ nguyên.
- **Chấp nhận:**
  - Thông báo unconfigured/BUSY/stale/no-op/failure dùng tiếng Việt. Preview không mutate;
    Deploy vẫn tường minh.
  - Missing key không lộ value; chuyển Environment không nhận phản hồi cũ sai context.
    Graph và IDs kỹ thuật giữ nguyên.
- **Validation:** V-D, V-FE, V-UI(T12): draft save → pending preview → fake
  deploy/partial retry; delete/undo và stale messages.
- **Phụ thuộc/rủi ro/external:** T01/T10; fake only, không retained deploy.

#### T13 — Standalone Score Preview

- **Mục tiêu/mã:** VI-02/04/05; FE, S, ③/0 bước.
- **Docs trước:** chỉ UI docs UC-05 `ui/screens.md`, `ui/states.md`.
- **Code/test:** `preview/ScorePreviewPage.tsx`, `.test.tsx`, `parseScore.ts`;
  scenario T13.
- **Giao Claude:**
  - Claude dịch form/tab/kết quả/lỗi; không dịch YAML/JSON keys, graph descriptor, output
    keys hoặc thay parser Score.
- **Chấp nhận:**
  - Lỗi Score và các trạng thái unconfigured/stale/error/retry dùng tiếng Việt.
    Delta/Candidate/matches/batches giữ dữ liệu; standalone Preview chỉ đọc.
- **Validation:** V-D, V-FE, V-UI(T13): nhập Score hợp lệ/sai, xem graph/results,
  role/error states fake, assertion không provision.
- **Phụ thuộc/rủi ro/external:** T01/T10; T20 bổ sung code-based errors.
  Không external mutation.

#### T14 — Deployment history

- **Mục tiêu/mã:** VI-02/04/05; FE, S, ③/0 bước.
- **Docs trước:** chỉ UI docs UC-09 `ui/screens.md`, `ui/states.md`.
- **Code/test:** `deployments/DeploymentHistoryPage.tsx`, relevant cases trong
  `DeploymentPages.test.tsx`; scenario T14.
- **Giao Claude:**
  - Claude dịch filters/status/time labels/empty/error. Filter payload giữ enums/IDs;
    không thêm so sánh deployments.
- **Chấp nhận:**
  - History được scoped đúng app/env. UI dịch trạng thái nhưng gửi nguyên enum; mở detail
    giữ đúng deployment ID. Loading/retry accessible.
- **Validation:** V-D, V-FE, V-UI(T14): filters, empty/failure, open details fake.
- **Phụ thuộc/rủi ro/external:** T01/T10; không external mutation.

#### T15 — Deployment details

- **Mục tiêu/mã:** VI-02/04/05; FE, M, ③/0 bước.
- **Docs trước:** chỉ UI docs UC-09 `ui/screens.md`, `ui/states.md`.
- **Code/test:** `deployments/DeploymentDetailsPage.tsx`, relevant cases trong
  `DeploymentPages.test.tsx`; scenario T15.
- **Giao Claude:**
  - Claude dịch labels/progress/graph/output redaction/failure/not-found. Trang không trả
    resolved inputs, raw failure hoặc secret output.
- **Chấp nhận:**
  - Trang hiển thị đúng historical target identity và dịch state badges; IDs/path/module
    giữ nguyên.
  - Redacted value không trở thành dữ liệu thật. Inaccessible/missing/error/retry an toàn;
    không thêm live status hoặc rollback.
- **Validation:** V-D, V-FE, V-UI(T15): succeeded/failed/not-found, graph và
  secret-output redaction fake.
- **Phụ thuộc/rủi ro/external:** T01/T10; T20 lỗi BE; không external mutation.

### Đợt 2 — backend/contract và legacy từng vùng

#### T16 — VPC/EKS luôn dùng Connection của Environment

- **Mục tiêu/mã:** RD-02 Terraform; BE+D+FE, M; ① đích đã biết, bớt 1 account
  field trong VPC/EKS. Aurora vẫn chọn account riêng.
- **Docs trước:** UC-03 `specification.md` BR-08, `realization.md`, UI mapping;
  UC-06 `specification.md` BR-20, `realization.md`; UC-08 `specification.md`,
  `realization.md`; `architecture/contracts/operation-contracts.md`,
  `architecture/decisions/ADR-011-environment-execution-binding.md` phần account
  guards và ADR-013 cloud boundary. Không thay matching/ties hoặc scope.
- **Code/test:** `application/catalog/service.go`, `registration_test.go`,
  `definition_test.go`; `planning/match.go` chỉ đọc để kiểm guard, không sửa; `connection_binding_test.go`; `application/provisioning/service.go`
  và tests; `delivery/http/resource_definitions.go`; frontend T02 tests;
  `backend/internal/seed/seed.go` và seed tests để new product Definitions
  VPC/EKS không có ConnectionKey, không migrate seed/profile cũ.
  Bằng chứng registration bắt explicit `backend/internal/application/catalog/service.go:184`, fallback
  `backend/internal/application/provisioning/service.go:313`, guard `backend/internal/planning/match.go:22`.
- **Giao Claude:**
  - Registration Terraform module vpc/eks phải từ chối mọi ConnectionKey khác
    rỗng với lỗi rõ, kể cả key bằng Environment Connection. Definition reusable
    không cần Environment lúc đăng ký. Preview/Deploy luôn resolve Environment
    AWS READY; Aurora bắt account AWS READY đúng org/kind riêng.
  - Giữ `ConnectionMismatch` của VPC/EKS làm guard phòng thủ cho malformed
    catalog nội bộ/harness. Không sửa `match.go`, ranking, ties hoặc thêm fallback.
    Public API không còn đường đăng ký override VPC/EKS.
- **Chấp nhận:**
  - VPC/EKS ConnectionKey rỗng đăng ký được; khác rỗng bị từ chối và không insert.
    UI không hiện ô Connection cho hai module này và luôn gửi giá trị rỗng.
  - Preview/Deploy dùng đúng Environment Connection; hash/plan/ActiveResource
    lưu effective target nhất quán. Tests guard phải chứng minh malformed
    VPC/EKS key khác target bị chặn trước execute, không chọn ứng viên khác.
    Key rỗng không mismatch; không bổ sung compatibility cho key bằng target.
  - Aurora thiếu account bị từ chối; account riêng hợp lệ không bị thay bằng
    Environment Connection. Profile vẫn khóa theo T02.
- **Validation:** V-D, V-BE, V-FE, V-UI(T16): đăng ký VPC/EKS không có account,
  API reject key không rỗng, Aurora account riêng, Preview effective target bằng
  backend fake. Unit tests kiểm guard với malformed catalog. PostgreSQL checks
  chỉ chạy trên DB disposable đã được phép; không AWS run.
- **Conformance:** fixture accounts giữ nguyên qua loader; compatibility nằm
  ở harness. Không đổi profiles/criteria hoặc matching expectations của fixtures.
  New product seed VPC/EKS dùng ConnectionKey rỗng; đây không phải migrate seed
  hoặc đổi profile RD-01. Thêm product tests inheritance và guard, chạy suite.
- **Phụ thuộc/rủi ro/external:** T02; giữ credentials scoping/pinned target.
  Không external mutation mặc định; DB integration là opt-in riêng.

#### T17 — Đóng existing-cluster registration công khai

- **Mục tiêu/mã:** RD-07 backend; UI-02 backend; LEG-03. BE+D, M, ③;
  loại một workflow không cần, không đổi builtin execution.
- **Docs trước:** ADR-013 đoạn historical authored cluster Definitions/reads,
  UC-03 specification/realization/UI mapping; UC-06/08 contracts và
  `architecture/contracts/operation-contracts.md` về system admission.
- **Code/test:** catalog service/policy/registration tests; HTTP registration
  e2e `backend/test/e2e/uc02_04_http_test.go`, policy tests; `planning/builtin.go`,
  `connection_binding_test.go`, provisioning builtin tests chỉ kiểm invariant.
  Bằng chứng public branch `backend/internal/application/catalog/service.go:174`, `orchestrator_docs/architecture/decisions/ADR-013-implicit-existing-cluster.md:50`.
- **Giao Claude:**
  - Claude từ chối mọi public registration có driver existing-cluster; giữ enum, executor
    và trusted constructor.
  - Không thêm API xóa legacy records hoặc compatibility cho records cũ; dev DB records
    được bỏ khi reset T29.
- **Chấp nhận:**
  - Public POST có driver existing-cluster bị từ chối và không insert, bất kể key. Builtin
    reserved key vẫn được bảo vệ.
  - Internal plan/execute không cần user cluster Definition và ghi FK system definition
    đúng. UI không tạo được authored existing-cluster.
- **Validation:** V-D, V-BE; không Playwright mới vì T02 đã quay removal form,
  task này thuần API/unit. DB FK tests nếu opt-in DB disposable được phép.
- **Conformance:** authored cluster matching fixtures tiếp tục qua ReferenceCluster
  harness; không dùng public registration trong harness, không sửa reference.
- **Phụ thuộc/rủi ro/external:** T02/T03; phối hợp T29 bỏ old catalog record.
  Không external mutation, không xóa retained deployment lúc implement.

#### T18 — API danh sách bundle và hoàn tất selector mẫu

- **Mục tiêu/mã:** RD-08; UI-02 integration. BE+D+FE, M; ① một bundle,
  ③ chọn từ catalog; bớt 1 ID nhập/tra cứu.
- **Docs trước:** UC-03 UI API mapping/screens/states và specification phần
  renderer; `architecture/contracts/workload-rendering.md`,
  `architecture/contracts/operation-contracts.md`. Không đổi ADR-010 architecture.
- **Code/test:** `delivery/http/server.go`, handler bundle list mới/tests;
  `application/catalog/service.go` hoặc read service qua installed bundle registry;
  `RenderingTemplatesPage.tsx/.test.tsx`; backend `rendering_http_test.go`.
  Bằng chứng bundle registry `backend/internal/application/catalog/service.go:30`, `:323`; routes
  `backend/internal/delivery/http/server.go:118` chưa có bundle list.
- **Giao Claude:**
  - Claude thêm GET `/api/v1/render-bundles` với PE/Admin/org authorization. Response chỉ
    chứa metadata an toàn ID/version/status, không trả path/binary/template
    source/credentials.
  - Endpoint đọc installed bundle registry; UI không dùng danh sách hard-code.
- **Chấp nhận:**
  - Không có bundle thì UI khóa submit và hướng dẫn bằng tiếng Việt; một bundle tự chọn;
    nhiều bundle yêu cầu lựa chọn tường minh.
  - Lỗi list giữ form và cho retry; phản hồi đến muộn không đổi lựa chọn. POST vẫn gửi
    render_bundle ID; bundle không còn available bị registration từ chối.
  - Tên mẫu vẫn theo rule ID lowercase/digits/hyphens.
- **Validation:** V-D, V-BE, V-FE, V-UI(T18): 0/1/n bundle, list error, unavailable
  during submit và matching render Preview fake. No installed binary/cloud setup.
- **Conformance:** metadata endpoint không đổi planner; full suite phải nguyên
  kết quả. Fixture bundles product renderer tests cấp tường minh.
- **Phụ thuộc/rủi ro/external:** T03; no new rendering selection behavior.
  Không external mutation.

#### T19 — Matching preview chính xác, không persist

- **Mục tiêu/mã:** RD-06 backend; BE+D+FE, M, ③; giảm một vòng đăng ký/Preview
  lỗi khi nhận diện tie trước save.
- **Docs trước:** UC-03 specification/realization/UI mapping/screens;
  UC-05 specification/realization; `architecture/contracts/operation-contracts.md`,
  `implementation/uc06-planner-reference.md`. Giữ UC-03 specificity nguyên vẹn.
- **Code/test:** handler/read service mới dưới `delivery/http/` và
  `application/catalog/`; reuse pure `planning/match.go`/domain criterion;
  HTTP tests và catalog read-preview tests mới; T02B/T03 frontend integration.
  Bằng chứng fields `backend/internal/domain/resource/definition.go:40`; matching `backend/internal/planning/match.go:131`;
  Applications view `backend/internal/delivery/http/server.go:178`, `:294`.
- **Giao Claude:**
  - Claude thêm POST `/api/v1/resource-definitions/matching-preview` chỉ tính toán. Input
    gồm unsaved Definition và context app/env/resource tường minh.
  - Endpoint dùng scoped catalog snapshot và chỉ thay candidate cùng key trong memory.
    Endpoint không gọi registration/provision/system admission và không insert.
- **Chấp nhận:**
  - Response trả eligibility, best criterion/score, winner hoặc ties, Connection conflict
    và snapshot/version marker. Thiếu res_id/class phải báo incomplete, không kết luận
    ambiguous.
  - Equal winning score vẫn ambiguous; higher score loại tie thấp. Profile chỉ lọc
    eligibility, không có tie-break mới. UI phân biệt matching errors và transport errors.
  - Late/stale response không đảm bảo save. UI ghi rõ snapshot; planner vẫn kiểm khi triển
    khai. Request bounded và không mutation, kể cả builtin materialization/Vault/SQL/kube
    calls.
- **Validation:** V-D, V-BE, V-FE, V-UI(T19): candidate wildcard/shared vs AWS,
  class specific, exact tie, non-overlap và incomplete context fake.
- **Conformance:** dùng core matcher hoặc extraction preserving behavior;
  thêm no-mutation/weighted-match tests; không đổi expected fixtures.
- **Phụ thuộc/rủi ro/external:** T02/T02B/T03/T16/T17; scope preview không thay full
  graph Preview. Không external mutation.

#### T20 — Lỗi backend theo code, thông báo Console tiếng Việt

- **Mục tiêu/mã:** VI-03; VI-04/05 error coverage; BE+D+FE, M–L, ③.
- **Docs trước:** `architecture/contracts/operation-contracts.md`; affected UC
  00/01/02/03/04/05/07/09/12/16 `ui/api-mapping.md` nếu có và `ui/states.md`;
  `architecture/ui/README.md` quy tắc error presentation.
- **Code/test:** `shared/api/client.ts`, page-specific error mappers,
  `delivery/http/resource_types.go` management errors, server/workload/config/
  store/transition handlers; unit/e2e safe error tests. Bằng chứng client
  `frontend/src/shared/api/client.ts:9` chỉ đọc error/field, không code; target code `backend/internal/delivery/http/server.go:446`.
- **Giao Claude:**
  - Claude trả stable code/field/safe params, không trả secrets/submitted values/raw
    provider errors.
  - Mappers hard-code tiếng Việt theo glossary. Unknown code dùng fallback Việt và
    correlation an toàn nếu có, không hiển thị raw English error string.
- **Chấp nhận:**
  - Mọi luồng Console có lỗi tiếng Việt rõ nghĩa. HTTP status/scoping/CAS đúng và
    technical payload không dịch.
  - Retry không tự replay mutation; validation params chỉ có metadata an toàn. Selected
    renderer failure không chuyển sang native. Product không giữ parser English-message
    legacy.
- **Validation:** V-D, V-BE, V-FE, V-UI(T20): representative ID/duplicate/invalid
  variable/auth/stale/BUSY/store-copy/unknown error và secret-leak checks fake.
- **Conformance:** planner output/error classification không bị thay bởi public
  envelope; suite unchanged. Product HTTP tests assert codes và safe fields.
- **Phụ thuộc/rủi ro/external:** T02..T19 để có catalog handlers mới và Việt hóa
  pages. Không external mutation; không log raw errors chứa credentials.

#### T21 — Bỏ context app.profile/app.region aliases

- **Mục tiêu/mã:** LEG-04; BE+D, S–M, ③/0 bước trực tiếp.
- **Docs trước:** ADR-011 alias paragraph; UC-03 specification,
  `implementation/humanitec-compatibility.md`, `architecture/contracts/operation-contracts.md`.
- **Code/test:** `planning/context.go`, `environment_scope_test.go`, template/
  placeholder tests và seed/Definitions dùng alias nếu tìm thấy.
  Bằng chứng `backend/internal/planning/context.go:100`.
- **Giao Claude:**
  - Claude chỉ bỏ app.profile/app.region; env.profile/env.region là nguồn chuẩn. Humanitec
    app.id/name và context khác giữ nguyên.
  - Claude tìm references toàn repo; không sửa reference fixtures hoặc evidence lịch sử.
- **Chấp nhận:**
  - Product ResolveContext báo unknown với aliases đã bỏ. Env values đúng target; authored
    current product inputs không còn aliases. Server không rewrite để giữ compatibility.
- **Validation:** V-D, V-BE; `rg -n 'app\.(profile|region)' backend frontend orchestrator_docs`;
  không Playwright vì chỉ context/unit, frontend không có control alias.
- **Conformance:** nếu fixtures cần aliases thì adapter ở `backend/test/conformance/`
  cung cấp legacy reference resolver, không để alias trong default product.
- **Phụ thuộc/rủi ro/external:** T20 để lỗi mới Việt; T29 dựng dev mới không alias.
  Không external mutation.

#### T22 — Bỏ HOST_CONTEXT và registration body cũ

- **Mục tiêu/mã:** LEG-05 toàn bộ; BE+D, L, ③/0 bước trực tiếp.
- **Docs trước:** UC-04 specification/realization; ADR-013 host-context paragraph;
  `architecture/connection-credentials.md`, domain/schema; UC-06/08 target contracts.
- **Code/test:** `delivery/http/connections.go`, `application/connection/service.go`,
  upload/service tests; `domain/application/application.go`, `adapters/kubernetes/`
  verifier/kubectl/existing executor và credential target tests; seed,
  postgres/registration.go, store defaults; HTTP uc04 tests.
  Bằng chứng old body `backend/internal/delivery/http/connections.go:100`, legacy inference `backend/internal/domain/application/application.go:156`.
- **Giao Claude:**
  - Kubernetes Connection phải có scoped credential KUBECONFIG tường minh. Claude bỏ
    HOST_CONTEXT execution/inference/API body và không giữ host fallback.
  - Seed không giả READY khi chưa upload credential. Nếu default link của Organization cần
    record, sửa schema/seed nhất quán, không tạo dangling reference.
  - Fake/unit targets có metadata và fake credentials tường minh. AWS flow/seed hiện hành
    giữ nguyên; không mở rộng onboarding.
- **Chấp nhận:**
  - Body cũ {key,clusterId,kubeContext} bị từ chối; upload được chấp nhận. Thiếu private
    credential phải fail closed; org được resolve đúng khi invocation.
  - Cancellation chỉ dọn temp files của attempt; execution không dùng host context. Seed
    local không làm create app fail vì default reference thiếu.
- **Validation:** V-D, V-BE; DB constraints trên disposable DB được phép;
  không Playwright mới vì upload UI T06 unchanged. Credential tests dùng stub
  kubectl, không cluster verification. Shell changed: `bash -n <changed scripts>`.
- **Conformance:** synthetic target nằm ở harness/fake execution only, không
  restore HOST_CONTEXT runtime branch; run full suite.
- **Phụ thuộc/rủi ro/external:** T17/T20; database baseline cleanup T28.
  Retained K8S-4F KUBECONFIG không bị mất bởi thay đổi này; không mutate cluster.

#### T23 — Bỏ legacy Secret Store/bootstrap paths

- **Mục tiêu/mã:** LEG-06; BE+D, M–L, ③/0 bước trực tiếp.
- **Docs trước:** ADR-012 legacy store/backfill/Compose conversion paragraphs;
  ADR-006 historical provider ownership ghi phần bị thay thế; UC-04/12 specs;
  `architecture/database/schema.md`, `operations/docker-local.md`.
- **Code/test:** `adapters/vault/registry.go`, registry/bootstrap tests;
  `bootstrap/legacy.go`, `bootstrap.go`, `platformvault.go`, tests;
  secretstores/bootstrap.go/tests; persistence Store.Legacy/backfill/managed
  admission APIs và SQL/inmemory implementations; HTTP legacy DTOs.
  Bằng chứng registry token legacy `backend/internal/adapters/vault/registry.go:42`, backfill `backend/internal/bootstrap/legacy.go:19`.
- **Giao Claude:**
  - Claude bỏ legacy flags-to-runtime credential path và conversion. Ordinary Compose
    bootstrap, private credential store, immutable refs, cleanup và managed credential
    refresh CAS được giữ.
- **Chấp nhận:**
  - Workload store resolve private credential ref; không fallback qua flags/token/global.
    Fresh Compose boot verify/probe-cleanup trước READY và restart idempotent.
  - Owned token refresh CAS không ghi đè user store. Public DTO không có legacy/token/ref.
    Store chưa cấu hình phải fail closed.
- **Validation:** V-D, V-BE; V-FE nếu gỡ legacy label/types ở T05/T07;
  V-UI(T23) chỉ khi UI label thay đổi, fake ordinary-store selection; SQL gates
  disposable nếu đã được phép. Không bật live bootstrap Vault ngoài scope.
- **Conformance:** secret-store lifecycle không reference matching; suite nguyên
  kết quả; harness không phụ thuộc global legacy Vault configuration.
- **Phụ thuộc/rủi ro/external:** T20; coordinated LEG-07 T24 và T28 schema.
  Không dựng lại K8S-4F ordinary store lúc implement; reset thuộc T29.

#### T24 — Variable metadata-only, bỏ materialize legacy refs

- **Mục tiêu/mã:** LEG-07; BE+D, M, ③/0 bước trực tiếp.
- **Docs trước:** ADR-012 variables/store-copy, UC-12 specification/realization;
  domain/schema và `architecture/contracts/operation-contracts.md`.
- **Code/test:** `domain/configuration/configuration.go`,
  `application/configuration/service.go`, `store_switch.go`, tests;
  vault/provider/registry resolution, SQL/inmemory persistence backfill tests.
  Bằng chứng `backend/internal/domain/configuration/configuration.go:26`, `backend/internal/application/configuration/service.go:159`.
- **Giao Claude:**
  - Variable.Value nằm trong metadata; Secret dùng StoreKey/ref. Claude bỏ
    LegacyVariable/materialize/legacy error branches, không đọc secret bytes vào
    variable/snapshots và giữ owning store của immutable refs.
- **Chấp nhận:**
  - Variable CRUD không gọi Vault; Secret resolve đúng scoped store khi execution. Đổi
    store chỉ copy Secrets; Variables giữ nguyên.
  - Public DTO không có Legacy field; revision/config versions/BUSY/admission đúng.
- **Validation:** V-D, V-BE; không Playwright mới vì UI behavior T07 unchanged,
  service/HTTP tests chứng minh no-Vault variable and secret redaction.
  PostgreSQL integration chỉ khi disposable DB được phép.
- **Conformance:** product config adapter không thay fixture params/output;
  run suite, không sửa fixtures.
- **Phụ thuộc/rủi ro/external:** T23/T20; old dev refs không migrate, reset T29.
  Không mutate actual Vault values/retained DB lúc implement.

#### T25 — Chỉ publicRoutes, bỏ publicPort alias

- **Mục tiêu/mã:** LEG-09; BE+FE+D, M, ③/0 bước trực tiếp.
- **Docs trước:** UC-06 specification public routing; UC-16 specification
  publicPort compatibility, realization/UI mapping; routing operation contracts.
- **Code/test:** `domain/environment/document.go`, `planning/score/score.go`,
  score tests, route renderer tests; `WorkloadEditorPage.tsx/.test.tsx`,
  backend workloadconfig tests và scripts samples thuộc product.
  Bằng chứng alias `backend/internal/domain/environment/document.go:31`, editor conversion `frontend/src/features/workloads/WorkloadEditorPage.tsx:112`.
- **Giao Claude:**
  - Product Score/public projection từ chối publicPort; không tự rewrite old drafts.
    Multi-path validation, same-host ownership và port/class/reference semantics giữ
    nguyên.
  - Claude giữ protected literal security policy dù nhánh code đang mang tên legacy.
- **Chấp nhận:**
  - publicRoutes round-trip đúng; publicPort bị từ chối với stable code. Invalid/duplicate
    path không deploy; UI không serialize publicPort.
  - Port selection/default của T08 giữ đúng và không có silent route fallback.
- **Validation:** V-D, V-BE, V-FE, V-UI(T25): route edit/import valid/old alias
  fail, pending Preview fake. Không reconcile Ingress cluster thật.
- **Conformance:** nếu reference có alias thì loader normalize reference-only;
  fixtures read-only, full suite unchanged.
- **Phụ thuộc/rủi ro/external:** T08/T20; old dev drafts bỏ ở T29.
  Không external mutation.

### Đợt 3 — baseline/reset và cải tiến lớn

#### T26 — Application chỉ giữ identity/config metadata hiện hành

- **Mục tiêu/mã:** LEG-01; BE+D, L, ③/0 bước trực tiếp.
- **Docs trước:** ADR-011 legacy Application columns/migration; ADR-012 ownership;
  UC-01/06 specs/realizations; domain model, schema/ERD/operation contracts.
- **Code/test:** `domain/application/application.go`, application service/tests;
  SQL repositories/migration guards, store/store.go/legacy_binding_test.go;
  persistence binding tests, target/deployment snapshot/seed/tests.
  Bằng chứng legacy fields `backend/internal/domain/application/application.go:46`, JSON migration `backend/internal/adapters/store/store.go:290`.
- **Giao Claude:**
  - Claude bỏ profile/connection/region/runtime/provider cũ trên Application; chỉ
    Environment giữ binding. Historical target snapshot/generation/version/fencing/org
    scoping được giữ.
  - Có thể dùng destructive dev schema alteration tạm trước baseline T28 để fresh task DB
    pass; không chạy trên DB đang giữ và không backfill dữ liệu.
- **Chấp nhận:**
  - Create chỉ nhận name/subdomain; Environment mới unconfigured. Mọi Preview/Deploy
    resolve Environment; Application repo không lưu target fields.
  - SQL/inmemory structs/save/list thống nhất; các Environment vẫn bind độc lập đúng.
- **Validation:** V-D, V-BE; fresh/reopen/transaction SQL tests trên disposable
  DB khi được phép. Không Playwright vì user flow unchanged/T07 đã quay.
- **Conformance:** harness cấp explicit env context, không yêu cầu app legacy
  fields; preserve app.id/contract fixtures và output expectations.
- **Phụ thuộc/rủi ro/external:** T21..T25; cùng T27/T28. Không runtime mutation;
  DB đang giữ chưa nâng cấp cho đến approval T29.

#### T27 — Bỏ product LEGACY_APPLICATION scope và seed upgrade templates

- **Mục tiêu/mã:** LEG-02; BE+D, L, ③/0 bước trực tiếp.
- **Docs trước:** ADR-011 scope/name/legacy migration, ADR-013 cloud boundary,
  ADR-012 generation contract; UC-03/06/08 specs/realizations, schema/ERD,
  `implementation/uc06-planner-reference.md`, compatibility matrix.
- **Code/test:** `domain/environment/environment.go`, planning/context.go,
  expand.go/identity.go/graph.go/target hash paths, environment_scope_test.go;
  seed/seed.go/binding_test.go; persistence/SQL scope checks;
  `backend/test/conformance/loader.go` và tests.
  Bằng chứng branches `backend/internal/planning/context.go:29`, seed `backend/internal/seed/seed.go:380`, harness
  `backend/test/conformance/loader.go:135`.
- **Giao Claude:**
  - Product chỉ có scope ENVIRONMENT, @infra theo app/env/generation. Claude bỏ seed-
    upgrade heuristics cho old templates và app-scope compatibility.
  - Generation 0 namespace/descriptor hiện hành và generation >0 isolation giữ nguyên;
    không đổi identity chỉ để loại chữ legacy.
  - Local acceptance seed dùng current env semantics tường minh; RD-01 profile và matcher
    ranking không đổi.
- **Chấp nhận:**
  - Product không còn enum/fallback LEGACY_APPLICATION; invalid scope bị từ chối. VPC/EKS
    env-scoped và provider-safe name bounds đúng.
  - Gen0/gen>0 không collision; new-scope product không còn hardcoded app infra refs.
    Trusted internal cluster binding/descriptor giữ theo ADR-013.
- **Validation:** V-D, V-BE; whole-repo search ScopeLegacy/LEGACY_APPLICATION và
  legacy template refs, phân biệt test harness/evidence. SQL constraints fresh
  DB được phép. Không Playwright hay AWS run, planning/unit/CLI only.
- **Conformance:** di chuyển historical path/name semantics vào harness adapter
  dưới `backend/test/conformance/`, không giữ default product branch; fixture
  14/22/25/26 matching unchanged và full accepted/rejected suite pass.
- **Phụ thuộc/rủi ro/external:** T26/T16/T17/T21; tiếp T28. Không rebuild retained
  cluster lúc implement. Old seeded cloud identity incompatible: không cloud run.

#### T28 — Baseline DB dev sạch và bỏ startup upgrade branches

- **Mục tiêu/mã:** LEG-08, tích hợp LEG-01/02/05/06/07; BE+D, L, ③/0 bước.
- **Docs trước:** `architecture/database/schema.md`, ERD;
  `operations/docker-local.md`, `operations/k8s4f-dev.md` procedure reset/backup;
  ADR-011/012 phần migration obsolete và UC-09 history backfill design.
  Giữ ADR-002 transaction invariants, không bỏ history/snapshot safety.
- **Code/test:** `adapters/postgres/migration.sql`, `store.go`, repositories,
  connection/environment/workload migration tests thay bằng fresh-baseline
  tests; `adapters/store/store.go` JSON load/backfill, ops/legacy tests;
  persistence binding/registration/operations suites; disposable DB test runner
  nếu cần, build/Compose startup paths.
  Bằng chứng versioned migrations `backend/internal/adapters/postgres/store.go:91`, JSON upgrades
  `backend/internal/adapters/store/store.go:277`.
- **Giao Claude:**
  - Claude tạo baseline current schema từ DB rỗng và bỏ old upgrade/backfill support;
    không chỉ xóa ledger rồi giả schema đúng.
  - Current JSON format vẫn reopen được; missing-required data có lỗi rõ.
    Transaction/FK/unique/CAS/fencing/integrity checks giữ nguyên.
  - Task chỉ chuẩn bị code/runner, không reset Compose volumes hoặc cluster hiện có.
- **Chấp nhận:**
  - Fresh DB init, boot thứ hai và reopen đúng schema constraints và idempotent. Old
    baseline mismatch báo cần dev reset trước serving, không silent conversion.
  - LEG-01..09 runtime branches đã bỏ hoặc có mapped task rõ; harness/evidence là ngoại lệ
    duy nhất.
  - Backup/restore-smoke/owner inventory và exact commands cho T29 đủ cụ thể để review.
- **Validation:** V-D, V-BE; full fresh PostgreSQL test suite trên DB disposable
  được phép, bao gồm races/multibackend/reopen/rollback; changed shell `bash -n`.
  Không Playwright vì task schema/CLI; T29 chứng minh real user flow.
- **Conformance:** all fixtures untouched, old formats normalized only in harness;
  suite phải pass sau baseline không cần historical DB/JSON adapters.
- **Phụ thuộc/rủi ro/external:** T22..T27. External disposable DB gate riêng;
  **không reset IDP hoặc K8S-4F**. Nếu chưa có DB được phép, code review được
  nhưng task chưa done về PostgreSQL checks; báo gate, không claim pass.

#### T29 — Backup, xác nhận reset và dựng lại dev K8S-4F

- **Mục tiêu/mã:** LEG-08 execution; VI-04/05 real evidence; D+BE/Shell, L.
  Không phải giảm bước UI. Dựng lại trạng thái dev là quyết định đã chốt, nhưng
  mỗi run external mutation cần xác nhận ngay trước thực hiện.
- **Docs trước:** `operations/k8s4f-dev.md`, `operations/docker-local.md`,
  verification record mới và code map/current state sau thành công. Không sửa
  evidence a0ef186 thành kết quả mới. Nguồn ownership/data `orchestrator_docs/operations/k8s4f-dev.md:29`,
  `:58`, `:65`, `:81`.
- **Code/test dự kiến:** reuse/update `frontend/test/e2e/k8s4f-dev-human.mjs`,
  backend integration k8s4f-dev-playwright.sh và k8s4f-dev-compose-video.sh;
  reset/backup wrapper mới dưới `backend/test/integration/` nếu runbook cần.
  Claude chỉ chạy code/mutation được Codex giao rõ sau approval, không tự deploy.
- **Chuẩn bị độc lập trước hỏi:** đọc metadata/config không secret, inventory cả
  hai retained apps/namespaces/PVC/Vault/VSO/Connection/Store. Ghi run ID, ownership,
  versions, exact target DB/volume/namespace và danh sách không chạm (cluster cũ,
  unrelated workload, Vault credential/data/bootstrap volumes). Chuẩn bị backup
  IDP PostgreSQL và workload PostgreSQL/PVC bằng phương án nhất quán; artifacts
  0700/0600 ngoài repo, không video/upload. Kiểm readability/restore-smoke trên
  disposable target khi đã được phép; nếu cần external backup read/write thì
  approval run phải bao gồm bước đó, không chạy trước hỏi. Lưu handoff recovery.
- **Gate bắt buộc:** Codex trình exact commands, backup/restore và cleanup plan,
  downtime/data scope, rồi hỏi người dùng ngay trước reset/deploy. Approval chờ
  thật; thời gian trôi không phải đồng ý. Nếu chưa được xác nhận, dừng T29 nhưng
  các task độc lập T30/T31 có thể tiếp tục local. Không dùng approval cũ cho run
  khác hoặc mở rộng resource list.
- **Trình tự run sau xác nhận:** backup trước destructive step, verify backup;
  quiesce writers theo plan, xử lý source workloads/PVC owned bằng lựa chọn đã
  duyệt (retain có recovery hoặc xóa sau backup). Không để DB IDP mới vô tình
  nhận quản lý resource cũ qua name collision. Reset đúng IDP DB/baseline;
  giữ Vault volumes và cluster, không dùng `docker compose down -v` toàn stack.
  Re-register KUBECONFIG/ordinary Store bằng UI; Create app, chọn staging đích/
  store, variables/secrets, workloads PostgreSQL/BE/FE, Preview và Deploy UI.
  Keys/IDs mới có thể khác; runbook phải ghi ID/key/namespace/URL mới. Không tạo
  lại cluster nếu còn healthy. Không làm production hoặc AWS deployment.
- **Chấp nhận:**
  - Baseline DB sạch và cluster/VSO thật hoạt động. Workloads/PVC của app mới Ready;
    diagnostics connection/environment/secret/database pass và reload dùng state mới.
  - Nếu run plan yêu cầu giữ data, phải chứng minh records được restore; nếu chấp nhận DB
    dev rỗng, phải ghi rõ. Secret/kubeconfig/token không có trong public artifacts.
  - Cleanup ownership được xác minh; generation cũ không tiếp tục serve/write trái plan.
    Backup giữ đúng thời hạn đã duyệt và không tự xóa khi run fail.
- **Validation:** V-D, V-BE, V-FE cho code thay đổi; `bash -n <changed scripts>`;
  headed real Playwright k8s4f-dev runner với phụ đề Việt, evidence ngoài Git.
  Exact runner environment/commands do runbook chuẩn bị T28 ghi, không đọc/in
  secrets ở CLI output. Runbook checks read-only sau deploy, assertion paths
  và full video decode/frame review. Không replay live chỉ để quay đẹp hơn.
- **Conformance:** run V-BE trước live; live không thay fixture semantics.
- **Phụ thuộc/rủi ro/external:** T01..T28, user approval và backup gate. **External
  mutation có:** IDP DB, Vault store credential registration, Kubernetes owned
  namespace/workloads/PVC. Đây là mục buộc dựng lại trạng thái K8S-4F; không
  cấp quyền xóa unrelated cluster/PVC/Vault data hoặc upload evidence.

#### T30 — Rename key và cập nhật tham chiếu pending

- **Mục tiêu/mã:** CFG-01; BE+FE+D, L; ② opt-in, ③ atomic multi-workload edit;
  bớt n vòng mở/sửa/lưu workload.
- **Docs trước:** UC-12 specification VAR-01/BR-05, realization/UI screens/states/
  api-mapping; UC-16 specification/realization/UI; UC-05/07 pending semantics;
  `architecture/contracts/operation-contracts.md`, ADR-002 transaction boundaries.
- **Code/test:** `application/configuration/service.go`, service tests;
  workloadconfig reconstruction/service; pending preview/service; persistence
  atomic config+draft command/SQL/inmemory tests; HTTP configuration.go;
  SettingsPage.tsx/.test.tsx, workload key binding tests; e2e T30.
  Bằng chứng rename hiện chỉ entries `backend/internal/application/configuration/service.go:241`.
- **Giao Claude:**
  - Claude thêm checkbox explicit “Đổi tên và cập nhật tham chiếu”; không tự rewrite
    runtime/current set. Không chọn checkbox thì giữ rename-only behavior đã documented.
  - KEY refs được cập nhật đúng app/env, không đổi resource/Service refs khác. Container
    aliases/imported many-to-one mappings giữ nguyên; không copy secret bytes.
- **Chấp nhận:**
  - Config revision và desired drafts commit nguyên tử với expected config/env/draft
    versions; BUSY/CAS an toàn. Deployed workload chưa có draft được reconstruct thành
    reference-based desired Score.
  - Collision hoặc edit không lossless phải fail, không partial rename. Preview liệt kê
    workloads bị ảnh hưởng; Deploy tường minh và old key không thành literal.
  - Lỗi và code mappings dùng tiếng Việt đầy đủ.
- **Validation:** V-D, V-BE, V-FE, V-UI(T30): variable/secret rename affects 2
  workloads, alias giữ, no opt-in, collision/stale/BUSY và pending Preview fake.
  DB atomic/race/reopen tests chỉ disposable được phép; không retained rollout.
- **Conformance:** reference resolution/delta invariants unchanged; fixtures
  inline config normalized qua harness, full suite pass; product tests atomicity.
- **Phụ thuộc/rủi ro/external:** T07/T08/T20/T24/T28; không phụ thuộc approval
  live T29 để code local. Không mutate real workloads/secret stores mặc định.

#### T31 — Form cấu hình typed theo driver/module

- **Mục tiêu/mã:** RD-05; RD-04; RT-04 metadata authoritative; BE+FE+D, L.
  ① context/default schema, ② chọn preset; bớt viết 2 JSON objects và 1–3
  context/reference expressions trong trường hợp phổ biến.
- **Docs trước:** UC-03 specification driver input/provision policy, realization/
  UI mapping/screens/states; UC-02 UI capability note; `architecture/contracts/operation-contracts.md`,
  workload/resource contracts; UC-06/08 reference/dependency contracts.
- **Code/test:** catalog/policy.go/service.go, Terraform inspect.go/module contract,
  read-only capability metadata handler mới; ResourceDefinitionsPage.tsx/tests,
  ResourceTypesPage capability status; placeholder/provision/planning tests;
  schema typed form/editor components mới dưới frontend platform; e2e T31.
  Bằng chứng static variable schema `backend/internal/application/catalog/policy.go:79`, executor-owned vars
  `backend/internal/adapters/terraform/inspect.go:22`, JSON form `frontend/src/features/platform/ResourceDefinitionsPage.tsx:71`.
- **Giao Claude:**
  - Claude cung cấp metadata chỉ đọc theo supported type/driver/module:
    types/defaults/required/resource-param/context sources và executor-owned exclusions.
    Không suy toàn bộ Resource Type từ Terraform vars.
  - Namespace preset `${context.env.namespace}` và VPC refs @infra dùng policy hiện có;
    overrides opt-in. Không đoán storage/CIDR/engine/capacity ngoài defaults đã chốt.
- **Chấp nhận:**
  - Literals đúng types; whole placeholders giữ deferred checking và interpolated string
    rules. Params của workload không bị điền lại vào Definition hoặc mất override.
  - Unknown/null/credential/master_password bị từ chối; default omission đúng contract.
    Advanced JSON round-trip lossless với known fields; module ngầm theo T02; không nhận
    remote source/templates/commands.
  - Provision editor mô tả consumer/provider/is_dependent/match_dependents và refs, không
    đảo edge consumer → provider. Advanced mode giữ được trường hợp không biểu diễn an
    toàn.
  - Text và aria-label dùng tiếng Việt; form không tự preview/provision.
- **Validation:** V-D, V-BE, V-FE, V-UI(T31): postgres/namespace/vpc/eks/aurora
  forms fake, typed literals/placeholders/defaults/override/errors và provision
  reference Preview; no cloud execution. Metadata no-mutation/auth tests;
  PostgreSQL registration tests nếu disposable được phép.
- **Conformance:** defaults/type/reference/provision graph semantics không đổi;
  full suite, fixtures read-only, runtime metadata không viết lại fixtures.
- **Phụ thuộc/rủi ro/external:** T02/T04/T16/T18/T19/T20/T28; không cần T29 live
  approval để implement local. Không executor/provider extension.

## 6. Thứ tự và phụ thuộc tổng thể

| Nhóm | Thứ tự/gate |
|---|---|
| Nền | T01 trước mọi task trang; T10 nên chạy đầu đợt 1. |
| FE catalog | T02 → T02B → T03; T04 sau T02; T05/T06/T07/T08/T09/T11..15 sau nền, mỗi trang một task. |
| BE resource | T16/T17 → T18/T19; T20 bao phủ API errors hiện hành/mới. |
| LEG từng vùng | T21..25 sau T20; T23 → T24; T25 sau T08. |
| Structural cleanup | T26 → T27 → T28; T22..25 phải hoàn tất trước baseline. |
| Live reset | T29 sau baseline và page/API checks; backup + xác nhận ngay trước mutation. |
| Cuối | T30/T31 sau các dependency đã ghi; local work không bị chặn bởi live approval T29. |

Danh sách mã nguồn đầy đủ:

| Mã nguồn | Task hoặc trạng thái |
|---|---|
| RD-01 | T02; renderer profile ngầm T03. |
| RD-02 | T02 Kubernetes; T16 Terraform. |
| RD-03 | T02. |
| RD-04 | T31. |
| RD-05 | T31. |
| RD-06 | T02B editor, T03 tích hợp; T19 endpoint chính xác. |
| RD-07 | T02 removal UI; T17/LEG-03 removal API. |
| RD-08 | T03 UI extraction; T18 installed bundle endpoint/selector. |
| RD-09 | Tương lai, không task triển khai. |
| RT-01..RT-04 | T04; RT-04 authority metadata T31. |
| SS-01..SS-04 | T05. |
| CN-01..CN-02 | T06. |
| ENV-01 | T07. |
| WL-01..WL-02 | T08. |
| APP-01 | T09. |
| CFG-01 | T30. |
| UI-01 | T02/T04/T05/T06 và patterns dùng chung; không đổi layout ngoài các trang này. |
| UI-02 | T02/T03; public existing-cluster T17; bundle T18. |
| VI-01 | T01. |
| VI-02 | T02..T15 và T02B, các controls mới T16/T18/T19/T30/T31 cũng hard-code Việt. |
| VI-03 | T20; handlers mới dùng contract error đã chốt. |
| VI-04 | T01 nền, test/video mỗi task Web Console, T29 real evidence. |
| VI-05 | UI docs mỗi task trang, runbook/evidence T29, terminology T01. |
| LEG-01 | T26. |
| LEG-02 | T27. |
| LEG-03 | T17. |
| LEG-04 | T21. |
| LEG-05 | T22, toàn HOST_CONTEXT. |
| LEG-06 | T23. |
| LEG-07 | T24. |
| LEG-08 | T28 code/baseline; T29 approved external reset/rebuild. |
| LEG-09 | T25. |
| LEG-10 | Chờ quyết định, không task triển khai. |

Không cần hỗ trợ data/API cũ để nối các task. Các commit structural cleanup
được kiểm trên fresh disposable state; không deploy chúng lên IDP đang giữ
trước T29. Ghi trạng thái “cần reset dev trước dùng” trong handoff T26..28.
Frontend intermediate T03 dùng API hiện tại; T18 hoàn tất bundle selector.
Không tuyên bố RD-08 hoặc exact matching-preview done ở đợt 1.

## 7. Các mục chờ quyết định và tương lai

- **Cách chọn mẫu dựng ứng dụng:** automatic criteria như hiện tại hay Developer
  chọn theo Environment. Chỉ ghi câu hỏi, không thiết kế binding/selector mới.
- **LEG-10:** VSO-only hay tiếp tục Agent delivery. Giữ Agent hiện tại; không
  xóa Injector/Agent code/dependency trong LEG-01..09. Nếu chốt bỏ thì review
  ADR-008/006/010 và lập task riêng.
- **Tên hiển thị mẫu:** hiện chỉ technical ID; friendly name là mở rộng contract
  tương lai, không thuộc T03/T18.
- **RD-09:** chỉ cân nhắc sau khi modes/copy/preview RD-06 không đủ. Shared
  condition set cần entity org-scoped/name/version, Definition FK, unique
  (set,type), chặn xóa đang dùng, impact preview khi sửa, atomic policy version
  và Preview pin. Phải có ADR/schema/API/planner design; fixtures inline giữ ở
  harness. Không lập task triển khai trong kế hoạch này.

Hard-code Việt, VPC/EKS inherit, dọn HOST_CONTEXT và reset DB không còn là câu
hỏi product. T29 vẫn cần xác nhận run cụ thể theo yêu cầu external mutation.
Tại gate đó người dùng xác nhận backup retention, giữ/restore hay xóa data dev
workload và danh sách cleanup cụ thể; chưa có approval để tự xóa PVC.

## 8. Definition of done và mẫu handoff

Một task done khi acceptance kiểm được, docs/code/tests nhất quán, checks đúng
scope pass, conformance không đổi fixtures, evidence reviewed và không mất
user-owned changes. Claude không commit; Codex review và commit/push task theo
AGENTS.md sau validation. Browser tasks có headed video/assertions/phase marks/
phụ đề Việt, ghi adapter thật/fake; task API/unit ghi rõ vì sao không có video.
Session tmux task được đóng theo runbook sau completion hoặc saved handoff.

Toàn kế hoạch done khi T01..31 và T02B hoàn tất (T29 thực hiện sau approval), mọi trang
Console và lỗi user-facing Việt, kỹ thuật/payload/staging/production nguyên vẹn;
RD-01 matching/seed policy giữ; public existing-cluster và product LEG-01..09
đã bỏ; default renderer/Agent hiện hành giữ; fresh DB baseline và K8S-4F mới
được xác minh. RD-09/LEG-10/cách chọn mẫu/friendly name vẫn là deferred, không
được tính như thiếu implementation đã chốt. Nếu T29 bị chặn approval, ghi rõ
kế hoạch chưa done về external rebuild, không giả live proof từ fake checks.

Mẫu handoff từng task:

```text
Task: Txx; mã nguồn; branch.
Commit: message prefix Txx:; SHA thật sau khi commit (hoặc lý do chưa commit).
Outcome: behavior và payload cụ thể đã tồn tại.
Docs trước code: paths/requirement IDs/ADR thay đổi.
Vùng code/test: file chính; giới hạn scope; user-owned changes còn lại.
Checks: lệnh, exit status, SQL opt-in có/không, conformance kết quả.
Evidence: run ID, adapter mode, video/captions/phase marks/assertion paths;
          frame/decode review; không browser scenario thì nêu lý do.
External: approval/run scope, backup/restore/cleanup result; retained resources.
Rủi ro/deviation/chưa chạy: mô tả thật và lý do, dependency còn chờ.
Commit/push: SHA thật sau khi commit, push result hoặc lý do chưa thực hiện;
             tmux task đã đóng. Tra lịch sử bằng git log --grep '^Txx:'.
```

Khi chỉ lập hoặc review kế hoạch: không chạy product mutation, không commit/push
nếu task yêu cầu như vậy. Các checks của file này chỉ docs/whitespace/source
coverage, không phải bằng chứng implementation hoặc live verification.

## 9. Trạng thái

Mỗi task có một dòng; trạng thái hợp lệ là TODO / IN_PROGRESS / DONE / BLOCKED.
Commit message của mọi commit thuộc task bắt đầu bằng ID task, ví dụ `T05: ...`.
Tra commit bằng `git log --grep '^Txx:'`, thay Txx bằng ID thật (gồm T02B).
Commit hoàn tất task cập nhật trạng thái DONE và evidence path trong cùng
commit đó. Bảng không lưu SHA; handoff ghi SHA thật sau khi commit.
BLOCKED phải có lý do và điều kiện mở khóa ở cột ghi chú. Ghi evidence path
và kết quả kiểm chứng; chỉ đánh dấu DONE khi đáp ứng mục 8. Bảng ghi trạng thái triển khai; chỉ task DONE có implementation/evidence đã
review theo mục 8.
Các khoảng Txx–Tyy gồm toàn bộ task trong khoảng; T02B ghi riêng khi cần.

| ID | Tên ngắn | Phụ thuộc | Trạng thái | Ghi chú/evidence path |
|---|---|---|---|---|
| T01 | Chốt thuật ngữ và runner/locator dùng chung | Không | DONE | V-D/V-FE/V-UI pass; fake headed smoke, failure retention/cleanup reviewed. Evidence: `orchestrator_docs/verification/2026-10-10-T01-refactor-foundation.md`; `/tmp/poc-refactor-T01-pass-1791628004`. |
| T02 | Form tạo Cấu hình tài nguyên | T01 | TODO | — |
| T02B | Editor Điều kiện áp dụng dùng chung | T01, T02 | TODO | — |
| T03 | Trang Mẫu dựng ứng dụng | T01, T02, T02B | TODO | — |
| T04 | Trang Loại tài nguyên | T01, T02 | TODO | — |
| T05 | Trang Kho bí mật | T01 | TODO | — |
| T06 | Trang Kết nối | T01 | TODO | — |
| T07 | Trang Environment Settings | T01 | TODO | — |
| T08 | Trang Workload editor | T01 | TODO | — |
| T09 | Trang Tạo ứng dụng | T01 | TODO | — |
| T10 | Đăng nhập và shared shell | T01 | TODO | — |
| T11 | Danh sách ứng dụng | T01, T10 | TODO | — |
| T12 | Application home | T01, T10 | TODO | — |
| T13 | Standalone Score Preview | T01, T10 | TODO | — |
| T14 | Deployment history | T01, T10 | TODO | — |
| T15 | Deployment details | T01, T10 | TODO | — |
| T16 | VPC/EKS luôn dùng Connection của Environment | T02 | TODO | — |
| T17 | Đóng existing-cluster registration công khai | T02, T03 | TODO | — |
| T18 | API danh sách bundle và hoàn tất selector mẫu | T03 | TODO | — |
| T19 | Matching preview chính xác, không persist | T02, T02B, T03, T16, T17 | TODO | — |
| T20 | Lỗi backend theo code, thông báo Console tiếng Việt | T02–T19, T02B | TODO | — |
| T21 | Bỏ context app.profile/app.region aliases | T20 | TODO | — |
| T22 | Bỏ HOST_CONTEXT và registration body cũ | T17, T20 | TODO | — |
| T23 | Bỏ legacy Secret Store/bootstrap paths | T20 | TODO | — |
| T24 | Variable metadata-only, bỏ materialize legacy refs | T23, T20 | TODO | — |
| T25 | Chỉ publicRoutes, bỏ publicPort alias | T08, T20 | TODO | — |
| T26 | Application chỉ giữ identity/config metadata hiện hành | T21–T25 | TODO | — |
| T27 | Bỏ product LEGACY_APPLICATION scope và seed upgrade templates | T26, T16, T17, T21 | TODO | — |
| T28 | Baseline DB dev sạch và bỏ startup upgrade branches | T22–T27 | TODO | — |
| T29 | Backup, xác nhận reset và dựng lại dev K8S-4F | T01–T28, T02B; xác nhận run/backup | TODO | — |
| T30 | Rename key và cập nhật tham chiếu pending | T07, T08, T20, T24, T28 | TODO | — |
| T31 | Form cấu hình typed theo driver/module | T02, T04, T16, T18, T19, T20, T28 | TODO | — |
