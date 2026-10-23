FROM gcr.io/distroless/static-debian13:latest@sha256:15f8637509792e05b97d83451b0b6ccfa4abd548f997e406ac1e72c87fe1fa78



# Gophercloud expects this to be set
ENV HOME=/

# Let's set some sane defaults to amekt
ENV OS_CLIENT_CONFIG_FILE=/etc/openstack/clouds.yaml
ENV OS_CLOUD=openstack

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/external-dns-openstack-webhook /external-dns-openstack-webhook
USER 1000
ENTRYPOINT ["/external-dns-openstack-webhook"]
