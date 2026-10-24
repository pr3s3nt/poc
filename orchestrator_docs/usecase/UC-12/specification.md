---
id: UC-12-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-24
related: UC-01, UC-05, UC-06, UC-07, UC-16
---

# UC-12 — Quản lý Variables & Secrets

## Mục tiêu

Developer cấu hình variable và secret để các workload trong cùng Application,
cùng Environment có thể tham chiếu và tái sử dụng.

## Primary actor

- Developer

## Tiền điều kiện

- **PRE-01:** Developer đã đăng nhập và có quyền truy cập Application.
- **PRE-02:** Application và hai Environment `staging`, `production` đã tồn tại.

## Trigger

**TRG-01:** Developer mở Application Settings → Variables & Secrets hoặc
form tạo key tại chỗ từ bảng Biến môi trường của UC-16.

## Main success scenario

1. **MS-01:** Hệ thống hiển thị hai tab `Staging` và `Production`. Trong tab
   Environment đang chọn, hệ thống hiển thị cả hai mục `Environment variables`
   và `Secrets` trên cùng một trang.
2. **MS-02:** Developer thêm key mới hoặc chọn key hiện có để cập nhật giá trị.
3. **MS-03:** Hệ thống kiểm tra tên key trong mục và Environment đã chọn, rồi
   lưu thay đổi cấu hình mong muốn.
4. **MS-04:** Hệ thống hiển thị Environment và các workload tham chiếu key bị
   ảnh hưởng; đánh dấu thay đổi chờ Preview → Deploy.

## Luồng biến thể

- **VAR-01 — Đổi tên key:** Developer nhập tên mới. Nếu key đang được workload
  tham chiếu, hệ thống cảnh báo và liệt kê workload bị ảnh hưởng. Developer có
  thể chọn explicit “Đổi tên và cập nhật tham chiếu” (mặc định bỏ chọn). Nếu
  không chọn, chỉ đổi tên key; Developer tự sửa từng workload trong UC-16. Nếu
  chọn, đổi tên và sửa desired references của các workload trong cùng app/env
  bằng một commit nguyên tử theo BR-20; runtime/current set giữ nguyên.
- **VAR-02 — Xóa key:** Developer yêu cầu xóa. Nếu key đang được workload tham
  chiếu, hệ thống cảnh báo và liệt kê workload bị ảnh hưởng nhưng vẫn cho xác
  nhận. Hệ thống không tự xóa/sửa binding trong workload.
- **VAR-03 — Thay secret:** Developer nhập giá trị mới cho secret. Hệ thống
  không hiển thị lại giá trị cũ hoặc mới sau khi lưu.

## Hậu điều kiện

- **POST-01:** Cấu hình mong muốn của đúng Application, loại và Environment đã
  chọn được cập nhật; workload đang chạy và current Deployment Set không đổi.
- **POST-02:** Thay đổi chờ Preview → Deploy. Nếu đổi tên/xóa làm tham chiếu
  workload không hợp lệ, Preview báo lỗi và không cho Deploy cho tới khi Developer
  chỉnh workload.
- **POST-03:** Giá trị secret không xuất hiện trong danh sách, response đọc,
  thông báo, Preview hoặc cấu hình workload.

## Quy tắc nghiệp vụ

- **BR-01:** Mỗi key thuộc một Application, một Environment và một loại
  `Variable` hoặc `Secret`. Key có thể chỉ tồn tại ở Staging; cùng tên ở
  Production là cấu hình độc lập. Thao tác ở một Environment không sửa key ở
  Environment kia.
- **BR-02:** Tên key là duy nhất trong cùng Application và Environment, kể cả
  giữa Variable và Secret. Thêm hoặc đổi tên trùng bị từ chối.
- **BR-03:** Variable có thể xem/chỉnh giá trị. Secret chỉ cho nhập giá trị mới;
  hệ thống không hiển thị lại giá trị đã lưu, kể cả ở Preview hoặc lỗi.
- **BR-04:** Khi đổi giá trị, hệ thống chỉ tạo thay đổi cấu hình mong muốn và
  thông báo các workload đang tham chiếu. Runtime không đổi cho tới khi thay đổi
  được Preview và Deploy ở đúng Environment.
- **BR-05:** Đổi tên/xóa key đang được tham chiếu phải cảnh báo nhưng không bị
  chặn. Xóa và đổi tên không opt-in giữ nguyên tham chiếu trong workload. Chỉ
  đổi tên với lựa chọn tường minh VAR-01 mới cập nhật desired references. Preview
  phải chỉ rõ tham chiếu thiếu và chặn Deploy cho tới khi workload được chỉnh hợp lệ.
