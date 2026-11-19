# Dedicated Keycloak test inputs

Implements only the setup for [UC-00 SSO](../../../orchestrator_docs/usecase/UC-00/specification.md#keycloak-sso--issue-77).
Requires Node 22+, Docker Compose and the existing feature proxy/Tailscale path
from [handoff runbook](../../../orchestrator_docs/operations/tailscale-handoff.md).

From repository root, set `FEATURE_DOMAIN` to the verified feature domain and
`KEYCLOAK_PRIVATE_DIR` to an absolute private directory outside Git, then run:

```bash
node e2e/setup/keycloak/prepare.mjs
```

The script generates random test passwords, confidential-client secrets, a realm
import and a backend configuration file with mode 0600. It refuses to overwrite
existing configuration. Use its files with `deploy/local/compose.keycloak.yml`;
see the [OIDC runbook](../../../orchestrator_docs/operations/keycloak-sso.md).

This is a dedicated realm, not company configuration. Three role accounts and a
no-role rejection fixture are created. `test-controller` has realm-admin solely
to automate test-account role/enablement changes; never copy that client into a
company realm. All account email addresses use reserved `example.invalid`.
Preparation alone is not acceptance. No platform/Kubernetes/cloud mutation is
performed by this script. Keep private files while preview volumes are retained;
remove only these task-owned inputs when removing the preview stack after approval.
