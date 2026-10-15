#!/bin/sh
set -eu

if [ -z "${POSTGRES_ADMIN_HOST:-}" ]; then
  echo "missing POSTGRES_ADMIN_HOST" >&2
  exit 1
fi
if [ -z "${POSTGRES_ADMIN_PORT:-}" ]; then
  echo "missing POSTGRES_ADMIN_PORT" >&2
  exit 1
fi
if [ -z "${POSTGRES_ADMIN_USER:-}" ]; then
  echo "missing POSTGRES_ADMIN_USER" >&2
  exit 1
fi
if [ -z "${POSTGRES_ADMIN_PASSWORD:-}" ]; then
  echo "missing POSTGRES_ADMIN_PASSWORD" >&2
  exit 1
fi
if [ -z "${THINGS_BASELINE_DB:-}" ]; then
  echo "missing THINGS_BASELINE_DB" >&2
  exit 1
fi
if [ -z "${THINGS_BASELINE_DB_USER:-}" ]; then
  echo "missing THINGS_BASELINE_DB_USER" >&2
  exit 1
fi
if [ -z "${THINGS_BASELINE_DB_PASSWORD:-}" ]; then
  echo "missing THINGS_BASELINE_DB_PASSWORD" >&2
  exit 1
fi

[ "$THINGS_BASELINE_DB" = "ming_iot_things_baseline" ] || {
  echo "CONFIG_INVALID: protected database target" >&2
  exit 2
}
[ "$THINGS_BASELINE_DB_USER" = "ming_iot_things_baseline" ] || {
  echo "CONFIG_INVALID: unexpected database role" >&2
  exit 2
}

export PGHOST="$POSTGRES_ADMIN_HOST"
export PGPORT="$POSTGRES_ADMIN_PORT"
export PGUSER="$POSTGRES_ADMIN_USER"
export PGPASSWORD="$POSTGRES_ADMIN_PASSWORD"

psql \
  --dbname=postgres \
  --set=ON_ERROR_STOP=1 \
  --set=app_user="$THINGS_BASELINE_DB_USER" \
  --set=db_name="$THINGS_BASELINE_DB" <<'SQL'
\getenv app_password THINGS_BASELINE_DB_PASSWORD

SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'app_user', :'app_password')
WHERE NOT EXISTS (
  SELECT 1 FROM pg_roles WHERE rolname = :'app_user'
)
\gexec

SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'app_user', :'app_password')
\gexec

SELECT format('CREATE DATABASE %I OWNER %I', :'db_name', :'app_user')
WHERE NOT EXISTS (
  SELECT 1 FROM pg_database WHERE datname = :'db_name'
)
\gexec

SELECT format('ALTER DATABASE %I OWNER TO %I', :'db_name', :'app_user')
\gexec
SQL

psql \
  --dbname="$THINGS_BASELINE_DB" \
  --set=ON_ERROR_STOP=1 <<'SQL'
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS postgis_topology;
SELECT current_database();
SELECT postgis_full_version();
SQL
