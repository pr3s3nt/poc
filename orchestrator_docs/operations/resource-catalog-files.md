---
id: OPS-RESOURCE-CATALOG-FILES
artifact: operations-runbook
status: current
last_reviewed: 2026-11-12
related: [UC-02-SPEC, UC-03-SPEC]
---

# Viết và nhập file Resource Type / Resource Definition

Hướng dẫn dành cho **Kỹ sư nền tảng** và **Quản trị viên**, dùng hai tab
**Biểu mẫu** và **Nhập YAML/JSON** trên Console.

- **Resource Type — Loại tài nguyên:** khai báo input mà workload được truyền
  và output mà tài nguyên cung cấp; ví dụ `postgres` có input `database` và
  output `host`, `password`.
- **Resource Definition — Cấu hình tài nguyên:** chọn driver, module/template,
  tham số và điều kiện để cung cấp một Type; ví dụ PostgreSQL trong Kubernetes
  cho môi trường `staging`.

Đăng ký Type trước rồi mới đăng ký Definition của Type đó. Đăng ký chỉ lưu
cấu hình; tài nguyên được tạo khi ứng dụng thực hiện **Preview → Deploy**.

## 1. Nhập file trên Console

### Loại tài nguyên

1. Mở **Loại tài nguyên** (`/ui/platform/resource-types`).
2. Dưới danh sách đã đăng ký, chọn tab **Nhập YAML/JSON** cạnh **Biểu mẫu**.
3. Bấm **Chọn file**, hoặc dán trực tiếp vào **Nội dung YAML/JSON**.
4. Xem và sửa nội dung trong ô soạn thảo, rồi bấm **Đăng ký**.

### Cấu hình tài nguyên

1. Mở **Cấu hình tài nguyên** (`/ui/platform/resource-definitions`).
2. Bấm **Tạo cấu hình**.
3. Chọn tab **Nhập YAML/JSON** cạnh **Biểu mẫu**.
4. Upload hoặc dán nội dung, sửa nếu cần, rồi bấm **Đăng ký**.

Upload và dán dùng cùng cách kiểm tra dữ liệu. Chuyển tab giữ hai bản nháp
riêng biệt: nội dung YAML/JSON không tự điền sang các trường của Biểu mẫu và
ngược lại. Tab nhập dùng để đăng ký; muốn sửa Definition đã lưu, mở **Sửa**
trong danh sách.

### Kết quả đăng ký

| Kết quả | Ý nghĩa | Việc tiếp theo |
|---|---|---|
| `CREATED` | Key chưa tồn tại trong Organization và cấu hình đã được lưu | Kiểm tra lại trong danh sách |
| `REUSED` | Key đã tồn tại với cấu hình tương đương | Dùng cấu hình hiện có; không có revision mới |
| `CONFLICT` | Key đã tồn tại nhưng cấu hình khác | Xem các field khác biệt, sửa file hoặc chọn key mới nếu muốn tạo Definition khác |
| Lỗi | Sai cú pháp, schema, pin, account hoặc tham chiếu | Sửa ngay trong ô soạn thảo rồi đăng ký lại |

`REUSED` và `CONFLICT` không ghi đè cấu hình hoặc tạo revision. Đổi thứ tự key,
trình bày YAML/JSON hoặc dùng default tương đương không phải là một thay đổi
cấu hình. Nếu cần thay đổi Definition hiện có, dùng **Sửa**, không dùng import
để cập nhật key trùng. Resource Type không có chức năng sửa toàn bộ schema.

## 2. Quy cách chung của file

- Phần mở rộng `.yaml`, `.yml` hoặc `.json`, mã hóa **UTF-8**, tối đa **1 MiB**.
- Mỗi lần nhập đúng **một đối tượng**. Tạo hai file riêng cho Type và Definition.
- Nội dung là body đăng ký trực tiếp như các mẫu dưới đây: không có wrapper
  `apiVersion`, `kind`, `metadata`, `resourceTypes` hay `definitions`.
