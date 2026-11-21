---
id: VERIFY-20261121-KEYCLOAK-GUIDE
artifact: verification-record
status: current
last_reviewed: 2026-11-21
---

# Keycloak deployment guide verification

Issue [#96](https://github.com/pr3s3nt/poc/issues/96) verified frontend commit
`e6f2f9e6` against backend base `0ee38f4f`. The Console guide includes the
[Score file](../../frontend/src/features/documentation/examples/keycloak.score.yaml)
and [extension file](../../frontend/src/features/documentation/examples/keycloak.extension.yaml).

## Console and runtime

- A real browser opened the guide through the feature Console over Tailscale.
  The documentation menu highlighted the article, Vietnamese and English
  rendered, and both downloads matched their source files byte for byte.
- The first trial reached a valid Preview. Deployment stopped at preflight
  before namespace creation because the trial Vault lacked Kubernetes auth
  configuration. The registered workload address was also local to Docker.
  The trial Application was deleted through the Console.
- A task-owned Console and Vault used a configured kind delivery account and
  registered catalog. The workload address was reachable from kind. Application
  setup created a Fleet Role and RoleBinding inside the test namespace.
- A fresh Application followed the guide through Secret creation, Score and
  extension import, draft save, Preview, and Deploy. The import changed only the
  sample endpoint name. The Console reported `SUCCEEDED`.
- Keycloak and its PostgreSQL Deployment became ready, the PVC was bound,
  Vault Secrets Operator reported `SecretSynced`, and the HTTPRoute reported
  `Accepted` and `ResolvedRefs`. Chromium signed in through the assigned
  endpoint and the authenticated Admin API returned the `master` realm. OIDC
  discovery returned the assigned issuer and PostgreSQL contained the
  `master` realm.
- An earlier component check independently exercised the same example on kind
  with PostgreSQL, Gateway routing, OIDC discovery and authenticated Admin API
  access. Its temporary namespace and PV were removed. The Console run above
  separately verified platform provisioning.

## Cleanup

The Console deployed removal of the test workload. After checking that the
Deployment and HTTPRoute were gone, cleanup removed the setup-owned Fleet Role
and RoleBinding with UID and resource-version preconditions. The Console then
deleted the test Application. Kubernetes reads verified absence of its
namespace, PostgreSQL PV and Fleet GitRepo. The shared Gateway's UID was
preserved. The external Git repository retains delivery history.

The test bridges and rehearsal volumes were removed. The feature Console was
recreated for reading the guide, with a fresh database and Vault.

## Checks

The 53 focused tests passed. Typecheck, lint, all 1,114 frontend tests across
118 files, and build passed in a temporary kind namespace. The documentation
checker and whitespace check passed. The feature has no Go changes. The Go
suite was outside this run. [Prior kind baseline](2026-11-19-application-database-purge.md).
