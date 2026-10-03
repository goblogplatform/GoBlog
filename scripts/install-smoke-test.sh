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
# What it cannot do is the wizard's last step, authorising a GitHub OAuth
# app. It writes placeholder client_id/client_secret into the container's
# .env and restarts instead, so GitHub's callback and the first admin login
# are not covered.
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
APP_DIR=/go/src/github.com/compscidr/goblog
TITLE="Smoke Test Blog"
TMP=$(mktemp -d)

cleanup() {
  code=$?
  if [ "$code" -ne 0 ]; then
    echo "--- goblog log (last 60 lines)" >&2
    docker logs "$APP" 2>&1 | tail -60 >&2 || true
  fi
  docker rm -f "$APP" "$DBC" >/dev/null 2>&1 || true
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
docker network rm "$NET" >/dev/null 2>&1 || true
docker network create "$NET" >/dev/null

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

step "starting goblog ($DB)"
docker run -d --name "$APP" --network "$NET" -p "127.0.0.1:$PORT:7000" "$IMAGE" >/dev/null
wait_for goblog 60 app_up

step "a fresh install shows the database wizard"
request "GET /" 200 "$BASE/"
body_has "Install Wizard" "GET /"
body_has 'name="dbtype"' "GET /"

step "Test Database"
request "POST /test_db, no database type" 400 -X POST "$BASE/test_db" -d dbtype=
if [ "$DB" != sqlite ]; then
  request "POST /test_db, wrong password" 400 -X POST "$BASE/test_db" "${badform[@]}"
fi
request "POST /test_db" 200 -X POST "$BASE/test_db" "${form[@]}"
body_has success "POST /test_db"

step "saving the database step connects and migrates"
request "POST /wizard_db" 303 -X POST "$BASE/wizard_db" "${form[@]}"
request "GET / after the database step" 200 "$BASE/"
body_has 'id="settings-form"' "GET / after the database step"

step "settings step"
request "PATCH /api/v1/settings" 202 -X PATCH "$BASE/api/v1/settings" -H 'Content-Type: application/json' \
  -d "[{\"key\":\"site_title\",\"value\":\"$TITLE\",\"type\":\"text\"},{\"key\":\"site_subtitle\",\"value\":\"installed by the smoke test\",\"type\":\"text\"}]"
request "GET /?page=auth" 200 "$BASE/?page=auth"
body_has 'name="client_id"' "GET /?page=auth"

# The real last step is GitHub redirecting back to /login, whose handler
# stores the credentials and registers the site's routes. With placeholders
# there is no callback, so the routes arrive with the restart below instead;
# that also covers a restart of a finished install (.env re-read, database
# reconnected, migrations re-run).
step "auth step (placeholder GitHub credentials written to .env), then restart"
docker exec "$APP" sh -c "printf 'client_id=smoke\nclient_secret=smoke\n' >> $APP_DIR/.env"
docker restart "$APP" >/dev/null
wait_for "goblog after restart" 60 app_up

check_site() {
  request "GET /" 200 "$BASE/"
  body_has "$TITLE" "GET /"
  body_lacks "Install Wizard" "GET /"
  for path in /login /search?q=goblog /robots.txt /rss.xml /sitemap.xml /theme/css/goblog.css; do
    request "GET $path" 200 "$BASE$path"
  done
  request "GET /sitemap.xml" 200 "$BASE/sitemap.xml"
  body_has "<urlset" "GET /sitemap.xml"
  request "GET /no-such-page" 404 "$BASE/no-such-page"
  request "GET /admin, logged out" 302 "$BASE/admin"
}

step "the site is up"
check_site

# A query the database rejects is logged but often still answers 200 (a
# setting silently reads as its default), so the log is part of the result.
# The wrong-password check above is the one failure that is meant to be there.
step "no panics or SQL errors in the log"
if docker logs "$APP" 2>&1 | grep -v -e "Couldn't connect to the database" -e "failed to initialize database" \
    | grep -E "panic|Error [0-9]{4} \(|SQLSTATE|syntax error|SQL logic error|no such (table|column)" >"$TMP/errors"; then
  head -20 "$TMP/errors" >&2
  fail "goblog logged errors"
fi

echo "PASS: install smoke test ($DB)"
