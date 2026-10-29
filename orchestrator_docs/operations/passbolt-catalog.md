---
id: PASSBOLT-CATALOG-RUNBOOK
artifact: operations-runbook
status: draft
last_reviewed: 2026-10-28
---

# Deploy Passbolt mới qua workload catalog

**Runbook chuẩn bị, chưa executable trên product hiện tại.** Xem
[implementation gates](../implementation/workload-catalog.md). Không migration,
không adopt namespace/PVC hiện hữu, không dọn MySQL cũ. Thực thi sau khi feature
được nghiệm thu và có authorization target cụ thể.

## Deployment inventory

| Component | Owner | Configuration cần xác minh |
|---|---|---|
| MySQL | Terraform resource | Image/digest tương thích Passbolt, 1 replica, data PVC 50Gi, headless Service 3306 |
| Public images | Terraform storage resource | PVC 2Gi, StorageClass/access mode; mount path từ image |
| Passbolt | Workload StatefulSet template | 1 replica, ClusterIP Service 80 → container 80; governing Service theo template; route URL/TLS |
| Backup destination | S3-compatible resource/binding | Dedicated bucket/prefix, endpoint/region/TLS; explicit create-new hoặc existing binding, ownership không suy từ tên |
| Backup | Scheduled workload | 00:00 theo timezone chốt, mysqldump + upload, DB/S3 secret refs |
| Cleanup | Scheduled workload | 01:00 cùng timezone, prefix-scoped list/delete, retention >7 ngày khi tổng >=10 |

S3 không mount như PVC. Job dùng API/client tương thích S3. Credentials reference
qua platform secret store. Existing bucket không được platform destroy; chỉ
managed prefix/run-owned objects được cleanup theo quyền được cấp. New bucket
creation cần Definition/provider hỗ trợ endpoint đó; không giả định AWS module
chạy được với mọi S3-compatible server.

## Input checklist (không gửi secret plaintext)

- Image digest + Dockerfile/entrypoint Passbolt và DB/backup image; phiên bản app,
  DB compatibility matrix do chủ ứng dụng xác nhận. Không tự nâng MySQL 5.6 trong
  lần triển khai, cũng không coi phiên bản đó được chấp thuận production mới.
- Manifest đã bỏ secret: paths, UID/GID/fsGroup, GPG import/bootstrap, init schema,
  expected DB env names, mail config. Database image không nhất thiết đọc cùng
  env variables như image chuẩn; không đoán MYSQL_PASSWORD là đủ.
- Registry credential reference, cluster/account, Environment/namespace mới,
  StorageClass/capabilities, public host/route/TLS do platform hỗ trợ.
- S3 endpoint/region/bucket/prefix, CA/TLS configuration, authentication reference;
  backup và cleanup quyền tối thiểu theo prefix. Timezone 00:00/01:00 phải xác nhận.
- Fresh GPG server key tạo theo quy trình app, lưu secret store; không tạo lại mỗi
  Pod restart. Không đưa private key vào repository hoặc Terraform state.

## Backup/cleanup semantics

Backup: dump nhất quán theo storage engine hỗ trợ; mysqldump --single-transaction
chỉ dùng khi phù hợp tables/engine. Dùng pipefail hoặc kiểm exit từng stage để
upload success không che dump failure. Tên object unique UTC timestamp + run ID,
không overwrite. Chỉ công bố completed object sau dump/upload kiểm chứng thành
công; cleanup không xét multipart/incomplete upload. Log chỉ metadata, không SQL
hoặc credential. Ưu tiên stream, hoặc emptyDir tạm với budget; không hostPath.

Chốt S3 destination không đồng nghĩa chốt tự động bucket lifecycle. Giữ CronJob
cleanup để thực hiện count-aware policy. Trước implementation đối chiếu script
cũ: count tính một lần hay tính lại sau mỗi delete; có giữ tối thiểu bao nhiêu bản?
Thông tin “>7 ngày khi tổng >=10” chưa đủ để suy “luôn giữ 10 bản”. Review chính
sách này bắt buộc trước bật deletion. Trong lúc chưa chốt chỉ dry-run/report,
không xóa. Listing phải paginate; chỉ prefix và completed backups, xử lý timezone
UTC và boundary 7 ngày. Concurrency Forbid chỉ chống overlap cùng CronJob, không
khóa chéo backup/cleanup; cleanup loại upload chưa hoàn tất. Delete failure báo
Job failed, không che bằng exit 0.

## Prepare → deploy

1. Xác nhận gates contract/code/tests; namespace mới và account ownership; ghi
   run ID/evidence path ngoài Git. Kiểm cluster storage/registry/route/VSO và S3
   connectivity/TLS/quota; không mutate hệ thống cũ.
2. Publish immutable workload templates và Terraform Definitions theo contract.
   Tạo Application/Environment mới, chọn secret store/account theo cơ chế hiện tại.
3. Tạo secret bindings DB/mail/GPG/S3 qua Console; kiểm file-mode/entrypoint.
4. Khai báo MySQL + image storage + S3 binding. Tạo Passbolt từ stateful template,
   map DB outputs và secrets; tạo 2 scheduled workloads độc lập tham chiếu DB/S3.
   Ban đầu suspend schedules để tránh chạy trước initialization; cleanup chưa
   bật deletion nếu policy chưa được xác nhận.
5. Save → Preview: review image pins, namespace, MySQL/PVC create, object storage
   ownership, services/routes, cron fields, secret refs không bytes. Không có
   resource của namespace cũ trong plan.
6. Deploy: DB initialized/Ready, Passbolt init schema/key setup theo image,
   rollout Ready, URL truy cập được. Không tự chạy migration command cũ trên DB mới
   nếu chưa xác định bootstrap app hỗ trợ.
7. Tạo tài khoản mới, đăng nhập, tạo/đọc mật khẩu thử, upload ảnh; restart/update
   giữ DB, GPG identity và ảnh. Mail flow theo SMTP đã cấp, không tự gửi đến người ngoài.
8. Chạy backup một lần qua operation đã implement/ủy quyền, kiểm object/checksum;
   restore vào DB disposable riêng để chứng minh bản dump dùng được. Không restore
   đè DB đang nghiệm thu. Kiểm cleanup với objects thử trong prefix cô lập và policy
   đã chốt. Sau đó unsuspend lịch qua Preview/Deploy, xác nhận lịch timezone.

## Evidence và cleanup

Ghi deployment IDs, pinned versions, resource UID/namespace, PVC identity trước/
sau restart, Job result, object metadata và restore assertions; không ghi secret,
SQL dump, GPG key. Schedule installed không tính là backup pass.

Remove workload chỉ gỡ compute/cron theo active-job policy, giữ storage. Nếu môi
trường demo cần xóa dữ liệu: explicit resource deletion Preview/confirm, kiểm state/
UID; S3 chỉ xóa run-owned objects/prefix đã được cấp quyền, không xóa bucket dùng
chung. Namespace/Environment chỉ xóa sau retained resources đã xử lý. Cleanup
system cũ nằm ngoài scope. Không coi runbook tồn tại là quyền chạy cluster/cloud.
