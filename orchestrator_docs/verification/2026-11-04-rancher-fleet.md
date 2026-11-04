---
id: VERIFY-RANCHER-FLEET-20261104
artifact: verification-record
status: current
last_reviewed: 2026-11-04
related: [ADR-034, UC-04, DELIVERY-RECONCILIATION-CONTRACT]
---

# Rancher-compatible Fleet discovery — kind-only acceptance

The user approved extending discovery/Verify to Rancher-managed Fleet and pushing
the completed change to main, then explicitly selected kind-only verification
because no real Rancher installation was available. The
[scenario](../../e2e/scenarios/rancher-fleet/README.md) reuses ordinary Console
forms and the real private GitHub/Fleet/Kubernetes application flow. No Rancher
installation, cloud mutation or staging-stack change is part of this run.

## Implemented behavior

Discovery and Verify share the released Fleet 0.13–0.15 version/schema policy.
They recognize Deployment/StatefulSet controllers and agents, mirrored registries,
vendor tags and digest images with consistent version metadata. Unknown or
conflicting versions, unserved/incompatible required CRD fields, stale readiness
and ambiguous physical matches are rejected. No fixed v0.15.2 image string is
required. Runtime acceptance here uses Fleet v0.15.2; other admitted versions
have contract/fixture coverage, not separate live-release acceptance.

The resolver reads the target's nonsecret `fleet-agent` ConfigMap with its scoped
Kubernetes Connection and matches its opaque `clientID` to the Fleet registration.
It pins physical kube-system UID and verifies it on Verify/preflight. Default
UID/local-bootstrap matching remains supported. Rancher display-name labels
improve workspace/name choices; labels alone never prove target identity.
Duplicate Connection aliases for one physical cluster are allowed; distinct
physical clusters claiming one registration or multiple matching agents are not.

New topology derives an existing deployment account/release namespace, including
explicit native Fleet identity mode when no override account exists. It creates
no RBAC or controller resources. Existing binding settings remain unchanged.
The four-field form and existing shell/layout remain; safe detail labels have
VI/EN translations for the added internal metadata.

## Primary-source comparison and fixture scope

The implementation was compared with pinned upstream sources:

