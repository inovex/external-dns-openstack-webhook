#!/usr/bin/env bash
set -euo pipefail

for _ in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/healthz >/dev/null; then
    exit 0
  fi
  sleep 2
done

echo "Webhook did not become healthy" >&2
if [[ -f /tmp/webhook.log ]]; then
  echo "===== /tmp/webhook.log =====" >&2
  cat /tmp/webhook.log >&2
fi
if [[ -f /tmp/webhook.pid ]]; then
  pid="$(cat /tmp/webhook.pid || true)"
  if [[ -n "${pid}" ]] && ps -p "${pid}" >/dev/null 2>&1; then
    echo "Webhook process ${pid} is still running" >&2
  else
    echo "Webhook process is not running" >&2
  fi
fi
exit 1
