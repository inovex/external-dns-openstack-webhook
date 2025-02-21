FROM gcr.io/distroless/static-debian12

# Gophercloud expects this to be set
ENV HOME=/

# Let's set some sane defaults to amekt
ENV OS_CLIENT_CONFIG_FILE=/etc/openstack/clouds.yaml
ENV OS_CLOUD=openstack
COPY external-dns-openstack-webhook /external-dns-openstack-webhook
ENTRYPOINT ["/external-dns-openstack-webhook"]
