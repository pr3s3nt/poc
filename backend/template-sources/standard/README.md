# Standard workload manifest templates

This source implements [ADR-035](../../../orchestrator_docs/architecture/decisions/ADR-035-generic-workload-template-driver.md)
and the [Generic Workload contract](../../../orchestrator_docs/architecture/contracts/generic-workload.md).
It generates manifests directly using bounded Go templates; no external generator
or provisioning stage runs. Register it through Resource Definitions → workload →
Template, using Inline, Git, ZIP or Catalog. Git refs resolve to immutable commits;
ZIP and Inline are content-addressed. Preview and Deploy use the verified cache.

`bundle.yaml` declares a `TemplateSource`, renderer `template` 1.0.0, its input
schema, defaults, optional normalization and ordered manifest/patch templates.
`resources/deployment.tpl` and `resources/service.tpl` create the base objects;
`patches/` supplies shared policy and Deployment/StatefulSet/CronJob execution.
Service is conditional on workload intent. Storage and Secret files reuse only
bindings approved by the platform. Templates never provision namespace/database
resources or read credentials.

The context provides `.Input` (target, identity, workload, approved bindings),
`.Workload` (normalized application spec), `.Inputs` (Definition values),
`.Defaults`, `.Normalized` and `.Manifests` (objects emitted so far). Explicit
workload values take precedence over defaults, including replicas zero. Helpers
are deterministic, with no shell, network or host filesystem access.

The platform validates intent, authorized references, ownership and namespace,
then passes manifests to the existing CD/Fleet delivery path. New source content
never changes an existing revision's pin. Publish/select a new Definition revision
and Preview/Deploy to apply it.

Focused renderer verification from `backend/`:

```sh
go test ./internal/adapters/templatedriver -run 'TestStandardBundleExamples|TestExecutionRendersPrimaryControllersAndApprovedMounts'
```
