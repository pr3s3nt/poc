# Keycloak qua Score — bản thử issue #83

Hai file dùng với Console của nhánh `features/score`:

- `score.yaml`: Keycloak, Service HTTP 8080, PostgreSQL riêng, tham chiếu Secret và endpoint
  do nền tảng quản lý.
- `humanitec.score.yaml`: profile `backend`, class `git-dev`, một replica,
  filesystem ghi được và startup/liveness probe. Startup cho phép khoảng 10 phút
  để image chuẩn build cấu hình và khởi tạo schema PostgreSQL.

Image Keycloak 26.8.2 được ghim cả tag và OCI index digest; manifest được kiểm tra
trên registry. Đây là cấu hình **thử nghiệm** dùng `start-dev`, HTTP và cache local.
Dữ liệu Keycloak nằm trong PostgreSQL, không dùng database nhúng trong container.
Không dùng mẫu này như cấu hình production/HA.

## Cách thử

1. Trong Console tính năng, tạo Application mới hoặc chọn Application thử riêng,
   chọn `staging`. Environment cần có Account/CD binding, Secret Store Connection
   và các Definition namespace/cluster/PostgreSQL/workload/endpoint hợp lệ.
   Platform phải đăng ký Type `endpoint` và Definition driver `endpoint` với
   `rootDomain`, `scheme`, Gateway/listener thực tế theo
   [endpoint setup](../../../projects/platform-resources/bootstrap/HTTPROUTE.md).
2. Mở **Biến & Bí mật**, tạo key `KEYCLOAK_ADMIN_PASSWORD`, loại **Secret**,
   nhập mật khẩu admin bạn tự chọn. Giá trị này không đặt trong Score hay Git.
   Bootstrap chỉ tạo admin lúc database chưa có admin; đổi Secret này về sau
   không tự đổi mật khẩu của tài khoản Keycloak đã tồn tại.
3. **Thêm Workload → Nhập Workload**: chọn/dán `score.yaml` và extension
   `humanitec.score.yaml`, bấm **Kiểm tra** rồi **Lưu draft Workload**.
4. Kiểm tra form có tên `keycloak`, database `keycloak`, tham chiếu
   `KC_BOOTSTRAP_ADMIN_PASSWORD` tới key trên và `KC_DB_PASSWORD` tới output
   `db.password`. Resource `env` chỉ là nguồn cấu hình, không provision hạ tầng.
5. **Xem trước**: kiểm tra địa chỉ public dự kiến `keycloak.<rootDomain>` và
   các thay đổi, rồi **Triển khai** trên toàn Environment. PostgreSQL được tạo từ
   dependency `db`; không cần tạo workload PostgreSQL thứ hai. Theo dõi cả
   provisioning và CD/readiness. Chưa có PostgreSQL cho dependency này thì lần
   deploy sẽ tạo database mới; không trỏ tới database của ứng dụng khác.
6. Khi deployment thành công, mở URL trong **Địa chỉ public** rồi vào `/admin/`,
   đăng nhập `admin` cùng mật khẩu ở bước 2. `KC_HOSTNAME` tham chiếu
   `${resources.public.url}` để Keycloak phát hành URL/issuer đúng domain;
   `KC_PROXY_HEADERS=xforwarded` dành cho Gateway reverse proxy. Nền tảng giữ
   chỗ hostname và sinh HTTPRoute qua CD. DNS/listener/TLS phải được platform
   chuẩn bị; chỉ parse thành công chưa chứng minh URL truy cập được. Đường mở
   web được kiểm tra theo [runbook Tailscale](../../../orchestrator_docs/operations/tailscale-handoff.md).

`git-dev` và profile `backend` thuộc setup đã dùng cho feature Score. Môi trường
khác cần class/profile phù hợp với Definition đã chọn. Type `postgres` phải có
inputs `database`, `username`; outputs `host`, `port`, `database`, `username` và
`password` (đánh dấu secret). Secret reference phải chiếm toàn bộ binding.

Trên cụm kind thử hiện có, namespace mới còn cần bounded Fleet access setup
sau khi namespace được tạo; dùng [helper/runbook hiện có](../../../e2e/setup/score-authoring-namespace/README.md).
Bộ rules bounded hiện có của helper đã bao gồm HTTPRoute. Kiểm tra đúng
Application/namespace UID và ServiceAccount Fleet trước khi áp dụng; không thay
shared cluster RBAC để bỏ qua bước này. Cleanup qua Application
Deletion; nếu có setup Role/RoleBinding riêng, gỡ đúng ownership/UID sau khi CD
withdrawal rồi tiếp tục product cleanup theo runbook.

## Phạm vi đã kiểm tra

Bản ban đầu chưa có endpoint được nhập qua browser trên URL Tailscale của feature Console. Score
qua schema/conversion và dừng ở guard key chưa tồn tại (`INVALID_VARIABLE`), đúng
với bước chuẩn bị Secret bắt buộc ở trên. Bản kiểm tra chỉ trong bộ nhớ thay
reference admin bằng literal giả không bí mật để kiểm tra các binding PostgreSQL,
probe, resource sizing và template pin; export/reparse giữ nguyên kết quả đó.
File bàn giao vẫn giữ reference Secret. Không lưu draft, không tạo Secret hay
deploy Keycloak trong lần kiểm tra mẫu. Runtime Keycloak chưa được nghiệm thu
từ kết quả parse này. Bản bổ sung endpoint đã có Type/Definition app-scoped trên feature stack, dùng
Gateway hiện có và domain thử Tailscale riêng (URL/policy cụ thể chỉ lưu trong
private handoff). Exact files parse thành công trên Application Keycloak đã có
Secret; export/reparse giữ nguyên các binding và saved draft không đổi.

Schema Score pinned và docs/whitespace checker pass. Definition matching-preview
hiện từ chối driver endpoint (`definition.driverType`); đây là giới hạn sẵn có,
không phải kết quả Environment Preview. Chưa submit Preview/Deploy trên draft
của người dùng. DNS và ingress relay tới Gateway được kiểm tra; URL Keycloak
hiện trả 404 vì chưa có route/workload được triển khai. Không coi đây là link
Keycloak sẵn sàng sử dụng. Cần reimport mẫu endpoint, Save và Preview/Deploy,
sau đó kiểm tra HTTPRoute Accepted/ResolvedRefs, Keycloak readiness và browser
login trước khi bàn giao URL hoạt động.

Nguồn:
[Keycloak downloads](https://www.keycloak.org/downloads),
[container/start-dev/bootstrap admin](https://www.keycloak.org/server/containers),
[database configuration](https://www.keycloak.org/server/db),
[Score contract](../../../orchestrator_docs/architecture/contracts/score-workload.md).
