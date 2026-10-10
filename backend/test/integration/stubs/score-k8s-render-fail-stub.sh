#!/bin/sh
# score-k8s stand-in for local browser verification of rendering selection
# (refactor T03). It answers the version probe the renderer adapter requires
# and logs every call; `generate` always fails, so a workload whose selected
# Definition uses this renderer cannot render. It renders nothing, reads no
# credential and touches no cluster. The renderer runs it with PATH limited to
# its own directory, so only shell builtins are used. Calls are logged next to
# the stub.
if [ "${1:-}" = "--version" ]; then
  echo "score-k8s 0.15.0 (t03-render-fail-stub)"
  exit 0
fi
printf 'call\t%s\n' "$*" >> "${0%/*}/score-k8s-calls.log"
case "${1:-}" in
  init) exit 0 ;;
  generate) echo "stub: generation intentionally fails" >&2; exit 1 ;;
  *) exit 1 ;;
esac
