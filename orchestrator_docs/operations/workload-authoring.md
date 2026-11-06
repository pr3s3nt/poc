---
id: OPS-WORKLOAD-AUTHORING
artifact: operations-runbook
status: current
last_reviewed: 2026-11-06
---

# Hướng dẫn sử dụng trang Thêm Workload

Hướng dẫn áp dụng cho trang Thêm/Sửa Workload của issue #37, với default lấy từ
template Git theo [ADR-039](../architecture/decisions/ADR-039-template-owned-workload-defaults.md).
Tên nút tiếng Anh được ghi ở những bước chính để đối chiếu khi Console dùng English.

## 1. Chọn ứng dụng, môi trường và template

1. Mở ứng dụng trong **Ứng dụng / Applications**.
2. Chọn đúng môi trường **staging** hoặc **production**.
3. Bấm **Thêm Workload / Add Workload**.
4. Trong **Chọn loại workload**, chọn template phù hợp, chẳng hạn **Backend API**.
   Template hiển thị kiểu thực thi và phiên bản; chỉ những mẫu đã được nền tảng
   công bố mới xuất hiện. Có thể tìm theo tên hoặc kiểu thực thi.
5. Chọn **Tự cấu hình / nhập Workload** khi bạn muốn tự khai báo cấu hình.

Một workload mô tả ứng dụng hoặc tác vụ cần chạy. Nó có thể chứa nhiều container;
Deployment có thể tạo nhiều Pod theo số replica. Một workload không đồng nghĩa
với đúng một Pod.

| Kiểu thực thi | Khi sử dụng |
|---|---|
| Deployment | Web, API hoặc worker chạy liên tục |
| StatefulSet | Ứng dụng cần định danh và storage theo Pod; hiện giới hạn 1 replica |
| CronJob | Tác vụ chạy theo lịch cron |

Khi chọn template, kiểu thực thi đi theo mẫu. Với tự cấu hình, chọn kiểu thực thi
trong **Hiện nâng cao Workload → Cấu hình ít thay đổi**.

## 2. Điền thông tin cơ bản

Ban đầu chỉ hiện tên workload, tên/image của container và biến môi trường.
CronJob có thêm phần lịch chạy. Các trường còn lại nằm trong **Hiện nâng cao**.

| Trường | Cách điền | Ví dụ |
|---|---|---|
| Tên Workload | Tên nhận diện workload trong môi trường; ưu tiên chữ thường, số và dấu gạch ngang | `demo-web` |
| Tên Container | Tên phân biệt container trong workload; giữ `main` nếu chỉ có một container | `main` |
| Ảnh Container | Image mà bạn muốn chạy, kèm tag hoặc digest | `nginxinc/nginx-unprivileged:1.27-alpine` |

Template **Backend API** không cấp image mặc định: phải nhập image trước khi form
có thể tính default. Thông báo yêu cầu nhập image lúc này là hướng dẫn điền trường.
Template có image sẵn sẽ hiện image mặc định mờ; có thể giữ nguyên để kế thừa.

Bấm **+ Thêm Container** nếu ứng dụng cần nhiều container. Mỗi container có image,
biến môi trường và phần nâng cao riêng. Tên các container phải khác nhau.

## 3. Hiểu giá trị mờ và Khôi phục mặc định

Giá trị mờ trong ô trống là giá trị đang kế thừa. Bạn không cần nhập lại, cũng
không cần xóa nó trước khi gõ giá trị riêng.

Ví dụ ô CPU hiện mờ `100m`:

- Giữ nguyên ô trống: dùng `100m` từ template đã chọn.
- Gõ `250m`: dùng giá trị riêng `250m` cho workload này.
- Xóa `250m` hoặc bấm **Khôi phục mặc định / Restore defaults**: quay lại kế thừa.

Dropdown cũng hiện default mờ khi chưa ghi đè. Khi mở menu, mỗi lựa chọn chỉ xuất
hiện một lần. Chọn một giá trị cụ thể là tạo override, kể cả khi trùng default;
bấm **Khôi phục mặc định** để bỏ override. `0`, `false` hoặc **Tắt / Disabled** được
chọn rõ ràng là giá trị riêng, khác với để kế thừa.

Các mẫu chuẩn 1.1.0 hiện khai báo:

