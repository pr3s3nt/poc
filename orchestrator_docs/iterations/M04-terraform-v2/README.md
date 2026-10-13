---
id: M04
artifact: milestone-plan
status: current
last_reviewed: 2026-10-13
related: [ADR-014, IMP-021, D01, D03, D12, D13, D14]
---

# M04 — Terraform v2

## Mục tiêu

Thực hiện Bước 2–8 sau khi người dùng duyệt tài liệu Bước 1 đã sửa: durable
state/registry/executor, modules AWS/Kubernetes, Console forms và Xóa tài nguyên,
Redis acceptance app, tích hợp và kiểm chứng UI thật. Product requirements
thuộc [ADR-014](../../architecture/decisions/ADR-014-terraform-v2-lifecycle.md),
use-case specs/contracts, không thuộc plan này.

## Ba chặng và điểm duyệt (quyết định 2026-10-14)

| Chặng | AI được thực hiện | Điểm dừng |
|---|---|---|
| 1 | Hoàn tất A–D, validation/review và docs commit/push | Dừng để người dùng duyệt tài liệu đã sửa và plan M04, trước code/test mutation |
| 2 | Toàn bộ code/unit/integration PostgreSQL thật; k8s-4f A01/E01/E02 + video | A01/E01/E02 pass, video + dự toán đã lưu, k8s-4f sạch thì tự sang I04-12 |
| 3 | Sau I04-11 đạt: AWS A02/A03, cleanup verified, independent review/validation/commit-push | Dừng để người dùng duyệt final handoff/cleanup evidence |

Người dùng bỏ điểm duyệt 2 ngày 2026-10-14; video và dự toán vẫn phải lưu.
Điểm duyệt 1 đã đạt; điểm duyệt 3 bàn giao cuối vẫn giữ. Không tự coi im lặng là duyệt; không hỏi lại quyền đã giao
trong chặng, trừ thiếu credential/capability hoặc đổi behavior/security/scope.
Cleanup tài nguyên đã tạo luôn tiếp tục khi cần, không để chờ approval gây phí.

## Task order

| Iteration | Phạm vi | Chặng | Trạng thái |
|---|---|---|---|
| I04-00 | [Duyệt tài liệu A–D và plan M04](I04-00-design-review/README.md) | 1 | Đã duyệt, sửa đổi selected-set trong lượt này |
| I04-01 | [State backend, schema và ownership graph](I04-01-state-backend/README.md) | 2 | Hoàn tất |
| I04-02 | [Registry module bất biến](I04-02-module-registry/README.md) | 2 | Hoàn tất |
| I04-03 | [Executor tạo, cập nhật và xóa](I04-03-terraform-executor/README.md) | 2 | Hoàn tất |
| I04-04 | [Module namespace, Redis và PostgreSQL](I04-04-kubernetes-modules/README.md) | 2 | Hoàn tất |
| I04-05 | [Module VPC, EKS, ElastiCache và Aurora](I04-05-aws-modules/README.md) | 2 | Hoàn tất |
| I04-06 | [AWS Connection qua Console và Vault](I04-06-aws-connection/README.md) | 2 | Đang thực hiện |
| I04-07 | [Definition, Workload class, Preview và renderer](I04-07-definition-workload-ui/README.md) | 2 | Chưa bắt đầu |
| I04-08 | [Xóa tài nguyên và dependency qua Console](I04-08-resource-delete-ui/README.md) | 2 | Chưa bắt đầu |
| I04-09 | [Redis cho acceptance app hiện có](I04-09-acceptance-redis/README.md) | 2 | Chưa bắt đầu |
| I04-10 | [Tích hợp và runner/evidence/cleanup supervisor](I04-10-integration/README.md) | 2 | Chưa bắt đầu |
| I04-11 | [V2-A01/E01/E02 và dự toán AWS](I04-11-k8s4f-acceptance/README.md) | 2 | Chưa bắt đầu |
| I04-12 | [V2-A02/A03, cleanup và handoff cuối](I04-12-aws-acceptance/README.md) | 3 | Chưa bắt đầu |

