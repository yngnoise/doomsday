#!/bin/sh
set -eu

until curl --silent --fail http://toxiproxy:8474/version >/dev/null; do
  sleep 1
done

curl --silent --request DELETE http://toxiproxy:8474/proxies/postgres >/dev/null || true
curl --silent --request DELETE http://toxiproxy:8474/proxies/redis >/dev/null || true

curl --silent --fail --request POST http://toxiproxy:8474/proxies \
  --header 'Content-Type: application/json' \
  --data '{"name":"postgres","listen":"0.0.0.0:15432","upstream":"postgres:5432"}' >/dev/null
curl --silent --fail --request POST http://toxiproxy:8474/proxies \
  --header 'Content-Type: application/json' \
  --data '{"name":"redis","listen":"0.0.0.0:16379","upstream":"redis:6379"}' >/dev/null
