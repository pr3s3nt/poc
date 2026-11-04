# HTTPRoute dùng Gateway công ty đang có

Resource Type `httproute`, class `in-cluster`, dùng module `terraform/httproute`.
Module chỉ quản lý HTTPRoute riêng của instance, không tạo/sửa Gateway, controller,
DNS, TLS hay cluster RBAC. Gateway và các web hiện có giữ nguyên.

## Điền input staging

| Tham số | Giá trị công ty |
|---|---|
| `parameters.httprouteGatewayName` | Tên Gateway đang phục vụ các web |
| `parameters.httprouteGatewayNamespace` | Namespace chứa Gateway đó |
| `parameters.httprouteGatewaySectionName` | Tên listener, ví dụ https; rỗng để không giới hạn sectionName |
| `parameters.httprouteHostname` | Hostname chính xác của web mới, không có scheme/path |
| `parameters.httproutePath` | PathPrefix, mặc định / |
| `parameters.httprouteModulePath` | terraform/httproute ở root repo tài nguyên |
| `parameters.httprouteModuleId`, `httprouteModuleSHA256` | Để prepare helper tính từ source; backend inspect xác minh Git pin |

`verification.url` của preset nhận hostname này với scheme https; nếu Gateway
listener chỉ phục vụ HTTP thì đổi URL thành http tương ứng. DNS/hostname/TLS và
listener phải phù hợp route. Listener hiện có cần cho phép HTTPRoute từ namespace
app qua allowedRoutes; không tự sửa policy Gateway dùng chung để chạy qua.

## Workload

Frontend acceptance đã có resource này, dùng Service port 8080, backend Service
cùng namespace. Với workload khác, khai báo như sau trong phần resources:

```yaml
resources:
  public_http:
    type: httproute
    class: in-cluster
    id: my-web-http
    params:
      hostname: web.staging.company.example
      service: my-web
      port: 8080
      path: /
```

Port là port của Kubernetes Service, không phải port listener Gateway hoặc mặc
định containerPort. Hostname/service/port là params bắt buộc, path tùy chọn.
Gateway reference thuộc Definition do PE cấu hình; namespace route lấy từ
namespace app. Không hỗ trợ backend khác namespace nên không cần tạo ReferenceGrant
cho backend. Outputs: name, namespace, hostname.

## Chạy và kiểm tra

Chuẩn bị lại input sau khi publish commit mới, chạy profile console qua Playwright
theo enterprise README; bootstrap chỉ đăng ký Type/Definition. Mode all deploy
frontend resource sẽ tạo HTTPRoute, rồi browser checks dùng URL đã cấu hình.
Không cần Git/kubectl/gh CLI trên staging runner. Backend dùng Kubernetes account
đã cấu hình để tạo route; account cần quyền HTTPRoute trong namespace đích.

Route có thể được tạo trước Service; Gateway controller sẽ cập nhật backendRef
khi Service xuất hiện. Provider completion không xác minh status Gateway. Qua
công cụ quan sát platform đã được công ty duyệt, kiểm Accepted=True và
ResolvedRefs=True cho đúng parent/listener và generation, rồi mở đúng hostname.
Không tuyên bố route chạy thật chỉ từ mock tests hoặc registration thành công.

Tên route có prefix http- theo stable instance ID, không chiếm object orch-public.
Đừng khai báo thêm service.publicRoutes cho cùng hostname/path/Service khi dùng
resource này: đó là cơ chế platform route riêng. Không đổi hoặc xóa route của
web khác. Input mới thay workload nên bootstrap cũ có thể BLOCKED; dùng workflow
review/Preview/Deploy của sản phẩm cho app đã tồn tại, không xóa state để ép rerun.

Nguồn schema: [Gateway API HTTPRoute](https://gateway-api.sigs.k8s.io/reference/api-types/httproute/).
