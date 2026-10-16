# Cluster lifecycle

Add `hub` to a cluster's fleet file and that hub builds the cluster. Set `state: absent` and the hub destroys it. Once ACM imports the cluster, the same fleet file drives its Day-2 configuration like any other cluster ([Fleet files](applicationset.md#fleet-files)).

How it works:

1. The hub's ApplicationSet `policystack-lifecycle` creates Application `lifecycle-<cluster>` for each fleet file that names the hub.
2. The Application renders the [`lifecycle/`](https://github.com/PolicyStack/PolicyStack/tree/main/lifecycle) chart into Policy `present-lifecycle-<cluster>`, placed on the hub itself. A managed hub imported into a Global Hub in default mode does not manage itself, so its parent renders and places the Policy for it ([Global Hub](global-hub.md#lifecycle)).
3. ACM governance on the hub creates the cluster's objects one step at a time, each step waiting for the one before it.
4. When the install finishes, ACM imports the cluster and the Day-2 ApplicationSet takes over. No manual step.

The cluster is built when `present-lifecycle-<cluster>` is Compliant. Nothing outside ACM and Argo CD runs.

## Pick an engine

`install.engine` picks what builds the cluster. `install.platform` picks the HyperShift platform.

| You want | `engine` | Example | Upgraded by |
|---|---|---|---|
| OpenShift on AWS, GCP or Azure | `hive` | [AWS](#aws), [GCP](#gcp), [Azure](#azure) | `upgrade` on the cluster |
| OpenShift on bare metal with a BMC | `siteconfig` | [Bare metal](#bare-metal) | `upgrade` on the cluster |
| Hosted control plane, workers as VMs on the hub | `hypershift` (default), `platform: KubeVirt` | [KubeVirt](#kubevirt) | The hub |
| Hosted control plane, bare-metal workers | `hypershift`, `platform: Agent` | [Agent](#agent) | The hub |
| Hosted control plane, workers you boot yourself | `hypershift`, `platform: None` | [None](#none) | The hub |

A hosted control plane runs as pods on the hub. `hive` and `siteconfig` build standalone clusters.

## Prerequisites

On every hub that builds clusters:

- ACM 2.16 or later.
- The hub manages itself: its ManagedCluster has the `local-cluster` label, the default. Or it is a default-mode Global Hub managed hub, and the global hub runs PolicyStack.
- `fleet/hubs/<hubName>.yaml` exists for the hub, or `fleet/<hubName>.yaml` for a default-mode managed hub.
- For `hypershift`: the hypershift-addon on the hub, on by default. Check with `oc get managedclusteraddon hypershift-addon -n local-cluster`.
- A credential Secret on the hub with `pullSecret` and `ssh-publickey`, plus the cloud keys for `hive`. Each example below shows the command. To show it under Infrastructure > Credentials in the console, add the labels `cluster.open-cluster-management.io/credentials=` and `cluster.open-cluster-management.io/type=<type>`.

Each example lists what else its engine needs. The fleet files are complete: copy one, change the names and addresses, and merge it. [Values](#values) shows how to move the common parts into a values layer.

## Examples

### AWS

Needs a public Route 53 zone for `baseDomain` in the account, and EC2 quota.

```sh
oc create namespace hive-credentials
oc create secret generic aws -n hive-credentials \
  --from-literal=aws_access_key_id=<key> --from-literal=aws_secret_access_key=<secret> \
  --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub
```

```yaml
# fleet/ocp-aws.yaml
revision: main
hub: acm-dc1
version: 4.20.8
channel: stable-4.20
install:
  engine: hive
  credential:
    namespace: hive-credentials
    name: aws
  baseDomain: aws.example.com
  hive:
    platform:
      aws:
        region: us-east-2
    installConfig:
      controlPlane:
        name: master
        architecture: amd64
        replicas: 3
        platform:
          aws:
            type: m6i.xlarge
      compute:
        - name: worker
          architecture: amd64
          replicas: 3
          platform:
            aws:
              type: m6i.xlarge
```

`installConfig` is any [install-config](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/installing_on_aws/) content. The chart sets `apiVersion`, `metadata`, `baseDomain` and `platform`, Hive adds `pullSecret`, and `sshKey` defaults to the credential's. Keep `architecture: amd64`: without it the install follows the architecture of the hub node it runs on. For single-node OpenShift, set `controlPlane.replicas: 1` with `type: m6i.2xlarge`, and `compute[0].replicas: 0`.

### GCP

Needs a public Cloud DNS zone for `baseDomain` in the project, a service account with the [installer's roles](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/installing_on_gcp/) or Owner, and the compute, dns, iam, cloudresourcemanager and serviceusage APIs enabled.

```sh
oc create secret generic gcp -n hive-credentials \
  --from-file=osServiceAccount.json=sa-key.json \
  --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub
```

```yaml
# fleet/ocp-gcp.yaml
revision: main
hub: acm-dc1
version: 4.20.8
channel: stable-4.20
install:
  engine: hive
  credential:
    namespace: hive-credentials
    name: gcp
  baseDomain: gcp.example.com
  hive:
    platform:
      gcp:
        projectID: my-project
        region: us-central1
    installConfig:
      controlPlane:
        name: master
        architecture: amd64
        replicas: 3
        platform:
          gcp:
            type: n2-standard-4
      compute:
        - name: worker
          architecture: amd64
          replicas: 3
          platform:
            gcp:
              type: n2-standard-4
```

### Azure

Needs an Azure DNS zone for `baseDomain` in resource group `baseDomainResourceGroupName`, and a service principal with Contributor and User Access Administrator on the subscription.

```sh
cat > sp.json <<EOF
{"subscriptionId":"<sub>","clientId":"<appId>","clientSecret":"<password>","tenantId":"<tenant>"}
EOF
oc create secret generic azure -n hive-credentials \
  --from-file=osServicePrincipal.json=sp.json \
  --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub
```

```yaml
# fleet/ocp-azure.yaml
revision: main
hub: acm-dc1
version: 4.20.8
channel: stable-4.20
install:
  engine: hive
  credential:
    namespace: hive-credentials
    name: azure
  baseDomain: azure.example.com
  hive:
    platform:
      azure:
        region: eastus
        baseDomainResourceGroupName: dns-rg
    installConfig:
      controlPlane:
        name: master
        architecture: amd64
        replicas: 3
        platform:
          azure:
            type: Standard_D8s_v5
      compute:
        - name: worker
          architecture: amd64
          replicas: 3
          platform:
            azure:
              type: Standard_D4s_v5
```

### Bare metal

Needs:

- SiteConfig on the hub: `stack.acm.config.siteConfig: true` in the hub's values layer.
- Central infrastructure management: an AgentServiceConfig named `agent`, and the Provisioning `provisioning-configuration` with `watchAllNamespaces: true` (plus `provisioningNetwork: Disabled` on a hub without one). See [Enabling central infrastructure management](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/).
- Hosts with a Redfish BMC that supports virtual media, reachable from the hub. Each host must reach the hub's image service and API.
- DNS for `api.<cluster>.<baseDomain>`, resolvable from the hosts and the hub. Multi-node clusters also need `api-int` and `*.apps`.

```sh
oc create namespace bm-credentials
oc create secret generic hostinventory -n bm-credentials \
  --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub
oc create secret generic bmc -n bm-credentials --from-literal=username=<user> --from-literal=password=<password>
```

Single node:

```yaml
# fleet/sno-edge.yaml
revision: main
hub: acm-dc1
version: 4.20.8
channel: stable-4.20
install:
  engine: siteconfig
  credential:
    namespace: bm-credentials
    name: hostinventory
  baseDomain: edge.example.com
  siteconfig:
    bmcCredential:
      namespace: bm-credentials
      name: bmc
    clusterInstance:
      clusterType: SNO
      networkType: OVNKubernetes
      machineNetwork:
        - cidr: 10.20.30.0/24
    nodes:
      sno-edge.edge.example.com:
        role: master
        bmcAddress: idrac-virtualmedia://10.20.0.11/redfish/v1/Systems/System.Embedded.1
        bootMACAddress: "00:11:22:33:44:55"
        rootDeviceHints:
          deviceName: /dev/sda
```

Three nodes: set `clusterType: HighlyAvailable`, add `apiVIPs` and `ingressVIPs`, and list each host under `nodes`:

```yaml
install:
  siteconfig:
    clusterInstance:
      clusterType: HighlyAvailable
      networkType: OVNKubernetes
      machineNetwork:
        - cidr: 10.20.40.0/24
      apiVIPs: [10.20.40.5]
      ingressVIPs: [10.20.40.6]
    nodes:
      m0.bm-ha.dc1.example.com:
        role: master
        bmcAddress: redfish-virtualmedia://10.20.0.21/redfish/v1/Systems/1
        bootMACAddress: "00:11:22:33:44:01"
      m1.bm-ha.dc1.example.com: ...
      m2.bm-ha.dc1.example.com: ...
```

Use `idrac-virtualmedia://` for Dell and `redfish-virtualmedia://` for other vendors. Quote `bootMACAddress`. `clusterInstance` takes any [ClusterInstance](https://github.com/stolostron/siteconfig/blob/release-2.16/api/v1alpha1/clusterinstance_types.go) spec field, and each `nodes` entry any node field.

Image-based install: set `siteconfig.templates: ibi`. It is single node only, needs the multicluster engine `image-based-install-operator` component, and the host must be preinstalled with a seed image whose version is `version`.

### KubeVirt

Needs OpenShift Virtualization on the hub, and a LoadBalancer implementation such as MetalLB: the hosted API server is published as a `LoadBalancer` Service on the hub. The credential may already exist from OpenShift Virtualization setup.

```sh
oc create secret generic kubevirt-secret -n open-cluster-management \
  --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub
```

```yaml
# fleet/hcp-kubevirt.yaml
revision: main
hub: acm-dc1
version: 4.20.8
channel: stable-4.20
install:
  platform: KubeVirt
  credential:
    namespace: open-cluster-management
    name: kubevirt-secret
  nodePools:
    workers:
      replicas: 2
```

`baseDomain` is optional on KubeVirt: HyperShift uses the hub's ingress domain when it is empty.

### Agent

Needs:

- Central infrastructure management (an AgentServiceConfig), as for [bare metal](#bare-metal).
- An InfraEnv in `agentNamespace`, and hosts booted from its discovery ISO, approved, and labeled to match the NodePool's `agentLabelSelector`. A pool without a selector claims any free Agent in the namespace.
- A LoadBalancer implementation on the hub, as for KubeVirt.
- DNS for `*.apps.<cluster>.<baseDomain>` pointing at the guest's ingress. [`values/clusters/hcp-agent.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values/clusters/hcp-agent.yaml) pins it with the `metallb` element.

```sh
oc create namespace hcp-credentials
oc create secret generic hostinventory -n hcp-credentials \
  --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub
```

```yaml
# fleet/hcp-agent.yaml
revision: main
hub: acm-dc1
version: 4.20.8
channel: stable-4.20
install:
  platform: Agent
  credential:
    namespace: hcp-credentials
    name: hostinventory
  baseDomain: dc1.example.com
  agentNamespace: hardware-inventory
  nodePools:
    workers:
      replicas: 2
      platform:
        agent:
          agentLabelSelector:
            matchLabels:
              example.com/cluster: hcp-agent
```

### None

HyperShift creates no machines. You boot each worker with the NodePool's Ignition and approve it. Use this when the workers are VMs or hosts that ACM cannot provision.

Needs the same credential as Agent, a LoadBalancer implementation on the hub, and DNS for `*.apps.<cluster>.<baseDomain>` pointing at the workers. The guest router listens on the workers' host network, ports 80 and 443.

```yaml
# fleet/hcp-none.yaml
revision: main
hub: acm-dc1
version: 4.20.8
channel: stable-4.20
install:
  platform: None
  credential:
    namespace: hcp-credentials
    name: hostinventory
  baseDomain: dc1.example.com
  nodePools:
    workers: {}
```

Once the HostedCluster is up, add a worker:

```sh
# Ignition pointer config with the ignition server URL, token and CA
oc get secret -n clusters-hcp-none -o name | grep user-data-hcp-none-workers \
  | xargs oc get -n clusters-hcp-none -o jsonpath='{.data.value}' | base64 -d > worker.ign
# Boot RHCOS with worker.ign as its Ignition or cloud user data, then approve its CSRs twice,
# a minute apart: client, then serving certificate
oc extract secret/hcp-none-admin-kubeconfig -n clusters --to=- > hcp-none.kubeconfig
oc --kubeconfig hcp-none.kubeconfig get csr -o name | xargs oc --kubeconfig hcp-none.kubeconfig adm certificate approve
```

HyperShift's default service network is `172.31.0.0/16`, the AWS default VPC range. On AWS workers set `hostedCluster.spec.networking.serviceNetwork` to another range at create. A single worker also needs `hostedCluster.spec.infrastructureAvailabilityPolicy: SingleReplica`.

## Fleet keys

| Key | Default | Effect |
|---|---|---|
| `hub` | none | `hubName` of the hub that builds and owns the cluster ([The hub](applicationset.md#the-hub)). Without it the cluster is import-only. |
| `state` | `present` | `present` builds the cluster and keeps it as declared. `absent` destroys it ([Destroy](#destroy)). Requires `hub`. |
| `version` | none | OpenShift release, `x.y.z`. Required. Bumping it upgrades the cluster ([Upgrades](#upgrades)). |
| `channel` | none | Update channel, such as `stable-4.20`. Optional. |
| `install` | none | What to build. Requires `hub`. Every key is in the [reference](lifecycle-reference.md#install-keys). |

The cluster name is the fleet file name: a DNS label of at most 30 characters, the most every element fits. The lifecycle chart alone allows 38 ([Naming limits](policies.md#naming-limits)). Most `install` keys are fixed at create ([Immutable fields](lifecycle-reference.md#immutable-fields)).

## Values

The lifecycle Application reads the same cascade as the cluster's Day-2 Applications, lowest precedence first ([Order](values.md#order)):

1. `lifecycle/values.yaml`
2. `values.yaml`
3. Each `valueFiles` entry, in list order
4. `values/clusters/<cluster>.yaml`
5. `hub`, `state`, `version`, `channel` and `install` from the fleet file

Move what clusters share into a layer. These ship with defaults:

| Layer | Sets |
|---|---|
| `values/platforms/aws.yaml` | `engine: hive`, credential `hive-credentials/aws`, a 3+3 `m6i.xlarge` install-config |
| `values/platforms/baremetal.yaml` | `engine: siteconfig`, credentials `bm-credentials/hostinventory` and `bm-credentials/bmc`, `networkType` |
| `values/platforms/kubevirt.yaml` | `platform: KubeVirt`, credential `open-cluster-management/kubevirt-secret`, 2 workers |
| `values/platforms/agent.yaml` | `platform: Agent`, credential `hcp-credentials/hostinventory`, `agentNamespace: hardware-inventory` |

With the AWS layer, the [AWS example](#aws) shrinks to:

```yaml
# fleet/ocp-aws.yaml
revision: main
hub: acm-dc1
version: 4.20.8
channel: stable-4.20
valueFiles:
  - platforms/aws.yaml
install:
  baseDomain: aws.example.com
  hive:
    platform:
      aws:
        region: us-east-2
```

Maps merge across layers and lists replace ([Merge rules](values.md#merge-rules)): an `installConfig.compute` list in the fleet file replaces the layer's whole list. `version` and `channel` go only in the fleet file. A shared layer is part of every cluster that reads it, so changing a fixed-at-create key there, such as `baseDomain`, turns every existing cluster that reads it NonCompliant.

`values/clusters/<cluster>.yaml` can hold both `install` for the build and `stack` for the cluster's elements.

## Watching a build

| Engine | Progress |
|---|---|
| `hive` | `oc get clusterdeployment -n <cluster>`, and the installer log: `oc logs -f -n <cluster> -l hive.openshift.io/install=true -c installer`. About 45 minutes on AWS. |
| `siteconfig` | `oc get clusterinstance,bmh,agentclusterinstall,agent -n <cluster>`. About an hour for single node. |
| `hypershift` | `oc get hostedcluster,nodepool -n clusters` |

For every engine:

```sh
oc get policy -n pstack present-lifecycle-<cluster>                 # Compliant = built
oc get configurationpolicy -n local-cluster | grep <cluster>        # each step; -n <managed-hub> in default mode
oc get applications.argoproj.io lifecycle-<cluster> -n openshift-gitops -o jsonpath='{.status.conditions}'   # render errors
```

[Policy chains](lifecycle-reference.md#policy-chains) lists every step.

## Upgrades

Bump `version` in the fleet file, and `channel` with it on a minor bump ([Upgrades](upgrades.md)).

- `hive` and `siteconfig` clusters upgrade on the cluster through `upgrade`, like imported ones. The hub keeps the release image the cluster was installed with, since Hive and SiteConfig reject changes to it.
- `hypershift` clusters upgrade from the hub. With `channel` set, the HostedCluster waits until HyperShift lists `version` in `status.version.availableUpdates`. Without `channel`, it moves at once. NodePools follow once the control plane reports the new version Completed. `KubeVirt` pools replace their VMs; other platforms upgrade nodes in place.

During a hosted upgrade `present-lifecycle-<cluster>` is NonCompliant, and Compliant again once the control plane and every NodePool report the new version. A minor upgrade that HyperShift blocks because `ClusterVersionUpgradeable` is False needs the annotation `hypershift.openshift.io/force-upgrade-to: <release image>` in `install.hostedCluster.metadata.annotations`.

Clusters install from the `-multi` release payload, so they are multi-architecture clusters and their updates are too.

## Destroy

Set `state: absent` in the fleet file and merge:

```diff
 # fleet/ocp-aws.yaml
 revision: main
 hub: acm-dc1
+state: absent
```

The hub removes the cluster from ACM, deletes it, waits until it is gone, then deletes the secrets and `install.manifests` it created. Detaching deletes the cluster's Day-2 Applications and Policies without pruning anything on the cluster. Wait for the Policy, then delete the fleet file and `values/clusters/<cluster>.yaml`:

```sh
oc get policy -n pstack absent-lifecycle-<cluster>
```

What is left:

| Engine | After destroy |
|---|---|
| `hive` | Nothing in the cloud account. Hive deprovisions it, usually within 20 minutes. |
| `siteconfig` | The hosts keep running and their disks are not wiped. Power them off or reuse them. |
| `hypershift` Agent | The Agents return to the InfraEnv, free for the next cluster. |
| `hypershift` None | The workers you booted. Delete them, and their DNS. |

Deleting the fleet file before the Policy is Compliant abandons the cluster at that step.

## Abandon

Deleting the fleet file, or removing its `hub` key, deletes the Application `lifecycle-<cluster>` and its Policies. The cluster keeps running, unmanaged. Deleting the ApplicationSets does the same for every cluster the hub built; `helm uninstall policystack` keeps them ([Install PolicyStack](install.md#install-policystack)).

To re-adopt, restore the file or the key with the same `install` values.

Moving a cluster to another hub is not supported. Changing `hub` abandons it on the old hub and builds a second cluster with the same name on the new one. Destroy it on the old hub first.

## Reference

[Lifecycle reference](lifecycle-reference.md) has every `install` key, the objects each engine creates, the policy chains, ownership, immutable fields, troubleshooting, and how to add an engine.
