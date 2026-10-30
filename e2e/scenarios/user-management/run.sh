#!/usr/bin/env bash
# UC-00 BR-08/09/10/11: first-Admin bootstrap, Admin-only user management,
# password change and Admin password reset with full session revocation —
# API level, no browser.
# Everything lives in a private temporary directory; nothing touches a shared
# platform, database, cluster or cloud.
set -u

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
BACKEND="$ROOT/backend"
PORT="${ORCH_E2E_PORT:-18099}"
BASE="http://127.0.0.1:$PORT"

TMP="$(mktemp -d)"
chmod 700 "$TMP"
SERVER_PID=""
cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null
  [ -n "$SERVER_PID" ] && wait "$SERVER_PID" 2>/dev/null
  rm -rf "$TMP"
}
trap cleanup EXIT

FAILURES=0
check() { # check <label> <actual> <expected>
  if [ "$2" = "$3" ]; then
    echo "PASS: $1"
  else
    echo "FAIL: $1 (got [$2], want [$3])"
    FAILURES=$((FAILURES + 1))
  fi
}

# api <jar> <method> <path> [body] -> "<status> <body>" on stdout
api() {
  local jar="$1" method="$2" path="$3" body="${4:-}"
  local args=(-s -o "$TMP/resp.json" -w '%{http_code}' -b "$jar" -c "$jar" -X "$method")
  [ -n "$body" ] && args+=(-H 'Content-Type: application/json' -d "$body")
  local code
  code="$(curl "${args[@]}" "$BASE$path")" || { echo "000 {}"; return; }
  echo "$code $(cat "$TMP/resp.json")"
}

echo "== build =="
if ! (cd "$BACKEND" && go build -o "$TMP/orchestrator" ./cmd/orchestrator); then
  echo "FAIL: go build ./cmd/orchestrator"
  exit 1
fi

echo "== BR-10: bootstrap the first ADMIN =="
printf 'bootstrap-secret-1' > "$TMP/admin-password"
chmod 600 "$TMP/admin-password"

OUT="$("$TMP/orchestrator" -bootstrap-admin -bootstrap-admin-password-file "$TMP/admin-password" -state "$TMP/state.json" 2>&1)"
case "$OUT" in
  *'ADMIN account "admin" created'*) echo "PASS: bootstrap creates the ADMIN" ;;
  *) echo "FAIL: bootstrap output: $OUT"; FAILURES=$((FAILURES + 1)) ;;
esac
case "$OUT" in
  *bootstrap-secret-1*) echo "FAIL: bootstrap output leaks the password"; FAILURES=$((FAILURES + 1)) ;;
  *) echo "PASS: bootstrap output keeps the password secret" ;;
esac
if "$TMP/orchestrator" -bootstrap-admin -bootstrap-admin-password-file "$TMP/admin-password" -state "$TMP/state.json" >/dev/null 2>&1; then
  echo "FAIL: duplicate bootstrap was accepted"
  FAILURES=$((FAILURES + 1))
else
  echo "PASS: duplicate bootstrap refused"
fi
if "$TMP/orchestrator" -bootstrap-admin -bootstrap-admin-password-file "$TMP/admin-password" >/dev/null 2>&1; then
  echo "FAIL: bootstrap accepted an ephemeral store"
  FAILURES=$((FAILURES + 1))
else
  echo "PASS: bootstrap refuses the ephemeral store"
fi

echo "== start the API =="
"$TMP/orchestrator" -state "$TMP/state.json" -addr "127.0.0.1:$PORT" -ui-dir "" > "$TMP/server.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 100); do
  curl -s -o /dev/null "$BASE/api/v1/healthz" && break
  sleep 0.1
done
check "server is healthy" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/healthz")" "200"

echo "== BR-09: Admin-only user management =="
R="$(api "$TMP/anon.jar" GET /api/v1/users)"
check "anonymous list is unauthorized" "${R%% *}" "401"

R="$(api "$TMP/admin.jar" POST /api/v1/auth/sign-in '{"username":"admin","password":"bootstrap-secret-1"}')"
check "admin signs in" "${R%% *}" "200"

