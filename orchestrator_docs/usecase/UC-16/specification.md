---
id: UC-16-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-02
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
3. **MS-03:** Developer khai báo container, image, Service/cổng, tài nguyên và
   resource dependencies cần dùng.
4. **MS-04:** Với mỗi container, Developer tick các Application variables đã
   cấu hình trong UC-12 cho Environment này. Tên biến trong container mặc định
   bằng tên key; chỉ nhập tên khác khi cần. Output resource không bí mật và
   Service/cổng của workload khác được thêm ở mục nguồn khác riêng.
5. **MS-05:** Developer tick Application secrets từ danh sách key cùng
   Environment; không nhập lại tên hoặc giá trị. Có thể chọn tên khác trong
   container, hoặc thêm output bí mật của resource dependency ở mục nguồn khác.
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
- **BR-02:** Biến môi trường và secret của container chỉ được lưu dưới dạng
  tham chiếu tới các nguồn ở MS-04/MS-05. Form không cho nhập giá trị trực tiếp;
  file Score nạp qua UC-16 chứa literal ở các binding này bị từ chối với lỗi chỉ
  rõ trường vi phạm.
- **BR-03:** Tham chiếu UC-12 phải thuộc đúng Application và được cấu hình cho
  Environment đang chọn. Workload không sao chép giá trị variable/secret đó.
- **BR-04:** Resource output chỉ được chọn từ dependency khai báo trong workload
  và phải có trong output contract. Output được phân loại secret chỉ được chọn
  trong mục Secrets; giá trị secret không hiển thị ở form, lỗi hoặc preview.
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
  Environment đã chọn; Secret chỉ được dùng làm toàn bộ một binding Secret,
  không được ghép với literal hoặc đưa vào mục Variable.
- **BR-10:** Mỗi lần lưu/sửa/xóa tăng version draft của Environment. Preview
  gắn với version draft và revision UC-12; Deploy từ Preview cũ bị từ chối.
- **BR-11:** Với mỗi resource dependency trên form, hiển thị các input của
  Resource Type; input bắt buộc phải có giá trị đúng kiểu trước khi lưu.
  Tham số được lưu vào `resources.<alias>.params` của Score và được khôi phục
  khi Edit. Import Score vẫn cho phép cấu trúc phức tạp mà form không hỗ trợ.
- **BR-12:** Mỗi workload có thể khai báo nhiều public route `{path, port}`;
  `path` là URL prefix bắt đầu bằng `/`, `port` là tên Service port đã khai báo.
  Preview kiểm tra path hợp lệ và không trùng trong toàn Environment; `/` và
  `/api` được phép ở hai workload. `service.publicPort` cũ tương đương route
  `{path: "/", port: publicPort}`. Save chỉ lưu desired state; Deploy mới
  tạo/cập nhật hoặc gỡ route của Environment.
- **BR-13:** Draft có nội dung ngữ nghĩa giống current Deployment Set không
  tạo thay đổi trong Preview; thay đổi revision UC-12 của key workload đang
  dùng vẫn là update. Mỗi Save vẫn tăng draft version theo BR-10.

## Ngoài phạm vi

- **OOS-01:** Quản lý variable/secret của Application; thuộc UC-12.
- **OOS-02:** Preview, approval hoặc deploy; thuộc UC-05, UC-06 và UC-07.
- **OOS-03:** DNS/TLS và đường mạng từ ngoài cluster tới Ingress Controller;
  Service reference ở BR-05 vẫn là địa chỉ nội bộ cho giao tiếp giữa workload.
- **OOS-04:** Cú pháp tham chiếu Service trong Score và resolution tại Deploy
  được định nghĩa trong shared operation contract.

## Trạng thái implementation hiện tại

Đã có UI/API lưu draft, đánh dấu xóa/hoàn tác, import Score và validation
tham chiếu Application key, resource output, Service/cổng cùng Environment.
Save và import kiểm tra toàn bộ non-virtual resource params theo input contract
trước khi ghi draft; local code/API/UI tests đã pass ngày 2026-10-02.
Planner/executor resolve tham chiếu `environment` và `service`; Application home
có Preview → Deploy của các draft. Workload đã deploy không có draft được tái
tạo thành Score tham chiếu để sửa trên form. Kind đã kiểm chứng create,
configuration-only redeploy và remove; broader cloud path chưa kiểm chứng lại.

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

## Select existing Application keys

- **BR-15:** Each container presents existing UC-12 keys for the selected
  Application/Environment as separate Variables and Secrets checklists. A new
  container starts with no keys selected. Checking a key creates a reference
  using the key name as the container variable name. Only selected keys are
  used; new Settings keys are never automatically selected. Values are neither
  re-entered nor copied. Secret values are never displayed in this picker.
- **BR-16:** Each selected key has an optional "Use a different container name"
  control. For example `DATABASE_PASSWORD` can map to `PGPASSWORD`. Turning
  off the override restores the key name. Container names must be non-empty
  and unique among all bindings in that container, including resource and
  Service sources. A collision or incomplete selected binding blocks form
  Save with an actionable error; no silent overwrite, suffix or normalization.
  Names may repeat in different containers.
- **BR-17:** Unchecking a key removes its Application-key bindings only in
  that container. Other keys, containers and resource/Service bindings remain.
  Edit and supported Score import restore the selected keys and original
  container names. If an imported/existing Score maps one key to multiple
  container names, preserve all mappings and expose them for review/edit;
  do not collapse them to a single default name. Unchecking removes those
  mappings for that container. Complex Score import keeps its existing
  lossless advanced path.
- **BR-18:** A selected key that no longer exists remains visible as an
  unavailable reference with its original container name. The user can remove
  it or correct it; it must not disappear silently or be replaced with another
  key. Missing catalog/loading/error states are distinct from an empty key
  list, and form Save is blocked until selected key references can be checked.
  Changing Application, Environment or workload starts the destination editor
  from its own draft/empty state; selections, aliases and imported Score from
  the previous scope must not carry over. Late responses from the previous
  scope are ignored. Same-scope stale reload continues preserving local edits.
  Resource-output and Service bindings remain explicit under a separate
  "Other sources" section, without Application-variable/secret source rows.
- Checklist actions only edit desired form state. Persistence continues using
  `containers.<container>.variables.<name>: ${resources.env.KEY}` with the
  virtual `resources.env` resource. No API, database schema, provider permission
  or Preview/Deploy lifecycle change is introduced.

## ADR-010 renderer-only pending updates

BR-13 no-op suppression also requires unchanged rendering intent. Compare the
current selected Definition/bundle with the workload's last deployment plan.
A selection change is an update even without a saved Score draft; Preview pins
it through the plan hash and Deploy rejects an obsolete token before execution.
Configuration revision handling and resource/update/remove identities remain
as specified above.
