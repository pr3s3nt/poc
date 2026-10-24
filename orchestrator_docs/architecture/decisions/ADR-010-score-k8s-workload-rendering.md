---
id: ADR-010
artifact: architecture-decision
status: current
last_reviewed: 2026-10-24
superseded_sections: [Decision-1-renderer-choice, Decision-10-optionality, internal-only-scope]
related: [ADR-014]
---

# ADR-010 — Definition-selected rendering với score-k8s

Status: Accepted, thay một phần bởi ADR-014. Bản quyết định 2026-10-06 đầy đủ
ở `git show a897d6b:orchestrator_docs/architecture/decisions/ADR-010-score-k8s-workload-rendering.md`
và `3367f7e` cho checkpoint Bước 1.

## Phần được thay

ADR-014 thay lựa chọn renderer ở Decision 1, tính tùy chọn ở Decision 10 và
giới hạn internal-k8s: score-k8s trở thành renderer bắt buộc trên internal-k8s
và aws-eks. Không có matching Definition/bundle hoặc rendering lỗi thì dừng.
Bản này không sở hữu account/provider/state Terraform; không ghi supersession
Aurora hay module nguồn vốn thuộc ADR khác.

[ADR-016](ADR-016-definition-selected-workload-targets.md) extends workload
Definitions from render-only selection to lifecycle policy and explicit typed
placement. score-k8s still performs render/generate and controlled patches;
existing adapters perform apply/readiness/update/remove. Platform-managed patches
remain sufficient for V03; PE-authored patch configuration is deferred in
[issue5](https://github.com/pr3s3nt/poc/issues/5).

## Các quyết định còn hiệu lực

1. Definition Type workload chọn rendering bundle; Orchestrator sở hữu matching
   và pin Plan. Theo ADR-014 chỉ score-k8s là lựa chọn hợp lệ, không thay UC-08
   infrastructure provisioning bằng renderer.
2. UC-08 sở hữu cluster/namespace/database/cache identity, state và outputs.
   score-k8s không tạo credentials/tài nguyên hoặc destroy chúng.
3. Giữ `modules.*`, workload class default và weighted five-field matching.
   Không công bố Humanitec Generic Workload/API compatibility.
4. Rendering port tách khỏi delivery. Direct/Fleet/readiness, per-workload
   result, routes và current-set commit giữ ở UC-06 services.
5. Bundle ID là selector do platform quản lý; Plan pin exact renderer/binary/
   provisioner/patch digests. Không upload commands/templates/mutable URLs.
6. Chuẩn hóa Score nội bộ, giữ probes/replicas/Service references/CPU-memory/
   UC-12 secrets/routes; không tự mở rộng public Score subset.
7. Preview không provision hoặc khẳng định final manifests khi outputs chưa
   có. Deploy dùng pinned intent, resolve rồi validate manifest trước apply;
   intent/version đổi làm Preview stale.
8. Workspace score-k8s riêng tư, dùng xong dọn; không publish `.score-k8s`.
   Names do product identity quyết định, không do random renderer GUID.
9. sourceFingerprint ở registration là audit, không runtime equality gate.
   Sau platform nâng bundle A -> B tường minh, Plan mới pin B; token A stale
   và có renderer-only update. Không thêm Definition overwrite/retire API.
10. Không fallback renderer khi thiếu/lỗi score-k8s. Native policy helpers có
    thể dùng để kiểm protected fields/auxiliary delivery, không sinh ứng dụng
    thay score-k8s.

## Ownership và verification

Planner chọn Definition/graph; UC-08 cung cấp validated outputs/Secret refs;
rendering adapter chuẩn hóa Score và protected fields; score-k8s sinh manifests;
UC-06 delivery/readiness/routes và persistence commit giữ nguyên trách nhiệm.
Contract chi tiết: [workload rendering](../contracts/workload-rendering.md).
Execution scope/điểm duyệt nằm ở M04 và acceptance runbook; implementation/evidence
0.15.0 internal bundle cũ nằm trong CURRENT_STATE/verification, không là giới
hạn required behavior. Exact phiên bản v2 thuộc immutable release manifest.

## Sources


- [score-k8s CLI](https://docs.score.dev/docs/score-implementation/score-k8s/cli/)
  documents initialization, manifest generation and potentially sensitive local state.
- [Provisioners](https://docs.score.dev/docs/score-implementation/score-k8s/resources-provisioners/)
  document output-only integration mechanisms and encoded Kubernetes Secret references.
- [Patch templates](https://docs.score.dev/docs/score-implementation/score-k8s/patch-templates/)
  document post-generation patches; their syntax is distinct from the product Delta JSON Patch.
- [Humanitec Generic Workload inputs](https://developer.humanitec.com/platform-orchestrator/docs/platform-orchestrator/working-with/workloads/)
  explain the conceptual separation of workload specification and driver execution.

## Git rendering amendment — 2026-10-24

[ADR-021](ADR-021-git-render-bundles.md) supersedes installed-only/Go-owned
rendering policy and exact-three-workload-variable restrictions for Git bundles.
Legacy installed pins retain their contract. Terraform provisioning, dependency
matching, target/state/ownership and cleanup rules in this ADR are unchanged.
Git behavior, inputs, cache and versioning follow the linked contract; this
amendment does not claim implementation or authorize retained-data migration.
