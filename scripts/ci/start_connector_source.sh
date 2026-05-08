#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${ZONE_NAME:-}" ]]; then
  echo "ZONE_NAME is required" >&2
  exit 1
fi

connector_addr="${CONNECTOR_SOURCE_SERVER:-127.0.0.1:18080}"

nohup env \
  ZONE_NAME="${ZONE_NAME}" \
  CONNECTOR_SOURCE_SERVER="${connector_addr}" \
  GOCACHE="${PWD}/.gocache" \
  go run ./cmd/ci-connector-source \
  >/tmp/connector-source.log 2>&1 &

echo $! >/tmp/connector-source.pid
echo "Started connector source on ${connector_addr}"

for _ in $(seq 1 60); do
  if grep -q "connector source listening on ${connector_addr}" /tmp/connector-source.log 2>/dev/null; then
    echo "Connector source is ready on ${connector_addr}"
    exit 0
  fi

  if [[ -f /tmp/connector-source.pid ]] && ! kill -0 "$(cat /tmp/connector-source.pid)" 2>/dev/null; then
    echo "Connector source exited before becoming ready" >&2
    cat /tmp/connector-source.log >&2 || true
    exit 1
  fi

  sleep 1
done

echo "Connector source did not become ready on ${connector_addr}" >&2
cat /tmp/connector-source.log >&2 || true
exit 1
