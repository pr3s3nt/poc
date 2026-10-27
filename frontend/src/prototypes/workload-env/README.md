# Workload environment variables — review mock

UI sản phẩm với dữ liệu giả, chỉ để duyệt thiết kế. Entry riêng tại
`frontend/workload-mock.html`; không import product API client, không lưu cấu
hình vào backend hoặc browser storage, không Preview/Deploy thật. Refresh sẽ
reset dữ liệu; sidebar dùng lại component sản phẩm và giữ preference thu gọn.
Navigation bị vô hiệu trong mock. Chỉ minh họa staging, một container.

## Phương án 1 — người dùng chọn ngày 2026-10-27

- Một bảng Tên biến / Giá trị. “Thêm biến” tạo ngay dòng nhập và focus tên.
- Giá trị nhập trực tiếp mặc định thuộc workload hiện tại. Không bắt tạo key
  ở Application trước; không có bốn tab hoặc bảng chọn/kết quả song song.
- Checkbox Bí mật che giá trị đang nhập. Khi lưu mock, giá trị secret bị xóa
  khỏi state/input và chỉ còn trạng thái đã lưu; đây không phải secret backend.
- “Chọn từ…” mở một hộp tìm kiếm Application key, resource output hoặc Service.
  Chọn source tự mang theo secret classification. Tên đã nhập được giữ; chỉ
  Application key tự điền tên nếu dòng còn trống. Resource/Service vẫn cần tên
  đúng với ứng dụng. Có thể quay lại nhập trực tiếp trong cùng hộp chọn.
- Chặn tên sai/trùng và dòng chưa có tên; cho phép giá trị chuỗi rỗng.
- Bỏ biến, lưu bản nháp giả và đặt lại đều có thể thao tác.

Đây là mock minh họa hướng thiết kế người dùng đã chọn, chưa implement hành vi
mới vào UC-12/UC-16 hay API. Quy tắc UC-16 hiện hành chỉ cho tham chiếu; trước
khi triển khai sản phẩm cần cập nhật specification và các artifact liên quan.
Việc dán nhiều dòng `.env` chưa thuộc mock này.

## Chạy riêng

Từ `frontend/`, dùng dependencies hiện có:

```sh
npx vite build --config src/prototypes/workload-env/vite.config.ts
npx vite preview --config src/prototypes/workload-env/vite.config.ts --host 127.0.0.1 --port 5187 --strictPort
```

Mở `http://127.0.0.1:5187/ui/workload-mock.html`. Để xem trong tailnet, bind
preview vào Tailscale IP của máy thay vì loopback; không dùng Funnel.
Artifact build nằm ngoài repository tại `/tmp/orchestrator-workload-env-mock`.

## Nguồn UI và đối chiếu

Đọc shared UI design và UC-16 UI; đã mở `WorkloadEditorPage` thật trong browser
với API fixture trước khi thiết kế. Import trực tiếp `AppShell`, `Button` và
`styles/index.css`; dùng lại tabs, status badge và layout panel của Console.
Chỉ phần nội dung biến môi trường được đề xuất thay đổi; thông tin
workload/container/image rút gọn để làm ngữ cảnh cho mock.

Tham khảo interaction trong tài liệu chính thức của
[Render](https://render.com/docs/configure-environment-variables) và
[Railway](https://docs.railway.com/variables): tên/giá trị là thao tác chính,
tham chiếu là lựa chọn tại nơi nhập giá trị. Giữ visual identity của Console.

Review nội dung: bỏ bốn tab, checklist key, bảng nguồn riêng, nút đổi tên và
hướng dẫn dài. Giữ scope workload, môi trường, thông báo mock và trạng thái
bản nháp. Nhãn tiếng Việt nạp từ catalog riêng `vi.json`.

## Giới hạn validation đã biết từ lượt đầu

- `npm run lint`: 46 lỗi ở các file design/E2E có sẵn ngoài prototype; không sửa
  những file đó trong task này.
- `npm test`: 854 pass, 6 timeout tại test sản phẩm hiện có. Chạy lại nhóm
  Definition/editor bằng một worker: 102 pass, 1 timeout ở
  `ResourceDefinitionsPage.test.tsx`. Test còn lại pass khi chạy riêng bằng tên
  test (1 pass, 67 test không được chọn). Không thay assertions hoặc timeout;
  không coi toàn bộ suite đã pass trong một lượt.
- Không chạy backend, Kubernetes hoặc cloud checks: mock không sửa những phần đó.

## Validation phương án 1 — 2026-10-27

- Typecheck, lint riêng prototype và build mock: pass.
- Focused Vitest `AppShell` + `WorkloadEditorPage`: 29/29 pass, không skip.
- Browser E2E trên URL Tailscale: nhập trực tiếp, thêm/focus dòng, mask và xóa
  Secret sau lưu, tìm/chọn source, giữ tên đã nhập, phân loại secret output,
  Service, tên trùng/thiếu, bỏ biến, quay lại nhập trực tiếp, Escape, sidebar,
  lưu/đặt lại: pass; không có lỗi JavaScript hoặc request API.
- Đã xem screenshot trang chính và hộp chọn nguồn ở viewport 1440×1000,
  đối chiếu shell/styles với reference Console. Trang chính chỉ còn bảng nhập;
  danh sách nguồn chỉ mở khi chọn, không lặp hướng dẫn hoặc danh sách đã chọn.
- Documentation checker và diff whitespace check: pass.
- Không rerun full suite/lint toàn repository; không thay product code hoặc
  những file gây lỗi baseline đã ghi ở trên.
