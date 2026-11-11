---
id: ADR-042
artifact: architecture-decision
status: current
last_reviewed: 2026-11-10
related: [ADR-012, ADR-030, ADR-036, ADR-040, UC-06, UC-12]
---

# ADR-042 — Một Fleet bundle mỗi Environment, secret theo version

Status: Accepted — 2026-11-10, người dùng duyệt nội dung issue #67 và cho phép
làm liên tục tới nghiệm thu.

## Bối cảnh

Trước quyết định này, mỗi lần Deploy một workload:

- chép mọi giá trị secret sang Vault `values/<deploymentID>`;
- tạo Vault policy/role, ServiceAccount, VaultConnection, VaultAuth và
  VaultStaticSecret mới theo `deploymentID`;
- tách manifest thành các unit `<workload>/<phase>` (Prerequisite, Job, Rollout,
  Route), mỗi unit một cây Git, một Fleet Bundle và một Helm release.

Hệ quả: pha Prerequisite luôn phải publish dù giá trị không đổi, pod luôn bị tạo
lại, object cũ tích lũy, và có nhiều trạng thái dở dang giữa các pha. Đó là nguồn
gốc độ phức tạp của recovery ở issue #48 và #55.

## Quyết định

1. **Kho secret khóa theo Environment.** Người có quyền với Application (kể cả
   Developer) chọn Secret Store Connection cho Environment một lần, khi
   Environment chưa có kho (người dùng chốt ngày 2026-11-11). Đã chọn thì người
   dùng app không đổi được. Đổi kho chỉ được làm khi Environment không còn
   workload đang chạy, do PE/Admin thực hiện theo
   [runbook đổi kho](../../operations/secret-store-change.md). Quyết định này thay
   phần đổi kho tự phục vụ của ADR-012 và UC-12 BR-16..18, VAR-04.
2. **Giá trị secret đổi qua Deploy, theo version.** Mỗi workload có một path cố
   định trong Vault KV v2 của kho Environment:
   `orchestrator/apps/<app>/envs/<env>/values/workload-<workload>`. Path chứa mọi
   giá trị secret mà workload dùng: secret UC-12 và giá trị dẫn xuất từ output
   private của resource (kể cả giá trị ghép nhiều output, ví dụ connection URL).
   Output gốc của resource vẫn nằm ở kho output của Terraform; workload chỉ đọc
   path của chính nó, nên quyền đọc hẹp hơn đọc thẳng path resource.
   - Deploy so nội dung cần giao với các version gần nhất của path (tối đa 10).
     Trùng một version còn đọc được thì dùng lại version đó, kể cả khi rollback về
     giá trị cũ. Không trùng thì ghi version mới với CAS theo version hiện tại.
   - `VaultStaticSecret` ghim `spec.version`. Tên VSS và Secret đích là
     `<workload>-config-v<N>`.
   - Giá trị không đổi thì manifest secret giữ nguyên, không sinh object mới.
3. **Định danh Vault cố định theo workload.** ServiceAccount, VaultAuth, Vault
   policy và role được đặt tên tất định theo Application, Environment, workload
   và namespace; không theo deployment. Role chỉ đọc path của workload đó.
   `VaultConnection` dùng chung theo kho trong namespace.
4. **Một Environment là một Fleet bundle.** Mọi workload của Environment nằm
   trong unit `app` tại `<root>/<env>/app/`. Một lần Deploy batch publish đúng một
   commit cho `app`. Mỗi unit giữ metadata riêng tại `metadata/<env>/app/`.
5. **Job tách bundle riêng.** Job do template sinh nằm trong unit `migrate` tại
   `<root>/<env>/migrate/`. Unit này chỉ publish khi lần Deploy thay đổi Job.
   Orchestrator publish `migrate`, tự chờ mọi Job `Complete`, rồi mới publish
   `app`. Không dùng Fleet `dependsOn`: với Fleet v0.15.2 và cli-utils v0.37.2,
   kstatus coi Job đã bắt đầu là `Current`, nên bundle Ready trước khi Job xong.
   Tên Job mang hash nội dung (`<tên>-<hash8>`). Job không đổi giữ nguyên tên và
   nội dung nên Fleet đồng bộ lại không tạo Job mới; Job đổi là object mới.
   Vì vậy yêu cầu render lại GitRepo khi Fleet chậm commit được phép cả khi có
   unit `migrate` (khác Job phase theo workload của ADR-030).
   Job thường cần Secret của workload (ví dụ mật khẩu database). Khi Job đã đổi
   tham chiếu một Secret mà bản `app` đã publish chưa khai báo (lần deploy đầu
   hoặc version giá trị mới), Orchestrator publish trước một revision `app` chỉ
   thêm phần giao secret (ServiceAccount, VaultAuth, VaultConnection, VSS) của
   workload thay đổi, chờ các object thêm vào được áp dụng và khỏe, rồi mới tới
   `migrate` và revision `app` cuối. Object đã publish từ trước mà chưa Ready
   (ví dụ workload hỏng đang được sửa trong chính lần Deploy này) không chặn bước
   chuẩn bị. Helm của unit Environment chờ tối đa 240 giây, ngắn hơn deadline
   readiness, để lỗi apply được báo và gán đúng workload trước deadline.