R="$(api "$TMP/dev.jar" POST /api/v1/auth/sign-in '{"username":"developer","password":"test-password"}')"
check "developer signs in" "${R%% *}" "200"

R="$(api "$TMP/dev.jar" GET /api/v1/users)"
check "developer list is forbidden" "${R%% *}" "403"
R="$(api "$TMP/dev.jar" POST /api/v1/users '{"username":"mallory","password":"password-1","role":"ADMIN"}')"
check "developer cannot create an account" "${R%% *}" "403"

R="$(api "$TMP/admin.jar" GET /api/v1/users)"
check "admin lists accounts" "${R%% *}" "200"
case "$R" in
  *'"username":"developer"'*'"username":"platform-engineer"'*) echo "PASS: list holds the seeded accounts" ;;
  *) echo "FAIL: list body: $R"; FAILURES=$((FAILURES + 1)) ;;
esac
case "$R" in
  *assword*) echo "FAIL: list leaks password material: $R"; FAILURES=$((FAILURES + 1)) ;;
  *) echo "PASS: list has no password material" ;;
esac

R="$(api "$TMP/admin.jar" POST /api/v1/users '{"username":"developer","password":"password-1","role":"DEVELOPER"}')"
check "duplicate username refused" "${R%% *}" "409"
case "$R" in *'"code":"DUPLICATE"'*) echo "PASS: duplicate code" ;; *) echo "FAIL: duplicate body: $R"; FAILURES=$((FAILURES + 1)) ;; esac
R="$(api "$TMP/admin.jar" POST /api/v1/users '{"username":"No Good!","password":"password-1","role":"DEVELOPER"}')"
check "invalid username refused" "${R%% *}" "400"
R="$(api "$TMP/admin.jar" POST /api/v1/users '{"username":"newdev","password":"short","role":"DEVELOPER"}')"
check "short password refused" "${R%% *}" "400"
R="$(api "$TMP/admin.jar" POST /api/v1/users '{"username":"newdev","password":"password-1","role":"SUPERUSER"}')"
check "invalid role refused" "${R%% *}" "400"

R="$(api "$TMP/admin.jar" POST /api/v1/users '{"username":"newdev","password":"password-1","role":"DEVELOPER"}')"
check "admin creates an account" "${R%% *}" "201"
NEWDEV_ID="$(printf '%s' "$R" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
[ -n "$NEWDEV_ID" ] || { echo "FAIL: create response has no account id: $R"; exit 1; }
R="$(api "$TMP/newdev.jar" POST /api/v1/auth/sign-in '{"username":"newdev","password":"password-1"}')"
check "the new account signs in" "${R%% *}" "200"

echo "== BR-08: password change revokes every session =="
api "$TMP/newdev-second.jar" POST /api/v1/auth/sign-in '{"username":"newdev","password":"password-1"}' > /dev/null

R="$(api "$TMP/newdev.jar" POST /api/v1/auth/change-password '{"currentPassword":"wrong","newPassword":"new-secret-1","confirmPassword":"new-secret-1"}')"
check "wrong current password refused" "${R%% *}" "401"
R="$(api "$TMP/newdev.jar" POST /api/v1/auth/change-password '{"currentPassword":"password-1","newPassword":"new-secret-1","confirmPassword":"different-1"}')"
check "confirmation mismatch refused" "${R%% *}" "400"
R="$(api "$TMP/newdev.jar" POST /api/v1/auth/change-password '{"currentPassword":"password-1","newPassword":"short","confirmPassword":"short"}')"
check "short new password refused" "${R%% *}" "400"
R="$(api "$TMP/newdev.jar" GET /api/v1/auth/session)"
check "failed attempts keep the session" "${R%% *}" "200"

