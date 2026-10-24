---
id: UC-16-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-24
related: UC-01, UC-05, UC-06, UC-07, UC-12
---

# UC-16 — Quản lý cấu hình workload

## Mục tiêu

Developer thêm, sửa hoặc xóa cấu hình workload trong một Environment của
Application. UC-16 lưu cấu hình mong muốn, không tự preview hoặc deploy.

## Primary actor

- Developer

## Tiền điều kiện

- **PRE-01:** Developer đã đăng nhập và có quyền truy cập Application.
- **PRE-02:** Application và Environment `staging` hoặc `production` đã tồn tại.

## Trigger

**TRG-01:** Developer chọn Add, Edit hoặc Delete workload trên Application home
của Environment đang chọn.

## Main success scenario

1. **MS-01:** Hệ thống hiển thị workload và thay đổi cấu hình đang chờ trong
   Environment đã chọn.
2. **MS-02:** Developer chọn Add hoặc Edit. Hệ thống mở form cấu hình mặc định;
   Developer cũng có thể nạp một file Score.
3. **MS-03:** Developer khai báo container/image, Service/cổng và dependencies.
   Mỗi container có một bảng **Biến môi trường**, không tách checklist và
   “Nguồn khác”. Dependency có thể thêm ngay trong bước chọn nguồn tài nguyên.
4. **MS-04:** Developer thêm dòng, đặt tên biến và chọn một nguồn: Giá trị nhập,
   Biến của ứng dụng, Bí mật, Tài nguyên hoặc Workload khác. Chỉ hiện control
   cần cho nguồn đang chọn; giữ mọi mapping đã có khi Edit/import.
5. **MS-05:** Nếu thiếu key, Developer mở form UC-12 tại chỗ, tạo và chọn key
   trong cùng app/env. Nếu thiếu dependency, thêm vào bản nháp tại chỗ rồi chọn
   output. Không phải rời workload editor cho các thao tác này.
6. **MS-06:** Hệ thống kiểm tra cấu hình và tham chiếu. Nếu nạp Score, hệ thống
   hiển thị nội dung đã đọc để Developer kiểm tra trước khi lưu.
7. **MS-07:** Developer lưu. Hệ thống lưu cấu hình mong muốn cho đúng Environment,
   hiển thị thay đổi đang chờ và cho phép yêu cầu Preview changes theo UC-05.

## Luồng biến thể

- **VAR-01 — Nạp Score:** tại MS-02, Developer chọn file Score mô tả một
  workload. Cấu hình nhập từ file chịu cùng quy tắc với form; nạp file không tự
  lưu hoặc deploy.
- **VAR-02 — Xóa workload:** tại TRG-01, Developer chọn Delete và xác nhận.
  Hệ thống đánh dấu workload chờ xóa trong Environment; Developer có thể hoàn
  tác trước khi deploy. Workload đang chạy chưa bị xóa. Nếu workload mới chỉ
  tồn tại ở draft, chưa từng deploy, Delete hủy pending add thay vì tạo pending
  delete cho workload không tồn tại ở runtime.

## Hậu điều kiện

- **POST-01:** Cấu hình mong muốn của Environment được lưu hoặc workload được
  đánh dấu chờ xóa; thay đổi sẵn sàng để preview.
- **POST-02:** Current Deployment Set, resource và workload đang chạy không đổi.
- **POST-03:** Giá trị secret không xuất hiện trong cấu hình workload hay màn
  hình đọc lại.

## Quy tắc nghiệp vụ

- **BR-01:** Cấu hình workload thuộc đúng một Environment; thay đổi ở `staging`
  không sửa cấu hình `production`, và ngược lại. Tên workload là duy nhất trong
  Environment.
- **BR-02:** Container hỗ trợ giá trị thường nhập trực tiếp và các tham chiếu
  ở MS-04. Giá trị trực tiếp là string thuộc riêng container/workload/environment,
  không tự tạo key UC-12. Nó được lưu trong Score, đọc lại được và không dùng
  để chứa bí mật. Secret phải dùng tham chiếu; không có chế độ secret literal.
- **BR-03:** Tham chiếu UC-12 phải thuộc đúng Application và được cấu hình cho
  Environment đang chọn. Workload không sao chép giá trị variable/secret đó.
