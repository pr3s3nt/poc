---
id: VERIFY-2026-09-29-UC04-KIND-CONTEXT
artifact: verification-record
status: current
last_reviewed: 2026-09-29
---

# UC-04 host-context verifier — 2026-09-29

`ORCH_KIND_VERIFY=1 go test ./internal/adapters/kubernetes -run
TestKindConnectionVerifierReadOnly -count=1 -v` passed against
`kind-idp-internal` in 0.96 seconds. The verifier resolved a server endpoint
and version, checked API access and read-only RBAC review for Namespace,
Deployment, StatefulSet, Service and Secret creation. It did not create or
modify cluster resources. HTTP authorization and Organization scoping were
tested with a fake verifier in the backend e2e suite; no live UI/API registration
was asserted in this record.
