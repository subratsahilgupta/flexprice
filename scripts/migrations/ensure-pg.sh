#!/usr/bin/env bash
# Make sure a scratch Postgres is reachable, starting one if not.
#
# The migration checks build throwaway databases; they need a Postgres, not YOUR
# Postgres. This keeps that an implementation detail rather than a setup step
# every developer has to be told about.
#
# Does nothing when something already answers on the port — which covers CI, where
# the workflow provides a service container, and a developer who already has one.
set -euo pipefail
PGHOST_="${PGHOST_:-localhost}"; PGPORT_="${PGPORT_:-5440}"
PGUSER_="${PGUSER_:-flexprice}"; PGPASS_="${PGPASS_:-flexprice123}"
NAME="${MIG_PG_CONTAINER:-flexprice-mig-pg}"
IMAGE="${MIG_PG_IMAGE:-postgres:17}"   # matches AlloyDB's major
export PGPASSWORD="$PGPASS_"

reachable() {
  psql -X -tAc "SELECT 1" \
    "postgres://$PGUSER_@$PGHOST_:$PGPORT_/postgres?sslmode=disable" >/dev/null 2>&1
}

reachable && exit 0

command -v docker >/dev/null 2>&1 || {
  echo "No Postgres on $PGHOST_:$PGPORT_ and docker is not available." >&2
  echo "Start one yourself, or set PGHOST_/PGPORT_ to an existing instance." >&2
  exit 1; }

if [ -n "$(docker ps -aq -f "name=^${NAME}$")" ]; then
  docker start "$NAME" >/dev/null
else
  echo "starting scratch postgres ($IMAGE on :$PGPORT_) …" >&2
  docker run -d --name "$NAME" \
    -e POSTGRES_USER="$PGUSER_" -e POSTGRES_PASSWORD="$PGPASS_" \
    -e POSTGRES_DB=postgres -p "$PGPORT_:5432" "$IMAGE" >/dev/null
fi

for _ in $(seq 1 30); do reachable && exit 0; sleep 1; done
echo "scratch postgres did not become ready — try: docker logs $NAME" >&2
exit 1
