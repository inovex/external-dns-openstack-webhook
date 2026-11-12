FROM gcr.io/distroless/static-debian13:latest@sha256:1a64f46364620465ef9e9a8f5413757dfdf22695e050bbfd7303eeaf17a59383



# Gophercloud expects this to be set
ENV HOME=/

# Let's set some sane defaults to amekt
ENV OS_CLIENT_CONFIG_FILE=/etc/openstack/clouds.yaml
ENV OS_CLOUD=openstack

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/external-dns-openstack-webhook /external-dns-openstack-webhook
USER 1000
ENTRYPOINT ["/external-dns-openstack-webhook"]