- **BR-04:** Resource output chỉ được chọn từ dependency khai báo trong workload
  và phải có trong output contract. Output được phân loại secret hiện nhãn
  “Bí mật” trong cùng bảng; giá trị secret không hiển thị ở form, lỗi hoặc preview.
- **BR-05:** Tham chiếu workload-to-workload luôn đi qua Service, không trỏ
  trực tiếp tới Pod. Workload đích phải thuộc cùng Environment, có Service/cổng
  đã khai báo và không đang chờ xóa. Hệ thống tạo địa chỉ nội bộ từ Service/cổng
  đã chọn; Developer không phải nhập URL trực tiếp.
- **BR-06:** BR-02 chỉ giới hạn giá trị biến môi trường và secret của container,
  không cấm nhập các trường khác như image, port, CPU/memory hoặc tham số resource.
  Quy tắc này áp dụng cho luồng UC-16, không ngầm sửa Score API/CI hiện có.
- **BR-07:** Lưu/sửa/xóa cấu hình không tự provision resource, apply Kubernetes
  manifest hay thay current Deployment Set. UC-05 sở hữu preview; UC-06/UC-07
  sở hữu việc áp dụng thay đổi.
- **BR-08:** Form và file Score chịu cùng validation của UC-16; nạp file không
  được bỏ qua BR-02–BR-05.
- **BR-09:** Form tạo và import nhận tham chiếu Application key qua resource
  `env` loại `environment` với `${resources.env.KEY}`. Key phải tồn tại trong
  Environment đã chọn; Secret phải chiếm toàn bộ một binding, không ghép với
  literal. Phân loại secret lấy từ metadata UC-12/output contract, không do tên biến hay toggle UI.
- **BR-10:** Mỗi lần lưu/sửa/xóa tăng version draft của Environment. Preview
  gắn với version draft và revision UC-12; Deploy từ Preview cũ bị từ chối.
- **BR-11:** Form hiển thị typed Type inputs; required phải hợp lệ trước
  Save, params lưu vào `resources.<alias>.params` và khôi phục khi Edit.
  Save kiểm Type schema; Preview/Deploy kiểm module mappings/constraints biết
  trước provision. Không module match/input lỗi thì chặn Deploy. Import giữ
  cấu trúc phức tạp và shared ID, không mất thông tin khi đổi controls.
- **BR-12:** Mỗi workload có thể khai báo nhiều public route `{path, port}`;
  `path` là URL prefix bắt đầu bằng `/`, `port` là tên Service port đã khai báo.
  Preview kiểm tra path hợp lệ và không trùng trong toàn Environment; `/` và
  `/api` được phép ở hai workload. Chỉ nhận `service.publicRoutes`; mọi field
  `service.publicPort` bị từ chối, kể cả rỗng/null hoặc đi kèm publicRoutes,
  không tự chuyển draft cũ. HTTP trả code ổn định `INVALID_SCORE` và không
  thay đổi draft/version/current set/runtime. Save chỉ lưu desired state; Deploy mới
  tạo/cập nhật hoặc gỡ route của Environment.
- **BR-13:** Draft có nội dung ngữ nghĩa giống current Deployment Set không
  tạo thay đổi trong Preview; thay đổi revision UC-12 của key workload đang
  dùng vẫn là update. Mỗi Save vẫn tăng draft version theo BR-10.

## Ngoài phạm vi

- **OOS-01:** Quản lý giá trị dùng chung thuộc UC-12; UC-16 nhúng thao tác
  tạo key tại chỗ theo BR-21. Sửa/đổi tên/xóa key dùng chung vẫn ở Settings.
- **OOS-02:** Preview, approval hoặc deploy; thuộc UC-05, UC-06 và UC-07.
- **OOS-03:** DNS/TLS và đường mạng từ ngoài cluster tới Ingress Controller;
  Service reference ở BR-05 vẫn là địa chỉ nội bộ cho giao tiếp giữa workload.
- **OOS-04:** Cú pháp tham chiếu Service trong Score và resolution tại Deploy
  được định nghĩa trong shared operation contract.

## Save/import input contract enforcement

