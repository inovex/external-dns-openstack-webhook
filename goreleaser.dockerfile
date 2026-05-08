FROM gcr.io/distroless/static-debian12

# Gophercloud expects this to be set
ENV HOME=/

# Let's set some sane defaults to amekt
ENV OS_CLIENT_CONFIG_FILE=/etc/t-cloud-public/clouds.yaml
ENV OS_CLOUD=t-cloud-public
COPY external-dns-t-cloud-public-webhook /external-dns-t-cloud-public-webhook
USER 1000
ENTRYPOINT ["/external-dns-t-cloud-public-webhook"]
