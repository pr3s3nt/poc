---
id: ADR-041
artifact: architecture-decision
status: current
last_reviewed: 2026-11-09
---

# ADR-041 — Stable template renderer pin

User decision 2026-11-09, issue #57. It refines
[DV-05](../../usecase/UC-03/definition-versions.md) for Git template sources of the
template driver ([ADR-035](ADR-035-generic-workload-template-driver.md)).

## Context

A template source pin recorded a renderer identity that hashed the whole running
`orchestrator` executable together with the template driver and render engine
sources. Every backend build therefore changed the identity, even when no
rendering code changed. After each upgrade all registered template sources became
UNAVAILABLE and Deploy failed with `TEMPLATE_UNAVAILABLE` until a Platform
Engineer registered the template again, revised and promoted the workload
Definition and selected the new revision in every Environment.

## Decision

The renderer identity of a template source pin is the digest of:

- the engine contract (`template-v1`) and template driver version;
- the embedded sources of the template driver and the render engine (as before);
- the Go toolchain version that built the backend (`text/template` belongs to it);
- the module versions of the libraries the renderer links for data handling
  (`gopkg.in/yaml.v3`, `github.com/tidwall/sjson` and its `gjson`, `match`, `pretty`).

The whole executable digest is no longer part of it. A rebuild that leaves these
inputs unchanged keeps every pin AVAILABLE; any change to them still makes the
pin UNAVAILABLE and requires an explicit new registration and selection, so a pin
is never reinterpreted by a different renderer.

Code outside these sources can still shape renderer input (normalization,
bindings). Preview shows the manifests before Deploy, and the standard template
examples are rendered against committed expected output in the backend test
suite; a change in their output fails the build instead of reaching a pin.

## Existing pins

Pins recorded with the executable identity cannot prove which renderer produced
them, so they are not accepted automatically. After upgrading to the release that
contains this decision, each template source is registered again once, and the
workload Definitions and Environments select it as today. Later upgrades keep the
new pins AVAILABLE while the identity inputs are unchanged. No stored pin,
revision or Environment selection is rewritten.

## Consequences

- Upgrades that do not change rendering no longer require template re-registration.
- Changing the Go toolchain or a listed library version is a deliberate renderer
  change and invalidates pins, as does changing renderer or engine code.
- Terraform module pins and their binary/provider rules are unchanged.
