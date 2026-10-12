---
id: UC-02-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-12
related: UC-02
---

# UC-02 HTTP mapping

`GET /api/v1/resource-types` lists the session Organization's catalog.
`POST /api/v1/resource-types` registers one contract, Platform Engineer/Admin
only. Body contains `key`, `inputs` and `outputs` in the supported MVP schema;
one bounded JSON object, unknown fields/trailing/non-object bodies rejected.
Success is 201, validation 400, duplicate 409, missing session 401, wrong role
403, oversize 413. Unknown store errors return generic 500 without raw cause.
Registration never updates an existing contract, even with concurrent requests.
New contracts are available to Score validation without restart. Runtime matching
still requires a supported Definition; registration does not add a new executor.

## Registration policy

New IDs follow specification BR-05; `environment` and `service` are reserved (BR-06). Invalid IDs return 400 without normalization; form retains user input and explains lowercase letters, digits and hyphens.

## Projection Console T04

GET cung cấp `resourceTypes` với `key`, `inputs`, `outputs`; chi tiết và copy
chỉ dùng Resource Type metadata này. Copy clone schema/flags/types vào
draft với ID mới; POST giữ body hiện tại và insert-only semantics.
Capability metadata UC-03 OC-04d theo [screens](screens.md) không tạo executor
cho custom ID. CTA điều hướng UC-03 với query `resourceType` là key kỹ thuật;
chỉ prefill sau khi GET xác nhận key tồn tại và có cặp hỗ trợ từ metadata.
Query thiếu/không hợp lệ không chọn type và không phát sinh POST. Reload hoặc
response đến muộn không ghi đè lựa chọn/form đã sửa. Không đổi API đăng ký Resource Type.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.
