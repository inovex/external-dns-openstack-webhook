FROM gcr.io/distroless/static-debian13:latest@sha256:034df5c1737e8241f6ed6423a12e720ee0f6d6f088609ee172f98f6f2166ef5d



# Gophercloud expects this to be set
ENV HOME=/

# Let's set some sane defaults to amekt
ENV OS_CLIENT_CONFIG_FILE=/etc/openstack/clouds.yaml
ENV OS_CLOUD=openstack

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/external-dns-openstack-webhook /external-dns-openstack-webhook
USER 1000
ENTRYPOINT ["/external-dns-openstack-webhook"]
