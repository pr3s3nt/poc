---
id: VERIFY-DEFINITION-SOURCE-PROVENANCE-20261104
artifact: verification-record
status: current
last_reviewed: 2026-11-04
---

# Saved Definition source provenance

UC-03 SP-01–03: Git/Catalog/ZIP/Inline origin summary, repository/path/full pin,
explicit Change source, and accurate Registered sources label for prepared
workload templates. Parameter-only saves preserve pins; missing saved modules
cannot silently select the sole newer module. Existing cluster method changes
are explicit and preserve per-method drafts. Shared shell/cards/buttons remain.

129 relevant frontend tests pass across Definition/Git/Catalog/template/cluster,
localization and application-shell coverage. Typecheck, lint and build pass.
The full frontend suite was run; two historical Score scenario contract failures
(`terraform-v2-labels`, `v04-internal-full`) were independently reproduced in a
temporary archive of unchanged `b6fd702a`, without touching the working checkout.
Initial failures in the changed feature were corrected and their focused suites
rerun successfully. No backend/schema/deploy/resource changes for this UI fix.

Built `orchestrator-frontend:source-provenance` and refreshed only the frontend
of `git-bootstrap-20261104`. Browser checked the actual Tailscale route:
[Resource Definitions](http://desktop-bdjp2rq-1.tail342836.ts.net:3043/ui/platform/resource-definitions).
Reopened `git-dev-postgres`: Git repository, module path and full commit visible,
replacement controls closed until Change source. Reopened `git-dev-workload`:
Git template origin visible. Both views matched the existing UI shell and card
style. Before/after Definition response hashes match; no non-auth mutations.
Private evidence: `/home/thanhnt1/.local/state/issue33-git/source-ui-browser.json`,
`definition-before.png`, `definition-after.png`, `template-source-after.png`.
The saved-source summary is concise; full hashes/provider details remain folded.
App/data/forward retained; no acceptance rerun or Pod recreation for the UI fix.
