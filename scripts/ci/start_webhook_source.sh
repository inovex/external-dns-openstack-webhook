#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${ZONE_NAME:-}" ]]; then
  echo "ZONE_NAME is required" >&2
  exit 1
fi

mkdir -p ./build/bin
go build -o ./build/bin/external-dns-t-cloud-public-webhook ./cmd/webhook
nohup env \
  ZONE_TYPE="${MATRIX_ZONE_TYPE}" \
  ./build/bin/external-dns-t-cloud-public-webhook --domain-filter "${ZONE_NAME}" \
  >/tmp/webhook.log 2>&1 &

echo $! >/tmp/webhook.pid
