---
id: UC-03-UI-MODULE-FORMS-PROTOTYPE
artifact: use-case-ui-prototype
status: current
last_reviewed: 2026-10-22
related: UC-03
---

# Mock module forms — issue #11

Mock desktop người dùng duyệt ngày 2026-10-22 là visual reference cho
[issue #11](https://github.com/pr3s3nt/poc/issues/11). Behavior chuẩn nằm ở
[UI11-01..05](../specification.md); mock không thay domain/backend contract.

- [Hướng dẫn mở và giới hạn mock](../../../../frontend/design/resource-definition-module-forms/README.md)
- [Giao diện HTML](../../../../frontend/design/resource-definition-module-forms/index.html)
- Ảnh: [AWS](../../../../frontend/design/resource-definition-module-forms/screenshots/aws.png),
  [Kubernetes](../../../../frontend/design/resource-definition-module-forms/screenshots/kubernetes.png),
  [chọn nguồn](../../../../frontend/design/resource-definition-module-forms/screenshots/source-picker.png),
  [tài nguyên đi kèm](../../../../frontend/design/resource-definition-module-forms/screenshots/companions.png)

Snapshot nguồn: branch `design/module-forms-mock-20261022`, commit
`41b69c6451839eccbdb1bdd037d0c180240a9de3`; module pin
`6b991b16c39ac8485c74fa6756cf3223a129c8c9` của `pr3s3nt/terraform-modules`.

Account/class/mapping mẫu, catalog hard-code, output catalog đơn giản, JSON
summary, validation một phần và localStorage save của mock không phải behavior
sản phẩm. Sản phẩm dùng API/metadata thật và giữ kiểm tra server.