- Không dùng YAML nhiều document (`---` để ghép các đối tượng), key trùng,
  key dạng object/list, số `.nan`/`.inf` hoặc các giá trị JSON không biểu diễn được.
- Giữ đúng tên trường và kiểu dữ liệu: `true` khác `"true"`, `5432` khác `"5432"`.
  YAML có thể dùng comment; JSON không có comment.
- Không thêm `version`, `revisionId`, `sourceFingerprint`, `renderPin`,
  `executionProfile`, `connectionKey` hoặc trường ngoài contract.
- Credential được cấu hình trong **Tài khoản hạ tầng / Cấu hình Git / Kho bí mật**,
  không điền token, kubeconfig, mật khẩu hoặc private key vào file Definition.

Các phần `<...>` trong mẫu là chỗ cần thay bằng dữ liệu trên Console đích.
Mẫu mô tả cấu trúc, không bảo đảm một module hoặc source cụ thể đã có trên
installation của bạn. Không thay các biểu thức `${resources[...]...}` thành
`<...>`: đó là tham chiếu mà platform sẽ xử lý khi lập kế hoạch và triển khai.

## 3. Viết Resource Type

### Các trường

| Trường | Cách viết |
|---|---|
| `key` | ID duy nhất trong Organization, gồm chữ thường ASCII, số và dấu `-` |
| `inputs` | Danh sách input mà workload được phép truyền qua `params`; dùng `[]` nếu không có |
| `outputs` | Danh sách output mà tài nguyên có thể cung cấp; dùng `[]` nếu không có |
| `inputs[].name`, `outputs[].name` | Tên không rỗng và không trùng trong cùng danh sách |
| `type` | Ghi rõ `string`, `number`, `bool` hoặc `any` |
| `required` | Boolean; input required phải được workload truyền, output required phải được Definition cung cấp |
| `outputs[].secret` | Boolean; `true` đánh dấu output bí mật, không phải nơi ghi giá trị bí mật |

Input chỉ gồm `name`, `type`, `required`. Output có thêm `secret`. Resource Type
không có thuộc tính `default`; fallback thuộc Definition hoặc module.
Không dùng các Type ID ảo được platform dành riêng như `environment`, `service`.

### Mẫu YAML: `postgres-type.yaml`

```yaml
key: postgres
inputs:
  - name: database
    type: string
    required: true
  - name: username
    type: string
    required: false
outputs:
  - {name: host, type: string, required: true, secret: false}
  - {name: port, type: number, required: true, secret: false}
  - {name: database, type: string, required: true, secret: false}
  - {name: username, type: string, required: true, secret: false}
  - {name: password, type: string, required: true, secret: true}
```

Mẫu này dành cho việc đăng ký Type mới. Nếu `postgres` đã tồn tại, hãy xem schema
hoặc tải Type hiện có: input required và danh sách output có thể khác mẫu.
Không sửa Type đang dùng để làm cho mẫu nhập được. Cũng không chỉ đổi `key`
thành một Type mới rồi dùng module PostgreSQL cũ: module phải công bố hỗ trợ
Type tương ứng. Một Type được đăng ký thành công chưa có nghĩa là có module
để provision Type đó.

### JSON tương đương: `postgres-type.json`

```json
{
  "key": "postgres",
  "inputs": [
    {"name": "database", "type": "string", "required": true},
    {"name": "username", "type": "string", "required": false}
  ],
  "outputs": [
    {"name": "host", "type": "string", "required": true, "secret": false},
    {"name": "port", "type": "number", "required": true, "secret": false},
    {"name": "database", "type": "string", "required": true, "secret": false},
    {"name": "username", "type": "string", "required": true, "secret": false},
    {"name": "password", "type": "string", "required": true, "secret": true}
  ]
}
```

## 4. Viết Resource Definition

### Chuẩn bị trước khi viết

