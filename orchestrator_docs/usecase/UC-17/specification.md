---
id: UC-17-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-23
related: UC-06, UC-07, UC-09, issue #12
---

# UC-17 — Theo dõi và báo cáo hiệu quả platform

## Mục tiêu

Cho Platform Engineer/Admin xem số liệu có căn cứ về mức sử dụng platform và
hiệu quả giao phần mềm của Organization. Số liệu tính từ lịch sử thực thi thật
của platform. Chỉ số cần nguồn ngoài platform được công bố là chưa thu thập,
không thay bằng proxy.

Phạm vi đợt 1 do người dùng chốt ngày 2026-10-23 trong
[issue #12](https://github.com/pr3s3nt/poc/issues/12#issuecomment-6361990505):
chỉ các chỉ số platform tự đo; danh mục chỉ số phải mở rộng được.

## Actors

- **Primary:** Platform Engineer, Organization Administrator.
- **Không có quyền:** Developer (không xem, không sửa).

## Tiền điều kiện

- **PRE-01:** Actor đã đăng nhập (UC-00) với role `PLATFORM_ENGINEER` hoặc `ADMIN`.
- **PRE-02:** Organization của session là phạm vi báo cáo duy nhất.

## Trigger

**TRG-01:** Actor mở “Hiệu quả platform”, đổi kỳ/phạm vi, mở chi tiết chỉ số,
xuất CSV hoặc lưu baseline.

## Main success scenario

1. **MS-01:** Actor mở báo cáo; mặc định phạm vi **Toàn tổ chức**, kỳ 30 ngày
   gần nhất theo timezone báo cáo.
2. **MS-02:** Orchestrator đọc Deployment, cleanup tài nguyên, Application và
   User Account thuộc Organization; không gọi provider, cluster hay executor.
3. **MS-03:** Orchestrator dựng công việc logic từ các lượt chạy theo
   [hợp đồng đo lường](#hợp-đồng-đo-lường).
4. **MS-04:** Orchestrator tính từng chỉ số trong danh mục, kèm tử số/mẫu số,
   độ phủ và trạng thái thu thập; chỉ số chưa có nguồn trả `NOT_COLLECTED`.
5. **MS-05:** Orchestrator trả báo cáo gồm tổng quan phạm vi, phân bố theo phạm
   vi con (Application ở cấp tổ chức, Workload ở cấp Application) và xu hướng.
6. **MS-06:** Actor chọn Application, rồi Workload, để xem sâu; cùng hợp đồng
   tính lại trên phạm vi mới.
7. **MS-07:** Actor mở chi tiết một chỉ số: cách tính, phạm vi, độ phủ và các
   bản ghi nguồn; mỗi bản ghi dẫn tới lịch sử triển khai hiện có (UC-09).
8. **MS-08:** Actor xuất CSV của phạm vi/kỳ đang xem.

## Luồng thay thế

- **ALT-01 Baseline:** Actor thêm phiên bản baseline cho thời gian thực hiện
  của phạm vi Toàn tổ chức hoặc một Application, với giá trị, nguồn, ngày áp
  dụng và tuỳ chọn số mẫu/kỳ đo. Phiên bản cũ giữ nguyên. Ngừng áp dụng cũng
  là một phiên bản mới.
- **ALT-02 Không có dữ liệu:** phạm vi/kỳ không có bản ghi thì chỉ số hiển thị
  “Chưa có dữ liệu”, không hiển thị 0 cho chỉ số thời gian/tỷ lệ.
- **ALT-03 Bản cài đặt không phải production:** báo cáo vẫn tính nhưng gắn
  nhãn dữ liệu kiểm thử (BR-11).

## Ngoại lệ

- **EX-01:** Chưa đăng nhập → `UNAUTHORIZED`; Developer → `FORBIDDEN`.
- **EX-02:** Application/Workload không thuộc Organization → `NOT_FOUND`.
- **EX-03:** Kỳ sai định dạng, `from > to`, dài hơn 366 ngày, hoặc Workload
  không kèm Application → `INVALID_REQUEST` với field tương ứng.
- **EX-04:** Baseline không hợp lệ → `INVALID_REQUEST` với field; phiên bản
  baseline đã đổi từ lúc đọc → `STALE_VERSION`, không ghi gì.

## Hậu điều kiện

- **POST-01:** Xem báo cáo/chi tiết/CSV không đổi bất kỳ trạng thái nào.
- **POST-02:** Lưu baseline chỉ thêm đúng một phiên bản mới; không sửa phiên
  bản cũ hoặc lịch sử thực thi.
- **POST-03:** Không có secret, provider input, resolved input, output hoặc
  failure reason thô trong response/CSV.

## Quy tắc nghiệp vụ

- **BR-01 Tenant:** mọi đọc/ghi chỉ trong Organization của session; không gộp
  dữ liệu giữa Organization.
- **BR-02 Nguồn thật:** chỉ dùng lịch sử thực thi đã persist; không pipeline
  giả, không tính trong trình duyệt, không đưa fixture UI vào báo cáo.
- **BR-03 Không proxy ẩn:** chỉ số thiếu nguồn trả `NOT_COLLECTED` kèm nguồn
  cần có; pipeline `FAILED` không thay change fail rate, thời gian chạy không
  thay lead time hay giờ công.
- **BR-04 Lượt chạy và công việc logic:** retry không thành nhiều công việc
  hoàn thành; số lượt chạy giữ riêng để phản ánh công sức xử lý.
- **BR-05 Người dùng duy nhất:** đếm User Account duy nhất trong phạm vi/kỳ,
  không cộng các phạm vi con.
- **BR-06 Tỷ lệ:** tỷ lệ tổng hợp tính lại từ tử số/mẫu số; không trung bình
  phần trăm của phạm vi con.
- **BR-07 Thời gian:** công bố cách tổng hợp (trung vị, p90) và độ phủ; không
  cộng/trung bình các trung vị con.
- **BR-08 Không “điểm DORA” tổ chức:** cấp tổ chức dùng bảng/phân bố/xu hướng
  theo Application.
- **BR-09 Baseline có phiên bản:** baseline lưu nguồn, phạm vi, giá trị, ngày áp
  dụng, người tạo; báo cáo nêu phiên bản đã dùng. Phiên bản mới không viết lại
  ý nghĩa kỳ trước ngày áp dụng của nó.
- **BR-10 Thời gian quy trình ≠ giờ công:** chênh lệch với baseline là thời
  gian quy trình, giữ cả giá trị âm; không quy ra giờ công/tiền.
- **BR-11 Production:** chỉ số DORA chỉ dùng Environment `production`. Bản cài
  đặt chạy seed profile khác `production` gắn `dataOrigin=TEST_INSTALLATION`
  cho toàn báo cáo/CSV. Cleanup `RUN_CLEANUP` của harness kiểm thử bị loại.
- **BR-12 Truy vết:** mọi số tổng hợp truy được về bản ghi và phương pháp tính.

## Hợp đồng đo lường

### Kỳ, timezone và lưu giữ

- Timestamp lưu UTC. Ranh giới ngày theo timezone báo cáo `Asia/Ho_Chi_Minh`.
- Kỳ là khoảng ngày `[from, to]` gồm cả hai đầu, mặc định 30 ngày kết thúc hôm
  nay; tối đa 366 ngày. Xu hướng theo ngày khi kỳ ≤ 14 ngày, ngược lại theo
  tuần bắt đầu thứ Hai.
- Lưu giữ toàn bộ lịch sử thực thi; đợt 1 không purge. Báo cáo đọc toàn bộ lịch
  sử của Organization để dựng chuỗi retry vượt ranh giới kỳ.

### Bản ghi nguồn

| Nguồn | Bản ghi | Trường dùng |
|---|---|---|
| Deployment (UC-06/07) | Một lượt chạy `DEPLOY`/`UPDATE`/`REMOVE` của một workload | Application, Environment, workload, action, actor, status, `started_at`, `finished_at` |
| Resource cleanup (UC-07 DEL-*) | Một operation `RESOURCE_CLEANUP` mode `USER_CONFIRMED` | Application, Environment, root resource, actor, status, `started_at`, `finished_at` |
| User Account | Danh tính người thực hiện | Organization, username |
| Baseline (ALT-01) | Một phiên bản baseline | Xem BR-09 |

### Công việc logic

- **Lượt chạy** là một bản ghi nguồn. Trạng thái: thành công (`SUCCEEDED`),
  thất bại (`FAILED`, cleanup lỗi/cần xử lý), đang chạy (còn lại).
- **Khóa chuỗi:** Deployment theo `(Environment, workload, action)`; cleanup
  theo `(Environment, root resource)`.
- Sắp các lượt cùng khóa theo `started_at`. Một **công việc logic** gồm các lượt
  liên tiếp kết thúc ở lượt thành công đầu tiên. Lượt tiếp theo mở công việc
  mới. Đổi action mở chuỗi khác.
- Công việc **hoàn thành** khi có lượt thành công; thuộc kỳ chứa `finished_at`
  của lượt đó. Công việc **đang mở** khi chưa có lượt thành công: *đang chạy*
  nếu lượt cuối đang chạy, ngược lại *thất bại chưa xử lý*.
- **Thời gian thực hiện** = `finished_at` lượt thành công − `started_at` lượt
  đầu. Không phải thời gian yêu cầu → hoàn thành và không phải giờ công.
- **Số lần thử** của công việc = số lượt trong chuỗi.

### Danh mục chỉ số

`status`: `COLLECTED` (có giá trị), `NO_DATA` (có nguồn, kỳ không có bản ghi đủ
điều kiện), `NOT_COLLECTED` (chưa có nguồn).

| ID | Nhóm | Đơn vị | Công thức | Tử số / mẫu số / độ phủ | Thiếu dữ liệu |
|---|---|---|---|---|---|
| `deployment_frequency` | DORA | lượt triển khai / kỳ; kèm lượt/tuần | Số công việc Deployment `DEPLOY`/`UPDATE` hoàn thành trên `production` trong kỳ | Tử số: số công việc; mẫu số: số ngày của kỳ (để ra lượt/tuần); phân bố Application theo nhóm tần suất: hằng ngày (≥1/ngày), hằng tuần (≥1/tuần), hằng tháng (≥1/30 ngày), thưa hơn, không có | 0 lượt là giá trị thật khi có nguồn; Application không có lượt nằm nhóm “không có” |
| `change_lead_time` | DORA | giờ | Commit → deployment production thành công | — | `NOT_COLLECTED`: cần liên kết deployment → commit và thời điểm commit |
| `change_fail_rate` | DORA | % | Deployment production cần can thiệp ngay / tổng deployment production | — | `NOT_COLLECTED`: cần dữ liệu sự cố |
| `failed_deployment_recovery_time` | DORA | giờ | Phát hiện → khôi phục sự cố do deployment | — | `NOT_COLLECTED`: cần mốc phát hiện/khôi phục |
| `deployment_rework_rate` | DORA | % | Deployment khắc phục ngoài kế hoạch / tổng deployment production | — | `NOT_COLLECTED`: cần phân loại deployment khắc phục |
| `active_applications` | Mức sử dụng | ứng dụng | Application có ≥1 công việc hoàn thành ở bất kỳ Environment trong kỳ | Tử số: Application hoạt động; mẫu số: Application hiện có của Organization | Không có Application → `NO_DATA` |
| `active_users` | Mức sử dụng | người | User Account duy nhất là actor của ≥1 lượt chạy trong kỳ | Kèm số lượt có actor không ánh xạ được User Account (tự động/CI), không cộng vào số người | Không có lượt → `NO_DATA` |
| `completed_work` | Mức sử dụng | công việc | Số công việc logic hoàn thành trong kỳ, theo loại và Environment | Kèm số công việc đang mở cuối kỳ | Không có → 0 khi có nguồn |
| `execution_attempts` | Mức sử dụng | lượt; lần/công việc | Lượt chạy bắt đầu trong kỳ; lượt thất bại; lần thử trung bình = tổng lượt của công việc hoàn thành / số công việc hoàn thành | Tử số/mẫu số công bố riêng | Không có công việc hoàn thành → lần thử `NO_DATA` |
| `work_duration` | Tự phục vụ | phút/giờ | Trung vị và p90 thời gian thực hiện của công việc hoàn thành trong kỳ | Độ phủ: công việc có đủ hai timestamp / công việc hoàn thành | Không có → `NO_DATA` |
| `work_duration_vs_baseline` | Lợi ích | phút/giờ | Baseline − trung vị `work_duration` của cùng phạm vi; dương là nhanh hơn | Nêu phiên bản baseline, nguồn, ngày áp dụng | Thiếu baseline hoặc `work_duration` → `NO_DATA` |
| `request_lead_time` | Tự phục vụ | giờ | Yêu cầu → hoàn thành | — | `NOT_COLLECTED`: cần thời điểm yêu cầu |
| `self_service_rate` | Tự phục vụ | % | Công việc không cần hỗ trợ / công việc đã ghi nhận hỗ trợ | — | `NOT_COLLECTED`: cần bản ghi hỗ trợ; thiếu phản hồi ≠ tự phục vụ |
| `ops_effort` | Tự phục vụ | giờ công | Công sức Ops hỗ trợ/bảo trì | — | `NOT_COLLECTED`: cần bản ghi công sức |
| `onboarding_time` | Tự phục vụ | ngày | Bắt đầu onboarding → deployment production thành công đầu tiên | — | `NOT_COLLECTED`: cần mốc bắt đầu |
| `labor_hours_saved` | Lợi ích | giờ công | Baseline công sức − công sức qua platform tương đương | — | `NOT_COLLECTED`: cần cả hai dữ liệu công sức; không tuyên bố ROI tiền |

### Phạm vi và tổng hợp

- **Toàn tổ chức:** mọi Application của Organization. Phân bố theo Application.
- **Application:** một Application. Phân bố theo Workload. Cleanup không gắn
  Workload nên chỉ tính ở cấp Application trở lên.
- **Workload:** một Workload của một Application; chỉ nguồn Deployment.
- Số đếm cấp cha bằng tổng phạm vi con chỉ với số đếm cộng được (công việc,
  lượt). Người dùng, tỷ lệ và thời gian luôn tính lại từ bản ghi của phạm vi cha.

### Mở rộng danh mục

Thêm chỉ số mới bằng một mục danh mục có ID ổn định, nhóm, đơn vị, nguồn và bộ
tính. Chỉ số chưa có nguồn vẫn có mục với `NOT_COLLECTED`. Thêm nguồn ngoài
platform (commit, sự cố, hỗ trợ) phải cập nhật specification, schema và hợp
đồng này trước; không đổi ý nghĩa ID đã công bố.

## Ngoài phạm vi

- **OOS-01:** Tích hợp Git/CI, incident, ticket, nhập tay sự cố/công sức.
- **OOS-02:** Nhóm/team, RBAC mới, báo cáo cho Developer.
- **OOS-03:** Quy đổi tiền/ROI; giờ công tiết kiệm.
- **OOS-04:** Purge/retention policy, báo cáo đa Organization.
- **OOS-05:** Giao diện mobile; Console dùng trên desktop web.
- **OOS-06:** Thay đổi lifecycle deploy/update/cleanup.