- [Rancher v2.13.0 fleetcluster controller](https://github.com/rancher/rancher/blob/v2.13.0/pkg/controllers/provisioningv2/fleetcluster/fleetcluster.go)
  supplies `management.cattle.io/cluster-name`, optional display-name and Fleet
  workspace/agent namespace; these are display/topology hints, not physical proof.
- [Fleet v0.15.2 agent ConfigMap](https://github.com/rancher/fleet/blob/v0.15.2/charts/fleet-agent/templates/configmap.yaml)
  and [agent registration](https://github.com/rancher/fleet/blob/v0.15.2/internal/cmd/agent/register/register.go)
  define configured clientID versus default kube-system UID.
- [Fleet v0.15.2 impersonation](https://github.com/rancher/fleet/blob/v0.15.2/internal/helmdeployer/impersonate.go)
  defines explicit account, `fleet-default`, then native agent identity behavior.
- Required field presence/types were compared against actual upstream CRD YAML
  for [v0.13.4](https://github.com/rancher/fleet/blob/v0.13.4/charts/fleet-crd/templates/crds.yaml)
  and [v0.14.0](https://github.com/rancher/fleet/blob/v0.14.0/charts/fleet-crd/templates/crds.yaml),
  plus the installed kind v0.15.2 CRDs.

Go fixtures exercise Rancher-shaped `fleet-default/c-abc` registration, mirrored
Fleet 0.14.1 controller and StatefulSet agent, an opaque clientID, native-account
fallback, pinned physical UID and discovery→Verify agreement. Assertions cover
unsupported versions, missing/wrong CRD field types, conflicting metadata,
replaced/missing physical pins, wrong agent namespace, minor skew and ambiguous
physical targets. The fixture command log proves no Secret read or mutation by
discovery/Verify. These are fixture results, not a live Rancher claim.

## Real kind run

Dev is Compose project `deploy-main`, Console port 3031, backend port 8091,
kind `k8s-4f`, Fleet v0.15.2, real PostgreSQL/Vault/GitHub. Schema remains 7;
this change adds no database migration/reset. Private configuration, screenshots,
receipts and reports are under `~/.local/state/rancher-fleet/20261104/`.
Credentials remain private file references and were not printed or committed.

Read-only `TestFleetTopologyKindReadOnly` passed on the explicit authorized dev
kubeconfig. It discovered controller version, healthy agent and physical UID and
then ran the same Verify contract. The actual opaque agent ConfigMap clientID
matched the native Fleet registration.

An additional read-only probe removed the new optional topology pins from that
local binding and verified the original bootstrap identity path successfully;
legacy local configuration requires no rewrite or credential migration.

Browser setup created run-owned Connections/binding, verified it and selected it
as temporary default. Both preexisting Environment desired/applied binding pins
were recorded and preserved. `verify-discovery.mjs` independently compared the
browser discovery result, saved binding and native registration/agent ConfigMap.
It passed before and after restart onto the final backend image.

Application `394f80f5-14c1-4445-9adf-04b2811c68a8` used binding
`a0b7b88f-8e26-4aa7-93f3-ecf77513d9ee` version 1. Baseline and update passed:
three workloads Ready, real PostgreSQL/Redis/PVC and job identities retained,
five web business checks passed. Independent technical verification passed for
six SHA-pinned GitRepos, corresponding Bundle commits, 19 YAML files in the private
Application repository, safe Helm policy and owned reader Secret.

Rollback passed after the API confirmed the original request succeeded. During
that request, refreshing the frontend image interrupted the long HTTP response
with 502 while the backend operation stayed ACTIVE. No deploy was blindly
resubmitted: the observer waited for the exact reviewRequestId to become
SUCCEEDED, then resumed read-back verification. Preview reused unchanged state;
all five business checks passed with retained data/job identity. Version 2 reused
the Git Connection and became the temporary default while the existing Application
kept desired/applied version 1. Post-rollback technical GitRepo/Bundle/YAML checks
passed again for six units/19 files.

Cleanup passed through the product workload-removal and Application-deletion
flows. Native checks proved zero application Deployments, Application API 404,
namespace absence and zero PVs referring to that namespace. Only run-labeled setup
Role/RoleBinding were removed directly, with UID preconditions. The original
`bootstrap-dev` default (binding `55a49b4c-7e40-4e05-aa9a-3a23d7eb701b`, version 1)
was restored through the Console; both preexisting Environment desired/applied
pins stayed unchanged. The run-owned workload forward was stopped. Reusable
Kubernetes/Git Connections, binding versions 1/2, reader Secret and private Git
history remain deliberately. No unrelated resources or existing data were removed.
Private `e2e/setup.json` records setup/rollback/workload-removal/cleanup gates true.

## Checks and handoff boundary

Backend `go test ./...` and `go build ./...` passed, followed by focused Fleet/
delivery tests for added edge assertions. Source/config/scripts remain task-only.
Documentation/PlantUML validation and diff whitespace checks passed; new Node
scenario syntax was checked. No generated runtime evidence or credentials enter Git.
Frontend edits are locale labels; typecheck, lint, 96 test files/1,001 tests and
build passed. Existing hooks/React act/bundle-size warnings remain. Real VI/EN
detail pages and same-origin binding APIs passed through the exact Tailscale
origin after the final frontend image restart. Visual comparison used the existing
CD page/AppShell/shared styles and the actual updated details screenshots. No
shell, navigation or form fields changed; added technical metadata stays in safe
details, with no repeated guidance in the main form.

Unchanged SQL schema/upgrade integration is reused from the
[schema-7 acceptance](2026-11-04-simplified-cd.md), rather than claiming a new
PostgreSQL migration run. AWS and real Rancher acceptance were not run because
the user selected kind only.

The Console handoff origin is
`http://desktop-bdjp2rq-1.tail342836.ts.net:3031/ui/platform/cd-providers`,
as published by Tailscale Serve. Real browser reads used this origin and
same-origin authenticated APIs, without interception. Host-through-Tailscale
evidence does not assert access from every tailnet device.

Dev backend image is `orchestrator-backend:rancher-fleet`; final locale image is
`orchestrator-frontend:rancher-fleet`. Preserve existing database/Vault/cache
volumes and Tailscale Serve mapping. Restart already configured dev services with:

```sh
BACKEND_IMAGE=orchestrator-backend:rancher-fleet \
FRONTEND_IMAGE=orchestrator-frontend:rancher-fleet \
FRONTEND_PORT=3031 BACKEND_PORT=8091 \
docker compose -p deploy-main -f docker-compose.yml \
  -f deploy/local/compose.kind.yml -f deploy/local/compose.bootstrap.yml \
  up -d --no-deps --no-build --wait backend frontend
```
