---
id: VERIFICATION-2026-09-28-PUBLIC-ROUTES-RECHECK
artifact: verification-evidence
status: current
last_reviewed: 2026-09-28
---

# Public routes and no-op Preview recheck on kind

- Direct script: `backend/test/integration/public-ingress-kind-verify.sh` passed
  with two BusyBox workloads on one host. Traefik served `/` from `probe` and
  `/api` from `api`; deleting `probe` preserved `/api`, deleting `api` removed
  the Ingress. Re-saving the deployed `probe` Score yielded zero Preview changes
  and its Pod UID did not change. Run `route-20260928114800-15830` exited 0;
  run-scoped namespace was deleted.
- Fleet script: `backend/test/integration/fleet-gitrepo-kind-verify.sh` passed
  with Harbor BusyBox image, `_routes/ingress-orch-public.json` in GitOps repo,
  route-hash and Fleet objectset metadata on the cluster Ingress, and Fleet
  prune of Ingress and route bundle on removal. Run
  `fleet-20260928114446-11179` exited 0; run-scoped namespace was deleted.
  Verification pushed run-scoped manifests/removal commits to the dedicated
  GitOps repository; its final worktree and tracked desired state were clean.
- An earlier Fleet attempt failed because a 64-character SHA-256 value exceeded
  the Kubernetes label limit. The implementation now uses a 32-character
  route-hash label. The failed run's workload and route bundles were removed
  through Preview/Deploy using its saved state; its namespace was deleted.
- Go tests/build, React typecheck/lint/tests/build and documentation checker
  passed after the implementation change.

This does not verify the Backstage image, guest auth endpoint, DNS/TLS or the
specific "toggle public then back" Pod-UID flow. The no-op draft and route
transfer are separately covered by backend tests.
