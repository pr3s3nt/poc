#!/usr/bin/env bash
# Root-only EC2 service operations. Secrets never enter command output.
set -euo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'Run with sudo on the EC2 host.' >&2; exit 2; }
config=/etc/orchestrator/config.json
release=$(readlink -f /opt/orchestrator/current)
export COMPOSE_PROJECT_NAME=orchestrator-ec2
export EC2_APPLICATION_KEY EC2_RUN_ID EC2_OWNER
EC2_APPLICATION_KEY=$(jq -er .deployment "$config")
EC2_RUN_ID=$EC2_APPLICATION_KEY
EC2_OWNER=$(jq -er .owner "$config")
set_images() {
  local sha
  sha=$(git -C "$release" rev-parse HEAD)
  export BACKEND_IMAGE="orchestrator-backend:$sha"
  export FRONTEND_IMAGE="orchestrator-frontend:$sha"
}
compose() {
  local args=(-p "$COMPOSE_PROJECT_NAME" --project-directory "$release" -f "$release/docker-compose.yml" -f "$release/docker-compose.aws.yml" -f "$release/docker-compose.ec2.yml")
  if [[ $(jq -r .enable_alb "$config") == true ]]; then args+=(-f "$release/deploy/ec2/compose.alb.yml"); fi
  docker compose "${args[@]}" "$@"
}
metadata_block() {
  iptables -N ORCH-IMDS 2>/dev/null || true
  if ! iptables -C DOCKER-USER -d 169.254.169.254/32 -j ORCH-IMDS 2>/dev/null; then
    iptables -I DOCKER-USER 1 -d 169.254.169.254/32 -j ORCH-IMDS
  fi
  iptables -F ORCH-IMDS
  iptables -A ORCH-IMDS -j REJECT
}
metadata_allow_backend() {
  local cid address
  cid=$(compose ps -q backend)
  address=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$cid")
  [[ $address =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]
  iptables -I ORCH-IMDS 1 -s "$address/32" -j RETURN
}
health() {
  curl -fsS --retry 12 --retry-delay 5 --retry-connrefused --max-time 5 http://127.0.0.1:8080/api/v1/healthz >/dev/null
  curl -fsS --retry 12 --retry-delay 5 --retry-connrefused --max-time 5 http://127.0.0.1:3001/ui/ >/dev/null
}
backup() (
  local backup_dir stamp
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  backup_dir=$(mktemp -d "/var/backups/orchestrator/$stamp-XXXXXX")
  trap 'compose up -d --no-build --pull never --wait --wait-timeout 180 >/dev/null; metadata_block; metadata_allow_backend' EXIT
  compose stop frontend backend vault postgres >/dev/null
  docker volume ls --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME" --format '{{.Name}}' > "$backup_dir/volumes.txt"
  test -s "$backup_dir/volumes.txt"
  while IFS= read -r volume; do
    local mountpoint
    mountpoint=$(docker volume inspect --format '{{.Mountpoint}}' "$volume")
    tar -C "$mountpoint" -czf "$backup_dir/$volume.tar.gz" .
  done < "$backup_dir/volumes.txt"
  git -C "$release" rev-parse HEAD > "$backup_dir/release.txt"
  cp "$config" "$backup_dir/config.json"
  (cd "$backup_dir" && sha256sum -- *.tar.gz > SHA256SUMS)
  echo "Backup saved privately: $backup_dir"
)
set_images
case ${1:-} in
  start)
    metadata_block
    compose up -d --build --wait --wait-timeout 240
    metadata_allow_backend
    health
    ;;
  stop) compose stop --timeout 60 ;;
  health) health; compose ps ;;
  backup)
    exec 9>/run/lock/orchestrator-operations.lock; flock -n 9
    backup
    ;;
  update)
    [[ ${2:-} =~ ^[a-f0-9]{40}$ ]] || { echo 'Supply an exact release commit.' >&2; exit 2; }
    exec 9>/run/lock/orchestrator-operations.lock; flock -n 9
    previous=$release
    next="/opt/orchestrator/releases/$2"
    if [[ ! -d $next ]]; then
      git clone --filter=blob:none --no-checkout "$(jq -er .repository_url "$config")" "$next"
      git -C "$next" fetch --depth=1 origin "$2"
      git -C "$next" checkout --detach "$2"
    fi
    [[ $(git -C "$next" rev-parse HEAD) == "$2" && -z $(git -C "$next" status --porcelain) ]]
    release=$next; set_images; compose build
    release=$previous; set_images; backup
    systemctl stop orchestrator
    ln -sfn "$next" /opt/orchestrator/current
    if ! systemctl start orchestrator; then
      ln -sfn "$previous" /opt/orchestrator/current
      systemctl reset-failed orchestrator
      systemctl start orchestrator || true
      echo 'Update failed. Previous release selected; inspect schema compatibility and private backup before recovery.' >&2
      exit 1
    fi
    ;;
  *) echo 'Usage: sudo host.sh {start|stop|health|backup|update COMMIT}' >&2; exit 2 ;;
esac
