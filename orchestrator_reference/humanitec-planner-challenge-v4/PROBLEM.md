# Humanitec-style Deployment Delta & Resource Planner — v4

## 1. Mục tiêu

Viết chương trình **Go** nhận trạng thái hiện tại của một Humanitec Application Environment và cấu hình Score trước/sau của **đúng một workload**, sau đó:

1. chuyển hai Score Workload sang biểu diễn `modules`/`shared` của Humanitec Deployment Set;
2. kiểm tra Score `before` thật sự khớp workload trong Deployment Set hiện tại;
3. tạo Humanitec Deployment Delta và áp dụng Delta để thu được Candidate Deployment Set;
4. dựng Resource Graph;
5. chọn Resource Definition bằng Humanitec Matching Criteria;
6. mở rộng Resource Reference và co-provisioned resource tới fixed point;
7. đọc mã Terraform `.tf` được Resource Definition tham chiếu để tìm input, output và fingerprint;
8. xuất thứ tự provision theo các batch topo cùng phân loại Active Resource.

Chương trình chỉ lập kế hoạch. Không gọi Humanitec API, `terraform init/plan/apply/destroy`, Git hay cloud provider.

## 2. Mô hình tình huống

Một file Score mô tả **một workload**. Workload có thể chứa nhiều container, gồm container chính và sidecar. Một Application có thể có nhiều workload, vì vậy `current-deployment-set.yaml` là snapshot đầy đủ của Application Environment, còn `before.score.yaml` và `after.score.yaml` chỉ mô tả workload đang thay đổi.

| Tình huống | `beforeScore` | `afterScore` |
|---|---:|---:|
| sửa workload/resource | có | có |
| thêm workload | `null` | có |
| xóa workload | có | `null` |

Hai file Score không phải hai Application và không phải hai phần của cùng một trạng thái. Chúng là phiên bản cũ/mới của cùng workload. Các workload khác chỉ nằm trong Deployment Set hiện tại và phải được giữ nguyên.

## 3. Giao diện chương trình

Build một executable tên tùy ý. Grader gọi:

```bash
planner --case /absolute/path/to/testcases/05-add-private/case.yaml
```

Executable ghi đúng một JSON object ra `stdout`. Log chỉ được ghi ra `stderr`.

Case hợp lệ:

```json
{
  "status": "ACCEPTED",
  "delta": {},
  "deploymentSet": {},
  "challengePlan": {}
}
```

Case bị từ chối:

```json
{
  "status": "REJECTED",
  "error": {
    "phase": "match",
    "code": "NO_MATCHING_DEFINITION",
    "path": "postgres.default#modules.api.externals.db"
  }
}
```

`path` chỉ có khi expected result có trường đó. JSON được so sánh theo cấu trúc nên thứ tự key không quan trọng; thứ tự array phải đúng.

## 4. Phân biệt dữ liệu Humanitec và dữ liệu của đề

Các cấu trúc sau bám trực tiếp định dạng/khái niệm Humanitec:

- Score workload;
- Deployment Set `modules`, `shared` và private resource `externals`;
- Deployment Delta `modules.add/remove/update` và `shared` JSON Patch;
- Resource Definition Entity YAML;
- Matching Criteria và trọng số;
- Resource descriptor, Resource Reference, co-provisioning;
- Terraform Driver `driver_inputs.values.source` và `variables`;
- lifecycle Active Resource.

Các cấu trúc sau chỉ là test harness để bài có thể chấm offline:

- envelope `PlannerCase` trong `case.yaml`;
- `terraformSourceMap`, ánh xạ Git URL/revision sang checkout local;
- projection rút gọn `active-resources.yaml`;
- `challengePlan`, vì Humanitec không xuất artifact này theo schema của đề.

Không được diễn giải `challengePlan` như một API chính thức của Humanitec.

## 5. Manifest testcase

```yaml
apiVersion: challenge.humanitec.io/v1alpha1
kind: PlannerCase
metadata:
  name: 05-add-private
spec:
  context: context.yaml
  currentDeploymentSet: current-deployment-set.yaml
  beforeScore: before.score.yaml
  afterScore: after.score.yaml
  resourceTypes:
    - resource-types/postgres.yaml
    - resource-types/workload.yaml
  resourceDefinitions:
    - resource-definitions/postgres-default.yaml
    - resource-definitions/workload-default.yaml
  activeResources: active-resources.yaml
  terraformSourceMap:
    - url: https://fixtures.invalid/postgres.git
      rev: v1
      directory: terraform/checkouts/01-postgres-v1
```

