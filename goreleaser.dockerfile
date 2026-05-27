FROM gcr.io/distroless/static-debian12

# Cloud auth libraries expect a writable home directory to exist.
ENV HOME=/

# Default runtime config for a mounted clouds.yaml.
ENV OS_CLIENT_CONFIG_FILE=/etc/openstack/clouds.yaml
ENV OS_CLOUD=openstack

COPY external-dns-t-cloud-public-webhook /external-dns-t-cloud-public-webhook
USER 1000
ENTRYPOINT ["/external-dns-t-cloud-public-webhook"]
