---
id: M04
artifact: milestone-plan
status: current
last_reviewed: 2026-10-15
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
| 3 | Sau I04-11 đạt: AWS A02/A03, cleanup verified, independent review/validation/commit-push | Gửi báo cáo cuối, không chờ duyệt (2026-10-15) |

Người dùng bỏ điểm duyệt 2 ngày 2026-10-14; video và dự toán vẫn phải lưu.
Điểm duyệt 1 đã đạt; điểm duyệt3 bỏ ngày2026-10-15, hoàn tất gửi báo cáo. Không tự coi im lặng là duyệt; không hỏi lại quyền đã giao
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
| I04-06 | [AWS Connection qua Console và Vault](I04-06-aws-connection/README.md) | 2 | Hoàn tất local |
| I04-07 | [Definition, Workload class, Preview và renderer](I04-07-definition-workload-ui/README.md) | 2 | Hoàn tất local |
| I04-08 | [Xóa tài nguyên và dependency qua Console](I04-08-resource-delete-ui/README.md) | 2 | Hoàn tất local |
| I04-09 | [Redis cho acceptance app hiện có](I04-09-acceptance-redis/README.md) | 2 | Hoàn tất local |
| I04-10 | [Tích hợp và runner/evidence/cleanup supervisor](I04-10-integration/README.md) | 2 | Hoàn tất local |
| I04-11 | [V2-A01/E01/E02 và dự toán AWS](I04-11-k8s4f-acceptance/README.md) | 2 | Current; A01r4/E01r2 overallPASS/SQL/video/cleanupCLEAN; E02 đang chạy |
| I04-12 | [V2-A02/A03, cleanup và handoff cuối](I04-12-aws-acceptance/README.md) | 3 | Chưa bắt đầu |

Mỗi charter link sources, criteria và checks cụ thể; WORK_ITEMS chỉ là checklist.
Dependencies triển khai: schema/state -> registry -> executor -> modules ->
Connection/catalog/Workload/renderer -> deletion/app Redis -> integration ->
k8s-4f đạt + video/estimate + cleanup sạch -> AWS. Không bỏ task hoặc đổi adapter để rút acceptance.
I04-00 đã được người dùng duyệt sau sửa selected-set và đối chiếu I06-10.
I04-11 là iteration current; I04-12 chưa bắt đầu. M02/I06-10 deferred,
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
- Gửi bàn giao cuối theo quyền 2026-10-15, không chờ duyệt hoặc tự ghi đã duyệt.

## Current handoff

