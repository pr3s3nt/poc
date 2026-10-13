#!/bin/sh
# PostgreSQL docker-entrypoint-initdb.d script: runs only when the data
# directory is initialized for the first time, so it never touches a retained
# database. It creates the option-A roles (ADR-014): the owner, which owns the
# empty orchestrator database and runs the explicit owner setup, and the
# separate Console and State runtime logins. Grants are made by the owner
# setup (orchestrator -setup-database), not here. Passwords come from files
# written by init-db-credentials.sh and are never printed.
set -eu
dir=/run/orchestrator/role-passwords
for role in owner console state; do
  [ -s "$dir/$role" ] || { echo "missing generated password for the $role role" >&2; exit 1; }
done
# psql reads each password from its file (\set with a backtick command), so
# no password appears in any process argument list.
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<'SQL'
\set owner_pw `cat /run/orchestrator/role-passwords/owner`
\set console_pw `cat /run/orchestrator/role-passwords/console`
\set state_pw `cat /run/orchestrator/role-passwords/state`
CREATE ROLE orchestrator_owner LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD :'owner_pw';
CREATE ROLE orchestrator_console LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD :'console_pw';
CREATE ROLE orchestrator_state LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD :'state_pw';
ALTER DATABASE orchestrator OWNER TO orchestrator_owner;
SQL
