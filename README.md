# ExternalDNS - OpenStack Designate Webhook

This is an [ExternalDNS provider](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/webhook-provider.md) for [OpenStack's Designate DNS server](https://docs.openstack.org/designate/latest/).
It serves as a replacment for the former in-tree OpenStack Designate provider which never left the `Alpha` state and has since been removed (https://github.com/kubernetes-sigs/external-dns/pull/5126).
The webhook, while already a drop in replacement, is not perfect (yet)! If you have bugfixes and new feature suggestions - please kindly open issues and send in PRs if you feel there is something missing / broken.

## Installation

This webhook provider is run easiest as sidecar within the `external-dns` pod. This can be achieved using the 
[official `external-dns` Helm chart](https://kubernetes-sigs.github.io/external-dns/latest/charts/external-dns/)
and [its support for the `webhook` provider type]([https://kubernetes-sigs.github.io/external-dns/latest/charts/external-dns/#providers]).

Setting the `provider.name` to `webhook` allows configuration of the
`external-dns-openstack-webhook` via a few additional values:

```yaml
provider:
  name: webhook
  webhook:
    image:
      repository: ghcr.io/inovex/external-dns-openstack-webhook
      tag: 2.1.0
    extraVolumeMounts:
      - name: oscloudsyaml
        mountPath: /etc/openstack/
    resources: {}
extraVolumes:
  - name: oscloudsyaml
    secret:
      secretName: oscloudsyaml
```

The referenced `extraVolumeMount` points to a `Secret` containing a [`clouds.yaml` file](https://docs.openstack.org/python-openstackclient/latest/configuration/index.html#clouds-yaml),
which provides the OpenStack Keystone credentials to the webhook provider.
`OS_*` environment variables are not supported for configuration, since the use of a `clouds.yaml` file offers more structure, capabilities and allows for better validation.
The one exception to this is `OS_CLOUD` for setting the name of the cloud in `clouds.yaml` to use.

The following example is a basic example of a `clouds.yaml` file, using `openstack` as the cloud name (the default used by this webhook):

```yaml
clouds:
  openstack:
    auth:
      auth_url: https://auth.cloud.example.com
      application_credential_id: "TOP"
      application_credential_secret: "SECRET"
    region_name: "earth"
    interface: "public"
    auth_type: "v3applicationcredential"
```

An existing file can be converted into a Secret via kubectl:

```shell
kubectl create secret generic oscloudsyaml --namespace external-dns --from-file=clouds.yaml
```

## Bugs or feature requests

This webhook certainly still contains bugs or lacks certain features.
In such cases, please raise a GitHub issue with as much detail as possible. PRs with fixes and features are also very welcome.

## Development

To run the webhook locally, you'll also require a [clouds.yaml](https://docs.openstack.org/python-openstackclient/pike/configuration/index.html#clouds-yaml) file in one of the standard-locations.
Also the name of the entry to be used has be given via `OS_CLOUD` environment variable.
You can then start the webhook server using:

```sh
go run cmd/webhook/main.go
```
