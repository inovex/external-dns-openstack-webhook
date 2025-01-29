FROM scratch
COPY external-dns-openstack-webhook /external-dns-openstack-webhook
ENTRYPOINT ["/external-dns-openstack-webhook"]
