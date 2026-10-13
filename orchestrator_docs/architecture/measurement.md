---
id: ARCH-MEASUREMENT
artifact: architecture-proposal
status: draft
last_reviewed: 2026-10-13
---

# Thiết kế đo lường hiệu quả Orchestrator

## 1. Mục tiêu và phạm vi

Đo cả kết quả giao phần mềm (5 DORA metrics) và đóng góp trực tiếp của platform
(6 platform metrics). Mỗi số liệu phải truy ngược được tới evidence, có mẫu số,
coverage và phiên bản công thức. Không suy luận hiệu quả hoặc quan hệ nhân quả
chỉ từ số deployment tăng.

Đây là bản thiết kế để triển khai sau, không phải capability đã có. Không thay
đổi lifecycle deployment, schema đang chạy hoặc active M04 roadmap. Khi implement,
chuyển required behavior vào specification tương ứng, cập nhật shared domain,
[schema](database/schema.md), ERD, contracts và traceability trước code.

Nguồn định nghĩa DORA: [DORA software delivery performance metrics](https://dora.dev/guides/dora-metrics/)
(review 2026-10-13). Quy tắc phân cohort, denominator, verification và cửa sổ
quan sát dưới đây là chính sách đo của sản phẩm, không phải ngưỡng DORA quy định.

## 2. Baseline hiện có và phần cần bổ sung

| Hiện có | Có thể dùng | Giới hạn |
|---|---|---|
| Deployment StartedAt/FinishedAt, status, action, ActorRef | Runtime duration và execution failure rate | Không phải commit lead time hoặc change fail rate |
| WorkloadSnapshot, ManifestDigest, AppliedConfigRevisionID | Liên kết deployment với trạng thái workload bất biến | Manifest digest không phải image digest hoặc commit SHA |
| DeltaSnapshot và resource progress | Nhận diện no-op, provision, update và reuse | Không chứng minh app phục vụ đúng hoặc database kết nối thành công |
| Organization/Application/Environment/target generation | Scope và execution target | Tên environment không chứng minh đó là production |
| PostgreSQL system of record | Lưu evidence, projection, transactional outbox | Chưa có measurement model |

Entry points kiểm tra: `backend/internal/domain/deployment/deployment.go`,
`backend/internal/application/deployment/service.go`,
`backend/internal/adapters/postgres/deployment.go`,
`backend/internal/delivery/http/`, `frontend/src/features/deployments/`.
Không claim rằng những package measurement đề xuất dưới đây đã tồn tại.

## 3. Kiến trúc và luồng dữ liệu

```text
Deployment service ── transaction: lifecycle + measurement event/outbox ─┐
CI release manifest ── authenticated ingestion + artifact verification ─┤
Incident UI/API ── incident, attribution, recovery evidence ────────────┤
Task UI/API ── task, work sessions, assistance, acceptance evidence ────┤
Cost import ── platform cost + allocation + coverage ──────────────────┘
                                  │
                        PostgreSQL evidence ledger
                                  │
                  Durable projector (retry + dedup + watermark)
                                  │
                  Facts + versioned metric snapshots
                                  │
                    Scoped query API → Console / CSV
```

Backend giữ Go layering: delivery → application → domain/ports; adapters implement
ports. Đề xuất `domain/measurement`, `application/measurement`,
`ports/measurement`, PostgreSQL adapter và `frontend/src/features/measurement`.
MVP dùng PostgreSQL và worker trong backend, chưa cần Kafka/warehouse. Prometheus
chỉ đo sức khỏe worker (lag, retries, rejected events); không dùng làm ledger
DORA hoặc gắn user/deployment ID thành labels.

Lifecycle transition và event được commit trong cùng UnitOfWork. Worker chỉ đọc
committed outbox, projection và checkpoint cập nhật cùng transaction. Khi restart,
lease hết hạn cho worker khác tiếp tục; unique event ID ngăn đếm trùng. Lỗi query
analytics không chặn deploy; lỗi ghi outbox cùng transaction phải fail nhất quán,
không báo lifecycle thành công nhưng thiếu event. Cần integration test invariant này.

Chỉ lưu event hợp lệ, typed, allowlisted. External inbox xác minh authentication,
source scope, signature khi connector có signature, delivery ID, body-size và rate
limit trước nhận; không cho client tự khai event lifecycle nội bộ. Bản đầu hỗ trợ
CI gửi release manifest qua service token và incident nhập tay; connector Git/CI,
incident vendor bổ sung sau, không giả định vendor đang dùng.

## 4. Identity, provenance và thời gian

- `service_id` là workload identity ổn định trong Application; rename không đổi ID.
  DORA primary scope: Organization + Application + service + production stream.
- Production stream là service và tập environment production được khai báo rõ;
  region/canary không tự động nhân số lần release. Snapshot classification và
  stream policy tại deployment; thay tên/profile không đổi lịch sử.
- `delivery_id` gom retry/rollout của cùng release; `deployment_id` là execution
  attempt hiện có; `task_id` gom công việc người dùng; không dùng thay thế nhau.
- CI cung cấp repository ID, commit SHA + committed_at, build ID, từng container
  image digest, artifact-to-commit manifest và source provider. Chỉ nhận nguồn
  commit timestamp được xác thực; tag image tự khai chưa đủ provenance.
- Đối chiếu digest đã quan sát ở runtime với release manifest. Multi-container
  lưu đủ digest và commit riêng. Cấu hình/infra-only không có Git SHA phải hiện
  là change không có provenance, không gán commit gần nhất.
- Event có `occurred_at` (UTC, source time), `received_at` (server), schema version,
  source, source_event_id, aggregate_id/sequence, correlation_id và evidence_ref.
  Organization lấy từ auth và kiểm tra membership của mọi referenced ID.
- Timestamp tương lai quá 5 phút hoặc duration âm vào quarantine, không tính.
  Các mốc đúng ngữ nghĩa dùng source time; received_at đo ingestion lag.
- Sửa attribution/timestamp bằng correction record có lý do, actor, revision và
  supersedes; không overwrite evidence gốc. Late event làm recompute bucket bị
  ảnh hưởng; report/export đã phát hành giữ snapshot cũ để đối chiếu.

## 5. Semantic delivery và success

Execution `SUCCEEDED` vẫn giữ nguyên nghĩa hiện tại. Bổ sung measurement fact
`ProductionDelivery`, không thêm success giả vào Deployment.

Một delivery được tính khi workload/config thay đổi thật, đến production stream
và đạt acceptance policy đã version: rollout readiness + kiểm tra ứng dụng cần
thiết. Database-dependent onboarding có check kết nối database; Kubernetes Ready
đơn thuần chưa đạt acceptance đó. Evidence ghi loại check, target generation,
release digest, kết quả và thời điểm. Runtime fake luôn tách khỏi live.

Delivery states: PENDING → VERIFIED hoặc REJECTED; timeout → UNKNOWN, chờ evidence
bổ sung. Cùng delivery có nhiều attempt, chỉ VERIFIED một lần ở mốc đạt policy.
Preview, no-op, pure provision, staging và remove bị loại khỏi primary DORA;
xóa service đo ở lifecycle report riêng. Failed attempt chưa thay đổi production
đo execution reliability; thay đổi đã tới production nhưng gây sự cố vẫn là
production change và được incident-attributed. Không loại bản lỗi chỉ vì thiếu
successful health check: fact ghi `production_effective_at` với runtime evidence;
incident có thể xác nhận change đã có hiệu lực. Denominator gồm verified delivery
và effective production change có evidence; report tách hai nhóm.

## 6. Metric catalog: DORA

Tất cả báo cáo dùng `[from,to)` UTC, duration giây, rate gồm numerator và denominator.
Báo cáo theo service/stream, chưa trộn team hoặc service khác nhau thành một score.

| ID | Công thức và cohort | Nguồn và điều kiện |
|---|---|---|
| D1 Change lead time | Với mỗi commit lần đầu đạt VERIFIED ở stream: verified_at − committed_at; cohort theo verified_at, median/P90 | Artifact/commit provenance + acceptance evidence. Commit qua staging không kết thúc timer; cùng commit redeploy không tạo sample mới |
| D2 Deployment frequency | Số delivery có effective production change duy nhất trong kỳ / số ngày lịch của kỳ; kèm count theo ngày/tuần | Bao gồm rollback/hotfix làm đổi trạng thái; dedup delivery_id; no-op bị loại |
| D3 Failed deployment recovery time | restored_at − impact_started_at cho mỗi failure episode do deployment gây ra; cohort theo impact_started_at | Incident attribution được xác nhận và recovery evidence; unresolved hiện count + elapsed riêng, không gán 0 hoặc bỏ khỏi report |
| D4 Change fail rate | Số effective production deliveries cần immediate intervention / tổng effective production deliveries trong kỳ | Incident xác nhận quan hệ gây lỗi, yêu cầu rollback/hotfix hoặc xử lý ngay. Một delivery có nhiều incident vẫn đếm một lần |
| D5 Deployment rework rate | Số effective production deliveries không kế hoạch phát sinh do incident / tổng effective production deliveries trong kỳ | Explicit purpose INCIDENT_REMEDIATION + linked incident + unplanned=true. Rollback action đơn thuần không chứng minh rework |

Failure episode thuộc service và có thể link nhiều deliveries/incidents; một episode
chỉ có một recovery duration, tránh nhân mẫu do alert duplicate. Impact start là
mốc sản phẩm bị ảnh hưởng theo evidence, không phải lúc mở ticket. Restored là
mốc phục vụ được xác minh theo policy, không phải đóng ticket hoặc deploy success.
Nếu không xác định được attribution hoặc impact time, đánh dấu unknown.

D1 có thể nhiều commit trong một release: mỗi commit một sample, dedup theo
repository + SHA + service + stream. P90 nearest-rank chỉ hiển thị khi n≥20;
ít hơn hiển thị median/range/n. D3 hiển thị phân bố mẫu resolved và số open,
không tuyên bố median của resolved đại diện toàn bộ failures.

D4/D5 cần nguồn incident được khai báo active cho service và review xác nhận
classification. MVP cửa sổ quan sát sau release 7 ngày, cấu hình/version được;
đây là thời gian thu evidence, không tự kết luận incident trong 7 ngày là do deploy.
Bucket mới là provisional; late incident vẫn sửa attribution sau cửa sổ và tạo
revision. Không có incident connector hoặc xác nhận manual coverage thì rate là
unavailable/partial, không hiển thị 0% như đã chứng minh không có sự cố.

## 7. Metric catalog: platform

Task template khai báo scope, acceptance policy, deployment goal, tiêu chí retry,
cohort (baseline/platform), infrastructure mode (existing/new), execution mode
(fake/live), environment và người tham gia pseudonymous. Task có trạng thái
OPEN, SUCCEEDED, FAILED, CANCELLED; acceptance verification riêng. Failed/cancelled
và task chưa kết thúc xuất hiện trong report, không chỉ lấy các mẫu thành công.

| ID | Công thức | Thu thập và giới hạn |
|---|---|---|
| P1 Time to first working deployment | first acceptance_passed_at − onboarding_task.started_at | Bắt đầu task rõ ràng, không dùng login hoặc Application created làm mốc thay thế. Report successful duration và completion/failure/open rates |
| P2 Hands-on effort | Tổng thời lượng work sessions thực tế của developer và PE trên task | Start/pause/stop hoặc nhập time log có evidence, actor và source; idle timeout gợi ý xác nhận, không suy từ page-open. Overlap cùng người union interval; nhiều người cộng person-minutes |
| P3 Self-service success rate | Successful tasks có xác nhận không cần PE can thiệp / toàn bộ eligible tasks bắt đầu trong kỳ | Denominator gồm failed/cancelled; open tạo provisional. Hỏi assistance lúc complete, UNKNOWN không tính là không hỗ trợ. Report completion và assistance coverage riêng |
| P4 First-attempt success rate | Eligible tasks hoàn thành acceptance bằng first attempt không cần sửa cấu hình/retry / toàn bộ eligible tasks trong cohort | Version task/config + attempt ledger kể cả rejection trước khi Deployment được tạo. Automatic retries ghi riêng; policy nghiêm ngặt tính là retry. Infrastructure retry có nhãn nguyên nhân |
| P5 Time to identify correct cause | confirmed_diagnosis_at − failure_notified_at | Incident/task diagnosis nhập tay hoặc ticket source; cần confirmation/evidence đúng nguyên nhân. Report unknown/unresolved; mở trang graph không phải chẩn đoán thành công |
| P6 Platform operating cost per active app/team | Allocated platform operating cost trong kỳ / distinct active app hoặc team trong cùng kỳ | Actual infra billing + PE maintenance/support person-hours × configured internal cost rate; metadata allocation version/currency/period. Workload/cloud bill tách khỏi platform cost; không dùng estimate làm actual |

P3 assistance gồm chat, pair, ticket và thao tác thay developer; role của người bấm
Deploy không đủ. Work sessions PE vừa hỗ trợ task vừa bảo trì không được double
count: cost category và allocation duy nhất. Active app là có eligible task hoặc
effective production delivery trong kỳ; team membership version tại activity.
Mẫu số bằng 0 trả unavailable. Baseline có thể nhập cùng model với source MANUAL,
không tạo deployment production giả để hợp thức hóa dữ liệu.

P6 phân bổ shared cost theo chính sách cấu hình được (MVP chia đều active apps),
luôn công khai unattributed cost và coverage. Team costs roll up allocations của
apps, không đếm hai lần app chung; tỷ giá có source/date, thiếu tỷ giá không cộng
nhiều currency. Setup/migration cost báo riêng với recurring operating cost.

## 8. Logical data model đề xuất

Các tables dưới đây chưa phải migration; UUID PK, timestamptz UTC, scoped FK
và organization_id NOT NULL cho mọi tenant-owned record. FK phải bảo đảm cả scope,
không chỉ reference UUID. PostgreSQL là primary implementation, memory cho tests.

| Entity/table | Trường/constraint chính |
|---|---|
| measurement_policies | id, organization_id, version, production streams, acceptance rules, observation window, metric definition version; immutable versions |
| measurement_services | stable id, application_id, workload identity, active period; team membership có effective_from/to |
| measurement_events/outbox | id, source/source_event_id UNIQUE trong org/source, aggregate sequence UNIQUE, occurred/received_at, schema version, typed sanitized payload, processing lease/retries |
| release_manifests | delivery_id, service_id, build/source refs, image digests, provenance status, policy_version; digest→commits qua bảng nối |
| release_commits | repository_id, sha, committed_at, source_verified; UNIQUE org/repository/sha |
| production_deliveries | delivery_id UNIQUE trong org/stream, service_id, environment_id, generation, purpose, unplanned, effective_at, verified_at, acceptance evidence; attempt links riêng |
| task_runs | task_id, template/version, cohort, mode, actor pseudonym, start/end, status, config revision, assistance classification |
| task_attempts | task_id, attempt_index UNIQUE, request idempotency key, deployment_id nullable, outcome, config revision, rejection/retry reason |
| work_sessions / assistance_records | task_id, actor, start/end hoặc duration/manual source, role/category; assistance event evidence |
| failure_episodes / incidents | service_id, impact/notified/diagnosed/restored times, status, evidence source; episode dedup key |
| incident_delivery_links | incident/episode, delivery, relation CAUSED_BY hoặc REMEDIATED_BY, confirmed_by/time, attribution revision |
| acceptance_evidence / corrections | service/task/delivery refs, check type, release/target identity, observed_at, result, evidence reference; correction lineage |
| cost_entries / cost_allocations | period, currency, actual/estimate, category, external entry ID unique, amount numeric >=0, allocation policy và target |
| metric_snapshots / projector_checkpoints | metric/scope/window/definition version + as_of + revision UNIQUE; numerator/denominator/n/coverage, source watermark |

Indexes: event processing status/next_retry; org/service/effective_at;
org/task/cohort/started_at; incident service/impact time; commit artifact mappings.
Pagination ổn định theo timestamp + ID, query range giới hạn 366 ngày, export async
khi >10.000 rows. Snapshot giữ metric version và source cutoff, raw evidence cho
recompute; policy mới không âm thầm viết lại report cũ.

## 9. API contracts đề xuất

Paths dưới đây là design, phải đồng bộ naming/auth pattern của HTTP layer trước
implementation. Queries lấy org từ session/service token, không tin query org.

| Operation | Contract |
|---|---|
| POST /api/measurement/release-manifests | CI ingestion: delivery/build/service, immutable image digests, verified commit manifest; Idempotency-Key |
| POST /api/measurement/tasks | Start task, template, cohort, infrastructure mode; server actor/start time |
| POST /api/measurement/tasks/{id}/attempts | Ghi attempt trước validation/deploy; authenticated correlation với deployment |
| POST /api/measurement/tasks/{id}/work-sessions | Start/stop hoặc manual duration; chống overlapping double count |
| POST /api/measurement/tasks/{id}/completion | Outcome, acceptance refs và assistance confirmation; revision/If-Match |
| POST /api/measurement/incidents | Impact, affected service, attribution evidence; state OPEN |
| POST /api/measurement/incidents/{id}/diagnosis | Confirmed cause + time/evidence, không lấy failureReason làm confirmation |
| POST /api/measurement/incidents/{id}/recovery | Restore evidence, time, remediation deliveries; optimistic concurrency |
| POST /api/measurement/cost-imports | CSV/JSON validated, source IDs, currency/category; preview errors trước commit |
| POST /api/measurement/corrections | Versioned correction + reason, no raw evidence delete |
| GET /api/measurement/summary | scope/from/to/cohort/mode; metric value, unit, n, numerator/denominator, coverage, completeness, as_of, provisional, definition_version |
| GET /api/measurement/evidence | Scoped paginated drill-down, exclusion reasons; no secrets |
| POST /api/measurement/exports | Snapshot-pinned CSV job; export owner/org và expiry |

Mutation cùng Idempotency-Key/payload trả kết quả cũ; cùng key khác payload 409.
Invalid 400, unauthorized 401, forbidden 403, missing/out-of-scope object 404,
conflicting revision 409. Missing metric trả value=null + reason, không trả 0.
Mỗi response query cung cấp ingestion lag và coverage breakdown theo source;
coverage D1 = attributable commits/deliveries có provenance, đồng thời hiện
unknown deliveries để không che thiếu dữ liệu bằng denominator đã lọc.

MVP authorization dựa trên auth/scoping hiện có: developer ghi task/incident của
application được phép; PE/Admin cấu hình policies và cost, xác nhận attribution.
Mọi role vẫn bắt buộc org isolation. Đây không phải claim production-grade RBAC đã
có; implementation phải kiểm tra authorization thực tế và test từng endpoint.

## 10. Console và báo cáo

Trang Measurement có hai tab DORA và Platform, bộ lọc Application/service,
production stream, kỳ đo, baseline/platform, infrastructure mode và fake/live.

Mỗi card hiện value/unit, n hoặc numerator/denominator, trend, coverage và badge
complete/partial/provisional/unavailable. Click mở evidence gồm included/excluded,
lý do exclusion và lineage; không chỉ một biểu đồ tổng hợp.

Workflow task: chọn tác vụ → bắt đầu → theo dõi attempt → ghi công sức/hỗ trợ →
acceptance check → kết thúc. Time logging là tùy chọn, thiếu thì P2 partial.
Workflow incident: khai báo ảnh hưởng → xác nhận deployment gây lỗi → xác nhận
nguyên nhân → ghi remediation → xác minh phục hồi. Không bắt developer tick một
checklist analytics để được deploy; metadata thiếu được thể hiện qua coverage.

Compare view: cùng template/version, service, acceptance và infrastructure mode;
hiện before/after, sample size, failure/open counts và effort nguồn manual/timer.
Mức cải thiện duration = (baseline median − platform median)/baseline median;
baseline=0 không tính %. Export lưu filters, policy/formula version, sample list,
source mode, missing data và as_of. Không dùng fake/live lẫn nhau hoặc so cold EKS
provision với warm cluster. Không xếp hạng năng suất cá nhân.

## 11. Data protection và vận hành

Không thu Score đầy đủ, resolvedInputs, resource outputs, Terraform state,
credential, secret, nội dung gõ phím hoặc session replay. Payload dùng opaque ID,
error category và digests allowlisted; evidence URL chỉ nhận schemes/domains
cho phép, backend không tự fetch arbitrary URL. CSV escape formula injection.

Đề xuất retention mặc định: facts/evidence sanitized 13 tháng, delivery inbox
sanitized 30 ngày, snapshots aggregate 24 tháng; cấu hình org, pseudonymize actor
và lưu cost rate hạn chế quyền. Retention purge khiến drill-down không còn đầy đủ
phải hiện evidence_expired; snapshot giữ n/as_of, không giả vờ còn recomputable.

Worker metrics: processing lag, pending/quarantined count, retry/dead-letter count,
projection duration. Retry exponential bounded; exhausted vào dead-letter với
replay có audit. Unavailability hiện rõ trên Console. Historical backfill chỉ từ
facts thực sự có, source BACKFILL và coverage riêng; không suy effort, commit hoặc
absence of incidents từ deployment history.

## 12. Lộ trình và acceptance criteria

| Phase | Deliverable | Exit criteria |
|---|---|---|
| A Evidence foundation | Stable service/stream, policies, ledger/outbox, task model, scoped queries | Restart/duplicate/reorder/correction replay cho kết quả như nhau; no secret; org isolation |
| B Platform pilot | Task UI, acceptance, effort/assistance, P1–P5 và CSV baseline | Cùng scenarios trước/sau có full attempt/failure evidence; unknown không thành success/zero |
| C Production delivery | CI provenance, runtime digest checks, production acceptance, D1/D2 | Retry/no-op/staging/canary không inflate; multi-container provenance verified; missing data visible |
| D Incidents | Manual incident workflow, optional connector, D3–D5 | Open episode, duplicated alerts, late incidents và remediation attribution tính đúng |
| E Cost/comparison | Actual cost import, allocation, P6, snapshot exports | Shared costs/person-time không double count; currency/coverage đúng; report tái lập |

Không tự kích hoạt các phase này như M04 work order. Thiết kế chưa cần cloud
mutation; live acceptance sau này cần đúng scope và authorization theo runbook.

Validation khi implement: domain table-driven fixtures cho công thức; PostgreSQL
transaction/outbox/restart tests; auth isolation; API idempotency; browser evidence
cho task/incident/report. Ví dụ fixture bắt buộc:

1. Commit 09:00 → staging 09:10 → production verified 10:00 → redeploy 10:20:
   D1 một sample 3.600s; D2 hai changes chỉ khi redeploy thực sự đổi state, no-op zero.
2. Một release có 3 failed attempts chưa ảnh hưởng production, lần 4 verified:
   D2 một; P4 fail; không tự gán D4 failure nếu không có production impact.
3. Hai effective production deliveries, một cần immediate intervention; một trong
   hai là unplanned remediation: D4=1/2, D5=1/2, xác nhận attribution riêng.
4. Incident impact 11:00, ticket 11:10, diagnosis 11:20, restore 11:40:
   D3=40 phút; P5=10 phút nếu notification 11:10; close ticket không đổi recovery.
5. Người A sessions 10:00–10:20 và 10:10–10:30, người B 10:00–10:15:
   P2=45 person-minutes; task elapsed 30 phút, không đồng nhất hai metric.
6. 10 eligible tasks: 6 self-service successful, 1 assisted successful, 1 unknown
   assistance successful, 1 failed, 1 open: P3=6/10 provisional; assistance coverage
   công khai. Không chuyển task unknown thành tự phục vụ.
7. Missing incident source: D4/D5 unavailable; denominator zero: null; fake check
   không thành verified live; actual bill thiếu: P6 partial/unavailable.

Các lựa chọn mặc định để review: task-based onboarding; stream-level dedup;
manual incident + CI manifest trước vendor connectors; 7-day observation;
retention như mục 11; equal allocation cho shared platform cost. Có thể điều chỉnh
trước implementation, mọi thay đổi phải version policy để giữ lịch sử giải thích được.
