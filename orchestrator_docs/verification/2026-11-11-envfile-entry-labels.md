---
id: VERIFY-2026-11-11-ENVFILE-ENTRY-LABELS
artifact: verification
status: evidence
last_reviewed: 2026-11-11
related: [UC-16]
---

# Environment editor entry labels — issue #72

User requested removing the UI label and using Nhập từ file .env. The variable
table is now the default view with one quiet action on the right. Text editing
shows Quay lại bảng (EN Back to table / Import from .env file). Removed tab roles
and arrow-key handling; native button Enter/Space and expanded/control semantics
apply. Invalid text still prevents returning to a stale table or saving.

46 existing focused editor/table tests, typecheck, lint (one preexisting warning),
production Docker build, documentation checker, diff check and runner Node syntax
pass. Browser checks used the real retained feature Console through its exact
Tailscale URL in VI/EN, with keyboard switching, 36 saved rows and the metadata
reconciliation/cancellation assertion; zero workload mutations occurred.

Visually compared against the previous editor screenshot and shared shell/styles:
no UI tab remains; the file entry is above the table on the right. Extra syntax
help remains collapsed. The feature stack and review draft remain available.

No backend/data/Secret lifecycle change. No new Go or full kind suite was run for
this presentation-only refinement; the earlier full-check/baseline evidence is in
[the feature record](2026-11-11-workload-envfile.md). Required full checks must run
again before merge under AGENTS. No merge/main deployment was performed.
