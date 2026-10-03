#!/usr/bin/env bash
# Install smoke test (#651): a fresh goblog container, walked through the
# install wizard the way a new user's browser would, against one database.
#
#   scripts/install-smoke-test.sh sqlite|mysql|postgres
#
# Needs docker and curl. Builds the image from the working tree unless
# GOBLOG_IMAGE names one that already exists. GOBLOG_PORT (default 7007) is
# the host port the container is published on.
#
# The container runs the way the quick start says to: GOBLOG_DATA_DIR on a
# volume. Half way through, the container is removed and a new one started
# on the same volume, which is what an upgrade does (#653). With
# GOBLOG_SMOKE_LEGACY=1 there is no data directory and the container is
# restarted instead: the layout of installs that predate GOBLOG_DATA_DIR.
#
# It finishes the install the way the wizard offers first: an admin account
# with a password. The other way, authorising a GitHub OAuth app, cannot run
# in CI and is not covered.
set -euo pipefail

DB=${1:-}
case "$DB" in
  sqlite|mysql|postgres) ;;
  *) echo "usage: $0 sqlite|mysql|postgres" >&2; exit 2 ;;
esac

IMAGE=${GOBLOG_IMAGE:-goblog:smoke}
PORT=${GOBLOG_PORT:-7007}
BASE="http://127.0.0.1:$PORT"
NET="goblog-smoke-$DB"
APP="goblog-smoke-$DB-app"
DBC="goblog-smoke-$DB-db"
VOL="goblog-smoke-$DB-data"
LEGACY=${GOBLOG_SMOKE_LEGACY:-}
if [ -n "$LEGACY" ]; then
  run_args=()
else
  run_args=(-e GOBLOG_DATA_DIR=/data -v "$VOL:/data")
fi
TITLE="Smoke Test Blog"
TMP=$(mktemp -d)

cleanup() {
  code=$?
  if [ "$code" -ne 0 ]; then
    echo "--- goblog log (last 60 lines)" >&2
    docker logs "$APP" 2>&1 | tail -60 >&2 || true
  fi
  docker rm -f "$APP" "$DBC" >/dev/null 2>&1 || true
  docker volume rm "$VOL" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$TMP"
  exit "$code"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
step() { echo "== $*"; }

# request DESC WANT_STATUS CURL_ARGS...: the body is left in $TMP/body.
request() {
  desc=$1 want=$2; shift 2
  got=$(curl -sS -o "$TMP/body" -w '%{http_code}' "$@") || fail "$desc: curl failed"
  [ "$got" = "$want" ] || { head -c 600 "$TMP/body" >&2; echo >&2; fail "$desc: status $got, want $want"; }
}
body_has() { grep -qF -- "$1" "$TMP/body" || { head -c 600 "$TMP/body" >&2; echo >&2; fail "$2: body does not contain '$1'"; }; }
body_lacks() { ! grep -qF -- "$1" "$TMP/body" || fail "$2: body contains '$1'"; }

wait_for() { # wait_for DESC SECONDS COMMAND...
  desc=$1 secs=$2; shift 2
  for _ in $(seq "$secs"); do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  fail "timed out after ${secs}s waiting for $desc"
}
app_up() { [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/")" = 200 ]; }

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  step "building $IMAGE"
  docker build -q -t "$IMAGE" --build-arg VERSION=smoke "$(dirname "$0")/.."
fi

docker rm -f "$APP" "$DBC" >/dev/null 2>&1 || true
docker volume rm "$VOL" >/dev/null 2>&1 || true
docker network rm "$NET" >/dev/null 2>&1 || true
docker network create "$NET" >/dev/null

start_app() {
  docker run -d --name "$APP" --network "$NET" -p "127.0.0.1:$PORT:7000" "${run_args[@]}" "$IMAGE" >/dev/null
  wait_for goblog 60 app_up
}

# The wizard's database form, as the browser posts it.
case "$DB" in
  sqlite)
    form=(-d dbtype=sqlite -d sqlite_file=goblog.db)
    ;;
  mysql)
    step "starting mysql"
    docker run -d --name "$DBC" --network "$NET" \
      -e MYSQL_ROOT_PASSWORD=rootpass -e MYSQL_DATABASE=goblog \
      -e MYSQL_USER=goblog -e MYSQL_PASSWORD=goblogpass mysql:9.6 >/dev/null
    # Over TCP: the image's first-run init server only listens on the socket.
    wait_for mysql 120 docker exec "$DBC" mysql -h127.0.0.1 -ugoblog -pgoblogpass -e 'select 1' goblog
    form=(-d dbtype=mysql -d "mysql_host=$DBC" -d mysql_port=3306 -d mysql_user=goblog -d mysql_pass=goblogpass -d mysql_db=goblog)
    badform=(-d dbtype=mysql -d "mysql_host=$DBC" -d mysql_port=3306 -d mysql_user=goblog -d mysql_pass=wrong -d mysql_db=goblog)
    ;;
  postgres)
    step "starting postgres"
    docker run -d --name "$DBC" --network "$NET" \
      -e POSTGRES_USER=goblog -e POSTGRES_PASSWORD=goblogpass -e POSTGRES_DB=goblog postgres:16-alpine >/dev/null
    wait_for postgres 60 docker exec -e PGPASSWORD=goblogpass "$DBC" psql -h127.0.0.1 -Ugoblog -c 'select 1' goblog
    form=(-d dbtype=postgres -d "postgres_host=$DBC" -d postgres_port=5432 -d postgres_user=goblog -d postgres_pass=goblogpass -d postgres_db=goblog -d postgres_sslmode=disable)
    badform=(-d dbtype=postgres -d "postgres_host=$DBC" -d postgres_port=5432 -d postgres_user=goblog -d postgres_pass=wrong -d postgres_db=goblog -d postgres_sslmode=disable)
    ;;