- **BR-06:** UC-16 chỉ được chọn key tồn tại trong đúng Application,
  Environment và loại. Giá trị UC-12 không được sao chép thành literal vào
  cấu hình workload.
- **BR-07:** Resource outputs và Service của workload khác không phải key do
  UC-12 quản lý; UC-16 cho chọn trực tiếp các nguồn đó.
- **BR-08:** Mỗi Environment chọn Secret Store Connection riêng qua Settings;
  hỗ trợ Vault KV v2 trước. Variable lưu trong Orchestrator; Secret lưu ở kho
  được chọn với refs bất biến mang store identity. Không kế thừa provider cấp Application.
  Variable chỉ chứa Value trong metadata, không StoreKey/ValueRef; CRUD không gọi
  Vault. Không đọc/materialize Variable refs cũ; format cũ bị từ chối và cần reset dev.
  DTO cấu hình không có Legacy field.
- **BR-09:** Lưu thay đổi tạo revision cấu hình mong muốn nhưng không thay
  revision đã áp dụng. Workload đang chạy và Pod được tạo trước Deploy phải tiếp
  tục dùng revision đã áp dụng. Preview xác định workload bị ảnh hưởng; chỉ
  Deploy hợp lệ mới chuyển revision áp dụng và khởi động lại các workload đó.
- **BR-10:** Với provider Vault trên Kubernetes, VSO đồng bộ đúng revision được
  Deploy thành Kubernetes Secret trong namespace của workload. Container nhận
  key đã tham chiếu qua `env.valueFrom.secretKeyRef`; image không phải đọc file
  hoặc thay đổi startup command. Giá trị không được ghi vào Score, Deployment
  Set, GitOps repo, Pod spec/annotations, log hay response đọc. Tiến trình đang
  chạy không tự nhận giá trị mới khi Secret thay đổi.
- **BR-11:** UC-16 dùng resource `env` loại `environment` trong Score; một key
  được tham chiếu bằng `${resources.env.KEY}`. Hệ thống xác định key là Variable
  hay Secret từ cấu hình UC-12 của đúng Application/Environment, không từ tên
  placeholder. Secret phải chiếm toàn bộ binding, không ghép với literal.
- **BR-12:** Preview gắn với đúng revision cấu hình và draft workload đã đọc.
  Nếu một trong hai thay đổi sau Preview, Deploy từ Preview cũ bị từ chối và
  Developer phải Preview lại.
- **BR-13:** Nếu Deploy nhiều workload bị lỗi giữa chừng, hệ thống báo trạng
  thái áp dụng một phần và tên workload thành công/thất bại. Không đánh dấu
  Environment đã áp dụng toàn bộ revision; workload thành công giữ revision đã
  áp dụng của chính nó để lần Deploy sau tiếp tục an toàn.
- **BR-14:** VSO chỉ đồng bộ các key mà workload tham chiếu từ bundle bất biến
  của Application/Environment/workload/revision. Mỗi bundle có Secret đích riêng;
  quyền đọc Vault chỉ cho bundle đó. Deploy phải xác nhận Secret của revision đã
  đồng bộ trước khi apply workload và chỉ báo thành công sau readiness. Đổi giá
  trị tạo revision chờ Preview → Deploy, không tự rollout workload.

## Đổi tên với cập nhật tham chiếu — CFG-01 / T30

- **BR-20:** Đổi tên opt-in tạo cùng lúc immutable configuration revision và
  toàn bộ desired workload drafts bị ảnh hưởng trong một transaction. Kiểm tra
  expected configuration, Environment và draft versions cùng trạng thái BUSY;
  mọi conflict/failure giữ nguyên key, revision, drafts và versions.
- **BR-21:** Chỉ đổi token KEY của reference tới virtual resource loại
  `environment` trong đúng Application/Environment. Giữ container names/aliases,
  imported many-to-one mappings và cấu trúc Score khác, kể cả resource/Service
  references. Không thay tên container variable theo tên key mới, không biến
  reference thành literal và không đọc/copy Secret bytes; giữ owning StoreKey/ValueRef.
- **BR-22:** Desired upsert draft có ưu tiên hơn current workload; pending delete
  giữ nguyên và không hồi sinh. Workload deployed chưa có draft được reconstruct
  lossless thành reference-based Score trước khi rewrite. Không reconstruct an
  toàn hoặc collision thì từ chối toàn bộ thao tác, không partial rename.
- **BR-23:** Lưu chỉ tạo pending change; current Deployment Set, applied revisions,
  runtime và Environment khác không đổi. Preview liệt kê workloads bị ảnh hưởng
  và pin cả configuration/draft versions; Developer Deploy tường minh. Preview
  tạo trước rename không còn hợp lệ.

## Ngoài phạm vi