| Giá trị | staging | production |
|---|---|---|
| CPU request của App container | `100m` | `200m` |
| Memory request của App container | `128Mi` | `256Mi` |
| Memory limit | `512Mi` | `512Mi` |
| CPU limit | Không khai báo | Không khai báo |
| Revision history | `3` | `10` |

Đây là giá trị của các template hiện tại; mẫu khác có thể khai báo khác. Xem giá
trị thực tế trên form/Preview. Workload tự cấu hình không chọn template sẽ không
nhận bộ default authoring này từ backend.

## 4. Thêm biến môi trường

Trong container cần dùng, bấm **+ Thêm biến / Add variable**:

- Điền trực tiếp **Tên** và **Giá trị** cho giá trị thông thường, ví dụ
  `LOG_LEVEL` = `info`.
- Dùng **Chọn từ… / Choose from…** để lấy Variable/Secret của ứng dụng, output
  của resource hoặc Service của workload khác trong cùng phạm vi được phép.
- Nếu thiếu Variable/Secret hoặc resource, dùng thao tác tạo tại chỗ rồi chọn
  nguồn vừa tạo. Với mật khẩu/token, dùng Secret thay cho giá trị literal.

Biến thuộc container đang sửa. Thêm một biến vào `main` không tự thêm nó vào
container khác. Khi đổi loại nguồn, chọn lại nguồn/output tương ứng.

Variable/Secret được tạo bằng hộp thoại riêng sẽ được lưu ngay. Hủy form workload
không xóa Variable/Secret đã tạo.

## 5. Mở nâng cao của Container khi cần

Bấm **Hiện nâng cao** trong đúng khối container. Đóng/mở phần này giữ các giá trị
đang nhập. Các nhóm thường dùng:

| Nhóm | Dùng để làm gì | Lưu ý |
|---|---|---|
| Tài nguyên tính toán | CPU/memory request và limit | CPU dùng `100m`, `250m`, `1`; memory dùng `128Mi`, `512Mi`, `1Gi`. Limit phải đủ bao phủ request có hiệu lực |
| Cổng container | Khai báo cổng tiến trình lắng nghe | Ví dụ tên `http`, cổng `8080`, protocol `TCP`. Khai báo cổng không làm tiến trình tự mở cổng |
| Kiểm tra sức khỏe | Readiness, Liveness, Startup | Chỉ bật khi có đường dẫn/cổng hoặc lệnh kiểm tra đúng với ứng dụng |
| Khởi chạy | Command, args, working directory, vai trò App/Init/Sidecar | Mỗi dòng command/args là một đối số. Để trống để dùng cấu hình của image |
| Volumes & files | Mount dữ liệu hoặc file vào container | Chọn nguồn và đường dẫn mount; xem mục 7 |
| Bảo mật container | UID/GID, non-root, filesystem chỉ đọc, capabilities | Chỉ chọn cấu hình image hỗ trợ; thay đổi có thể làm ứng dụng không khởi động |

**Readiness** xác định khi nào container sẵn sàng nhận lưu lượng; **Liveness** phát
hiện container cần khởi động lại; **Startup** dành thời gian cho ứng dụng khởi động.
Ví dụ HTTP probe cho Nginx trên cổng 8080: path `/`, port `8080`. Không sao chép
endpoint này cho ứng dụng chưa có endpoint tương ứng. Init thông thường không có probes.

Vai trò **App** dành cho tiến trình chính, **Init** chạy trước ứng dụng rồi kết
thúc, **Sidecar** chạy hỗ trợ bên cạnh ứng dụng. Init/Sidecar cần thứ tự khởi tạo;
ít nhất phải có một App container.

## 6. Mở nâng cao của Workload khi cần

Bấm **Hiện nâng cao Workload**, ở ngoài các khối container.

### Service & kết nối / Service & networking

Dùng Service để workload khác trong cùng môi trường truy cập ứng dụng:

1. Chọn **Bật / Enabled** nếu muốn bật rõ ràng; có thể giữ kế thừa nếu template
   đã cung cấp Service phù hợp.
2. Bấm **+ Thêm cổng / Add port**.
3. Điền tên cổng, cổng Service và chọn protocol phù hợp.
4. Mở **Hiện nâng cao** của chính dòng cổng đó để nhập **Cổng đích của Container**
   (target port). Nếu để trống, target port lấy cùng số với cổng Service.

