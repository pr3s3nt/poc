---
id: UC-05-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-07
related: UC-05
---

# UC-05 screens

Application home provides **Preview Score** for the selected Environment.
The standalone page visibly identifies Application, Environment and the persisted Application
connection key/profile, provides
action, Workload ID, Run ID and labeled Score before/after JSON/YAML editors.
Action hides irrelevant editors; validation never silently changes input.
Warn users to use configuration placeholders instead of literal secrets.

**Preview** performs only the read-only API operation. Back returns to the
Application; it does not save input. The result shows base Set/version, plan
hash, add/update/remove Delta summary, resource classification, provider-first
batches, matched definitions, graph nodes/edges and expandable formatted Delta
and Candidate Set. An empty change is explicitly explained as no workload
change; planning artifacts may still contain the Environment graph.

No standalone Save or Deploy control. Existing pending-change panel remains
unchanged in responsibility. Long artifacts must remain readable/scrollable.
