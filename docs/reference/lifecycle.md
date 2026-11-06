# Lifecycle keys

Every `install` key the [`lifecycle/`](https://github.com/PolicyStack/PolicyStack/tree/main/lifecycle) chart reads, the objects it renders from them, the steps of its Policies, and what to do when a build fails. Read [Cluster lifecycle](../concepts/lifecycle.md) first for the model; [Building clusters](../guides/lifecycle.md) has complete examples. `hub`, `state`, `version` and `channel` are in [Fleet files](fleet.md#keys).

!!! warning "Every engine and platform is a preview"
    Lifecycle follows the framework's version, but no engine or platform is stable yet: a framework minor may change their converters, behaviour and defaults not fixed at create, with the steps in its release notes. Their `install` keys and the names of the objects they create change only in a major ([Maturity](compatibility.md#maturity)). Pin clusters the hub builds to tags.

How to read this page:

- `hypershift` keys sit directly under `install` (`install.platform`, `install.nodePools`). `hive` and `siteconfig` keys sit under `install.hive` and `install.siteconfig`, so `install.hive.platform` is a different key from `install.platform`. Any [values layer](../concepts/values.md) can set `install`; the fleet file's `install` wins.
- The chart's schema rejects unknown keys, and rejects a non-empty `hive` or `siteconfig` block unless `engine` selects it. It does not reject `hypershift` keys under another engine: `hive` and `siteconfig` ignore them.
- **Fixed at create** means the engine's API rejects later changes. Changing the key never rebuilds the cluster: `present-cluster-<cluster>` goes NonCompliant until you revert it ([Immutable fields](#immutable-fields)).
- **Fails the render** means Helm stops on a schema error, `required` or `fail`. Application `lifecycle-<cluster>` shows the message as a ComparisonError and nothing on the hub changes. A step that **fails** does so later, when ACM evaluates it on the hub: that step goes NonCompliant with the message, and the steps after it stay Pending ([Troubleshooting](#troubleshooting)).

## Install keys

Defaults are in [`lifecycle/values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/lifecycle/values.yaml).

| Key | Default | Notes |
|---|---|---|
| `engine` | `hypershift` | `hypershift`, `hive` or `siteconfig` ([Pick an engine](../concepts/lifecycle.md#pick-an-engine)). Never change it on a live cluster: the hub can build a second cluster ([Changing `engine`](../concepts/lifecycle.md#ownership)). Destroy first. |
| `credential.namespace`, `.name` | required | Opaque hub Secret you create by hand, in any namespace; an ACM console credential (Infrastructure > Credentials) of the matching type has these data keys. Creation commands: [Building clusters](../guides/lifecycle.md#examples). Keys: `pullSecret`, `ssh-publickey`, and for `hive` the cloud keys (AWS `aws_access_key_id`, `aws_secret_access_key`; GCP `osServiceAccount.json`; Azure `osServicePrincipal.json`; vSphere `username`, `password`, `cacertificate`). |
| `baseDomain` | `""` | Required except on `KubeVirt`, which uses the hub's ingress domain. Fixed at create. |
| `releaseRepository` | `quay.io/openshift-release-dev/ocp-release` | Image is `<releaseRepository>:<version>-multi`, `-x86_64` on `siteconfig`. Point at a mirror when disconnected. |
| `manifests` | `[]` | Extra hub objects with `apiVersion`, `kind`, `metadata`, applied by the `manifests` step before the cluster. Applied as `musthave` with no ownership check, so an existing object of the same name is patched. `cleanup` deletes every object listed at destroy time, one that existed before included. Removing an entry stops managing the object but never deletes it. ACM templates inside entries resolve on the hub. |

!!! warning "Keep the credential for the cluster's life"
    The `credentials` step re-reads it on every evaluation and copies it into per-cluster Secrets. If it is deleted, `credentials` fails and every later step stops, so version bumps and NodePool changes no longer apply. Lifecycle never deletes it.

=== "hypershift"

    | Key | Default | Notes |
    |---|---|---|
    | `platform` | required | `KubeVirt`, `Agent` or `None`. No default: without it the render fails with `install.platform is required`. Fixed at create. |
    | `namespace` | `clusters` | HostedCluster, NodePools and copied Secrets. The control plane runs in `<namespace>-<cluster>`. |
    | `agentNamespace` | `""` | Namespace of the InfraEnv and Agents you created. Required on `Agent`. |
    | `services` | `APIServer: LoadBalancer`, others `Route` | Service name to `servicePublishingStrategy`. `APIServer`, `OAuthServer`, `Konnectivity`, `Ignition`: HyperShift needs all four. Maps merge, so setting one keeps the defaults for the others. Fixed at create. |
    | `hostedCluster` | `{}` | `metadata` and `spec` merged over the generated HostedCluster: your values win, maps merge, lists replace. `spec.release` is ignored. Removing a field later does not remove it from the live object. `metadata.labels` are copied to the ManagedCluster once, at creation; later changes are not, so edit the ManagedCluster directly. |
    | `nodePools` | `{}` | Pool name to NodePool spec, without a `spec:` wrapper, merged over the generated spec. `release` is ignored. Named `<cluster>-<pool>`. No `replicas` means 0. Removing a pool stops managing it; delete the NodePool by hand ([Removing a NodePool](../guides/lifecycle.md#removing-a-nodepool)). |

    `hcp-agent` reads `environments/prod.yaml`, `datacenters/dc1.yaml`, `platforms/agent.yaml`, [`values/clusters/hcp-agent.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values/clusters/hcp-agent.yaml) and its fleet file. PolicyStack ships no `fleet/`; the example fleet file is the header comment of `values/clusters/hcp-agent.yaml`, with the `nodePools` of the Agent example in [Building clusters](../guides/lifecycle.md#examples). The merged `install`, trimmed:

    ```yaml
    install:
      platform: Agent                    # (1)!
      agentNamespace: hardware-inventory
      credential:
        namespace: hcp-credentials
        name: hostinventory
      baseDomain: dc1.example.com        # (2)!
      services:                          # (3)!
        APIServer:
          type: LoadBalancer
          loadBalancer:
            hostname: api-int-hcp-agent.dc1.example.com
        OAuthServer:
          type: Route
          route:
            hostname: oauth-hcp-agent.dc1.example.com
        Konnectivity: ...
        Ignition: ...
      hostedCluster:                     # (4)!
        spec:
          kubeAPIServerDNSName: api-hcp-agent.dc1.example.com
      nodePools:
        workers:                         # (5)!
          replicas: 2
          platform:
            agent:
              agentLabelSelector:
                matchLabels:
                  example.com/cluster: hcp-agent
      manifests:
        - apiVersion: metallb.io/v1beta1 # (6)!
          kind: IPAddressPool
          metadata:
            name: hcp-agent-apiserver
            namespace: metallb-system
          spec:
            addresses:
              - 10.20.9.13/32
    ```

    1.  `platform`, `agentNamespace` and `credential` come from `platforms/agent.yaml`.
    2.  From `datacenters/dc1.yaml`, shared by every cluster in dc1.
    3.  From `values/clusters/hcp-agent.yaml`, which sets all four services to add hostnames. A layer that set only `APIServer` would keep the default `Route` for the other three.
    4.  Also from `values/clusters/hcp-agent.yaml`, with a `configuration.apiServer` serving certificate not shown.
    5.  From the fleet file. NodePool `hcp-agent-workers`. A pool without `agentLabelSelector` claims any free Agent in `agentNamespace`.
    6.  From `values/clusters/hcp-agent.yaml`, with an L2Advertisement and a cert-manager Certificate not shown. The `manifests` step creates it before the HostedCluster, so MetalLB has the address when the kube-apiserver Service asks for one.

=== "hive"

    | Key | Default | Notes |
    |---|---|---|
    | `hive.platform` | required | Exactly one of `aws`, `gcp`, `azure`, `vsphere`, as in the installer's install-config. Needs `region`; GCP adds `projectID`, Azure `baseDomainResourceGroupName`. `region`, `baseDomainResourceGroupName` and `cloudName` also go to the ClusterDeployment. vSphere: needs `vcenters` and `failureDomains`, refuses `user`/`password`; every `vcenters` entry gets the credential's login, in the install-config only. Fixed at create. |
    | `hive.installConfig` | `{}` | Merged under the generated install-config: the chart's `apiVersion`, `metadata.name`, `baseDomain` and `platform` win over the same keys here. `pullSecret` is left out; Hive adds it from `pullSecretRef`. `sshKey` defaults to the credential's `ssh-publickey`. Hive drops fields its installer does not know. Changes after create reach the install-config Secret, not the cluster. |
    | `hive.clusterDeployment` | `{}` | `metadata` and `spec` merged over the generated ClusterDeployment; your values win. `spec.provisioning.releaseImage` is ignored. |

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

    # fleet/ocp-aws.yaml
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
    | `siteconfig.templates` | `ai` | `ai` (assisted installer) or `ibi` (image-based, single node). `ibi` needs the host preinstalled from a seed image at `version` beforehand, outside PolicyStack, and the MCE `image-based-install-operator` component enabled; no key here sets the seed ([Building clusters](../guides/lifecycle.md#examples)). Fixed at create. |
    | `siteconfig.templateNamespace` | `open-cluster-management` | Namespace of the stock `<templates>-cluster-templates-v1` and `<templates>-node-templates-v1` ConfigMaps. |
    | `siteconfig.bmcCredential.namespace`, `.name` | required | Hub Secret with `username` and `password`, copied to `<cluster>-bmc`. `credentials` fails on an empty `username` or `password`. Keep it for the cluster's life, as for `credential`. |
    | `siteconfig.machineNetwork` | | Node subnets. Required with `ai` and one master. |
    | `siteconfig.apiVIPs`, `.ingressVIPs` | | Required with more than one master, forbidden with one. Up to 2 (dual-stack), the same number in each. |
    | `siteconfig.clusterNetwork` | installer default | `{cidr, hostPrefix}` list. |
    | `siteconfig.serviceNetwork` | installer default | Subnets. |
    | `siteconfig.ntpSources` | | Extra NTP sources. |
    | `siteconfig.proxy` | | `httpProxy` and/or `httpsProxy`, `noProxy`. |
    | `siteconfig.extraManifests` | | ConfigMap names in namespace `<cluster>`, from `install.manifests`. |
    | `siteconfig.fips` | `false` | `true` installs in FIPS mode. `ai` only. |
    | `siteconfig.cpuPartitioning` | `false` | `true` installs with workload partitioning. `ai` only. Enable [`partitioning`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/partitioning) on the cluster for its PerformanceProfile. |
    | `siteconfig.nodes` | required | hostName to the keys below. Layers merge per host. |

    | Node key | Default | Notes |
    |---|---|---|
    | `bmcAddress` | required | `redfish-`, `idrac-` or `ilo5-virtualmedia`, with optional `+http`/`+https`; `ilo4-virtualmedia`, optional `+https`. |
    | `bootMACAddress` | required | Quote it: YAML 1.1 tools read some all-digit MACs as base-60 numbers. |
    | `role` | `master` | `master`, `worker` or `arbiter`. |
    | `rootDeviceHints` | | BareMetalHost hints. `deviceName` takes `/dev/` or `/dev/disk/by-path/` only. |
    | `network` | | Static install network: `interfaces` (NIC name to MAC) and `config` (nmstate). Install only; afterwards change node networking with the [`nmstate`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/nmstate) element on the cluster. |
    | `remove` | | `true` on a worker [scales it in](../guides/lifecycle.md#scaling-siteconfig-workers). The schema requires `role: worker` on the same node, set explicitly. Not allowed with `ibi`. |

    All of it is fixed at create; afterwards you can only add and remove workers. `ai` takes 1 or 3 masters, or 2 or more with an arbiter (4.19+). `ibi` takes one node and no VIPs, `clusterNetwork`, `serviceNetwork`, `fips` or `cpuPartitioning`: the seed image fixes them. Anything else fails the render, for example `install.siteconfig.nodes: 2 masters; need 1, 3, or 2 or more with an arbiter`.

    !!! warning "FIPS"
        The hub's nodes must run in FIPS mode: assisted-service runs the installer there, and the installer refuses FIPS on a non-FIPS host. The `ssh-publickey` must be RSA or ECDSA, as FIPS mode disables ed25519. `present-cluster-<cluster>` fails before create on either.

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

    # fleet/sno-edge.yaml
    install:
      siteconfig:
        machineNetwork: [10.20.30.0/24]
        nodes:
          sno-edge.dc1.example.com:     # (1)!
            bmcAddress: redfish-virtualmedia://10.20.0.11/redfish/v1/Systems/1
            bootMACAddress: "00:11:22:33:44:55"
    ```

    1.  The key becomes `hostName`.

## Generated objects

What the `credentials` and `cluster` steps create. `<cluster>` is the fleet file name. Everything not listed uses the operator's defaults; override it in `hostedCluster` or `hive.clusterDeployment`. `siteconfig` takes only the keys above. Every root object, and the ManagedCluster on `hive` and `siteconfig`, carries the [ownership marker](../concepts/lifecycle.md#ownership).

=== "hypershift"

    | Object | Content |
    |---|---|
    | Secrets `<cluster>-pull-secret`, `<cluster>-ssh-key` | Copied from the credential into `namespace`. |
    | Secret `<cluster>-etcd-encryption-key` | `Agent` and `None` only; HyperShift creates its own on `KubeVirt`. |
    | HostedCluster `<namespace>/<cluster>` | Release image, `pullSecret`, `sshKey`, platform, `services`, `channel` and `dns.baseDomain` when set. `Agent` adds `agentNamespace`. `Agent` and `None` add `secretEncryption` with the etcd key. |
    | NodePool `<namespace>/<cluster>-<pool>` | `clusterName`, release image, platform, `management.upgradeType` (`Replace` on `KubeVirt`, else `InPlace`). |

    !!! warning "etcd encryption key"
        Generated once, while neither the key nor the HostedCluster exists. A deleted key is never regenerated: a new key cannot decrypt etcd. Restore it from backup.

    ??? example "Rendered for the hcp-agent values above, trimmed"
        ```yaml
        apiVersion: hypershift.openshift.io/v1beta1
        kind: HostedCluster
        metadata:
          name: hcp-agent
          namespace: clusters
          annotations:
            lifecycle.policystk.io/hub: acm-dc1
        spec:
          channel: stable-4.20
          dns:
            baseDomain: dc1.example.com
          kubeAPIServerDNSName: api-hcp-agent.dc1.example.com
          platform:
            type: Agent
            agent:
              agentNamespace: hardware-inventory
          pullSecret:
            name: hcp-agent-pull-secret
          sshKey:
            name: hcp-agent-ssh-key
          secretEncryption:
            type: aescbc
            aescbc:
              activeKey:
                name: hcp-agent-etcd-encryption-key
          services:
            - service: APIServer
              servicePublishingStrategy:
                type: LoadBalancer
                loadBalancer:
                  hostname: api-int-hcp-agent.dc1.example.com
            # Ignition, Konnectivity, OAuthServer: Route with their hostnames
          release:
            image: quay.io/openshift-release-dev/ocp-release:4.20.8-multi  # (1)!
        ---
        apiVersion: hypershift.openshift.io/v1beta1
        kind: NodePool
        metadata:
          name: hcp-agent-workers
          namespace: clusters
        spec:
          clusterName: hcp-agent
          management:
            upgradeType: InPlace
          platform:
            type: Agent
            agent:
              agentLabelSelector:
                matchLabels:
                  example.com/cluster: hcp-agent
          replicas: 2
          release:
            image: quay.io/openshift-release-dev/ocp-release:4.20.8-multi
        ```

        1.  Set from `version` when allowed: at once without `channel`, with `channel` once HyperShift lists it in `availableUpdates`, never a downgrade. Otherwise the live image stays ([Upgrades](../guides/lifecycle.md#upgrades)).

=== "hive"

    | Object | Content |
    |---|---|
    | Namespace `<cluster>` | Holds everything below except the ManagedCluster. |
    | Secret `<cluster>-pull-secret` | Copied from the credential. |
    | Secret `<cluster>-creds` | Only the cloud keys Hive reads, copied from the credential. |
    | Secret `<cluster>-certs` | vSphere: `.cacert` from `cacertificate`. |
    | Secret `<cluster>-install-config` | `install-config.yaml` built from `hive.installConfig` and `hive.platform`. vSphere: the only place the vCenter login goes. |
    | ClusterDeployment `<cluster>/<cluster>` | `baseDomain`, `clusterName`, platform with `credentialsSecretRef` `<cluster>-creds`, `pullSecretRef`, `installConfigSecretRef`, release image, `installAttemptsLimit: 1` (a failed install is not retried). vSphere: `infrastructure` is `hive.platform.vsphere` with a blank login, plus `certificatesSecretRef` `<cluster>-certs`. |
    | ManagedCluster `<cluster>`, KlusterletAddonConfig | Every add-on on. |

=== "siteconfig"

    | Object | Content |
    |---|---|
    | Namespace `<cluster>`, Secrets `<cluster>-pull-secret`, `<cluster>-bmc` | Copied from the credential and `bmcCredential`. |
    | ClusterImageSet `lifecycle-<cluster>` | The release image. |
    | ClusterInstance `<cluster>/<cluster>` | The keys above, plus `clusterName`, `baseDomain`, `pullSecretRef`, `sshPublicKey`, templates, `clusterImageSetNameRef`, per node `hostName`, `bmcCredentialsName` `<cluster>-bmc` and node templates. `clusterType` follows the masters: 1 is `SNO`, an arbiter `HighlyAvailableArbiter`, else `HighlyAvailable`. `fips` becomes `installConfigOverrides: '{"fips":true}'`, `cpuPartitioning` `cpuPartitioningMode: AllNodes`. SiteConfig defaults `bootMode: UEFI`, `automatedCleaningMode: disabled`. |
    | KlusterletAddonConfig | Every add-on on ([Day-2 handoff](../concepts/lifecycle.md#day-2-handoff)). |

Three fields are read back from the live objects instead of from your values: the ClusterDeployment's `spec.provisioning.releaseImage` (`hive`), and the ClusterImageSet's `releaseImage` and the ClusterInstance's `sshPublicKey` (`siteconfig`). Hive rejects release image changes and SiteConfig most spec changes, so a `version` bump or a new `ssh-publickey` never reaches them, and a hand edit to the image set is kept. Those clusters upgrade through [`upgrade`](../guides/upgrades.md). Pull secret, cloud keys and BMC login are not read back: `credentials` re-copies them on every evaluation.

## Policy chains

The steps of `present-lifecycle-<cluster>` and `absent-lifecycle-<cluster>` per engine. Why they are chained and gated: [The step chain](../concepts/lifecycle.md#the-step-chain). Each template is named `<state>-<step>-<cluster>`, e.g. `present-guard-hcp-kubevirt`, and waits for the one before it to be Compliant.

Mode column: **enforce** creates or patches (`musthave`), or deletes with `mustnothave`. **InformOnly** only reports, even though the Policy is enforce; with `mustnothave` it is Compliant once the object is gone. The `hypershift` and `siteconfig` `ready` gates, while unsatisfied, put a `mustnothave` on the object they wait for, only to name it in the message; nothing is deleted. `hive` `ready` is a `musthave` on `spec.installed: true`.

`present-lifecycle-<cluster>`:

| Template | Mode | `hypershift` | `hive` | `siteconfig` |
|---|---|---|---|---|
| `guard` | InformOnly | NonCompliant, naming the object, while the [ownership check](../concepts/lifecycle.md#ownership) finds a foreign one. Stops the chain. | same | same |
| `credentials` | enforce | Namespace `install.namespace`, `<cluster>-pull-secret`, `<cluster>-ssh-key`, etcd key on `Agent`/`None` | Namespace `<cluster>`, `-pull-secret`, `-creds`, `-install-config`, `-certs` on vSphere | Namespace `<cluster>`, `-pull-secret`, `-bmc`. Fails on an empty BMC `username` or `password`. |
| `manifests` | enforce | `install.manifests` | same | same |
| `cluster` | enforce | HostedCluster, NodePools | ClusterDeployment, ManagedCluster, KlusterletAddonConfig | ClusterImageSet, ClusterInstance, KlusterletAddonConfig |
| `ready` | InformOnly | The HostedCluster's desired version and newest Completed version both equal `version`, and every NodePool reports `version` | `spec.installed: true` | `Provisioned` True and every host joined (Agent stage `Done`) or removed. Names the pending host's BareMetalHost while it exists, otherwise the ClusterInstance. |

`absent-lifecycle-<cluster>`:

| Template | Mode | `hypershift` | `hive` | `siteconfig` |
|---|---|---|---|---|
| `guard` | InformOnly | Same check | same | same |
| `detach` | enforce `mustnothave` | Deletes the ManagedCluster while the cluster is up, so the klusterlet cleans up | same | none: SiteConfig would re-create it |
| `detached` | InformOnly `mustnothave` | Waits for the ManagedCluster to go | same | none |
| `destroy` | enforce `mustnothave` | Deletes the HostedCluster; HyperShift deletes the NodePools and control plane | Deletes the ClusterDeployment; Hive deprovisions with `<cluster>-creds` (and `-certs` on vSphere) | Deletes the ClusterInstance; SiteConfig deletes what it rendered: BareMetalHosts, then ManagedCluster |
| `destroyed` | InformOnly `mustnothave` | Waits for the HostedCluster to go | Waits for deprovision | Waits for the ClusterInstance to go |
| `cleanup` | enforce `mustnothave` | Copied secrets, etcd key, `install.manifests` | Namespace `<cluster>`, `install.manifests` | Namespace `<cluster>`, ClusterImageSet, `install.manifests` |

Create templates are `musthave`: only the fields the template lists are compared, so fields HyperShift or Hive fill in never cause a fight, and nothing is ever removed. The ClusterInstance alone is `mustonlyhave` with `metadataComplianceType: musthave`: `musthave` matches list items by name, which ClusterInstance nodes lack, so a changed node would be appended as a duplicate and a node could never be dropped. `metadataComplianceType` keeps the labels and annotations SiteConfig adds.

## Immutable fields

The engines reject changes to these, among others:

| Object | Immutable |
|---|---|
| HostedCluster | `services`, `platform`, `networking`, `etcd`, `issuerURL`, `controllerAvailabilityPolicy`, `fips`, `dns.baseDomain` once set. `secretEncryption` cannot be removed. |
| NodePool | `clusterName`, `platform.type`, `arch`, `management.upgradeType` |
| ClusterDeployment | Everything but `installAttemptsLimit`, `powerState`, `preserveOnDelete` and a few others. `hive.installConfig` changes reach the Secret, not the cluster. On vSphere all of `hive.platform.vsphere`. |
| ClusterInstance | Everything until the install completes. After: `extraAnnotations`, `extraLabels`, `suppressedManifests`, `pruneManifests`, `clusterImageSetNameRef`, and adding or removing workers. The chart never changes `clusterImageSetNameRef`. |

A change to any of these is denied and blocks every other change to that object ([Immutable fields](../concepts/lifecycle.md#immutable-fields)). Revert the value.

- NodePool `upgradeType` or `arch`: add a new pool, remove the old one.
- Any `siteconfig` key on a live cluster, including a field of an existing worker: `unauthorized changes in immutable fields` (cluster keys) or `unauthorized change to …` (host keys). Only adding and removing workers is allowed.
- Any ClusterInstance spec edit before the install completes: `spec update not allowed during provisioning or cluster reinstalls`. The step retries after the install, but SiteConfig then accepts only the changes listed in the table, so anything else is denied again as immutable. Revert it, or destroy and build again. A new worker never hits this: the chart holds it back until the install completes.
- Failed SiteConfig install: destroy, then build again with the fix ([Destroy](../guides/lifecycle.md#destroy)).

!!! warning "ClusterInstance is `mustonlyhave`"
    A render change on an installed cluster is denied on every evaluation. A hand-set `spec.reinstall` keeps `present-cluster-<cluster>` NonCompliant the same way, and a SiteConfig webhook outage turns installed clusters NonCompliant until it returns ([why](../concepts/lifecycle.md#immutable-fields)).

## Troubleshooting

Check the Application first for render errors (missing `version`, credential or required key), then the Policy's steps. Read the first step that is not Compliant; the ones after it are Pending.

```sh
oc get applications.argoproj.io lifecycle-<cluster> -n openshift-gitops -o jsonpath='{.status.conditions}'
oc get policy present-lifecycle-<cluster> -n pstack                                  # overall Compliant/NonCompliant
oc get policy pstack.present-lifecycle-<cluster> -n local-cluster -o jsonpath='{range .status.details[*]}{.templateMeta.name}{": "}{.history[0].message}{"\n"}{end}'   # each step's compliance and message
oc get configurationpolicy -n local-cluster present-<step>-<cluster> -o yaml         # one step in full
```

The steps run in the hub's own cluster namespace, `local-cluster`; under a default-mode Global Hub, in the managed hub's namespace on that hub ([Global Hub](../guides/global-hub.md#lifecycle)).

| Symptom | Cause | Fix |
|---|---|---|
| No `lifecycle-<cluster>` Application after ~6 min | Fleet file not on the default branch; `hub` differs from the hub's `hubName`; hub has no `local-cluster`; ApplicationSet generation error | `oc get applicationset policystack-lifecycle -n openshift-gitops -o jsonpath='{.status.conditions}'`; see [Fleet files](../concepts/overview.md#fleet-files). |
| `guard`: `…: not built by lifecycle (no lifecycle.policystk.io/hub annotation); refusing to build it` (`to detach it` or `to destroy it` when absent). Other steps: `<Kind> <cluster> was not built by lifecycle (no lifecycle.policystk.io/hub annotation); refusing` | Imported, or built by hand or the console. Usually `hub` was added to an imported cluster's fleet file. | Remove `hub`, `state` and `install` from the fleet file. Lifecycle never adopts a cluster it did not build ([Ownership](../concepts/lifecycle.md#ownership)). |
| `present-credentials` NonCompliant, template error | Credential Secret missing | Create it. Without `pullSecret` the API rejects the pull secret. |
| Cluster has no SSH key, no error | `ssh-publickey` missing from the credential | Add it. |
| `hive`/`siteconfig`: `present-credentials-<cluster>` NonCompliant: `because it is being terminated` | Namespace `<cluster>` from the last destroy is still terminating | Wait until `oc get ns <cluster>` returns NotFound. |
| `present-cluster-<cluster>` NonCompliant with an API or webhook denial | A [fixed-at-create](#immutable-fields) key changed | Revert it. |
| Two clusters with the same name after changing `engine` | Ownership check missed the switch | Destroy the new one; never change `engine` without destroying first ([Changing `engine`](../concepts/lifecycle.md#ownership)). |
| `hypershift` cluster built but no Day-2 Applications | `autoImportDisabled` in AddOnDeploymentConfig `hypershift-addon-deploy-config` | Remove it, or import by hand. |

??? failure "Hive: install failed"
    `present-ready` stays NonCompliant, ClusterDeployment shows `ProvisionStopped`. Read the installer log:

    ```sh
    oc logs -n <cluster> -l hive.openshift.io/install=true -c installer
    ```

    Hive does not retry (`installAttemptsLimit: 1`), and provisioning fields are immutable, so fixed values never reach this ClusterDeployment. Fix the values, [destroy](../guides/lifecycle.md#destroy) and build again.

    `RequirementsMet` False with `InstallConfigValidationFailed`: the install-config and ClusterDeployment regions differ, through `hive.clusterDeployment` or an edit of `hive.platform` during install. Revert it.

    A cloud key missing from the credential is copied as an empty value and fails the install.

??? failure "Hive: vSphere credentials rejected"
    ClusterDeployment `AuthenticationFailure` True: the hub could not log in to vCenter with `<cluster>-creds` and `<cluster>-certs`. Check for a wrong login, a trailing newline, a `cacertificate` that is not PEM, vCenter on a port other than 443, or no route from the hub. Right after create it can be brief while the trust bundle fills.

    `RequirementsMet` False, "install config does not contain username/password": the credential is missing `username` or `password`.

    `cluster` NonCompliant with `must specify vSphere vCenter`: the hub's Hive predates zonal vSphere support. Upgrade MCE.

??? failure "Hive: deprovision stuck"
    `absent-destroyed` stays NonCompliant; ClusterDeployment shows `DeprovisionLaunchError`, or the ClusterDeprovision `AuthenticationFailure`. Fix the keys in `<cluster>-creds` (or `<cluster>-certs` on vSphere) directly: the absent Policy no longer copies the credential.

    `absent-destroy-<cluster>` NonCompliant with a webhook denial mentioning protected delete (HiveConfig `deleteProtection: enabled`):

    ```sh
    oc annotate clusterdeployment <cluster> -n <cluster> hive.openshift.io/protected-delete-
    ```

??? failure "SiteConfig: rejected or failed"
    `present-cluster` shows a webhook denial, or the ClusterInstance shows `Provisioned` False:

    ```sh
    oc get clusterinstance <cluster> -n <cluster> -o jsonpath='{.status.conditions}'
    ```

    Check the AgentClusterInstall too. Destroy, fix, build again. A wrong ref, like `templateNamespace`, can be fixed by creating the missing object. For a hub-side problem, check Policy `ready-cim-<hubName>` from the [`cim`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/cim) element, which reports whether the hub's provisioning services run: `oc get policy -n pstack ready-cim-<hubName>`.

??? failure "SiteConfig: cluster never imported"
    The hub cannot resolve `api.<cluster>.<baseDomain>`. Add the DNS record where the hub resolves it.

??? failure "SiteConfig: deletion paused"
    The ClusterInstance has `clusterinstance.siteconfig.open-cluster-management.io/paused`: a rendered object (usually a BareMetalHost or the ManagedCluster) was not gone within 30 minutes. Fix what holds it, then remove the annotation.

??? failure "HyperShift: ManagedCluster stuck Terminating"
    `absent-detached` stays NonCompliant. A hosted cluster's klusterlet runs on the hub in hosted mode, as Klusterlet `klusterlet-<cluster>`, and its cleanup waits for the guest to answer; ACM gives up after 5 minutes unreachable. Check:

    ```sh
    oc get klusterlet klusterlet-<cluster> -o jsonpath='{.metadata.finalizers}'
    ```

    If it waits on `operator.open-cluster-management.io/klusterlet-hosted-cleanup` and the guest will not answer, remove that finalizer.

??? failure "HyperShift Agent: Machines stuck Deleting (destroy or NodePool scale-down)"
    Machines in the control-plane namespace `<namespace>-<cluster>` (`clusters-<cluster>` by default) stay `Deleting`, `PreTerminateDeleteHookSucceeded` reports `WaitingExternalHook`. Remove the hook:

    ```sh
    for m in $(oc get machines.cluster.x-k8s.io -n clusters-<cluster> -o name); do
      oc annotate "$m" -n clusters-<cluster> pre-terminate.delete.hook.machine.cluster.x-k8s.io/agentmachine-
    done
    ```

??? failure "Agent: stuck reclaiming"
    After destroy, Agents go to `reclaiming` while they reboot into discovery. One that cannot ends in `UnbindingPendingUserAction`: boot it from the discovery ISO by hand. Agents still holding the `agent.agent-install.openshift.io/ai-deprovision` finalizer block any new cluster that selects them.
