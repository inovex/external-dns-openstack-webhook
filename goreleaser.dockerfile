FROM gcr.io/distroless/static-debian12

# Gophercloud expects this to be set
ENV HOME=/home
COPY external-dns-openstack-webhook /external-dns-openstack-webhook
ENTRYPOINT ["/external-dns-openstack-webhook"]
