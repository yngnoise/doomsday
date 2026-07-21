#!/bin/sh
set -eu

BASE_URL="${BASE_URL:-http://localhost:18080}"
TOXIPROXY_URL="${TOXIPROXY_URL:-http://localhost:18474}"

set_proxy() {
  name="$1"
  enabled="$2"
  curl --silent --fail --request POST "${TOXIPROXY_URL}/proxies/${name}" \
    --header 'Content-Type: application/json' \
    --data "{\"enabled\":${enabled}}" >/dev/null
}

restore_proxies() {
  set_proxy postgres true || true
  set_proxy redis true || true
}
trap restore_proxies EXIT

wait_for_status() {
  path="$1"
  expected="$2"
  attempts=0
  while [ "$attempts" -lt 20 ]; do
    status="$(curl --silent --output /tmp/doomsday-health.json --write-out '%{http_code}' --max-time 5 "${BASE_URL}${path}" || true)"
    if [ "$status" = "$expected" ]; then
      return 0
    fi
    attempts=$((attempts + 1))
    sleep 1
  done
  echo "${path} did not reach HTTP ${expected}" >&2
  return 1
}

exercise_failure() {
  dependency="$1"
  echo "Disabling ${dependency} proxy"
  set_proxy "$dependency" false
  wait_for_status /health/ready 503
  wait_for_status /health/live 200
  wait_for_status /health/dependencies 503
  if ! grep -q "\"${dependency}\":{\"status\":\"down\"" /tmp/doomsday-health.json; then
    echo "dependency health did not identify ${dependency} as down" >&2
    cat /tmp/doomsday-health.json >&2
    return 1
  fi
  echo "Restoring ${dependency} proxy"
  set_proxy "$dependency" true
  wait_for_status /health/ready 200
}

wait_for_status /health/ready 200
exercise_failure redis
exercise_failure postgres
echo "PostgreSQL and Redis failure recovery checks passed"
