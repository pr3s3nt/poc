---
id: UC-16-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-24
related: UC-16, UC-12
---

# UC-16 UI screens

## Application home — selected Environment

Giữ tab staging/production, danh sách Workload, thêm/sửa/xóa/hoàn tác và Xem
trước thay đổi. Đích chạy do matched Definitions/references quyết định theo
UC-01 V04–V06, không có Environment deployment Connection picker. Settings
vẫn quản lý key dùng chung. Lưu editor chỉ tạo thay đổi chờ triển khai.

## Add/Edit workload — DEVEX-ENV

Giữ shell, typography, container cards, nút nâng cao và footer Lưu/Hủy của
sản phẩm. Thay hai checklist “Biến & Bí mật của ứng dụng” và “Nguồn khác” bằng
một bảng **Biến môi trường** cho mỗi container. Không thêm trang/wizard mới.
Nhãn chuẩn: **Tên biến**, **Lấy giá trị từ**, **Giá trị / Nguồn**, **Xóa biến**.

```text
Container: main
Biến môi trường                                       [+ Thêm biến]
Tên biến       Lấy giá trị từ       Giá trị / Nguồn
LOG_LEVEL      Giá trị nhập         debug                         [Xóa]
REGION         Biến của ứng dụng    APP_REGION                    [Xóa]
API_TOKEN      Bí mật              PAYMENT_API_KEY · Đã cấu hình [Xóa]
DB_HOST        Tài nguyên          db → host                     [Xóa]
DB_PASSWORD    Tài nguyên          db → password · Bí mật        [Xóa]
BACKEND_URL    Workload khác        backend → http                [Xóa]
```

Đây là layout contract, không đổi nhận diện sản phẩm. Các giá trị/tên trên là
ví dụ; outputs lấy từ Type metadata, không hard-code PostgreSQL/Redis fields.
Tái sử dụng controls/styles hiện hành; bảng dùng được bằng bàn phím. Dropdown
key/dependency/workload hỗ trợ tìm theo tên khi danh sách dài; output/port
cũng là lựa chọn có nhãn, không bắt nhập `${...}` hoặc biết cú pháp Score.

| Nguồn | Control ở cột Giá trị / Nguồn | Thao tác tại chỗ |
|---|---|---|
| Giá trị nhập | Text input giá trị thường; rỗng là giá trị hợp lệ | Một dòng trợ giúp khi chọn: “Giá trị thường được lưu cùng cấu hình. Dùng Bí mật cho dữ liệu nhạy cảm.” |
| Biến của ứng dụng | Chọn key VARIABLE của app/env; không copy value vào dòng | Tạo biến |
| Bí mật | Chọn key SECRET cùng scope; tên + Đã cấu hình, không value | Tạo bí mật |
| Tài nguyên | Chọn alias dependency, rồi output theo Type; output secret có nhãn Bí mật | Thêm tài nguyên |
| Workload khác | Chọn workload cùng env, rồi cổng Service hợp lệ | Không tạo workload lồng trong editor |

**+ Thêm biến** thêm dòng trống, focus Tên biến, nguồn Giá trị nhập. Đổi nguồn
chỉ hiện control cần dùng, giữ tên; không xóa key/dependency. Chọn key có sẵn
hoặc vừa tạo chỉ gợi ý tên khi tên biến đang trống. Nhiều dòng được dùng cùng
key/output; xóa một dòng không ảnh hưởng dòng khác. Empty state chỉ cần “Chưa
có biến môi trường” và nút thêm. Không thêm explanatory paragraphs cho mỗi hàng.

## Tạo biến hoặc bí mật tại chỗ

