# Cluster lifecycle

Add `hub` to a fleet file and that hub builds the cluster. Set `state: absent` and it destroys it. After import, the same fleet file drives Day-2 ([Fleet files](../reference/fleet.md)).

1. ApplicationSet `policystack-lifecycle` creates Application `lifecycle-<cluster>` for each fleet file naming the hub.
2. It renders the [`lifecycle/`](https://github.com/PolicyStack/PolicyStack/tree/main/lifecycle) chart into Policy `present-lifecycle-<cluster>`, placed on the hub. For a default-mode Global Hub managed hub, the parent does this ([Global Hub](global-hub.md#lifecycle)).
3. ACM creates the cluster's objects in order, each step waiting for the last.
4. ACM imports the finished cluster and the Day-2 ApplicationSet takes over ([Day-2 handoff](../concepts/architecture.md#day-2-handoff)).

The cluster is built when `present-lifecycle-<cluster>` is Compliant. Only ACM and Argo CD run. Every key and object: [Lifecycle keys](../reference/lifecycle.md).

## Pick an engine

| You want | `install.engine` | `install.platform` | Upgraded by |
|---|---|---|---|
| OpenShift on AWS, GCP, Azure or vSphere | `hive` | | `upgrade` on the cluster |
| OpenShift on bare metal with a BMC | `siteconfig` | | `upgrade` on the cluster |
| Hosted control plane, workers as VMs on the hub | `hypershift` (default) | `KubeVirt` | The hub |
| Hosted control plane, bare-metal workers | `hypershift` | `Agent` | The hub |
| Hosted control plane, workers you boot yourself | `hypershift` | `None` | The hub |

`hypershift` runs the control plane as pods on the hub. `hive` and `siteconfig` build standalone clusters.

## Prerequisites

On every hub that builds clusters:

- ACM 2.16 or later.
- The hub manages itself (`local-cluster` label, the default), or is a default-mode Global Hub managed hub with PolicyStack on the global hub.
- `fleet/hubs/<hubName>.yaml` exists.
- `hypershift`: the hypershift-addon, on by default (`oc get managedclusteraddon hypershift-addon -n local-cluster`).
- A credential Secret with `pullSecret`, `ssh-publickey` and, for `hive`, cloud keys (vSphere: `username`, `password`, `cacertificate`). Labels `cluster.open-cluster-management.io/credentials=` and `cluster.open-cluster-management.io/type=<type>` show it in the console.

## Examples

Copy one, change names and addresses, merge. [Values](#values) moves shared parts into a layer.

=== "AWS"

    Needs a public Route 53 zone for `baseDomain` and EC2 quota.

    ```sh
    oc create namespace hive-credentials
    oc create secret generic aws -n hive-credentials \
      --from-literal=aws_access_key_id=<key> --from-literal=aws_secret_access_key=<secret> \
      --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub
    ```

    ```yaml
    # fleet/ocp-aws.yaml
    revision: main
    hub: acm-dc1 # (1)!
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
        installConfig: # (2)!
          controlPlane:
            name: master
            architecture: amd64 # (3)!
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

    1.  `hubName` of the hub that builds the cluster.
    2.  Any [install-config](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/installing_on_aws/) content. Set for you: `apiVersion`, `metadata`, `baseDomain`, `platform`, `pullSecret`, and `sshKey` from the credential.
    3.  Keep it, or the install follows the hub node's architecture.

    Single node: `controlPlane.replicas: 1` with `type: m6i.2xlarge`, and `compute[0].replicas: 0`.

=== "GCP"

    Needs a public Cloud DNS zone for `baseDomain`, a service account with the [installer's roles](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/installing_on_gcp/) or Owner, and the compute, dns, iam, cloudresourcemanager and serviceusage APIs.

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

=== "Azure"

    Needs an Azure DNS zone for `baseDomain` in `baseDomainResourceGroupName`, and a service principal with Contributor and User Access Administrator on the subscription.

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

=== "vSphere"

    Needs vCenter and ESXi 7.0.2+, vCenter on port 443, DNS for `api.<cluster>.<baseDomain>` and `*.apps.<cluster>.<baseDomain>` pointing at the VIPs, and DHCP on the network unless `hosts` sets static IPs. Inventory paths are absolute. More than one failure domain needs the `openshift-region` and `openshift-zone` tag categories attached to the datacenters and clusters. The hub downloads the RHCOS OVA unless `topology.template` names one.

    `cacertificate` is the vCenter CA in PEM, from `https://<vcenter>/certs/download.zip` (`certs/lin/*.0`). It is required even for a publicly trusted vCenter. Use `--from-literal`: a trailing newline in the login fails the install.

    ```sh
    oc create secret generic vsphere -n hive-credentials \
      --from-literal=username=<user> --from-literal=password=<password> \
      --from-file=cacertificate=vcenter-ca.pem \
      --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub
    ```

    ```yaml
    # fleet/ocp-vsphere.yaml
    revision: main
    hub: acm-dc1
    version: 4.20.8
    channel: stable-4.20
    install:
      engine: hive
      credential:
        namespace: hive-credentials
        name: vsphere
      baseDomain: vsphere.example.com
      hive:
        platform:
          vsphere: # (1)!
            apiVIPs: [10.0.10.5]
            ingressVIPs: [10.0.10.6]
            vcenters:
              - server: vcenter.example.com # (2)!
                datacenters: [dc1]
            failureDomains:
              - name: dc1-cl1
                region: dc1
                zone: cl1
                server: vcenter.example.com
                topology:
                  datacenter: dc1
                  computeCluster: /dc1/host/cl1
                  datastore: /dc1/datastore/ds1
                  networks: [ocp-vlan10]
        installConfig:
          controlPlane:
            name: master
            architecture: amd64
            replicas: 3
          compute:
            - name: worker
              architecture: amd64
              replicas: 3
          networking:
            machineNetwork:
              - cidr: 10.0.10.0/24 # (3)!
    ```

    1. The installer's `platform.vsphere`, also the ClusterDeployment's `infrastructure`. Fixed at create.
    2. Every vCenter logs in with the credential. `user` and `password` here are refused.
    3. Holds the node IPs and both VIPs.

    ??? example "Static IPs, two failure domains"
        `hosts` lists exactly one `bootstrap`, then one entry per control-plane and compute replica.

        ```yaml
        vsphere:
          apiVIPs: [10.0.10.5]
          ingressVIPs: [10.0.10.6]
          vcenters:
            - {server: vcenter.example.com, datacenters: [dc1]}
          failureDomains:
            - {name: dc1-cl1, region: dc1, zone: cl1, server: vcenter.example.com,
               topology: {datacenter: dc1, computeCluster: /dc1/host/cl1, datastore: /dc1/datastore/ds1, networks: [ocp-vlan10]}}
            - {name: dc1-cl2, region: dc1, zone: cl2, server: vcenter.example.com,
               topology: {datacenter: dc1, computeCluster: /dc1/host/cl2, datastore: /dc1/datastore/ds2, networks: [ocp-vlan10]}}
          hosts:
            - {role: bootstrap, networkDevice: {ipAddrs: [10.0.10.10/24], gateway: 10.0.10.1, nameservers: [10.0.0.53]}}
            - {role: control-plane, failureDomain: dc1-cl1, networkDevice: {ipAddrs: [10.0.10.11/24], gateway: 10.0.10.1, nameservers: [10.0.0.53]}}
            - {role: control-plane, failureDomain: dc1-cl2, networkDevice: {ipAddrs: [10.0.10.12/24], gateway: 10.0.10.1, nameservers: [10.0.0.53]}}
            - {role: control-plane, failureDomain: dc1-cl1, networkDevice: {ipAddrs: [10.0.10.13/24], gateway: 10.0.10.1, nameservers: [10.0.0.53]}}
            - {role: compute, failureDomain: dc1-cl1, networkDevice: {ipAddrs: [10.0.10.21/24], gateway: 10.0.10.1, nameservers: [10.0.0.53]}}
            - {role: compute, failureDomain: dc1-cl2, networkDevice: {ipAddrs: [10.0.10.22/24], gateway: 10.0.10.1, nameservers: [10.0.0.53]}}
            - {role: compute, failureDomain: dc1-cl1, networkDevice: {ipAddrs: [10.0.10.23/24], gateway: 10.0.10.1, nameservers: [10.0.0.53]}}
        ```

    !!! warning "Static IPs and Day-2 nodes"
        With `hosts`, the installer creates the workers as standalone Machines and leaves the worker MachineSets at 0, drawing addresses from an IPPool. Nothing installs an IPAM controller for it, so Machines from scaling, `infra-nodes` or `storage-nodes` wait for an address.

    !!! note
        Hive cannot hibernate vSphere clusters: leave `powerState` unset. After rotating the vCenter password, update the credential and the cluster's `kube-system/vsphere-creds`.

=== "Bare metal"

    Needs:

    - `stack.cim.enabled: true` in the hub's values layer. It manages the Provisioning CR, AgentServiceConfig `agent` and the MultiClusterHub `siteconfig` component, and `ready-cim-<hub>` reports when they run. Yourself: a default StorageClass (or `config.storage.storageClass`), [the bare-metal ports](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/), and the NLB on an AWS hub.
    - Hosts with a virtual-media BMC the hub reaches. Hosts reach the hub's image service and API.
    - DNS for `api.<cluster>.<baseDomain>` from hosts and hub. Multi-node adds `api-int` and `*.apps`.
    - Disconnected: mirrors and RHCOS images on the hub ([Bare metal](disconnected.md#bare-metal)).

    ```sh
    oc create namespace bm-credentials
    oc create secret generic hostinventory -n bm-credentials \
      --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub # (1)!
    oc create secret generic bmc -n bm-credentials --from-literal=username=<user> --from-literal=password=<password>
    ```

    1.  One key: the stock templates fold a second into the first, so the step fails on it. RSA or ECDSA with `siteconfig.fips`.

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
        bmcCredential: # (1)!
          namespace: bm-credentials
          name: bmc
        machineNetwork: [10.20.30.0/24]
        nodes: # (2)!
          sno-edge.edge.example.com:
            bmcAddress: idrac-virtualmedia://10.20.0.11/redfish/v1/Systems/System.Embedded.1 # (3)!
            bootMACAddress: "00:11:22:33:44:55" # (4)!
            rootDeviceHints:
              deviceName: /dev/sda
    ```

    1.  `username` and `password`, used for every node.
    2.  Keyed by host name. [All keys](../reference/lifecycle.md#install-keys).
    3.  `idrac-virtualmedia://` for Dell, `ilo5-` or `ilo4-virtualmedia://` for HPE, `redfish-virtualmedia://` otherwise ([all variants](../reference/lifecycle.md#install-keys)). Other schemes need a provisioning network, which the [`cim`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/cim) default doesn't have, so the schema refuses them.
    4.  Keep the quotes: an unquoted MAC is not read as a string by YAML.

    !!! warning "Fixed after create"
        SiteConfig allows no spec change until the install completes, and after it only adding and removing workers. A failed install is destroyed and rebuilt. Check addresses and networks before merging.

    !!! warning "BMC certificates"
        The stock BareMetalHost template hard-codes `disableCertificateVerification: true`, with no setting to change it. Keep BMCs on a restricted network.

    ??? example "Three nodes"
        One entry per host; `HighlyAvailable` follows from the masters:

        ```yaml
        install:
          siteconfig:
            machineNetwork: [10.20.40.0/24]
            apiVIPs: [10.20.40.5]
            ingressVIPs: [10.20.40.6]
            nodes:
              m0.bm-ha.dc1.example.com:
                bmcAddress: redfish-virtualmedia://10.20.0.21/redfish/v1/Systems/1
                bootMACAddress: "00:11:22:33:44:01"
              m1.bm-ha.dc1.example.com: ...
              m2.bm-ha.dc1.example.com: ...
        ```

    ??? info "Image-based install"
        `siteconfig.templates: ibi`. Single node only. Needs the multicluster engine `image-based-install-operator` component, which you enable yourself, and a host preinstalled with a seed image at `version`.

    See [Scaling](#scaling-siteconfig-workers) to add and remove workers.

=== "KubeVirt"

    Needs OpenShift Virtualization and a LoadBalancer implementation such as MetalLB on the hub; the hosted API server is a `LoadBalancer` Service.

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
      platform: KubeVirt # (1)!
      credential:
        namespace: open-cluster-management
        name: kubevirt-secret
      nodePools:
        workers: # (2)!
          replicas: 2
    ```

    1.  `engine` defaults to `hypershift`. Without `baseDomain`, HyperShift uses the hub's ingress domain.
    2.  NodePool `hcp-kubevirt-workers`, merged over the generated spec.

=== "Agent"

    Needs:

    - An AgentServiceConfig (`stack.cim`, as for bare metal) and a LoadBalancer implementation, as for KubeVirt.
    - An InfraEnv in `agentNamespace`, with hosts booted from its ISO, approved, and labeled to match `agentLabelSelector`.
    - DNS for `*.apps.<cluster>.<baseDomain>` at the guest ingress. [`values/clusters/hcp-agent.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values/clusters/hcp-agent.yaml) pins it with the `metallb` element.

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
              agentLabelSelector: # (1)!
                matchLabels:
                  example.com/cluster: hcp-agent
    ```

    1.  A pool without a selector claims any free Agent in `agentNamespace`.

=== "None"

    No machines are created: you boot each worker from the NodePool's Ignition. For VMs or hosts ACM cannot provision. Needs the Agent credential, a hub LoadBalancer, and DNS for `*.apps.<cluster>.<baseDomain>` at the workers (router on host ports 80/443).

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

    Add a worker once the HostedCluster is up:

    ```sh
    # Ignition pointer config with the ignition server URL, token and CA
    oc get secret -n clusters-hcp-none -o name | grep user-data-hcp-none-workers \
      | xargs oc get -n clusters-hcp-none -o jsonpath='{.data.value}' | base64 -d > worker.ign
    # Boot RHCOS with worker.ign as Ignition or cloud user data, then approve its CSRs twice,
    # a minute apart: client, then serving certificate
    oc extract secret/hcp-none-admin-kubeconfig -n clusters --to=- > hcp-none.kubeconfig
    oc --kubeconfig hcp-none.kubeconfig get csr -o name | xargs oc --kubeconfig hcp-none.kubeconfig adm certificate approve
    ```

    !!! warning "Workers on AWS"
        The default service network `172.31.0.0/16` is the AWS default VPC range. Set `hostedCluster.spec.networking.serviceNetwork` to another range at create. A single worker also needs `hostedCluster.spec.infrastructureAvailabilityPolicy: SingleReplica`.

## Fleet keys

`hub`, `state`, `version`, `channel` and `install` are listed in [Fleet files](../reference/fleet.md); `install` takes the [Install keys](../reference/lifecycle.md#install-keys). Most `install` keys are fixed at create ([Immutable fields](../reference/lifecycle.md#immutable-fields)): changing one turns the Policy NonCompliant and never rebuilds the cluster.

The cluster name is the fleet file name: a DNS label of at most 30 characters, so every element fits (lifecycle alone allows 38, [Naming limits](../concepts/policies.md#naming-limits)).

## Values

Same [values cascade](../concepts/values.md#order) as Day-2, starting from `lifecycle/values.yaml`. The fleet file's `install` ranks above every layer.

Shipped layers:

| Layer | Sets |
|---|---|
| `values/platforms/aws.yaml` | `engine: hive`, credential `hive-credentials/aws`, 3+3 `m6i.xlarge`, `networking` |
| `values/platforms/vmware.yaml` | `engine: hive`, credential `hive-credentials/vsphere`, 3+3 amd64 at installer default size |
| `values/platforms/baremetal.yaml` | `engine: siteconfig`, credentials `bm-credentials/hostinventory` and `bm-credentials/bmc` |
| `values/platforms/kubevirt.yaml` | `platform: KubeVirt`, credential `open-cluster-management/kubevirt-secret`, 2 workers |
| `values/platforms/agent.yaml` | `platform: Agent`, credential `hcp-credentials/hostinventory`, `agentNamespace: hardware-inventory` |

The AWS example with its layer:

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

`version` and `channel` belong only in the fleet file. `values/clusters/<cluster>.yaml` may hold both `install` and `stack`.

!!! warning "Shared layers"
    Maps merge, lists replace ([Merge rules](../concepts/values.md#merge-rules)): `installConfig.compute` in the fleet file replaces the layer's list. Changing a fixed-at-create key like `baseDomain` in a layer turns every existing cluster reading it NonCompliant.

## Watching a build

```sh
oc get policy -n pstack present-lifecycle-<cluster>             # Compliant = built
oc get configurationpolicy -n local-cluster | grep <cluster>    # each step; -n <managed-hub> in default mode
oc get applications.argoproj.io lifecycle-<cluster> -n openshift-gitops -o jsonpath='{.status.conditions}'   # render errors
```

| Engine | Progress | Typical time |
|---|---|---|
| `hive` | `oc get clusterdeployment -n <cluster>`<br>`oc logs -f -n <cluster> -l hive.openshift.io/install=true -c installer` | 45 min on AWS |
| `siteconfig` | `oc get clusterinstance,bmh,agentclusterinstall,agent -n <cluster>` | 1 h single node |
| `hypershift` | `oc get hostedcluster,nodepool -n clusters` | |

Every step is in [Policy chains](../reference/lifecycle.md#policy-chains).

## Upgrades

Bump `version`, and `channel` on a minor bump ([OpenShift upgrades](upgrades.md)):

```diff
 # fleet/hcp-kubevirt.yaml
-version: 4.20.8
-channel: stable-4.20
+version: 4.21.2
+channel: stable-4.21
```

| Engine | How |
|---|---|
| `hive`, `siteconfig` | On the cluster by `upgrade`, like imported clusters. The hub keeps the install release image: Hive rejects changes to it, and the chart reads the `siteconfig` image set back from the live object. |
| `hypershift` | From the hub. With `channel`, waits until `version` is in `status.version.availableUpdates`; without, moves at once. A `version` below the control plane's desired version is never applied. NodePools follow once the control plane's newest Completed version is `version`. `KubeVirt` pools replace VMs; others upgrade in place. Disconnected, `channel` needs `install.hostedCluster.spec.updateService` pointing at your OSUS, or the bump waits forever ([Hosted clusters](disconnected.md#hosted-clusters)). |

During a `hypershift` upgrade, `present-lifecycle-<cluster>` is NonCompliant until the control plane and every NodePool reach `version`. A held downgrade, such as a reverted bump after the control plane moved, keeps it NonCompliant until the fleet file catches up. `hypershift` and `hive` clusters install from the `-multi` payload, so they are multi-architecture. `siteconfig` installs `-x86_64` and stays single-architecture.

??? info "Minor upgrade blocked by `ClusterVersionUpgradeable=False`"
    Force it with the release image:

    ```yaml
    install:
      hostedCluster:
        metadata:
          annotations:
            hypershift.openshift.io/force-upgrade-to: quay.io/openshift-release-dev/ocp-release:4.21.2-multi
    ```

## Scaling

Workers change after create through `nodePools` (`hypershift`) and `siteconfig.nodes` (`siteconfig`).

### Removing a NodePool

`musthave` never deletes. Removing an entry only stops managing it. Remove the `nodePools` entry, then:

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

### Scaling SiteConfig workers

!!! note
    Red Hat documents this flow for `clusterType: SNO`. On `HighlyAvailable`, SiteConfig accepts the same changes and its sample output shows it, but no doc covers it explicitly.

**Scale out.** Add a host with `role: worker`. It reaches the ClusterInstance once the install completes, so one-master clusters can list workers from the start. The hub needs a valid kubeconfig for the cluster; if the API certificate changed, [update it first](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/). Keep the `bmcCredential` Secret on the hub: without it `credentials` fails and `cluster` never applies the change. `present-ready` stays NonCompliant until the host's Agent reports `Done`, and names the host once its BareMetalHost exists.

**Scale in.** Set `remove: true` on the worker. Masters and arbiters cannot be removed.

```yaml
install:
  siteconfig:
    nodes:
      w0.bm-ha.dc1.example.com:
        role: worker
        bmcAddress: redfish-virtualmedia://10.20.0.24/redfish/v1/Systems/1
        bootMACAddress: "00:11:22:33:44:04"
        remove: true
```

The `cluster` step then runs SiteConfig's documented scale-in, each write gated on the last:

| Step | Written to the node | Waits for |
|---|---|---|
| 1 | `extraAnnotations` `remove-agent-and-node-on-delete` | The annotation on the BareMetalHost |
| 2 | `pruneManifests`: BareMetalHost, InfraEnv, NMStateConfig | BareMetalHost gone, SiteConfig done (`observedGeneration`) |
| 3 | Node dropped | |

BMAC drains the Node, deprovisions the host and removes the Agent and Node. Follow it with `oc get bmh,agent -n <cluster>`. Once `present-ready` is Compliant, delete the fleet entry. Cleaning is metadata only: data stays on the disks.

| You do | Result |
|---|---|
| Delete a live node's entry, or rename it | `cluster` fails with `not install.siteconfig.nodes`: SiteConfig would accept the drop and delete nothing, stranding the BareMetalHost, Agent and Node. |
| Add a master to a live cluster | `cluster` fails: `is a new master`. |
| Clear `remove` before step 2 | Scale-in cancelled. |
| Clear `remove` after step 2 | `cluster` fails: `cannot be cancelled`. |
| `remove` on a worker still joining | `cluster` fails: `has not finished joining`. Its drain would loop until SiteConfig's 30-minute timeout pauses the ClusterInstance. |
| `remove` on a worker whose Node you already deleted | Not detected: the hub cannot see spoke Nodes. The drain fails the same way, and the BareMetalHost's drain-status annotation shows `failed to drain node`. |

??? warning "Removing a worker that never joined, or whose Node is gone"
    Either finish the install, or annotate its BareMetalHost so the drain gives up. BMAC honours this annotation, but no Red Hat doc covers it:

    ```sh
    oc annotate bmh <host> -n <cluster> bmac.agent-install.openshift.io/drain-timeout=5m
    ```

    The value is a Go duration, counted from the first drain attempt. If the ClusterInstance has already paused, also remove its paused annotation.

## Destroy

Set `state: absent` and merge:

```diff
 # fleet/ocp-aws.yaml
 revision: main
 hub: acm-dc1
+state: absent
```

`hive` and `hypershift`: the hub detaches the cluster from ACM, deletes it, waits until it is gone, then deletes its secrets and `install.manifests`. Detaching drops the Day-2 Applications and Policies without pruning the cluster. `siteconfig` has no detach: the hub deletes the ClusterInstance, SiteConfig deletes the BareMetalHosts and then the ManagedCluster, and the hub deletes Namespace `<cluster>`, the ClusterImageSet and `install.manifests`. When the Policy is Compliant, delete the fleet file and `values/clusters/<cluster>.yaml`:

```sh
oc get policy -n pstack absent-lifecycle-<cluster>    # Compliant = gone
```

!!! warning
    Deleting the fleet file before `absent-lifecycle-<cluster>` is Compliant abandons the cluster mid-destroy.

!!! warning "ACS"
    Before re-creating a cluster under the same name, delete it in ACS ([Re-register a cluster](../elements/acs.md#re-register-a-cluster), step 1).

| Engine | Left behind |
|---|---|
| `hive` | Nothing. Hive deprovisions the cloud resources or VMs, usually within 20 minutes. vSphere: the DNS records you created. |
| `siteconfig` | Hosts still running, disks not wiped. Power off or reuse. A worker scaled in is powered off with its disk metadata cleaned; data stays on disk. |
| `hypershift` Agent | Agents return to the InfraEnv for the next cluster. |
| `hypershift` None | Your workers and their DNS. Delete them. |

## Abandon

Delete the fleet file or its `hub` key: Application `lifecycle-<cluster>` and its Policies go, the cluster keeps running unmanaged. Deleting the ApplicationSets does this for every cluster the hub built; `helm uninstall policystack` keeps them ([Install chart](../reference/install.md)). Restore the file or key with the same `install` to re-adopt.

!!! warning "Changing `hub`"
    Moving a cluster between hubs is not supported. Changing `hub` abandons it on the old hub, and the new hub builds a second, same-named cluster unless the root object is already there with its [marker](../reference/lifecycle.md#ownership). Destroy it on the old hub first. ACS: run steps 2-3 of [Re-register a cluster](../elements/acs.md#re-register-a-cluster) after the move.

## Hub disaster recovery

ACM backup leaves out PolicyStack's ApplicationSets and their Applications (`velero.io/exclude-from-backup`), so a passive hub builds nothing. Do not install PolicyStack on it before takeover. To take over:

1. Run the ACM activation restore (`veleroManagedClustersBackupName: latest`). ClusterDeployments, ClusterInstances and ManagedClusters come back with their annotations.
2. `hypershift`: restore the HostedClusters with the HCP OADP procedure. ACM backup does not include them, and without them the hub builds new, empty control planes.
3. [Install PolicyStack](../start/install.md#install-policystack) with the old hub's `hubName`. No fleet edits: the restored root objects carry the marker, so the new hub owns them.
