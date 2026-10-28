---
id: ADR-027
artifact: architecture-decision
status: current
last_reviewed: 2026-10-28
---

# ADR-027 — Versioned PostgreSQL upgrades preserving retained data

Issue #23 and user approval replace the PostgreSQL fresh-only/reset policy of
T28 and earlier ADRs. JSON snapshots and workload databases are outside scope.

Owner setup remains separate from the least-privilege Console/State runtime.
Deploy executes setup before starting the new runtime; failure blocks startup.
Stop old writers and take a verified backup first. No retained installation is
upgraded by this implementation task without separate environment authorization.

Supported initial sources are the exact tracked SQL baselines at `1516dec`
(SHA-256 `c8d5170caaba9bce4fb41d99f562a31442a157cdb39235f6dcf2b6ff0a1eb161`)
and `2a111db` (SHA-256
`b908a4b187741913e64f8ff751499530309c8ec471484f3f0b42defaf4c328d5`).
These were identified by read-only marker queries on staging/preview. No other
historical source is inferred or adopted.

Each schema version has a frozen complete SQL reference and immutable migration.
The owner validates source marker, recorded catalog against actual structure,
and actual structure against that reference before changes. Migration runs under
the existing advisory transaction lock; DDL, history, grants and target marker
commit together. Failure rolls back all changes. Concurrent setup serializes;
repeated setup validates without changes. Installation UUID, creation time and
all existing domain/state identities survive. Existing Applications gain ACTIVE
status and empty deletion metadata; existing render bundles gain git source kind.

The version-1 reference includes owner-written `schema_upgrade_history` with
version, migration checksum, source baseline checksum and application time.
Fresh installs record version 1 as well. Both runtime roles may SELECT history,
never write it. Runtime validates the current version, immutable checksums and
recorded catalog read-only. Unknown sources, ahead-of-code versions, damaged
catalogs and altered history fail explicitly; errors never prescribe data reset.
A supported old version directs operators to owner setup.

The active baseline is compared structurally with its frozen version reference;
comments/formatting do not change the version identity. Future schema changes
require a new immutable version/reference, migration, populated-source integration
tests and documentation. CI checks historical files against Git base and tests
that the active schema matches the current version reference. Never edit a released
migration or update marker checksums manually to bypass compatibility checks.
