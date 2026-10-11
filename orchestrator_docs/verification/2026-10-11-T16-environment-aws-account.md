---
id: VERIFY-T16-ENV-AWS-ACCOUNT-20261011
artifact: verification-record
status: evidence
last_reviewed: 2026-10-11
---

# T16 — VPC/EKS dùng Kết nối AWS của Môi trường

## Outcome và phạm vi

RD-02 Terraform, branch `refactor_ui`; T02 DONE, working tree sạch ban đầu.
Codex cập nhật UC-03 BR-08, UC-06 BR-20, UC-08 BR-07, realizations, sequences/PNG,
OC-04/OC-10, ADR-011/013 và UC-03 UI docs trước code. Claude implement code/tests
qua `clauded -p` trong tmux `codex-T16-20261011`, không sửa docs/commit/push.
Codex review và giao vòng sửa bằng `clauded --continue -p` cùng session.

Public Terraform VPC/EKS đăng ký với ConnectionKey rỗng, không cần Environment;
mọi key khác rỗng bị từ chối trước insert, kể cả key bằng target. Form không có
selector AWS cho hai module, luôn gửi rỗng dù đã chọn Aurora/Kubernetes trước đó;
không phụ thuộc danh sách Connection tải thành công. Aurora giữ selector và
account riêng bắt buộc. Metadata/criteria/driver inputs và profile policy giữ.
New product seed VPC/EKS bỏ key, không migrate catalog/profile cũ.

Preview plan target/hash giữ Environment binding; provisioning VPC/EKS resolve
READY AWS Connection đúng Organization của Environment. ActiveResource lưu cùng
effective identity với executor request. Aurora thiếu key fail closed, không
inherit target. Winning malformed catalog vẫn bị ConnectionMismatch chặn trước
executor, không chọn ứng viên khác. `match.go`, ranking/ties và reference fixtures
không đổi. IMP-020 phát hiện qua review được giải quyết: prior state khác account
không được reuse; lỗi repository ngoài NotFound không bị nuốt.

## Checks và review

- Codex độc lập: `cd backend && env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY go test ./... && go build ./...`, exit 0. Full suite gồm conformance pass;
  không chạy lặp suite conformance. Logs `codex-go-test.log`, `codex-go-build.log`
  ở `/tmp/poc-T16-handoff-20261011/`.
- Codex độc lập V-FE: typecheck/lint/378 tests (20 files)/build, exit 0. Logs
  `codex-fe-{typecheck,lint,test,build}.log` cùng thư mục. Chunk >500 kB là
  warning hiện hữu, không build failure.
- Claude checks cuối cũng exit 0: V-BE/V-FE, T16/T02/T02B và bash syntax; logs/status ở `/tmp/poc-T16-handoff-20261011/logs/`.
- Shell runner `bash -n`, scenario `node --check`, docs checker với PlantUML và
  whitespace pass. Ba sequence PNG được regenerate bằng PlantUML.
- Registration/service/HTTP tests: empty accepted; equal-target/nonexistent/
  foreign/wrong-kind nonempty keys rejected, no insert; Aurora explicit account.
  Planner tests: independent account targets/hash, empty-key inheritance,
  malformed more-specific winner fails despite a valid lower-score candidate.
  Provision tests: request/persisted identity, missing/nonREADY/foreign/wrong-kind
  accounts, explicit Aurora, same-account prior-state reuse, cross-account reject
  before executor and preserved stored identity. Frontend tests kiểm loading/
  error/empty Connection catalog, controls/payload và type-switch preservation.
- Review sửa Kubernetes override leak trong nhánh submit, hint Aurora, prior-state
  account guard và failed repository reads; browser scenario sửa duplicate VPC tie
  bằng criteria production cho registration phụ, không đổi matching semantics.
- SQL opt-in không chạy: chưa có disposable PostgreSQL được phép trong task này.
  Không AWS/kind/Vault run; local tests không chứng minh adapter thật.

## Browser evidence

Primary `/tmp/poc-T16-evidence-20261011141415`, run ID
`refactor-20261011141416-14301`, PASS; fake adapters, JSON state disposable.
9 assertion records, 12 phase marks, raw/captioned MP4, SRT/ASS và frames giữ ngoài
Git. Raw 1440×900; captioned 1440×996; duration 226.13 giây. Codex full decode
cả hai exit 0, review VPC inheritance/Aurora required/Preview result frames và
captioned frame: UI/cursor rõ, phụ đề Việt ở strip dưới browser, không che UI;
không thấy credentials thật. Không upload evidence.

Setup qua UI: Developer tạo app và bind staging tới seeded fake `aws-account`.
Product steps qua UI: PE đăng ký VPC/EKS với criteria theo app, kiểm Aurora thiếu
account/no POST, chọn account Aurora và kiểm type switching/payload rỗng VPC,
Developer mở standalone Score Preview và xem đúng matched Definitions/target.
Bốn direct POST reject probes là setup/assertion API riêng, không giả thao tác UI;
GET catalog phục vụ assertions. Không Deploy hoặc provision qua browser.
Browser dùng cùng fake key cho Environment và account Aurora tường minh; hai
account khác nhau được backend tests chứng minh riêng, không claim từ video.

T02 regression dùng cùng headed/caption runner: form advanced/Kubernetes override,
validation/duplicate/retention và empty VPC payload pass: `/tmp/poc-T16-evidence-20261011141839`, run ID `refactor-20261011141840-31016`, 113.6 giây. T02B criteria regression PASS: `/tmp/poc-T16-evidence-20261011142103`. Codex decode raw/captioned và review frame của hai regression. Report/check details ở `/tmp/poc-T16-handoff-20261011/report.md`.

Các lượt T16 fail trước primary được giữ: Connection không offered khi fake region thiếu, invalid EKS Definition, thiếu Score params, locator/synchronization và losing/tied Preview candidates. Primary sửa EKS class `eks` và VPC phụ chỉ production để winner đúng theo specificity hiện hành. Evidence paths `/tmp/poc-T16-evidence-20261011135339`, `20261011135437`, `20261011135703`, `20261011140133`, `20261011140534`, `20261011141009` (các timestamp sau dùng cùng prefix `/tmp/poc-T16-evidence-`). Run logs và danh sách run IDs ở `/tmp/poc-T16-handoff-20261011/`. Vòng đầu được ngắt để gửi phản hồi review trong
cùng session, không thay Claude bằng Codex coding; artifacts đã tạo được giữ.
Không xem failure hoặc setup assertions là live proof.

## External và handoff

Không đọc .env/credential thật, không mutate retained DB/Vault/K8S-4F/AWS.
Runner cleanup process/state thuộc run, không xóa evidence. Không user-owned
changes ban đầu hoặc thay đổi ngoài task. Codex cập nhật traceability/current
state/code map và DONE trong commit prefix T16, push `origin refactor_ui`, rồi
đóng session task. Tra SHA bằng `git log --grep '^T16:'`.
