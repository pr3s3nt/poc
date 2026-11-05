# Workload runtime form

Checks UC-16 WC-16..21 and GW-06/07 through the real Console/API using the shared
[Console helper](../../lib/console.mjs). This is authoring/Preview verification,
not Kubernetes deployment acceptance. No provision/apply/delete is invoked.

Prerequisites: install `e2e/` and `scripts/bootstrap/` dependencies; a running
current backend and frontend; PE and Developer accounts in the same Organization;
a verified current standard template source (including GW-06/07 capabilities),
ready target/namespace/storage Definitions and a ready UC-12 Secret Store.
The target can be real, but this scenario never mutates Kubernetes or cloud.

```bash
ORCH_PLATFORM_URL=http://your-tailnet-console:5191 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=/private/pe-password \
ORCH_DEV_USERNAME=developer ORCH_DEV_PASSWORD_FILE=/private/dev-password \
ORCH_HEADLESS=1 node e2e/scenarios/workload-runtime-form/run.mjs \
  --evidence /private/workload-runtime-evidence
```

A new evidence directory records its own run/application/Definition identities.
Reuse that directory to resume after a failure; an existing unrelated name is
never adopted. The scenario creates one Application and one app-scoped workload
Definition, uploads a generated UTF-8 secret through the create dialog, and saves
one desired workload using PVC, emptyDir, hostPath, ConfigMap and Secret-file mounts with
app/init/sidecar containers. It reopens the form, compares the desired snapshot,
and requests Preview. It verifies the concrete rendered controller lifecycle,
PVC/config/Secret references, storage intent and reference-only secret handling.
It never resolves secret bytes into a workload document or logs them.

The app, draft, scoped Definition and private evidence are deliberately retained
for manual testing. Cleanup is a separate operator action: only the receipt-owned
Application/Definition may be removed through product APIs; verify its identity
and absence of deployed resources first. Never delete a shared namespace, account,
Secret Store, source, provider or Tailscale endpoint. Screenshots/credentials and
receipt files remain outside Git. Runner errors do not automatically clean up or
rerun an unknown mutation outcome.

HostPath extension reuses the receipt-owned application and adds a read-only
Directory source `/var/local/chinhform` mounted in all three roles. Save/reopen
and parsed Preview must keep the exact source and modes. No node path is inspected
or created; this does not claim hostPath runtime acceptance. The prepared source
must advertise host-path-volumes-v1 in addition to the earlier capabilities.