Mọi path tương đối được resolve từ thư mục chứa `case.yaml`. `beforeScore` hoặc `afterScore` có thể là YAML `null`, nhưng không được cùng `null`.

`context.yaml`:

```yaml
org_id: acme
app_id: shop
env_id: production
env_type: production
```

## 6. Score subset

Hỗ trợ `apiVersion: score.dev/v1b1` và các field:

```yaml
apiVersion: score.dev/v1b1
metadata:
  name: api
containers:
  main:
    image: ghcr.io/acme/api:v2
    command: ["/app"]
    args: ["serve"]
    variables:
      DB_URL: postgresql://${resources.db.host}:${resources.db.port}/app
    resources:
      requests: { cpu: 100m, memory: 128Mi }
      limits: { cpu: 500m, memory: 512Mi }
  metrics:
    image: ghcr.io/acme/metrics:v1
resources:
  db:
    type: postgres
    class: premium
    params:
      size: large
      region: eu-west-1
  cache:
    type: redis
    id: team-cache
    params:
      memory: 2
```

Quy tắc:

- `metadata.name` là workload ID.
- Resource không có `id` là private resource của workload.
- Resource có `id` là shared resource. Các Score resource có cùng `type`, `class`, `id` trỏ tới cùng resource giữa các workload.
- `class` thiếu thì dùng `default`.
- `params` phải tuân theo `inputs_schema` của Resource Type.
- Hỗ trợ placeholder `${resources.<name>}` và `${resources.<name>.<output>}` trong `containers.*.variables.*` và mọi string leaf của `resources.*.params`.
- `$${...}` là escaped literal và không tạo graph edge.
- Output được dùng trong Score phải tồn tại trong `outputs_schema` của Resource Type.
- Các field Score khác nằm ngoài phạm vi v4.

## 7. Chuyển Score thành Deployment Set fragment

Container được chuyển thành:

```yaml
modules:
  api:
    profile: humanitec/default-module
    spec:
      containers:
        main:
          id: main
          image: ghcr.io/acme/api:v2
          variables:
            DB_HOST: ${externals.db.host}
```

Private resource:

```yaml
    externals:
      db:
        type: postgres
        class: premium
        params:
          size: large
```

Shared resource:

```yaml
shared:
  team-cache:
    type: redis
    class: default
    params:
      memory: 2
```

Placeholder mapping:

| Score | Deployment Set |
|---|---|
| `${resources.db.host}` với private `db` | `${externals.db.host}` |
| `${resources.cache.host}` với `cache.id: team-cache` | `${shared.team-cache.host}` |

Mọi resource được khai báo trong Score đều là dependency, kể cả khi không có container variable đọc output của nó.

## 8. Kiểm tra trạng thái trước và tạo Candidate Set

Sau khi convert `before.score.yaml`:

- module phải deep-equal `currentDeploymentSet.modules[workloadID]`;
- mỗi shared resource do Score `before` khai báo phải deep-equal entry tương ứng trong `currentDeploymentSet.shared`;
- sai khác trả `BEFORE_MISMATCH` tại phase `before-check`.

Candidate Set được tạo bằng cách chỉ thay phần đóng góp của workload mục tiêu:

- module mục tiêu được add/update/remove;
- shared entry có trong `before` nhưng không còn trong `after` bị remove;
- shared entry trong `after` được add/update;
- mọi module và shared entry khác giữ nguyên;
- thêm shared entry trùng ID nhưng khác nội dung trả `SHARED_CONFLICT`.

## 9. Deployment Delta

Delta dùng cấu trúc Humanitec:

```yaml
modules:
  add:
    api: { ... }
  remove: [worker]
  update:
    api:
      - op: replace
        path: /spec/containers/main/image
        value: ghcr.io/acme/api:v2
shared:
  - op: add
    path: /team-cache
    value:
      type: redis
      class: default
```

