#!/usr/bin/env bash
# kubectl stand-in for local browser verification without a cluster. It answers
# only the read-only calls UC-04 host-context verification makes and refuses
# everything else, so no cluster or cloud state can be touched.
# STUB_CONTEXTS: space separated kube contexts the stub "knows".
set -euo pipefail
args=("$@")
# Drop the leading --context <name> / --kubeconfig <path> pairs.
while [[ ${#args[@]} -ge 2 && ( "${args[0]}" == "--context" || "${args[0]}" == "--kubeconfig" ) ]]; do
  if [[ "${args[0]}" == "--context" ]]; then context="${args[1]}"; fi
  args=("${args[@]:2}")
done
known="${STUB_CONTEXTS:-lab-context}"
case "${args[*]}" in
  "config get-contexts -o name")
    for name in ${known}; do echo "${name}"; done ;;
  "version -o json")
    echo '{"serverVersion":{"gitVersion":"v1.30.0-stub"}}' ;;
  "config view -o json")
    printf '{"contexts":['
    sep=""
    for name in ${known}; do printf '%s{"name":"%s","context":{"cluster":"%s"}}' "${sep}" "${name}" "${name}"; sep=","; done
    printf '],"clusters":['
    sep=""
    for name in ${known}; do printf '%s{"name":"%s","cluster":{"server":"https://%s.stub.invalid:6443"}}' "${sep}" "${name}" "${name}"; sep=","; done
    printf ']}\n' ;;
  auth\ can-i\ *)
    echo yes ;;
  *)
    echo "kubectl-connection-stub: refused: ${args[*]}" >&2
    exit 1 ;;
esac
