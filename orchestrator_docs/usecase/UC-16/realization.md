---
id: UC-16-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-14
---

# UC-16 — Use Case Realization

This collaboration traces the approved [specification](specification.md).
Application keys use the virtual Score `environment` resource and
`${resources.env.KEY}` syntax. HTTP and state contracts are defined in
[OC-12/OC-16](../../architecture/contracts/operation-contracts.md).

## Responsibilities

- Workload configuration boundary: present per-container UC-12 Variables and
  Secrets checklists, optional container-name overrides, and a separate
  resource/Service source editor; never offer a literal value field. Retain
  binding references as the form model so no new persistence model is needed.
- Reference catalog: list UC-12 keys for the selected Application/Environment,
  declared resource inputs/outputs/classes và shared IDs từ safe metadata,
  cùng same-Environment workload
  Services and ports.
- Workload configuration control: validate the selected references, save a
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
| MS-04–MS-05 | Load scoped UC-12 key metadata; tick per-container keys with same-name defaults and optional overrides. Restore all existing/imported mappings; keep resource/Service sources separate and secret values hidden. |
| MS-06 | Validate selected-key availability and per-container name uniqueness before Score serialization; then validate references, output classification and target Service/port using the existing backend contracts. Retain input on failure. |
| MS-07 | Save desired configuration, return pending status and a UC-05 Preview affordance. |
| VAR-01 | Parse one imported Score, run the same validation and show the resulting form before save. |
| VAR-02 | Confirm deletion, mark desired change pending and permit Undo before deploy. |

## Boundary

UC-16 has no runtime side effects. UC-05 reads pending desired configuration to
produce preview; UC-06/UC-07 apply an approved change later against the pinned
draft version and UC-12 revision.

## Input validation collaboration

MS-06 validates all declared resource params against the Organization Resource Type catalog, not only resource outputs used by bindings. Save and ValidateImport share this path before any desired-state write. Failed validation never advances the draft version. See specification BR-14.

## Checklist projection

The UI projects the existing `Binding` collection onto scoped key metadata.
Selection does not store key values or a new API field. Existing mappings,
including aliases and multiple names referencing the same key, are retained.
Unavailable referenced keys remain visible until explicitly removed/corrected.
Validate collisions across Application/resource/Service bindings before
building the variables map, so serialization cannot overwrite an earlier
binding. Selection is independent for each container. Lỗi tải keys/class/module capability không được biến thành danh sách rỗng
hoặc overwrite imported/custom selection. Form không nhận Terraform module
selector/provider credential; Preview/Deploy kiểm matching riêng.

## T25 — Public routing boundary

Save and ValidateImport accept only `service.publicRoutes`. Reject `publicPort`
before writing a draft or advancing its version; the UI retains the import/error
for correction without converting it to `/`. Current-set reconstruction and
public projection reject the removed field rather than silently dropping it.
Protected container literal/reference validation remains in force.

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
