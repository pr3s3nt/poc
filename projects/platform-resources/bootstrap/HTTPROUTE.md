# Địa chỉ public của workload qua Gateway Traefik có sẵn

Cách khuyến nghị: workload khai báo resource `endpoint`; Definition `endpoint.json`
(driver dựng sẵn `endpoint`) giữ chính sách domain gốc/Gateway. Deploy giữ chỗ
hostname trong database (một owner trên toàn installation), rồi sinh HTTPRoute vào
cùng manifest Git của workload để Fleet apply/remove theo inventory. Không có
Terraform module/state cho route. Gateway/controller/DNS/TLS vẫn do platform quản lý.
Hợp đồng: [EP-01–06](../../../orchestrator_docs/architecture/contracts/endpoint.md).

## Endpoint (khuyến nghị)

| Tham số | Ý nghĩa |
|---|---|
| `parameters.endpointRootDomain` | Domain gốc dành riêng cho installation, ví dụ `staging.company.example` |
| `parameters.endpointScheme` | `https` hoặc `http`, theo listener thật |
| `parameters.httprouteGatewayName`, `httprouteGatewayNamespace`, `httprouteGatewaySectionName` | Gateway/listener có sẵn; section rỗng để bỏ sectionName |
| `parameters.endpointName`, `endpointPath` | Nhãn DNS và PathPrefix của frontend mẫu |

```yaml
resources:
  public:
    type: endpoint
    params:
      name: acceptance   # → acceptance.<rootDomain>
      port: http         # tên port trong service.ports
      path: /
```

Developer chỉ chọn name/port/path; tham số khác bị từ chối. Preview hiện URL dự
kiến và báo nếu hostname đã bị workload khác giữ. HTTPRoute tên ổn định
`endpoint-<hash owner>`, trỏ Service đã render và port số của port có tên đó.
Đổi name: hostname mới được giữ khi Deploy, hostname cũ chỉ nhả sau khi Deploy
thành công; bỏ endpoint hoặc gỡ workload thì route bị prune và hostname được nhả.
Không khai báo endpoint cùng `service.publicRoutes` hoặc template tự sinh route.
Database không thấy route ngoài hệ thống: hãy giao riêng subdomain cho installation,
hoặc liệt kê tên đã dùng trong `reservedNames` của Definition.

## Route do template sinh (cách cũ)

Vẫn hỗ trợ cho workload đã dùng annotation; chuyển sang endpoint cần đổi workload
có chủ đích. Frontend mẫu không còn dùng cách này.

### Input

| Tham số | Ý nghĩa |
|---|---|
| `parameters.httprouteEnabled` | Bật khả năng render route; đặt false nếu không cần |
| `parameters.httprouteGatewayName` | Tên Gateway đang có |
| `parameters.httprouteGatewayNamespace` | Namespace chứa Gateway |
| `parameters.httprouteGatewaySectionName` | Tên listener, ví dụ https; rỗng để bỏ sectionName |

Workload Definition truyền enabled/Gateway/listener vào template_inputs với
các tên httpRouteEnabled, httpRouteGatewayName, httpRouteGatewayNamespace,
httpRouteGatewaySectionName. Hostname/path/port nằm trên từng workload để một
Definition dùng chung không vô tình tạo route cho mọi ứng dụng.

Workload dùng ba annotation:

```yaml
metadata:
  name: frontend
  annotations:
    routing.orchestrator.dev/hostname: web.staging.company.example
    routing.orchestrator.dev/path: /
    routing.orchestrator.dev/service-port: http
service:
  ports:
    http:
      port: 8080
      targetPort: 8080
```

Đây là phần ghép vào Generic Workload, không phải manifest apply độc lập. Chỉ
workload có annotation hostname và template bật httpRouteEnabled mới sinh
HTTPRoute. Bỏ annotation hostname để không tạo route cho workload đó.
Backend Service name và namespace lấy từ workload; port số lấy từ Service
port có tên đã chọn, không lấy containerPort hoặc port listener của Gateway.
Template chặn port không có trong Service và chặn khai báo đồng thời
service.publicRoutes trên cùng workload. Không khai báo resources.public_http.

## Gateway có sẵn và kiểm tra

Route do template sinh nằm trong namespace app, tên bằng workload (ví dụ frontend);
route endpoint tên `endpoint-<hash>`. Cả hai
parentRefs trỏ Gateway/listener được chỉ định. Gateway cần cho phép routes từ
namespace ứng dụng; backendRefs chỉ tới Service cùng namespace. Không tạo
ReferenceGrant hoặc sửa allowedRoutes của Gateway dùng chung.

Giữ DNS/TLS/listener phù hợp hostname. verification.url của preset ghép từ
endpointScheme/endpointName/endpointRootDomain; đặt endpointScheme=http chỉ khi
listener thật chỉ phục vụ HTTP. Chuẩn bị lại input và
source/template pins sau khi publish commit; bootstrap qua Playwright/Console
không cần Git/kubectl/gh CLI và không tự nâng quyền cluster.

Mode bootstrap đăng ký workload Definition/template source. Mode all deploy app,
Fleet nhận HTTPRoute cùng Deployment/Service. Qua công cụ platform đã duyệt,
kiểm Accepted/ResolvedRefs cho đúng parent/generation và mở URL thật. Render
thành công không chứng minh Gateway/DNS/TLS đã hoạt động trên staging.

## Nếu đã dùng bản HTTPRoute Terraform trước đây

Bỏ module khỏi source không tự xóa remote route, Type/Definition, state hay
historical pin. Không import/adopt route cũ vào Fleet, không state-rm hoặc xóa
trực tiếp trên cluster. Giữ artifact/old pin để cleanup đúng ownership qua
workflow sản phẩm được review; xem Preview để remove resource Terraform cũ
và publish/select revision workload dùng template mới. Không để hai route
cùng hostname/path trỏ một web tồn tại ngoài ý muốn. Nếu không thể chứng minh
ownership hoặc outcome thì dừng phần chuyển đổi, giữ dữ liệu và state.

Nếu chưa deploy route Terraform, dùng ngay bộ nguồn mới cho setup mới. Script
bootstrap vẫn chặn khác cấu hình trên app đã có; không xóa receipt để ép rerun.

Schema route theo [Gateway API HTTPRoute](https://gateway-api.sigs.k8s.io/reference/api-types/httproute/).
