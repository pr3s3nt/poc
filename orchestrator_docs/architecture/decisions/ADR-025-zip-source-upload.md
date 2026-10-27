---
id: ADR-025
artifact: architecture-decision
status: current
last_reviewed: 2026-10-28
related: [ADR-014, ADR-021, UC-03]
---

# ADR-025 — Uploaded ZIP module and render bundle sources

User decision 2026-10-28 adds ZIP upload alongside existing packaged/installed
and Git sources. Preserve the Git directory layout and existing inspectors,
contracts, runtime/cache verification and Definition lifecycle. This adds source
preparation, not another executor or rendering engine.

PE/Admin selects a ZIP and a relative directory (empty means `.`). No implicit
wrapper-directory stripping: `repo-main/modules/db` is selected explicitly if
present. The server validates the entire archive before inspecting the selected
directory. Terraform local child modules may use other directories in the same
archive; pinned HTTPS child modules retain the existing inspection rules.

Source kind is `zip`. The canonical full archive-tree SHA-256 is the revision
(identity independent of ZIP timestamps, compression, order and uploaded name).
The selected directory has its own canonical content digest. Terraform ZIP
metadata belongs to the uploading session Organization. Terraform
source ID is `zip-` plus the first 56 SHA-256 hex characters of
`organizationKey + NUL + archiveRevision`; the full revision remains in the pin.
Persist the Organization key in the immutable module contract JSON. Filter ZIP
capability discovery and reject foreign-Organization ZIP pins at registration;
existing packaged/public-Git catalog scope is unchanged. Module ID follows
Type/path identity, retaining trusted packaged
identity only on exact content/Type equivalence, as Git inspection already does.
Render bundles use `zip:sha256:<revision>` as a stable source locator, never a
fetch URL. Store source kind explicitly in render persistence; Git records keep
`git`. No original ZIP/filename, host path or credentials are exposed or retained.

Only bounded standard ZIP regular files/directories are accepted: at most
32 MiB compressed, 32 MiB expanded, 2,000 entries, 1 MiB per file. Reject links,
special/executable/encrypted entries, duplicates, file/directory conflicts,
absolute/traversing/backslash/control-character paths, Terraform state and
variable-value files (`terraform.tfvars`, `*.auto.tfvars`, `*.tfvars.json`), Git
and Terraform working directories. Existing selected-tree and contract limits also
apply (render bundle 256 files/4 MiB). Read bounded content without filesystem
extraction or code execution. Failure creates no verified record or Definition.

Upload endpoints use strict multipart fields `file`, `path` and, for Terraform,
`resourceType`, bounded request body and 120-second inspection deadline. Existing
PE/Admin/session/Organization and safe error contracts apply. Success returns the
same capability/renderBundle shape as Git inspection. Upload and inspect are one
explicit action; changing source, file, directory or Type invalidates the result
and ignores old responses. Saving remains explicit Create/CAS. Git remains the
new-draft default; ZIP is an additional source tab. Verified ZIP records can be
selected again and reopened after restart. To restore a missing ZIP cache, the
user explicitly uploads the same content; never fetch a ZIP locator as Git.

The fresh database baseline gains source_kind and ZIP locator constraints; no
retained database reset, migration or external deployment is authorized here.

Canonical behavior: [UC-03 ZIP requirements](../../usecase/UC-03/specification.md#zip-source-upload--adr-025).
Shared contracts: [Terraform](../contracts/terraform-modules.md#zip-upload--adr-025)
and [render bundles](../contracts/git-render-bundles.md#zip-upload--adr-025).