`modules.update.<id>` là RFC 6902 JSON Patch relative với module. `shared` là JSON Patch relative với object `shared`. Đây cũng là hình thức mà `score-humanitec` sinh cho shared resources.

Diff deterministic:

1. object: duyệt hợp key theo lexical order;
2. key bị mất → `remove`; key mới → `add`; key có ở cả hai → đệ quy;
3. array: diff phần index chung, remove đuôi từ index lớn xuống nhỏ, add đuôi bằng path `/-`;
4. primitive hoặc khác kiểu → `replace`;
5. JSON Pointer escape `~` thành `~0`, `/` thành `~1`;
6. nhánh rỗng bị omit; no-op delta là `{}`.

Invariant bắt buộc:

```text
currentDeploymentSet + delta = deploymentSet
```

## 10. Resource Type subset

Fixture dùng Entity YAML rút gọn:

```yaml
apiVersion: entity.humanitec.io/v1b1
kind: ResourceType
metadata:
  id: postgres
entity:
  inputs_schema:
    type: object
    properties:
      size: { type: string }
    additionalProperties: false
  outputs_schema:
    type: object
    properties:
      host: { type: string }
      port: { type: number }
    additionalProperties: false
```

V4 chỉ cần object schema có `properties` và `additionalProperties: false`.

## 11. Resource Definition subset

Chỉ chấp nhận Entity YAML mà `humctl create -f` sử dụng:

```yaml
apiVersion: entity.humanitec.io/v1b1
kind: Definition
metadata:
  id: postgres-prod
entity:
  name: postgres-prod
  type: postgres
  driver_type: humanitec/terraform
  driver_account: fixture-account
  driver_inputs:
    values:
      source:
        url: https://fixtures.invalid/postgres.git
        rev: v1
        path: modules/postgres
      variables:
        subnet_id: ${resources['network.default#shared.vpc'].outputs.subnet_id}
        owner: ${context.app.id}-${context.env.id}
  criteria:
    - class: premium
      env_type: production
  provision:
    audit.@#@:
      is_dependent: true
      match_dependents: true
      params:
        target: database
```

V4 hỗ trợ `humanitec/terraform` và `humanitec/echo`. Với `entity.humanitec.io/v1b1`, `criteria` nằm trong `entity`. Dạng `core.api.humanitec.io/v1` có top-level `criteria` được Humanitec docs minh họa ở nơi khác nhưng không thuộc input của đề này.

## 12. Resource descriptor và graph

Node identity:

```text
<type>.<class>#<res_id>
```

| Resource | `res_id` |
|---|---|
| workload `api` | `modules.api` |
| private `db` của `api` | `modules.api.externals.db` |
| shared `team-cache` | `shared.team-cache` |
| descriptor chỉ rõ ID | ID trong descriptor |

Graph edge có chiều **consumer → provider**. Vì thế provider phải xuất hiện ở batch provision sớm hơn consumer.

Initial graph gồm:

- một `workload.default#modules.<id>` cho mỗi module;
- mọi private/shared dependency trong Candidate Set;
- edge workload → mỗi private resource;
- edge phát sinh từ placeholder trong workload spec và resource params.

V4 không tự thêm `base-env`, `k8s-cluster`, `k8s-namespace`; chúng chỉ xuất hiện khi được Resource Definition tham chiếu hoặc co-provision. Đây là ranh giới subset, không phải mô tả toàn bộ graph nội bộ của Humanitec.

## 13. Matching Criteria

Một criterion match khi mọi field nó khai báo bằng context của node. Field và trọng số theo Humanitec:

| Field | Trọng số |
|---|---:|
| `env_type` | 1 |
| `app_id` | 2 |
| `env_id` | 4 |
| `res_id` | 8 |
| `class` | 16 |

`entity.type` luôn phải bằng resource type. Trong tất cả criterion match, chọn criterion có tổng trọng số lớn nhất. Không có match → `NO_MATCHING_DEFINITION`. Có nhiều Definition đồng hạng cao nhất → `AMBIGUOUS_DEFINITION`. Definition không có criterion hoặc `criteria: []` không được xét; criterion `{}` có score 0 và match mọi context của cùng type.

## 14. Resource Reference

Hỗ trợ ở mọi string leaf của `driver_inputs` và `provision.*.params`:

