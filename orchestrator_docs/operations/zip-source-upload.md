---
id: ZIP-SOURCE-UPLOAD-OPERATIONS
artifact: operations-runbook
status: current
last_reviewed: 2026-10-28
related: [ADR-026, UC-03]
---

# Upload Terraform modules and rendering bundles as ZIP

Use **Cấu hình tài nguyên → Tạo cấu hình → Tải ZIP** for Terraform, or
**Mẫu dựng ứng dụng → Tải ZIP** for rendering. ZIP records are scoped to the uploading Organization. PE/Admin chooses a `.zip`, enters
the directory and clicks **Tải lên và kiểm tra**. Configure the resulting typed
inputs and save explicitly, just as with a verified Git source.

The archive retains its Git layout. One archive may contain several modules and
bundles; each inspection selects one directory:

```text
source.zip
├── modules/
│   ├── postgres/       # select modules/postgres
│   │   └── main.tf
│   └── shared/         # local Terraform child modules may refer here
└── bundles/
    └── standard/       # select bundles/standard
        ├── bundle.yaml
        ├── inputs.schema.json
        ├── defaults.yaml
        └── ...         # stage files required by bundle.yaml
```

If files are at the ZIP root, leave the directory empty or use `.`. If the ZIP
has a wrapper such as `repo-main/`, include it in the directory. The server does
not flatten archives. Files follow the same Terraform/render bundle contracts
as Git; this is an additional source, not support for arbitrary modules or engines.

Maximum archive size is 32 MiB compressed and expanded, 2,000 entries and 1 MiB
per file. Rendering's existing 256-file/4-MiB selected-tree limits still apply.
Exclude Git/Terraform working data, Terraform state and variable-value files,
links, executable files and unsafe paths. The server rejects the complete
archive on unsafe content even outside the selected directory.

Successful inspection pins the archive-tree SHA-256 and selected content digest;
repacking identical paths/content preserves identity. Save a newly uploaded pin
explicitly to select changed content. Preview/Deploy/restart/cleanup use the
verified durable cache and do not require uploading again. Restore a missing
cache by explicitly uploading the same content and selecting the same directory;
a drifted Terraform cache entry remains rejected under the existing integrity rule.
Keep configured module/render cache volumes: the uploaded ZIP itself is not retained.

APIs are `POST /api/v1/terraform-modules/upload` and
`POST /api/v1/render-bundles/upload`. Send multipart `file` and optional `path`;
Terraform also requires the scoped `resourceType`. Existing authentication,
Organization scope, public errors and Create/CAS operations apply. See
[ADR-026](../architecture/decisions/ADR-026-zip-source-upload.md) and
[local browser verification](../../e2e/scenarios/zip-source-upload/README.md).

The PostgreSQL fresh baseline now stores render source kind. Existing retained
installations are not migrated/reset by this feature; follow the current explicit
fresh-baseline setup policy. Upload does not authorize any cloud/cluster mutation.
