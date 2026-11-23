#!/bin/sh
# Runs before the backend mounts its workspace as the runtime user.
set -eu
umask 077

data=$1
[ -d "$data" ] && [ ! -L "$data" ]
chown "$PRIVATE_UID:$PRIVATE_UID" "$data"
chmod 00700 "$data"

for workspace in recovery module-cache template-source-cache delivery-git tfroot plugin-cache; do
  directory="$data/$workspace"
  [ ! -L "$directory" ]
  mkdir -p "$directory"
  chown "$PRIVATE_UID:$PRIVATE_UID" "$directory"
  chmod 00700 "$directory"
  # Restore privacy on retained files previously updated by Kubernetes fsGroup.
  find "$directory" -xdev \( -type d -o -type f \) -perm /077 -exec chmod go-rwx '{}' +
done