R="$(api "$TMP/newdev.jar" POST /api/v1/auth/change-password '{"currentPassword":"password-1","newPassword":"new-secret-1","confirmPassword":"new-secret-1"}')"
check "password change succeeds" "${R%% *}" "204"
R="$(api "$TMP/newdev.jar" GET /api/v1/auth/session)"
check "current session ended" "${R%% *}" "401"
R="$(api "$TMP/newdev-second.jar" GET /api/v1/auth/session)"
check "the other session ended too" "${R%% *}" "401"
R="$(api "$TMP/dev.jar" GET /api/v1/auth/session)"
check "other accounts keep their sessions" "${R%% *}" "200"

R="$(api "$TMP/old.jar" POST /api/v1/auth/sign-in '{"username":"newdev","password":"password-1"}')"
check "old password rejected" "${R%% *}" "401"
R="$(api "$TMP/new.jar" POST /api/v1/auth/sign-in '{"username":"newdev","password":"new-secret-1"}')"
check "new password signs in" "${R%% *}" "200"

echo "== BR-11: Admin password reset revokes the target sessions =="
R="$(api "$TMP/reset-a.jar" POST /api/v1/auth/sign-in '{"username":"newdev","password":"new-secret-1"}')"
check "target signs in (first session)" "${R%% *}" "200"
R="$(api "$TMP/reset-b.jar" POST /api/v1/auth/sign-in '{"username":"newdev","password":"new-secret-1"}')"
check "target signs in (second session)" "${R%% *}" "200"

R="$(api "$TMP/dev.jar" POST "/api/v1/users/$NEWDEV_ID/reset-password" '{"newPassword":"reset-secret-1","confirmPassword":"reset-secret-1"}')"
check "non-admin reset is forbidden" "${R%% *}" "403"
R="$(api "$TMP/anon.jar" POST "/api/v1/users/$NEWDEV_ID/reset-password" '{"newPassword":"reset-secret-1","confirmPassword":"reset-secret-1"}')"
check "anonymous reset is unauthorized" "${R%% *}" "401"
R="$(api "$TMP/admin.jar" POST "/api/v1/users/$NEWDEV_ID/reset-password" '{"newPassword":"reset-secret-1","confirmPassword":"different-1"}')"
check "reset confirmation mismatch refused" "${R%% *}" "400"
R="$(api "$TMP/admin.jar" POST "/api/v1/users/$NEWDEV_ID/reset-password" '{"newPassword":"short","confirmPassword":"short"}')"
check "reset short password refused" "${R%% *}" "400"
R="$(api "$TMP/admin.jar" POST "/api/v1/users/no-such-user/reset-password" '{"newPassword":"reset-secret-1","confirmPassword":"reset-secret-1"}')"
check "reset of an unknown account is not found" "${R%% *}" "404"
R="$(api "$TMP/reset-a.jar" GET /api/v1/auth/session)"
check "failed reset attempts keep the target session" "${R%% *}" "200"

R="$(api "$TMP/admin.jar" POST "/api/v1/users/$NEWDEV_ID/reset-password" '{"newPassword":"reset-secret-1","confirmPassword":"reset-secret-1"}')"
check "admin resets the password" "${R%% *}" "204"
R="$(api "$TMP/reset-a.jar" GET /api/v1/auth/session)"
check "first target session ended" "${R%% *}" "401"
R="$(api "$TMP/reset-b.jar" GET /api/v1/auth/session)"
check "second target session ended" "${R%% *}" "401"
R="$(api "$TMP/admin.jar" GET /api/v1/auth/session)"
check "admin session survives the reset" "${R%% *}" "200"

R="$(api "$TMP/old-reset.jar" POST /api/v1/auth/sign-in '{"username":"newdev","password":"new-secret-1"}')"
check "pre-reset password rejected" "${R%% *}" "401"
R="$(api "$TMP/new-reset.jar" POST /api/v1/auth/sign-in '{"username":"newdev","password":"reset-secret-1"}')"
check "reset password signs in" "${R%% *}" "200"

echo
if [ "$FAILURES" -gt 0 ]; then
  echo "FAIL: $FAILURES assertion(s) failed; server log follows"
  tail -20 "$TMP/server.log"
  exit 1
fi
echo "PASS: user-management scenario"
