#!/bin/sh
set -eu
umask 077
{ printf 'hostnossl all all all reject\n'; cat "$PGDATA/pg_hba.conf"; } > "$PGDATA/pg_hba.conf.new"
mv "$PGDATA/pg_hba.conf.new" "$PGDATA/pg_hba.conf"
