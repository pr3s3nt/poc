---
id: VERIFY-20261118-ROUTES-EDITOR-MAIN
artifact: verification-record
status: current
last_reviewed: 2026-11-18
---

# Accepted main merge — Gateway/routes and Definition editor, issues #84/#87

The user explicitly accepted merge into main and waived repeated testing because
the combined trial had already been checked. Merge `6084919e` joins accepted
feature `aeeb338b` into main `3e42b2e8` and is pushed. Incoming main changes since
the tested baseline `725541ef` are documentation-only. The single conflict in the
verification index was resolved by retaining both entries. Backend/frontend/e2e/
scripts/deploy trees match the accepted feature exactly. No product source was
edited for this merge and no product test suite or functional/browser scenario
was rerun. The [combined evidence](2026-11-18-integrated-routes-editor.md) retains
its exact source and baseline failures/skips; no new green-backend claim is made.

Both feature branches are deleted locally/remotely. The separate editor checkout
is preserved at its original commit in detached HEAD, with no file change. Both
trial Docker stacks are stopped, retaining containers, volumes, credentials and
private backups. Their historical trial URLs are retired. The r7 Gateway/Endpoint
trial configuration is preserved in that trial database; it was not silently
imported into the existing main installation or applied to its Applications.

## Main runtime rollout

Existing accepted runtime images were reused and tagged
`orchestrator-backend:main-6084919e` and
`orchestrator-frontend:main-6084919e`, then imported into authorized kind. This is
image reuse by identical product source, not a fresh build/test claim. The
retained backend/frontend Deployment UIDs and specifications were captured.
Preflight found no ACTIVE/RECOVERING/INTERRUPTED Environment operation.

Writers were quiesced before a private custom-format PostgreSQL backup and
platform storage archive. Archive listings were validated; no backup restore
test was run. Existing volumes, runtime/owner credentials, Vault and OIDC
configuration were preserved. The normal owner setup Job completed before
runtime and reported baseline/grants already existed, nothing changed. No schema,
released migration, marker, Terraform state or identity was rewritten.

Only backend/frontend image references changed in the retained Deployment specs;
backend/frontend/Vault rolled out Ready. Selective installation/Organization/user/
connection/Application snapshots match before/after, including all seven visible
Application identities and the schema installation identity. This is operational
rollout/data-preservation evidence, not a repeated functional test. No main
catalog/Definition selection or Application configuration was imported or changed.

The temporary backup Pod and image-loader namespace were deleted using original
owner label/UID plus fresh resourceVersion preconditions, and their absence
confirmed. The completed owner setup Job remains as the upgrade audit. Main
services remain running. No browser login, app deploy/restart, full suite or CI
wait was performed after merge, following the user's instruction. No new UI
handoff URL is claimed verified by this merge run.
