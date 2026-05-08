#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${IMAGE_TAG:-}" ]]; then
  echo "IMAGE_TAG is required" >&2
  exit 1
fi

if [[ -z "${ZONE_NAME:-}" ]]; then
  echo "ZONE_NAME is required" >&2
  exit 1
fi

image="ghcr.io/${GITHUB_REPOSITORY_OWNER}/external-dns-t-cloud-public-webhook:${IMAGE_TAG}"

echo "Starting webhook container from image: ${image}"
echo "Using zone type: ${MATRIX_ZONE_TYPE}"
echo "Using cloud entry: ${OS_CLOUD}"

for attempt in $(seq 1 30); do
  if docker pull "${image}"; then
    break
  fi
  if [[ "${attempt}" -eq 30 ]]; then
    echo "Image did not become available in GHCR: ${image}" >&2
    exit 1
  fi
  echo "Image not available yet, retrying in 10s (${attempt}/30)..."
  sleep 10
done

container_id="$(
  docker run -d \
  --name webhook \
  -p 8888:8888 \
  -p 8080:8080 \
  -e OS_CLIENT_CONFIG_FILE=/etc/t-cloud-public/clouds.yaml \
  -e OS_CLOUD="${OS_CLOUD}" \
  -e ZONE_TYPE="${MATRIX_ZONE_TYPE}" \
  -e OS_ZONE_TYPE="${MATRIX_ZONE_TYPE}" \
  -v "$PWD/.ci/t-cloud-public:/etc/t-cloud-public:ro" \
  "${image}" \
  --domain-filter "${ZONE_NAME}"
)"

echo "Started container id: ${container_id}"
docker ps -a --filter "id=${container_id}"

sleep 2
echo "Initial container logs:"
docker logs webhook || true

echo "Container state after initial wait:"
docker inspect --format '{{.State.Status}}' webhook || true
