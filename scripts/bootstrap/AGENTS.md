# Bootstrap tasks — instructions for the internal AI

Read the root `AGENTS.md`, then `enterprise/README.md`, `enterprise/INPUT_REFERENCE.md`
and `enterprise/AI_HANDOFF.md`. User instructions and already granted target
permissions take precedence; do not ask again for actions already authorized.

- This directory configures platform resources; it is not an installer for
  Kubernetes/Fleet/Vault or a database reset/upgrade tool.
- Inspect branch/diff first. Preserve user changes. Prefer filling private inputs,
  credential file references and reviewed Git documents over company-specific code.
- Never invent company endpoints, credentials, CA roots, namespace/account identity
  or deployment permission. Record missing facts; continue independent read-only
  checks. Request only the facts/authorization actually missing for a dependent step.
- Config examples do not grant staging, production, cloud or RBAC mutation rights.
  Confirm the existing session's authorized target before those steps.
- Use HTTPS GitHub Enterprise and exact commits/digests. Do not remove assertions,
  turn off TLS, use mutable latest refs or fall back to public sample repositories.
- Keep secrets/state/logs/video/browser sessions outside Git, with owner-only
  permissions. Never paste secret values into chat, commands, evidence or Git.
- `lock-enterprise-input.mjs` reads existing local Git objects only. It does not
  clone, fetch, push or establish that a module contract is valid. Use the same
  Orchestrator release's metadata and verify real Console inspection afterwards.
- Use mode `check`, review observations, then authorized `bootstrap`/`all`. Distinct
  targets need distinct state/report/cache paths. Preserve old identities on rerun;
  differences or unknown outcomes require reconciliation, not reset/replay.
- Report platform BOOTSTRAPPED separately from application acceptance SUCCEEDED.
  A successful setup is not proof of successful deploy. Retain only the final
  successful scenario video if recording is requested.
- Hand off exact commands, versions, nonsecret pins/identities, results, retained
  resources/forwards, tested URLs and unresolved failures. Never claim real company
  staging/GHES verification based on local/mocked tests.

Schema-3 staging defaults to `runtime.profile=console`: local prepared inputs,
Playwright UI mutations and Console read-back only. Do not require Git/kubectl/gh
CLIs, perform native RBAC setup or port-forward on this path. Use
`prepare-enterprise-input.mjs` with supplied local source folders; its SHA receipts
are not proof of remote Git membership. Backend inspects Git module/template pins.
Explicit dev-test retains native tooling for authorized development verification,
with separate state/reports; never mix its evidence with Console-only reports.
