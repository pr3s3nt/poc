---
id: I06-07-WORK
artifact: iteration-work-items
status: historical
last_reviewed: 2026-09-22
related: I06-07
---

# I06-07 work items

## Implementation order

1. Add Score parser tests for requests/limits CPU and memory.
2. Extend typed Score/environment module models without relaxing unknown-field
   rejection.
3. Preserve values in Score conversion and Candidate Deployment Set.
4. Carry requirements through workload binding to Kubernetes renderer.
5. Replace unconditional hard-coded requests with declared values plus explicit
   omitted-value policy.
6. Add manifest integration tests and run kind verification/cleanup.
7. Update implementation-owned documentation and remove IMP-009 after pass.

## Handoff checklist

- [x] No CPU/memory Resource Graph nodes were introduced.
- [x] Declared requests and limits survive every boundary.
- [x] Omitted resources follow one documented default policy.
- [x] Invalid/unknown resource fields remain rejected.
- [x] Backend regression, conformance and kind verification pass.