Ví dụ ứng dụng lắng nghe ở cổng `8080`:

| Trường | Giá trị |
|---|---|
| Tên cổng | `http` |
| Cổng Service | `80` |
| Target port | `8080` |
| Protocol | `TCP` |

Client gọi Service ở cổng 80; Service chuyển tới tiến trình ở cổng 8080 của Pod.
Cổng container và target port cần khớp với ứng dụng thực sự đang lắng nghe.
Chỉ chọn Enabled mà chưa có cổng hợp lệ chưa tạo được một kết nối sử dụng được.
Service trên trang này phục vụ kết nối nội bộ, không tự cấp URL public/Internet.

Chọn **Tắt / Disabled** để tắt Service, kể cả Service kế thừa từ template.
Headless chỉ dùng khi ứng dụng cần; nó không phải tùy chọn để công khai web.

### Tài nguyên phụ thuộc

Thêm database, cache, storage hoặc resource mà ứng dụng cần. Chọn nguồn có sẵn
phù hợp để dùng lại; chỉ tạo resource mới khi cần. Sau đó có thể dùng output của
resource trong biến môi trường. Lưu draft chưa tạo tài nguyên hạ tầng; Deploy mới
thực hiện thay đổi đã được Preview, trừ Variable/Secret tạo riêng như mục 4.

### Mở rộng & cập nhật / Scaling & updates

- **Số replica:** số Pod mong muốn cho Deployment, không phải số container trong Pod.
- **Chiến lược cập nhật:** giữ default nếu không có yêu cầu riêng. Deployment có
  persistent mount phải tuân thủ chiến lược an toàn Recreate; StatefulSet có các
  tùy chọn riêng và hiện chỉ hỗ trợ 1 replica.
- **revisionHistoryLimit:** số revision lịch sử cần giữ; nhập `0` là đặt rõ giá trị 0.
- **PodDisruptionBudget:** chỉ bật khi cần kiểm soát eviction tự nguyện. Cần
  Deployment stateless có ít nhất **2 replica**, không dùng persistent mount.
  Chọn một trong `minAvailable` hoặc `maxUnavailable`, rồi dùng default hoặc nhập
  ngân sách riêng. PDB không thay thế chiến lược rollout và không bảo đảm không gián đoạn.

### Bảo mật Pod và cấu hình ít thay đổi

Bảo mật Pod áp dụng ở cấp Pod: token, UID/GID, fsGroup, seccomp. Bảo mật container
áp dụng cho từng container. ServiceAccount do nền tảng/secret delivery quản lý;
giữ cấu hình được phép. Vault Agent có thể cần token, nên không chọn false khi
xung đột với cơ chế xác thực đang dùng.

**Cấu hình ít thay đổi** chứa kiểu thực thi, template inputs nếu có, workload
class và annotations. Class ảnh hưởng cách nền tảng chọn cấu hình resource.
Annotations workload không mặc nhiên được chép sang Pod. Không có dropdown chọn
backend defaults policy.

## 7. Gắn volume hoặc file

Trong **Container → Hiện nâng cao → Volumes & files**:

1. Bấm thêm volume/file, tìm và chọn nguồn phù hợp có sẵn trước.
2. Nếu chưa có, chọn tạo nguồn mới: storage/PVC, emptyDir, ConfigMap, hostPath hoặc
   Secret file theo quyền/capability đang có.
3. Điền **mount path** trong container, ví dụ `/data` hoặc `/etc/myapp/app.conf`.
4. Điền **subPath** nếu chỉ mount một phần của nguồn; đây là đường dẫn tương đối
   trong nguồn. Để trống nếu mount toàn bộ.
5. Chọn quyền đọc/ghi phù hợp rồi xác nhận.

| Nguồn | Phạm vi và thời gian giữ dữ liệu |
|---|---|
| emptyDir | Tạm trong một Pod; container trong cùng Pod có thể chia sẻ, dữ liệu mất khi Pod bị xóa |
| Storage/PVC | Dữ liệu lưu bền; có thể dùng lại resource phù hợp. Gỡ mount không xóa PVC |
| ConfigMap | File cấu hình không bí mật, mount chỉ đọc |
| Secret file | File bí mật tham chiếu Secret đã cấu hình; chế độ mặc định 0400 |
| hostPath | Đường dẫn trên node chạy Pod, không phải trên máy đang mở Console |

