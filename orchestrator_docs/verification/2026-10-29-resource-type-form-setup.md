---
id: VERIFY-RESOURCE-TYPE-FORM-SETUP-20261029
artifact: verification-record
status: current
last_reviewed: 2026-10-29
---

# Resource Type and render bundle setup through paced forms

User requested extending the [portable setup runner](../../scripts/bootstrap/README.md)
to register Resource Types, then required visible paced UI interactions with no
Score import or JSON entry in the Console. Requirements remain
[UC-02](../usecase/UC-02/specification.md) and
[UC-03 RB](../usecase/UC-03/specification.md#git-rendering-bundle-authoring--adr-021).

## Retained Tailscale preview

Console `http://100.77.224.24:18596`, Compose project `main-a0fe3a4-preview` and
product images `main-a0fe3a4` were reused. Headed Chromium ran on the existing
desktop with `ORCH_STEP_DELAY_MS=1000`; login used the shared Console helper.

- Real Resource Type form registered `persistent-storage`: optional string input
  `storage`, required public string outputs `claimName` and `namespace`.
- The existing `standard-workload` Definition was reused only after comparing
  the full authored configuration and Git pin against the example config.
- The Add Workload dialog displayed all five 1.0.0 catalog templates.
- Selected Backend, observed Add volume mount disabled, added local dependency
  `data` with type `persistent-storage`, observed the button enabled and added
  mount path `/data`. No draft was saved.
- This Resource Type is retained for user testing. It is a contract only;
  Terraform infrastructure Definitions/PVC provisioning were not added.

## Isolated form creation and rerun

An owned temporary container `orch-form-registration-check` served the same
production Web Console assets and backend binary on loopback port 18598. It
used an in-memory store, fake execution adapters and the real score-k8s/Git
bundle registry. This proves registration UI behavior, not workload execution.

The runner registered a fresh Resource Type and a fresh `standard-workload`
Definition using form fields only. The first invocation then correctly failed
the documented Application prerequisite: no Application existed in the temporary
store. A temporary Application was created through the UI; rerun with a custom
`custom-ui-policy` Definition then passed. This exercised string `team=payments`,
number `defaultReplicas=2`, explicit false `runAsNonRoot` and class criteria
through textboxes/selects/checkboxes/radio controls. Five-template picker and
Backend volume-mount checks passed. A second invocation reused both Type and
Definition with exact schema/config comparison and passed the same checks.

The temporary container was stopped and its absence verified; `--rm` removes
its in-memory registrations/cache/Application. No retained preview objects were
deleted. Private files and temporary configuration stay outside Git under the
preview's `/tmp` directory; no credentials are recorded here.

## Validation and boundaries

- E2E helper tests: 14/14 pass, zero skipped, including Resource Type schema and
  flag comparison, invalid configuration rejection and old config compatibility.
- Runner syntax, documentation checker and diff whitespace checks pass.
- No product code changed; prior image build evidence is reused, with no new
  Go/frontend build or full product suite required.
- All registration writes used actual UI form actions. Technical API reads
  verified persisted schemas, pins and catalog projections; no hidden API create.
- No JSON/Score was entered in UI. The external JSON configuration remains the
  portable parameter source. Object/array render inputs and nonempty provision
  are rejected before login because those controls require JSON text today.
- No workload deployment, cloud/Kubernetes mutation, PVC creation or existing
  Application data change occurred. Browser sessions were signed out and closed.
