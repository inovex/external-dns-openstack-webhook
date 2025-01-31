# ExternalDNS - OpenStack Designate Webhook

This is an [ExternalDNS provider](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/webhook-provider.md) for [OpenStack's Designate DNS server](https://docs.openstack.org/designate/latest/).
This projects externalizes the yet in-tree [OpenStack Designate provider](https://github.com/kubernetes-sigs/external-dns/tree/master/provider/designate).

### 🌪 This project is in a very early stage and likely does not do what you expect!

## Development

To run the webhook locally, you'll need to create a [clouds.yaml](https://docs.openstack.org/python-openstackclient/pike/configuration/index.html#clouds-yaml)
file and put it in one of the standard-locations.
Then set the cloud to be used in the `OS_CLOUD` environemnt variable.
You can then start the webhook server using:

```sh
go run cmd/webhook/main.go
```
