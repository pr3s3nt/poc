---
id: UC-00-SETUP
artifact: use-case-specification
status: current
last_reviewed: 2026-11-18
---

# First-run Console and platform setup — issue #89

Approved scope: user decisions 2026-11-18, issue #89. Extends BR-01/10 with
operator-authorized browser initialization; existing CLI and OIDC remain supported.

## Required behavior

- SETUP-01: An operator explicitly enables browser initialization with a private
  setup-token file (at least 32 characters) and durable storage. No default token
  or password. An installation with any Organization is already initialized;
  enabling setup never adopts, resets or renames existing data. Fresh setup mode
  defers normal seed writes. OIDC configuration and first-run mode cannot combine.
- SETUP-02: Public status reveals only availability. The operator supplies token,
  Organization key/name, username, password and confirmation. The backend validates
  them and atomically creates the Organization and first Admin under an installation
  lock. Concurrent claims cannot create multiple Organizations. Presence of the
  Organization permanently closes initialization, including after restart. A failed
  transaction creates neither identity. No automatic replay on uncertain outcomes.
- SETUP-03: Successful setup returns to ordinary sign-in. Admin/Platform Engineer
  can open the platform setup guide; Developers cannot use its APIs or authoring.
  Existing installations retain their authentication/SSO and data.
- SETUP-04: The guide reuses Git, Driver Accounts, Secret Stores and CD configuration
  screens and verification APIs. Progress comes from persisted server configuration,
  never a browser-local READY flag. Users can leave, return and retry a failed step.
- SETUP-05: Users supply an HTTPS repository URL, branch/tag/commit and root path.
  No fixed repository, folder names or resource list. Read bounded YAML/JSON trees
  with existing Git access/CA policy, pin the resolved commit and recognize product
  Type/Definition objects by content. Workload is a Resource Type, not an Application
  workload instance. Unsupported documents are not executed as manifests.
- SETUP-06: Before import, review files, keys, dependencies, placeholders and
  conflicts. Users select documents and fill environment values. Template-source
  placeholders require explicit Git URL/ref/path; prepare the verified template
  and substitute its installation-local ID. Terraform sources use normal inspection.
- SETUP-07: Confirmation imports Types before Definitions. Equivalent existing data
  is REUSED, different data is CONFLICT. No overwrite, promotion, rotation or reset.
  Successful steps remain; rerunning a pinned source reconciles against server data.
  Partial failures identify the document and keep resumable results. Metadata/cache
  preparation is distinct from business import; no workload deployment is implied.
- SETUP-08: Secrets remain transient or in existing credential stores. No credentials
  in URL, browser storage, logs, source files or public evidence. API errors expose
  safe codes/field paths. Browser state may hold a transient draft only.

## Boundaries

No controller installation, cluster permissions, DNS/TLS, Application creation,
workload instance import/deployment or SSO configuration changes. PostgreSQL HA
requires an existing operator. User Git source is reference data, never code to run.

## Realization and contracts

Browser → setup status/claim API → installation setup service → locked persistence
transaction → Organization and hashed Admin. PostgreSQL uses a transaction advisory
lock scoped to its schema, memory/snapshot uses its transaction lock; no schema DDL.
Bootstrap skips seeding in enabled setup mode and restores the saved Organization
identity on restart. Existing runtime remains least privilege.

Authenticated guide → existing configuration screens; Git discovery → hardened
GitCLI exact snapshot → strict document parsing → review draft → ordinary catalog
prepare/import services. Reuse existing validation, account ownership and immutable
source identity. Each import response reports CREATED/REUSED/CONFLICT/error.

API: GET /api/v1/setup (public availability); POST /api/v1/setup (claim token and
identity fields, no session creation); GET /api/v1/platform/setup (scoped progress);
POST /api/v1/platform/setup/discover (url/ref/path); POST /api/v1/platform/setup/preview
(documents with Type or Definition and optional template Git source); existing prepare/import APIs
perform confirmed registration. All setup responses are no-store.

## UI

Unsigned setup reuses sign-in styles/PasswordInput/LanguageSelect. Platform guide
uses current AppShell, cards, buttons, error and status patterns; navigation gains
Setup for PE/Admin only. Steps: Git, Kubernetes, Secret Store, CD, Git init, summary.
Each step declares loading, missing, configured, failed and retry states. Document
review uses a selection list and editable YAML/JSON with ordinary registration
validation. VI/EN catalogs own labels; details/help hold technical explanations.

## Traceability

SETUP-01/02/03 → initialization service, adapters, bootstrap and HTTP authorization
checks; SETUP-04 → platform progress and existing configuration forms; SETUP-05/06/07
→ discovery/parser, catalog import and browser acceptance; SETUP-08 → transport,
secret handling and access-control tests. Implementation status and validation
belong in the implementation/verification records, not this specification.
