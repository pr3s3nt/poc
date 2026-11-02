# Bootstrap platform qua Catalog

Bootstrap đăng nhập Console, cấu hình Git access nếu bật, kiểm tra/đăng ký
Infrastructure Account và Resource Types, rồi đăng ký nguồn Catalog và refresh.
Definitions và rendering templates được quản lý trong Git. Script không tạo
Definition toàn cục, không chọn release thay người dùng và không deploy.

1. Copy nội dung `platform-catalog/` sang gốc repository Git công ty.
2. Review `catalog.yaml` và `definitions/`, đặc biệt placement, criteria và
   storage class; chỉnh cấu hình theo cluster thực tế.
3. Push lên nhánh theo dõi và tạo tag SemVer, ví dụ `v1.0.0`.
4. Chạy bootstrap với repository thật:

```bash
node scripts/bootstrap/run.mjs --target staging \
  --url https://console.company.example \
  --kubeconfig /private/staging.kubeconfig \
  --git-url https://git.company.example/platform/catalog.git
```

Dùng `--target dev|staging` hoặc `--config /private/bootstrap.json`. Presets chứa
prerequisites và `catalog: {id, repository, branch, tagPattern, manifestPath,
includePrereleases, accountBindings}`. `accountBindings` ánh xạ slot trong Git tới
tên/ID tài khoản platform đã đăng ký. Script chuyển tên thành ID qua Console.
Repository mẫu phải được thay trước khi chạy. `--git-ref` và `--update-existing`
không còn là luồng cấu hình; sửa Git, phát hành tag mới rồi refresh Catalog.
Nguồn đã tồn tại phải khớp cấu hình; script không tự ghi đè nguồn khác.

Git riêng tư dùng `gitAccess` với `enabled`, `username`, `tokenFile`, tùy chọn
`url`, `repositoryUrl`, `ref`, `caPemFile`. Mặc định kiểm tra cùng repository và
nhánh Catalog. Token nằm ngoài Git, file quyền 0600/0400. `--update-git-access`
cho phép rotate credential. Script không ghi token vào log/ảnh/video/trace.
Console credentials dùng `ORCH_PLATFORM_USERNAME`, `ORCH_PLATFORM_PASSWORD_FILE`;
preset local có tài khoản test, không dùng cho production.

Sau bootstrap, mở Cài đặt của từng Environment, chọn Catalog release và lưu.
Preview/deploy chỉ hoạt động sau khi có lựa chọn. Để đổi cấu hình, commit vào Git,
phát hành release mới, refresh, chọn release rồi Preview trước khi deploy.
Không có tự động chọn phiên bản mới nhất.

`npm --prefix scripts/bootstrap test` kiểm tra parser, reuse và prerequisites.
Xem [luồng thực thi](EXECUTION_FLOW.md) và
[contract Catalog](../../orchestrator_docs/architecture/contracts/catalog-releases.md).
