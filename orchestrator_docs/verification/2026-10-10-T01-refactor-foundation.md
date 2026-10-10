---
id: VERIFY-T01-REFACTOR-FOUNDATION-20261010
artifact: verification-record
status: evidence
last_reviewed: 2026-10-10
---

# T01 — Thuật ngữ và runner Console local

## Scope và outcome

Branch `refactor_ui`; VI-01, VI-04 nền, VI-05 nền. Codex cập nhật
[glossary](../GLOSSARY.md#nhãn-web-console-vi-01),
[shared UI](../architecture/ui/README.md) trước khi Claude implement code/test
qua session tmux riêng. Không đổi page labels, API, matcher, seed profile hoặc
backend product. Locator role/label có nhãn Việt và nhãn hiện tại, với cờ riêng
cho từng trang cho tới task Việt hóa trang đó; không thêm i18n.

Runner `backend/test/integration/refactor-ui-local.sh` hỗ trợ `--scenario`,
`--headed`, `--captions vi`, `--evidence` và dùng `ORCH_VIDEO_DIR`. Registry
hiện chỉ có T01. Reuse `video-lib.sh`, `video.mjs`, `human.mjs`, captions và
pattern backend local disposable hiện có. Không cần stub/provider mới cho smoke.

## Checks

- V-FE exit 0: typecheck, lint, 18 files / 154 tests pass, build pass.
  Log `/tmp/poc-T01-coordination-20261010/vfe.log`.
- V-UI(T01) exit 0, cả trước và sau review cleanup. Lệnh tại root:
  `bash backend/test/integration/refactor-ui-local.sh --scenario T01 --headed --captions vi --evidence /tmp/poc-refactor-T01-pass-1791628004`.
- `bash -n backend/test/integration/refactor-ui-local.sh` pass sau lần sửa cuối:
  bounded TERM → group KILL → wait, tránh wait vô hạn trước group KILL.
  Không chạy lại full suite sau reorder này; code frontend/scenario không đổi.
- V-D: `python3 scripts/check_docs.py` và `git diff --check` pass.
- Scenario T99 bị từ chối exit 2 trước khi tạo directory/process.
- Assertion fail có chủ đích sau sign-in: exit 1, giữ result FAIL, assertions,
  raw/captioned video, marks và captions ở
  `/tmp/poc-refactor-T01-fail-1791627853`.
- SIGTERM tới wrapper: exit 143; raw video decode sạch, result FAIL, assertion
  `interrupted by SIGTERM`, marks/SRT được giữ tại
  `/tmp/poc-refactor-T01-term-1791628064`. Status lấy từ wrapper PID thật;
  số 0 của launcher trong report đầu đã được supersede ở `status2.txt`.
- Node SIGKILL: wrapper exit 137, không còn owned process sau cleanup tại
  `/tmp/poc-refactor-T01-kill-1791628097`. Không coi đây là bằng chứng video
  finalize khi nhận SIGKILL.

## Evidence review

Run ID `refactor-20261010102645-11677`, adapters **fake**, JSON state riêng
ngoài repository; chỉ seeded synthetic test account/catalog, không API seed
thay cho product UI. UI steps: sign-in → bốn mục catalog → Applications →
sign-out. API read assertion xác nhận 401 sau sign-out; cookie HttpOnly.

Evidence chính: `/tmp/poc-refactor-T01-pass-1791628004/`:

- `T01-raw.mp4`: H.264, 1440×900, 15 fps, 40.53 giây.
- `T01-captioned.mp4`: 1440×996, strip phụ đề dưới browser.
- `captions.srt`, `captions.ass`, `marks.json` (8 phases), `assertions.txt`
  (8 PASS), `result.txt` (PASS), `run.json`, `ffprobe.json`, `frames/`.

Codex full-decode raw và captioned của run cuối, captioned assertion-failure
video và raw SIGTERM video: pass. Review frame đăng nhập, resource types,
resource definitions, secret stores, đăng xuất của run đầu và frame Connections
có caption của run cuối: UI không trống, cursor hiện, phụ đề khớp bước và không
che UI. Mật khẩu masked; không hiển thị credential thật. Review frames bổ sung
nằm ở `/tmp/poc-T01-coordination-20261010/`.

## Cleanup và giới hạn

State JSON tạm đã xóa; backend/Xvfb/Node/browser/recorder thuộc run đã dừng.
Session tmux của task đóng trong handoff; sessions người dùng giữ nguyên.
Evidence giữ ngoài Git, không upload. SIGTERM giữ raw video và SRT, không tạo
bản burn-in captioned. SIGKILL không đảm bảo recorder/evidence finalize.

Không chạy V-BE/conformance vì không đổi Go product/planner; runner build Go
binary để smoke. Không PostgreSQL opt-in, kind, Vault thật, AWS hoặc mutation
trên retained K8S-4F; fake video không chứng minh adapters thật. Việt hóa trang
vẫn thuộc các task sau. User-owned `refactor.md` được đưa vào commit đầu T01
theo mục 0; không có user-owned change khác lúc bắt đầu.