Mỗi charter link sources, criteria và checks cụ thể; WORK_ITEMS chỉ là checklist.
Dependencies triển khai: schema/state -> registry -> executor -> modules ->
Connection/catalog/Workload/renderer -> deletion/app Redis -> integration ->
k8s-4f đạt + video/estimate + cleanup sạch -> AWS. Không bỏ task hoặc đổi adapter để rút acceptance.
I04-00 đã được người dùng duyệt sau sửa selected-set và đối chiếu I06-10.
I04-06 là iteration current; các task sau chưa bắt đầu. M02/I06-10 deferred,
không coi milestone cũ hoàn thành; xem [coverage còn lại](../M02-usecase-completion/I06-10-uc01-04-management/README.md#đối-chiếu-công-việc-còn-dở-với-m04).

## Validation và acceptance

Go/frontend/changed-shell/docs checks theo AGENTS.md. Mọi Go validation M04
chạy với `REQUIRE_POSTGRES=1` và `ORCHESTRATOR_POSTGRES_TEST_URL` của server
disposable; thiếu URL phải fail, không skip. PostgreSQL thật cô lập
phải chạy state/locks/CAS/fencing/reopen/rollback và CRUD/dependency/cleanup
contracts, không để required SQL tests skipped. Pure unit có fake hợp lệ nhưng
không dùng fake evidence để đạt live scope. Headed Playwright có cursor/click/
typing/pauses/phụ đề Việt/phase marks/assertions; product steps qua UI.
Real adapter scenarios và assertions là nguồn
[acceptance runbook](../../operations/terraform-v2-acceptance.md), không thêm
ma trận update/delete ngoài bước deletion A01 đã chốt.

## Ownership, quyền và deferred scope

Chỉ namespace test k8s-4f, AWS account/region và owned assets đã được giao;
preflight/pins/estimates/tagging/deadlines/independent cleanup trước create.
Không v1 migration/import, cluster deletion hoặc retained reset.
D12 automatic resume, D13 orphan reconciliation và D14 backup/restore vẫn
deferred; minimal state/locking/unknown-outcome hold và cleanup không hoãn.

## Exit criteria và handoff

- Đủ code/contracts/validations và cả năm scenario thật, A01 có deletion UI.
- Có video, phase marks, assertions, actual adapter/environment và failed attempts.
- Cleanup từng run xác minh hết owned cloud/k8s assets; remaining IDs là chưa complete.
- CURRENT_STATE/code map/traceability ghi kết quả thật; IMP-021 chỉ đóng khi đủ bằng chứng.
- Task-only changes reviewed/committed/pushed, nhánh Claude đã xóa.
- Handoff cuối sẵn sàng và người dùng duyệt; không giả claim approval.

## Current handoff

2026-10-14: I04-01..05 hoàn tất, `5dac2e6` đã push; working tree sạch khi tiếp.
I04-06 cũ dừng trước implementation, không tài nguyên còn tồn tại;
`/tmp/poc-i0406-aws-connection/handoff-paused.md` lưu bàn giao cũ.
Tiếp I04-06 bằng phiên Claude cloud mới; Codex chạy checks và recording local.
Session: `session_01UowTTQea8MqLBRaPktRfSX`;
[Claude cloud I04-06](https://claude.ai/code/session_01UowTTQea8MqLBRaPktRfSX).
Nhánh `claude/m04-i04-06`, base `9b00978`. Đã yêu cầu xác nhận quyền thực tế;
cờ `--dangerously-skip-permissions` chưa phải bằng chứng chế độ của phiên.
I04-11 đạt A01/E01/E02 + video + estimate + k8s-4f sạch thì tự chạy I04-12;
chỉ bàn giao cuối chờ người dùng, không tự ghi đã duyệt.