- **BR-14:** MS-06 validates every declared non-virtual resource dependency,
  including dependencies with no container binding, against its registered
  Resource Type. Unknown Type, undeclared param, missing required input, null
  or wrong literal type is rejected before draft persistence and before a
  successful Score import response. Form and import share this rule. Virtual
  `environment`/`service` resources follow their existing dedicated contracts.
  Use the same supported input subset as Score planning; do not introduce
  generic JSON Schema or change the direct Score API.
- Errors identify `resources.<alias>.params.<field>` (or the resource Type)
  without including submitted values. Failed validation leaves drafts, draft
  version, current Deployment Set and runtime untouched.

## Bảng biến môi trường thống nhất — DEVEX-ENV (2026-10-24)

Thiết kế được duyệt thay thế checklist và chính sách cấm mọi literal cũ.
[Screens](ui/screens.md) sở hữu bố cục/nhãn; [OC-16](../../architecture/contracts/operation-contracts.md#oc-16-workloadconfigurationservicelist--save--delete--undo)
sở hữu mapping HTTP/Score. Trạng thái code ở CURRENT_STATE, không suy từ thiết kế.

- **BR-15:** Mỗi container có một bảng; **+ Thêm biến** tạo một dòng trống,
  nguồn mặc định Giá trị nhập. Không tự thêm key hoặc variable khác. Năm nguồn
  là Giá trị nhập, Biến của ứng dụng (VARIABLE), Bí mật (SECRET), Tài nguyên,
  Workload khác. Bí mật và output bí mật chỉ hiển thị tên/trạng thái.
- **BR-16:** Tên biến bắt buộc, theo identifier hiện hành và duy nhất trong
  container trên mọi nguồn; không trim/đổi tên/suffix hoặc overwrite âm thầm.
  Tên được lặp ở container khác. Chọn key UC-12 gợi ý tên key chỉ khi ô tên
  còn trống; tên đã nhập được giữ. Đổi nguồn giữ tên và bỏ payload nguồn cũ
  khỏi dòng được serialize; không chuyển giá trị secret sang literal.
- **BR-17:** Xóa dòng chỉ bỏ mapping đó khỏi container. Không xóa key UC-12,
  dependency hay mapping khác. Nhiều dòng được tham chiếu cùng key/output.
  Edit/import giữ tất cả tên, alias, params, class, shared IDs, routes và
  advanced fields. Không biểu diễn lossless thì giữ đường Score nâng cao hiện có.
- **BR-18:** Key/output/dependency/Service không còn hợp lệ vẫn hiện với lựa
  chọn và tên cũ; chặn lưu cho tới khi sửa hoặc bỏ dòng, không tự retarget.
  Loading/error catalog khác empty; cho retry GET và giữ draft. Scope đổi
  reset editor theo đích và bỏ late replies; reload cùng scope giữ edits.

## Giá trị trực tiếp và tạo nguồn tại chỗ

- **BR-20:** Giá trị trực tiếp là string, cho phép chuỗi rỗng và giữ nguyên
  whitespace; từ chối NUL. Ô form không diễn giải placeholder: `${...}` nhập
  trực tiếp phải được escape bằng cơ chế Score hiện hành và round-trip đúng.
  File Score tiếp tục dùng cú pháp Score: full reference được phân loại theo
  nguồn; literal/escaped literal thường được nhận, malformed/unknown reference
  bị từ chối, không fallback thành text. Ghép literal + reference không thuộc
  form cơ bản và vẫn bị từ chối trên UC-16; không mở rộng direct Score/CI API.
  Không tự đoán bí mật theo tên biến. UI nói rõ giá trị thường được lưu cùng
  cấu hình và hướng tới nguồn Bí mật khi cần.
- **BR-21:** Từ nguồn Biến của ứng dụng/Bí mật, **Tạo biến**/**Tạo bí mật** mở
  form UC-12 tại chỗ. Hiện app/env, tên key và giá trị; bí mật nhập che, không
  readback. **Tạo và chọn** tạo key bằng create-only contract UC-12 BR-24–28,
  rồi chọn key cho đúng dòng, giữ tên container đã nhập. Đây là commit UC-12
  riêng; Hủy workload sau đó không xóa key đã tạo. Workload còn chưa lưu phải
  có thông báo rõ, không tuyên bố hai thao tác là một transaction.
- **BR-22:** Nguồn Tài nguyên cho chọn dependency đã khai báo và output từ Type
  contract, hoặc **Thêm tài nguyên** tại chỗ. Form con tái sử dụng alias/type,
  class, shared ID và typed inputs; không cho chọn provider/module/credential.
  **Thêm và chọn** chỉ thêm dependency vào draft và chọn alias ở dòng gốc;
  output vẫn cần chọn tường minh. Hủy form con không sửa draft. Resource mới
  dùng được ở các container khác trong cùng workload; không tự tạo mapping.
  Xóa/đổi alias/type có binding giữ các dòng bị ảnh hưởng ở trạng thái lỗi,
  không tự xóa binding hoặc đổi output. Class/params đổi mà output còn hợp lệ
  giữ binding và chờ Preview xác minh matching.
- **BR-23:** Form và Score import cùng hỗ trợ BR-20; Save/import/Preview không
  resolve hoặc copy secret. Edit sau Deploy, restart và UC-12 rename phải giữ
  giá trị thường mới cùng các references. Không biến literal lịch sử chưa có
  bằng chứng là giá trị thường thành dữ liệu có thể đọc. Backend chỉ cho đọc
  lại literal đã được UC-16 mới chấp nhận tường minh, với provenance gắn đúng
  workload/container/name/value và version; provenance không có hoặc không
  khớp thì giữ trạng thái không thể mở an toàn, không trả value. Không xóa/reset
  dữ liệu cũ để đáp ứng tính năng. Contract chi tiết ở OC-16 DEVEX-ENV.
- **BR-24:** Secret Store chưa chọn: ngay form Tạo bí mật có selector READY
  stores và **Lưu kho bí mật** theo UC-01 hiện hành. Không tự chọn/lưu; giữ
  workload draft, không yêu cầu chuyển trang. Chỉ cho initial selection khi
  chưa có kho; thay kho đã có vẫn qua Settings/copy flow UC-12. Chọn kho là
  commit riêng; nếu tạo secret tiếp theo lỗi, kho đã lưu vẫn được giữ và báo rõ.
- **BR-25:** Binding Workload khác chọn workload khác trong cùng env, Service
  và port hợp lệ, không tự tham chiếu hay chọn workload chờ xóa. Chỉ gắn vào
  dòng đang sửa. Lưu chỉ tạo desired draft; explicit Preview → Deploy giữ nguyên.

## ADR-010 renderer-only pending updates

BR-13 no-op suppression also requires unchanged rendering intent. Compare the
current selected Definition/bundle with the workload's last deployment plan.
A selection change is an update even without a saved Score draft; Preview pins
it through the plan hash and Deploy rejects an obsolete token before execution.
Configuration revision handling and resource/update/remove identities remain
as specified above.

## Desired references sau đổi tên key — T30

- **BR-19:** UC-12 BR-20–23 có thể tạo/sửa nhiều desired drafts nguyên tử khi
  Developer opt-in rename. Giữ mọi container aliases, many-to-one imported
  mappings, resource params/outputs, Service refs, routes và advanced fields.
  Deployed workload không có draft phải reconstruct lossless; không sửa current
  Deployment Set hoặc pending delete. Không bảo toàn được thì fail toàn bộ.
- Editor đọc draft đã rewrite và vẫn dùng explicit Save, Preview và Deploy;
  editor đang mở với draft version cũ bị stale, không last-write-wins.


## Dependency class

- **V2-02:** Redis/PostgreSQL có alias/type/params và class từ metadata:
  `in-cluster` cho k8s-4f/EKS, `cloud-managed` cho AWS. Developer không chọn
  Terraform module/provider credential; class sai target hoặc no match báo
  lỗi, không tự đổi cách cung cấp.
- **V2-03:** Shared dependency giữ shared ID hiện hành. Loading/error metadata
  không thành choices rỗng; class imported/custom được giữ để review, không
  rewrite thành default. Các alias, bindings/Secret/Service và advanced fields
  phải round-trip không mất thông tin.
