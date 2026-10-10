---
id: VERIFY-T02B-CRITERIA-EDITOR-20261010
artifact: verification-record
status: evidence
last_reviewed: 2026-10-10
---

# T02B — Editor Điều kiện áp dụng dùng chung

## Scope và outcome

RD-06 frontend, VI-02/04/05 trên branch `refactor_ui`; T01/T02 đã DONE.
Working tree sạch lúc bắt đầu, không có user-owned changes. Codex cập nhật
UC-03 UI screens/states/api-mapping trước khi giao Claude implement qua tmux.

`CriteriaEditor.tsx` là editor controlled dùng chung: Mọi nơi `[{}]`,
Theo loại môi trường `env_type`, Theo ứng dụng (+ môi trường) dùng ID thực
qua GET applications, và Tùy chỉnh nâng cao đủ năm field/nhiều dòng. Copy
clone độc lập; criteria nâng cao không biểu diễn được cần xác nhận trước
chuyển mode. Loading/error/retry/late replies không ghi đè edits. Tóm tắt và
xem trước phân biệt profile với env_type, thiếu context có thông báo rõ;
cảnh báo overlap không kết luận ambiguous hoặc winner. Không endpoint mới,
không persist/provision khi xem trước; matching authoritative thuộc T19.

Code/test: editor và tests, ResourceDefinitionsPage và integration tests,
CSS editor, scenario T02B trong refactor-local.mjs. Canonical business
specification/matcher/API payload không đổi. Code map và traceability cập nhật.

## Checks và browser evidence

- V-FE cuối exit 0: typecheck/lint/**193 tests, 19 files**/build; logs và
  `exit=0` tại `/tmp/poc-T02B-coordination/logs/*-final.log`.
- V-UI(T02B) exit 0, 11 assertions / 17 phase marks; command:
  `bash backend/test/integration/refactor-ui-local.sh --scenario T02B --headed --captions vi --evidence /tmp/poc-refactor-T02B-final4-1791634101`.
- Codex độc lập kiểm bash syntax runner, documentation checker và whitespace:
  pass. Runner shell không đổi.

Regression V-UI(T02) exit 0, 9 assertions / 10 marks tại
`/tmp/poc-refactor-T02-regress-1791634354`; log `scenario-T02.log`, run ID
`refactor-20261010121235-19768`. Codex decode captioned regression exit 0.
Các UI runs dùng `ORCH_VIDEO_XDOTOOL` trỏ tới binary đã giải nén ở evidence
final2, chỉ đọc, để tránh tải apt lỗi; không sửa helper/runtime dependency.

Run `refactor-20261010120822-1868`, adapters **fake**, JSON state disposable
ngoài repo. Seed synthetic app/catalog/account; product steps qua UI, API GET
chỉ assertion. Sign-in → resource form → bốn modes và đăng ký payload → app/env
dropdown → incomplete/complete scope context → advanced nhiều dòng → chặn
unsafe switch/giữ criteria → copy/sửa độc lập → sign-out.

Evidence `/tmp/poc-refactor-T02B-final4-1791634101/`: `T02B-raw.mp4` (1440×900,
139.6 giây), `T02B-captioned.mp4` (1440×996), captions SRT/ASS, marks.json,
assertions.txt, result.txt, run.json, ffprobe.json và frames. Codex full decode
hai video exit 0, review dropdown/advanced confirmation và captioned incomplete
context tại `/tmp/poc-T02B-coordination/caption-preview.png`: cursor hiện,
phụ đề Việt trong strip dưới browser, không che controls/không credential thật.

## Review fixes và sự cố evidence

Review đầu phát hiện copy bỏ wildcard khi criteria chứa `[{}, {env_type: ...}]`.
Claude sửa classify/normalize/lossless để giữ mọi dòng, kể cả nhiều wildcard;
known own fields tránh prototype keys. Tests thêm mixed wildcard, multi-blank,
copy/round-trip, late successful/failed replies và profile mẫu khác/unknown.
Source list error/retry được hiển thị trong editor dùng chung.

Lượt đầu `/tmp/poc-refactor-T02B-1791632582` bị Claude xóa nhầm sau fail,
trái quy tắc retention; không thể review evidence đó. Các lượt sau được giữ:
`/tmp/poc-refactor-T02B-1791632627-b`,
`/tmp/poc-refactor-T02B-final-1791633562`,
`/tmp/poc-refactor-T02B-final2-1791633789`,
`/tmp/poc-refactor-T02B-final3-1791634065` (stale dist, dừng trước browser).
Lỗi dịch vụ Claude, setup xdotool/download/focus và expectation ứng dụng seed
đã được xử lý trước run final. Không dùng intermediate failure làm proof pass.

Không Go product changes nên không chạy V-BE/conformance/PostgreSQL opt-in.
Không external mutation trên retained DB/K8S-4F/Vault/AWS; video fake không
chứng minh provisioning thật. Evidence ngoài Git, không upload.

## Cleanup

Final T02B/T02 JSON state đã xóa; không còn runner/scenario process của hai run.
Claude báo cáo ở `/tmp/poc-T02B-coordination/report.txt`; session riêng task
được đóng sau review, không đóng session người dùng. Không có user-owned
changes ngoài task. Một lần Claude dùng `pkill -f` quá rộng khiến shell của
chính lượt chạy bị ngắt (exit 144); không có bằng chứng mutation external,
và final runs dùng cleanup ownership của runner.
