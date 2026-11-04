# HTTPRoute của workload qua Gateway Traefik có sẵn

HTTPRoute nằm trong Git workload template, được render cùng Service và đưa
vào Git manifest để Fleet apply/remove theo inventory của workload. Không có
Resource Type/Definition/module Terraform riêng cho route và không có state
Terraform mới. Gateway/controller/DNS/TLS vẫn do platform công ty quản lý.

## Input staging

| Tham số | Ý nghĩa |
|---|---|
| `parameters.httprouteEnabled` | Bật khả năng render route; đặt false nếu không cần |
| `parameters.httprouteGatewayName` | Tên Gateway đang có |
| `parameters.httprouteGatewayNamespace` | Namespace chứa Gateway |
| `parameters.httprouteGatewaySectionName` | Tên listener, ví dụ https; rỗng để bỏ sectionName |
| `parameters.httprouteHostname` | Domain của frontend mẫu, không có scheme/path |
| `parameters.httproutePath` | PathPrefix, mặc định / |
| `parameters.httprouteServicePort` | Tên port khai báo trong Service của frontend, mặc định http |

Workload Definition truyền enabled/Gateway/listener vào template_inputs với
các tên httpRouteEnabled, httpRouteGatewayName, httpRouteGatewayNamespace,
httpRouteGatewaySectionName. Hostname/path/port nằm trên từng workload để một
Definition dùng chung không vô tình tạo route cho mọi ứng dụng.

Frontend mẫu nhận ba annotation từ input:

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

HTTPRoute v1 nằm trong namespace app, tên bằng workload (ví dụ frontend),
parentRefs trỏ Gateway/listener được chỉ định. Gateway cần cho phép routes từ
namespace ứng dụng; backendRefs chỉ tới Service cùng namespace. Không tạo
ReferenceGrant hoặc sửa allowedRoutes của Gateway dùng chung.

Giữ DNS/TLS/listener phù hợp hostname. verification.url của preset dùng https;
đổi thành http nếu listener thực sự chỉ phục vụ HTTP. Chuẩn bị lại input và
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
