---
id: VERIFY-20261118-INTEGRATED-ROUTES-EDITOR
artifact: verification-record
status: current
last_reviewed: 2026-11-18
---

# Integrated Gateway/routes and Definition editor — issues #84/#87

The user approved combining the two trial branches before acceptance. Merge
`35cbc638` joins `features/definition-code-editor` at `96d59826` into
`features/Httproute`, incorporating main `725541ef`. It does not merge either
feature into main. Earlier isolated verification records retain their original
source attribution; they are not combined-run evidence.

## Integration and focused checks

Gateway Echo/legacy hydration is inside the shared editor's hydrate callback;
opening establishes the editor context first. Reset clears both editor and
Gateway/reference state. The Gateway heading/class controls and the code editor's
busy navigation guard remain. English/Vietnamese message catalogs were merged
as dictionaries with matching shared values and no duplicate keys.

32 focused Definition editor/Endpoint/mapping tests pass, including three new
integration cases for Echo Gateway, legacy Gateway and reference-only Endpoint.
Focused planning/catalog/provisioning/Kubernetes/template-driver Go packages and
Go build pass. Frontend typecheck/lint/build, Node syntax, documentation checker
and whitespace checks pass. Existing PlatformSettingsPage lint and bundle-size
warnings remain. No diagram changed; local PlantUML is unavailable.

## Preserved trial installation

The existing task Console is rebuilt from the merged source with its own Docker
PostgreSQL/Vault volumes. Two private custom-format database backups were checked
with pg_restore's archive listing while the old backend writer was stopped, before
owner setup. The normal released owner upgrade advances schema v10→v11; runtime
does not migrate. Installation, Organization, local user, connection, Application
and active-resource identities/status/versions match the selective pre-upgrade
snapshot. Authenticated catalog/application API responses match exactly before
any deliberate edits. Existing password still authenticates. No released
migration, marker, checksum or Terraform state was modified.

Native checks after upgrade confirm the existing one PostgreSQL/PVC is shared by
both workloads, authenticated SELECT 1 and both init clients succeed, two routes
have the expected hosts/Services/parents/listener and Accepted/ResolvedRefs, and
recorded resource/Gateway UIDs remain unchanged. No shared Gateway, Traefik,
cluster, DNS or TLS objects are changed. The separate #87 sample stack is retained.

## Integrated browser editor

The browser opens the exact Tailscale Console URL, authenticates and reads real
catalog APIs. For legacy Gateway/Endpoint it round-trips form→YAML→form read-only,
retaining driver, reference class, values and version. For the owned Echo review
Gateway and reference Endpoint it edits the display name in the form, confirms
JSON, supplies YAML, returns to the form and saves, then saves JSON. Each Save
adds exactly one proposed revision. Driver/account/input pins, criteria,
provision and active revision remain unchanged; no new Definitions are created.
Other Definitions and all Applications remain identical; no deploy request occurs
in the isolated editor check. Echo/Endpoint names now identify the review pair.

The first attempt completed all four round-trips/Saves but its final Application
snapshot assertion detected draftVersion changing because a separate workload
update harness had been started concurrently. This was a verification scheduling
error. Its failed log/receipt are retained. After workload authoring finished,
the complete editor check ran alone and passed, including the unchanged full
Application snapshot. The two review Definitions consequently have four proposed
name-only revisions, latest version 5, with their original active revision and
running Environment pins preserved. Legacy versions remain 1.

A second read-only browser check covers the actual cluster, Terraform namespace,
workload template and Terraform PostgreSQL Definitions on the integrated Console.
Form→JSON→YAML→form→JSON preserves the full document, including source/account
pins, and exact catalog/Application snapshots remain unchanged. No Save, new
fixture, version activation or deployment is performed by that check.

UI references are the existing signed-in Definition editor, namespace/provider
forms and AppShell/shared styles. Visual inspection confirms class selectors,
collapsed technical details and full-width code panel coexist with the existing
shell/spacing. Optional help remains collapsed; no additional explanation banner,
shell redesign or navigation change was introduced. Private screenshots preserve
both forms and code. Access was tested from this host through the actual tailnet
URL; this is not evidence for every other device's ACL.

## Runtime update and full-suite checks

Both native and Score workload updates were authored before one Environment
Preview/Deploy; it succeeded for both. SQL/init/route checks passed afterward.
The first native Pod restart was rejected by Kubernetes because the recorded
resourceVersion changed during ownership verification; no deletion bypassed the
precondition. After re-reading the stable Pod and rechecking the ownership chain,
the next guarded attempt succeeded. Replacement readiness, SQL/init/routes and
preserved PostgreSQL/PVC/Gateway/Deployment/route UIDs passed again. Browser
confirmed the exact Console Application and both real nginx web URLs over
Tailscale. The review Application and its data remain available for acceptance;
this update/restart is not a new create/delete lifecycle claim. Earlier cleanup
evidence stays attributed to the previous source.

Fresh candidate/main backend full checks are still running. Full frontend from exact `35cbc638` on
kind passes **116 files / 1102 tests**, with install/typecheck/lint/build all 0.
No completed backend comparison, cleanup or green full-backend gate is claimed
until the final results are recorded here.
