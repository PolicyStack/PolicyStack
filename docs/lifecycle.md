# Cluster lifecycle

A fleet file with a `hub` key makes that hub build the cluster, and `state: absent` makes it destroy it. The same file then drives the cluster's Day-2 configuration once ACM imports it ([Fleet files](applicationset.md#fleet-files)).

The hub renders the [`lifecycle/`](https://github.com/PolicyStack/PolicyStack/tree/main/lifecycle) chart once per cluster it builds. The chart renders policy-library Policies placed on the hub's own ManagedCluster, so the hub's governance add-on creates and deletes the cluster's objects on the hub. Nothing outside ACM and Argo CD runs.

## Scope

Hosted control planes only: one HostedCluster and its NodePools, on the `KubeVirt`, `Agent` or `None` platform. The control plane runs on the hub that builds the cluster. Hive, SiteConfig and other engines are not supported ([Other engines](#other-engines)).

## Prerequisites

On every hub that builds clusters:

- ACM 2.16 or later. On `Agent` and `None` the credentials template generates the etcd encryption key with `randBytes`, which ACM 2.15 and earlier do not provide.
- The multicluster engine `hypershift` and `hypershift-local-hosting` components. Both are on by default. `hypershift-local-hosting` installs the hypershift-addon in the hub's ManagedCluster namespace, `local-cluster` by default:

    ```sh
    oc get managedclusteraddon hypershift-addon -n local-cluster
    ```

- HostedCluster auto-import, on by default on OpenShift. Setting `autoImportDisabled` in the `hypershift-addon-deploy-config` AddOnDeploymentConfig turns it off, and a built cluster then gets no Day-2 until imported by hand ([Day-2 handoff](#day-2-handoff)). A hand-made ManagedCluster needs both annotations the guard checks ([Create](#create)), or the guard stops both chains.
- The hub managing itself, with its ManagedCluster labeled `local-cluster` (the default). The lifecycle Placement selects that label, whatever `localClusterName` is.
- `fleet/hubs/<hubName>.yaml` for the hub. The validator rejects a `hub` value with no hub file ([Validation](validation.md)).
- An ACM console credential (Infrastructure > Credentials) holding the pull secret and the SSH public key. The chart reads the keys `pullSecret` and `ssh-publickey`. This secret may already exist with the creation of OCP Virt. The labels only make it show in the console:

    ```sh
    oc create secret generic kubevirt -n open-cluster-management \
      --from-file=pullSecret=pull-secret.json \
      --from-file=ssh-publickey=id_ed25519.pub
    oc label secret kubevirt-secret -n open-cluster-management \
      cluster.open-cluster-management.io/credentials= \
      cluster.open-cluster-management.io/type=kubevirt
    ```

    Use `type=hostinventory` for an Agent credential. A `baseDomain` key in the credential is ignored. Set `install.baseDomain` instead.

- A LoadBalancer implementation on the hub, such as MetalLB. The default publishes each cluster's API server through a `LoadBalancer` Service on the hub ([install keys](#install-keys)).
- `KubeVirt`: OpenShift Virtualization on the hub.
- `Agent`: an InfraEnv in `install.agentNamespace`, and approved Agents labeled to match each NodePool's `agentLabelSelector`.

## Fleet keys

| Key | Default | Effect |
|---|---|---|
| `hub` | none | `hubName` of the hub that builds and owns the cluster ([The hub](applicationset.md#the-hub)). Without it the cluster is import-only. |
| `state` | `present` | `present` builds and reconciles the cluster. `absent` destroys it ([Destroy](#destroy)). Requires `hub`. |
| `install` | none | What to build. Requires `hub`. Any values layer can set the same keys. The fleet file wins ([Values](#values)). |

The cluster name is the fleet file name. It must be a DNS label of at most 38 characters ([Naming limits](policies.md#naming-limits)).

```yaml
# fleet/hcp-kubevirt.yaml
revision: main
hub: acm-dc1
valueFiles:
  - environments/nonprod.yaml
  - platforms/kubevirt.yaml
install:
  version: 4.20.8
```

### `install` keys

Defaults are from [`lifecycle/values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/lifecycle/values.yaml). `values.schema.json` rejects unknown keys.

| Key | Default | Notes |
|---|---|---|
| `platform` | required | `KubeVirt`, `Agent` or `None`. Immutable. |
| `version` | required | `x.y.z`. Bumping it upgrades the cluster ([Upgrades](#upgrades)). |
| `releaseRepository` | `quay.io/openshift-release-dev/ocp-release` | The release image is `<releaseRepository>:<version>-multi`. Point it at a mirror when disconnected. |
| `namespace` | `clusters` | Namespace of the HostedCluster, its NodePools and the copied secrets. The control plane runs in `<namespace>-<cluster>`. |
| `credential.namespace`, `credential.name` | required | The console credential. |
| `baseDomain` | `""` | Required on `Agent` and `None`. On `KubeVirt`, HyperShift detects it when empty. |
| `agentNamespace` | `""` | Namespace of the InfraEnv and its Agents. Required on `Agent`. |
| `services` | `APIServer`: `LoadBalancer`; `OAuthServer`, `Konnectivity`, `Ignition`: `Route` | Map of service name to its `servicePublishingStrategy`. `null` drops an entry. HyperShift requires these four. Immutable. |
| `hostedCluster` | `{}` | `metadata` and `spec` merged over the generated HostedCluster. |
| `nodePools` | `{}` | Map of pool name to a NodePool `spec` merged over the generated one. The NodePool is `<cluster>-<pool>`. |
| `manifests` | `[]` | Extra hub objects, each with `apiVersion`, `kind` and `metadata`. Created before the HostedCluster, deleted after it. |

The generated HostedCluster sets the release image, `pullSecret` `<cluster>-pull-secret`, `sshKey` `<cluster>-ssh-key`, the platform, `services`, and `dns.baseDomain` when set. On `Agent` it adds `platform.agent.agentNamespace`. On `Agent` and `None` it adds `secretEncryption` with the key `<cluster>-etcd-encryption-key`. Networking, etcd and everything else are left to HyperShift's defaults: set them in `hostedCluster.spec`.

Each NodePool gets `clusterName`, the release image ([Upgrades](#upgrades) covers when it changes), the platform, and `management.upgradeType`: `Replace` on `KubeVirt`, `InPlace` otherwise. A pool with no `replicas` gets HyperShift's default of 0. On `Agent`, a pool without an `agentLabelSelector` claims any free Agent in the namespace, which is why `platforms/agent.yaml` ships no default pool.

### Values

The lifecycle Application reads the same cascade as the cluster's Day-2 Applications, with paths relative to `lifecycle/` ([Order](values.md#order)). Lowest precedence first:

1. `lifecycle/values.yaml`
2. `values.yaml`
3. Each `valueFiles` entry, in list order
4. `values/clusters/<cluster>.yaml`
5. `hub`, `state` and `install` from the fleet file, passed in `valuesObject`

Shared defaults go in the layers: the platform and credential in `values/platforms/kubevirt.yaml` or `agent.yaml`, `baseDomain` in a datacenter file, per-cluster services and manifests in `values/clusters/<cluster>.yaml`. The fleet file then holds little more than `version`. For `hcp-agent` ([fixture](https://github.com/PolicyStack/PolicyStack/blob/main/tools/validator/testdata/clusters/hcp-agent.yaml)):

| Key | Set by |
|---|---|
| `platform`, `agentNamespace`, `credential` | `values/platforms/agent.yaml` |
| `baseDomain` | `values/datacenters/dc1.yaml` |
| `services`, `hostedCluster`, `manifests` | `values/clusters/hcp-agent.yaml` |
| `version`, `nodePools` | The fleet file |

Maps merge across layers, so `services` and `nodePools` entries combine. `manifests` is a list and replaces ([Merge rules](values.md#merge-rules)). A shared default is part of every cluster that reads it: changing `services` or `baseDomain` in a platform or datacenter file turns every existing cluster built from it NonCompliant ([Immutable fields](#immutable-fields)).

## Create

Merge `fleet/<cluster>.yaml` with `hub` to the default branch, together with `values/clusters/<cluster>.yaml` if the cluster has one. Within about 6 minutes the hub's ApplicationSet `policystack-lifecycle` creates the Application `lifecycle-<cluster>` ([Generators](applicationset.md#lifecycle-generator)), which syncs the Policy `present-lifecycle-<cluster>`, with Placement and PlacementBinding `lifecycle-<cluster>`, into `policy`. Its templates run in order, each waiting for the one before it to be Compliant:

| Template | Mode | Does |
|---|---|---|
| `present-guard-<cluster>` | inform | NonCompliant if a ManagedCluster of that name exists without the annotations the hypershift-addon sets on a cluster hosted on this hub, `open-cluster-management/created-via: hypershift` and `import.open-cluster-management.io/hosting-cluster-name: <hub ManagedCluster>`. That stops the chain ([Troubleshooting](#troubleshooting)). |
| `present-credentials-<cluster>` | enforce | Namespace `install.namespace`; `<cluster>-pull-secret` and `<cluster>-ssh-key` copied from the credential; on `Agent` and `None`, `<cluster>-etcd-encryption-key`. |
| `present-manifests-<cluster>` | enforce | `install.manifests`, as given. |
| `present-cluster-<cluster>` | enforce | The HostedCluster, then one NodePool per `nodePools` entry. |

The credential is read with managed-cluster templates resolved on the hub, so it can live in any namespace. The etcd key is generated once, while neither the key nor the HostedCluster exists. A key deleted after create is not regenerated, since a new key cannot decrypt etcd: restore it from a backup.

Every template is `musthave`. Fields the operator fills in and the chart leaves out are not compared, so the policy does not fight HyperShift.

Watch it:

```sh
oc get policy -n policy present-lifecycle-hcp-kubevirt
oc get configurationpolicy -n local-cluster | grep hcp-kubevirt
oc get hostedcluster,nodepool -n clusters
oc get hostedcluster hcp-kubevirt -n clusters \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.message}{"\n"}{end}'
```

The ConfigurationPolicies live in the hub's ManagedCluster namespace, `local-cluster` by default. A template that has not run yet is Pending. The HostedCluster is done when `Available` is True. A render error, such as a missing `version`, shows on the Application instead:

```sh
oc get applications.argoproj.io lifecycle-hcp-kubevirt -n openshift-gitops \
  -o jsonpath='{.status.conditions}'
```

## Day-2 handoff

No step is needed:

1. Once the HostedCluster is available, the hypershift-addon creates ManagedCluster `<cluster>` and its KlusterletAddonConfig.
2. The ManagedCluster joins `global`. The GitOpsCluster creates its Argo CD cluster secret, and ACM adds the `clusterID` label once the cluster reports ([Import clusters into Argo CD](install.md#import-clusters-into-argo-cd)).
3. The Day-2 ApplicationSet pairs the secret with the same fleet file and creates `<element>-<cluster>` Applications. It ignores `hub`, `state` and `install`.

`values/clusters/<cluster>.yaml` serves both: `install` for the lifecycle chart, `stack` for the elements. Labels in `hostedCluster.metadata.labels` are copied to the ManagedCluster when the addon creates it and are not synced after.

## Upgrades

Bump `install.version`. The HostedCluster takes the new release image at once. Each existing NodePool keeps its current image until the HostedCluster's version history shows the new version `Completed`, then moves to it: HyperShift only reports version skew, it does not hold a NodePool back. A NodePool added during an upgrade starts on the new image. `KubeVirt` pools replace their VMs, other platforms upgrade nodes in place.

The chart owns every NodePool's `release`; a `release` key in a `nodePools` entry is ignored. A minor upgrade that HyperShift blocks because `ClusterVersionUpgradeable` is False needs the annotation `hypershift.openshift.io/force-upgrade-to: <release image>` in `hostedCluster.metadata.annotations`.

## Immutable fields

HyperShift rejects changes to these after create, among others:

- HostedCluster: `services`, `platform`, `networking`, `etcd`, `issuerURL`, `controllerAvailabilityPolicy`, `fips`, and `dns.baseDomain` once set. `secretEncryption` cannot be removed.
- NodePool: `clusterName`, `platform.type`, `arch`, `management.upgradeType`.

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
| `absent-detach-<cluster>` | enforce `mustnothave` | Deletes the ManagedCluster, while the control plane is still up to clean up the hosted klusterlet. It checks the same annotations in the same evaluation and leaves any other ManagedCluster alone, including one imported under the same name after teardown. |
| `absent-detached-<cluster>` | inform `mustnothave` | Waits until the ManagedCluster is gone. An enforced delete reports Compliant as soon as the API accepts it, so the next step gates on this. |
| `absent-destroy-<cluster>` | enforce `mustnothave` | Deletes the HostedCluster. HyperShift deletes the NodePools, then the control plane namespace `<namespace>-<cluster>`. |
| `absent-destroyed-<cluster>` | inform `mustnothave` | Waits until the HostedCluster is gone. |
| `absent-cleanup-<cluster>` | enforce | Deletes `<cluster>-pull-secret`, `<cluster>-ssh-key`, `<cluster>-etcd-encryption-key` and every `install.manifests` object. Leaves `install.namespace` and the credential. |

If the hypershift-addon restarts between detach and destroy, it can re-create the ManagedCluster. `absent-detach` deletes it again.

Detaching deletes the cluster's Argo CD secret, so the Day-2 ApplicationSet deletes its `<element>-<cluster>` Applications and their Policies stay in `policy`, selecting nothing ([Removing an element](rollout.md#removing-an-element)). ACM deletes the namespaces `<cluster>` and `klusterlet-<cluster>`.

Wait until the Policy is Compliant, then delete the fleet file and `values/clusters/<cluster>.yaml`:

```sh
oc get policy -n policy absent-lifecycle-hcp-kubevirt
```

Deleting the file earlier abandons the cluster at whichever step it reached ([Abandon](#abandon)).

## Abandon

Deleting the fleet file, or removing its `hub` key, deletes the Application `lifecycle-<cluster>`. The lifecycle ApplicationSet sets `preserveResourcesOnDeletion: false`, so Argo CD deletes the Policies with it. The HostedCluster, NodePools, secrets and manifests stay and keep running, unmanaged. `helm uninstall appset` does the same for every cluster the hub built ([Install the ApplicationSet](install.md#install-the-applicationset)).

To re-adopt, restore the file or the key with the same `install` values. The guard passes, since HyperShift created the ManagedCluster, the existing etcd key is kept, and `musthave` reconciles the existing objects.

Moving a cluster to another hub is unsupported. Changing `hub` abandons the cluster on the old hub and builds a second one, with the same name, on the new hub. Destroy it on the old hub, then build it on the new one.

## Removing a NodePool

`musthave` never deletes, so removing an entry from `nodePools` only stops managing the pool. Delete the entry, then the NodePool:

```sh
oc delete nodepool hcp-kubevirt-workers -n clusters
```

Setting a pool to `null` does not remove it: the schema rejects it. A pool defined in a shared layer, such as `workers` in `platforms/kubevirt.yaml`, cannot be removed from a higher layer. Scale it to `replicas: 0` or move it out of the shared layer.

## Troubleshooting

**Guard refusal.** `present-guard-<cluster>` or `absent-guard-<cluster>` is NonCompliant with "A ManagedCluster with this name was not built by this hub". A cluster of that name was imported, or its control plane runs on another hub, and the chart refuses to build over it or detach it. Usually `hub` was added to the fleet file of an imported cluster. Remove `hub`, `state` and `install` from that file.

**Missing credential.** An unset `credential.namespace` or `credential.name` fails the render, and the Application shows `install.credential.namespace is required`. A credential Secret that does not exist on the hub turns `present-credentials-<cluster>` NonCompliant with a template error, and nothing after it runs. Without a `pullSecret` key, `.dockerconfigjson` is empty, the API server rejects the Secret, and `present-credentials-<cluster>` stays NonCompliant. Without an `ssh-publickey` key, the SSH key is copied as an empty value and no error shows.

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

## Other engines

v1 builds HostedClusters only, and the schema rejects unknown `install` keys. Another engine, such as a Hive ClusterDeployment or a SiteConfig ClusterInstance, would add an `install` key selecting it, converters for its objects, and its own absent chain with inform gates. `hub` and `state`, the ApplicationSet and the guard stay as they are.
