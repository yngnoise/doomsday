#!/bin/sh
set -eu

REPO_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$REPO_ROOT"

COMPOSE_FILE="${COMPOSE_FILE:-compose.load.yml}"
RESULTS_DIR="$REPO_ROOT/loadtest/results"
mkdir -p "$RESULTS_DIR"

export DATABASE_URL="postgres://postgres:postgres@localhost:55432/doomsday_load?sslmode=disable"
export REDIS_URL="localhost:56379"
export LOAD_DROP_ID="demo-wraith-jacket"

stack_started=false
finalize() {
  exit_code="$?"
  trap - EXIT INT TERM
  set +e
  if [ "$stack_started" = true ]; then
    echo "Final invariant check"
    go run ./cmd/invariantcheck -drop "$LOAD_DROP_ID"
    invariant_code="$?"
    docker compose -f "$COMPOSE_FILE" down --volumes
    if [ "$exit_code" -eq 0 ] && [ "$invariant_code" -ne 0 ]; then
      exit_code="$invariant_code"
    fi
  fi
  exit "$exit_code"
}
trap finalize EXIT INT TERM

stack_started=true
docker compose -f "$COMPOSE_FILE" up --build --detach postgres redis toxiproxy toxiproxy-config api
docker compose -f "$COMPOSE_FILE" --profile load build k6

attempts=0
until curl --silent --fail http://localhost:18080/health/ready >/dev/null; do
  if [ "$attempts" -ge 60 ]; then
    echo "API did not become ready" >&2
    exit 1
  fi
  attempts=$((attempts + 1))
  sleep 1
done

for scenario in drop-opening contention checkout sse; do
  echo "Running k6 scenario: ${scenario}"
  docker compose -f "$COMPOSE_FILE" --profile load run --rm \
    --env "SCENARIO=${scenario}" \
    k6 run /scripts/scenarios.js --summary-export "/results/${scenario}.json"
  echo "Checking invariants after ${scenario}"
  go run ./cmd/invariantcheck -drop "$LOAD_DROP_ID"
done

BASE_URL="http://localhost:18080" TOXIPROXY_URL="http://localhost:18474" \
  sh loadtest/failure-recovery.sh
go run ./cmd/invariantcheck -drop "$LOAD_DROP_ID"

echo "Load and failure suite passed"
