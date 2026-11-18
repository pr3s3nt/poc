#!/bin/sh
# Root initContainer only; runtime mounts this emptyDir read-only.
set -eu
umask 077
for source in /source/*; do
  [ -f "$source" ] || continue
  cp -L "$source" "/private/$(basename "$source")"
done
if [ -f /private/vault-ca.crt ]; then
  cat /etc/ssl/certs/ca-certificates.crt /private/vault-ca.crt > /private/ca-bundle.crt
fi
chown -R "${PRIVATE_UID}:${PRIVATE_UID}" /private
chmod 700 /private
chmod 400 /private/*
