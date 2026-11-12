---
id: VER-2026-11-12-RESOURCE-TYPE-DRIVERS
artifact: verification-record
status: current
last_reviewed: 2026-11-12
---

# Resource Type driver support — issue #81

The user authorized implementation directly on main. Product commit `86d79cc5`
recognizes capability metadata for built-in endpoint, existing-cluster and
Template drivers, while retaining verified Terraform pins and metadata failure
states. Labels show the actual driver and deduplicate versions. Definition count
is not used to infer support. Backend, contracts and existing Definitions were
not changed.

## Checks

- ResourceTypesPage focused component tests: 23/23 pass, including endpoint,
  existing-cluster, template, arbitrary Type IDs, duplicate labels, missing/unknown
  capabilities and unverified Terraform pins.
- Frontend typecheck, lint and production build pass. The existing
  PlatformSettingsPage ref-cleanup lint warning and bundle-size warning remain.
- Documentation checker and diff check pass. Host PlantUML is unavailable;
  no diagram changed.
- Full suites and backend tests were not run: the user explicitly limited
  validation to the affected area, and this change is frontend-only.

## Real browser and rollout

An isolated Docker stack used the real backend and capability API. Browser
checks covered built-in driver labels, support badges, CTA and contract-only
fallback without API mocks. Only task-owned Type fixtures were added there.

The retained main Console received only frontend image `main-86d79cc5`; the
backend image remained `main-9bf70931`. Rollout completed. Browser opened the
exact published Tailscale Console hostname and Resource Types route, confirmed
endpoint, workload and existing-cluster labels/support, and followed endpoint's
CTA to the Definition form. No catalog or application mutations were performed
on main. Private screenshots and access/restart details stay outside Git; no
internal access URL is included in this public record.

UI reference: the pre-change live Resource Types screen, shared UI design,
AppShell, styles and Button. Screenshot comparison preserves shell, layout and
controls; only the existing badge, driver text and CTA visibility change. No
additional explanation or helper content was needed.

The task Docker stack was stopped with its volumes retained. The retained main
Console and its access forwarding remain running for handoff.
