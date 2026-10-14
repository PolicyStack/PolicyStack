# Cluster lifecycle

A fleet file with a `hub` key makes that hub build the cluster, and `state: absent` makes it destroy it. The same file then drives the cluster's Day-2 configuration once ACM imports it ([Fleet files](applicationset.md#fleet-files)).

The hub renders the [`lifecycle/`](https://github.com/PolicyStack/PolicyStack/tree/main/lifecycle) chart once per cluster it builds. The chart renders policy-library Policies placed on the hub's own ManagedCluster, so the hub's governance add-on creates and deletes the cluster's objects on the hub. Nothing outside ACM and Argo CD runs.

## Scope

`install.engine` picks what builds the cluster. Each engine is one component of the chart, with its own converters in `lifecycle/converters/<engine>/` ([Adding an engine](#adding-an-engine)).

| Engine | Builds | Platforms | Upgrades |
|---|---|---|---|
| `hypershift` (default) | One HostedCluster and its NodePools. The control plane runs on the hub that builds the cluster. | `KubeVirt`, `Agent`, `None` | From the hub ([Upgrades](#upgrades)) |
| `hive` | A standalone cluster, installed by a Hive ClusterDeployment in the cloud account of the credential. | `aws`, `gcp`, `azure` | On the cluster, by `openshift-upgrade` ([Upgrades](#upgrades)) |
| `siteconfig` | A standalone cluster on bare metal, installed by a SiteConfig ClusterInstance through the assisted installer (`ai`) or image-based install (`ibi`). | Hosts with a Redfish BMC | On the cluster, by `openshift-upgrade` ([Upgrades](#upgrades)) |

## Prerequisites

On every hub that builds clusters:

- ACM 2.16 or later. On `Agent` and `None` the credentials template generates the etcd encryption key with `randBytes`, which ACM 2.15 and earlier do not provide.
- `hypershift`: the multicluster engine `hypershift` and `hypershift-local-hosting` components. Both are on by default. `hypershift-local-hosting` installs the hypershift-addon in the hub's ManagedCluster namespace, `local-cluster` by default:

    ```sh
    oc get managedclusteraddon hypershift-addon -n local-cluster
    ```

- `hypershift`: HostedCluster auto-import, on by default on OpenShift. Setting `autoImportDisabled` in the `hypershift-addon-deploy-config` AddOnDeploymentConfig turns it off, and a built cluster then gets no Day-2 until imported by hand ([Day-2 handoff](#day-2-handoff)). A hand-made ManagedCluster needs both annotations the guard checks ([Create](#create)), or the guard stops both chains.
- The hub managing itself, with its ManagedCluster labeled `local-cluster` (the default). The lifecycle Placement selects that label, whatever `localClusterName` is.
- `fleet/hubs/<hubName>.yaml` for the hub. The validator rejects a `hub` value with no hub file ([Validation](validation.md)).
- An ACM console credential (Infrastructure > Credentials) holding the pull secret and the SSH public key. The chart reads the keys `pullSecret` and `ssh-publickey`, and for `hive` the cloud credentials. For `KubeVirt` this secret may already exist with the creation of OCP Virt. The labels only make it show in the console:

    ```sh
    oc create secret generic kubevirt-secret -n open-cluster-management \
      --from-file=pullSecret=pull-secret.json \
      --from-file=ssh-publickey=id_ed25519.pub
    oc label secret kubevirt-secret -n open-cluster-management \
      cluster.open-cluster-management.io/credentials= \
      cluster.open-cluster-management.io/type=kubevirt
    ```

    Use `type=hostinventory` for an Agent credential. A `baseDomain` key in the credential is ignored. Set `install.baseDomain` instead.

- `hive`: a console credential of type `aws`, `gcp` or `azr` (Azure). Hive reads `aws_access_key_id` and `aws_secret_access_key`, `osServiceAccount.json`, or `osServicePrincipal.json`. `platforms/aws.yaml` expects Secret `aws` in namespace `hive-credentials`. Create it in the console, or with `oc` as above in that namespace, with those keys and the label `cluster.open-cluster-management.io/type` set to its type. A region, `projectID` or `baseDomainResourceGroupName` in the credential is ignored. Set them in `install.hive.platform`.
- `hive`: a public DNS zone for `install.baseDomain` in the cloud account, and quota for the cluster. Hive is on by default in the multicluster engine.
- `siteconfig`: the SiteConfig component, off by default. The `advancedClusterManagement` element turns it on with `config.siteConfig: true`. Without it, `present-cluster-<cluster>` is NonCompliant on the missing ClusterInstance kind.
- `siteconfig`: central infrastructure management. That means an AgentServiceConfig named `agent`, and a Provisioning `provisioning-configuration` with `watchAllNamespaces: true`, plus `provisioningNetwork: Disabled` on a hub that has none. The hub needs the OpenShift `baremetal` capability. On an AWS hub, the image service needs an NLB IngressController ([Enabling central infrastructure management](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/)). The default AgentServiceConfig OS images cover OpenShift 4.19 to 4.22; list older ones in `osImages`.
- `siteconfig`: a console credential with `pullSecret` and `ssh-publickey`, type `hostinventory`, and a hub Secret with the BMC's `username` and `password`. `platforms/baremetal.yaml` expects `bm-credentials/hostinventory` and `bm-credentials/bmc`. Each host's BMC must be reachable from the hub, and each host must reach the hub's assisted image service and API by name. DNS must resolve `api.<cluster>.<baseDomain>`, and for `HighlyAvailable` also `api-int` and `*.apps`, from the hosts and from the hub. Without the hub's record, ACM never imports the cluster and Day-2 never starts.
- `siteconfig` with `ibi`: the multicluster engine `image-based-install-operator` component, set on the MultiClusterEngine, not the MultiClusterHub. Each host must be preinstalled with a seed image out of band, and `version` must be the seed's version.
- `hypershift`: a LoadBalancer implementation on the hub, such as MetalLB. The default publishes each cluster's API server through a `LoadBalancer` Service on the hub ([install keys](#install-keys)).
- `KubeVirt`: OpenShift Virtualization on the hub.
- `Agent`: an InfraEnv in `install.agentNamespace`, and approved Agents labeled to match each NodePool's `agentLabelSelector`.

## Fleet keys

| Key | Default | Effect |
|---|---|---|
| `hub` | none | `hubName` of the hub that builds and owns the cluster ([The hub](applicationset.md#the-hub)). Without it the cluster is import-only. |
| `state` | `present` | `present` builds and reconciles the cluster. `absent` destroys it ([Destroy](#destroy)). Requires `hub`. |
| `version` | none | OpenShift release, `x.y.z`. Required. Bumping it upgrades the cluster ([Upgrades](#upgrades)). |
| `channel` | none | Upgrade channel, such as `stable-4.20`. Optional ([Upgrades](#upgrades)). |
| `install` | none | What to build. Requires `hub`. Any values layer can set the same keys. The fleet file wins ([Values](#values)). |

The cluster name is the fleet file name. It must be a DNS label of at most 38 characters ([Naming limits](policies.md#naming-limits)).

```yaml
# fleet/hcp-kubevirt.yaml
revision: main
hub: acm-dc1
version: 4.20.8
valueFiles:
  - environments/nonprod.yaml
  - platforms/kubevirt.yaml
```

### `install` keys

Defaults are from [`lifecycle/values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/lifecycle/values.yaml). `values.schema.json` rejects unknown keys.

| Key | Default | Notes |
|---|---|---|
| `engine` | `hypershift` | The engine that builds the cluster ([Scope](#scope)). Immutable. |
| `platform` | required for `hypershift` | `KubeVirt`, `Agent` or `None`. Immutable. |
| `releaseRepository` | `quay.io/openshift-release-dev/ocp-release` | The release image is `<releaseRepository>:<version>-multi`. Point it at a mirror when disconnected. |
| `namespace` | `clusters` | `hypershift`: namespace of the HostedCluster, its NodePools and the copied secrets. The control plane runs in `<namespace>-<cluster>`. `hive` and `siteconfig` use namespace `<cluster>`. |
| `credential.namespace`, `credential.name` | required | The console credential. |
| `baseDomain` | `""` | Required for `hive` and `siteconfig`, and on `Agent` and `None`. On `KubeVirt`, HyperShift detects it when empty. Immutable. |
| `agentNamespace` | `""` | `hypershift`: namespace of the InfraEnv and its Agents. Required on `Agent`. |
| `services` | `APIServer`: `LoadBalancer`; `OAuthServer`, `Konnectivity`, `Ignition`: `Route` | Map of service name to its `servicePublishingStrategy`. `null` drops an entry. HyperShift requires these four. Immutable. |
| `hostedCluster` | `{}` | `metadata` and `spec` merged over the generated HostedCluster. `spec.release` is ignored. |
| `nodePools` | `{}` | Map of pool name to a NodePool `spec` merged over the generated one. The NodePool is `<cluster>-<pool>`. |
| `manifests` | `[]` | Extra hub objects, each with `apiVersion`, `kind` and `metadata`. Created before the cluster, deleted after it. |
| `hive.platform` | required for `hive` | Exactly one of `aws`, `gcp` or `azure`: the install-config `platform`. `region` is required, plus `projectID` on GCP and `baseDomainResourceGroupName` on Azure. `region`, `baseDomainResourceGroupName` and `cloudName` also go to the ClusterDeployment. Immutable. |
| `hive.installConfig` | `{}` | Merged under the generated install-config: `controlPlane`, `compute`, `networking` and the rest of the [install-config](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/installing_on_aws/). The chart sets `apiVersion`, `metadata.name`, `baseDomain` and `platform`. `sshKey` defaults to the credential's `ssh-publickey`. Hive drops fields its installer version does not know. |
| `hive.clusterDeployment` | `{}` | `metadata` and `spec` merged over the generated ClusterDeployment. `spec.provisioning.releaseImage` is ignored. |
| `siteconfig.templates` | `ai` | `ai` or `ibi`: the stock `<templates>-cluster-templates-v1` and `<templates>-node-templates-v1` ConfigMaps. `ibi` installs single-node OpenShift only. Immutable. |
| `siteconfig.templateNamespace` | `open-cluster-management` | Namespace of those ConfigMaps, the MultiClusterHub's. |
| `siteconfig.bmcCredential.namespace`, `.name` | required for `siteconfig` | Hub Secret with `username` and `password`, copied to `<cluster>-bmc` for every node. |
| `siteconfig.clusterInstance` | `{}` | Merged under the generated [ClusterInstance](https://github.com/stolostron/siteconfig/blob/release-2.16/api/v1alpha1/clusterinstance_types.go) spec: `clusterType` (`SNO` or `HighlyAvailable`), `networkType`, `machineNetwork`, `apiVIPs`, `ingressVIPs` and the rest. |
| `siteconfig.nodes` | required for `siteconfig` | Map of hostName to its ClusterInstance node: `bmcAddress` and `bootMACAddress` required, plus `role`, `nodeNetwork`, `rootDeviceHints` and the rest. Being a map, layers merge per host. |

`hypershift`, `hive` and `siteconfig` keys are each read only by their engine, and the schema rejects `hive` or `siteconfig` keys unless `engine` selects them.

The generated HostedCluster sets the release image, `pullSecret` `<cluster>-pull-secret`, `sshKey` `<cluster>-ssh-key`, the platform, `services`, and `dns.baseDomain` when set. On `Agent` it adds `platform.agent.agentNamespace`. On `Agent` and `None` it adds `secretEncryption` with the key `<cluster>-etcd-encryption-key`. Networking, etcd and everything else are left to HyperShift's defaults: set them in `hostedCluster.spec`.

Each NodePool gets `clusterName`, the release image ([Upgrades](#upgrades) covers when it changes), the platform, and `management.upgradeType`: `Replace` on `KubeVirt`, `InPlace` otherwise. A pool with no `replicas` gets HyperShift's default of 0. On `Agent`, a pool without an `agentLabelSelector` claims any free Agent in the namespace, which is why `platforms/agent.yaml` ships no default pool.

The generated ClusterDeployment `<cluster>/<cluster>` sets `baseDomain`, `clusterName`, the platform with `credentialsSecretRef` `<cluster>-creds`, `pullSecretRef` `<cluster>-pull-secret`, `provisioning.installConfigSecretRef` `<cluster>-install-config`, the release image `<releaseRepository>:<version>-multi`, and `installAttemptsLimit: 1`, so a failed install is not retried. The install-config carries no `pullSecret`: Hive fills it in. Set `architecture: amd64` on `controlPlane` and each `compute` pool, as `platforms/aws.yaml` does. Otherwise it follows the architecture of the hub node that runs the install.

The generated ClusterInstance `<cluster>/<cluster>` sets `clusterName`, `baseDomain`, `pullSecretRef` `<cluster>-pull-secret`, `sshPublicKey` from the credential at create, the templates, `clusterImageSetNameRef` `lifecycle-<cluster>`, and on each node `hostName`, `bmcCredentialsName` `<cluster>-bmc` and the node templates. ClusterImageSet `lifecycle-<cluster>` holds `<releaseRepository>:<version>-multi`. `bootMode` defaults to `UEFI` and `automatedCleaningMode` to `disabled`. SiteConfig rejects any spec change from create until the install completes, so nothing in the spec follows a later `version` or credential change: the image set name is fixed, and its image and the SSH key are read back from the live objects ([Immutable fields](#immutable-fields)). The stock `ai` KlusterletAddonConfig leaves application-manager off, without which ACM never creates the cluster's Argo CD secret, so the chart suppresses it and renders its own with every add-on on. With `-multi`, the assisted installer accepts at most 3 control-plane nodes.

### Values

The lifecycle Application reads the same cascade as the cluster's Day-2 Applications, with paths relative to `lifecycle/` ([Order](values.md#order)). Lowest precedence first:

1. `lifecycle/values.yaml`
2. `values.yaml`
3. Each `valueFiles` entry, in list order
4. `values/clusters/<cluster>.yaml`
5. `hub`, `state`, `version`, `channel` and `install` from the fleet file, passed in `valuesObject`

Shared defaults go in the layers: the platform and credential in `values/platforms/kubevirt.yaml` or `agent.yaml`, `baseDomain` in a datacenter file, per-cluster services and manifests in `values/clusters/<cluster>.yaml`. The fleet file then holds little more than `version`, which no layer may set. For `hcp-agent` ([fixture](https://github.com/PolicyStack/PolicyStack/blob/main/tools/validator/testdata/clusters/hcp-agent.yaml)):

| Key | Set by |
|---|---|
| `platform`, `agentNamespace`, `credential` | `values/platforms/agent.yaml` |
| `baseDomain` | `values/datacenters/dc1.yaml` |
| `services`, `hostedCluster`, `manifests` | `values/clusters/hcp-agent.yaml` |
| `nodePools` | The fleet file |

For `hive-aws`, `values/platforms/aws.yaml` sets `engine`, `credential` and `hive.installConfig`, and the fleet file sets `baseDomain` and `hive.platform.aws.region`.

Maps merge across layers, so `services` and `nodePools` entries combine. `manifests` is a list and replaces ([Merge rules](values.md#merge-rules)). A shared default is part of every cluster that reads it: changing `services` or `baseDomain` in a platform or datacenter file turns every existing cluster built from it NonCompliant ([Immutable fields](#immutable-fields)).

## Create

Merge `fleet/<cluster>.yaml` with `hub` to the default branch, together with `values/clusters/<cluster>.yaml` if the cluster has one. Within about 6 minutes the hub's ApplicationSet `policystack-lifecycle` creates the Application `lifecycle-<cluster>` ([Generators](applicationset.md#lifecycle-generator)), which syncs the Policy `present-lifecycle-<cluster>`, with Placement and PlacementBinding `lifecycle-<cluster>`, into `policy`. Its templates run in order, each waiting for the one before it to be Compliant:

| Template | Mode | Does |
|---|---|---|
| `present-guard-<cluster>` | inform | NonCompliant if a ManagedCluster of that name exists without the engine's owner annotations ([Ownership](#ownership)). That stops the chain ([Troubleshooting](#troubleshooting)). |
| `present-credentials-<cluster>` | enforce | `hypershift`: Namespace `install.namespace`; `<cluster>-pull-secret` and `<cluster>-ssh-key` copied from the credential; on `Agent` and `None`, `<cluster>-etcd-encryption-key`. `hive`: Namespace `<cluster>`; `<cluster>-pull-secret`, `<cluster>-creds` with only the keys Hive reads, and `<cluster>-install-config`. `siteconfig`: Namespace `<cluster>`, `<cluster>-pull-secret` and `<cluster>-bmc`. |
| `present-manifests-<cluster>` | enforce | `install.manifests`, as given. |
| `present-cluster-<cluster>` | enforce | `hypershift`: the HostedCluster, then one NodePool per `nodePools` entry. `hive`: the ClusterDeployment, then ManagedCluster `<cluster>` with the owner annotation, and its KlusterletAddonConfig with every add-on on. `siteconfig`: ClusterImageSet `lifecycle-<cluster>`, the ClusterInstance, and its KlusterletAddonConfig with every add-on on. SiteConfig renders the BareMetalHosts, the install objects and the ManagedCluster. |
| `present-ready-<cluster>` | inform | `hypershift`: NonCompliant until the HostedCluster's version history shows `version` Completed and every NodePool reports `version`. `hive`: NonCompliant until the ClusterDeployment's `spec.installed` is true. `siteconfig`: NonCompliant until the ClusterInstance's `Provisioned` condition is True. The Policy, and with it the Application's health, then means the cluster is built, and for `hypershift` upgraded, not just applied. A `hive` or `siteconfig` cluster's upgrades show on the cluster ([Upgrades](#upgrades)). |

### Ownership

The guard and detach act only on a ManagedCluster that carries its engine's owner annotations:

| Engine | Annotations | Set by |
|---|---|---|
| `hypershift` | `open-cluster-management/created-via: hypershift`, `import.open-cluster-management.io/hosting-cluster-name: <hub ManagedCluster>` | The hypershift-addon, on a cluster hosted on this hub |
| `hive` | `lifecycle.policystack.io/hub: <hubName>` | `present-cluster-<cluster>` |
| `siteconfig` | `lifecycle.policystack.io/hub: <hubName>` | SiteConfig, from the ClusterInstance's `extraAnnotations` |

The credential is read with managed-cluster templates resolved on the hub, so it can live in any namespace. The etcd key is generated once, while neither the key nor the HostedCluster exists. A key deleted after create is not regenerated, since a new key cannot decrypt etcd: restore it from a backup.

Every template is `musthave`. Fields the operator fills in and the chart leaves out are not compared, so the policy does not fight HyperShift or Hive.

Watch it:

```sh
oc get policy -n policy present-lifecycle-hcp-kubevirt
oc get configurationpolicy -n local-cluster | grep hcp-kubevirt
oc get hostedcluster,nodepool -n clusters
oc get hostedcluster hcp-kubevirt -n clusters \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.message}{"\n"}{end}'
```

For `siteconfig`, `oc get clusterinstance,bmh,agentclusterinstall,agent -n <cluster>` shows progress; an SNO install takes about an hour. For `hive`, `oc get clusterdeployment -n <cluster>` and the install pod's logs, `oc logs -f -n <cluster> -l hive.openshift.io/install=true -c installer`, show progress. An AWS install takes about 45 minutes.

The ConfigurationPolicies live in the hub's ManagedCluster namespace, `local-cluster` by default. A template that has not run yet is Pending. The cluster is done when `present-lifecycle-<cluster>` is Compliant. A render error, such as a missing `version`, shows on the Application instead:

```sh
oc get applications.argoproj.io lifecycle-hcp-kubevirt -n openshift-gitops \
  -o jsonpath='{.status.conditions}'
```

## Day-2 handoff

No step is needed:

1. `hypershift`: once the HostedCluster is available, the hypershift-addon creates ManagedCluster `<cluster>` and its KlusterletAddonConfig. `hive`: once Hive sets `spec.installed`, ACM imports the ManagedCluster the chart created. `siteconfig`: the same, with the ManagedCluster SiteConfig rendered.
2. The ManagedCluster joins `global`. The GitOpsCluster creates its Argo CD cluster secret, and ACM adds the `clusterID` label once the cluster reports ([Import clusters into Argo CD](install.md#import-clusters-into-argo-cd)).
3. The Day-2 ApplicationSet pairs the secret with the same fleet file and creates `<element>-<cluster>` Applications. It ignores `hub`, `state` and `install`.

`values/clusters/<cluster>.yaml` serves both: `install` for the lifecycle chart, `stack` for the elements. Labels in `hostedCluster.metadata.labels` are copied to the ManagedCluster when the addon creates it and are not synced after.

## Upgrades

Bump `version` in the fleet file ([Upgrades](upgrades.md) covers every cluster). Without `channel`, the HostedCluster takes the new release image at once. With `channel`, the chart sets `spec.channel` and holds the current image until HyperShift lists `version` in the HostedCluster's `status.version.availableUpdates`, so a version off the channel's update graph waits instead of being forced. Change `channel` in the same commit as a minor bump: HyperShift lists the new channel's updates once the HostedCluster carries it. While held, `present-ready-<cluster>` stays NonCompliant.

Each existing NodePool keeps its current image until the HostedCluster's version history shows the new version `Completed`, then moves to it: HyperShift only reports version skew, it does not hold a NodePool back. A NodePool added during an upgrade starts on the new image. `KubeVirt` pools replace their VMs, other platforms upgrade nodes in place.

The chart owns the HostedCluster's and every NodePool's `release`; a `release` key in `hostedCluster.spec` or a `nodePools` entry is ignored. A minor upgrade that HyperShift blocks because `ClusterVersionUpgradeable` is False needs the annotation `hypershift.openshift.io/force-upgrade-to: <release image>` in `hostedCluster.metadata.annotations`. A version that is not an available update needs `channel` removed as well.

`present-lifecycle-<cluster>` is Compliant again once the control plane and every NodePool report the new version.

A `hive` or `siteconfig` cluster upgrades on the cluster, through `openshift-upgrade`, like an imported one. Hive rejects changes to the ClusterDeployment's release image, and the assisted installer to the image set, so the chart keeps the image it installed with. It installs from the `-multi` payload, so the cluster is a multi-architecture cluster and its updates are too.

## Immutable fields

HyperShift, Hive and SiteConfig reject changes to these, among others:

- HostedCluster: `services`, `platform`, `networking`, `etcd`, `issuerURL`, `controllerAvailabilityPolicy`, `fips`, and `dns.baseDomain` once set. `secretEncryption` cannot be removed.
- NodePool: `clusterName`, `platform.type`, `arch`, `management.upgradeType`.
- ClusterInstance: everything from create until the install completes, then everything but `extraAnnotations`, `extraLabels`, `suppressedManifests`, `pruneManifests`, and adding a node. `musthave` keeps list entries it does not render, so removing a host from `siteconfig.nodes`, or changing one after install, also needs `oc edit clusterinstance <cluster> -n <cluster>`. A failed install cannot be fixed in place: set `state: absent`, then `present` again with the fix.
- ClusterDeployment: everything but `installAttemptsLimit`, `powerState`, `preserveOnDelete` and a few others. `baseDomain` and `hive.platform` included. `hive.installConfig` and `hive.platform` changes reach the Secret but not the installed cluster.

Editing one in the cascade does not change the cluster. The update is rejected and `present-cluster-<cluster>` goes NonCompliant with the API server's message. Revert the value. To change a NodePool's `upgradeType` or `arch`, add a new pool and remove the old one.

## Destroy

Set `state: absent` in the fleet file:

```diff
 # fleet/hcp-kubevirt.yaml
 revision: main
 hub: acm-dc1
+state: absent
```

The Application replaces `present-lifecycle-<cluster>` with `absent-lifecycle-<cluster>`. Deleting the present Policy deletes nothing, since no lifecycle template sets `pruneObjectBehavior`. The absent chain, each step waiting for the one before it:

| Template | Mode | Does |
|---|---|---|
| `absent-guard-<cluster>` | inform | Same check as on create, so a ManagedCluster this hub did not build shows NonCompliant. |
| `absent-detach-<cluster>` | enforce `mustnothave` | Deletes the ManagedCluster, while the cluster is still up to clean up its klusterlet. It checks the same annotations in the same evaluation and leaves any other ManagedCluster alone, including one imported under the same name after teardown. |
| `absent-detached-<cluster>` | inform `mustnothave` | Waits until the ManagedCluster is gone. An enforced delete reports Compliant as soon as the API accepts it, so the next step gates on this. |
| `absent-destroy-<cluster>` | enforce `mustnothave` | Deletes the HostedCluster. HyperShift deletes the NodePools, then the control plane namespace `<namespace>-<cluster>`. |
| `absent-destroyed-<cluster>` | inform `mustnothave` | Waits until the HostedCluster is gone. |
| `absent-cleanup-<cluster>` | enforce | Deletes `<cluster>-pull-secret`, `<cluster>-ssh-key`, `<cluster>-etcd-encryption-key` and every `install.manifests` object. Leaves `install.namespace` and the credential. |

`hive` runs the same chain, with these steps:

| Template | Does |
|---|---|
| `absent-destroy-<cluster>` | Deletes the ClusterDeployment. Hive deprovisions the cloud resources with `<cluster>-creds`, usually within 20 minutes. |
| `absent-destroyed-<cluster>` | Waits until the ClusterDeployment is gone, which is when deprovision finished. Hive needs `<cluster>-creds` and `<cluster>-install-config` until then. |
| `absent-cleanup-<cluster>` | Deletes Namespace `<cluster>` and every `install.manifests` object. ACM usually deletes the namespace first. Leaves the credential. |

`siteconfig` has no detach: the ClusterInstance owns the ManagedCluster, and SiteConfig would re-create one deleted on its own. Its chain is guard, then:

| Template | Does |
|---|---|
| `absent-destroy-<cluster>` | Deletes the ClusterInstance. SiteConfig deletes what it rendered, highest sync wave first: with `ai` the BareMetalHosts, then the ManagedCluster, then the install objects. With `ai` the hosts keep running the cluster: the assisted installer detached their BareMetalHosts, so deleting them powers nothing off. With `ibi` the hosts reboot and likely power off. Disks are not wiped either way; power off or reuse the hosts by hand. |
| `absent-destroyed-<cluster>` | Waits until the ClusterInstance is gone, a few minutes at least. |
| `absent-cleanup-<cluster>` | Deletes Namespace `<cluster>`, ClusterImageSet `lifecycle-<cluster>` and every `install.manifests` object. Leaves the credentials. |

If the hypershift-addon restarts between detach and destroy, it can re-create the ManagedCluster. `absent-detach` deletes it again.

Detaching deletes the cluster's Argo CD secret, so the Day-2 ApplicationSet deletes its `<element>-<cluster>` Applications and their Policies ([Removing an element](rollout.md#removing-an-element)). The governance add-on is gone by then, so nothing is pruned on the cluster. On `hypershift`, ACM deletes the namespaces `<cluster>` and `klusterlet-<cluster>`. On `hive`, ACM deletes Namespace `<cluster>` once the ClusterDeployment is gone.

Wait until the Policy is Compliant, then delete the fleet file and `values/clusters/<cluster>.yaml`:

```sh
oc get policy -n policy absent-lifecycle-hcp-kubevirt
```

Deleting the file earlier abandons the cluster at whichever step it reached ([Abandon](#abandon)).

## Abandon

Deleting the fleet file, or removing its `hub` key, deletes the Application `lifecycle-<cluster>`. The lifecycle ApplicationSet sets `preserveResourcesOnDeletion: false`, so Argo CD deletes the Policies with it. The cluster, its secrets and manifests stay and keep running, unmanaged: the HostedCluster and NodePools; the ClusterDeployment, ManagedCluster and KlusterletAddonConfig; or the ClusterImageSet, ClusterInstance and everything SiteConfig rendered from it. Deleting the ApplicationSets does the same for every cluster the hub built. `helm uninstall appset` keeps them ([Install the ApplicationSet](install.md#install-the-applicationset)).

To re-adopt, restore the file or the key with the same `install` values. The guard passes, since the ManagedCluster carries the owner annotations, the existing etcd key and the ClusterDeployment and ClusterImageSet release images are kept, and `musthave` reconciles the existing objects.

Moving a cluster to another hub is unsupported. Changing `hub` abandons the cluster on the old hub and builds a second one, with the same name, on the new hub. Destroy it on the old hub, then build it on the new one.

## Removing a NodePool

`musthave` never deletes, so removing an entry from `nodePools` only stops managing the pool. Delete the entry, then the NodePool:

```sh
oc delete nodepool hcp-kubevirt-workers -n clusters
```

Setting a pool to `null` does not remove it: the schema rejects it. A pool defined in a shared layer, such as `workers` in `platforms/kubevirt.yaml`, cannot be removed from a higher layer. Scale it to `replicas: 0` or move it out of the shared layer.

## Troubleshooting

**Guard refusal.** `present-guard-<cluster>` or `absent-guard-<cluster>` is NonCompliant with "A ManagedCluster with this name was not built by this hub". A cluster of that name was imported, or its control plane runs on another hub, and the chart refuses to build over it or detach it. Usually `hub` was added to the fleet file of an imported cluster. Remove `hub`, `state` and `install` from that file.

**Hive install failed.** `present-ready-<cluster>` stays NonCompliant and the ClusterDeployment shows `ProvisionStopped`. `installAttemptsLimit: 1` stops Hive from retrying. Read the install pod's logs, fix the values, then destroy (`state: absent`) and build again. A region that differs between the install-config and the ClusterDeployment shows as `RequirementsMet` False with `InstallConfigValidationFailed`. The chart builds both from `hive.platform`, so this happens through `hive.clusterDeployment`, or an edit of `hive.platform` before the install finished. Revert it.

**Hive deprovision stuck.** `absent-destroyed-<cluster>` stays NonCompliant and the ClusterDeployment shows `DeprovisionLaunchError`, or its ClusterDeprovision `AuthenticationFailure`. The cloud credentials are wrong or were rotated. Fix the keys in `<cluster>-creds` itself: the absent Policy no longer copies the credential. A HiveConfig with `deleteProtection: enabled` rejects the delete outright: `absent-destroy-<cluster>` is NonCompliant with `cannot delete while annotation is present`. Remove the `hive.openshift.io/protected-delete` annotation from the ClusterDeployment.

**SiteConfig install failed or spec rejected.** `present-cluster-<cluster>` is NonCompliant with a webhook denial, or `present-ready-<cluster>` stays NonCompliant and the ClusterInstance shows `Provisioned` False. A spec change before the install completes, and any change after it fails, is rejected. Check `oc get clusterinstance <cluster> -n <cluster> -o jsonpath='{.status.conditions}'` and the AgentClusterInstall. Destroy, fix the values, and build again. A wrong ref, such as `templateNamespace`, can also be fixed by creating the missing object under the referenced name.

**SiteConfig deletion paused.** `absent-destroyed-<cluster>` stays NonCompliant and the ClusterInstance carries the `clusterinstance.siteconfig.open-cluster-management.io/paused` annotation. A rendered object, usually a BareMetalHost or the ManagedCluster, was not gone within 30 minutes. Fix what holds it, then remove the annotation.

**Rebuilding a name.** Nothing can be created in Namespace `<cluster>` while it is still terminating after a destroy, and the first template that writes there stays NonCompliant until it is gone: `present-cluster-<cluster>` on `hypershift`, where ACM refuses the ManagedCluster, and `present-credentials-<cluster>` on `hive` and `siteconfig`, with `because it is being terminated`. Wait for `oc get ns <cluster>` to return NotFound.

**Missing credential.** An unset `credential.namespace` or `credential.name` fails the render, and the Application shows `install.credential.namespace is required`. A credential Secret that does not exist on the hub turns `present-credentials-<cluster>` NonCompliant with a template error, and nothing after it runs. Without a `pullSecret` key, `.dockerconfigjson` is empty, the API server rejects the Secret, and `present-credentials-<cluster>` stays NonCompliant. Without an `ssh-publickey` key no error shows: `hypershift` copies an empty key, and `hive` leaves `sshKey` out. On `hive`, a missing cloud key is copied into `<cluster>-creds` as an empty value, so `present-credentials-<cluster>` stays Compliant and the install fails on the ClusterDeployment instead.

**ManagedCluster stuck Terminating.** `absent-detached-<cluster>` stays NonCompliant. Hosted-mode cleanup waits for the guest, and ACM skips that cleanup once the guest has been unreachable for 5 minutes. Check what holds it:

```sh
oc get managedcluster hcp-kubevirt -o jsonpath='{.metadata.finalizers}'
oc get klusterlet klusterlet-hcp-kubevirt -o jsonpath='{.metadata.finalizers}'
```

If the Klusterlet waits on `operator.open-cluster-management.io/klusterlet-hosted-cleanup` and the guest will not answer, remove that finalizer.

**AgentMachine pre-terminate hook.** On `Agent`, `absent-destroyed-<cluster>` stays NonCompliant and Machines in `<namespace>-<cluster>` stay `Deleting` with `PreTerminateDeleteHookSucceeded` reporting `WaitingExternalHook`. The Agent CAPI provider failed to remove its hook after detaching the Agents. Remove it by hand:

```sh
for m in $(oc get machines.cluster.x-k8s.io -n clusters-hcp-agent -o name); do
  oc annotate "$m" -n clusters-hcp-agent pre-terminate.delete.hook.machine.cluster.x-k8s.io/agentmachine-
done
```

**Agents stuck reclaiming.** After a destroy, Agents go to `reclaiming` while the assisted service reboots them into discovery. An Agent that cannot start that ends in `UnbindingPendingUserAction`: boot it from the discovery ISO by hand. Agents that stay `reclaiming` with the `agent.agent-install.openshift.io/ai-deprovision` finalizer block any new cluster that selects them. Do not rebuild a cluster with the same name until its Agents are free:

```sh
oc get agents -n hardware-inventory
```

## Adding an engine

An engine `<e>` is a component of [`lifecycle/`](https://github.com/PolicyStack/PolicyStack/tree/main/lifecycle). `templates/policy.yaml` renders the component that `install.engine` names, so the ApplicationSet, `hub`, `state`, `version` and the shared converters stay as they are. To add one:

1. Put its converters in `converters/<e>/`: the objects to create, the root object by identity alone for destroy, and its credentials. Freeze every field the engine rejects changes to by reading it back with `lookup`, so a later `version` bump never touches it. End its cleanup converter with `include "lifecycle.cleanupManifests"`.
2. Add `stack.<e>` to `values.yaml`, with two Policies, `present` and `absent`, and no Policy-level `remediationAction` or `pruneObjectBehavior`. Each chain starts with an inform guard. `present` ends with an inform ready gate. `absent` ends with destroy (enforce `mustnothave`), destroyed (inform `mustnothave`, since an enforced delete is Compliant as soon as it is accepted), then cleanup. The shared `guard`, `detach`, `managedcluster` and `manifests` converters serve every engine whose ManagedCluster carries the owner annotations.
3. Add `<e>` to the `install.engine` enum in `values.schema.json`, with its keys. POLICY090 fails when the enum and the `stack` components differ ([Validation](validation.md)).
4. Add a present and an absent fleet file to `tools/validator/testdata/clusters/`, and a column to this page's tables.

An engine whose clusters upgrade from the hub maps `version` onto its objects, as `hypershift` does. One that only installs, such as a Hive ClusterDeployment, uses `version` at create and leaves upgrades to `openshift-upgrade` on the cluster ([Upgrades](upgrades.md)).
