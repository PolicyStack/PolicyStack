# Disconnected clusters

A disconnected cluster pulls images from a mirror registry and installs operators from mirrored catalogs. PolicyStack applies what [oc-mirror v2](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/disconnected_environments/mirroring-in-disconnected-environments) generates, and points every operator at the mirrored catalogs:

- the [`disconnected`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/disconnected) element applies the mirror sets, catalogs and CA trust;
- `global.catalogSources` replaces each operator's `subscription.source` ([Global keys](values.md#global-keys)).

## Limits

- Argo CD on the hub still downloads the `policy-library` chart from `policystack.github.io` when it renders an element. The hub needs that URL, directly or through its proxy. Could mirror to a disconnected repo and change all references as well.
- Neither piece mirrors images. Run oc-mirror yourself.
- No OpenShift Update Service is deployed. The `upgrade` element holds an upgrade until the target version is in the cluster's `availableUpdates`. Point `stack.upgrade.config.upstream` at your own update service.

## Site layer

Put the site's settings in one datacenter layer and list it in each fleet file at that site. Elements are enabled per cluster, as for any element ([Values cascade](values.md)).

```yaml
# values/datacenters/dc2.yaml
global:
  catalogSources:
    redhat-operators: cs-redhat-operator-index-v4-20

stack:
  disconnected:
    config:
      # From oc-mirror's working-dir/cluster-resources, unchanged
      clusterResources:
        - apiVersion: config.openshift.io/v1
          kind: ImageDigestMirrorSet
          metadata:
            name: idms-operator-0
          spec:
            imageDigestMirrors:
              - mirrors: [mirror.dc2.example.com:8443/redhat]
                source: registry.redhat.io/redhat
        - apiVersion: operators.coreos.com/v1alpha1
          kind: CatalogSource
          metadata:
            name: cs-redhat-operator-index-v4-20
            namespace: openshift-marketplace
          spec:
            image: mirror.dc2.example.com:8443/redhat/redhat-operator-index:v4.20
            sourceType: grpc
      registryCAs:
        mirror.dc2.example.com..8443: |
          -----BEGIN CERTIFICATE-----
          ...
      # The CA of the internal Git and Helm servers
      trustedCA: |
        -----BEGIN CERTIFICATE-----
        ...
```

```yaml
# values/clusters/prod-west-1.yaml
stack:
  disconnected:
    enabled: true
```

Every key in `catalogSources` must name a source the elements use, and the CatalogSource it maps to must be in `clusterResources`. Each oc-mirror run adds release signature ConfigMaps to `cluster-resources`. Add them to `clusterResources` as well, or the cluster rejects an upgrade to that release.

If `remediations` is enabled, add the mirror registry to its `allowedRegistries`.

## What the element applies

| Toggle | Applies |
|---|---|
| `resources` | Each object in `config.clusterResources`, as it is |
| `registry-ca` | ConfigMap `registry-cas` in `openshift-config` from `config.registryCAs`, and `image.config.openshift.io/cluster` `additionalTrustedCA`. A key is the registry host, with `..` before a port. |
| `trusted-ca` | ConfigMap `trusted-cas` in `openshift-config`, and the cluster Proxy's `trustedCA` pointing at it. The network operator adds it to every injected trust bundle, including the one the `gitops` element mounts into the Argo CD repo server. |
| `operatorhub` | `OperatorHub` `disableAllDefaultSources: true` |

`registry-ca` and `trusted-ca` render nothing while their input is empty.

`trusted-cas` is the cluster's `user-ca-bundle` (the installer's `additionalTrustBundle`) followed by `config.trustedCA`. `user-ca-bundle` is read on the cluster and never written, so a CA removed from `config.trustedCA` leaves the bundle on the next evaluation, and edits to `user-ca-bundle` carry over. A Proxy that already points at a ConfigMap other than `user-ca-bundle` is repointed, and that ConfigMap's CAs drop out of the trust bundle: add them to `config.trustedCA` first. Turning `trusted-ca` off or emptying `config.trustedCA` leaves the Proxy on a stale `trusted-cas`, so its CAs stay trusted: first set the Proxy's `trustedCA.name` back to `user-ca-bundle` (`""` if that ConfigMap does not exist), then delete `trusted-cas`.

## Hosted clusters

The HostedCluster owns a hosted cluster's mirrors, image config and proxy. Set `imageContentSources`, `additionalTrustBundle` and `configuration` under `install.hostedCluster.spec` instead ([Cluster lifecycle](lifecycle.md)). In the cluster's own file, keep only CatalogSources in `clusterResources` and turn `registry-ca` and `trusted-ca` off.

## The hub

The hub needs the mirrored catalogs before PolicyStack runs. Pass the site layer to the operators chart ([Install the operators](install.md#install-the-operators)) after applying the oc-mirror `cluster-resources` by hand:

```sh
helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
  -f values/datacenters/dc2.yaml | oc apply -f -
```

Then list the layer in the hub's fleet file and enable `disconnected` in `values/clusters/<hubName>.yaml`, so the element manages those objects from then on.
