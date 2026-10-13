#!/bin/sh
# Fresh-only provisioning of the option-A PostgreSQL logins (ADR-014) and the
# independent Terraform state key. It runs as a one-shot service before
# PostgreSQL starts and never prints a generated value.
#
# Volumes (each read by exactly one consumer):
#   /role-passwords  owner/console/state passwords for the PostgreSQL init script
#   /owner           owner URL, for the owner setup service only
#   /runtime         Console and State URLs, for the backend
#   /state-key       AES-256-GCM Terraform state key, for the backend
#   /pgdata          the PostgreSQL data volume, mounted read-only to check it is fresh
#
# Values are generated only when every credential volume AND the PostgreSQL
# data volume are fresh (empty). A complete, marked installation is accepted
# unchanged. Anything else (a missing or empty file, a missing marker, an
# interrupted earlier run, or a fresh credential set next to an initialized
# database) fails closed: nothing is rotated, regenerated or deleted.
set -eu
umask 077

backend_uid=65532
pg_uid=$(id -u postgres)
pg_gid=$(id -g postgres)
marker=.provisioned
staging=.staging

fail() {
  echo "db-credentials: $1" >&2
  echo "db-credentials: nothing was generated, rotated or deleted. Files of an interrupted run stay in each volume's $staging directory (private) for inspection." >&2
  echo "db-credentials: for a disposable local install, remove the PostgreSQL data volume and all four credential volumes of this Compose project together, then start again; never do this for a retained database." >&2
  exit 1
}

# is_empty DIR: no entries at all (hidden ones included).
is_empty() { [ -z "$(ls -A "$1")" ]; }

expected() {
  echo /role-passwords/owner /role-passwords/console /role-passwords/state \
    /owner/database-url /runtime/console-database-url /runtime/state-database-url /state-key/key
}

volumes="/role-passwords /owner /runtime /state-key"

fresh=1
for v in $volumes; do
  is_empty "$v" || fresh=0
done

if [ "$fresh" = 0 ]; then
  for v in $volumes; do
    [ -e "$v/$staging" ] && fail "an earlier provisioning run was interrupted ($v)"
    [ -f "$v/$marker" ] || fail "credential volume $v is not completely provisioned"
  done
  for f in $(expected); do
    [ -s "$f" ] || fail "provisioned credential $f is missing or empty"
  done
  echo "db-credentials: complete existing provisioning kept unchanged"
  exit 0
fi

if [ -e /pgdata/PG_VERSION ] || ! is_empty /pgdata; then
  fail "the PostgreSQL data volume is already initialized but the credential volumes are fresh; new credentials would not match its roles"
fi

random_hex() { od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'; }

for v in $volumes; do
  mkdir "$v/$staging"
  chmod 0700 "$v" "$v/$staging"
done

for role in owner console state; do
  random_hex 24 > "/role-passwords/$staging/$role"
done
url() { printf 'postgres://orchestrator_%s:%s@postgres:5432/orchestrator?sslmode=disable' "$1" "$(cat "/role-passwords/$staging/$1")"; }
url owner > "/owner/$staging/database-url"
url console > "/runtime/$staging/console-database-url"
url state > "/runtime/$staging/state-database-url"
random_hex 32 > "/state-key/$staging/key"

# Ownership and mode are set in staging; each file is then published with a
# rename, and each volume's marker is written last.
publish() {
  v=$1 owner=$2 group=$3
  for f in "$v/$staging"/*; do
    chown "$owner:$group" "$f"
    chmod 0400 "$f"
  done
  for f in "$v/$staging"/*; do
    mv "$f" "$v/$(basename "$f")"
  done
  rmdir "$v/$staging"
  chown "$owner:$group" "$v"
}
publish /role-passwords "$pg_uid" "$pg_gid"
publish /owner "$backend_uid" "$backend_uid"
publish /runtime "$backend_uid" "$backend_uid"
publish /state-key "$backend_uid" "$backend_uid"

for f in $(expected); do
  [ -s "$f" ] || fail "generated credential $f is empty"
done
for v in $volumes; do
  : > "$v/$marker"
  chmod 0400 "$v/$marker"
done
echo "db-credentials: fresh credentials generated"
