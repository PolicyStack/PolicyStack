# Disconnected clusters

Read [How PolicyStack works](../concepts/overview.md) and [Installation](../start/install.md) first; this page changes those steps for a site with no internet.

A disconnected cluster cannot reach the internet. PolicyStack normally pulls from five places on the internet, and each one is replaced by something you run at the site:

| Normally reached | By | Disconnected replacement | Set up |
|---|---|---|---|
| GitHub | Argo CD on the hub, your repository's CI, and copier | Internal Git. policy-library, the chart every element renders through, is in the repo (`lib/`), so Git is all Argo CD reads | By hand ([PolicyStack releases](#policystack-releases)) |
| Container images and operator catalogs | Every cluster | oc-mirror's mirror sets and CatalogSources, applied by the `disconnected` element; `global.catalogSources` points every operator at the mirrored catalog | [Site layer](#site-layer) |
| Public certificate authorities | Every cluster | Your mirror registry's CA and your internal servers' CAs, applied by the `disconnected` element | [Site layer](#site-layer) |
| Red Hat's upgrade graph | Every cluster's CVO | Your OSUS, set by the `upgrade` element or the HostedCluster | [Update Service](#update-service) |
| Release images and RHCOS ISOs on quay.io and mirror.openshift.com | The hub, for clusters it builds | `install.releaseRepository`, and per-engine mirror settings | [Clusters the hub builds](#clusters-the-hub-builds) |

Terms used on this page:

| Term | Meaning |
|---|---|
| oc-mirror | [v2](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/disconnected_environments/mirroring-in-disconnected-environments) copies release images and operator catalogs into your registry and writes the cluster objects that point clusters at it into `working-dir/cluster-resources/`. You run it; PolicyStack applies its output. |
| CVO | Cluster Version Operator. Runs on every cluster, reads ClusterVersion, fetches the upgrade graph and performs upgrades. |
| OSUS | OpenShift Update Service. Serves a local copy of the upgrade graph. You run it; PolicyStack only points clusters at it. |
| Site layer | A [values layer](../concepts/values.md) holding one datacenter's mirror data, such as `values/datacenters/dc2.yaml`. Every cluster at the site lists it in its fleet file's `valueFiles`. |
| HostedCluster, standalone | A HostedCluster is the hub object describing a hosted control plane cluster, whose control plane runs as pods on the hub. Any other cluster is standalone. |

Other terms: [Glossary](../concepts/overview.md#glossary).

Order of work: mirror, set up Git, write the site layer, bring up the hub, then add spokes.

## Before you start

### What to mirror

The mirrored operator catalog must hold every package, at the channel, that the elements you enable subscribe to. Otherwise that element's `install` OperatorPolicy stays NonCompliant, with the Subscription reporting that no operator was found in the channel. No tool generates the list; read it from the elements:

```sh
yq -N '.stack[].operatorPolicies[] | .subscription.name + " " + .subscription.channel' \
  stack/{acm,gitops,metallb}/values.yaml  # the elements you enable
# advanced-cluster-management release-2.17
# openshift-gitops-operator latest
# metallb-operator stable
```

The elements you enable are the `stack.<key>.enabled: true` entries in the layers your fleet files list. A layer that overrides a channel sets `stack.<key>.operatorPolicies.<op>.subscription.channel`; mirror that channel too. Two more things go in the list:

- Every package an operator requires. ODF's `odf-operator`, for one, pulls in `ocs-operator`, `mcg-operator` and more; take the list from the product's disconnected docs.
- Every version a layer pins. A missing version fails the same way. A channel listed without `minVersion`/`maxVersion` keeps only its newest bundle, so a layer that pins `versions` ([Operator versions](operators.md#pinning)) needs a range on that channel covering the pin.

Every shipped element subscribes from `redhat-operators`; your own elements may use `certified-operators` or others, which then need their catalog mirrored too.

```yaml
# imageset-config.yaml
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  platform:
    channels:
      - name: stable-4.20
        minVersion: 4.20.8
        maxVersion: 4.20.12 # (1)!
    architectures: [multi] # (2)!
    graph: true # (3)!
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: advanced-cluster-management
          channels: [{name: release-2.17}]
        - name: multicluster-engine # (4)!
          channels: [{name: stable-2.12}]
        - name: openshift-gitops-operator
          channels: [{name: latest}]
        - name: metallb-operator
          channels: [{name: stable}]
```

1.  Cover every `version` a fleet file targets and every hop on the way. A version outside the range is not in your OSUS graph, so its bump waits at the guard ([Update Service](#update-service)).
2.  The payload architecture your clusters run (`amd64` in oc-mirror is the `x86_64` release tag). Clusters the hub builds with `hypershift` or `hive` run `multi`, `siteconfig` clusters run `amd64` ([Clusters the hub builds](#clusters-the-hub-builds)). Imported clusters keep the payload they were installed with. List both when you have both.
3.  Mirrors the graph data your OSUS serves, and makes oc-mirror write an `updateService.yaml` for it.
4.  ACM installs it itself, so no element lists it. Mirror the channel that pairs with your ACM release: `stable-2.12` for ACM 2.17 (ACM release notes).

### PolicyStack releases

Your repository is created and updated from upstream's Git tags ([Installation](../start/install.md#create-your-repository)), so the site needs a copy of upstream's repository, tags included. policy-library is in it (`lib/`); there is no chart repository to mirror.

1.  Mirror upstream on a connected machine, then carry it across and push it to your Git server. Repeat to pick up new releases:

    ```sh
    git clone --mirror https://github.com/PolicyStack/PolicyStack.git
    git -C PolicyStack.git push --mirror https://git.example.internal/mirrors/PolicyStack.git
    ```

2.  On every machine that runs copier, send the upstream URL to the mirror:

    ```sh
    git config --global url."https://git.example.internal/mirrors/PolicyStack.git".insteadOf \
      https://github.com/PolicyStack/PolicyStack.git
    ```

    copier reaches upstream only through `git`, so the answers files keep the upstream URL in `_src_path` and every command in [Installation](../start/install.md) and [Upgrading PolicyStack](upgrading.md) works unchanged.

    !!! warning "Global or system config only"
        copier clones upstream into a temporary directory, where your repository's own `.git/config` doesn't apply. Set the rewrite with `--global` or `--system`, or for one shell with `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=url.<mirror>.insteadOf GIT_CONFIG_VALUE_0=<upstream>`. A rewrite in your repository's own config is ignored, and copier tries GitHub.

3.  Install copier from your Python package mirror: `pipx install copier` with `PIP_INDEX_URL` pointing at it. `mise install` installs the pinned copier the same way.

CI in your repository needs the tools in `mise.toml` and the validator's Go modules (`GOPROXY` pointing at your Go module proxy). The CRDs the validator checks against are committed in `tools/validator/crds/`, so it needs nothing else from the internet ([CRD schemas](../develop/validation.md#crd-schemas)).

### Git and the hub's Argo CD

Argo CD on the hub reads your repository before any Policy exists, and PolicyStack never manages that Argo CD instance (the hub layer turns off the `gitops` element's `instance` policy, [Installation](../start/install.md#hand-the-operators-to-policystack)). So `trustedCA` below never reaches it. If your Git server uses a private CA, add it to Argo CD by hand, as a [repository TLS certificate](https://argo-cd.readthedocs.io/en/stable/user-guide/private-repositories/).

## Site layer

One file holds everything about the site's mirror. Every cluster at the site lists it in its fleet file:

```yaml
# fleet/prod-west-1.yaml
revision: main
valueFiles:
  - environments/prod.yaml
  - datacenters/dc2.yaml
```

Build `clusterResources` from oc-mirror's output, which it writes under the directory you passed as `--workspace` or `file://`. Each object becomes one list item:

```sh
cd <dir>/working-dir/cluster-resources
yq ea -p yaml -o yaml -P '[.] | map(del(.status)) | {"clusterResources": .}' \
  $(ls | grep -v -e updateService -e signature-configmap.yaml)
```

- `updateService.yaml` is an UpdateService, which needs the OSUS operator, so `resources` goes NonCompliant on any cluster without it. Apply it by hand on the cluster that runs OSUS.
- oc-mirror writes the signature ConfigMap twice, as `signature-configmap.json` and `signature-configmap.yaml`. The command keeps the JSON copy; `-p yaml` reads it, as JSON is valid YAML.

Paste the output, indented, as `stack.disconnected.config.clusterResources`:

```yaml
# values/datacenters/dc2.yaml
global:
  catalogSources:
    redhat-operators: cs-redhat-operator-index-v4-20 # (1)!

stack:
  disconnected:
    enabled: true # (2)!
    config:
      clusterResources: # (3)!
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
        mirror.dc2.example.com..8443: | # (4)!
          -----BEGIN CERTIFICATE-----
          ...
      trustedCA: | # (5)!
        -----BEGIN CERTIFICATE-----
        ...
  remediations:
    config:
      allowedRegistries: # (6)!
        - mirror.dc2.example.com:8443
        - quay.io
        - registry.redhat.io
        - image-registry.openshift-image-registry.svc:5000
      allowedRegistriesForImport:
        - domainName: mirror.dc2.example.com:8443
          insecure: false
        - domainName: quay.io
          insecure: false
        - domainName: registry.redhat.io
          insecure: false
  upgrade:
    config:
      upstream: https://osus.dc2.example.com/api/upgrades_info/v1/graph # (7)!
```

1.  Every Subscription names its catalog, and every shipped element names `redhat-operators`, which oc-mirror does not create: it names the mirrored catalog `cs-<index>-<tag>`. This map renames the source in every OperatorPolicy the elements render, and in the hub's bootstrap Subscriptions. With it, `metallb` renders `source: cs-redhat-operator-index-v4-20`. Add one key per catalog you mirror.
2.  `global` applies to every cluster that lists this layer, whether or not it enables the element. Enabling the element in the same file keeps the mapping and the CatalogSource it points to together. Otherwise a cluster that lists the layer but leaves the element off has every operator pointed at a catalog it does not have, and no operator installs or upgrades. Your repository's CI validator ([POLICY038](../reference/rules.md#rules)) only checks that a source is a known catalog name or a `catalogSources` key, not that the CatalogSource exists on the cluster.
3.  The `yq` output. Applied as is, with `musthave`: the listed fields are set and nothing else on the object is touched ([ACM policies in brief](../concepts/acm.md#configurationpolicy)).
4.  The mirror registry's CA, keyed by host. A ConfigMap key cannot contain `:`, so OpenShift reads `..` as the port separator.
5.  CAs of your other internal HTTPS servers: Git, Helm, OSUS, the RHCOS image server.
6.  The `remediations` element restricts which registries the cluster may pull from. It only takes effect where it is enabled, and then blocks every registry not listed, the mirror included. Keep the source registries: pods still reference images by their source name and the mirror sets redirect the pull. Lists replace across layers ([Merge rules](../concepts/values.md#merge-rules)), so restate the element's defaults.
7.  Your OSUS graph URL ([Update Service](#update-service)). The `upgrade` element ships enabled, so this repoints every standalone cluster that lists the layer, the hub included, at once.

!!! info "registryCAs or trustedCA"
    `registryCAs` becomes ConfigMap `openshift-config/registry-cas`, referenced by the cluster image config's `additionalTrustedCA`: trusted for image pulls, builds and image stream imports. `trustedCA` goes into the cluster-wide trust bundle through the Proxy's `trustedCA`, which the CVO, operators and every ConfigMap labelled `config.openshift.io/inject-trusted-cabundle` use. Put the mirror registry's CA in `registryCAs` and every other internal server's CA in `trustedCA`. A server used both ways goes in both. The CVO trusts only the cluster trust bundle, so an OSUS CA in `registryCAs` alone leaves `RetrievedUpdates=False`.

!!! warning "Before you enable the element"
    - **Existing image config CA.** An image config whose `additionalTrustedCA` names another ConfigMap is repointed at `registry-cas`, and that ConfigMap's registries stop being trusted for pulls. Copy its keys into `config.registryCAs` first: `oc get image.config cluster -o jsonpath={.spec.additionalTrustedCA.name}`.
    - **Default catalogs disappear.** The `operatorhub` policy sets `disableAllDefaultSources: true`, so `redhat-operators` and the other default CatalogSources go away. Any Subscription not renamed by `global.catalogSources`, including ones created by hand, stops resolving. Put the mapping and the mirrored CatalogSource in place in the same commit that enables the element.
    - **Release signatures.** Before an upgrade the CVO checks the release image's signature. A disconnected cluster reads it from ConfigMap `openshift-config-managed/mirrored-release-signatures`, which oc-mirror writes when it mirrors releases. Without a signature for the release, the upgrade stops with ClusterVersion `ReleaseAccepted=False` and a message that the release cannot be verified. Each oc-mirror run writes that ConfigMap with only its own releases ([Changing the site data](#changing-the-site-data)).

### Check it

On the hub, `oc get policies.policy.open-cluster-management.io -n <cluster>` lists `pstack.resources-disconnected-<cluster>`, `pstack.registry-ca-disconnected-<cluster>`, `pstack.trusted-ca-disconnected-<cluster>` and `pstack.operatorhub-disconnected-<cluster>`, all Compliant. A NonCompliant one names the object at fault ([Compliance states](../concepts/acm.md#compliance-states)). On the cluster:

```sh
oc get imagedigestmirrorsets,catalogsources -A
oc get packagemanifests -n openshift-marketplace -l catalog=cs-redhat-operator-index-v4-20 # the mirrored packages
oc get mcp # nodes take new mirror sets through a pool update; wait for UPDATED True
```

## The hub

The hub needs the mirrored catalog before PolicyStack exists to apply it, because the bootstrap installs ACM and GitOps from it.

1. Install the hub disconnected, following Red Hat's docs, with the mirror registry's CA in the install-config `additionalTrustBundle`. The steps below apply no CA, so the hub must already trust the mirror.
2. Apply oc-mirror's output by hand, except the UpdateService:

    ```sh
    cd working-dir/cluster-resources
    for f in $(ls | grep -v updateService); do oc apply -f "$f"; done
    ```

3. [Install the operators](../start/install.md#install-the-operators), adding the site layer with `-f values/datacenters/dc2.yaml`. Its `global.catalogSources` points the ACM and GitOps Subscriptions at the mirrored catalog.
4. Merge the hub's fleet file, `fleet/hubs/<hubName>.yaml`, listing `datacenters/dc2.yaml` ([Add the hub's fleet file](../start/install.md#add-the-hubs-fleet-file), [Hub file](../reference/fleet.md#hub-file)). The `disconnected` element then enforces the same objects by name, so ACM manages the ones you applied instead of creating copies. If they differ from `clusterResources`, the Policy changes them to match.
5. [Install PolicyStack](../start/install.md#install-policystack) with `--set gitRepo=<your internal Git URL>`, once Argo CD can reach Git ([Before you start](#before-you-start)).

## What the element applies

Every policy is on; turn one off with `policies.<policy>.enabled: false`. With empty input, `resources`, `registry-ca` and `trusted-ca` apply nothing; the Policy stays, Compliant.

| Policy | Applies |
|---|---|
| `resources` | `config.clusterResources`, as is. |
| `registry-ca` | ConfigMap `openshift-config/registry-cas` from `config.registryCAs`, and `additionalTrustedCA` on `image.config.openshift.io/cluster`. |
| `trusted-ca` | ConfigMap `openshift-config/trusted-cas`, with the Proxy's `trustedCA` pointing at it. Lands in every injected trust bundle, including the one the `gitops` element mounts into the repo server of an Argo CD instance it manages (not the hub's own, [above](#git-and-the-hubs-argo-cd)). |
| `operatorhub` | `OperatorHub` `disableAllDefaultSources: true`. |

### trusted-cas

`trusted-cas` = `user-ca-bundle` (the installer's `additionalTrustBundle`) + `config.trustedCA`. The `user-ca-bundle` part is an ACM template that the cluster evaluates on every check, so a later change to `user-ca-bundle` is picked up. `user-ca-bundle` itself is never written.

!!! warning "Existing Proxy trustedCA"
    A Proxy pointing at a ConfigMap other than `user-ca-bundle` is repointed, and its CAs leave the trust bundle. Copy them into `config.trustedCA` first.

??? info "Turning trusted-ca off"
    Turning `trusted-ca` off or emptying `config.trustedCA` leaves the Proxy on a stale `trusted-cas`, still trusted. Clean up by hand:

    ```sh
    oc patch proxy/cluster --type merge -p '{"spec":{"trustedCA":{"name":"user-ca-bundle"}}}' # (1)!
    oc delete configmap trusted-cas -n openshift-config
    ```

    1.  Use `"name":""` if `user-ca-bundle` does not exist.

## Changing the site data

No template sets `pruneObjectBehavior` (ACM's setting that deletes what a policy created once the policy goes, [Removal and pruning](../concepts/removal.md)). So an object dropped from `clusterResources`, a policy turned off, or the element disabled stays on the cluster: an old CatalogSource keeps its pod running and the OperatorHub stays disabled. The two CA ConfigMaps are `mustonlyhave`, so a removed key does disappear, but emptying `registryCAs` or `trustedCA` leaves the last ConfigMap and its reference in place. Delete them by hand.

- **A new oc-mirror run.** Regenerate the list and replace `clusterResources` whole: oc-mirror v2's mirror sets cover the entire image set on every run, and a list in a later layer replaces the earlier one rather than adding to it, so a second layer cannot hold "the new objects".
- **Old signatures.** Each run writes one ConfigMap, `mirrored-release-signatures`, with only that run's releases. Clusters keep keys they already have (`musthave` never removes them), but a new cluster gets only the current run's. Keep `minVersion` low enough to cover every release a cluster may still upgrade to, or copy the earlier run's `binaryData` entries into the new ConfigMap.
- **A new catalog version** (`v4-20` to `v4-21`, when clusters move minor):
    1. Mirror the new index and add its CatalogSource to `clusterResources`, next to the old one.
    2. Once a cluster finishes its upgrade to 4.21, set `global.catalogSources.redhat-operators: cs-redhat-operator-index-v4-21` in `values/clusters/<cluster>.yaml`. The new index may hold bundles that need 4.21. Maps merge, so this one key overrides the site's.
    3. Once every cluster has moved, change the site mapping, drop the cluster overrides and the old CatalogSource, then `oc delete catalogsource cs-redhat-operator-index-v4-20 -n openshift-marketplace` on each cluster.

## Update Service

The `upgrade` element holds a fleet `version` until it appears in the ClusterVersion's `availableUpdates`, which the CVO fills from the upgrade graph ([The upgrade element](upgrades.md#the-upgrade-element)). A disconnected cluster gets no graph unless `stack.upgrade.config.upstream` points at your OSUS ([Site layer](#site-layer)), so every `version` bump waits forever. Hosted clusters take it from the HostedCluster instead ([Hosted clusters](#hosted-clusters)).

OSUS only offers releases in your mirror. `availableUpdates` empty or missing the target means one of:

- The target is outside the mirrored `minVersion`/`maxVersion`.
- oc-mirror did not mirror the cluster's payload architecture ([What to mirror](#what-to-mirror)), so the graph has no entries for it.
- The CVO cannot reach or trust OSUS: `oc get clusterversion version -o jsonpath='{.status.conditions[?(@.type=="RetrievedUpdates")]}'` says why.

## Clusters the hub builds

The credential, the hub Secret named by `install.credential` ([Install keys](../reference/lifecycle.md#install-keys)), must have the mirror registry's auth in its `pullSecret`, for every engine.

Every engine pulls the release image by tag, as `<install.releaseRepository>:<version>-<arch>` (`-x86_64` or `-multi`). An ImageDigestMirrorSet only redirects pulls by digest, so it does not cover this one. Point `releaseRepository` at the repository oc-mirror put the release images in:

```yaml
# values/datacenters/dc2.yaml
install:
  releaseRepository: mirror.dc2.example.com:8443/openshift/release-images
```

`install` in a layer is read only by the `lifecycle-<cluster>` Application of a cluster the hub builds, so imported clusters at the site ignore it. Set it before the hub builds a cluster: on a running `hypershift` cluster a new `releaseRepository` changes the HostedCluster's and NodePools' release image, which rolls the control plane and replaces or updates every node. Each engine also needs the mirrors and the mirror CA for the install itself, set as below.

### Hosted clusters

HyperShift writes a hosted cluster's image config, Proxy, node mirror configuration and, with the default `olmCatalogPlacement: management`, its OperatorHub from the HostedCluster, so policies writing them inside the guest would fight it. Set them on the HostedCluster, through `install.hostedCluster`. On the guest, the `disconnected` element keeps only the CatalogSource. For the same reason, turn off the `remediations` element's `registries` policy on guests and set the allowlist in `install.hostedCluster.spec.configuration.image` (`registrySources`, `allowedRegistriesForImport`).

=== "Site layer"

    ```yaml
    # values/datacenters/dc2.yaml
    install:
      releaseRepository: mirror.dc2.example.com:8443/openshift/release-images
      hostedCluster:
        spec:
          imageContentSources: # (1)!
            - source: quay.io/openshift-release-dev/ocp-release
              mirrors: [mirror.dc2.example.com:8443/openshift/release-images]
            - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
              mirrors: [mirror.dc2.example.com:8443/openshift/release]
            - source: registry.redhat.io/redhat
              mirrors: [mirror.dc2.example.com:8443/redhat]
            # ...one entry per imageDigestMirrors item in oc-mirror's idms-oc-mirror.yaml
          configuration:
            operatorhub:
              disableAllDefaultSources: true # (2)!
          updateService: https://osus.dc2.example.com/api/upgrades_info/v1/graph # (3)!
    ```

    1.  Copy every entry of oc-mirror's ImageDigestMirrorSets (`source`/`mirrors` keep the same shape). Missing the `ocp-v4.0-art-dev` entry fails the install while pulling payload images.
    2.  Replaces the guest's `operatorhub` policy, turned off in the cluster values.
    3.  The hosted CVO runs on the hub, so use an OSUS URL the hub reaches. Without it, a `version` bump with `channel` set is held forever. `hive` and `siteconfig` clusters ignore `install.hostedCluster`.

=== "Cluster values"

    ```yaml
    # values/clusters/hcp-agent.yaml
    install:
      hostedCluster:
        spec:
          additionalTrustBundle:
            name: hcp-agent-mirror-ca # (1)!
      manifests: # (2)!
        - apiVersion: v1
          kind: ConfigMap
          metadata:
            name: hcp-agent-mirror-ca
            namespace: clusters # (3)!
          data:
            ca-bundle.crt: |
              -----BEGIN CERTIFICATE-----
              ...
    stack:
      disconnected:
        policies:
          registry-ca:
            enabled: false
          trusted-ca:
            enabled: false
          operatorhub:
            enabled: false
        config:
          clusterResources: # (4)!
            - apiVersion: operators.coreos.com/v1alpha1
              kind: CatalogSource
              metadata:
                name: cs-redhat-operator-index-v4-20
                namespace: openshift-marketplace
              spec:
                image: mirror.dc2.example.com:8443/redhat/redhat-operator-index:v4.20
                sourceType: grpc
    ```

    1.  A hub ConfigMap with key `ca-bundle.crt`, in the HostedCluster's namespace. Name it per cluster: `install.manifests` are deleted when their cluster is destroyed, so a ConfigMap two clusters share disappears with the first one.
    2.  Hub objects created before the cluster ([Install keys](../reference/lifecycle.md#install-keys)). A list, so it replaces any `manifests` from another layer, and the fleet file's `install` overrides this file's. Keep all of a cluster's manifests in one place.
    3.  `install.namespace`, `clusters` by default.
    4.  Replaces the site's list, CatalogSources only. Restate each in full: without this, the guest inherits the site's mirror sets and signature ConfigMap.

### Hive clusters

The OpenShift installer takes the mirrors and the mirror CA from the install-config. `install.hive.installConfig` is merged into it as is:

```yaml
# values/platforms/vmware.yaml, or the cluster's own file
install:
  hive:
    installConfig:
      imageDigestSources: # (1)!
        - source: quay.io/openshift-release-dev/ocp-release
          mirrors: [mirror.dc2.example.com:8443/openshift/release-images]
        - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
          mirrors: [mirror.dc2.example.com:8443/openshift/release]
        # ...one entry per imageDigestMirrors item in oc-mirror's idms-oc-mirror.yaml
      additionalTrustBundle: |
        -----BEGIN CERTIFICATE-----
        ...
```

1.  Copy every entry of oc-mirror's ImageDigestMirrorSets (`source`/`mirrors` keep the same shape). Missing the `ocp-v4.0-art-dev` entry fails the install while pulling payload images. Once the cluster is built and imported, the `disconnected` element manages its mirror sets like any other cluster's.

!!! warning
    Keep `install.hive` out of layers that other engines read. The lifecycle schema rejects it on a `hypershift` or `siteconfig` cluster, and its `lifecycle-<cluster>` Application shows a ComparisonError: `install.hive: Must have at most 0 properties`.

### Bare metal

`siteconfig` clusters install through the hub's assisted service, the multicluster engine component that builds the discovery ISO each host boots to install ([Cluster lifecycle](../concepts/lifecycle.md#pick-an-engine)). So the hub holds their mirrors. The [`cim`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/cim) element, which configures the assisted service through AgentServiceConfig `agent`, reads the `disconnected` config from the hub's own layers, whether or not `disconnected` is enabled there:

| From | To |
|---|---|
| `ImageDigestMirrorSet`s in `clusterResources` (ImageTagMirrorSets are ignored: the installer pulls by digest) | `registries.conf` in ConfigMap `multicluster-engine/assisted-mirror`, referenced as AgentServiceConfig `mirrorRegistryRef` |
| `registryCAs` | `ca-bundle.crt` in the same ConfigMap |
| `trustedCA`, only when `osImages` is set: the image service needs it only to download your self-hosted ISOs over HTTPS | ConfigMap `image-service-additional-ca`, referenced as `OSImageCACertRef` |

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

1.  Replaces the default list, which points at mirror.openshift.com. List every version the hub's `siteconfig` clusters install, and host the ISOs yourself.
2.  RHCOS version of the ISO.

!!! note "Limits of hub-side mirrors"
    - One mirror set per hub: every `siteconfig` cluster it builds gets the hub's mirrors, even one at another site. The stock SiteConfig templates have no per-cluster mirror.
    - `ibi` (image-based install) does not use the assisted service, so `mirrorRegistryRef` never reaches it. Put an ImageDigestMirrorSet in a ConfigMap through `install.manifests` and list that ConfigMap in `siteconfig.extraManifests`.

!!! warning "Removing entries"
    The AgentServiceConfig is `musthave`, which never removes fields. A removed `osImages` entry, or a `mirrorRegistryRef` once no mirrors remain, stays on it. Remove it with `oc edit agentserviceconfig agent`.