2026-10-14: I04-01..09 đã verified local. I04-06 code `f3ed5bb` đã review,
1082 Go tests/zero skip + 479 frontend tests/build/vet/docs và Codex headed
browser trên SQL/Vault thật, AWS transport fake đạt. [Biên bản](../../verification/2026-10-14-i04-06-aws-connection.md).
Video `/tmp/poc-i0406-local-browser-r2/I04-06-captioned.mp4`; cleanup DB/roles,
Vault/PG/container/network/private files đã xác minh, không task-owned cloud/
cluster resources. Lượt trước và bàn giao cũ giữ tại `/tmp/poc-i0406-aws-connection/`.
Phiên I04-06 [session_01UowTTQea8MqLBRaPktRfSX](https://claude.ai/code/session_01UowTTQea8MqLBRaPktRfSX);
mode nội bộ không introspect được, Bash/Write/Edit/network quan sát unprompted.
Nhánh chính/tự sinh đã xóa sau push merge `9b49075`; checkout riêng đã dọn.
I04-07 code `3ed0a89`/handoff `00a8813` đã merge: 1121 Go/0 skip, 549 frontend,
build/vet/docs và Codex headed video đạt. [Biên bản](../../verification/2026-10-14-i04-07-definition-workload-ui.md).
Video `/tmp/poc-i0407-local-browser-r3/I04-07-captioned.mp4`; SQL/Vault thật,
executors/attempts video dựng sẵn. Cleanup DB/roles/Vault/PG/mạng/tệp riêng tư
đã xác minh. Phiên [session_01B8BFUdSwAFdkASEVQA1oKY](https://claude.ai/code/session_01B8BFUdSwAFdkASEVQA1oKY)
quan sát Bash/Write/Edit/git/network không prompt; mode nội bộ không introspect.
Brief/handoff/checks/video cũ giữ ở `/tmp/poc-i0407-definition-workload-ui/` và
các thư mục evidence. Snapshot pending/standalone đã sửa và có PG concurrency tests.
Nhánh I04-07 chính/tự sinh đã xóa, checkout review đã dọn sau push `b988f27`.
I04-08 code `8d213a3` đã merge `b4ed07e`: 1171 Go/0 skip, 609 frontend,
build/vet/docs và headed video 125,6 giây đạt. [Biên bản](../../verification/2026-10-14-i04-08-resource-delete-ui.md).
Video `/tmp/poc-i0408-local-browser-r2/I04-08-captioned.mp4`, r3 checks tại
`/tmp/poc-i0408-codex-checks-r3/`; cleanup DB/roles/Vault/PG/mạng/tệp riêng tư
đã xác minh. REAL engine/CLI/encrypted SQL/Vault, FAKE objects/absence;
không v2 live proof. [Phiên](https://claude.ai/code/session_015AaNpBjwF2dUGJQHUxUrRv)
quan sát Write/Edit/Read/Bash/git/network, mode nội bộ không introspect. Code/
handoff/brief giữ tại `/tmp/poc-i0408-resource-delete-ui/`; handoff cuối
`2235fde` đã merge, docs push `ee41f1d`; hai nhánh cloud và checkout đã dọn,
remote xác minh không còn nhánh I04-08. Không còn gap token/proof/scope/marker diagnostic hay DESTROYED intent.
I04-09 code `65223ef`/handoff `67e26a7` đã merge `45006fd`: 1254Go/0skip,
609frontend/allchecks và 6 ảnh thật amd64/arm64 với manifest digests đạt.
[Biên bản](../../verification/2026-10-14-i04-09-acceptance-redis.md); checks
`/tmp/poc-i0409-codex-checks-r3/`, ảnh `/tmp/poc-i0409-image-build/`.
PG/network/private files/Redis fixture/registry cleanup đã xác minh.
[Phiên](https://claude.ai/code/session_019GgofsetFjxTDq6V83vv6Q) quan sát
Read/Write/Edit/Bash/git push, proxy network; mode nội bộ không introspect.
Cloud fullsuite failures/skips không tính đạt, local không skip. Các lỗi
readiness/marker/ACL/cross-job masking đã sửa; không task-owned cluster/AWS.
Brief/handoff `/tmp/poc-i0409-acceptance-redis/`; nhánh chính/tự sinh và checkout
đã dọn sau push `6907de3`, remote xác minh vắng mặt.
I04-10 current: [session_01HtXzgfyBzoNma2DQ2SzNY5](https://claude.ai/code/session_01HtXzgfyBzoNma2DQ2SzNY5)
đã tạo từ refactor_ui `55b2fa0` đã push; nhánh yêu cầu `claude/m04-i04-10`.
Brief `/tmp/poc-i0410-integration/cloud-brief.txt`; cờ bypass đã truyền,
early status `88ad784` xác nhận Read/Write/Edit/Bash/git hoạt động; mode
nội bộ không introspect. Hai nhánh chính/tự sinh `-dwitbm` đồng bộ. Cloud
có PG/Redis, thiếu Terraform/score-k8s/kubectl và provider registry bị chặn;
Codex có local tools để kiểm độc lập, không tự thay Claude viết code. WIP `7e40432` gồm real-profile/Vault outputs/Redis binding và k8s runner.
Review phát hiện generated binaries, AWS runners chưa có, safety net xóa TF-owned
objects trực tiếp thay original Terraform lifecycle, dynamic appsecret scan và
dirty-marker còn thiếu; đã gửi về cùng phiên, chưa chấp nhận hoặc merge.
Local r1 `/tmp/poc-i0410-codex-checks-r1/` đang chạy trên WIP, PG/Redis cô lập;
Vault1.21.4 native đã chuẩn bị từ Docker image cache, container tool đã dọn.
Không task-owned cluster/AWS assets, chưa live proof.
Kiểm tra cloud mỗi **15 phút (900 giây)**; giữ lượt chạy khi chờ, kiểm sớm khi
lỗi/completion/yêu cầu người dùng. Không có scheduler tự khởi chạy lại lượt.
Dự toán đang chuẩn bị `/tmp/poc-m04-estimate-20261014/`; I04-11 chốt trước AWS.
A01/E01/E02 pass + video/estimate + k8s-4f sạch thì tự chạy I04-12. Điểm duyệt
cuối vẫn giữ; không tự ghi đã duyệt.

### Bàn giao chuyển công cụ — 2026-10-14

Người dùng báo Claude cloud hết credit; fetched head I04-10 `666da24` đã lưu
local, chưa merge. Chuyển Claude local qua tmux/`clauded`; nếu lại hết hạn mức
thì agent Codex `gpt-6-luna` viết code/test theo quyền mới. Code/checks/review
hiện có giữ nguyên; cloud session/URL ở trên, không tuyên bố file chưa push
đã được lấy về. Findings ownership/claim-before-effect, AWS runner, secrets
binding/scan/argv và dirty marker vẫn phải giải quyết. Không task-owned
cluster/AWS assets; r1 WIP1309Go/0skip/fullchecks đã dọn PG/network/files.

Claude local đã khởi chạy tại tmux `poc-m04-i0410-clauded-local`, dùng
`clauded --teleport session_01HtXzgfyBzoNma2DQ2SzNY5`; UI báo Session resumed
và bypass permissions on. Conversation khôi phục, code checkout `666da24`
không có dirty code cloud được phục hồi. Teleport chuyển checkout sang nhánh
`claude/m04-i04-10-dwitbm`; Claude được cấm commit/push, Codex quản lý commit
sau review. Brief `/tmp/poc-i0410-local-claude/brief.txt`, handoff yêu cầu
`/tmp/poc-i0410-local-claude/handoff.md`. Nếu local quota fail thì chuyển agent
`gpt-6-luna`, giữ code dở và mọi kiểm tra/cleanup; chưa dùng phương án này.

Khởi động local đã xác minh Bash đọc brief và kiểm checkout; tmux cần gửi
Enter dạng CSI-u để submit trên CLI hiện tại. Claude báo weekly usage92%,
chưa báo exhausted, đang chạy; khi lỗi quota sẽ giữ dirty code và chuyển
`gpt-6-luna`. Không coi cảnh báo92% là hoàn tất hoặc lỗi công cụ.

Lần kiểm local15 phút: Claude có dirty source9 tracked files và AWS helper/tests
untracked, đang chạy checks ở `/tmp/poc-i0410-local-claude/`. Một Bash chờ
`pgrep -f` tự khớp argv nên đã dừng riêng PID955552 sau kiểm parent825829;
không dừng checks/PG cleanup. Đã gửi lại yêu cầu dùng result file/child PID và
hoàn tất mọi findings. Không có quota failure được xác nhận ở lần kiểm này.

Lần kiểm~07:24UTC: Claude local không báo quota, bàn giao NOT_READY với1331Go
pass/0skip, frontend lượt đầu14timeouts/7files; rerun riêng621/37files đạt,
chưa fullchecks. Yêu cầu tiếp tục AWS A02/A03 runner và process-level PG/TF
kill chain. Teleport thực tế đã lấy thêm cloud `1f89f6c` (ownership/claim fixes).
Cloud late-push `bb0ae24` được fetch/import sau khi tạm ngắt writer; local WIP
giữ trong taskstash `4526d515a646a958b054f2c3c602b6bd7f5c0444` và backup
`/tmp/poc-i0410-local-claude/pre-import-*`. Có4 code conflicts cần Claude xử
lý; đã giao và resume local, chưa merge nhánh chính. Giữ mọi dirty code.

Lần kiểm~07:54UTC: local writer đang chạy finalchecks, chưa quota failure;
conflicts đã xử lý, thêm AWS scenarios và process killchain. Review tiếp:
đừng dùng wildcard `/tmp/TestKill*` để dọn; preserve exact-owned paths/evidence.
AWS Preview purity phải đối chiếu owned ECR aux baseline; manifest writes
không được silently bỏ lỗi; probe Pod setup phải tag/record/cleanup đúng.
Optional config-secret diagnostic AWS ghi chưa cấu hình theo nguồn chuẩn,
không giảm PG/Redis secret/cache criteria hoặc gọi toàn bộ bảng pass.

Lần kiểm ~08:30UTC: Claude local xác nhận “You've hit your weekly limit”,
reset Oct15 23:00 Asia/Bangkok. Đã dừng đúng tmux do task tạo để tránh hai
writer; toàn bộ index/worktree/untracked trên `bb0ae24` được giữ nguyên.
Chuyển code/test cho agent Codex `gpt-6-luna`, task `/root/i0410_luna`,
checkout `/tmp/poc-i0410-codex-review`; không commit/push hoặc chạy cluster/AWS.
Local final2 có1344Go pass/0fail/0skip, build/vet/typecheck/lint đạt nhưng
frontend suite lỗi, nên chưa chấp nhận I04-10. Evidence
`/tmp/poc-i0410-local-claude/checks-final2-1791965092/`; PG/network/private
credential fixture của lượt đó đã xác minh xóa. Findings còn lại phải sửa,
Codex điều phối sẽ review và chạy fullchecks độc lập trước merge. Không có
tài nguyên cluster/AWS do task tạo, I04-11/12 chưa chạy.

Local review tiếp: clauded quota đã hết, Luna bàn giao code; frontend độc lập
622/37files và typecheck/lint/build/docs đạt. Go/race1348/0fail/0skip lượt trước
đạt, bản cuối đang kiểm lại sau sửa AWS identity và cleanup closure. Late cloud
heads `42e9e42`/`0922eb6` đã fetch và review; source archive
`/tmp/poc-i0410-local-claude/cloud-late.bundle`. AWS runner cũ/common refactor
được đối chiếu với bản local đã bổ sung ECR/probe ownership, identity fail-closed,
kill-chain và Vault riêng. Không import bản cũ đè các sửa lỗi local; mọi source
đều được giữ. Traceability của các object Vault sẽ ghi theo fixture owned khi
chạy thật; không thêm yêu cầu configuration-secret AWS trái nguồn chuẩn.
Không task-owned live cluster/AWS assets; private kubeconfig/reviewer handles
đã tạo ngoài Git để Codex chạy I04-11, không gửi cho writer.

I04-10 hoàn tất local: code `b66b5ba`, squash `0964fb6`;1351Go/race/0fail/0skip,
622frontend/fullchecks/docs/shell đạt. [Biên bản](../../verification/2026-10-14-i04-10-integration.md).
Task I04-11 current: Codex chạy thật A01/E01/E02 trên `kind-k8s-4f`, chỉ owned
namespaces, độc lập cleanup và video. Private handles ở `/tmp/poc-i0411-private/`,
không gửi writer; headed Chromium/xdotool/pins đã chuẩn bị. AWS estimate còn
`/tmp/poc-m04-estimate-20261014/` draft; chốt trước gate. Không có task-owned
live assets lúc bàn giao này. Lỗi code trả agent Luna theo quyền fallback,
không tự viết code sản phẩm/test. Giữ final gate, không tự ghi đã duyệt.

I04-10 branch cleanup sau push2695056 đã xác minh: remote branches vắng, local
checkout/branches/exact taskstash đã dọn; source bundles và checks còn giữ.
I04-11 dùng checkout riêng `/tmp/poc-i0411-local-runtime` tại2695056, frontend
build đạt. A01 lượt đầu `a01-20261014-r1` lỗi422 khi đăng ký kho bí mật,
trước triển khai: Vault token fixture thiếu lookup-self/capabilities-self
(read-only xác nhận403). Video thất bại ở
`/tmp/poc-i0411-live/a01-20261014-r1/A01-captioned.mp4`.
Namespace/PV vắng, sentinel nguyên vẹn; supervisor NEEDS_ATTENTION tại guard
delivery trước khi bind Environment. Không tuyên bố CLEAN hoặc mở retry.
Agent `/root/i0410_luna` sửa code/test trong checkout I04-11; không đọc handles.
Các thành phần còn giữ: container/volume PostgreSQL
`poc-m04-i0411-platform-pg-20261014`/tên tương ứng hậu tố `-data`, hai Vault
run-owned và private recovery dưới evidence. Mười node image references
chính xác ghi trong `/tmp/poc-i0411-local-preparation/kind-image-ownership.json`;
giữ baseline images, không prune. Chưa tạo namespace/AWS.
Bước kế: review/check sửa lỗi, hoàn tất cleanup bằng state/handles gốc và
xác minh sạch trước một lượt retry A01.

Cập nhật phục hồi A01 r1: sản phẩm delivery/terraform/status đều CLEAN/no
remaining, chứng minh Environment chưa bind và không deploy. Supervisor phục
hồi dùng guard đã sửa, handles/manifest/deadline gốc xác minh no-create và
CLEAN; trạng thái cũ NEEDS_ATTENTION giữ nguyên làm bằng chứng. Namespace/PV
vắng/sentinel UID giữ nguyên được query lại; database/ba roles đã drop, hai
Vault xác minh PID/session/log riêng rồi dừng, mười node references/ba local
aliases dọn và baseline images giữ nguyên. Marker CLOSED_CLEAN; scenario vẫn
FAIL. Proof `operator-cleanup-proof.json`, `operator-cleanup-product.json`,
`operator-vault-cleanup.json` và private `operator-recovery/` dưới evidence.
Video348.2s h2641440x996 decode đạt; public secret scan24files đạt.
Container/volume PostgreSQL nền riêng còn giữ cho các scenario kế tiếp.
Code fix branch `codex/m04-i0411-fixes`, agent `/root/i0410_luna` đã xong;
Codex checks toàn bộ ở `/tmp/poc-i0411-codex-checks-r1/` đang chạy. Retry config
ngoài Git đã chuẩn bị nhưng chưa chạy; chỉ mở sau review/checks/merge.

I04-11 fix Luna `da524c6` merge/push `72a807e`: độc lập1352Go/0fail/0skip,
build/vet,622frontend/37files/typecheck/lint/build, docs có PlantUML,
shell/JS syntax/diff đều đạt. Fixture checks PG/network/credential đã dọn;
evidence `/tmp/poc-i0411-codex-checks-r1/`. A01 retry duy nhất
`a01-20261014-r2` đã bắt đầu trên code này, namespace dự kiến
`orch-v2-test-a01-20261014-r2`, evidence
`/tmp/poc-i0411-live/a01-20261014-r2/`. Ảnh retry đã xác minh immutable refs;
ownership/baseline ở `/tmp/poc-i0411-local-preparation/a01-r2-images/`.
Chưa có A01 pass, E01/E02 chưa chạy; AWS vẫn gate.

**Handoff hiện hành sau A01 retry fail:** I04-11 chưa đạt, E01/E02/I04-12 chưa
chạy. Retry deploy/Ready/bindings/real Vault/job/PostgreSQL/cache/hit/TTL/miss/
refill/reload đạt; UI gỡ Workload thiếu native confirm nên fail. Luna chẩn đoán
readonly, chưa sửa thêm hoặc mở run mới. Video/phase/assertions giữ ở evidence
r2, video1183.53s decode và26 public text files secret scan đạt.
Supervisor NEEDS_ATTENTION: delivery verifier đếm cả TF resource Services/
Secrets. Operator xác minh deployments vắng, chạy engine gốc dọn Redis/PG;
renderer backend-env/worker-env đối chiếu persisted deployment IDs rồi dọn
với UID preconditions. Namespace vẫn giữ vì default ServiceAccount thiếu
managedFields.39 resource types kiểm kê chỉ còn CA/default SA; namespace UID
a76ad236-5a87-44a2-b4bc-8c0040e5ba4e khớp state; không bypass guard/giả CLEAN.
Quyết định dọn thủ công đúng namespace/UID đã hỏi, chưa có trả lời. Còn namespace,
hai Vault PID3718805/3718896, database i0410_0831776d1fdc/ba roles và platform
PG container/volume; private handles giữ. PV/workload/app resources/cache refs
đã dọn;28 baseline UIDs giữ nguyên. Cloud/tmux cũ không chạy, agent
`/root/i0410_luna` idle; code root `72a807e`, docs mới hơn. Bước tiếp: nhận
quyết định ngoại lệ, cleanup có proof/state archive, sửa lỗi/checks rồi chạy
lại A01 theo quyết định2026-10-15 bên dưới. Dự toán chuẩn bị2.13USD A02/2.0505USD A03 cho5h,
không billed cost/ceiling. [Biên bản](../../verification/2026-10-14-i04-11-k8s4f.md).

Quyết định hiện hành2026-10-15: thất bại phải tìm nguyên nhân và giao writer
sửa, review/checks độc lập rồi chạy lại sau cleanup sạch; bỏ giới hạn một retry
scenario. Các câu "retry duy nhất" phía trên mô tả quyền lúc chạy r2, không còn
là giới hạn hiện hành. Không dừng chỉ vì retry fail, không giảm assertions hoặc
bypass ownership. Namespace r2 còn chờ quyết định ngoại lệ; quyền retry mới
không tự cấp quyền xóa namespace bị guard chặn. Tiếp tục sửa/checks độc lập
trong lúc giữ handles phục hồi; chưa mở run mới trước physical cleanup.

**Quyền và handoff mới nhất2026-10-15:** người dùng cho dọn đúng namespace/UID
r2 sau xác minh lại, sửa lỗi và tiếp tục đến hết I04-12 không cần duyệt giữa
chặng/cuối; hoàn tất gửi báo cáo. Các handoff "pending quyền"/"giữ final gate"
ở trên là lịch sử. Codex kiểm kê lại39 kinds và đúng UID/hai defaults rồi xóa
namespace bằng UID precondition; query namespace/hai PV ABSENT và sentinel UID
nguyên vẹn. Proof `operator-authorized-namespace-delete.json` dưới evidence r2.
Chưa gọi kết quả A01 pass; DB/Vault/private state còn giữ để khép registry và
phục hồi, chưa mở run mới. Agent `/root/i0410_luna` đang sửa code/test trong
checkout I04-11 nhánh `codex/m04-i0411-delete-fixes`; không đọc live handles.
Bước tiếp: review/checks, xác minh cleanup/retained/state archive rồi A01 mới,
E01/E02; đủ gate thì tự chạy AWS A02 sạch rồi A03 và báo cáo.

**Cleanup r2 hoàn tất2026-10-15:** helper đã review dùng engine/module pin gốc
xác minh namespace NotFound và original physical IDs vắng, registry3instances
DESTROYED, no remaining; không chỉnh SQL/tombstone bằng tay. Supervisor phục hồi
độc lập CLEAN với ba lớp DONE, original NEEDS_ATTENTION giữ làm lịch sử. Spec
phục hồi có finish trigger ngay, chỉ cấp expiry30s cho quá trình giám sát mới
vì expiry execution cũ đã qua; không mở provisioning mới, cleanup budget120m.
Query lại namespace/hai PV ABSENT, sentinel giữ nguyên. DB/ba role drop, hai
Vault PID/session/stdout riêng được xác minh rồi dừng; marker CLOSED_CLEAN,
scenario vẫn FAIL. Private SQL archives trước/sau cùng state key giữ0600.
Proof `operator-cleanup-product-final.json`, `operator-registry-final.json`,
`operator-cleanup-proof-final.json`, `operator-vault-cleanup.json` dưới r2.
PG platform container/volume riêng còn giữ để dùng cho lượt mới.

Code Luna đang finalize: UID API DeleteOptions, nguyên ownership qua removal
attempt, immutable namespace/defaultSA baseline và đúng layer verification.
Preliminarychecks r2 không đạt (inventoryfixture +/tmp full), r3-race chưa là
bằng chứng bản cuối vì có sửa sau review. Không skip/giảm assertion. Finalchecks
và runtime mới dùng TMPDIR riêng trên disk
`/home/thanhnt1/.cache/poc-m04-i0411-validation`; evidence runtime mới
`/home/thanhnt1/.cache/poc-m04-i0411-evidence/`, ngoài Git0700. Giữ failed
evidence cũ, không dọn wildcard hoặc user-owned /tmp. A02/A03 configs/preflight
chỉ đọc và estimates13lines/counts/5h đạt, chưa tạo AWS.

**Bản sửa đã chốt2026-10-15:** Luna hoàn tất UID DeleteOptions/Foreground,
đợi vắng có deadline, ownership qua removal attempt, baseline bất biến và
phân lớp cleanup; giữ nguyên module pin namespace. Codex đã review, đang chạy
full Go race/PostgreSQL/Terraform/Redis, build/vet, frontend và docs độc lập tại
`/home/thanhnt1/.cache/poc-m04-i0411-evidence/checks-r4-final`. Chưa ghi pass.

**Checks bản cuối đạt2026-10-15:** Luna `6bffefa` merge/push `ea101e0`;
1394Go/race/0fail/0skip với PG/Terraform/Redis thật,622frontend/37files,
build/vet/type/lint/docs/PlantUML/shell/node đạt. Fixture checks cleanup sạch.
Evidence `checks-r4-final/` trên disk. A01 mới `a01-20261015-r1` chuẩn bị,
28 retained UIDs nguyên vẹn; chưa scenario pass, AWS gate chưa mở.

**Handoff mới nhất15Oct:** A01 `a01-20261015-r1` FAIL/PLANNING trước create,
normal cleanup/28retainedUIDs/imagecache CLEAN, video1415.07s giữ. Provider
socket dài112bytes gây bind invalid argument, schema A/B rootngắn86bytes đạt.
Tất cả real evidence mới dùng `/home/thanhnt1/.m4/<run-id>`0700, giữprivate
isolation/modulepins. A01 `a01-20261015-r2` đang chạy, supervisor ARMED;
config `/tmp/poc-i0411-private/a01-20261015-r2.json`0600. Code runtime6bffefa.
AWS preparationfix3136dcc merge/push6d9fb2f,1395Go/race/0skip/622FE/fullchecks
đạt và fixtureclean; source/preflight schema/proof ngoàiGit. Sau A01 sạch chạy
E01/E02 từ checkout `/tmp/poc-i0412-local-runtime`3136dcc (latest code + dist).
Gateđủ thì AWSA02 sạch rồiA03; configs/gates trỏ shortroot, chưa tạoAWS.

**Handoff hiện hành:** A01shortroot r2 browser42PASS/8marks vàfull cleanupCLEAN,
nhưng overall exit1 vìSQLreopen hardcodewrongapplication/phase. Video1346.6s
vàproofs trong `.m4/a01-20261015-r2/`;28retainedUIDs và2PVabsence độc lập,
10cache refs/3aliases đãdọn. Không ghiI04-11PASS. Luna đang sửaCODE/TEST tại
`/tmp/poc-i0412-local-runtime`3136dcc: scopedscenario-awareSQLreopen/HTTPchecks,
overallrunnerPASSproof choAWSgate (browserPASS+cleanupCLEANchưa đủ), A03
explicitgp2 class vìEKS1.35/CSIdefaultfalse. Modulepins giữnguyên; estimateA03
5h2.0507USD/13lines đạt. Sauwriterfreeze: review/fullchecks0skip, merge/push,
A01fresh rồiE01/E02; đủgate mớiAWSA02sạch rồiA03. Các run cũ CLOSED_CLEAN,
platformPGcontainer/volume riêng còn giữ; chưaAWSresources.


**Tiếp tục kiểm chứng15Oct:** Writer đã đóng băng bản sửa SQL reopen theo đúng
Application/Connection và từng scenario, bằng chứng kết thúc toàn runner cho
AWS gate, yêu cầu marker đóng tường minh trước run mới và A03 storage class
`gp2` tường minh. Codex đang chạy lại bộ checks độc lập đầy đủ tại
`/home/thanhnt1/.m4/checks-i0412-runtime-r2/`; chưa ghi PASS hoặc merge.
Bộ sơ bộ runtime-r1 được dừng để bổ sung sửa guard cleanup, đã xác minh dọn
PostgreSQL/network/file credential riêng; không dùng làm bằng chứng đạt.
A01 mới dự kiến `a01-20261015-r3`, chưa deploy; E01/E02 và AWS chưa chạy.


**Bản sửa đã kiểm chứng15Oct:** Luna `34a9dcc` đã được Codex review/merge;
full1396Go/race/0fail/0skip với PostgreSQL/Terraform/Redis thật,632frontend/
38files, build/vet/type/lint/docs/PlantUML vàshell/node syntax đạt. Evidence
`/home/thanhnt1/.m4/checks-i0412-runtime-r3/`; sourcehash unchanged, PostgreSQL/
network/credentialfile riêng đãvắng. R2checks1395pass/1staleCLItestfail/0skip
được giữ; sửa đúngfixture explicitclosure, không giảmguard. SQLreopen nayđúng
scope/scenario, AWS gateyêu cầu overallrunnerreceipt, DirtyRunsyêu cầu đóng
marker tường minh; A03gp2 explicit, không đổi modulepins. A01fresh
`a01-20261015-r3` chuẩn bị ở `.m4/a01-20261015-r3/`,28retainedUIDskhớp,
namespace chưa có khipreflight; imageownership trong `.m4/prep-a01-r3/`.
Chưa ghi livePASS; đủA01/E01/E02 mới mởAWS.


**A01r3 kết thúc15Oct:**34a9dcc browser42PASS/8marks, nhưngoverallFAIL
vìreopen verifier chưa tính `builtin-existing-cluster` và `render-internal`
trongcatalog. Authenticated read-only API thậtxác nhậnđúng5definitionkeys,
proof `operator-reopen-definition-keys.json`. Receipt ghiFAIL/exit1/CLOSED_CLEAN,
khôngsuybrowserPASS thànhscenarioPASS. Namespace/haiPV vắng,28retainedUIDs
khớp, database `i0410_e38d60639323`/ba rolesvắng,haiVault dừng/privatevắng,
10imagerefs/3aliases dọn,baseline/sourceimages giữ. Video
`/home/thanhnt1/.m4/a01-20261015-r3/A01-captioned.mp4`1336.53s giải mãđạt,
24textfiles quétsecretđạt; khungREADY/CACHE/CLEANUP vàphụđề đãreview.
Luna sửa riênghaiJS verifier/test theo catalogthật củanăm scenario; Codex
đangchạy lại fullfrontend/docs, backendkhôngđổi so1396Go/race/0skip đãđạt.
A01fresh r4 chuẩn bị; E01/E02/AWS chưa chạy.


**Catalogfix đãđạt:** Luna `d1f1da4` merge sau independent632frontend/38files,
type/lint/build/node/docs/PlantUML/diffPASS, sourcehashunchanged tại
`/home/thanhnt1/.m4/checks-reopen-catalog-r1/`. Backendbyteunchanged34a9dcc,
giữ bằngchứng1396Go/race/0skip đãchạy; khôngchạy lại Go cho thayđổi riêngJS.
Reopen exactcatalog naygồm builtin vàrender template thực tế của từngscenario.
Chuẩn bịA01 `a01-20261015-r4`, không dùngreceiptFAILr3 đểmởAWS.


**A01 đạt toàn lượt15Oct:** `a01-20261015-r4`, code d1f1da4, exit0,
runner-completion PASS/CLOSED_CLEAN,42liveassertions/8marks. UI khai báo/
Preview/deploy/3Ready/jobDONE, actualPGrow/Rediswrite-hit/TTLmiss/refill/reload,
UIxóa workload vàselectedRedis+PGđạt, uncheckednamespace giữREADY đếnsupervisor.
SQLreopen đạt5types/5definitions/3DESTROYEDresources/6deployments.
Video `/home/thanhnt1/.m4/a01-20261015-r4/A01-captioned.mp4`1339.07s,
decode/READY-CACHE-CLEANUPframes/phụđềreview đạt;24textfiles quétsecret đạt.
Namespace/haiPV vắng,28retainedUIDs khớpđộc lập; database
`i0410_83705f083962`/ba rolesvắng,haiVaultdừng/privatevắng;10imagerefs/3aliases
đãdọn,baseline/sourceimages nguyênvẹn. Proofs `operator-retained-and-absence.json`,
`operator-auxiliary-cleanup.json`,imagecleanup `.m4/prep-a01-r4/`.
E01 `e01-20261015-r1` đang chạy từ cùngcheckout; E02/AWS chưa chạy,
I04-11 chưa đủba scenario, IMP-021 vẫn mở.


**E01r1 vàbản sửa05bba6e:** E01browser12PASS/4marks, noDeploy/noApply/
noResources/supervisorCLEAN nhưngoverallFAIL vìverifier kỳvọngbuiltin mà
Previewkhôngpersist. RealAPIexactkeys4 ghi `operator-reopen-definition-keys.json`.
ReceiptFAIL/exit1/CLOSED_CLEAN; independent28retainedUIDs/namespaceabsence,
DB `i0410_d0f4045dd407`/3roles/privatevắng,Vaultdừng. Video
`.m4/e01-20261015-r1/E01-captioned.mp4`482.2sdecode/frame/phụđềreview đạt,
22textfiles secret scanđạt. Source trace `provisioning.Service.provisionNode`
chỉgọiEnsureBuiltinCluster choreserved internal-clustermatch; E01Preview và
AWS/EKS khôngpersistbuiltin. Luna05bba6e sửaexactscenarioexpectations/test;
full632frontend/type/lint/build/node/docsPASS tại `.m4/checks-reopen-catalog-r2/`,
sourcehashunchanged/backendbyteunchanged34a9dcc (1396Go/race/0skip đãđạt).
A01r4 vẫnđạt; freshE01r2 tiếp theo, E02/AWS chưa chạy.


**E01 đạttoànlượt15Oct:** `e01-20261015-r2`, code05bba6e, exit0 vàreceipt
PASS/CLOSED_CLEAN;12liveassertions/4marks. UIchặnnamespace missing vàannotations
sai kiểu, khônglưuDefinitionlỗi; Previewpure,0deployments/0resources/noapply.
SQLreopen đạt5types/4definitions/0resources/0deployments. Video
`/home/thanhnt1/.m4/e01-20261015-r2/E01-captioned.mp4`484.73sdecodeđạt,
phaseframes/phụđềđãkiểm tra;22textfiles quétsecretđạt. Independentcleanup
28retainedUIDskhớp/namespacevắng, DB `i0410_c39149ce7f17`/ba roles/privatevắng,
Vaultdừng, khôngcóimageimport. Proof `operator-independent-cleanup.json`.
E02 `e02-20261015-r1` bắtđầu từ05bba6e; A01r4/E01r2 đãđạt, gateAWS cònE02.


E02 lượt `e02-20261015-r1` không đạt: namespace được tạo nhưng Redis không
có Pod/PVC, nên không chứng minh được lỗi kéo ảnh. Bộ kiểm chứng chờ hết hạn;
SQL reopen vẫn đọc được 2 resource và lịch sử thất bại. Supervisor CLEAN,
marker CLOSED_CLEAN, namespace và database `i0410_160695d878fe`/ba role vắng,
28 UID giữ lại nguyên vẹn; Vault dừng và thư mục riêng đã xóa. Video
`/home/thanhnt1/.m4/e02-20261015-r1/E02-captioned.mp4` dài1044 giây, giải mã
đạt; ảnh lỗi và bằng chứng dọn độc lập được giữ. Không ghi E02 đạt.

Nguyên nhân fixture: ô chuỗi nhận nguyên văn chuỗi JSON đã có dấu ngoặc kép,
đưa dấu đó vào image reference và bị module từ chối trước Pod. Danh mục AWS
cũng dùng chuỗi JSON cho giá trị typed. Luna `bb38fc8` sửa đường nhập liệu:
giải mã catalog trước khi nhập, chuỗi nhập nguyên giá trị, kiểu khác dùng JSON;
ảnh E02 nhập không kèm dấu kép. Giữ nguyên trường map sai kiểu có chủ đích của
E01. Codex review và chạy độc lập634frontend/38files, type/lint/build/node/
docs/PlantUML đạt, mã băm không đổi; backend không đổi so1396Go/race/0skip.
Bằng chứng `.m4/checks-form-overrides-r1/`. Chuẩn bịE02r2; AWS vẫn bị chặn.