esac

step "starting goblog ($DB${LEGACY:+, no data directory})"
start_app

# The session cookie is carried by hand, so the test does not depend on what
# curl's cookie jar makes of its attributes. keep_cookies stores whatever
# cookies the last response set.
COOKIE=""
keep_cookies() {
  set_cookies=$(grep -i '^set-cookie:' "$TMP/headers" | sed -E 's/^[^:]*: *([^;]*).*/\1/' | paste -sd ';' - | sed 's/;/; /g' || true)
  [ -z "$set_cookies" ] || COOKIE=$set_cookies
}
# as_owner is request with the session cookie; the response's cookies are kept.
as_owner() {
  desc=$1 want=$2; shift 2
  request "$desc" "$want" -D "$TMP/headers" -H "Cookie: $COOKIE" "$@"
  keep_cookies
}
header_has() { grep -qiF -- "$1" "$TMP/headers" || { cat "$TMP/headers" >&2; fail "$2: response headers do not contain '$1'"; }; }

ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD="smoke test password"
SETTINGS_JSON="[{\"key\":\"site_title\",\"value\":\"$TITLE\",\"type\":\"text\"},{\"key\":\"site_subtitle\",\"value\":\"installed by the smoke test\",\"type\":\"text\"}]"

step "a fresh install asks for the setup code"
request "GET /" 200 "$BASE/"
body_has "Install Wizard" "GET /"
body_has 'name="setup_code"' "GET /"
body_lacks 'name="dbtype"' "GET /"
request "POST /test_db without the setup code" 403 -X POST "$BASE/test_db" "${form[@]}"
request "POST /wizard_db without the setup code" 403 -X POST "$BASE/wizard_db" "${form[@]}"
request "POST /wizard/unlock, wrong code" 200 -X POST "$BASE/wizard/unlock" -d setup_code=AAAA-AAAA-AAAA
body_has "not the setup code" "POST /wizard/unlock, wrong code"