Mở dialog có accessible name **Tạo biến**/**Tạo bí mật**, focus Tên key và giữ
workload editor mounted. Hiện scope app/env, Tên key, Giá trị, nút **Hủy** và
**Tạo và chọn**. Có đúng một câu về ranh giới lưu: “Key được lưu cho staging;
workload áp dụng sau Lưu → Xem trước → Triển khai.” Thay staging bằng scope thật.

- Bí mật dùng password input; không prefill value từ catalog hoặc source cũ.
- Tên key gợi ý từ tên biến nếu identifier hợp lệ, vẫn sửa được; tên container
  đã có không bị đổi theo tên key. Variable/Secret phải theo validation UC-12
  hiện hành (giá trị không rỗng); không đồng nhất với empty literal được phép.
- Success chọn key vào dòng gọi dialog và trả focus về dòng đó. Workload vẫn
  là draft chưa lưu; key đã nằm trong Settings. Hủy workload không xóa key này.
- Existing key cùng tên: báo trùng, cho **Dùng key hiện có** sau reload/review
  nếu đúng loại; không ghi đè. Muốn sửa giá trị key dùng chung thì dùng Settings.
- Nếu chưa chọn kho, dialog Bí mật hiện selector **Kho bí mật** từ READY stores
  và nút **Lưu kho bí mật**. Chỉ bật Tạo và chọn sau lưu kho thành công. Không
  có kho phù hợp: báo “Chưa có kho bí mật sẵn sàng”, cho tải lại hoặc hủy; các
  dòng khác vẫn sửa được. Không có quyền đăng ký kho hạ tầng trong form này.
- Kho đã chọn: chỉ hiện tên; đổi kho qua Settings vì có flow copy/verify riêng.
  Lưu kho rồi tạo key lỗi phải giữ và hiển thị kho đã lưu, không ngầm rollback.
- Busy/stale/network xử lý theo [states](states.md); không replay POST sau GET.

## Thêm tài nguyên ngay từ dòng biến

**Thêm tài nguyên** mở dialog, dùng chung dependency form với “Tài nguyên của
Workload”: Alias, Loại, Cách cung cấp/Class, shared ID và typed inputs hiện có.
Nhãn action **Thêm và chọn**; validation alias duy nhất, không đụng alias ảo đã
được dùng (`env`/Service aliases), type/required params/class như UC-16 hiện hành.

Success thêm đúng một dependency vào cùng draft, chọn alias ở dòng gọi và
focus **Output** để người dùng chọn. Không auto chọn output, không auto tạo
các biến DB_HOST/DB_PASSWORD hoặc provision. Hủy không thêm dependency. Tất cả
container cùng workload thấy dependency mới. “Tài nguyên của Workload” vẫn là
nơi quản lý dependencies, kể cả dependency không có binding env.

Xóa hoặc sửa alias/type làm mất target: giữ dòng/target cũ với lỗi, yêu cầu sửa
hoặc bỏ dòng. Không xóa binding theo dependency. Không tự xóa dependency khi
bỏ dòng sử dụng cuối; người dùng có thể còn dùng dependency ở cấu hình khác.

## Import Score và Edit

Nhập một Score cho một workload; form và import cùng validation. Literal
thường mới được nhận theo UC-16 BR-20; full references phân loại lại vào năm
nguồn. Không copy output đã resolve hay secret vào Giá trị nhập. Escaped dollar,
chuỗi rỗng và whitespace phải giữ đúng semantics khi import → edit → save.
Không biểu diễn được lossless thì giữ Score nâng cao, không bỏ fields. Không
lưu hoặc Deploy khi chỉ import; xem lại rồi Lưu. Legacy literal chưa xác minh
không được trả về editor chỉ vì đã nới validation cho giá trị thường mới.

## Lưu, hủy và xóa

Footer hiện hành giữ Lưu draft Workload/Hủy. Lưu thành công về Application home
với Chờ thay đổi và Xem trước; failure giữ edits. Hủy bỏ workload edits nhưng
không xóa key/kho đã commit bằng nút riêng; thông báo boundary có ở dialog tạo.
Xóa workload vẫn cần xác nhận, pending delete và Hoàn tác như hiện hành.

## Accessibility và visual acceptance

- Các control có label chứa mục đích và container/dòng khi cần phân biệt;
  nút xóa có accessible name cho biến tương ứng, không chỉ icon.
- Tab order theo từng dòng; dialog trap focus, Escape hủy khi chưa submit,
  trả focus về nút gọi. Không nested dialogs, không đóng giữa mutation.
- Error gắn dòng/field, không chỉ màu. Source có secret hiển thị chữ Bí mật.
- Kiểm desktop 1512×980 và 1280×900, sidebar mở/thu gọn; không đè input hay
  footer, không horizontal overflow cả trang. Không thêm mobile redesign.
- Nguồn đối chiếu: AppShell, tokens/app styles, Button, WorkloadEditorPage và
  SettingsPage; [browser reference](../../../verification/2026-10-24-devex-env-design.md)
  là UI sản phẩm với app/config fixture, không phải bằng chứng tính năng mới.

## T08 — Form cơ bản và nâng cao, nhãn tiếng Việt

Nhãn Việt chuẩn của editor theo [glossary](../../../GLOSSARY.md#nhãn-web-console-vi-01),
bao gồm validation frontend, aria-label, trạng thái tải/rỗng/lỗi và bảng Biến môi trường.
Giữ `Workload`, `Container`, `Service`, `staging`/`production`, technical IDs,
image, Score keys và payload. Text/aria/validation dùng catalog ADR-020. Mapping lỗi backend theo contract code T20.

- Phần cơ bản giữ tên Workload, Container/image, bảng Biến môi trường, resource alias/type
  và các input có kiểu, Service port và đường dẫn công khai.
- Mỗi Container có phần nâng cao cho CPU/memory request và limit. Tóm tắt luôn
  hiện request có hiệu lực: request tường minh, nếu thiếu thì limit cùng loại,
  nếu cả hai thiếu thì CPU `10m`, memory `32Mi`. Limit thiếu hiển thị chưa đặt.
  Đây là giải thích defaults hiện hành, không tự thêm capacity vào Score.
- Resource Redis/PostgreSQL có class/cách cung cấp từ metadata trong phần
  cơ bản. Custom/imported class vẫn giữ ở nâng cao; class rỗng có hiệu lực
  default, không tự đổi thành in-cluster/cloud-managed.
- Target port override nằm trong phần nâng cao từng Service port. Khi rỗng,
  target port có hiệu lực bằng Service port và serialization giữ fallback số
  hiện hành. Không tự thêm public route.
- Sau thao tác “Thêm đường dẫn công khai”, chỉ tự chọn đích khi có đúng một
  Service port hợp lệ (tên không rỗng, số nguyên 1–65535, tên duy nhất).
  Với không hoặc nhiều cổng hợp lệ, yêu cầu chọn tường minh. Thay đổi/xóa cổng
  khiến đích route không hợp lệ phải hiển thị lỗi và chặn lưu; không retarget.
- Đóng phần nâng cao chỉ đổi hiển thị; imported/edited overrides và alias vẫn
  giữ nguyên. Hiện tóm tắt override ngay cả khi đóng, để cấu hình không bị ẩn.
- Container đầu mặc định `main` và gợi ý tên key theo BR-16;
  không tự chọn Secret hoặc key mới. Lưu vẫn chỉ tạo draft cho đúng môi trường.

## T12 — Entry points từ trang ứng dụng

Trang ứng dụng giữ scope môi trường đã chọn, với entry points “Thêm Workload”,
“Sửa”, “Xóa”, “Hoàn tác”, “Xem trước Score” và “Xem trước thay đổi”.
Lưu draft quay về danh sách có nhãn Chờ thay đổi; không triển khai ngầm.
Tab và payload giữ `staging`/`production`; missing key chỉ có metadata an toàn,
không hiện giá trị. Chuyển môi trường không nhận phản hồi của scope cũ.

## T25 — Import đường dẫn công khai

Form chỉ serialize `service.publicRoutes`. Import có `service.publicPort` báo
lỗi tiếng Việt theo `INVALID_SCORE`, giữ nội dung để sửa, không tạo route `/`
và không tự lưu. Draft/current projection cũ chứa field này bị từ chối rõ;
không mở form với routes bị bỏ mất. Valid import/edit giữ đầy đủ nhiều routes,
Service ports và overrides; Lưu → Xem trước vẫn là các bước tường minh.

## T30 pending rename boundary

UC-12 explicit rename opt-in chỉ sửa desired references nguyên tử với revision;
Preview đọc drafts mới, liệt kê workloads bị ảnh hưởng và pin config/draft/current
set versions. Deploy vẫn tường minh; token trước rename bị stale. Rename-only
hoặc delete có thể để reference thiếu và Preview phải chặn Deploy. Không đổi
runtime/current set hoặc Environment khác khi lưu.

## Class và metadata dependency

Keep the unified environment-variable table and Save draft → Xem trước thay đổi → Triển khai flow.
Each Redis/PostgreSQL dependency displays “Cách cung cấp / Class” beside its
alias/type/typed params using authoritative metadata: “Trong cluster”
(`in-cluster`) and “Dịch vụ managed AWS” (`cloud-managed`). This replaces hiding
these v2 choices solely under generic advanced class text. Redis in-cluster
on EKS is valid. Developer never chooses Terraform module or provider credential.
Keep advanced/custom/imported class lossless; do not silently rewrite default.
Shared ID control keeps existing semantics for backend/worker dependencies.
Show target incompatibility or missing metadata/Definition as actionable error,
not automatic class switch. Secret outputs remain bindings only; no values.


## Ngôn ngữ — tương thích main 8f4d66f

Các nhãn tiếng Việt ở đây là wording chuẩn; mọi text mới/đổi dùng `t()` và
catalog keys theo [ADR-020](../../../architecture/decisions/ADR-020-console-message-catalogs.md).
Bổ sung key/placeholder đồng bộ vào `vi.json`, `en.json` và các catalog có trong
manifest tại thời điểm implement; không hard-code text/aria/error. Không dịch
tên key, alias, output, image hay Score. Năm nguồn tiếng Anh là **Entered value**,
**Application variable**, **Secret**, **Resource**, **Other workload**; tiêu đề
**Environment variables**, action **Add variable**, **Create and select**,
**Add and select**. Browser checks phải chạy cả Việt và Anh. Giữ locale reload
hiện hành, không persist secret để khôi phục sau đổi ngôn ngữ/reload.