- **OOS-01:** Form cấu hình workload và sửa tham chiếu thủ công; thuộc UC-16.
  Đổi tên với cập nhật desired references tường minh theo BR-20–23 thuộc UC-12.
- **OOS-02:** Preview và Deploy; thuộc UC-05, UC-06 và UC-07.
- **OOS-03:** Provider ngoài Vault KV v2 và tự động rollback sau Deploy thất bại;
  chuyển giữa các Vault stores nằm trong phạm vi theo BR-15..19.

## Trạng thái implementation hiện tại

Đã có API/UI quản lý Application variables/secrets theo Environment, desired
revision, Vault KV v2 adapter, resolution tham chiếu khi Preview/Deploy và
applied revision theo workload. VSO/Secret delivery theo ADR-008 đã được kiểm
chứng trên kind; Agent Injector vẫn là đường tương thích cũ. Fake mode dùng
provider bộ nhớ. HA, backup và secret lifecycle production chưa thuộc MVP.

## Workload reuse

UC-16 MS-04/MS-05 và BR-15–24 sở hữu bảng chọn nguồn tại container. UC-12
sở hữu key/value/revision, kể cả khi form tạo key được nhúng trong UC-16.
Chọn key có sẵn không tạo bản sao giá trị; tạo tại chỗ là thao tác riêng tường minh.

## Environment secret-store transitions

- **BR-15:** Chưa chọn kho thì variable operations vẫn hoạt động; Secret write
  trả 422 secretStoreKey và dẫn tới Settings. Không backfill store identity hoặc dùng legacy Vault flags; runtime chỉ resolve
  private credential ref của kho đúng Organization.
- **BR-16:** Đổi kho dùng expected Environment/config versions và operation claim.
  Đọc refs từ đúng owning store, copy immutable desired Secret values sang kho
  mới, verify rồi commit selection và revision cùng transaction. Không copy
  Variable vào kho secret và không thay các revisions đã áp dụng.
- **BR-17:** Copy failure giữ selection/revision cũ; cleanup chỉ object của attempt.
  Old values/bundles giữ nguyên để Pods/history còn dùng; không xóa kho referenced.
- **BR-18:** Store identity/delivery change làm mọi workload dùng Secret bị pending
  kể cả giá trị không đổi; Preview pin store identity, desired revision và versions.
- **BR-19:** Active deploy/migration/store-copy chặn concurrent Settings/config/draft
  writes bằng safe 409. Không có secret plaintext trong DB/plan/API/log/recording.

**VAR-04 — Change store:** Developer chọn store READY khác trong Settings;
hiển thị impact; copy/verify/commit, báo progress hoặc safe recovery failure.
Quy tắc shared: [ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md).


## Tạo key tại workload editor — DEVEX-ENV (2026-10-24)

- **BR-24:** UC-16 được nhúng form tạo Variable/Secret cho đúng app/env đang
  sửa. Quyền, naming, uniqueness chung hai loại, value validation và Secret
  Store semantics giống Settings. Không tạo key cho Environment khác.
- **BR-25:** **Tạo và chọn** là create-only, không được dùng upsert để ghi đè
  key cùng tên/cùng loại. Check version, no-active-operation và key chưa tồn
  tại trước commit; concurrent create chỉ có một winner. Khi trùng, cho chọn
  key có sẵn sau GET/review tường minh nếu đúng loại, hoặc đổi tên. Không tự
  thay giá trị, không tự đổi loại, không tự gửi lại mutation với version mới.
- **BR-26:** Success trả safe metadata của configuration mới, không secret
  value; editor chọn key cho đúng dòng. Key commit độc lập với Save workload.
  Hủy/xóa dòng sau success giữ key trong Settings và không tự rollback key
  hoặc Secret Store đã lưu. Failed workload Save giữ key đã tạo và draft.
- **BR-27:** Secret chỉ tồn tại trong input che và request tạo; không đưa vào
  Score, bản nháp workload, URL, local/session storage, log hoặc evidence.
  Xóa input khi success, cancel, đổi scope hoặc đóng form; lỗi còn form giữ
  input che cho người dùng sửa/retry tường minh. Nếu kết quả mutation chưa rõ,
  không replay: GET/review metadata, cho **Dùng key hiện có** nếu phù hợp hoặc
  tạo lại nếu xác minh key chưa tồn tại; không khẳng định giá trị đã lưu đúng
  chỉ vì thấy cùng tên key. Không đọc secret để đối chiếu.
- **BR-28:** Variable tạo tại chỗ lưu như UC-12 hiện hành, khác với Giá trị nhập
  riêng container ở UC-16 BR-20. Khi chưa có Secret Store, form UC-16 hỗ trợ
  initial selection theo UC-16 BR-24; không khởi chạy copy/migrate tự động.
