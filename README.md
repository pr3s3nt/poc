# Internal Platform Orchestrator

Orchestrator nội bộ sử dụng các contract gần Humanitec, được phát triển theo
Unified Process và hiện thực bằng Go cùng React/TypeScript.

## Current status

- Phase 1–5: specification, realization, shared architecture, traceability và
  design gate đã hoàn thành.
- Phase 6 bước 1–3d: walking skeleton, kind happy path, AWS happy path,
  Humanitec-style planner conformance và đồng bộ thiết kế–implementation đã hoàn
  thành.
- Hạng mục đang hoạt động: **Phase 6 bước 4 — hoàn thiện UC-09 history, filter
  và quan sát trạng thái**.
- AWS happy path gần nhất chạy trước một số thay đổi planning cuối; phải chạy lại
  trước khi coi baseline là release-ready.

Trạng thái và giới hạn chính xác nằm trong
[Current project state](orchestrator_docs/CURRENT_STATE.md), không nằm trong
README này.

## Start here

- [AI agent workflow](AGENTS.md)
- [Documentation index](orchestrator_docs/INDEX.md)
- [Current project state](orchestrator_docs/CURRENT_STATE.md)
- [Active iteration](orchestrator_docs/iterations/M01-contract-hardening/I06-06-imp008-delta-snapshot/README.md)
- [Backend implementation guide](backend/README.md)
- [Web Console guide](frontend/README.md)
- [Project glossary](orchestrator_docs/GLOSSARY.md)

## Repository boundaries

- `backend/`: Go product module, tests và acceptance workloads.
- `frontend/`: Orchestrator Web Console React/TypeScript.
- `orchestrator_docs/`: current requirements, design, status, operations và
  verification evidence.
- `orchestrator_reference/`: technical references; read-only đối với product
  work.

PlantUML source (`.puml`) là artifact diagram chuẩn; ảnh `.png` đi kèm phục vụ
review. Product backend dùng Go; Web Console dùng React + TypeScript strict.
