---
id: VERIFY-K8S4F-DEV-RETAINED-20261009
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-09
---

# Docker IDP → cụm K8S-4F mới, ứng dụng giữ lại

## Outcome và môi trường

Người dùng cho phép tạo cụm mới, đăng ký Connection mới tên `K8S-4F`, triển khai
ứng dụng mẫu FE/BE/PostgreSQL, quay màn hình và upload GitHub. Cluster kind
`k8s-4f`/context `kind-k8s-4f` có node `k8s-4f-control-plane` Ready,
Kubernetes v1.36.1. Current host context đã trả lại `kind-idp-internal`.
Connection cũ `k8s-4f` không đổi; Connection mới có key tự cấp `k8s-4f-2`.

Lượt Deploy dùng **Console Docker lâu dài**, không phải backend test riêng.
Kubernetes executors, PostgreSQL resource, credential Vault, workload Vault và
VSO là adapter/service thật. Secret Store mới `K8S-4F Vault` có key
`k8s-4f-vault`, endpoint nội bộ `http://k8s-4f-workload-vault:8200`.
VSO 1.5.1 và Kubernetes auth reviewer được chuẩn bị riêng trên cụm mới.

Application **k8s4f-dev-final** (`3c5e4c33-b724-4032-80b1-d270e5c32a35`), staging
namespace `app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging`, được giữ lại.
Deployment backend/frontend Ready 1/1, PostgreSQL StatefulSet Ready 1/1 và PVC
1 GiB Bound. Image `acceptance-{backend,frontend}:k8s4f-dev` được build/load vào
cụm mới. Production không triển khai; không có worker.

## Assertions và recording

Platform Engineer đăng ký Connection/Store bằng UI. Developer tạo Application,
chọn staging Connection/Store, nhập cấu hình, tạo BE với resource PostgreSQL và
FE với Service binding, Preview và Deploy bằng UI. API reads và kubectl chỉ
quan sát/assert kết quả. Bốn kiểm tra backend connection/environment/secret/
database PASS, job được nhận PENDING; reload giữ job và execution binding.
Assertion so sánh bản ghi Connection cũ và các bản ghi store có sẵn không đổi.
Codex kiểm tra độc lập readiness/PVC và endpoint checks đều true.

Lượt 1 đăng ký thành công nhưng dừng trước khi lưu execution target do thao tác
native select chọn nhầm option. Không Deploy lên đích sai. Lượt 2 deploy và
diagnostics PASS nhưng assertion cuối đếm trùng store trong reuse mode bị lỗi.
Lượt 3 tạo Application mới, dùng lại Connection/Store và hoàn tất PASS.
Ứng dụng lượt 2 `k8s4f-dev`/ID `a43bb834-d2d7-42cd-a035-41841155b5f7` cũng giữ lại,
không xóa dữ liệu chỉ để chạy lại recording.

[Video đã review và upload GitHub](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/k8s4f-dev-full-20261009-1523.mp4)
ghép đoạn đăng ký của lượt 1 với phiên thành công lượt 3; title cards ghi rõ
hai phiên riêng biệt. Đây không phải recording liền mạch một execution.
Headed Chromium full-window có address bar, cursor, click, typing và pause;
token/kubeconfig/secret không hiển thị. Phụ đề tiếng Việt nằm trong strip bên
dưới UI sau khi sửa lỗi scaling libass phát hiện khi review.
H.264 1440×996, 15 fps, 499.2 giây; full decode pass, representative frames đã
review. GitHub asset size 4,878,268 bytes, tên mới không thay asset cũ.

Evidence local ngoài Git tại `/tmp/poc-k8s4f-dev-20261009/`:
`evidence-run1-failed/`, `evidence-run2-partial/`, `evidence-run3/`, `final/`.
Các lượt thất bại giữ raw video/captions, phase marks và assertions.
Video/credential/generated binaries không commit vào repository.

## Handoff và giới hạn

Checks: frontend typecheck/lint/154 tests/build exit 0 (gate logs ngoài Git);
`bash -n` năm shell scripts, `node --check` ba recording scripts, documentation
checker và diff whitespace check pass. Không sửa Go product code nên không chạy
Go test/build. Claude kiểm tra Vault setup idempotent, foreign container rejection,
token giữ nguyên sau recreation, Docker restart → healthy/unsealed và cleanup
credential/probe Pod. Reuse identity preflight bổ sung sau lượt 3 được kiểm tra
read-only với Console, không replay full Deploy sau patch này.

[Runbook](../operations/k8s4f-dev.md) ghi URL, ID/namespace, khởi động Compose
với overlay kind và đường mở lại frontend. Port-forward tại localhost:18480
phục vụ bản cuối, cần mở lại sau khi máy restart. Workloads/PVC, Connection,
stores, Vault volumes và VSO được giữ cho dev; không cleanup các tài nguyên này.
Không mutate AWS hoặc workload của cụm cũ.

Vault riêng renew periodic token và unseal tự động khi container restart.
Nếu offline quá 768 giờ/token bị revoke, token mới không tự cập nhật credential
đã đăng ký trong IDP. Cần xử lý store credential trước lần Deploy tiếp.
Dev single-node/file-storage không chứng minh HA, backup hay cloud delivery.