1. Xác nhận Resource Type đã có, xem đúng input/output và cờ required/secret.
2. Với Terraform hoặc template, chuẩn bị nguồn bằng luồng Git trên Console,
   inspect/xác minh nguồn rồi lấy **pin đã lưu** hoặc **template source ID**.
   Có thể thực hiện trong **Tạo cấu hình → Biểu mẫu**: chọn Type, chọn nguồn Git,
   xác minh rồi mở phần chi tiết module/nguồn hoặc phần xem lại JSON. Chép pin/ID
   từ đó trước khi chuyển sang tab nhập; không tự suy chúng từ URL/ref đã gõ.
3. Chuẩn bị Driver Account nếu driver cần; account phải thuộc Organization hiện
   tại, đúng loại và ở trạng thái sẵn sàng.
4. Xác định class, môi trường và các resource phụ thuộc, chẳng hạn cluster và
   namespace. Các Definition liên quan phải match được ngữ cảnh đó.

Import không tự inspect Git, tạo account hoặc chuẩn bị source. Cách ít sai nhất
khi đã có cấu hình gần giống là **Tải file** từ Definition đó rồi sửa phần cần
thay đổi. Pin/source ID trên installation khác không tự chuyển đổi khi import.

### Trường chung

| Trường | Cách viết |
|---|---|
| `key` | ID duy nhất trong Organization, chữ thường ASCII/số/`-`; không dùng key hệ thống |
| `name` | Tên hiển thị không rỗng, tối đa 253 ký tự, không chứa control characters |
| `resourceType` | `key` của Type đã đăng ký |
| `driverType` | Driver được hỗ trợ cho Type/module: các ví dụ dưới dùng `terraform`, `template`, `existing-cluster`, `endpoint` |
| `driverAccount` | Key account khi driver yêu cầu; không phải tên hiển thị |
| `driverInputs` | Cấu hình đúng schema của driver; các mẫu dùng `values` |
| `criteria` | Danh sách điều kiện chọn Definition; không được là `[]` |
| `provision` | Rules cho resource đi kèm; dùng `{}` nếu không cần |

`key` của Definition không phải `res_id` của resource. Driver Account và region
không phải điều kiện matching; không dùng chúng để thay cho `criteria`.

### Terraform trên Kubernetes: `staging-postgres.yaml`

Mẫu dưới dùng schema Type ở mục 3. Tên module variables/outputs và pin phải khớp
module thật; nếu Type hiện có thêm output required, phải bổ sung mapping cho chúng.

```yaml
key: staging-postgres
name: Staging PostgreSQL
resourceType: postgres
driverType: terraform
driverInputs:
  values:
    source:
      source_id: "<source ID đã đăng ký>"
      module_id: "<module ID PostgreSQL đã inspect>"
      revision: "<revision chính xác đã inspect>"
      subdirectory: "<subdirectory chính xác đã inspect>"
      content_sha256: "<SHA256 đầy đủ của module>"
    variables:
      namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
      username: app
    input_mapping:
      database: {param: database}
      username: {param: username}
    output_mapping:
      host: host
      port: port
      database: database
      username: username
      password: password
    provider_bindings:
      kubernetes:
        target: resource
        resource_ref: "k8s-cluster.bootstrap-dev#environments.@app.@env"
criteria:
  - env_type: staging
    class: in-cluster
provision: {}
```

#### Pin module: sao chép đủ năm trường

| Trường trong file | Giá trị cần lấy |
|---|---|
| `source_id` | `sourceId` của nguồn đã đăng ký |
| `module_id` | `moduleId` của module đã inspect |
| `revision` | Revision bất biến đã xác minh, không tự đổi thành branch/tag mới |
| `subdirectory` | Đường dẫn module trong source; sao chép nguyên giá trị đã inspect |
| `content_sha256` | `contentSha256` đầy đủ, 64 ký tự hex, không phải SHA rút gọn |

