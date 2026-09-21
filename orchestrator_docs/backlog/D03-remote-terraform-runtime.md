---
id: D03
artifact: backlog-item
status: deferred
last_reviewed: 2026-09-21
---

# D03 — Remote Terraform source execution

Planner/inspector có thể nhận dạng source identity dạng `url[@rev][/path]`, nhưng
runtime executor chỉ cho phép embedded modules `vpc`, `eks` và `aurora`.

Trước khi hỗ trợ remote source cần quyết định allowlist/trust, immutable revision,
download cache, integrity verification, credential boundary và cleanup/state
ownership. Không coi inspector support là authorization để execute remote code.
