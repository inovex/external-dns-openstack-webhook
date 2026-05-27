# ExternalDNS - T-Cloud Public DNS Webhook

This is an [ExternalDNS provider](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/webhook-provider.md) for T-Cloud Public DNS.
It replaces the former in-tree OpenStack Designate provider that never left `Alpha` and was later removed from ExternalDNS (https://github.com/kubernetes-sigs/external-dns/pull/5126).
The webhook is already a drop-in replacement for that provider, but it is still evolving. If you find bugs or missing features, please open an issue or send a PR.

## Installation

This webhook provider is run easiest as sidecar within the `external-dns` pod. This can be achieved using the 
[official `external-dns` Helm chart](https://kubernetes-sigs.github.io/external-dns/latest/charts/external-dns/)
and [its support for the `webhook` provider type](https://kubernetes-sigs.github.io/external-dns/latest/charts/external-dns/#providers).

Setting the `provider.name` to `webhook` allows configuration of the
`external-dns-t-cloud-public-webhook` via a few additional values:

```yaml
provider:
  name: webhook
  webhook:
    image:
      repository: ghcr.io/opentelekomcloud/external-dns-t-cloud-public-webhook
      tag: 1.1.2
    extraVolumeMounts:
      - name: tcloudpubliccloudsyaml
        mountPath: /etc/openstack/
    resources: {}
extraVolumes:
  - name: tcloudpubliccloudsyaml
    secret:
      secretName: tcloudpubliccloudsyaml
```

The referenced `extraVolumeMount` points to a `Secret` containing a [`clouds.yaml` file](https://docs.openstack.org/python-openstackclient/latest/configuration/index.html#clouds-yaml),
which provides the T-Cloud Public IAM credentials to the webhook provider.
`OS_*` environment variables are not supported for configuration, since the use of a `clouds.yaml` file offers more structure, capabilities and allows for better validation.
The one exception to this is `OS_CLOUD` for setting the name of the cloud in `clouds.yaml` to use.

Note: custom TLS settings from `clouds.yaml`, such as `cacert`, `cert`, `key`, or disabled certificate verification, are not explicitly supported by the current webhook auth bootstrap yet. Environments that rely on private trust roots or client-certificate TLS may require a future dedicated implementation.

## Zone selection

The webhook can manage public and private Designate zones from one ExternalDNS instance.

By default, endpoints are created in public DNS zones. To target a private zone for a specific Kubernetes object, set the webhook provider-specific annotation:

```yaml
metadata:
  annotations:
    external-dns.alpha.kubernetes.io/webhook-zone-type: private
```

Supported values are `public` and `private`. If the annotation is omitted, `public` is used.

When managing the same DNS name in both public and private zones, add distinct ExternalDNS set identifiers so the Service source does not merge the endpoints before they reach the webhook:

```yaml
metadata:
  annotations:
    external-dns.alpha.kubernetes.io/hostname: split-horizon.example.com
    external-dns.alpha.kubernetes.io/target: 192.0.2.20
    external-dns.alpha.kubernetes.io/set-identifier: public
---
metadata:
  annotations:
    external-dns.alpha.kubernetes.io/hostname: split-horizon.example.com
    external-dns.alpha.kubernetes.io/target: 10.0.0.20
    external-dns.alpha.kubernetes.io/set-identifier: private
    external-dns.alpha.kubernetes.io/webhook-zone-type: private
```

## Examples

An end-to-end Kubernetes example with ExternalDNS, the webhook sidecar, credentials Secret, RBAC, public/private Services, CNAMEs, and split-horizon records is available at [`examples`](examples/k8s-webhook-test-services.yaml).

The example requires these zones to exist in T-Cloud Public before applying the manifest:

- `public-zone.webhook.com.` as `public`
- `private-zone.webhook.internal.` as `private`
- `shared-zone.webhook.test.` as `public`
- `shared-zone.webhook.test.` as `private`

The manifest is intended as a test fixture. Review image tags, credentials, domain names, and ownership settings before using it in a real cluster.

## Credentials

The following example is a basic example of a `clouds.yaml` file, using `openstack` as the cloud name (the default used by this webhook):

```yaml
clouds:
  openstack:
    auth:
      auth_url: https://iam.eu-de.otc.t-systems.com/v3
      user_domain_name: "OTC000000000010000XXXXX"
      project_name: "eu-de_project"
      password: secret
      username: name
    identity_api_version: 3
    region_name: "eu-de"
    interface: "public"
```

An existing file can be converted into a Secret via kubectl:

```shell
kubectl create secret generic tcloudpubliccloudsyaml --namespace external-dns --from-file=clouds.yaml
```

## Bugs or feature requests

This webhook certainly still contains bugs or lacks certain features.
In such cases, please raise a GitHub issue with as much detail as possible. PRs with fixes and features are also very welcome.

## Development

To run the webhook locally, you'll also require a [clouds.yaml](https://docs.openstack.org/python-openstackclient/pike/configuration/index.html#clouds-yaml) file in one of the standard-locations.
The name of the entry to use must be provided via the `OS_CLOUD` environment variable.
You can then start the webhook server using:

```sh
go run cmd/webhook/main.go
```

Private-zone selection is controlled through endpoint annotations, so the local webhook command is the same for public and private zones.