Gỡ mount chỉ gỡ liên kết. Sửa nguồn được dùng chung có thể ảnh hưởng các mount
đang dùng nguồn đó. Khi đổi tên hoặc xóa container, kiểm tra các mount/tham chiếu
liên quan; form không tự chuyển chúng sang container khác.

Với **StatefulSet → Lưu trữ StatefulSet**, claim template tạo claim theo Pod,
khác với chọn PVC resource có sẵn. Khai báo tên, dung lượng, access mode,
StorageClass nếu cần và mount vào container. Claim giữ dữ liệu khi xóa workload
hoặc scale down; cleanup ứng dụng có thể bị chặn cho tới khi storage được xử lý
riêng. Không tự adopt claim trùng tên. Cấu hình claim đã áp dụng không được sửa
như volume tạm.

## 8. Cấu hình tác vụ chạy theo lịch

Chọn template CronJob; phần **Lịch chạy** hiện ngoài nâng cao:

- Nhập lịch cron năm trường, ví dụ `15 3 * * *`: hằng ngày lúc 03:15 theo múi giờ đã chọn.
- Giữ múi giờ mờ nếu phù hợp; template scheduled chuẩn hiện dùng `Asia/Bangkok`.
- Mở **Chính sách Job** để chọn cách xử lý khi lần trước chưa xong:
  `Forbid` bỏ qua lần mới, `Allow` cho chạy đồng thời, `Replace` thay lần trước.
- Cấu hình retry, deadline, lịch sử Job hoặc tạm dừng lịch nếu cần.

Template không tự đặt biểu thức cron cho bạn. Không khai báo Service/cổng phục vụ
web cho CronJob. Workload chạy theo lịch dùng lịch thay cho số replica.

## 9. Lưu draft, Preview và Deploy

1. Bấm **Lưu draft Workload / Save Workload draft**.
2. Sau khi quay lại ứng dụng, bấm **Xem trước thay đổi / Preview changes**.
3. Kiểm tra giá trị hiệu lực, cổng/Service, quyền Pod, tài nguyên cần tạo/dùng lại,
   storage và những thay đổi sắp áp dụng.
4. Khi đúng, bấm **Triển khai các thay đổi này** để Deploy.
5. Theo dõi trạng thái triển khai và trạng thái workload; với CronJob, xem thêm
   lịch sử Job để biết kết quả từng lần chạy.

**Lưu draft chỉ lưu cấu hình mong muốn.** Preview không áp dụng tài nguyên.
Deploy mới thay đổi môi trường chạy. Nếu cấu hình/pin đã đổi sau Preview, cần
Preview lại theo yêu cầu của hệ thống.

Tab **Nhập Workload** nhận Generic Workload YAML/JSON được nền tảng hỗ trợ;
không nhận nguyên manifest Kubernetes Deployment/Service tùy ý. Sau khi phân tích,
kiểm tra lại cấu hình rồi lưu draft và thực hiện cùng luồng Preview → Deploy.

## 10. Ví dụ: thêm một web Nginx đơn giản

Ví dụ để tập điền form, chưa cần database, Secret, volume hay PDB:

1. Chọn **staging → Thêm Workload → Backend API**.
2. Đặt tên `demo-web`, giữ container `main`.
3. Nhập image `nginxinc/nginx-unprivileged:1.27-alpine`.
4. Giữ CPU/RAM và các default phù hợp; không cần điền lại giá trị mờ.
5. Trong nâng cao của container, thêm cổng tên `http`, số `8080`, protocol `TCP`.
6. Trong nâng cao Workload, cấu hình Service `http`, port `80`, target port `8080`,
   protocol `TCP` như mục 6.
7. Nếu muốn probe, bật Readiness HTTP với path `/`, port `8080`.
8. Giữ 1 replica và không bật PDB.
9. Lưu draft → Preview → kiểm tra → Deploy khi môi trường đã sẵn sàng.

Ví dụ dùng Backend API nên bạn tự khai báo image/cổng. Template **Nginx chỉ đọc**
có profile khác, cung cấp image/cổng/readiness và các mount cần thiết sẵn; nếu chọn
mẫu đó, xem và kế thừa cấu hình của nó thay vì nhập lại mọi trường.