Giữ cả bộ pin cùng một kết quả inspect. Không điền Git URL hoặc đường dẫn máy
cá nhân thay cho `source_id`; không tự tính digest từ một file module riêng lẻ.
Nếu thay module/revision, chuẩn bị nguồn mới rồi lấy bộ pin mới.

#### Variables và mapping

- `variables` chứa giá trị nền tảng muốn đặt, fallback hoặc biểu thức tham chiếu
  được hỗ trợ. Giữ kiểu dữ liệu theo module; không đặt trường executor-owned
  hoặc literal secret vào đây.
- `input_mapping`: **tên module variable → `{param: tên input của Type}`**.
  Ví dụ nếu module đặt tên biến là `db_name`, viết
  `db_name: {param: database}`. Mọi input của Type phải có mapping phù hợp;
  không tự mở thêm module variables cho developer qua `params`.
- `output_mapping`: **tên module output → tên output của Type**. Nếu module
  xuất `endpoint` còn Type gọi là `host`, viết `endpoint: host`.
  Phải phủ output required và không hạ output secret thành public.

Với input có mapping, thứ tự lấy giá trị là:
**workload param → Definition fallback → module default**.

| Input trên Type | Workload không truyền param | Workload truyền param |
|---|---|---|
| `required: false` | Dùng fallback/default nếu có; vẫn phải đáp ứng yêu cầu module | Param ghi đè fallback/default |
| `required: true` | Bị chặn, kể cả Definition có fallback | Param ghi đè fallback/default |

Ở mẫu trên, `database` bắt buộc. `username` có thể vắng mặt và dùng `app` chỉ
khi Type thực tế khai báo input đó là optional. Type `postgres` hiện có trên
Console của bạn có thể yêu cầu cả hai trường; phải theo schema đang lưu.

#### Namespace và provider binding là hai tham chiếu khác nhau

- `variables.namespace` lấy **tên namespace** từ output của resource namespace.
- `provider_bindings.kubernetes` lấy **đích Kubernetes** từ resource cluster;
  không chứa endpoint, credential hoặc account thứ hai.
- `bootstrap-dev` trong cluster descriptor là class. Nó phải khớp Definition
  cluster, ví dụ mẫu Existing cluster bên dưới. `default` là class của namespace
  trong ví dụ; thay khi nền tảng dùng class khác.
- `@app`, `@env` được platform resolve theo Application/Environment đang chạy.
  Reference tạo dependency; không thay nó bằng giá trị cluster/namespace đoán sẵn.

### Terraform trên AWS

Đây là phần khác biệt cần áp dụng vào **một Definition Terraform đầy đủ**, không
phải file có thể import riêng. Chọn source/mappings của module AWS tương ứng;
không giữ module PostgreSQL Kubernetes rồi chỉ đổi binding thành AWS.

```yaml
driverAccount: "<key AWS account sẵn sàng>"
driverInputs:
  values:
    variables:
      region: ap-southeast-1
    provider_bindings:
      aws:
        target: account
```

Region là literal phù hợp module/account đích. Account/region được khai báo
trên Definition; không tự lấy từ Environment để bù phần còn thiếu.

### Existing cluster: `staging-cluster.yaml`

```yaml
key: staging-cluster
name: Staging cluster
resourceType: k8s-cluster
driverType: existing-cluster
driverAccount: "<key Internal account sẵn sàng>"
driverInputs:
  values:
    variables: {}
criteria:
  - env_type: staging
    class: bootstrap-dev
provision: {}
```

Dùng **key** account đã đăng ký, không điền kubeconfig hoặc tên context vào
`variables`. Type `k8s-cluster` phải có contract target/output phù hợp platform.
Definition này sử dụng cluster đã có, không tạo cluster mới.

### Template workload: `staging-workload.yaml`

