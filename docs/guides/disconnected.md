# Disconnected clusters

Read [How PolicyStack works](../concepts/overview.md) and [Installation](../start/install.md) first; this page changes those steps for a site with no internet.

A disconnected cluster cannot reach the internet. PolicyStack normally pulls from five places on the internet, and each one is replaced by something you run at the site:

| Normally reached | By | Disconnected replacement | Set up |
|---|---|---|---|
| GitHub | Argo CD on the hub, your repository's CI, and the launcher | Internal Git. policy-library, the chart every element renders through, is in the repo (`lib/`), so Git is all Argo CD reads | By hand ([PolicyStack releases](#policystack-releases)) |
| Container images and operator catalogs | Every cluster | oc-mirror's mirror sets and CatalogSources, applied by the `disconnected` element; `global.catalogSources` points every operator at the mirrored catalog | [Site layer](#site-layer) |
| Public certificate authorities | Every cluster | Your mirror registry's CA and your internal servers' CAs, applied by the `disconnected` element | [Site layer](#site-layer) |
| Red Hat's upgrade graph | Every cluster's CVO | Your OSUS, run by the `update-service` element and set by the `upgrade` element or the HostedCluster | [Update Service](#update-service) |
| Release images and RHCOS ISOs on quay.io and mirror.openshift.com | The hub, for clusters it builds | `install.releaseRepository`, and per-engine mirror settings | [Clusters the hub builds](#clusters-the-hub-builds) |

Terms used on this page:

| Term | Meaning |
|---|---|
| oc-mirror | [v2](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/disconnected_environments/mirroring-in-disconnected-environments) copies release images and operator catalogs into your registry and writes the cluster objects that point clusters at it into `working-dir/cluster-resources/`. You run it; PolicyStack applies its output. |
| CVO | Cluster Version Operator. Runs on every cluster, reads ClusterVersion, fetches the upgrade graph and performs upgrades. |
| OSUS | OpenShift Update Service. Serves a local copy of the upgrade graph. The `update-service` element runs it on one cluster; the `upgrade` element points clusters at it. |
| Site layer | A [values layer](../concepts/values.md) holding one datacenter's mirror data, such as `values/datacenters/dc2.yaml`. Every cluster at the site lists it in its fleet file's `valueFiles`. |
| HostedCluster, standalone | A HostedCluster is the hub object describing a hosted control plane cluster, whose control plane runs as pods on the hub. Any other cluster is standalone. |

Other terms: [Glossary](../concepts/overview.md#glossary).

Order of work: mirror, set up Git, write the site layer, bring up the hub, then add spokes.

## Before you start

### What to mirror

The mirrored operator catalog must hold every package, at the channel, that the elements you enable subscribe to. Otherwise that element's `install` OperatorPolicy stays NonCompliant, with the Subscription reporting that no operator was found in the channel. The validator's `imageset` command writes the whole ImageSetConfiguration: it renders every element the fleet files you name enable, as Argo CD would, and lists every release and operator package they need. Run it from the repository root:

```sh
make -C tools/validator build
tools/validator/bin/policystack-validator imageset > imageset-config.yaml # (1)!
tools/validator/bin/policystack-validator imageset acm-dc2 prod-west-1 prod-west-2 > imageset-config.yaml # (2)!
```

1.  Every fleet file in `fleet/` and `fleet/hubs/`.
2.  Only the clusters this mirror serves, hub included. A name matches `fleet/<name>.yaml` and `fleet/hubs/<name>.yaml`. `--fleet-dir` reads another directory.

Each part of the output comes from the render:

| Output | From |
|---|---|
| `platform.channels` | Each fleet file's `channel`, `stable-<minor>` when unset. `minVersion` and `maxVersion` are the lowest and highest `version` on that channel. A fleet file without `version` adds no release, and with no `version` at all there is no `platform` section. |
| `platform.graph` | `true`, for your OSUS ([Update Service](#update-service)). |
| `operators[].catalog` | The catalog each OperatorPolicy names before `global.catalogSources` renames it, `redhat-operators` becoming `registry.redhat.io/redhat/redhat-operator-index`. The tag is the cluster's OpenShift minor, from `version`, else from `channel`, so clusters on two minors get two catalogs. |
| `packages` | Every OperatorPolicy the clusters render, except `mustnothave` ones, with its channel, which is also written as `defaultChannel`: oc-mirror rejects a catalog whose listed channels leave out the catalog's default. Pinned `versions` and `startingCSV` ([Operator versions](operators.md#pinning)) become the channel's `minVersion` and `maxVersion`, without `maxVersion` while any cluster leaves that operator unpinned on the channel. A subscription with no channel lists none, and oc-mirror takes the head of every channel. |

The command exits 1 and prints only the error when:

- a fleet file is invalid (POLICY050), or sets neither `version` nor `channel`;
- an element fails to render (RENDER000);
- an OperatorPolicy holds an ACM template, or a pin does not end in an `x.y.z` version (`metallb-operator.v4.20.0-202511040000` and `nfd.4.20.0-202511040403` both do);
- a package is subscribed without a channel and either pinned or also subscribed with one.

A package from a catalog OpenShift does not ship is left out and named in a comment at the top: its index image is unknown, so add that catalog by hand.

Three things the render cannot see. Add them before you mirror:

- **Packages no element names.** An operator that installs others, such as ACM installing `multicluster-engine` or ODF's `odf-operator` pulling in `ocs-operator`, `mcg-operator` and more. Take the list from the product's disconnected docs and pass each as `--package <name>[:<channel>]`, repeated. It joins `redhat-operators` at every minor in the output; without a channel, oc-mirror takes the head of every channel:

    ```sh
    tools/validator/bin/policystack-validator imageset \
      --package multicluster-engine:stable-2.12 --package ocs-operator --package mcg-operator \
      acm-dc2 prod-west-1 prod-west-2 > imageset-config.yaml
    ```

- **Architectures.** oc-mirror mirrors `amd64` unless `platform.architectures` says otherwise.
- **The old minor, during a minor upgrade.** The tag follows the fleet file's `version`, the release it targets. Once the bump to 4.21 merges, the output has only `v4.21`, while the clusters still use the `v4.20` catalog until you switch them ([Changing the site data](#changing-the-site-data)). Keep the previous run's `v4.20` block until every cluster has moved.

The output for a hub that enables `acm` and `gitops` and spokes that enable `metallb`, run with the `--package` above but without the ODF packages, after adding `architectures` by hand:

```yaml
# imageset-config.yaml
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  platform:
    channels:
      - name: stable-4.20
        minVersion: 4.20.8 # (1)!
        maxVersion: 4.20.12
    graph: true # (2)!
    architectures: [multi] # (3)!
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: advanced-cluster-management
          defaultChannel: release-2.17
          channels:
            - name: release-2.17
        - name: metallb-operator
          defaultChannel: stable
          channels:
            - name: stable
        - name: multicluster-engine # (4)!
          defaultChannel: stable-2.12
          channels:
            - name: stable-2.12
        - name: openshift-gitops-operator
          defaultChannel: latest
          channels:
            - name: latest
```

1.  The graph needs both ends of an upgrade: the CVO finds no update from a release that is not in it. Before a `version` bump merges, the clusters still run the old release, so lower `minVersion` to the oldest release a cluster runs. A target outside the range is not in your OSUS graph either, so its bump waits at the guard ([Update Service](#update-service)).
2.  Mirrors the graph data your OSUS serves, and makes oc-mirror write an `updateService.yaml` for it, whose two values go in `stack.updateService.config` ([Update Service](#update-service)).
3.  Added by hand. The payload architecture your clusters run (`amd64` in oc-mirror is the `x86_64` release tag). Clusters the hub builds with `hypershift` or `hive` run `multi`, `siteconfig` clusters run `amd64` ([Clusters the hub builds](#clusters-the-hub-builds)). Imported clusters keep the payload they were installed with. List both when you have both.
4.  From `--package`. ACM installs it itself, so no element lists it. Mirror the channel that pairs with your ACM release: `stable-2.12` for ACM 2.17 (ACM release notes).

### PolicyStack releases

Your repository is created and updated from upstream's Git tags, so the site needs a copy of upstream's repository, tags included. policy-library is in it (`lib/`); there is no chart repository to mirror. Your workstation also needs the tools mise installs, the launcher's Python packages and the validator's Go modules. [Installation: Disconnected](../start/install.md#disconnected) carries all of them across in one folder and points the launcher at your mirror.

CI runners in your site need the same tools and Go modules: the same folder and variables on a runner of the same OS and CPU architecture, or your own Go module proxy as `GOPROXY`. The CRDs the validator checks against are committed in `tools/validator/crds/`, so it needs nothing else from the internet ([CRD schemas](../develop/validation.md#crd-schemas)).

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
  $(ls | grep -v -e updateService -e signature-configmap)
```

- `updateService.yaml` is an UpdateService, which needs the OSUS operator, so `resources` goes NonCompliant on any cluster without it. The `update-service` element creates it instead ([Update Service](#update-service)).
- `signature-configmap.json` and `signature-configmap.yaml` are the same release signature ConfigMap. It goes in `releaseSignatures` instead ([Release signatures](#release-signatures)).

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
    - **Release signatures.** Before an upgrade the CVO checks the release image's signature. A disconnected cluster reads it from ConfigMap `openshift-config-managed/mirrored-release-signatures`, which oc-mirror writes when it mirrors releases. Without a signature for the release, the upgrade stops with ClusterVersion `ReleaseAccepted=False` and a message that the release cannot be verified ([Release signatures](#release-signatures)).

### Release signatures

`config.releaseSignatures` becomes ConfigMap `openshift-config-managed/mirrored-release-signatures`, under the name oc-mirror gives it. Its keys are the ConfigMap's `binaryData`, as oc-mirror writes them: `sha256-<release digest>-<n>`, each holding one base64 signature. Merge each oc-mirror run's signatures into the site layer:

```sh
yq -i '.stack.disconnected.config.releaseSignatures *= (load("<dir>/working-dir/cluster-resources/signature-configmap.json").binaryData | ... style="")' \
  values/datacenters/dc2.yaml # (1)!
```

1.  `*=` merges: keys from earlier runs stay, so the layer holds every release a cluster may still upgrade to, while each oc-mirror run writes only its own. `style=""` writes block YAML instead of the JSON's flow style. `yq -i` also drops blank lines and normalizes comment spacing in the file, so review the diff, or keep `releaseSignatures` in a layer file of its own.

```yaml
# values/datacenters/dc2.yaml, after the command
stack:
  disconnected:
    config:
      releaseSignatures:
        sha256-3c8e...b1f2-1: owGbwMvMwMEoOU9... # (1)!
```

1.  Shortened. A key that doesn't match `sha256-<64 hex>-<n>`, or a value that isn't base64, fails the render.

The `signatures` policy owns the ConfigMap with `mustonlyhave` on `binaryData`: delete a key and the signature leaves every cluster, so the layer is the whole list. It doesn't touch the ConfigMap's other labels and annotations. With `releaseSignatures` empty, the default, the policy doesn't render.

!!! warning "Signatures already in clusterResources"
    If `clusterResources` still holds `mirrored-release-signatures` from the earlier command, setting `releaseSignatures` fails the render: two templates would rewrite the same ConfigMap. Move its `binaryData` into `releaseSignatures` and drop it from `clusterResources` in the same commit.

To hold an upgrade until its signature is on the cluster, turn on the upgrade element's [signature check](upgrades.md#release-signature-check) in this layer.

### Check it

On the hub, `oc get policies.policy.open-cluster-management.io -n <cluster>` lists `pstack.resources-disconnected-<cluster>`, `pstack.registry-ca-disconnected-<cluster>`, `pstack.trusted-ca-disconnected-<cluster>`, `pstack.operatorhub-disconnected-<cluster>` and, with `releaseSignatures` set, `pstack.signatures-disconnected-<cluster>`, all Compliant. A NonCompliant one names the object at fault ([Compliance states](../concepts/acm.md#compliance-states)). On the cluster:

```sh
oc get imagedigestmirrorsets,catalogsources -A
oc get packagemanifests -n openshift-marketplace -l catalog=cs-redhat-operator-index-v4-20 # the mirrored packages
oc get mcp # nodes take new mirror sets through a pool update; wait for UPDATED True
```

## The hub

The hub needs the mirrored catalog before PolicyStack exists to apply it, because the bootstrap installs ACM and GitOps from it.

1. Install the hub disconnected, following Red Hat's docs, with the mirror registry's CA in the install-config `additionalTrustBundle`. The steps below apply no CA, so the hub must already trust the mirror.
2. Apply oc-mirror's output by hand, except the UpdateService, which the `update-service` element creates ([Update Service](#update-service)):

    ```sh
    cd working-dir/cluster-resources
    for f in $(ls | grep -v updateService); do oc apply -f "$f"; done
    ```

3. [Install the operators](../start/install.md#install-the-operators), adding the site layer with `-f values/datacenters/dc2.yaml`. Its `global.catalogSources` points the ACM and GitOps Subscriptions at the mirrored catalog.
4. [Install PolicyStack](../start/install.md#install-policystack) with `--set gitRepo=<your internal Git URL>`, once Argo CD can reach Git ([Before you start](#before-you-start)).
5. List `datacenters/dc2.yaml` in the hub's fleet file, `fleet/hubs/<hubName>.yaml` (`hubName` is the name you installed the hub with, [Hub file](../reference/fleet.md#hub-file)). The `disconnected` element then enforces the same objects by name, so ACM manages the ones you applied instead of creating copies. If they differ from `clusterResources`, the Policy changes them to match.

## What the element applies

Every policy is on; turn one off with `policies.<policy>.enabled: false`. With empty input, `resources`, `registry-ca` and `trusted-ca` apply nothing; the Policy stays, Compliant. `signatures` renders only when `releaseSignatures` is set.

| Policy | Applies |
|---|---|
| `resources` | `config.clusterResources`, as is. |
| `registry-ca` | ConfigMap `openshift-config/registry-cas` from `config.registryCAs`, and `additionalTrustedCA` on `image.config.openshift.io/cluster`. With the `update-service` element enabled, also key `updateservice-registry` ([Registry CA](#registry-ca)). |
| `trusted-ca` | ConfigMap `openshift-config/trusted-cas`, with the Proxy's `trustedCA` pointing at it. Lands in every injected trust bundle, including the one the `gitops` element mounts into the repo server of an Argo CD instance it manages (not the hub's own, [above](#git-and-the-hubs-argo-cd)). |
| `signatures` | ConfigMap `openshift-config-managed/mirrored-release-signatures` from `config.releaseSignatures` ([Release signatures](#release-signatures)). |
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

No template sets `pruneObjectBehavior` (ACM's setting that deletes what a policy created once the policy goes, [Removal and pruning](../concepts/removal.md)). So an object dropped from `clusterResources`, a policy turned off, or the element disabled stays on the cluster: an old CatalogSource keeps its pod running and the OperatorHub stays disabled. The two CA ConfigMaps and the signature ConfigMap are `mustonlyhave`, so a removed key does disappear, but emptying `registryCAs`, `trustedCA` or `releaseSignatures` leaves the last ConfigMap in place, and for the two CAs the reference to it. Delete them by hand.

- **A new oc-mirror run.** Regenerate the list and replace `clusterResources` whole: oc-mirror v2's mirror sets cover the entire image set on every run, and a list in a later layer replaces the earlier one rather than adding to it, so a second layer cannot hold "the new objects".
- **Old signatures.** Each run writes `signature-configmap.json` with only that run's releases. The [yq command](#release-signatures) merges it into `releaseSignatures`, so earlier releases stay. Delete a release's keys once no cluster will upgrade to it.
- **A new catalog version** (`v4-20` to `v4-21`, when clusters move minor):
    1. Mirror the new index and add its CatalogSource to `clusterResources`, next to the old one.
    2. Once a cluster finishes its upgrade to 4.21, set `global.catalogSources.redhat-operators: cs-redhat-operator-index-v4-21` in `values/clusters/<cluster>.yaml`. The new index may hold bundles that need 4.21. Maps merge, so this one key overrides the site's.
    3. Once every cluster has moved, change the site mapping, drop the cluster overrides and the old CatalogSource, then `oc delete catalogsource cs-redhat-operator-index-v4-20 -n openshift-marketplace` on each cluster.

## Update Service

The `upgrade` element holds a fleet `version` until it appears in the ClusterVersion's `availableUpdates`, which the CVO fills from the upgrade graph ([The upgrade element](upgrades.md#the-upgrade-element)). A disconnected cluster gets no graph unless `stack.upgrade.config.upstream` points at your OSUS ([Site layer](#site-layer)), so every `version` bump waits forever. Hosted clusters take it from the HostedCluster instead ([Hosted clusters](#hosted-clusters)).

### Run OSUS

The [`update-service`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/update-service) element follows Red Hat's [disconnected update procedure](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/disconnected_environments/updating-a-cluster-in-a-disconnected-environment): it installs the OpenShift Update Service Operator and creates one UpdateService from the two values in oc-mirror's `updateService.yaml`. Enable it on one cluster, usually the hub. One instance serves a whole site; run a second, on another cluster, only when an environment must get different update recommendations.

```yaml
# values/clusters/<hubName>.yaml
stack:
  updateService:
    enabled: true
    config:
      releases: mirror.dc2.example.com:8443/openshift/release-images # (1)!
      graphDataImage: mirror.dc2.example.com:8443/openshift/graph-image:latest # (2)!
```

1.  `spec.releases` from `updateService.yaml`: the repository holding the mirrored release images, with no tag or digest. A tag, a digest or a missing repository fails the render. Red Hat requires the release images in a repository of their own, or OSUS uses excessive memory.
2.  `spec.graphDataImage` from the same file. oc-mirror pushes a new one on every run, and the operator resolves the tag to a digest every 5 minutes, so OSUS picks up a new graph without a values change.

| Policy | Applies | Waits for |
|---|---|---|
| `install-update-service-<cluster>` | Namespace `openshift-update-service`, an OperatorGroup targeting only it, and package `cincinnati-operator`, channel `v1` | |
| `config-update-service-<cluster>` | The namespace's `openshift.io/node-selector: ""` annotation, so a cluster default node selector does not restrict the OSUS pods, then UpdateService `update-service` with 2 replicas | `install` Compliant |
| `ready-update-service-<cluster>` | Inform only. NonCompliant until UpdateService `ReconcileCompleted` is True, Deployment `update-service` is Available and Route `update-service-route` is admitted | `config` Compliant |

`ready` checks all three because the operator sets `ReconcileCompleted` once it has written the Deployment, even when the Route failed, and never checks that the Deployment is Available.

#### Registry CA

The operator reads the release registry's CA from one place: key `updateservice-registry` of the ConfigMap named by `image.config.openshift.io/cluster` `additionalTrustedCA`. With the `disconnected` element on, that ConfigMap is `registry-cas`. While `update-service` is enabled, the `disconnected` element copies the `registryCAs` entry for the `releases` host into that key. Nothing else to set.

!!! warning
    - Enable `disconnected` on the same cluster, with the `releases` host in `registryCAs` keyed as `host..port`. Without it UpdateService `RegistryCACertFound` is False and, for a registry signed by a private CA, the graph builder cannot read the release images.
    - The graph builder trusts that key and nothing else, so the entry must hold the full chain to the registry's root CA.

Disabling `update-service` removes the key again, as `registry-cas` is `mustonlyhave`.

### Point clusters at it

Once `ready-update-service-<hubName>` is Compliant, read the graph URL on that cluster:

```sh
oc get updateservice update-service -n openshift-update-service \
  -o jsonpath='{.status.policyEngineURI}/api/upgrades_info/v1/graph{"\n"}'
curl -sk --header Accept:application/json '<url>?channel=stable-4.20' | jq '.nodes | length' # (1)!
```

1.  The number of releases OSUS offers in that channel. 0 means none of the mirrored releases are in it.

Set the URL as `stack.upgrade.config.upstream` in the site layer ([Site layer](#site-layer)) and as `install.hostedCluster.spec.updateService` for hosted clusters ([Hosted clusters](#hosted-clusters)). The Route serves the OSUS cluster's default ingress certificate. Every standalone cluster whose CVO does not trust it, the OSUS cluster included, needs its CA in `config.trustedCA`.

??? info "Removing OSUS"
    No template prunes, so disabling the element leaves the operator and the UpdateService running. Repoint `stack.upgrade.config.upstream` first, then follow [Stopping and uninstalling](operators.md#stopping-and-uninstalling): Red Hat deletes the UpdateService before the operator.

    ```sh
    oc delete updateservice update-service -n openshift-update-service
    ```

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
          signatures: # (5)!
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
    4.  Replaces the site's list, CatalogSources only. Restate each in full: without this, the guest inherits the site's mirror sets.
    5.  `releaseSignatures` is a map, so the guest inherits the site's. HyperShift upgrades the guest from the HostedCluster, not through the guest's ClusterVersion.

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