```text
${resources['TYPE[.CLASS][#ID]'].outputs.OUTPUT}
${resources.TYPE.outputs.OUTPUT}
```

Thiếu `CLASS`, thiếu `ID`, hoặc dùng `@` thì kế thừa class/ID của resource hiện tại. Ví dụ bốn descriptor sau tương đương trong context hiện tại:

```text
dns
dns.@
dns#@
dns.@#@
```

Reference tạo provider node nếu chưa có và tạo edge current → provider. Output referenced giữa các Resource Definition được kiểm tra bằng Terraform `output` của provider, hoặc bằng `driver_inputs.values` nếu provider dùng Echo Driver. Resource Selector có `<`/`>` nằm ngoài v4.

## 15. Co-provisioning

Với mỗi `entity.provision`:

- key được parse như Resource descriptor;
- node mới kế thừa class/ID khi thiếu hoặc là `@`;
- `params` trở thành Resource inputs của node mới;
- `is_dependent: true` tạo edge child → parent;
- `match_dependents: true` tạo edge từ mọi consumer hiện có của parent → child;
- tiếp tục match Definition, scan reference và provision của node mới đến fixed point.

Cycle ở graph cuối trả `RESOURCE_GRAPH_CYCLE` tại phase `schedule`.

## 16. Terraform source và contract

`driver_inputs.values.source` dùng `url`, optional `rev`, optional `path`. Tìm entry có cùng `(url, rev)` trong `terraformSourceMap`, ghép `directory/source.path`, rồi đọc đệ quy mọi file `*.tf`.

Không có `contract.json`. Solver phải đọc HCL:

- top-level `variable "name" { ... }` là Terraform input;
- variable không có `default` là required;
- top-level `output "name" { ... }` là Terraform output;
- mọi output trong Resource Type `outputs_schema.properties` phải tồn tại trong module;
- Score/co-provision `params` được truyền thẳng thành Terraform variables cùng tên;
- `driver_inputs.values.variables` bổ sung Terraform variables;
- Terraform default chỉ dùng khi không có hai nguồn trên;
- fixtures hợp lệ không đặt cùng một key ở `params` và `driver_inputs.values.variables`.

Unknown/missing input trả `UNKNOWN_TERRAFORM_INPUT` hoặc `MISSING_TERRAFORM_INPUT`. Thiếu output của Resource Type trả `MISSING_TERRAFORM_OUTPUT`.

Fingerprint:

1. lấy mọi `.tf` dưới module directory, sort theo relative path lexical;
2. với mỗi file, feed vào SHA-256: `relativePath`, byte `0`, raw content, byte `0`;
3. render `sha256:<lowercase hex>`.

Fixtures chỉ dùng HCL tĩnh, không dùng generated blocks. Nên dùng HCL parser thay vì regex trong lời giải thực tế.

Placeholder `${context.app.id}`, `${context.env.id}`, `${context.env.type}`, `${context.org.id}`, `${context.res.id}`, `${context.res.class}`, `${context.res.type}` được resolve trước khi lập Terraform invocation. Resource output reference được giữ làm binding runtime nhưng vẫn tạo dependency.

## 17. Active Resources

Fixture là projection của dữ liệu `humctl get active-resources`:

```yaml
- metadata:
    type: postgres
    class: default
    res_id: modules.api.externals.db
  status:
    resource_definition_id: postgres-default
    gu_res_id: 0123456789abcdef
```

Khóa so sánh là descriptor `(type, class, res_id)`:

- `existing`: có trong graph mong muốn và Active Resources;
- `new`: có trong graph mong muốn nhưng chưa active;
- `unreferenced`: đang active nhưng không còn trong graph.

Humanitec cố provision **mọi resource trong graph ở mỗi deployment**; Driver quản lý state của resource đã tồn tại. Bỏ dependency chỉ làm Active Resource thành `unreferenced`, không tự `destroy`. Vì vậy đề không có action `reuse`, `update`, `remove` hay `destroy`.

## 18. Output `challengePlan`

