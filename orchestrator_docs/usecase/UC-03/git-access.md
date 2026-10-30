---
id: UC-03-GIT-ACCESS
artifact: specification-extension
status: current
last_reviewed: 2026-10-30
---

# Platform Git access

GA-01. Platform Engineer and Admin manage one Organization-scoped Git access
setting at **Nền tảng → Cài đặt nền tảng → Truy cập Git**. Developer has neither
navigation nor API access. User administration remains Admin-only.

GA-02. Configure an HTTPS URL scope, anonymous or username/token authentication,
and optional PEM CA certificates. Scope matches the exact origin and a whole
path segment prefix; sibling paths, different ports/hosts and redirects never
receive the credential. System TLS verification remains enabled. Public sources
outside the scope use anonymous access.

GA-03. Terraform Git inspection (including remote child modules) and render
bundle inspection automatically resolve settings from the authenticated
Organization. No connection picker, credential in source URLs, or staging
bootstrap token flag is introduced. Execution continues using verified cache.

GA-04. Token is write-only in the UI/API and stored only through the existing
credential-store port; database/snapshots hold an opaque reference. Blank token
on an authenticated update retains the saved token, but changing URL scope or
username requires token re-entry. Anonymous mode explicitly removes saved auth.
Writes use expectedVersion (0 means absent); stale writes return STALE_VERSION.
Uncertain persistence failure must not delete a credential that may be committed.

GA-05. Test connection checks a submitted repository URL/ref using the draft
settings, without saving settings or catalog data. It uses a bounded HTTPS Git
read. Test failure returns a fixed safe error, never Git stderr or credentials.
Test success proves readability of that repository/ref at that time only.

GA-06. Save is independent of Test. Existing settings load with tokenConfigured,
version and updatedAt. Loading/API failure offers Retry; mutation failure keeps
nonsecret draft, clears entered token and reports a safe error. Concurrent edit
requires reload and review. Test result clears whenever draft changes.

## Realization and contract

Console → platform-role HTTP handler → Git access application service → settings
repository + credential store. Git transport → service resolver → matching org
record → opaque credential read → per-process Git environment. CA lives in a
private temporary file removed with the Git snapshot. Neither process arguments,
persistent Git configuration nor ambient Git helpers contain platform tokens.

- GET `/api/v1/platform/settings/git`: `{settings: {url, username, authenticated,
  caPem, tokenConfigured, version, updatedAt}}`; absent returns version 0.
- PUT same path: `{url, username, authenticated, token, caPem, expectedVersion}`;
  returns the same safe settings view. Updated actor comes from the session.
- POST `/api/v1/platform/settings/git/test`: same draft plus `repositoryUrl`
  and `ref`; no persisted writes; returns `{reachable:true}` or a safe code.
- Invalid fields: INVALID_REQUEST; stale: STALE_VERSION; unavailable storage:
  SERVICE_UNAVAILABLE; failed Git read: VERIFICATION_FAILED; authorization uses
  existing UNAUTHORIZED/FORBIDDEN codes. Request bodies are bounded and strict.

UI reuses AppShell, page headers, create-form, Button and PasswordInput. URL scope,
authentication and credentials are basic fields. CA and repository test fields
are under Advanced. All labels/states use VI/EN catalogs. No unrelated platform
settings are included. [Shared design](../../architecture/ui/README.md).

Persistence adds `platform_git_settings` via immutable PostgreSQL version 2;
owner setup preserves existing identities/data and must run before new runtime.
[Schema](../../architecture/database/schema.md),
[upgrade procedure](../../operations/postgres-upgrades.md).
