#!/bin/sh
set -eu

attempt=1
until psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f /app/migrations/migrate.sql; do
  if [ "$attempt" -ge 30 ]; then
    echo "Database migrations failed after $attempt attempts" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 2
done

exec /app/doomsday
