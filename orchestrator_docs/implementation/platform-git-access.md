---
id: IMP-PLATFORM-GIT-ACCESS
artifact: implementation-handoff
status: current
last_reviewed: 2026-10-30
---

# Platform Git access

[Canonical behavior/API](../usecase/UC-03/git-access.md).
Existing Admin account management was merged from `features/login` first.
Git access belongs to the platform group, accessible to PLATFORM_ENGINEER and
ADMIN, at `/ui/platform/settings`. Developer cannot use these API endpoints.

## Operator flow

1. Deploy the new version through the existing owner database setup flow. Schema
   v2 adds settings metadata only. Preserve database, Vault and bundle caches.
2. As Platform Engineer/Admin, open Cài đặt nền tảng → Truy cập Git.
3. Enter the company Git HTTPS host/group or repository scope, read username and
   token. Add CA PEM under Nâng cao when the server uses an internal CA.
4. For a group/host scope, enter an actual Repo kiểm tra within that scope and
   its branch/tag/commit under Nâng cao. Test reads it from the backend, without
   saving. Lưu persists the configuration. Save does not require a passing test.
5. Use the bootstrap script's existing Git URL/ref options. Terraform module
   names and bundle paths remain unchanged. No bootstrap token option is needed.

Blank token retains the saved value unless scope/username changes. Anonymous
mode removes the active reference. Immutable old credential objects are retained
for concurrent reads and uncertain commit safety; external token revocation is
still performed at the Git provider. No credential GC is introduced in this slice.
Existing credential-store availability remains required for token persistence;
this page does not provision Vault, network connectivity or DNS.

## Review and limitations

Source UI: shared AppShell/styles/Button/PasswordInput and product Secret Stores.
Headless browser displayed the existing reference and new product screen using
fixture API responses, with screenshots outside Git in `/tmp`. The form reuses
create-form/editor-grid styles; CA and repository-test details are collapsed.
Only task fields and action outcomes remain on the basic screen.

Formatting and ERD generation were performed. Automated tests, typecheck, lint,
build, docs checker and migration guard were not run per user instruction.
Added assertions cover scope/Organization isolation, write-only tokens, CAS,
draft Test, HTTPS Git auth/CA and redirect refusal, settings snapshot/reconnect
persistence and populated PostgreSQL v1→v2
preservation/rollback/repeated setup/runtime-role isolation. These assertions
have not been executed in this task. No staging/database/cluster mutation or
real company-Git authentication was performed. No preexisting uncommitted user
changes existed at task start.
