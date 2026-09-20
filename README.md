# Internal Platform Orchestrator

Thiết kế và implementation plan cho orchestrator nội bộ tương thích khái niệm/contract Humanitec, phát triển theo Unified Process.

## Current status

- Phase 1–2: specifications và planner-reference analysis hoàn thành.
- Phase 3–5: use-case realization, shared architecture, traceability và design gate hoàn thành.
- Phase 6: Go implementation chưa bắt đầu.

## Start here

1. [Agent workflow](AGENT.md)
2. [Development plan](plan.md)
3. [Documentation index](orchestrator_docs/INDEX.md)
4. [Design gate result](orchestrator_docs/traceability/design-gate.md)

Toàn bộ diagram dùng PlantUML; `.puml` là source chuẩn. Lõi orchestrator/API/executor dùng Go; Orchestrator Web Console dùng React + TypeScript. Không dùng Python cho code sản phẩm.
