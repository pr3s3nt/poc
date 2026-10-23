#!/usr/bin/env bash
# Own project/volumes; no retained stack reset, kube context change or deploy.
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
case "${1:-}" in
  up|stop|status) action="$1" ;;
  *) echo "usage: $0 {up|stop|status}" >&2; exit 2 ;;
esac
compose=(docker compose --env-file /dev/null --project-name preview-kind
  -f "${repo_dir}/docker-compose.yml"
  -f "${repo_dir}/deploy/local/compose.kind.yml"
  -f "${repo_dir}/deploy/local/compose.preview-kind.yml")
# Do not let ambient image/port/project settings target another installation.
unset COMPOSE_FILE COMPOSE_PROJECT_NAME BACKEND_IMAGE FRONTEND_IMAGE BACKEND_PORT FRONTEND_PORT
case "$action" in
  up)
    docker network inspect kind >/dev/null
    "${compose[@]}" up -d --build --wait --wait-timeout 180
    ;;
  stop) "${compose[@]}" stop ;;
  status) "${compose[@]}" ps ;;
esac
