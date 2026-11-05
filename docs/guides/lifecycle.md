# Building clusters

You build, upgrade, scale and destroy OpenShift clusters by editing fleet files. Read [Cluster lifecycle](../concepts/lifecycle.md) first: it explains how a fleet file with `hub:` becomes a Policy on the hub that drives ACM's own installers, and which engine to pick ([Pick an engine](../concepts/lifecycle.md#pick-an-engine)). Every key and generated object is in [Lifecycle keys](../reference/lifecycle.md).

## Prerequisites

On every hub that builds clusters:

- ACM 2.16 or later.
- The hub manages itself: `oc get managedcluster -l local-cluster=true` returns one cluster (ACM creates it by default). Without it the hub's lifecycle ApplicationSet creates no `lifecycle-<cluster>` Application, and nothing reports an error. Exception: a hub imported into Multicluster Global Hub in default mode has no `local-cluster`; see [Global Hub](global-hub.md#lifecycle).
- `fleet/hubs/<hubName>.yaml` exists, the hub's own fleet file ([Add the hub's fleet file](../start/install.md#add-the-hubs-fleet-file)). Your fork's CI fails a cluster whose `hub` has no hub file (POLICY050), which catches a typo that would leave the cluster with no hub to build or destroy it. The hub file's `valueFiles` are also where hub-side settings go, such as `stack.cim` for bare metal.
- For `engine: hypershift`: the hypershift-addon is Available (`oc get managedclusteraddon hypershift-addon -n local-cluster` shows `AVAILABLE True`). It is on by default.

## Build a cluster

1. Create the credential Secret on the hub with `oc`, in any namespace, and never commit it. It holds `pullSecret`, `ssh-publickey` and, for `hive`, the cloud login; each example below shows the exact keys and command. The hub copies its keys into per-cluster Secrets on every Policy evaluation, so a rotated key reaches existing clusters without a merge. Destroy never deletes it.
2. Add `fleet/<cluster>.yaml` with `hub`, `version` and `install` ([Examples](#examples)). The file name is the cluster name. `install` can also come from any values layer ([Values](#values)).
3. Merge to the default branch. Fleet files are read only there ([Fleet files](../concepts/overview.md#fleet-files)). Within about 6 minutes `oc get applications.argoproj.io -n openshift-gitops lifecycle-<cluster>` shows the Application. It renders the `lifecycle/` chart at the fleet file's `revision` (branch, tag or SHA, [Revision](../reference/fleet.md#revision)), so `revision: main` follows the default branch.
4. Follow the build ([Watching a build](#watching-a-build)). The hub creates Policy `present-lifecycle-<cluster>`, whose steps run in order: guard, credentials, manifests, cluster, ready ([The step chain](../concepts/lifecycle.md#the-step-chain), with the full AWS render in [Worked example](../concepts/lifecycle.md#worked-example)).

!!! note "Cluster name"
    A DNS label of at most 30 characters. Day-2 element names are built from it and must stay under ACM's 63-character limit; CI fails a name that pushes any Policy past it (POLICY001, [Naming limits](../concepts/policies.md#naming-limits)).

!!! warning "Most install keys are fixed at create"
    Changing one later is refused, not rebuilt ([Immutable fields](../concepts/lifecycle.md#immutable-fields)). `install.hive.installConfig` raises no error: a change only rewrites the install-config Secret, and the running cluster ignores it.

!!! warning "Imported clusters cannot be adopted"
    Adding `hub` to the fleet file of a cluster that was imported, or built by hand or in the console, builds nothing. The `guard` step finds an object named `<cluster>` without the lifecycle [ownership marker](../concepts/lifecycle.md#ownership) and stays NonCompliant with `... not built by lifecycle (no lifecycle.policystk.io/hub annotation); refusing to build it`. Remove `hub`, `state` and `install` again; the cluster keeps its Day-2 config as an imported cluster.

## Examples

Copy one, change names and addresses, merge. [Values](#values) moves the shared parts into a layer.

=== "AWS"

    Needs a public Route 53 zone for `baseDomain` and EC2 quota.

    ```sh
    oc create namespace hive-credentials
    oc create secret generic aws -n hive-credentials \
      --from-literal=aws_access_key_id=<key> --from-literal=aws_secret_access_key=<secret> \
      --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub # (1)!
    ```

    1.  Optional: the labels `cluster.open-cluster-management.io/credentials=` and `cluster.open-cluster-management.io/type=<type>` list the Secret under Credentials in the console. Nothing else reads them.

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
        platform: # (2)!
          aws:
            region: us-east-2
        installConfig: # (3)!
          controlPlane:
            name: master
            architecture: amd64 # (4)!
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
    2.  Exactly one of `aws`, `gcp`, `azure` or `vsphere`. `hive` ignores `install.platform`, which is for `hypershift` only.
    3.  Any [install-config](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/installing_on_aws/) content. The chart sets `apiVersion`, `metadata.name`, `baseDomain` (from `install.baseDomain`) and `platform` (from `hive.platform`), and these win over the same keys here. Other keys under `platform` and `metadata` are merged in. Leave out `pullSecret`: Hive fills it in from the ClusterDeployment's pull secret. `sshKey` comes from the credential's `ssh-publickey` unless you set it here.
    4.  Keep it, or the install follows the hub node's architecture.

    Single node: in `installConfig`, set `controlPlane.replicas: 1` and `compute[0].replicas: 0`, and raise the control-plane type to `m6i.2xlarge`, because one node then also runs every workload.

=== "GCP"

    Needs a public Cloud DNS zone for `baseDomain`, a service account with the [installer's roles](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/installing_on_gcp/) or Owner, and the compute, dns, iam, cloudresourcemanager and serviceusage APIs.

    ```sh
    oc create namespace hive-credentials   # once per hub
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
    oc create namespace hive-credentials   # once per hub
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

    Needs vCenter and ESXi 7.0.2+, vCenter on port 443, DNS for `api.<cluster>.<baseDomain>` and `*.apps.<cluster>.<baseDomain>` pointing at the VIPs, and DHCP on the network unless `hive.platform.vsphere.hosts` sets static IPs (see Static IPs below). Inventory paths are absolute. More than one failure domain needs the `openshift-region` and `openshift-zone` tag categories attached to the datacenters and clusters. The hub downloads the RHCOS OVA unless `topology.template` names one.

    `cacertificate` is the vCenter CA in PEM, from `https://<vcenter>/certs/download.zip` (`certs/lin/*.0`). It is required even for a publicly trusted vCenter. Use `--from-literal` for the login: a trailing newline fails the install.

    ```sh
    oc create namespace hive-credentials   # once per hub
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
        With `hosts`, the installer creates the workers as standalone Machines and leaves the worker MachineSets at 0, drawing addresses from an IPPool. Nothing installs an IPAM controller for that pool, so new Machines (from `oc scale machineset`, or the `infraNodes` and `storageNodes` elements) never get an address. Use DHCP if you plan to add nodes after install, or install an IPAM controller on the cluster yourself.

    !!! note "No hibernation"
        Hive cannot hibernate vSphere clusters, so leave `install.hive.clusterDeployment.spec.powerState` unset.

    !!! warning "Rotating the vCenter password"
        Update both:

        - The credential on the hub. The hub copies it to `<cluster>-creds`, which Hive uses to destroy the cluster.
        - `kube-system/vsphere-creds` on the cluster, used by its storage and machine controllers. Nothing updates it for you.

=== "Bare metal"

    Needs:

    - The [`cim`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/cim) element on the hub (central infrastructure management): `stack.cim.enabled: true` in a values file the hub reads, one listed in `fleet/hubs/<hubName>.yaml` or `values/clusters/<hubName>.yaml`, not the new cluster's files. It manages the Provisioning CR, AgentServiceConfig `agent` and the MultiClusterHub `siteconfig` component. Policy `ready-cim-<hubName>` is Compliant once they run (`oc get policies.policy.open-cluster-management.io -n pstack ready-cim-<hubName>`).
    - On the hub, yourself: a default StorageClass (or `stack.cim.config.storage.storageClass`) and [the bare-metal ports](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/). On an AWS hub, Red Hat's procedure for enabling central infrastructure management on AWS also adds an ingress controller backed by a Network Load Balancer so hosts can reach the assisted service ([Clusters](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/)).
    - Hosts with a virtual-media BMC the hub reaches. Hosts reach the hub's image service and API.
    - DNS for `api.<cluster>.<baseDomain>` from hosts and hub. Multi-node adds `api-int` and `*.apps`.
    - Disconnected: mirrors and RHCOS images on the hub ([Bare metal](disconnected.md#bare-metal)).

    ```sh
    oc create namespace bm-credentials
    oc create secret generic hostinventory -n bm-credentials \
      --from-file=pullSecret=pull-secret.json --from-file=ssh-publickey=id_ed25519.pub # (1)!
    oc create secret generic bmc -n bm-credentials --from-literal=username=<user> --from-literal=password=<password>
    ```

    1.  One key only: the stock templates fold a second into the first, so `present-cluster-<cluster>` fails before create with `install.credential ssh-publickey must hold one key`. Use RSA or ECDSA with `siteconfig.fips`: FIPS mode disables ed25519, so the example's `id_ed25519.pub` fails before create.

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
    2.  Keyed by host name. `role` defaults to `master`; set `role: worker` for workers. [All keys](../reference/lifecycle.md#install-keys).
    3.  `idrac-virtualmedia://` for Dell, `ilo5-` or `ilo4-virtualmedia://` for HPE, `redfish-virtualmedia://` otherwise ([all variants](../reference/lifecycle.md#install-keys)). Other schemes need a provisioning network, which the `cim` default doesn't have, so the schema refuses them.
    4.  Keep the quotes: an unquoted MAC is not read as a string by YAML.

    !!! warning "Fixed after create"
        SiteConfig allows no spec change until the install completes, and after it only adding and removing workers. A failed install is not retried: destroy it and build again ([When a build fails](#when-a-build-fails)). Check addresses and networks before merging.

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

    1.  `engine` defaults to `hypershift`; `platform` has no default, and without it the render fails with `install.platform is required`. Without `baseDomain`, HyperShift uses the hub's ingress domain.
    2.  NodePool `hcp-kubevirt-workers`. The chart generates `clusterName`, the platform and `management.upgradeType` (`Replace` on KubeVirt), and merges this map over them. It always sets the release image itself and ignores `release` here. Without `replicas` the pool has 0 nodes.

=== "Agent"

    Needs:

    - An AgentServiceConfig (`stack.cim` on the hub, as for bare metal) and a LoadBalancer implementation on the hub, as for KubeVirt.
    - An InfraEnv in `agentNamespace`, with hosts booted from its discovery ISO, approved, and labeled to match `agentLabelSelector`. PolicyStack creates none of this; follow [Host inventory](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/) (create the InfraEnv, boot the ISO, approve with `oc patch agent <id> -n hardware-inventory --type merge -p '{"spec":{"approved":true}}'`, then `oc label agent <id> -n hardware-inventory example.com/cluster=hcp-agent`).
    - DNS for `*.apps.<cluster>.<baseDomain>` at the guest ingress. In [`values/clusters/hcp-agent.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values/clusters/hcp-agent.yaml), the `metallb` element runs as Day-2 config on the hosted cluster and gives its router a fixed address (`10.20.13.10`) for that record. The hub-side MetalLB pool for the hosted API comes from the same file's `install.manifests`.

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

    Use it for VMs or hosts that ACM cannot provision. No machines are created: you boot each worker from the NodePool's Ignition. No InfraEnv or Agents are involved. Needs a credential with `pullSecret` and `ssh-publickey`, created like the Agent tab's `hcp-credentials/hostinventory`, a hub LoadBalancer, and DNS for `*.apps.<cluster>.<baseDomain>` at the workers (router on host ports 80/443).

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

    Add a worker once the HostedCluster is up. The hosted control plane runs in namespace `<install.namespace>-<cluster>` (`clusters-hcp-none` here); the admin kubeconfig Secret is in `install.namespace` (`clusters`).

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

## Values

Lifecycle uses the same [values cascade](../concepts/values.md#order) as Day-2, starting from `lifecycle/values.yaml`, the lifecycle chart's defaults (every `install` key, commented). The fleet file's `install` ranks above every layer.

Shipped layers:

| Layer | Sets |
|---|---|
| `values/platforms/aws.yaml` | `engine: hive`, credential `hive-credentials/aws`, 3 control-plane + 3 worker `m6i.xlarge`, `OVNKubernetes` with machine network `10.0.0.0/16` |
| `values/platforms/vmware.yaml` | `engine: hive`, credential `hive-credentials/vsphere`, 3 control-plane + 3 worker amd64 at installer default size |
| `values/platforms/baremetal.yaml` | `engine: siteconfig`, credentials `bm-credentials/hostinventory` and `bm-credentials/bmc` |
| `values/platforms/kubevirt.yaml` | `platform: KubeVirt`, credential `open-cluster-management/kubevirt-secret`, 2 workers |
| `values/platforms/agent.yaml` | `platform: Agent`, credential `hcp-credentials/hostinventory`, `agentNamespace: hardware-inventory` |

`aws`, `vmware` and `baremetal` also select the matching `infraNodes` and `storageNodes` Day-2 policies, so imported clusters on those platforms list the same layer; lifecycle reads only its `install`.

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

`version` and `channel` belong in the fleet file only; CI rejects them in a layer (POLICY034, [why](upgrades.md#how-it-works)). For per-cluster `install` settings you'd rather keep out of the fleet file, use `values/clusters/<cluster>.yaml`: it is read last, after every listed layer, and can also hold the cluster's Day-2 `stack` settings.

!!! warning "Shared layers reach the build too"
    - Maps merge, lists replace ([Merge rules](../concepts/values.md#merge-rules)): `installConfig.compute` in the fleet file replaces the layer's list.
    - Changing a fixed-at-create key like `baseDomain` in a layer turns every existing cluster reading it NonCompliant.
    - The lifecycle chart schema-checks every `stack.*` key in the layers it reads, so an element typo in a shared layer, such as `certManager: Additional property enabeld is not allowed`, also fails every `lifecycle-<cluster>` render that reads it.
    - `global.remediationAction: inform` ([Dry run](rollout.md#dry-run)) also sets the lifecycle Policy to inform: nothing is built, and `state: absent` destroys nothing.

## Watching a build

```sh
oc get policies.policy.open-cluster-management.io -n pstack present-lifecycle-<cluster>  # Compliant = built
oc get configurationpolicy -n local-cluster | grep <cluster>    # each step (Global Hub default-mode hub: run on the global hub with -n <managed-hub>)
oc get applications.argoproj.io lifecycle-<cluster> -n openshift-gitops -o jsonpath='{.status.conditions}'   # render errors
# each step's latest message. ACM's copy of the Policy in the hub's own namespace is named
# <policy-namespace>.<policy> (concepts/acm.md, How a Policy reaches a cluster)
oc get policies.policy.open-cluster-management.io -n local-cluster pstack.present-lifecycle-<cluster> \
  -o jsonpath='{range .status.details[*]}{.templateMeta.name}{": "}{.history[0].message}{"\n"}{end}'
oc get policies.policy.open-cluster-management.io -n pstack | grep <cluster>   # Day-2 elements, not covered by present-lifecycle
```

NonCompliant with `Waiting for …` on `ready` is normal until the install finishes; read the first step that isn't Compliant ([The step chain](../concepts/lifecycle.md#the-step-chain)). Errors and fixes: [Troubleshooting](../reference/lifecycle.md#troubleshooting).

| Engine | Progress | Typical time |
|---|---|---|
| `hive` | `oc get clusterdeployment -n <cluster>`<br>`oc logs -f -n <cluster> -l hive.openshift.io/install=true -c installer` | 45 min on AWS |
| `siteconfig` | `oc get clusterinstance,bmh,agentclusterinstall,agent -n <cluster>` | 1 h single node |
| `hypershift` | `oc get hostedcluster,nodepool -n <install.namespace>` (default `clusters`) | Varies with platform |

What each step does per engine: [Policy chains](../reference/lifecycle.md#policy-chains).

## When a build fails

On `hive` and `siteconfig` a failed install is final: Hive gets `installAttemptsLimit: 1`, and SiteConfig refuses spec changes during install. `present-ready` stays NonCompliant either way.

| Engine | Failure shows as | Recover |
|---|---|---|
| `hive` | ClusterDeployment condition `ProvisionStopped=True` | Destroy and rebuild (steps below) |
| `siteconfig` | ClusterInstance condition `Provisioned` False | Destroy and rebuild |
| `hypershift` | Step message; HyperShift keeps reconciling | Fix a mutable value and merge; a [fixed-at-create](../reference/lifecycle.md#immutable-fields) one needs the steps below |

```sh
oc get clusterdeployment -n <cluster> <cluster> -o jsonpath='{.status.conditions[?(@.type=="ProvisionStopped")]}'
```

To start over:

1. Set `state: absent` and merge ([Destroy](#destroy)). Keep `install` and its layers.
2. Wait for `absent-lifecycle-<cluster>` to be Compliant.
3. Fix the values, then remove `state` (or set `state: present`) and merge. The guard finds nothing left, so the build starts fresh.

On `hive` and `siteconfig`, `present-credentials-<cluster>` can report `because it is being terminated` until Namespace `<cluster>` from the destroy is gone. It clears on its own. Engine-specific failures: [Troubleshooting](../reference/lifecycle.md#troubleshooting).

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
| `hive`, `siteconfig` | On the cluster, by the Day-2 [`upgrade`](upgrades.md#the-upgrade-element) element, like imported clusters. The hub keeps the install release image: Hive rejects changes to it, and the chart reads the `siteconfig` image set back from the live object. |
| `hypershift` | From the hub: the `cluster` step sets the HostedCluster release image to `<install.releaseRepository>:<version>-multi`, then each NodePool once the control plane reaches `version` ([rules](upgrades.md#hosted-control-planes)). `KubeVirt` pools replace VMs; others upgrade in place. Disconnected, `channel` needs `install.hostedCluster.spec.updateService` pointing at your OSUS, or the bump waits forever ([Hosted clusters](disconnected.md#hosted-clusters)). |

Watch a `hypershift` upgrade from the hub, reading the VERSION column:

```sh
oc get hostedcluster <cluster> -n <install.namespace>
oc get nodepool -n <install.namespace> | grep <cluster>
```

`present-lifecycle-<cluster>` is NonCompliant until the control plane and every NodePool reach `version`. A held downgrade, such as a reverted bump after the control plane moved, keeps it NonCompliant until `version` is back at or above the control plane's version. `hypershift` and `hive` clusters install from the `-multi` payload, so they are multi-architecture. `siteconfig` installs `-x86_64` and stays single-architecture.

??? info "Minor upgrade blocked by `ClusterVersionUpgradeable=False`"
    HyperShift sets `ClusterVersionUpgradeable=False` on the HostedCluster when the guest reports a minor upgrade unsafe (for example, removed APIs still in use), and `present-ready` keeps waiting. Fix the cause if you can. To override, set the annotation to the exact release image. It forces only the image it names, so remove it after the upgrade:

    ```yaml
    install:
      hostedCluster:
        metadata:
          annotations:
            hypershift.openshift.io/force-upgrade-to: quay.io/openshift-release-dev/ocp-release:4.21.2-multi
    ```

## Scaling

Workers change after create through `install.nodePools` (`hypershift`: add a key for a new pool, change `replicas` to scale) and `install.siteconfig.nodes` (`siteconfig`). On `hive`, `installConfig` is read once at install; scale with MachineSets on the cluster (`oc scale machineset`, or the `infraNodes`/`storageNodes` elements).

### Removing a NodePool

Lifecycle creates NodePools with `musthave`, the ACM compliance type that makes sure an object exists and never deletes it ([ACM policies in brief](../concepts/acm.md)). Removing a `nodePools` entry only stops managing the pool; its nodes keep running. Remove the entry, then delete the pool:

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

**Scale out.** Add a host with `role: worker` under `install.siteconfig.nodes`. The chart adds it to the ClusterInstance only after the install completes; that is also true for workers listed at create on a one-master cluster, which join by scale-out after the master is up. Before you merge:

- The hub's admin kubeconfig for the cluster (Secret `<cluster>-admin-kubeconfig` in namespace `<cluster>`) still works. If the API certificate changed, [update it first](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/).
- The `bmcCredential` Secret still exists on the hub. Without it `present-credentials-<cluster>` fails and `present-cluster-<cluster>` never applies the change.

`present-ready-<cluster>` stays NonCompliant until the host's Agent reports `Done`, and names the host once its BareMetalHost exists.

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

`present-cluster-<cluster>` then runs SiteConfig's documented scale-in. Each write waits for the previous one:

| Step | Written to the node | Waits for |
|---|---|---|
| 1 | `extraAnnotations` `remove-agent-and-node-on-delete` | The annotation on the BareMetalHost |
| 2 | `pruneManifests`: BareMetalHost, InfraEnv, NMStateConfig | BareMetalHost gone, SiteConfig done (`observedGeneration`) |
| 3 | Node dropped | |

BMAC (the BareMetal Agent Controller in assisted-service) drains the Node, deprovisions the host and removes the Agent and Node. Follow it with `oc get bmh,agent -n <cluster>`. Once `present-ready-<cluster>` is Compliant, delete the host's entry under `install.siteconfig.nodes`. Do not delete the fleet file: that abandons the cluster ([Abandon](#abandon)). Cleaning is metadata only: data stays on the disks.

| You do | Result |
|---|---|
| Delete a live node's entry, or rename it | `present-cluster` fails with `not install.siteconfig.nodes`: SiteConfig would accept the drop and delete nothing, stranding the BareMetalHost, Agent and Node. |
| Add a master to a live cluster | `present-cluster` fails: `is a new master`. |
| Clear `remove` before step 2 | Scale-in cancelled. |
| Clear `remove` after step 2 | `present-cluster` fails: `cannot be cancelled`. |
| `remove` on a worker still joining | `present-cluster` fails: `has not finished joining`. Its drain would loop until SiteConfig's 30-minute timeout pauses the ClusterInstance. |
| `remove` on a worker whose Node you already deleted | Not detected: the hub cannot see spoke Nodes. The drain fails the same way, and the BareMetalHost's drain-status annotation shows `failed to drain node`. |

??? warning "Removing a worker that never joined, or whose Node is gone"
    Either finish the install, or annotate its BareMetalHost so the drain gives up. BMAC honours this annotation, but no Red Hat doc covers it:

    ```sh
    oc annotate bmh <host> -n <cluster> bmac.agent-install.openshift.io/drain-timeout=5m
    ```

    The value is a Go duration, counted from the first drain attempt. If the ClusterInstance has already paused, also remove its paused annotation.

## Destroy

The hub detaches the cluster from ACM, deletes it through its engine, then cleans up the copied Secrets and `install.manifests` ([Destroy and abandon](../concepts/lifecycle.md#destroy-and-abandon)). The credential is never deleted.

1. Set `state: absent` and merge. Change nothing else:

    ```diff
     # fleet/ocp-aws.yaml
     revision: main
     hub: acm-dc1
    +state: absent
    ```

    !!! danger "Keep `install` and its layers"
        The absent Policy finds the cluster's objects through `install.engine`. Stripped of `install` and the `valueFiles` that set it, the engine falls back to `hypershift`: on a `hive` cluster the Policy detaches the ManagedCluster, "deletes" a HostedCluster that never existed and goes Compliant, while the ClusterDeployment and its cloud resources keep running. Deleting the fleet file after that orphans them.

2. Wait for `absent-lifecycle-<cluster>` to be Compliant. Deleting the fleet file earlier abandons the cluster mid-destroy.

    ```sh
    oc get policies.policy.open-cluster-management.io -n pstack absent-lifecycle-<cluster>  # Compliant = gone
    ```

3. Delete the fleet file and `values/clusters/<cluster>.yaml`. To build a new cluster under the same name instead, remove `state` again ([When a build fails](#when-a-build-fails)).
4. If the cluster ran the `acs` element and you will re-create it under the same name, delete it in ACS first, or Central refuses to register the new one under the old name ([Re-register a cluster](../elements/acs.md#re-register-a-cluster), step 1).

Until step 3, the absent Policy keeps running and deletes these again if anything re-creates them:

| Engine | Deleted |
|---|---|
| `hive` | Namespace `<cluster>`, `install.manifests` |
| `siteconfig` | Namespace `<cluster>`, ClusterImageSet `lifecycle-<cluster>`, `install.manifests` |
| `hypershift` | Secrets `<cluster>-pull-secret`, `-ssh-key` and `-etcd-encryption-key` in `install.namespace`, `install.manifests` |

| Engine | Left behind |
|---|---|
| `hive` | Nothing. Hive deprovisions the cloud resources or VMs, usually within 20 minutes. vSphere: the DNS records you created. |
| `siteconfig` | Hosts still running, disks not wiped. Power off or reuse. A worker scaled in is powered off with its disk metadata cleaned; data stays on disk. |
| `hypershift` Agent | Agents return to the InfraEnv for the next cluster. |
| `hypershift` None | Your workers and their DNS. Delete them. |

## Abandon

Abandoning stops lifecycle from managing a cluster without destroying it ([Destroy and abandon](../concepts/lifecycle.md#destroy-and-abandon)). The cluster keeps running and stays an ACM ManagedCluster: deleting a lifecycle Policy never deletes what it created.

| You do | Lifecycle | Day-2 |
|---|---|---|
| Remove `hub`, `state` and `install` together from the fleet file | Abandoned | Keeps its Applications and Policies |
| Delete the fleet file | Abandoned | Its Applications and Policies are deleted too ([Removal and pruning](../concepts/removal.md)) |

Remove all three keys: the validator rejects `state` or `install` without `hub` (POLICY050). Layers in `valueFiles` that set `install` can stay; nothing reads them without `hub`.

`helm uninstall policystack` leaves the ApplicationSets in place (`helm.sh/resource-policy: keep`, [Install chart](../reference/install.md)), so it abandons nothing. Deleting the ApplicationSets with `oc delete` abandons every cluster the hub built. To re-adopt, restore the keys or file with the same `install`: the root object still carries the [ownership marker](../concepts/lifecycle.md#ownership), so the guard accepts it.

!!! warning "Changing `hub`"
    Moving a cluster between hubs is not supported. Changing `hub` abandons the cluster on the old hub, which keeps running it. The new hub's lifecycle ApplicationSet now matches the file and runs the present Policy there; its guard finds no root object, so it builds a second, same-named cluster. It only takes over a root object that was restored or migrated onto it with the marker. Destroy the cluster on the old hub first. ACS: run steps 2-3 of [Re-register a cluster](../elements/acs.md#re-register-a-cluster) after the move.

## Hub disaster recovery

A DR hub takes over the `hubName` of the hub it replaces ([The hub](../concepts/architecture.md#the-hub)). ACM backup leaves out PolicyStack's ApplicationSets and their Applications (`velero.io/exclude-from-backup`), so a passive hub (one receiving ACM backups but not managing clusters) builds nothing. Do not install PolicyStack on it before takeover: with the same `hubName`, its lifecycle ApplicationSet would match the same fleet files and, finding no root objects of its own, build a second copy of every cluster. To take over:

1. Make sure the old hub stays down, or delete its ApplicationSets (`oc delete applicationset -n openshift-gitops policystack policystack-lifecycle`). Two hubs with one `hubName` both act on the same fleet files.
2. Run the ACM activation restore with `veleroManagedClustersBackupName: latest` in the Restore resource ([Business continuity](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/business_continuity/)). ClusterDeployments, ClusterInstances and ManagedClusters come back with their annotations.
3. `hypershift`: restore the HostedClusters with the OADP procedure in [Hosted control planes](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/hosted_control_planes/). ACM backup does not include them, and without them the hub builds new, empty control planes.
4. [Install PolicyStack](../start/install.md#install-policystack) with the old hub's `hubName`. No fleet edits: the restored root objects carry the ownership marker, so the new hub owns them.