## 11. Xử lý lỗi thường gặp

| Hiện tượng | Cách xử lý |
|---|---|
| Yêu cầu nhập image | Điền image cho các App/Init/Sidecar cần có; Backend API không có image mặc định |
| Không tải được template | Thử lại; kiểm tra Catalog/source và phiên bản được nền tảng công bố cho môi trường |
| Effective configuration is unavailable or invalid | Giữ form; kiểm tra trường vừa sửa, đọc thông báo rồi sửa và chờ tính lại. Thử lại nếu lỗi tải dữ liệu. Kiểm tra các trường hợp PDB/request-limit bên dưới |
| Bật PDB bị báo cần ít nhất 2 replica | Đặt ít nhất 2 replica cho Deployment stateless phù hợp, hoặc tắt/khôi phục PDB. Không bật PDB cho workload đang dùng persistent mount |
| CPU/memory không hợp lệ | Dùng đúng đơn vị; so cả request đã nhập/kế thừa với limit đã nhập/kế thừa. Ví dụ request `768Mi` xung đột limit mờ `512Mi` |
| Service chưa truy cập được | Kiểm tra ứng dụng thực sự lắng nghe, container/target port, Service ports, readiness và kết quả Deploy; Enabled không thay thế khai báo cổng |
| Không lưu được do phiên bản đã thay đổi | Làm theo thao tác tải lại/xem lại cấu hình mới; đừng bấm lưu lặp để ghi đè thay đổi của người khác |
| Pod không chạy sau Deploy | Xem trạng thái/log ứng dụng, image/command, probes, đường dẫn ghi và security; không suy từ việc lưu draft thành công rằng image đã chạy được |

## 12. Quản lý default qua Git — dành cho người quản trị template

Người dùng ghi đè một workload bằng form. Muốn đổi default của template cho các
workload dùng phiên bản mới, sửa **`runtimeDefaults`** trong `bundle.yaml` ở repo
Git của template. `defaults` là default của template inputs, tách khỏi runtimeDefaults.

Với bundle chuẩn trong repository này, thư mục là `backend/template-sources/standard`:

```yaml
# Một phần của mục template backend trong workloadTemplates.
- id: backend
  version: 1.1.1
  # Giữ kind, labels, inputsSchema, defaults và các trường khác của mẫu.
  runtimeDefaults:
    resources:
      requests: {cpu: 250m, memory: 256Mi}
      limits: {memory: 768Mi}
    environments:
      production:
        resources:
          requests: {cpu: 500m, memory: 512Mi}
```

Đoạn trên minh họa các trường cần sửa, không phải một bundle hoàn chỉnh để tải lên.
Giữ các giá trị runtime khác cần thiết; environment override được ghép với profile chung.

Quy trình cập nhật:

1. Sửa template, tăng version và commit/push lên Git.
2. Trong **Cấu hình tài nguyên**, mở workload Definition liên quan → **Đổi nguồn → Git**.
3. Điền repository HTTPS URL, thư mục template và tag/commit mới; kiểm tra template.
4. Lưu Definition để tạo phiên bản mới; trong **Cài đặt môi trường → Phiên bản tài nguyên**,
   chủ động chọn revision Definition cần dùng.
5. Với workload mới, chọn mẫu/version mới từ danh sách. Workload hiện có giữ pin
   cũ; nếu cần đổi pin, cập nhật Generic Workload qua luồng nhập hợp lệ bằng
   metadata đã kiểm tra, rồi xem lại default và Preview trước Deploy. Trang sửa
   hiện không có nút tự nâng template. Không đoán hoặc tự tạo version/hash.

Hệ thống ghim commit/hash đã kiểm tra. Push lên nhánh Git không tự đổi workload
đang dùng pin cũ; mở lại hoặc lưu nguyên form cũng không tự nâng default. Source
cần công bố capability tương ứng để các trường nâng cao hoạt động. Thay đổi nguồn
render và thay đổi pin template đều cần được đối chiếu, không coi là một lần tự
nâng tất cả workload.

Nguồn chuẩn: [UC-16](../usecase/UC-16/README.md),
[contract default](../architecture/contracts/workload-defaults.md),
[contract Generic Workload](../architecture/contracts/generic-workload.md).
