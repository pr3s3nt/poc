#!/usr/bin/env bash
# Build the three acceptance workload images as static Go binaries in scratch
# images. Usage: build-images.sh <tag> [output-dir]
set -euo pipefail

TAG="${1:?usage: build-images.sh <tag> [output-dir]}"
OUT="${2:-$(mktemp -d)}"
ARCH="${ARCH:-amd64}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

mkdir -p "$OUT"
for workload in frontend backend worker; do
  echo "building acceptance-${workload}:${TAG} (linux/${ARCH})"
  CGO_ENABLED=0 GOOS=linux GOARCH="${ARCH}" go build -trimpath -ldflags "-s -w" \
    -o "${OUT}/${workload}" "${ROOT}/examples/acceptance-app/${workload}"
  docker build --quiet \
    --platform "linux/${ARCH}" \
    --build-arg "BINARY=${workload}" \
    -f "${ROOT}/examples/acceptance-app/Dockerfile" \
    -t "acceptance-${workload}:${TAG}" \
    "${OUT}" > /dev/null
done
echo "images built with tag ${TAG}"
