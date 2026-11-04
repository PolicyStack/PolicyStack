# Disconnected clusters

Run [oc-mirror v2](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/disconnected_environments/mirroring-in-disconnected-environments) yourself. PolicyStack applies its output:

- The [`disconnected`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/disconnected) element applies the mirror sets, catalogs and CA trust.
- `global.catalogSources` points every operator at the mirrored catalog ([Global keys](values.md#global-keys)).

!!! note "Not covered"
    - **Image mirroring.** oc-mirror's job.
    - **The chart repo.** Argo CD on the hub pulls `policy-library` from `policystack.github.io`, directly or through a proxy. Or mirror the chart and repoint every `Chart.yaml`.
    - **Update Service.** Run OSUS yourself; PolicyStack only points clusters at it ([Update Service](#update-service)).
    - **RHCOS images.** Host the live ISOs for bare metal yourself ([Bare metal](#bare-metal)).

## Site layer

One datacenter layer holds the site data. List it in every fleet file at the site.

```yaml
# values/datacenters/dc2.yaml
global:
  catalogSources:
    redhat-operators: cs-redhat-operator-index-v4-20 # (1)!

stack:
  disconnected:
    config:
      clusterResources: # (2)!
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
        mirror.dc2.example.com..8443: | # (3)!
          -----BEGIN CERTIFICATE-----
          ...
      trustedCA: | # (4)!
        -----BEGIN CERTIFICATE-----
        ...
  remediations:
    config:
      allowedRegistries: # (5)!
        - mirror.dc2.example.com:8443
        - quay.io
        - registry.redhat.io
        - image-registry.openshift-image-registry.svc:5000
  upgrade:
    config:
      upstream: https://osus.dc2.example.com/api/upgrades_info/v1/graph # (6)!
```

1.  A `subscription.source` the elements use, mapped to a CatalogSource in `clusterResources`.
2.  oc-mirror's `working-dir/cluster-resources`, unchanged.
3.  Registry host, with `..` before a port.
4.  CAs of internal Git, Helm and RHCOS image servers.
5.  Only with `remediations` enabled. Lists replace, so restate the defaults.
6.  Your OSUS graph. `channel-pin` writes it before the upgrade guard needs `availableUpdates`.

Enable it per cluster ([Values cascade](values.md)):

```yaml
# values/clusters/prod-west-1.yaml
stack:
  disconnected:
    enabled: true
```

!!! warning "Release signatures"
    Each oc-mirror run adds release signature ConfigMaps to `cluster-resources`. Add them to `clusterResources`, or the cluster rejects that upgrade.

## What the element applies

Every policy is on; turn one off with `policies.<policy>.enabled: false`. Empty input renders nothing.

| Policy | Applies |
|---|---|
| `resources` | `config.clusterResources`, as is. |
| `registry-ca` | ConfigMap `openshift-config/registry-cas` from `config.registryCAs`, and `additionalTrustedCA` on `image.config.openshift.io/cluster`. |
| `trusted-ca` | ConfigMap `openshift-config/trusted-cas`, with the Proxy's `trustedCA` pointing at it. Lands in every injected trust bundle, including the one the `gitops` element mounts into Argo CD's repo server. |
| `operatorhub` | `OperatorHub` `disableAllDefaultSources: true`. |

### trusted-cas

!!! warning "Existing Proxy trustedCA"
    A Proxy pointing at a ConfigMap other than `user-ca-bundle` is repointed, and its CAs leave the trust bundle. Copy them into `config.trustedCA` first.

`trusted-cas` = `user-ca-bundle` (the installer's `additionalTrustBundle`) + `config.trustedCA`. `user-ca-bundle` is read on the cluster, never written.

??? info "Turning trusted-ca off"
    Turning `trusted-ca` off or emptying `config.trustedCA` leaves the Proxy on a stale `trusted-cas`, still trusted. Clean up by hand:

    ```sh
    oc patch proxy/cluster --type merge -p '{"spec":{"trustedCA":{"name":"user-ca-bundle"}}}' # (1)!
    oc delete configmap trusted-cas -n openshift-config
    ```

    1.  Use `"name":""` if `user-ca-bundle` does not exist.

## Update Service

The `upgrade` element holds a `version` until it shows in `availableUpdates`, so a disconnected cluster needs `stack.upgrade.config.upstream` ([Site layer](#site-layer)).

- Put the OSUS CA in `config.trustedCA`, not `registryCAs`. The CVO trusts only `trusted-ca-bundle`, which the Proxy's `trustedCA` feeds.
- `hypershift` and `hive` clusters run `-multi` payloads and the CVO asks for the multi graph. `siteconfig` clusters run `-x86_64` and the CVO asks for the amd64 graph. Mirror the matching release architecture, or the install can't pull its image and `availableUpdates` stays empty.

## Hosted clusters

The HostedCluster owns mirrors, image config and proxy. Set them in the fleet file; the element keeps only the catalog.

=== "Fleet file"

    ```yaml
    # fleet/hcp-agent.yaml
    install:
      hostedCluster:
        spec: # (1)!
          imageContentSources:
            - source: registry.redhat.io/redhat
              mirrors: [mirror.dc2.example.com:8443/redhat]
          additionalTrustBundle:
            name: mirror-ca # (2)!
          updateService: https://osus.dc2.example.com/api/upgrades_info/v1/graph # (3)!
    ```

    1.  `configuration` (image, proxy) goes here too ([Cluster lifecycle](lifecycle.md)).
    2.  Hub ConfigMap with key `ca-bundle.crt` in the HostedCluster's namespace. Create it with `install.manifests` ([Install keys](lifecycle-reference.md#install-keys)).
    3.  The hosted CVO runs on the hub; use the OSUS Route URL. Without it, a `version` bump with `channel` set is held forever.

=== "Cluster values"

    ```yaml
    # values/clusters/hcp-agent.yaml
    stack:
      disconnected:
        enabled: true
        policies:
          registry-ca:
            enabled: false
          trusted-ca:
            enabled: false
        config:
          clusterResources: # (1)!
            - apiVersion: operators.coreos.com/v1alpha1
              kind: CatalogSource
              ...
    ```

    1.  Replaces the site layer's list. CatalogSources only.

## Bare metal

`siteconfig` clusters install through the hub's assisted service, so the hub holds their mirrors. [`cim`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/cim) reads the site layer's `disconnected` config on the hub:

| From | To |
|---|---|
| `ImageDigestMirrorSet`s in `clusterResources` | `registries.conf` in ConfigMap `multicluster-engine/assisted-mirror`, referenced as AgentServiceConfig `mirrorRegistryRef` |
| `registryCAs` | `ca-bundle.crt` in the same ConfigMap |
| `trustedCA`, once `osImages` is set | ConfigMap `image-service-additional-ca`, referenced as `OSImageCACertRef` |

Assisted writes the mirrors and CAs into the discovery ISO and into each spoke's install-config, so the spoke installs from the mirror with its ImageDigestMirrorSet in place.

```yaml
# values/clusters/<hubName>.yaml
stack:
  cim:
    enabled: true
    config:
      osImages: # (1)!
        "4.20":
          version: 9.6.20251015-1 # (2)!
          url: https://images.dc2.example.com/rhcos-4.20-x86_64-live.iso
```

1.  Replaces the default list, which points at mirror.openshift.com. List every version the hub's `siteconfig` clusters install.
2.  RHCOS version of the ISO.

```yaml
# values/datacenters/dc2.yaml
install:
  releaseRepository: mirror.dc2.example.com:8443/openshift/release-images # (1)!
```

1.  The ClusterImageSet pulls by tag, which digest-only mirrors don't cover.

!!! note
    - One mirror set per hub: every `siteconfig` cluster it builds uses it. The stock SiteConfig templates have no per-cluster mirror.
    - Mirror the multicluster engine images with oc-mirror. The credential's `pullSecret` needs the mirror's auth.
    - `ibi`: put an ImageDigestMirrorSet in a ConfigMap through `install.manifests` and list it in `siteconfig.extraManifests`.

!!! warning "Removing entries"
    The AgentServiceConfig is `musthave`. A removed `osImages` entry, or a `mirrorRegistryRef` once no mirrors remain, stays on it. Remove it with `oc edit agentserviceconfig agent`.

## The hub

The hub needs the mirrored catalogs before PolicyStack runs.

1. Apply oc-mirror's output by hand:

    ```sh
    oc apply -f working-dir/cluster-resources/
    ```

2. [Install the operators](install.md#install-the-operators) with the site layer:

    ```sh
    helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
      -f values/datacenters/dc2.yaml | oc apply -f -
    ```

3. List `datacenters/dc2.yaml` in `fleet/hubs/<hubName>.yaml` and set `stack.disconnected.enabled: true` in `values/clusters/<hubName>.yaml`. The element takes over those objects.