step "entering the setup code from the log"
SETUP_CODE=$(docker logs "$APP" 2>&1 | sed -n 's/.*GoBlog setup code: \([A-Z0-9-]*\).*/\1/p' | tail -1)
[ -n "$SETUP_CODE" ] || fail "no setup code in the log"
as_owner "POST /wizard/unlock" 303 -X POST "$BASE/wizard/unlock" -d "setup_code=$SETUP_CODE"
[ -n "$COOKIE" ] || fail "POST /wizard/unlock set no cookie"
# Plain http to an IP address: a Secure cookie would be dropped by a browser,
# and the install could not get past this page (#665).
! grep -i '^set-cookie:' "$TMP/headers" | grep -qi '; *secure' || fail "the session cookie is Secure over plain http on an IP address"
as_owner "GET /" 200 "$BASE/"
body_has 'name="dbtype"' "GET / with the setup code"

step "Test Database"
as_owner "POST /test_db, no database type" 400 -X POST "$BASE/test_db" -d dbtype=
if [ "$DB" != sqlite ]; then
  as_owner "POST /test_db, wrong password" 400 -X POST "$BASE/test_db" "${badform[@]}"
fi
as_owner "POST /test_db" 200 -X POST "$BASE/test_db" "${form[@]}"
body_has success "POST /test_db"

step "saving the database step connects and migrates"
as_owner "POST /wizard_db" 303 -X POST "$BASE/wizard_db" "${form[@]}"
as_owner "GET / after the database step" 200 "$BASE/"
body_has 'id="settings-form"' "GET / after the database step"

step "settings step"
request "PATCH /api/v1/settings without the setup code" 401 -X PATCH "$BASE/api/v1/settings" -H 'Content-Type: application/json' -d "$SETTINGS_JSON"
as_owner "PATCH /api/v1/settings" 202 -X PATCH "$BASE/api/v1/settings" -H 'Content-Type: application/json' -d "$SETTINGS_JSON"
echo "smoke test upload" >"$TMP/smoke-upload.txt"
request "POST /api/v1/upload without the setup code" 401 -X POST "$BASE/api/v1/upload" -F "file=@$TMP/smoke-upload.txt"
as_owner "POST /api/v1/upload" 200 -X POST "$BASE/api/v1/upload" -F "file=@$TMP/smoke-upload.txt"
body_has "/uploads/smoke-upload.txt" "POST /api/v1/upload"

step "admin account step"
as_owner "GET /?page=auth" 200 "$BASE/?page=auth"
body_has 'action="/wizard/admin"' "GET /?page=auth"
request "POST /wizard/admin without the setup code" 200 -X POST "$BASE/wizard/admin" \
  --data-urlencode "email=$ADMIN_EMAIL" --data-urlencode "password=$ADMIN_PASSWORD" --data-urlencode "password_confirm=$ADMIN_PASSWORD"
body_has 'name="setup_code"' "POST /wizard/admin without the setup code"
as_owner "POST /wizard/admin, passwords differ" 200 -X POST "$BASE/wizard/admin" \
  --data-urlencode "email=$ADMIN_EMAIL" --data-urlencode "password=$ADMIN_PASSWORD" --data-urlencode "password_confirm=something else"
body_has "do not match" "POST /wizard/admin, passwords differ"
as_owner "POST /wizard/admin, short password" 200 -X POST "$BASE/wizard/admin" \
  --data-urlencode "email=$ADMIN_EMAIL" --data-urlencode "password=short" --data-urlencode "password_confirm=short"
body_has "at least 10 characters" "POST /wizard/admin, short password"
as_owner "POST /wizard/admin" 303 -X POST "$BASE/wizard/admin" \
  --data-urlencode "email=$ADMIN_EMAIL" --data-urlencode "password=$ADMIN_PASSWORD" --data-urlencode "password_confirm=$ADMIN_PASSWORD"
header_has "location: /admin" "POST /wizard/admin"