```yaml
key: staging-workload
name: Staging workload
resourceType: workload
driverType: template
driverInputs:
  values:
    variables:
      template_source: "<ID template source đã chuẩn bị>"
      template_inputs: {}
      namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
      cluster: "${resources['k8s-cluster.bootstrap-dev#environments.@app.@env'].outputs.cluster}"
criteria:
  - env_type: staging
    class: bootstrap-dev
provision: {}
```

`template_source` là ID đã đăng ký trên Console đích, không phải URL Git.
`template_inputs` phải theo schema của source: điền các input required, chỉ dùng
`{}` khi schema cho phép. Template source Git có schema hỗ trợ các input này;
source cài sẵn kiểu cũ không nhận `template_inputs`. Không đặt Terraform
`source`/`input_mapping`/`provider_bindings` vào template driver.
Namespace/cluster ở đây là resource references; namespace có thể đã cung cấp
placement cluster theo cấu hình platform hiện có.

### Endpoint: `staging-endpoint.yaml`

```yaml
key: staging-endpoint
name: Staging endpoint
resourceType: endpoint
driverType: endpoint
driverInputs:
  values:
    variables:
      rootDomain: apps.company.example
      scheme: https
      gatewayName: traefik-gateway
      gatewayNamespace: traefik
      gatewaySectionName: websecure
criteria:
  - env_type: staging
provision: {}
```

Điền domain, scheme và Gateway/listener thật của môi trường. Nếu không chọn
listener cụ thể, bỏ `gatewaySectionName`. Type `endpoint` phải được chuẩn bị với
contract phù hợp. Driver này không cần module/template source hoặc Driver Account.
Việc đăng ký không tạo Gateway, DNS/TLS hay route; route thuộc luồng deploy.

## 5. Viết criteria và provision

### Chọn Definition bằng criteria

| Field | So khớp với |
|---|---|
| `env_type` | Loại môi trường, chẳng hạn `staging`, `production` |
| `app_id` | Key Application trên Console, ví dụ `idp-...`; không phải tên hiển thị hoặc UUID trong database |
| `env_id` | Key Environment trong Application, thường là `staging` hoặc `production` |
| `res_id` | ID resource trong descriptor |
| `class` | Class mà resource yêu cầu |

Các field trong cùng một dòng là **AND**; các dòng khác nhau là **OR**.
Ví dụ sau áp dụng khi staging/in-cluster **hoặc** production/managed:

```yaml
criteria:
  - env_type: staging
    class: in-cluster
  - env_type: production
    class: managed
```

`criteria: []` không hợp lệ. `criteria: [{}]` là wildcard. Matching vẫn chọn
Definition có độ đặc hiệu cao nhất; hòa điểm cao nhất là lỗi, không tự chọn
Definition đăng ký trước. Khi muốn giới hạn cho một ứng dụng, thêm `app_id`
vào dòng điều kiện tương ứng; dùng cùng `env_id` khi chỉ muốn một Environment
của Application đó. Xem quy tắc chuẩn trong
[UC-03 specification](../usecase/UC-03/specification.md).

### Resource đi kèm bằng provision

Để `provision: {}` nếu không có resource đi kèm. Khi cần, mỗi key là một
resource descriptor; các tham số phải hợp Type của resource đi kèm.
Đây là **đoạn trích để ghép vào Definition**, không phải file import riêng:

```yaml
provision:
  "<resource-type>.<class>#<resource-id>":
    is_dependent: true
    match_dependents: false
    params:
      "<input name được Type cho phép>": "<value đúng kiểu>"
```

- `is_dependent: true`: resource đi kèm chờ resource cha đang cấu hình.
- `match_dependents: true`: các consumer của resource cha cũng phải chờ resource
  đi kèm. Hai cờ mô tả dependency, không phải điều kiện `criteria`.

Chỉ thêm rule khi chủ động thiết kế dependency tương ứng. Quy tắc descriptor,
phạm vi và graph nằm trong [UC-06](../usecase/UC-06/specification.md).

