#!/usr/bin/env bash
# Upgrade the selected Compose installation, preserving a private backup.
# Usage: bash deploy/local/upgrade-database.sh /absolute/backup.dump [Compose global options...]
set -euo pipefail
if [[ $# -lt 1 || $1 != /* ]]; then
  echo 'Usage: upgrade-database.sh /absolute/backup.dump [Compose global options...]' >&2
  exit 2
fi
backup=$1
shift
repo_root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
backup=$(realpath -m -- "$backup")
case "$backup" in "$repo_root"|"$repo_root"/*) echo 'Backup must be outside Git.' >&2; exit 2;; esac
if [[ -e "$backup" ]]; then echo 'Backup path already exists; choose a new path.' >&2; exit 2; fi
umask 077
partial=$(mktemp "${backup}.partial.XXXXXX")
trap 'rm -f -- "$partial"' EXIT
compose() { docker compose "$@"; }
# Build before downtime. All selected overrides/options apply to every command.
compose "$@" build db-setup backend frontend
compose "$@" stop frontend backend
# No upgrade occurs unless dump and archive inspection both succeed.
compose "$@" exec -T postgres sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$partial"
compose "$@" exec -T postgres pg_restore --list < "$partial" > /dev/null
# Hard-link publishes without overwriting an existing backup (including a race).
ln -- "$partial" "$backup"
if ! compose "$@" run --rm --no-deps db-setup; then
  echo 'Upgrade failed; application remains stopped. Preserve backup and inspect setup logs.' >&2
  exit 1
fi
compose "$@" up -d --no-deps --wait backend frontend
echo 'Database setup and application health checks passed; retain the private backup.'