step "the wizard leaves the browser logged in as the admin, and is now closed"
as_owner "GET /admin/dashboard" 200 "$BASE/admin/dashboard"
as_owner "POST /wizard_db after install" 403 -X POST "$BASE/wizard_db" "${form[@]}"
as_owner "POST /wizard/admin after install" 403 -X POST "$BASE/wizard/admin" \
  --data-urlencode "email=other@example.com" --data-urlencode "password=$ADMIN_PASSWORD" --data-urlencode "password_confirm=$ADMIN_PASSWORD"

check_site() {
  request "GET /" 200 "$BASE/"
  body_has "$TITLE" "GET /"
  body_lacks "Install Wizard" "GET /"
  for path in /search?q=goblog /robots.txt /rss.xml /sitemap.xml /theme/css/goblog.css; do
    request "GET $path" 200 "$BASE$path"
  done
  request "GET /login" 200 "$BASE/login"
  body_has 'action="/login/password"' "GET /login"
  request "GET /sitemap.xml" 200 "$BASE/sitemap.xml"
  body_has "<urlset" "GET /sitemap.xml"
  request "GET /uploads/smoke-upload.txt" 200 "$BASE/uploads/smoke-upload.txt"
  body_has "smoke test upload" "GET /uploads/smoke-upload.txt"
  request "GET /no-such-page" 404 "$BASE/no-such-page"
  request "GET /admin/dashboard, logged out" 302 "$BASE/admin/dashboard"
}

# login PASSWORD WANT_LOCATION: the login page's password form.
login() {
  COOKIE=""
  as_owner "POST /login/password" 303 -X POST "$BASE/login/password" \
    --data-urlencode "email=$ADMIN_EMAIL" --data-urlencode "password=$1" --data-urlencode "next=/admin"
  header_has "location: $2" "POST /login/password"
}

step "the site is up"
check_site

if [ -n "$LEGACY" ]; then
  step "restarting the container"
  docker restart "$APP" >/dev/null
  wait_for "goblog after restart" 60 app_up
else
  # Everything the install wrote has to be on the volume: .env, the SQLite
  # file, the upload.
  step "replacing the container, keeping only the data volume"
  docker logs "$APP" >"$TMP/first.log" 2>&1
  docker rm -f "$APP" >/dev/null
  start_app
fi
check_site

step "signing in with the password"
login "not the password" "/login?password=1&login_error=invalid"
as_owner "GET /admin/dashboard after a failed login" 302 "$BASE/admin/dashboard"
login "$ADMIN_PASSWORD" "/admin"
as_owner "GET /admin/dashboard" 200 "$BASE/admin/dashboard"

step "goblog reset-admin-password"
NEW_PASSWORD=$(docker exec "$APP" ./goblog reset-admin-password 2>/dev/null | sed -n "s/^New password for $ADMIN_EMAIL: //p")
[ -n "$NEW_PASSWORD" ] || fail "reset-admin-password printed no password"
as_owner "GET /admin/dashboard with the session from before the reset" 302 "$BASE/admin/dashboard"
login "$ADMIN_PASSWORD" "/login?password=1&login_error=invalid"
login "$NEW_PASSWORD" "/admin"
as_owner "GET /admin/dashboard" 200 "$BASE/admin/dashboard"

# A query the database rejects is logged but often still answers 200 (a
# setting silently reads as its default), so the log is part of the result.
# The wrong database password above is the one failure that is meant to be there.
step "no panics or SQL errors in the log"
if { cat "$TMP/first.log" 2>/dev/null; docker logs "$APP" 2>&1; } | grep -v -e "Couldn't connect to the database" -e "failed to initialize database" \
    | grep -E "panic|Error [0-9]{4} \(|SQLSTATE|syntax error|SQL logic error|no such (table|column)" >"$TMP/errors"; then
  head -20 "$TMP/errors" >&2
  fail "goblog logged errors"
fi

echo "PASS: install smoke test ($DB)"
