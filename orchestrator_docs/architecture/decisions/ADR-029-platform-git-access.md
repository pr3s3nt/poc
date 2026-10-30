---
id: ADR-029
artifact: architecture-decision
status: current
last_reviewed: 2026-10-30
---

# ADR-029 — Organization-scoped platform Git access

Company catalog repositories require backend authentication. User approval scopes
this change to PE/Admin-managed Git access through platform settings; general
platform URLs and other environment configuration are outside this slice.

Store one setting per Organization: HTTPS origin/path scope, auth mode, username,
optional CA, version and actor/time. Tokens are immutable objects behind the
existing credentials.Store port; product metadata contains an opaque reference.
No manual Pod Git Secret mount, ambient Git credential helper, token URL or
per-source credential selector is introduced. Anonymous Git remains available.

Authenticated catalog services propagate Organization context to module and
render transport. Match exact origin and whole path prefix, reject ambiguous
paths, disable redirects and preserve TLS verification. Pass auth in the child
process environment only; write CA to a private temporary snapshot file. Never
return/log tokens, raw Git errors or credential references. SSH is out of scope.

Test operates on draft-only access and performs a bounded Git read. Save performs
an optimistic metadata CAS independently of Test. Blank token retains existing
credential only under unchanged scope/username. Immutable old objects remain
for in-flight reads/uncertain commits; provider revocation is external and
credential garbage collection is outside this slice.

PostgreSQL version 2 adds the metadata table through owner-only setup following
[ADR-027](ADR-027-postgres-upgrades.md). Runtime never performs DDL or obtains
owner credentials. Existing installations must upgrade without reset.

[GA-01–06](../../usecase/UC-03/git-access.md) owns behavior and API/UI contract.