```yaml
matchedDefinitions:
  postgres.default#modules.api.externals.db:
    definitionId: postgres-default
    score: 0
    criterion: {}
resourceGraph:
  nodes:
    - descriptor: postgres.default#modules.api.externals.db
      origins: [private-dependency]
      resourceInputs: { size: large }
  edges:
    - from: workload.default#modules.api
      to: postgres.default#modules.api.externals.db
      reason: deployment-set-private
      path: /modules/api/externals/db
terraform:
  - resource: postgres.default#modules.api.externals.db
    definitionId: postgres-default
    source: { url: https://fixtures.invalid/postgres.git, rev: v1 }
    localDirectory: terraform/checkouts/01-postgres-v1
    fingerprint: sha256:...
    inputs:
      - name: size
        source: resource-input
        value: large
        type: string
    outputs: [host, name, port]
provisionBatches:
  - [postgres.default#modules.api.externals.db]
  - [workload.default#modules.api]
activeResources:
  existing: []
  new: []
  unreferenced: []
```

Luật sort:

- node, descriptor list, Active Resource list: lexical;
- edge: `(from, to, reason, path)` lexical;
- Terraform record: resource descriptor lexical;
- Terraform inputs và outputs: tên lexical;
- mỗi topo batch: lexical;
- Kahn algorithm chọn toàn bộ node đang không còn dependency làm một batch.

## 19. Phase và lỗi

Nên tổ chức Go code theo các phase sau; starter đã cung cấp chữ ký hàm:

```text
LoadCase
ValidateHumanitecDocuments
ConvertScoreToWorkloadFragment
ValidateBeforeFragment
BuildDeploymentDelta
ApplyDeploymentDelta
BuildInitialResourceGraph
MatchResourceDefinitions
ExpandResourceReferences
ExpandProvisionRules
InspectTerraformSource
ResolveDriverInputs
ClassifyActiveResources
TopologicalBatches
RenderResult
```

Error code được chấm gồm:

| Phase | Code tiêu biểu |
|---|---|
| `validate` | `INVALID_SCORE`, `UNKNOWN_RESOURCE_TYPE`, `MISSING_SCORE`, `WORKLOAD_NAME_CHANGED` |
| `convert` | `UNKNOWN_RESOURCE`, `UNKNOWN_OUTPUT`, `SHARED_CONFLICT` |
| `before-check` | `BEFORE_MISMATCH`, `WORKLOAD_ALREADY_EXISTS` |
| `graph` | `UNKNOWN_RESOURCE`, `UNKNOWN_OUTPUT`, `INVALID_RESOURCE_REFERENCE`, `UNSUPPORTED_SELECTOR` |
| `match` | `NO_MATCHING_DEFINITION`, `AMBIGUOUS_DEFINITION` |
| `terraform` | `SOURCE_REQUIRED`, `SOURCE_NOT_FOUND`, `MISSING_TERRAFORM_INPUT`, `UNKNOWN_TERRAFORM_INPUT`, `MISSING_TERRAFORM_OUTPUT` |
| `schedule` | `RESOURCE_GRAPH_CYCLE` |

Nếu nhiều lỗi cùng tồn tại, phase order ở trên quyết định lỗi quan sát được. Trong một phase, duyệt descriptor/path lexical.

## 20. Constraints

- tối đa 100 workload;
- tối đa 1.000 resource node sau fixed point;
- tối đa 2.000 graph edge;
- tối đa 1.000 Resource Definition;
- tối đa 200 `.tf` file cho một source, tổng 10 MiB;
- mọi identifier trong fixture dùng `[A-Za-z0-9_-]+`;
- graph hợp lệ là DAG.

Giải pháp dự kiến chạy trong `O(D·C + V + E + T log T)`, với `D` là số Definition, `C` là số criterion, `V/E` là graph, `T` là số Terraform file/contract item.

## 21. Ngoài phạm vi v4

- secret values và `secret_refs`;
- Terraform `script`, `files`, runner modes, credentials, state, plan/apply/destroy;
- Resource Selector;
- đầy đủ implicit cluster/namespace graph;
- driver khác Terraform/Echo;
- scheduled deletion, detach, rollback và drift;
- toàn bộ Score fields ngoài subset;
- tương thích byte-for-byte với implementation nội bộ của Humanitec.

Các field có thể được bổ sung ở phiên bản sau mà không thay đổi pipeline phase.
