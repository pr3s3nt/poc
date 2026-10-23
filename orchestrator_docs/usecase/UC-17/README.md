---
id: UC-17-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-23
---

# UC-17 context — Theo dõi và báo cáo hiệu quả platform

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md). Đợt 1 (issue #12) chỉ tính các chỉ
số platform tự đo; chỉ số cần nguồn ngoài platform là `NOT_COLLECTED` và được
liệt kê để xem xét sau trong
[issue #12](https://github.com/pr3s3nt/poc/issues/12#issuecomment-6361990505).
Đợt 1 đã implement và kiểm chứng: [verification](../../verification/2026-10-23-uc17-platform-effectiveness.md).

## Read in this order

1. [Specification và hợp đồng đo lường](specification.md)
2. [UI design](ui/README.md)
3. [Realization](realization.md), [sequence](sequence.puml) và [VOPC](vopc.puml)
4. [OC-20 HTTP contract](../../architecture/contracts/operation-contracts.md#oc-20-effectivenessservicereport--records--baselines)
5. [Schema `effectiveness_baselines`](../../architecture/database/schema.md#effectiveness_baselines)
6. [UC-09 deployment history](../UC-09/README.md): nơi xem chi tiết bản ghi nguồn

UC-17 chỉ đọc lịch sử của UC-06/07/09 và không đổi lifecycle deploy, update hay
cleanup. Ghi duy nhất là phiên bản baseline.
