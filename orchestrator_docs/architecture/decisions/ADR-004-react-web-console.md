---
id: ADR-004
artifact: architecture-decision
status: current
last_reviewed: 2026-09-23
---

# ADR-004 — React web console served from the Go backend origin

Status: Accepted
Date: 2026-09-20

## Context

UC-00 đến UC-09 cần giao diện cho sign-in, catalog, preview, deploy và quan sát trạng thái. Reference `final_idp/idp/frontend` tại repository `https://github.com/pr3s3nt/final_idp.git`, branch `uc03-impl`, commit `e6dc6631ba9db1bc80e2ff56380f50db99d490f9` đã chứng minh một cấu trúc nhỏ, feature-oriented và có thể được Go backend phục vụ cùng origin. Dự án này đồng thời có một acceptance application frontend được triển khai lên Kubernetes; đó là artifact khác với giao diện quản trị.

## Decision

1. Orchestrator Web Console nằm trong `frontend/`, dùng React, TypeScript strict và Vite.
2. Source được chia thành `app`, `features`, `shared` và `styles`. Feature bám use case và sở hữu API client, local draft/reducer, page và component của nó.
3. Browser gọi typed JSON API của Go backend dưới `/api/v1/` cùng origin. Production bundle được Go server phục vụ dưới `/ui/`; Vite proxy `/api` trong development.
4. Ban đầu dùng History API router và React state/reducer, không thêm router, global state hoặc UI framework khi chưa có nhu cầu rõ.
5. Vitest, Testing Library và jsdom kiểm tra reducer, validation, API client và page states. Go handler/integration tests kiểm tra server-side API contract và business rules.
6. MVP có UC-00 internal-account sign-in qua same-origin opaque session. Fine-grained RBAC và secret-management UI không thuộc MVP; UI không hiển thị hoặc persist secret value.
7. `final_idp/idp/frontend` tại exact Git provenance nêu trên chỉ là nguồn tham khảo về cấu trúc và delivery; repository local không phải dependency và không được copy nguyên domain, API contract, authentication flow hoặc source code.
8. Acceptance application frontend là Kubernetes workload riêng, không nằm trong `frontend/` và không dùng chung source với web console.

## Consequences

- Repository có Go và Node toolchain ở build/test time, nhưng Go server không cần Node.js ở runtime.
- Same-origin delivery tránh CORS và tạo một ranh giới contract rõ giữa frontend/backend.
- Mỗi Phase 6 increment phải hoàn thành cả API behavior và UI state tương ứng; frontend không bị dồn thành một phase trang trí cuối cùng.
- `node_modules/`, `dist/`, coverage và TypeScript build-info không được commit.
