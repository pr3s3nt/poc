# Resource Definition — module-driven forms (approved desktop mock)

User accepted this visual direction on 2026-10-22 and requested an implementation
issue. This folder is the frozen handoff, not production React code or a backend
specification. It supplements the earlier issue #8 prototype for section 2 and
companion resources; preserve the rest of the Console.

## Open locally

```bash
node frontend/design/resource-definition-module-forms/serve.mjs
```

Open <http://127.0.0.1:18090/#module-section>. No installation, credentials or
backend required. Use an HTTP server (the page fetches its adjacent catalog JSON).
No Terraform/provider execution occurs. Drafts use browser localStorage only.

[Hosted demo](https://orchestrator-companion-ui-demo.thanhnt11x11.chatgpt.site/#module-section)
may require its owner's login; the checked-in files and images are the portable
reference for a coding agent. No Sites metadata, tokens or account data is included.

## Screens

- [AWS account and region](screenshots/aws.png)
- [Kubernetes connection and inputs](screenshots/kubernetes.png)
- [Readable reference source picker](screenshots/source-picker.png)
- [Companion resources and dependency flags](screenshots/companions.png)

## Source and provenance

Module repository: <https://github.com/pr3s3nt/terraform-modules>.
Pinned commit: `6b991b16c39ac8485c74fa6756cf3223a129c8c9`.
Site source snapshot: `7e3f15d64753ed9102ce45570fa9bf010631f2ba`.

`catalog.json` was extracted with HashiCorp's HCL parser from all seven `main.tf`
files: vpc, eks, aurora, elasticache, k8s-namespace, postgres-k8s, redis-k8s.
It preserves input types/defaults, original validation expressions/messages,
sensitive markers, outputs and source line numbers. Vietnamese labels are UI
annotations. Resource Type associations come from the module repository README.

Mock account/class options and input/output mappings are illustrative. PostgreSQL
`database`/`username` parameter mappings and companion parameter schemas follow
`backend/internal/seed/seed.go`; Terraform variables are NOT companion params.
Production must discover actual API metadata and Type contracts, not ship this
fixture catalog or infer resource type/account requirements from module names.

## Approved direction

- Desktop scope; retain Console sidebar, typography, spacing, tabs and green action.
- Group AWS account/region and Kubernetes target controls under section 2,
  "Kết nối hạ tầng". One authoritative provider-binding state, not duplicated edits.
- Build inputs from the selected verified module. Show required/unresolved values,
  source summaries and optional/default inputs in an expandable group.
- Use "Cung cấp giá trị" / "Thay đổi" to choose a typed literal, module default,
  mapped application param, supported context value or compatible resource output.
- Keep raw placeholders/descriptors in technical details/advanced editing; never
  make users type a placeholder after selecting an automatic source.
- Companion cards select resource type/class/scope, Type input params, and the two
  dependency flags expressed with concrete consumer/provider names.
- No "Thứ tự thực hiện", live dependency diagram or mobile redesign.

## Integration guardrails

Follow UC-03 BR-23–26 and UC-06 BR-08 plus their UI contracts. Update affected
specification/UI artifacts before code when the approved interaction changes.
Keep backend contracts, pins, secret rules, ordering and matching semantics.

Actual workload params outrank Definition fallback values, then module defaults.
Omitted defaults stay omitted. Mapped required params can satisfy required inputs;
optional params alone cannot. Sensitive/executor-owned inputs have no author control.
Region appears once. Presets/context are explicit author choices, not automatic
hard-coded defaults for all Git modules.

Retain existing Git inspection, registered-module selection, account READY/type
checks, literal-region validation, mapping editor, advanced JSON lossless roundtrip,
unknown-field handling, create/edit/CAS behavior, loading/errors and draft recovery.
Metadata change must not silently discard or submit stale fields. Don't reproduce
mock-only shortcuts as product behavior.

`is_dependent` adds companion → parent. `match_dependents` adds each existing
parent consumer → companion. Neither flag means no extra edge from these flags;
other references still create edges. Save must not Preview or Deploy automatically.

## Deliberate prototype limits

The module picker demonstrates seven modules, not the final production Git-source
workflow. Context/reference choices are proposed UI, not proof the current API
advertises presets. Reference output options are a small public Type-contract
catalog; they do not prove matching, output compatibility at runtime or acyclicity.
Mock validates types and selected bounds, not every Terraform rule. Technical JSON
is a summary, not a valid Definition DTO. Local save allows incomplete drafts;
production registration must retain its actual validation and error behavior.

Static surrounding tabs/identity controls are context only. Keep the real working
controls when integrating. No auth, cloud readiness or successful deployment is
represented by screenshots.

## Validation already performed

Desktop Playwright passed all seven module/provider forms, field-count parity,
default omission, secret exclusion, enum/number/list/bool/map/empty-string inputs,
selected bounds, VPC/namespace references, per-module drafts, browser reload,
companion params/flags, add/remove and absence of the rejected diagram.

For implementation, run the repository-required frontend typecheck, lint, tests
and build, plus focused regression/E2E checks for actual create/edit/API payloads.
Use the module fixture as coverage data, not runtime production configuration.
