---
id: D06
artifact: backlog-item
status: deferred
last_reviewed: 2026-09-22
---

# D06 — Humanitec Resource Definition and Score boundary compatibility

Happy path hiện dùng contract nội bộ đủ cho seed-backed AWS EKS và internal
Kubernetes. Contract này chưa phải public Humanitec/Score-compatible boundary.

Trước khi công bố compatibility phải quyết định và test:

- ánh xạ driver ID có namespace sang executor enum nội bộ;
- ánh xạ `driver_account` sang Connection và thiết kế
  `driver_inputs.secret_refs`/secret lifecycle;
- remote Terraform `url`/`rev`/`path` so với extension `source.module`;
- danh sách context placeholder chuẩn và namespace cho các extension nội bộ;
- Score probe nested `httpGet` và cách xử lý extension top-level `replicas`;
- migration output namespace từ `name` sang `namespace`;
- request/response compatibility, validation errors và contract tests.

Không đổi seed/code chỉ để đổi tên field trước khi có migration và adapter
boundary. D03 vẫn sở hữu runtime/trust model của remote Terraform source; D06
chỉ sở hữu external contract/mapping.
