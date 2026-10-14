# Lifecycle reference

Details behind [Cluster lifecycle](lifecycle.md).

## Install keys

Defaults are in [`lifecycle/values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/lifecycle/values.yaml). `values.schema.json` rejects unknown keys, and rejects `hive` or `siteconfig` keys unless `engine` selects them.

Shared:

| Key | Default | Notes |
|---|---|---|
| `engine` | `hypershift` | `hypershift`, `hive` or `siteconfig`. Fixed at create. |
| `credential.namespace`, `credential.name` | required | Hub Secret with `pullSecret`, `ssh-publickey`, and for `hive` the cloud keys. It can live in any namespace. |
| `baseDomain` | `""` | Required except on `KubeVirt`, where HyperShift uses the hub's ingress domain. Fixed at create. |
| `releaseRepository` | `quay.io/openshift-release-dev/ocp-release` | The release image is `<releaseRepository>:<version>-multi`. Point it at a mirror when disconnected. |
| `manifests` | `[]` | Extra hub objects, each with `apiVersion`, `kind` and `metadata`. Created before the cluster, deleted after it. |

`hypershift`:

| Key | Default | Notes |
|---|---|---|
| `platform` | required | `KubeVirt`, `Agent` or `None`. Fixed at create. |
| `namespace` | `clusters` | Namespace of the HostedCluster, its NodePools and the copied secrets. The control plane runs in `<namespace>-<cluster>`. |
| `agentNamespace` | `""` | Namespace of the InfraEnv and its Agents. Required on `Agent`. |
| `services` | `APIServer`: `LoadBalancer`; `OAuthServer`, `Konnectivity`, `Ignition`: `Route` | Service name to `servicePublishingStrategy`. HyperShift requires all four. Fixed at create. |
| `hostedCluster` | `{}` | `metadata` and `spec` merged over the generated HostedCluster. `spec.release` is ignored. |
| `nodePools` | `{}` | Pool name to a NodePool `spec` merged over the generated one. The NodePool is `<cluster>-<pool>`. |

`hive`:

| Key | Default | Notes |
|---|---|---|
| `hive.platform` | required | Exactly one of `aws`, `gcp` or `azure`. `region` is required, plus `projectID` on GCP and `baseDomainResourceGroupName` on Azure. Fixed at create. |
| `hive.installConfig` | `{}` | Merged under the generated install-config. Hive drops fields its installer version does not know. |
| `hive.clusterDeployment` | `{}` | `metadata` and `spec` merged over the generated ClusterDeployment. `spec.provisioning.releaseImage` is ignored. |

`siteconfig`:

| Key | Default | Notes |
|---|---|---|
| `siteconfig.templates` | `ai` | `ai` (assisted installer) or `ibi` (image-based install, single node). Fixed at create. |
| `siteconfig.templateNamespace` | `open-cluster-management` | Namespace of the stock `<templates>-cluster-templates-v1` and `<templates>-node-templates-v1` ConfigMaps. |
| `siteconfig.bmcCredential.namespace`, `.name` | required | Hub Secret with `username` and `password`, copied to `<cluster>-bmc` for every node. |
| `siteconfig.clusterInstance` | `{}` | Merged under the generated ClusterInstance spec. |
| `siteconfig.nodes` | required | hostName to its ClusterInstance node. `bmcAddress` and `bootMACAddress` are required. Being a map, layers merge per host. |

## Generated objects

**HostedCluster** `<namespace>/<cluster>`: the release image, `pullSecret` `<cluster>-pull-secret`, `sshKey` `<cluster>-ssh-key`, the platform, `services`, `channel` when set, and `dns.baseDomain` when set. `Agent` adds `platform.agent.agentNamespace`. `Agent` and `None` add `secretEncryption` with the generated key `<cluster>-etcd-encryption-key`. Networking, etcd and everything else are HyperShift's defaults; set them in `hostedCluster.spec`.

**NodePool** `<namespace>/<cluster>-<pool>`: `clusterName`, the release image, the platform, and `management.upgradeType`, `Replace` on `KubeVirt` and `InPlace` otherwise. A pool without `replicas` gets 0.

**ClusterDeployment** `<cluster>/<cluster>`: `baseDomain`, `clusterName`, the platform with `credentialsSecretRef` `<cluster>-creds`, `pullSecretRef` `<cluster>-pull-secret`, `installConfigSecretRef` `<cluster>-install-config`, the release image, and `installAttemptsLimit: 1`, so a failed install is not retried. `<cluster>-creds` holds only the keys Hive reads: `aws_access_key_id` and `aws_secret_access_key`, `osServiceAccount.json`, or `osServicePrincipal.json`. Then ManagedCluster `<cluster>` and its KlusterletAddonConfig with every add-on on.

**ClusterInstance** `<cluster>/<cluster>`: `clusterName`, `baseDomain`, `pullSecretRef`, `sshPublicKey`, the templates, `clusterImageSetNameRef` `lifecycle-<cluster>`, and per node `hostName`, `bmcCredentialsName` `<cluster>-bmc` and the node templates. `bootMode` defaults to `UEFI`, `automatedCleaningMode` to `disabled`. ClusterImageSet `lifecycle-<cluster>` holds the release image. The stock `ai` KlusterletAddonConfig turns application-manager off, which stops ACM from creating the cluster's Argo CD secret, so the chart suppresses it and renders its own with every add-on on. With `-multi`, the assisted installer accepts at most 3 control-plane nodes.

Hive and SiteConfig reject changes to the release image, and SiteConfig any spec change until the install completes. The chart reads those fields back from the live objects, so a later `version` bump or credential rotation never touches them.

## Policy chains

`present-lifecycle-<cluster>` runs these templates in order, each waiting for the one before it to be Compliant:

| Template | Mode | `hypershift` | `hive` | `siteconfig` |
|---|---|---|---|---|
| `present-guard-<cluster>` | inform | NonCompliant if a ManagedCluster with this name exists without the engine's owner annotations ([Ownership](#ownership)). Stops the chain. | same | same |
| `present-credentials-<cluster>` | enforce | Namespace `install.namespace`, `<cluster>-pull-secret`, `<cluster>-ssh-key`; on `Agent` and `None` the etcd key | Namespace `<cluster>`, `<cluster>-pull-secret`, `<cluster>-creds`, `<cluster>-install-config` | Namespace `<cluster>`, `<cluster>-pull-secret`, `<cluster>-bmc` |
| `present-manifests-<cluster>` | enforce | `install.manifests` | same | same |
| `present-cluster-<cluster>` | enforce | HostedCluster, then the NodePools | ClusterDeployment, ManagedCluster, KlusterletAddonConfig | ClusterImageSet, ClusterInstance, KlusterletAddonConfig |
| `present-ready-<cluster>` | inform | The version history shows `version` Completed and every NodePool reports it | `spec.installed` is true | `Provisioned` is True |

`absent-lifecycle-<cluster>`:

| Template | Mode | `hypershift` | `hive` | `siteconfig` |
|---|---|---|---|---|
| `absent-guard-<cluster>` | inform | Same check as on create | same | same |
| `absent-detach-<cluster>` | enforce `mustnothave` | Deletes the ManagedCluster while the cluster is up, so its klusterlet is cleaned up | same | none: SiteConfig would re-create it |
| `absent-detached-<cluster>` | inform `mustnothave` | Waits until the ManagedCluster is gone | same | none |
| `absent-destroy-<cluster>` | enforce `mustnothave` | Deletes the HostedCluster | Deletes the ClusterDeployment; Hive deprovisions with `<cluster>-creds` | Deletes the ClusterInstance; SiteConfig deletes what it rendered |
| `absent-destroyed-<cluster>` | inform `mustnothave` | Waits until the HostedCluster is gone | Waits until deprovision finished | Waits until the ClusterInstance is gone |
| `absent-cleanup-<cluster>` | enforce | Deletes the copied secrets, the etcd key and `install.manifests` | Deletes Namespace `<cluster>` and `install.manifests` | Deletes Namespace `<cluster>`, the ClusterImageSet and `install.manifests` |

The credential is never deleted. An enforced delete is Compliant as soon as the API accepts it, so each destroy step is followed by an inform wait. Every template is `musthave`: fields the operator fills in are not compared, so the Policy does not fight HyperShift, Hive or SiteConfig. Deleting the present Policy deletes nothing, since no lifecycle template sets `pruneObjectBehavior`.

The etcd key is generated once, while neither it nor the HostedCluster exists. A deleted key is not regenerated, since a new key cannot decrypt etcd: restore it from a backup.

## Day-2 handoff

1. `hypershift`: once the HostedCluster is available, the hypershift-addon creates ManagedCluster `<cluster>`. `hive` and `siteconfig`: once installed, ACM imports the ManagedCluster the chart or SiteConfig created.
2. The ManagedCluster joins `global`, the GitOpsCluster creates its Argo CD cluster secret, and ACM adds the `clusterID` label ([Import clusters into Argo CD](install.md#import-clusters-into-argo-cd)).
3. The Day-2 ApplicationSet pairs the secret with the same fleet file and creates the `<element>-<cluster>` Applications. It ignores `hub`, `state` and `install`.

HyperShift auto-import is on by default. Setting `autoImportDisabled` in the `hypershift-addon-deploy-config` AddOnDeploymentConfig leaves a built cluster without Day-2 until it is imported by hand. Labels in `hostedCluster.metadata.labels` are copied to the ManagedCluster once, when it is created.

## Ownership

The guard and detach act only on a ManagedCluster with its engine's owner annotations:

| Engine | Annotations | Set by |
|---|---|---|
| `hypershift` | `open-cluster-management/created-via: hypershift`, `import.open-cluster-management.io/hosting-cluster-name: <hub ManagedCluster>` | The hypershift-addon |
| `hive` | `lifecycle.policystack.io/hub: <hubName>` | `present-cluster-<cluster>` |
| `siteconfig` | `lifecycle.policystack.io/hub: <hubName>` | SiteConfig, from the ClusterInstance's `extraAnnotations` |

A hand-made ManagedCluster needs these annotations, or the guard stops both chains.

## Immutable fields

The operators reject changes to these, among others:

- HostedCluster: `services`, `platform`, `networking`, `etcd`, `issuerURL`, `controllerAvailabilityPolicy`, `fips`, and `dns.baseDomain` once set. `secretEncryption` cannot be removed.
- NodePool: `clusterName`, `platform.type`, `arch`, `management.upgradeType`.
- ClusterDeployment: everything but `installAttemptsLimit`, `powerState`, `preserveOnDelete` and a few others. `hive.installConfig` changes reach the Secret, not the installed cluster.
- ClusterInstance: everything until the install completes, then everything but `extraAnnotations`, `extraLabels`, `suppressedManifests`, `pruneManifests`, and adding a node.

Changing one in the cascade does not change the cluster: `present-cluster-<cluster>` goes NonCompliant with the API server's message. Revert the value. To change a NodePool's `upgradeType` or `arch`, add a new pool and remove the old one. A failed SiteConfig install cannot be fixed in place: set `state: absent`, then build again with the fix.

## Removing a NodePool or host

`musthave` never deletes, so removing an entry only stops managing it:

- NodePool: remove the `nodePools` entry, then `oc delete nodepool <cluster>-<pool> -n clusters`. A pool from a shared layer, such as `workers` in `platforms/kubevirt.yaml`, cannot be removed from a higher layer: scale it to `replicas: 0`. The schema rejects `null`.
- SiteConfig host: remove the `nodes` entry, then `oc edit clusterinstance <cluster> -n <cluster>`. Changing a host after install needs the same edit.

## Troubleshooting

**Guard refusal.** A guard is NonCompliant with "A ManagedCluster with this name was not built by this hub". The cluster was imported, or runs on another hub. Usually `hub` was added to an imported cluster's fleet file: remove `hub`, `state` and `install`. The guard cannot catch an engine switch before the cluster is imported, so destroy before changing `engine`.

**Render error.** A missing `version`, credential or required key shows on the Application, not the Policy: `oc get applications.argoproj.io lifecycle-<cluster> -n openshift-gitops -o jsonpath='{.status.conditions}'`.

**Missing credential.** A Secret that does not exist turns `present-credentials-<cluster>` NonCompliant with a template error. Without `pullSecret` the API server rejects the pull secret. Without `ssh-publickey` no error shows: the cluster gets no SSH key. On `hive`, a missing cloud key is copied as an empty value and the install fails on the ClusterDeployment.

**Hive install failed.** `present-ready-<cluster>` stays NonCompliant and the ClusterDeployment shows `ProvisionStopped`. Read the installer log, fix the values, destroy and build again. `RequirementsMet` False with `InstallConfigValidationFailed` means the install-config and ClusterDeployment regions differ, through `hive.clusterDeployment` or an edit of `hive.platform` during the install. Revert it.

**Hive deprovision stuck.** `absent-destroyed-<cluster>` stays NonCompliant and the ClusterDeployment shows `DeprovisionLaunchError`, or its ClusterDeprovision `AuthenticationFailure`. Fix the keys in `<cluster>-creds` itself; the absent Policy no longer copies the credential. With HiveConfig `deleteProtection: enabled`, remove the `hive.openshift.io/protected-delete` annotation from the ClusterDeployment.

**SiteConfig rejected or failed.** `present-cluster-<cluster>` shows a webhook denial, or the ClusterInstance shows `Provisioned` False. Check `oc get clusterinstance <cluster> -n <cluster> -o jsonpath='{.status.conditions}'` and the AgentClusterInstall. Destroy, fix, build again. A wrong ref, such as `templateNamespace`, can also be fixed by creating the missing object.

**SiteConfig cluster never imported.** The hub cannot resolve `api.<cluster>.<baseDomain>`. Add the record where the hub resolves it.

**SiteConfig deletion paused.** The ClusterInstance carries `clusterinstance.siteconfig.open-cluster-management.io/paused`: a rendered object, usually a BareMetalHost or the ManagedCluster, was not gone within 30 minutes. Fix what holds it, then remove the annotation.

**Rebuilding a name.** Nothing can be created in Namespace `<cluster>` while it terminates after a destroy, so the first step that writes there stays NonCompliant with `because it is being terminated`. Wait for `oc get ns <cluster>` to return NotFound.

**ManagedCluster stuck Terminating.** `absent-detached-<cluster>` stays NonCompliant. Hosted-mode cleanup waits for the guest; ACM skips it once the guest has been unreachable for 5 minutes. Check `oc get klusterlet klusterlet-<cluster> -o jsonpath='{.metadata.finalizers}'`. If it waits on `operator.open-cluster-management.io/klusterlet-hosted-cleanup` and the guest will not answer, remove that finalizer.

**AgentMachine pre-terminate hook.** On `Agent`, Machines in `<namespace>-<cluster>` stay `Deleting` with `PreTerminateDeleteHookSucceeded` reporting `WaitingExternalHook`. Remove the hook:

```sh
for m in $(oc get machines.cluster.x-k8s.io -n clusters-<cluster> -o name); do
  oc annotate "$m" -n clusters-<cluster> pre-terminate.delete.hook.machine.cluster.x-k8s.io/agentmachine-
done
```

**Agents stuck reclaiming.** After a destroy, Agents go to `reclaiming` while they reboot into discovery. One that cannot ends in `UnbindingPendingUserAction`: boot it from the discovery ISO by hand. Agents still holding the `agent.agent-install.openshift.io/ai-deprovision` finalizer block any new cluster that selects them.

## Adding an engine

An engine `<e>` is a component of [`lifecycle/`](https://github.com/PolicyStack/PolicyStack/tree/main/lifecycle). `templates/policy.yaml` renders the component `install.engine` names, so the ApplicationSet and the fleet keys stay as they are.

1. Add `converters/<e>/`: the objects to create, the root object by identity alone for destroy, and its credentials. Read back with `lookup` every field the engine rejects changes to. End the cleanup converter with `include "lifecycle.cleanupManifests"`.
2. Add `stack.<e>` to `values.yaml` with a `present` and an `absent` Policy, following the [chains](#policy-chains), with no Policy-level `remediationAction` or `pruneObjectBehavior`.
3. Add `<e>` to the `install.engine` enum in `values.schema.json`, with its keys. POLICY090 fails when the enum and the `stack` components differ ([Validation](validation.md)).
4. Add an owner case to `lifecycle.ownerAnnotations`, a present and an absent fleet file to `tools/validator/testdata/clusters/`, and an example to [Cluster lifecycle](lifecycle.md).

An engine that upgrades from the hub maps `version` onto its objects, as `hypershift` does. One that only installs uses `version` at create and leaves upgrades to `openshift-upgrade` ([Upgrades](upgrades.md)).
