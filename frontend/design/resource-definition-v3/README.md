# Resource Definition — v3 review prototype (issue #8)

This is the preserved interactive **v3** design reviewed by the user, not a
product implementation. It keeps the existing Console style and the existing
backend mapping model. V4 is explicitly excluded.

## Open the prototype

Keep `index.html` and `advanced.js` in the same directory, then open `index.html`
in a browser. It is self-contained: no install, server, account, external fonts,
network calls or API credentials are required. The JavaScript path is relative
so the saved files also work directly from disk.

For source-controlled local serving, from the repository root:

```bash
node frontend/design/resource-definition-v3/serve.mjs
```

Open <http://127.0.0.1:18089/>. Override `HOST`/`PORT` for another interface or
port if needed. The default binds loopback only. This server serves only the
prototype files listed in its route map.

## Review views

- [Configuration](configuration.png): existing module/Git source selection,
  account/region and typed inputs; advanced settings collapsed initially.
- [Matching criteria](matching.png): same Definition, separate tab; AND within
  a case, OR across cases, optional copy/check/help collapsed initially.
- [Advanced settings](advanced.png): editable input/output mappings and
  co-provisioned resources; form/JSON mode changes preserve represented values.

## Preserve in the product refactor

- Two tabs only: Configuration and Matching criteria. No Usage tab.
- Existing dark sidebar, green primary button, orange active-tab indicator,
  type scale, form controls and borders.
- Keep `input_mapping`, `output_mapping`, `provider_bindings`, provision
  descriptors/params/flags, criteria rules, CAS and current secrets/module pins.
- Inputs and outputs may use different names. The UI still configures mappings.
- Preserve drafts when switching tabs/modes and on invalid JSON.
- Reuse actual API metadata and existing guards; do not copy fixture values as
  runtime defaults or permit values simply because the prototype accepts them.

## Prototype limits — do not implement these as product behavior

All content is synthetic: Acme, repository URL, accounts, resource names, module
schemas, version and verification status. Save only shows a message. Git checks
and matching results are placeholders; no resource is created, no credentials
are resolved and nothing is persisted. Schema/options and some field validation
are intentionally simplified for visual review.

The advanced JSON is a **fragment** for editing mappings/provider bindings and
provision, not the full public Definition DTO. The production editor must retain
its actual lossless contract, type/secret checks, metadata validation, copy
confirmation, snapshot/late-reply handling and create/edit outcome protections.

The product specification and issue #8 own behavior/scope. These files fix the
visual/interaction reference and must not be treated as an alternative backend
specification or a copy of the current Humanitec SaaS UI. Do not take artifacts
from `/tmp` or the abandoned v4 proposal when implementing issue #8.
