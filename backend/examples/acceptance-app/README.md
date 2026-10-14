# Acceptance application

Three independent Go workloads the orchestrator deploys to verify UC-06 and UC-08
end to end:

| Workload | Role |
|---|---|
| `frontend` | Serves the sample page, proxies `/api` to the backend Service. |
| `backend` | Owns the `jobs` schema, submits and reads jobs. |
| `worker` | Claims pending jobs, writes results back. |

`backend` and `worker` bind to the same shared Score resource
(`postgres.default#acceptance-db`), so the happy path proves:

```text
frontend -> backend -> PostgreSQL -> worker -> backend -> frontend
```

Database connection settings come from the resource outputs the orchestrator
injects as `PGHOST`, `PGPORT`, `PGDATABASE`, `PGUSER` and `PGPASSWORD`. The
password always arrives through a Kubernetes Secret, never as a plain
Deployment environment value.

The frontend also renders a minimal deployment-check table. Its backend checks
that `ACCEPTANCE_CONFIG` equals `acceptance-config-ok`, that the injected
`ACCEPTANCE_SECRET` hashes to the non-secret `ACCEPTANCE_SECRET_SHA256`, and
that PostgreSQL responds to a ping. The frontend checks it can reach the
backend Service. `/api/checks` returns safe checks and cache evidence, never the secret or
database credentials. Missing bindings show `FAIL`; the existing job form
remains available for a deeper frontend → backend → database → worker check.

## Redis trong Terraform v2

Backend và worker dùng chung PostgreSQL và Redis của Environment. Worker lưu
kết quả PostgreSQL trước, sau đó ghi Redis với TTL. Backend đọc Redis trước;
cache miss đọc cùng job trong PostgreSQL rồi nạp lại Redis. Chỉ cache job
`DONE`, không cache `PENDING`. Khóa có dạng
`acceptance:v1:<app>:<env>:<run>:job:<id>`.

| Biến cấu hình | Giá trị |
|---|---|
| `REDIS_HOST`, `REDIS_PORT`, `REDIS_USERNAME`, `REDIS_PASSWORD`, `REDIS_TLS` | Bắt buộc từ outputs `host/port/username/password/tls`; mật khẩu qua Secret, TLS nhận `true` hoặc `false` |
| `ACCEPTANCE_CACHE_APP`, `ACCEPTANCE_CACHE_ENV`, `ACCEPTANCE_CACHE_RUN` | Bắt buộc, mỗi thành phần 1–63 ký tự chữ/số hoặc `.`, `_`, `-` |
| `ACCEPTANCE_CACHE_TTL_SECONDS` | 1–3600 giây, mặc định 30 |

TLS xác minh CA, hostname và SNI. Các binary nhúng public CA roots để ảnh
`scratch` dùng được TLS; không có tùy chọn bỏ xác minh chứng thư.

Lỗi Redis không biến thành kiểm chứng đạt nhờ PostgreSQL fallback. Kết quả bền
vững vẫn hiển thị, nhưng dòng `cache` trong bảng kiểm tra hiện có và `/readyz`
báo thất bại khi binding/auth/TLS/thao tác cache lỗi hoặc ghi worker chưa được
xác minh. `jobs.cache_write` ghi `OK`/`FAILED` sau mỗi lần ghi; job `DONE` thiếu
marker là chưa xác minh. `FAILED` giữ trong database của run, không xử lý lại
job hoặc xóa lỗi bằng refill. Khoảng giữa commit và ghi marker có thể tạm thời
chưa sẵn sàng.

`/api/checks` giữ các boolean `checks` và thêm bằng chứng `cache`: bộ đếm đọc,
job ID/kết quả phân loại gần nhất, số lần ghi worker đạt/thất bại/chưa xác minh,
và lỗi chưa phục hồi theo từng job/thao tác. Không trả giá trị cache, mật khẩu
hoặc raw error. Bộ đếm chỉ thuộc tiến trình backend; runner còn đối chiếu khóa
và TTL Redis thật. Tối đa 64 lỗi được theo dõi; vượt giới hạn giữ trạng thái
thất bại cho vòng đời tiến trình. Một job đọc thành công không xóa lỗi của job
khác. Mỗi lần kiểm tra thử lại tối đa 16 thao tác đã lỗi trong deadline, không
thêm hoặc xử lý lại job. Lỗi đọc chỉ phục hồi sau GET đúng khóa thành công;
lỗi refill chỉ phục hồi sau SET đúng job thành công.

Kiểm tra phụ thuộc dùng thêm SET/GET/DEL của một khóa probe ngẫu nhiên trong
scope cache, TTL tối đa 10 giây, để PING đạt không che quyền thao tác bị thiếu.

Kiểm thử tích hợp cần PostgreSQL và `redis-server` thật. `REQUIRE_REDIS=1`
được suy ra từ `REQUIRE_POSTGRES=1`; thiếu dịch vụ là fail, không skip. Runner
`bash backend/test/integration/acceptance-redis-local.sh` dựng fixture riêng,
yêu cầu native PostgreSQL binaries và Redis, giữ evidence ngoài Git và xác minh
cleanup. Bindings Score/Definition và browser nghiệp vụ trên cluster được
kiểm chứng trong I04-10..12, không suy unit/CLI thành live proof.

Build the images with `test/integration/build-images.sh <tag>`; it produces a
static binary per workload and wraps it in the shared `Dockerfile`.

For a browser-driven deployment and these four basic checks on the existing
`kind-idp-internal` cluster, build the Web Console and run
`bash backend/test/integration/acceptance-playwright-kind.sh` from the repo
root. It creates a run-scoped Application and namespace, then removes only
that namespace after the check. It requires the local Vault/VSO installation
and the scoped backend token file described in the kind runbook. The script
prints the path to `acceptance-full.webm`, a single video of the browser flow
from sign-in through the final check page. Browser actions are slowed and key
screens pause long enough for a person to review them. Evidence stays in an owner-only
temporary directory, including when the browser test fails; do not publish the
video without reviewing it for local test data. Pass `--human` for a slower
recording that types into every form with a visible cursor; see the kind
runbook for publishing it.
