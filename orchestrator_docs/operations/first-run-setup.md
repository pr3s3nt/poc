---
id: OPS-FIRST-RUN-SETUP
artifact: operations-runbook
status: current
last_reviewed: 2026-11-18
---

# Console initialization and platform setup

Behavior: [UC-00 SETUP](../usecase/UC-00/first-run-setup.md), issue #89.
First deploy PostgreSQL with the existing owner setup and runtime-only grants.
Backend still never performs schema DDL. No database schema change is required.

## Enable for a new installation

Mount a private, regular setup-token file readable by the backend's runtime UID,
mode 0600/0400, with at least 32 random characters. Configure
`ORCHESTRATOR_SETUP_TOKEN_FILE` or `-setup-token-file`. It requires durable
PostgreSQL or JSON storage and local authentication; OIDC cannot be combined with
initialization mode. Keep the same setting/file across restarts: it suppresses
legacy fixture seeding and restores the persisted Organization identity. No token
is logged or returned by the API. Existing Organizations close initialization,
even if they do not yet have Admin accounts; use the existing operator CLI for
those installations, never reset them to expose setup.

Local Compose provides an opt-in overlay:

```sh
docker compose -f docker-compose.yml -f deploy/local/compose.first-run.yml up -d --build
```

The setup-token service generates a stable token on a separate volume and grants
only UID 65532 access; backend mounts it read-only. Read it with an explicit
operator command into a private file (example for a feature project):

```sh
mkdir -p /private/setup
chmod 700 /private/setup
(umask 077; docker run --rm --user 0 -v poc-setup_setup-token:/setup:ro \
  postgres:16-alpine cat /setup/token > /private/setup/token)
```

Adapt the project volume name; never print token/passwords into issue comments.
Use a verified Tailscale Console URL per the [handoff runbook](tailscale-handoff.md).
Enter token, Organization key/name and Admin credentials. Organization/Admin are
created in one locked transaction; the setup endpoint then closes permanently.
Sign in normally. An uncertain response requires checking status and trying the
chosen sign-in, never blindly replaying initialization. Invalid token, mismatched
password or invalid identity leaves the installation uninitialized.

## Configure platform

Open Thiết lập platform (`/ui/setup`) as Admin or Platform Engineer. Existing
installations can use this guide without enabling initialization mode.

1. Configure Git access/CA using the normal Git form; public sources can use
   anonymous access. Use its connection check for the intended repository/ref.
2. Register and verify the existing Kubernetes account using a concealed kubeconfig.
3. Register the existing workload Secret Store with its scoped token.
4. Configure/verify the CD binding with the existing provider form.
5. Init Types/Definitions from a repository of your choosing.

The guide reads persisted configuration; Kiểm tra lại refreshes its summary.
Git configured is not the same as a fresh connectivity check; other connection
statuses reflect their saved verification. A configured catalog count does not
prove completeness for every possible workload. Full readiness still requires
Application Preview/Deploy and workload acceptance separately.

## Git init

Enter an HTTPS repository URL, branch/tag/full commit and relative catalog path
(blank means repository root). Directory names and resource keys are not fixed.
The backend resolves one commit and reads only regular bounded files through the
existing hardened Git transport, credentials and CA configuration. YAML/JSON
objects are classified by product fields. Invalid documents are shown and cannot
be selected. Non-product documents are ignored. Maximum: 100 recognized documents,
1 MiB each, existing Git tree bounds and no symlinks/executables/submodules.

Review/select documents and edit their YAML/JSON if needed. Environment placeholders
written as `<...>` become inputs. Values replace placeholders only; product
`${resources[...]}` expressions remain intact. For template Definitions, enter the
Git template URL/ref/directory explicitly. Inspect prepares its source and supplies
the local template ID; no ID from another installation is guessed. Terraform
Definitions keep their own Git source URL/ref/path and receive verified pins through
normal module inspection. Review the prepared configuration before confirming.

Kiểm tra trước khi nhập validates prospective Types together with Definitions,
accounts, references, mappings and source pins. Source metadata/cache may be created;
Types/Definitions are not written yet. NEW means it will be created, REUSED means
matching data exists, CONFLICT means different data exists, INVALID identifies an
invalid document/field. Changing the selection/content/values invalidates the review.

Confirm imports Types before Definitions, using the ordinary insert-only APIs.
A failure or conflict stops the remaining writes and preserves successful steps.
After reload/restart, re-enter the same source commit and values, check again and
reuse the saved records. Source/edit drafts and credentials are never saved in
browser storage. A different record must be reviewed in the ordinary editor;
import does not update revisions, promote, rotate or reset it.

Repository structure reference only:

```text
resource-catalog/<resource>/*-type.yaml
resource-catalog/<resource>/*-definition.yaml
templates/<template>/bundle.yaml
terraform/<module>/main.tf
```

`workload` is an ordinary Resource Type with template Definitions. Setup does not
create Application workload instances. The supplied Git repository owns its files;
the wizard does not add missing resources or change that repository. Cluster,
Gateway, DNS/TLS, Vault, CD controllers and permissions must already exist. Optional
HA Definitions do not prove operator installation, failover or production readiness.

Browser verification: [scenario](../../e2e/scenarios/first-run-setup/README.md).
