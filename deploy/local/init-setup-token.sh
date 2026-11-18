#!/bin/sh
# Private, stable operator authorization; never print it to container logs.
set -eu
umask 077
mkdir -p /setup
if [ ! -e /setup/token ]; then
  head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > /setup/token.new
  printf '\n' >> /setup/token.new
  chmod 600 /setup/token.new
  mv /setup/token.new /setup/token
fi
[ -f /setup/token ] && [ ! -L /setup/token ]
[ "$(wc -c < /setup/token)" -ge 33 ]
chown -R 65532:65532 /setup
chmod 700 /setup
chmod 600 /setup/token
