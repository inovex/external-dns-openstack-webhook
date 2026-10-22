FROM gcr.io/distroless/static-debian12:latest@sha256:d75cdd72874d4790092fcb1b058493ecf6bb5bf2b2b897045b00ff01d91843f2



# Gophercloud expects this to be set
ENV HOME=/

# Let's set some sane defaults to amekt
ENV OS_CLIENT_CONFIG_FILE=/etc/openstack/clouds.yaml
ENV OS_CLOUD=openstack

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/external-dns-openstack-webhook /external-dns-openstack-webhook
USER 1000
ENTRYPOINT ["/external-dns-openstack-webhook"]