6. **Giữ secret của lần deploy thành công gần nhất.** Bundle `app` gồm VSS mà
   workload đang cần cộng VSS của lần publish `app` thành công gần nhất nếu khác.
   VSS giữ lại mang annotation `orchestrator.io/retained-secret: "true"`. Pod cũ
   vẫn chạy và restart được khi rollout lỗi. VSS chỉ bị bỏ khỏi Git khi không còn
   lần deploy thành công nào dùng; sau khi bản mới Ready, xóa guarded theo UID làm
   Kubernetes GC Secret đích (VSO 1.5.1 đặt ownerReference từ Secret tới VSS).

## Thành phần bundle và hợp nhất

Coordinator lưu kèm mỗi revision của `app`/`migrate` một artifact mã hóa
("composition") chứa manifest mong muốn theo từng workload, ID tất định theo
revision. Deploy batch:

1. Render workload thay đổi như trước; chưa publish từng workload.
2. Lấy composition của revision `app` đã publish gần nhất làm nền, thay workload
   thay đổi, bỏ workload bị xóa, giữ nguyên workload khác. Workload không đổi
   không render lại nên không restart.
3. Thêm VSS giữ lại theo quyết định 6, gộp object dùng chung (VaultConnection)
   theo khóa object; hai object trùng khóa khác nội dung thì từ chối.
4. Publish `migrate` (nếu có Job đổi, sau bước chuẩn bị secret khi cần) và chờ
   `Complete`, rồi publish `app`, chờ Ready. Revision mới Ready thì xóa guarded
   object không còn trong bundle, gồm cả object chỉ do một revision lỗi đã
   publish tạo ra (UID quan sát được ghi cùng bản ghi FAILED). Sau đó
   Orchestrator quét object đang sống mang đúng release Helm của unit và nhãn
   platform của Application/Environment, rồi xóa có điều kiện UID mọi object
   revision hiện tại không còn khai báo, kể cả object của revision lỗi không có
   UID trong bản ghi.

Mọi workload của Environment phải cùng cluster và namespace; khác thì Deploy
từ chối trước khi publish, với lỗi rõ ràng.

## Kết quả, lỗi và rollback

- Current Deployment Set được commit cho mỗi workload trong batch như trước. Bản
  ghi Deployment ở trạng thái DEPLOYING cho tới khi bundle Ready; sau đó
  SUCCEEDED, rồi mới chạy provisioning phụ thuộc workload (`AfterWorkloads`).
- Bundle không Ready: Operation FAILED và Environment được giải phóng (UC-06
  CD-10). Báo cáo đánh dấu FAILED đúng workload có object Degraded hoặc chưa
  Ready, kèm mã lỗi rollout; workload thay đổi khác có object Healthy được báo
  SUCCEEDED. Workload không thay đổi không bị restart nên vẫn chạy bản cũ.
- Git giữ trạng thái đã publish kể cả khi lỗi. Developer sửa cấu hình rồi Deploy
  lại, hoặc Rollback Environment theo ADR-040. Rollback khôi phục cấu hình mong
  muốn của bản thành công và Deploy lại; nhờ dùng lại version trùng nội dung,
  workload quay về đúng version secret của bản đó. Rollback chỉ publish unit
  `app`; unit `migrate` giữ nguyên, không chạy lại migration của bản cũ (thay
  quy tắc chặn rollback khi có Job của ADR-040 cho unit `migrate`).
- Recovery của submission giữ nguyên giao thức receipt, lease và hold của
  ADR-036 cho hai unit mới.

## Phạm vi áp dụng

- Environment chưa có unit delivery nào còn hoạt động theo bố cục cũ dùng mô
  hình mới ngay ở lần Deploy đầu.
- Environment đang có unit cũ giữ đường giao cũ cho tới khi Application bị xóa và
  tạo lại. Không chuyển đổi tại chỗ, không đổi Helm release owner của object đang
  chạy. Lịch sử và cleanup của unit cũ giữ nguyên.
- Xóa Application (UC-18) withdraw hai unit, xóa guarded object, xóa metadata
  mọi version của path workload và Vault policy/role tương ứng.

## Thay thế

- ADR-012: phần đổi kho tự phục vụ và copy giá trị khi đổi kho cho Environment
  có workload. Phần lựa chọn kho, fencing, phiên bản cấu hình giữ nguyên.
- ADR-030 quyết định 8 và ADR-036: tách package theo pha cho từng workload,
  thay bằng hai unit `app`/`migrate` mỗi Environment. Fleet v0.15.2, GitRepo theo
  Application, receipt, lease, UID guard và Terraform ownership giữ nguyên.
- ADR-040: không đổi; rollback dùng lại đường Deploy mới.
- UC-12 BR-10, BR-14: gói bất biến theo revision thay bằng path cố định theo
  workload và version KV v2.

## Hệ quả

- Không cần migration schema PostgreSQL: composition dùng bảng artifact delivery
  sẵn có; unit mới dùng các cột unit/revision/intent hiện có.
- Token backend của kho cần quyền đọc/ghi `values/*` như hiện tại; không cần path
  Vault mới ngoài prefix `values/`.
- Một workload hỏng làm cả bundle Environment không Ready; Orchestrator vẫn xác
  định được workload hỏng từ health theo object.
