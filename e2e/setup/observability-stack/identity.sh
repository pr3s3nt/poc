#!/usr/bin/env bash
set -euo pipefail
# Read-only generator. Apply output only after proving setup namespace ownership.
cluster_uid=$(kubectl --context kind-k8s-4f get namespace kube-system -o jsonpath='{.metadata.uid}')
[[ "$cluster_uid" =~ ^[0-9a-f-]{36}$ ]] || exit 1
cat <<YAML
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: orch-cluster-identity
  namespace: orch-observability
  labels:
    release: orch-observability
    orchestrator.dev/setup: observability-s1
spec:
  groups:
    - name: orchestrator-cluster-identity
      rules:
        - record: orchestrator_cluster_identity
          expr: vector(1)
          labels:
            cluster_uid: "$cluster_uid"
YAML
