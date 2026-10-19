# Lifecycle reference

Details behind [Cluster lifecycle](lifecycle.md). Chart: [`lifecycle/`](https://github.com/PolicyStack/PolicyStack/tree/main/lifecycle).

## Install keys

Defaults are in [`lifecycle/values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/lifecycle/values.yaml). The schema rejects unknown keys, and rejects `hive` or `siteconfig` keys unless `engine` selects them.

| Key | Default | Notes |
|---|---|---|
| `engine` | `hypershift` | `hypershift`, `hive` or `siteconfig`. Fixed at create. |
| `credential.namespace`, `.name` | required | Hub Secret with `pullSecret`, `ssh-publickey`, and for `hive` the cloud keys. Any namespace. |
| `baseDomain` | `""` | Required except on `KubeVirt`, which uses the hub's ingress domain. Fixed at create. |
| `releaseRepository` | `quay.io/openshift-release-dev/ocp-release` | Image is `<releaseRepository>:<version>-multi`. Point at a mirror when disconnected. |
| `manifests` | `[]` | Extra hub objects with `apiVersion`, `kind`, `metadata`. Created before the cluster, deleted after it. |

=== "hypershift"

    | Key | Default | Notes |
    |---|---|---|
    | `platform` | required | `KubeVirt`, `Agent` or `None`. Fixed at create. |
    | `namespace` | `clusters` | HostedCluster, NodePools and copied secrets. Control plane runs in `<namespace>-<cluster>`. |
    | `agentNamespace` | `""` | InfraEnv and Agents. Required on `Agent`. |
    | `services` | `APIServer: LoadBalancer`, others `Route` | Service to `servicePublishingStrategy`. HyperShift needs all four. Fixed at create. |
    | `hostedCluster` | `{}` | `metadata` and `spec` merged over the generated HostedCluster. `spec.release` is ignored. |
    | `nodePools` | `{}` | Pool to NodePool `spec`, merged over the generated one. Named `<cluster>-<pool>`. |

    `hcp-agent` after the cascade merges `platforms/agent.yaml`, [`values/clusters/hcp-agent.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values/clusters/hcp-agent.yaml) and its fleet file, trimmed:

    ```yaml
    install:
      platform: Agent                    # (1)!
      agentNamespace: hardware-inventory
      credential:
        namespace: hcp-credentials
        name: hostinventory
      services:
        APIServer:
          type: LoadBalancer
          loadBalancer:
            hostname: api-int-hcp-agent.dc1.example.com
        OAuthServer:
          type: Route                    # (2)!
        Konnectivity:
          type: Route
        Ignition:
          type: Route
      hostedCluster:
        spec:
          kubeAPIServerDNSName: api-hcp-agent.dc1.example.com
      nodePools:
        compute:                         # (3)!
          replicas: 3
          platform:
            agent:
              agentLabelSelector:
                matchLabels:
                  example.com/class: fc830
      manifests:
        - apiVersion: metallb.io/v1beta1 # (4)!
          kind: IPAddressPool
          metadata:
            name: hcp-agent-apiserver
            namespace: metallb-system
          spec:
            addresses:
              - 10.20.9.13/32
    ```

    1.  From `platforms/agent.yaml`. Every layer of the values cascade can set `install`.
    2.  Maps merge with the defaults, so setting only `APIServer` keeps the other three. `null` drops an entry.
    3.  NodePool `hcp-agent-compute`. A pool without `agentLabelSelector` claims any free Agent in `agentNamespace`.
    4.  Exists before the HostedCluster, so MetalLB has the address when the kube-apiserver Service asks for one.

=== "hive"

    | Key | Default | Notes |
    |---|---|---|
    | `hive.platform` | required | Exactly one of `aws`, `gcp`, `azure`. Needs `region`; GCP adds `projectID`, Azure `baseDomainResourceGroupName`. Fixed at create. |
    | `hive.installConfig` | `{}` | Merged under the generated install-config. Hive drops fields its installer does not know. |
    | `hive.clusterDeployment` | `{}` | `metadata` and `spec` merged over the generated ClusterDeployment. `spec.provisioning.releaseImage` is ignored. |

    ```yaml
    # values/platforms/aws.yaml, trimmed
    install:
      engine: hive
      credential:
        namespace: hive-credentials
        name: aws                       # (1)!
      hive:
        installConfig:
          controlPlane:
            name: master
            replicas: 3
            platform:
              aws:
                type: m6i.xlarge
          networking:
            networkType: OVNKubernetes

    # fleet/hive-aws.yaml
    install:
      baseDomain: aws.example.com
      hive:
        platform:
          aws:
            region: us-east-2           # (2)!
    ```

    1.  Holds `pullSecret`, `ssh-publickey`, `aws_access_key_id`, `aws_secret_access_key`.
    2.  Goes to both the install-config and the ClusterDeployment.

=== "siteconfig"

    | Key | Default | Notes |
    |---|---|---|
    | `siteconfig.templates` | `ai` | `ai` (assisted installer) or `ibi` (image-based, single node). Fixed at create. |
    | `siteconfig.templateNamespace` | `open-cluster-management` | Namespace of the stock `<templates>-cluster-templates-v1` and `<templates>-node-templates-v1` ConfigMaps. |
    | `siteconfig.bmcCredential.namespace`, `.name` | required | Hub Secret with `username` and `password`, copied to `<cluster>-bmc`. |
    | `siteconfig.clusterInstance` | `{}` | Merged under the generated ClusterInstance spec. |
    | `siteconfig.nodes` | required | hostName to ClusterInstance node. `bmcAddress` and `bootMACAddress` required. Layers merge per host. |

    ```yaml
    # values/platforms/baremetal.yaml
    install:
      engine: siteconfig
      credential:
        namespace: bm-credentials
        name: hostinventory
      siteconfig:
        bmcCredential:
          namespace: bm-credentials
          name: bmc
        clusterInstance:
          networkType: OVNKubernetes

    # fleet/sno-edge.yaml
    install:
      siteconfig:
        clusterInstance:
          clusterType: SNO
          machineNetwork:
            - cidr: 10.20.30.0/24
        nodes:
          sno-edge.dc1.example.com:     # (1)!
            role: master
            bmcAddress: redfish-virtualmedia://10.20.0.11/redfish/v1/Systems/1
            bootMACAddress: "00:11:22:33:44:55"
    ```

    1.  The key becomes `hostName`. `ibi` allows one node; `ai` with `-multi` at most 3 control-plane nodes.

## Generated objects

`<cluster>` is the fleet file name. Everything not listed uses the operator's defaults: set it in `hostedCluster`, `hive.clusterDeployment` or `siteconfig.clusterInstance`.

=== "hypershift"

    | Object | Content |
    |---|---|
    | HostedCluster `<namespace>/<cluster>` | Release image, `pullSecret` `<cluster>-pull-secret`, `sshKey` `<cluster>-ssh-key`, platform, `services`, `channel` and `dns.baseDomain` when set. `Agent` adds `agentNamespace`. `Agent` and `None` add `secretEncryption` with `<cluster>-etcd-encryption-key`. |
    | NodePool `<namespace>/<cluster>-<pool>` | `clusterName`, release image, platform, `management.upgradeType` (`Replace` on `KubeVirt`, else `InPlace`). No `replicas` means 0. |

    !!! warning "etcd encryption key"
        Generated once, while neither the key nor the HostedCluster exists. A deleted key is never regenerated: a new key cannot decrypt etcd. Restore it from backup.

=== "hive"

    | Object | Content |
    |---|---|
    | ClusterDeployment `<cluster>/<cluster>` | `baseDomain`, `clusterName`, platform with `credentialsSecretRef` `<cluster>-creds`, `pullSecretRef`, `installConfigSecretRef` `<cluster>-install-config`, release image, `installAttemptsLimit: 1` (a failed install is not retried). |
    | Secret `<cluster>-creds` | Only the keys Hive reads: `aws_access_key_id` + `aws_secret_access_key`, `osServiceAccount.json`, or `osServicePrincipal.json`. |
    | ManagedCluster `<cluster>`, KlusterletAddonConfig | Every add-on on. |

=== "siteconfig"

    | Object | Content |
    |---|---|
    | ClusterImageSet `lifecycle-<cluster>` | The release image. |
    | ClusterInstance `<cluster>/<cluster>` | `clusterName`, `baseDomain`, `pullSecretRef`, `sshPublicKey`, templates, `clusterImageSetNameRef`, per node `hostName`, `bmcCredentialsName` `<cluster>-bmc` and node templates. SiteConfig defaults `bootMode: UEFI`, `automatedCleaningMode: disabled`. |
    | KlusterletAddonConfig | Every add-on on. The stock `ai` one turns application-manager off, so ACM would never create the Argo CD secret. The chart suppresses it and renders its own. |

Hive and SiteConfig reject release image changes, and SiteConfig any spec change before install completes. The chart reads those fields back from the live objects, so a `version` bump or credential rotation never touches them. Those clusters upgrade through [`upgrade`](upgrades.md).

## Policy chains

The hub gets `present-lifecycle-<cluster>` or `absent-lifecycle-<cluster>`, picked by `state`. Each template waits for the one before it to be Compliant.

```mermaid
flowchart LR
  subgraph present
    pg[guard] --> pc[credentials] --> pm[manifests] --> pcl[cluster] --> pr[ready]
  end
  subgraph absent
    ag[guard] --> ad[detach] --> adt[detached] --> ax[destroy] --> axd[destroyed] --> acl[cleanup]
  end
```

Templates are named `<state>-<step>-<cluster>`, e.g. `present-guard-hcp-kubevirt`. `siteconfig` has no `detach` or `detached`.

`present-lifecycle-<cluster>`:

| Template | Mode | `hypershift` | `hive` | `siteconfig` |
|---|---|---|---|---|
| `guard` | inform | NonCompliant if a ManagedCluster of this name lacks the [owner annotations](#ownership). Stops the chain. | same | same |
| `credentials` | enforce | Namespace `install.namespace`, `<cluster>-pull-secret`, `<cluster>-ssh-key`, etcd key on `Agent`/`None` | Namespace `<cluster>`, `-pull-secret`, `-creds`, `-install-config` | Namespace `<cluster>`, `-pull-secret`, `-bmc` |
| `manifests` | enforce | `install.manifests` | same | same |
| `cluster` | enforce | HostedCluster, NodePools | ClusterDeployment, ManagedCluster, KlusterletAddonConfig | ClusterImageSet, ClusterInstance, KlusterletAddonConfig |
| `ready` | inform | `version` Completed in history and on every NodePool | `spec.installed: true` | `Provisioned` True |

`absent-lifecycle-<cluster>`:

| Template | Mode | `hypershift` | `hive` | `siteconfig` |
|---|---|---|---|---|
| `guard` | inform | Same check | same | same |
| `detach` | enforce `mustnothave` | Deletes the ManagedCluster while the cluster is up, so the klusterlet cleans up | same | none: SiteConfig would re-create it |
| `detached` | inform `mustnothave` | Waits for the ManagedCluster to go | same | none |
| `destroy` | enforce `mustnothave` | Deletes the HostedCluster | Deletes the ClusterDeployment; Hive deprovisions with `<cluster>-creds` | Deletes the ClusterInstance and what it rendered |
| `destroyed` | inform `mustnothave` | Waits for the HostedCluster to go | Waits for deprovision | Waits for the ClusterInstance to go |
| `cleanup` | enforce | Copied secrets, etcd key, `install.manifests` | Namespace `<cluster>`, `install.manifests` | Namespace `<cluster>`, ClusterImageSet, `install.manifests` |

- An enforced delete is Compliant once the API accepts it, so every destroy step has an inform wait after it.
- Create templates are `musthave`: operator-filled fields are not compared, so the Policy never fights HyperShift, Hive or SiteConfig.
- The credential is never deleted.

!!! note
    No lifecycle template sets `pruneObjectBehavior`. Deleting the present Policy deletes nothing; only `state: absent` destroys.

## Day-2 handoff

1. The ManagedCluster appears. `hypershift`: the hypershift-addon creates it once the HostedCluster is available. `hive`, `siteconfig`: ACM imports the one the chart or SiteConfig created.
2. It joins `global`, the GitOpsCluster creates its Argo CD secret, and ACM adds the `clusterID` label ([Install PolicyStack](install.md#install-policystack)).
3. The Day-2 ApplicationSet pairs the secret with the same fleet file and creates `<element>-<cluster>` Applications. It ignores `hub`, `state` and `install`.

Labels in `hostedCluster.metadata.labels` are copied to the ManagedCluster once, at creation.

??? info "HyperShift auto-import disabled"
    Auto-import is on by default. `autoImportDisabled` in the `hypershift-addon-deploy-config` AddOnDeploymentConfig leaves a built cluster without Day-2 until you import it by hand.

## Ownership

Guard and detach act only on a ManagedCluster carrying its engine's owner annotations:

| Engine | Annotations | Set by |
|---|---|---|
| `hypershift` | `open-cluster-management/created-via: hypershift`, `import.open-cluster-management.io/hosting-cluster-name: <hub ManagedCluster>` | hypershift-addon |
| `hive` | `lifecycle.policystk.io/hub: <hubName>` | `present-cluster-<cluster>` |
| `siteconfig` | `lifecycle.policystk.io/hub: <hubName>` | SiteConfig, from ClusterInstance `extraAnnotations` |

A hand-made ManagedCluster needs these, or the guard stops both chains.

## Immutable fields

The operators reject changes to these, among others:

| Object | Immutable |
|---|---|
| HostedCluster | `services`, `platform`, `networking`, `etcd`, `issuerURL`, `controllerAvailabilityPolicy`, `fips`, `dns.baseDomain` once set. `secretEncryption` cannot be removed. |
| NodePool | `clusterName`, `platform.type`, `arch`, `management.upgradeType` |
| ClusterDeployment | Everything but `installAttemptsLimit`, `powerState`, `preserveOnDelete` and a few others. `hive.installConfig` changes reach the Secret, not the cluster. |
| ClusterInstance | Everything until install completes. After: only `extraAnnotations`, `extraLabels`, `suppressedManifests`, `pruneManifests`, and adding a node. |

Changing one does not change the cluster: `present-cluster-<cluster>` goes NonCompliant with the API server's message. Revert the value.

- NodePool `upgradeType` or `arch`: add a new pool, remove the old one.
- Failed SiteConfig install: set `state: absent`, then build again with the fix.

## Removing a NodePool or host

`musthave` never deletes. Removing an entry only stops managing it.

=== "NodePool"

    Remove the `nodePools` entry, then:

    ```sh
    oc delete nodepool <cluster>-<pool> -n clusters
    ```

    A pool from a shared layer, like `workers` in `platforms/kubevirt.yaml`, cannot be removed from a higher layer (the schema rejects `null`). Scale it down:

    ```yaml
    install:
      nodePools:
        workers:
          replicas: 0
    ```

=== "SiteConfig host"

    Remove the `nodes` entry, then remove the host from the live object. Changing a host after install needs the same edit.

    ```sh
    oc edit clusterinstance <cluster> -n <cluster>
    ```

## Troubleshooting

Start with the Application for render errors (missing `version`, credential or required key), then the Policy:

```sh
oc get applications.argoproj.io lifecycle-<cluster> -n openshift-gitops -o jsonpath='{.status.conditions}'
```

| Symptom | Cause | Fix |
|---|---|---|
| Guard: "A ManagedCluster with this name was not built by this hub" | Cluster was imported or runs on another hub. Usually `hub` was added to an imported cluster's fleet file. | Remove `hub`, `state`, `install` from the fleet file. |
| `present-credentials` NonCompliant, template error | Credential Secret missing | Create it. Without `pullSecret` the API rejects the pull secret. |
| Cluster has no SSH key, no error | `ssh-publickey` missing from the credential | Add it. |
| `hive`/`siteconfig`: `present-credentials-<cluster>` NonCompliant: `because it is being terminated` | Namespace `<cluster>` from the last destroy is still terminating | Wait until `oc get ns <cluster>` returns NotFound. |

!!! warning "Switching engines"
    The guard cannot catch an engine switch before the cluster is imported. Destroy before changing `engine`.

??? failure "Hive: install failed"
    `present-ready` stays NonCompliant, ClusterDeployment shows `ProvisionStopped`. Read the installer log, fix the values, destroy and build again.

    `RequirementsMet` False with `InstallConfigValidationFailed`: the install-config and ClusterDeployment regions differ, through `hive.clusterDeployment` or an edit of `hive.platform` during install. Revert it.

    A cloud key missing from the credential is copied as an empty value and fails the install.

??? failure "Hive: deprovision stuck"
    `absent-destroyed` stays NonCompliant; ClusterDeployment shows `DeprovisionLaunchError`, or the ClusterDeprovision `AuthenticationFailure`. Fix the keys in `<cluster>-creds` directly: the absent Policy no longer copies the credential.

    With HiveConfig `deleteProtection: enabled`:

    ```sh
    oc annotate clusterdeployment <cluster> -n <cluster> hive.openshift.io/protected-delete-
    ```

??? failure "SiteConfig: rejected or failed"
    `present-cluster` shows a webhook denial, or the ClusterInstance shows `Provisioned` False:

    ```sh
    oc get clusterinstance <cluster> -n <cluster> -o jsonpath='{.status.conditions}'
    ```

    Check the AgentClusterInstall too. Destroy, fix, build again. A wrong ref, like `templateNamespace`, can be fixed by creating the missing object.

??? failure "SiteConfig: cluster never imported"
    The hub cannot resolve `api.<cluster>.<baseDomain>`. Add the DNS record where the hub resolves it.

??? failure "SiteConfig: deletion paused"
    The ClusterInstance has `clusterinstance.siteconfig.open-cluster-management.io/paused`: a rendered object (usually a BareMetalHost or the ManagedCluster) was not gone within 30 minutes. Fix what holds it, then remove the annotation.

??? failure "ManagedCluster stuck Terminating"
    `absent-detached` stays NonCompliant. Hosted-mode cleanup waits for the guest; ACM gives up after 5 minutes unreachable. Check:

    ```sh
    oc get klusterlet klusterlet-<cluster> -o jsonpath='{.metadata.finalizers}'
    ```

    If it waits on `operator.open-cluster-management.io/klusterlet-hosted-cleanup` and the guest will not answer, remove that finalizer.

??? failure "Agent: Machines stuck Deleting"
    Machines in `<namespace>-<cluster>` stay `Deleting`, `PreTerminateDeleteHookSucceeded` reports `WaitingExternalHook`. Remove the hook:

    ```sh
    for m in $(oc get machines.cluster.x-k8s.io -n clusters-<cluster> -o name); do
      oc annotate "$m" -n clusters-<cluster> pre-terminate.delete.hook.machine.cluster.x-k8s.io/agentmachine-
    done
    ```

??? failure "Agent: stuck reclaiming"
    After destroy, Agents go to `reclaiming` while they reboot into discovery. One that cannot ends in `UnbindingPendingUserAction`: boot it from the discovery ISO by hand. Agents still holding the `agent.agent-install.openshift.io/ai-deprovision` finalizer block any new cluster that selects them.

## Adding an engine

An engine `<e>` is a component of `lifecycle/`. `templates/policy.yaml` renders whichever component `install.engine` names, so the ApplicationSet and fleet keys do not change.

1. Add `converters/<e>/`: objects to create, the root object by identity alone (for destroy), and its credentials. `lookup` back every field the engine rejects changes to. End the cleanup converter with `include "lifecycle.cleanupManifests"`.
2. Add `stack.<e>` to `values.yaml` with `present` and `absent` Policies following the [chains](#policy-chains). No Policy-level `remediationAction` or `pruneObjectBehavior`: the first overrides the inform gates, the second lets a Policy delete a cluster.
3. Add `<e>` to the `install.engine` enum in `values.schema.json`, with its keys. POLICY090 fails when the enum and `stack` keys differ ([Rules](validation.md#rules)).
4. Add an owner case to `lifecycle.ownerAnnotations`, a present and an absent fleet file to `tools/validator/testdata/clusters/`, and an example to [Cluster lifecycle](lifecycle.md).

An engine that upgrades from the hub maps `version` onto its objects, like `hypershift`. One that only installs uses `version` at create and leaves upgrades to `upgrade` ([Upgrades](upgrades.md)).
