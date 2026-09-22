---
id: I06-13-WORK
artifact: iteration-work-items
status: deferred
last_reviewed: 2026-09-22
related: I06-13
---

# I06-13 work items

## Implementation order

1. Approve activation need and resolve D03/D06 trust/contract decisions.
2. Threat-model URL/revision, archive extraction, cache and credential access.
3. Add allowlist/immutable revision/integrity validation tests.
4. Implement downloader/cache behind a narrow port and isolated workspace.
5. Integrate inspector/executor/state/cleanup with failure-path tests.
6. Run only explicitly authorized external verification and document evidence.

## Handoff checklist

- [ ] No floating/untrusted source executes.
- [ ] Archive/path traversal and cache poisoning are tested.
- [ ] Credentials and Terraform state ownership remain explicit.
- [ ] External mutation follows runbooks and user authorization.
- [ ] IMP-007 status matches actual supported runtime contract.
