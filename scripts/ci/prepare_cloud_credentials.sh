#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${OS_CLIENT_CONFIG_FILE:-}" ]]; then
  echo "OS_CLIENT_CONFIG_FILE is required" >&2
  exit 1
fi

if [[ -z "${OS_CLOUDS_YAML_CONTENT:-}" ]]; then
  echo "OS_CLOUDS_YAML_CONTENT is required" >&2
  exit 1
fi

mkdir -p "$(dirname "$OS_CLIENT_CONFIG_FILE")"
cat >"$OS_CLIENT_CONFIG_FILE" <<EOF
${OS_CLOUDS_YAML_CONTENT}
EOF
