FROM gcr.io/distroless/static-debian13:latest@sha256:448810357cc8e67b7208988abdba0f5ec3d3aebee0a5b13eb624979792cffc0f



# Gophercloud expects this to be set
ENV HOME=/

# Let's set some sane defaults to amekt
ENV OS_CLIENT_CONFIG_FILE=/etc/openstack/clouds.yaml
ENV OS_CLOUD=openstack

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/external-dns-openstack-webhook /external-dns-openstack-webhook
USER 1000
ENTRYPOINT ["/external-dns-openstack-webhook"]
