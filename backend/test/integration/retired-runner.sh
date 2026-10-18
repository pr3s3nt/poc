#!/usr/bin/env bash
# Fail-fast diagnostic for historical runners that depend on the internal deployment contract ADR-016 retired
# (an Environment selecting a Kubernetes Connection, or the builtin existing-cluster). Sourced at the very top of
# such a runner so nothing is set up or mutated. Past evidence produced by these runners is retained unchanged.
retired_runner() { # NAME REPLACEMENT
  {
    echo "RETIRED RUNNER: $1"
    echo "It exercises the internal deployment Connection / builtin cluster contract that V03.10 (ADR-016) removed:"
    echo "internal placement now comes from Resource Definitions and typed references, an Environment no longer selects a"
    echo "Kubernetes Connection, and the Settings selector is gone. Nothing was started, created or changed."
    echo "Use instead: $2"
    echo "Past PASS evidence of this runner is historical and is not rewritten."
  } >&2
  exit 3
}
