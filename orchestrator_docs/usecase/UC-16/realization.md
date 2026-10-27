---
id: UC-16-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-27
---

# UC-16 — Use Case Realization

This collaboration traces the approved [specification](specification.md).
Application keys use the virtual Score `environment` resource and
`${resources.env.KEY}` syntax. HTTP and state contracts are defined in
[OC-12/OC-16](../../architecture/contracts/operation-contracts.md).

## Responsibilities

- Workload configuration boundary: one per-container environment table with
  literal/Application Variable/Secret/resource/Service sources and inline dialogs.
  Project rows into Score with lossless source conversion; secret creation input
  lives only in its UC-12 dialog. Shared dependency editor writes local form state.
- Reference catalog: list UC-12 keys for the selected Application/Environment,
  declared resource inputs/outputs/classes và shared IDs từ safe metadata,
  cùng same-Environment workload
  Services and ports.
- Workload configuration control: validate ordinary strings and selected references, save a
  desired change or mark a workload pending deletion; do not call deployment.
- Public access selection: store zero or more `{path, port}` bindings to
  declared Service ports in desired Score. Preview validates the complete
  Environment path set; saving alone never changes an Ingress.
- Desired configuration store: retain versioned pending changes separately
  from the current Deployment Set. Preview pins its draft version and UC-12
  revision so a later edit makes the preview stale.
- No-op drafts remain versioned but do not appear as affected workloads in
  Preview; a referenced UC-12 key revision change still produces an update.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-02 | Read selected Environment's workload list and open the form or Score importer. |
| MS-03 | Thu thập Workload fields, dependencies/shared IDs, class và typed Type params; giữ advanced/imported class, chặn required/input lỗi trước Save. |
| MS-04–MS-05 | Load scoped metadata, edit unified rows and create missing keys via UC-12 or dependencies in the parent draft. Restore mappings; secret values stay hidden. |
| MS-06 | Validate row source, ordinary string escaping and per-container names before serialization; then server validates Type params/references and establishes trusted literal provenance under OC-16. Retain input on failure. |
| MS-07 | Save desired configuration, return pending status and a UC-05 Preview affordance. |
| VAR-01 | Parse one imported Score, run the same validation and show the resulting form before save. |
| VAR-02 | Confirm deletion, mark desired change pending and permit Undo before deploy. |

## Boundary

UC-16 has no runtime side effects. UC-05 reads pending desired configuration to
produce preview; UC-06/UC-07 apply an approved change later against the pinned
draft version and UC-12 revision.

## Input validation collaboration

MS-06 validates all declared resource params against the Organization Resource Type catalog, not only resource outputs used by bindings. Save and ValidateImport share this path before any desired-state write. Failed validation never advances the draft version. See specification BR-14.

## Unified row and inline creation collaboration — DEVEX-ENV

The UI row has stable identity, container variable name, source and only the
active source payload. Source changes retain the name; removing one row does
not remove its key/dependency. Catalog filters classify UC-12 kinds and resource
secret outputs; scope/generation guards isolate asynchronous replies.

The inline key boundary invokes ConfigurationService.CreateKey with a known
configuration version; its CAS/absence check protects existing shared values.
Return safe metadata, select the key in the initiating row and keep the parent
form unsaved. Initial store selection delegates to UC-01 first if needed. Each
commit is separately visible; no compensating key delete on parent Cancel.
Inline dependency creation shares Type/class/params controls and updates only
parent form state, then asks for an output. Save calls workload configuration.

New ordinary strings are accepted as non-secret input under OC-16. Workload
configuration and persistence record provenance; pending deployment carries it
only into successful per-workload current-state commits. Reconstruction checks
that provenance against exact values before exposing literals; rename uses the
same lossless reconstruction and retains safe literals. Legacy unknown literals
remain unavailable. No generic bypass of reference/secret checks is permitted.

## T25 — Public routing boundary

Save and ValidateImport accept only `service.publicRoutes`. Reject `publicPort`
before writing a draft or advancing its version; the UI retains the import/error
for correction without converting it to `/`. Current-set reconstruction and
public projection reject the removed field rather than silently dropping it.
Container literal/reference validation follows OC-16 DEVEX-ENV; secret and legacy readback protection remains.

## T30 shared reconstruction

UC-12 explicit rename uses the same lossless current-set-to-Score reconstruction
as Edit, keeping references and advanced fields. Desired drafts take precedence
and pending deletes are excluded. Configuration control prepares all changes;
one atomic persistence command writes the immutable revision and affected drafts
under config/env/draft CAS. UC-16 editor and UC-05 consume the new draft version;
current set and runtime are untouched.

## Class metadata projection — I04-07

Reference catalog dùng read-only Resource Types query đã có, bổ sung computed
class names từ verified registry và scoped Definitions trong cùng snapshot.
Controller lấy Organization từ session; Developer projection không chứa module
pins/provider bindings/private inputs. Không thêm persistence field hoặc system
admission. UI giữ custom/imported class và metadata request/scope guards;
server Preview/Deploy remains authoritative. Payload/role/failure contract ở
[UI mapping](ui/api-mapping.md#metadata-class-và-source--i04-07).

## ENV-UX — direct-entry table (2026-10-27)

The boundary renders Name/Value first. One scoped reference picker projects
UC-12 keys, declared dependency outputs and valid same-environment Service ports
into selectable items. It never receives local variable values as source
candidates. Selection retains row identity/name and clears previous source
payload through the existing binding conversion. Source labels identify the
Application key explicitly. Inline creation delegates to the existing UC-12
create-only/Secret Store boundary; classification and persistence stay unchanged.
