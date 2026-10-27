# End-to-end scripts

`e2e/` ở root repository là nơi lưu runner E2E mới xuyên Web Console, backend
và hạ tầng. Đọc [AGENTS.md](../AGENTS.md) và
[documentation index](../orchestrator_docs/INDEX.md) trước khi tạo hoặc chạy.

## Cấu trúc

Tạo thư mục con khi có nội dung tương ứng:

```text
e2e/
├── README.md
├── setup/
├── scenarios/
│   └── <scenario-name>/
│       ├── README.md
│       └── run.mjs
├── fixtures/
└── lib/
```

| Path | Nội dung |
|---|---|
| `setup/` | Chuẩn bị platform/catalog trước khi chạy scenario; không coi setup là bằng chứng deploy thành công. |
| `scenarios/<scenario-name>/` | Một luồng nghiệp vụ và entrypoint có tên rõ ràng; dùng tên lowercase có hyphen, ví dụ `acceptance-app`. |
| `fixtures/` | Score YAML, cấu hình mẫu và dữ liệu kiểm thử không chứa credential. Fixture chỉ dùng cho một scenario có thể đặt cạnh scenario đó. |
| `lib/` | Helpers dùng chung: đăng nhập, thao tác Console, chờ trạng thái và kiểm tra cleanup. |

`run.mjs` chỉ là ví dụ cho Playwright/Node. Shell wrappers được đặt cùng setup
hoặc scenario của chúng. Go helpers/tests cần module Go tiếp tục ở `backend/`;
unit/component tests của Web Console tiếp tục trong `frontend/`. Không đưa
runtime code sản phẩm vào `e2e/` hoặc `orchestrator_docs/`.

## Dependencies và kết nối Console dùng chung

`e2e/package.json` khai báo dependencies cho script trong thư mục này; cài bằng
`npm ci` và `npx playwright install chromium` từ `e2e/`. Unit tests của helpers
chạy bằng `npm test`.

Mọi scenario mới thao tác qua Console phải lấy browser đã đăng nhập từ
[`lib/console.mjs`](lib/console.mjs), không tự viết đăng nhập hoặc ghi cứng địa
chỉ. Cấu hình chung:

| Biến | Ý nghĩa |
|---|---|
| `ORCH_PLATFORM_URL` | Địa chỉ gốc Console, ví dụ `http://localhost:3001` |
| `ORCH_PLATFORM_USERNAME` | Tài khoản đăng nhập |
| `ORCH_PLATFORM_PASSWORD_FILE` | File riêng tư (0600/0400) chỉ chứa mật khẩu |
| `ORCH_HEADLESS=1` | Không hiện cửa sổ trình duyệt |

Helper dùng lại phiên của đúng tài khoản, đăng nhập khi chưa có phiên, dừng khi
phiên thuộc tài khoản khác hoặc sai vai trò yêu cầu. Ví dụ tối thiểu:
[console-sign-in](scenarios/console-sign-in/README.md). Chuẩn bị platform:
[setup](setup/README.md); triển khai ứng dụng mẫu:
[acceptance-app](scenarios/acceptance-app/README.md).

## Nội dung bắt buộc cho một scenario

README của scenario phải ghi:

1. Mục tiêu và links tới specification/use case, runbook liên quan.
2. Prerequisites: phiên bản công cụ, dependencies, platform URL, account và
   tài nguyên cần có; phân biệt adapter thật và fixture/fake.
3. Cách chạy từ thư mục nào, biến cấu hình, defaults và cách cung cấp credential
   từ file riêng ngoài Git. Khai báo package/tool dependencies và cách cài;
   không phụ thuộc ngầm vào working directory hoặc `node_modules` tình cờ có sẵn.
4. Các bước nghiệp vụ, assertions và điều kiện PASS/FAIL.
5. Side effects: dữ liệu/resource tạo mới, phạm vi được phép thay đổi và hành vi
   khi chạy lại hoặc thất bại giữa chừng.
6. Cleanup: resource nào được giữ/xóa, ownership/UID/API checks và cách xác nhận
   cleanup; nếu chủ ý giữ dữ liệu thì ghi rõ.
7. Nơi lưu evidence và giới hạn của lượt kiểm chứng.

Setup script cũng cần hướng dẫn tương ứng, có thể link tới một runbook hiện có
thay vì chép lại procedure. Script không tự cấp quyền mutate platform, database,
Kubernetes hoặc cloud. Giữ quy định quyền external và cleanup trong AGENTS/runbook.

Logs, screenshot, video, Playwright traces, browser session/storage state,
kubeconfig, credentials và Terraform state của lượt chạy phải lưu ngoài Git.
Kết quả từng execution ghi tại `orchestrator_docs/verification/`; procedure
hiện hành ghi tại `orchestrator_docs/operations/`. README ở đây không thay thế
canonical product requirements hoặc execution evidence.

## Runner hiện có

Quy ước này áp dụng cho script mới. Runner đang có trong `frontend/test/e2e/`
và `backend/test/integration/` chưa được di chuyển trong thay đổi hướng dẫn này.
Chỉ chuyển khi có task migration hoặc refactor liên quan; cập nhật imports,
dependencies, commands, CI và runbooks, không giữ hai bản cùng implementation.

Ví dụ setup hiện tại vẫn chạy ở đường dẫn cũ:

- [bootstrap-staging-catalog.mjs](../frontend/test/e2e/bootstrap-staging-catalog.mjs)
- [Hướng dẫn đăng ký catalog staging](../orchestrator_docs/operations/staging-catalog.md)

Khi được di chuyển, script này thuộc `e2e/setup/`.