## 6. Tải file để dùng lại

- **Type:** trong danh sách **Loại tài nguyên**, mở **Xem schema → Tải file**.
- **Definition trên dòng danh sách:** bấm **Tải file · vN** trên dòng Definition.
- **Revision cụ thể:** mở **Phiên bản**, chọn **Tải file · rN** ở đúng revision.

Version/revision ghi trên nút là cấu hình sẽ được xuất; không mặc định đó là
revision mà mọi Environment đang ghim. File tải xuống là YAML ở cùng định dạng nhập, đã bỏ các trường hệ thống; giữ
source pin, account key và tham chiếu public, không xuất credential/giá trị secret.
`secret: true` trong schema output vẫn được giữ vì đó là phân loại của output.

Nhập lại file không thay đổi trên cùng Console thường trả `REUSED` nếu các
tham chiếu còn hợp lệ. Chuyển sang Console khác cần chuẩn bị lại Type, source,
account và cập nhật ID/pin tương ứng; import không chuyển đổi chúng tự động.
Muốn tạo Definition khác từ file tải về, đổi `key`, `name` và `criteria` theo
mục tiêu mới để tránh matching hòa điểm. Đổi key không làm thay đổi tài nguyên
đang chạy; chỉ Preview/Deploy của ứng dụng mới áp dụng cấu hình được chọn.

## 7. Lỗi thường gặp

| Hiện tượng | Kiểm tra và sửa |
|---|---|
| Lỗi cú pháp YAML/JSON | Kiểm tra thụt dòng, dấu ngoặc, dấu phẩy JSON, key trùng; chỉ nhập một object |
| File bị từ chối | Dùng UTF-8, đúng phần mở rộng, không quá 1 MiB; bỏ kiểu/giá trị không biểu diễn được bằng JSON |
| Trường không hợp lệ | So tên trường với mẫu; không dán nguyên GET response có `classes`, `version` hoặc metadata hệ thống |
| `CONFLICT` khi nhập mẫu postgres | Type/Definition đã có và khác mẫu; tải cấu hình đang lưu để đối chiếu, không ép thay schema |
| Source/module/template không hợp lệ | Chuẩn bị nguồn trên Console đích, chép đủ pin cùng lần inspect hoặc đúng template source ID |
| Account không hợp lệ | Dùng key account đúng Organization, loại phù hợp driver và trạng thái sẵn sàng |
| Sai input/output mapping | Kiểm tra chiều module → Type, tên trong metadata thật, độ phủ required và cờ secret |
| Tham chiếu namespace/cluster không hợp lệ | Kiểm tra descriptor, output name, class và Definition phụ thuộc; không dùng URL/credential thay reference |
| Thiếu param dù có fallback | Input của Type là required; workload vẫn phải truyền param |
| Đăng ký được nhưng Preview không match/hòa điểm | Đối chiếu resourceType, năm field criteria, class workload/resource và các Definition cạnh tranh |

Sau đăng ký, lưu workload rồi **Preview** để xem Definition và dependency thực sự
được chọn. Kiểm tra lại trước khi **Deploy**; `CREATED` hoặc `REUSED` không phải
bằng chứng tài nguyên đã được provision hoặc deploy thành công.

## Nguồn chuẩn

Hướng dẫn này giải thích cách sử dụng contract hiện có, không thay thế yêu cầu:
[UC-02](../usecase/UC-02/specification.md),
[UC-03](../usecase/UC-03/specification.md),
[Terraform module contract](../architecture/contracts/terraform-modules.md),
[Generic Workload](../architecture/contracts/generic-workload.md).
Các [fixture bootstrap](../../scripts/bootstrap/git-assets/) là dữ liệu cho luồng
bootstrap: thay pin/ID như hướng dẫn ở trên, không coi mọi placeholder bootstrap
hoặc file cấu hình nhiều Definition là một file Console có thể import trực tiếp.
