# Values cascade

Every Application renders its element chart with an ordered list of values files. The cluster's [fleet file](applicationset.md#fleet-files) chooses the files, and later files override earlier ones. The ApplicationSet builds the list in the `templatePatch` of [appset.yaml](https://github.com/PolicyStack/PolicyStack/blob/main/appset/templates/appset.yaml).

## Order

Lowest precedence first. Paths are relative to `stack/<element>`, the Application's source path.

1. `values.yaml`: the element defaults.
2. `../../values.yaml`: the repo root, shared by every element. It sets `policyNamespace`.
3. `../../values/<entry>`, one per entry in the `valueFiles` list of the cluster's fleet file, in list order. Nothing sorts the list, so a later entry overrides an earlier one.
4. `../../values/clusters/<name>.yaml`, where `<name>` is the ManagedCluster name, or `hubName` on the hub ([The hub](applicationset.md#the-hub)).
5. `valuesObject`, the keys the ApplicationSet [injects](applicationset.md#injected-values). Argo CD ranks it above every values file.

A lifecycle Application reads the same files with paths relative to `lifecycle/`: `values.yaml`, `../values.yaml`, `../values/<entry>`, `../values/clusters/<cluster>.yaml`. Its `valuesObject` holds the fleet file's `hub`, `state` and `install` ([Values](lifecycle.md#values)). Elements ignore the top-level `install` key and only `lifecycle/` reads it, so one cluster file can hold both `install` and `stack`.

## Missing files

The ApplicationSet sets `ignoreMissingValueFiles: true`, so Argo CD skips any listed file that does not exist at the cluster's revision. A cluster needs a file in `values/clusters/` only where it changes something.

The same setting hides mistakes. Each of these drops a layer without an error, and the Application syncs from the remaining files:

- A typo in a `valueFiles` entry.
- A `valueFiles` entry added on the default branch for a file that the cluster's pinned revision does not have yet.
- A cluster file whose name differs from the ManagedCluster name.

The validator reports a `valueFiles` entry that is not a file under `values/` (POLICY050, [Validation](validation.md)). It checks the tree it runs on, not each cluster's revision, so it does not catch the other two.

Elements default to off, so a dropped layer usually shows up as an element that never turns on. Compare the Application's value files with the repo at the cluster's revision:

```sh
oc get applications.argoproj.io infra-nodes-prod-east-1 -n openshift-gitops \
  -o jsonpath='{.spec.source.helm.valueFiles}'
```

## Merge rules

Helm merges the files in order.

- **Maps merge** key by key. A later file sets only the keys it changes: [nonprod-west-1.yaml](https://github.com/PolicyStack/PolicyStack/blob/main/values/clusters/nonprod-west-1.yaml) sets `retention` and `storage` under `stack.userWorkloadMonitoring.config.prometheus`, and the element defaults supply the rest.
- **Lists replace.** A later list overwrites the whole earlier list: [dc2.yaml](https://github.com/PolicyStack/PolicyStack/blob/main/values/datacenters/dc2.yaml) restates the element's default registries next to its mirror.

An element's policy entries are a list, so a layer cannot change one entry without restating them all. It flips an entry through the [`toggles`](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles) map instead, and settings go in the `config` map that the element's converters read ([Conventions](elements.md#conventions)).

## Layout

```text
stack/<element>/values.yaml    element defaults
values.yaml                    shared by every element: policyNamespace
values/
├── environments/   nonprod.yaml  prod.yaml
├── datacenters/    dc1.yaml  dc2.yaml
├── platforms/      agent.yaml  aws.yaml  baremetal.yaml  kubevirt.yaml  vmware.yaml
├── tenants/        payments.yaml
├── acm/            acm-dc1.yaml
└── clusters/       acm-dc1.yaml  aws-prod.yaml  hcp-agent.yaml  nonprod-west-1.yaml  prod-east-1.yaml
```

## Example files

The files under [values/](https://github.com/PolicyStack/PolicyStack/tree/main/values) configure the example clusters. They are a starting point, not a complete configuration. Every element ships with `enabled: false`, and the layer that should run an element turns it on. The Read by column names the example fleet files that list the file, or the cluster whose name matches it.

| File | Read by | Holds |
|---|---|---|
| `values/environments/prod.yaml` | `prod-east-1`, `hubs/acm-dc1`, `hcp-agent` | The security and compliance baseline for every prod cluster: cert-manager, External Secrets, the Compliance Operator, manual remediations and cluster DNS |
| `values/environments/nonprod.yaml` | `nonprod-west-1`, `hcp-kubevirt`, `hcp-retired` | Placeholder, sets nothing |
| `values/datacenters/dc1.yaml` | `prod-east-1`, `hubs/acm-dc1`, `hcp-agent` | `install.baseDomain` for hosted clusters built in dc1. Turns no element on |
| `values/datacenters/dc2.yaml` | `nonprod-west-1` | Site facts only: the registry allowlist through the dc2 mirror. Turns no element on |
| `values/platforms/aws.yaml` | `prod-east-1` | Toggles for the node elements: clone the worker MachineSet |
| `values/platforms/vmware.yaml` | No example | Toggles for the node elements: clone each worker MachineSet, one per vSphere failure domain |
| `values/platforms/baremetal.yaml` | `hubs/acm-dc1` | Toggles for the node elements: no infra MachineSets, and storage nodes are existing nodes labeled in place |
| `values/platforms/kubevirt.yaml` | `hcp-kubevirt`, `hcp-retired` | `install` only: platform `KubeVirt`, the credential and a `workers` NodePool of 2 |
| `values/platforms/agent.yaml` | `hcp-agent` | `install` only: platform `Agent`, the Agent namespace and the credential. No default NodePool |
| `values/tenants/payments.yaml` | No example | A custom layer: the GitOps operator without its instance, plus a payments team Argo CD |
| `values/acm/acm-dc1.yaml` | `hubs/acm-dc1` | Reserved for the hub's own ACM and GitOps elements. Left commented out so PolicyStack does not reconcile its own delivery path |
| `values/clusters/acm-dc1.yaml` | The hub, `hubName: acm-dc1` | The hub's elements, and a commented list of the elements left off and why |
| `values/clusters/prod-east-1.yaml` | ManagedCluster `prod-east-1` | Infra nodes, machine health checks, an update channel pin and user workload monitoring |
| `values/clusters/hcp-agent.yaml` | `lifecycle-hcp-agent` and ManagedCluster `hcp-agent` | `install`: the oac-apps style services, MetalLB API address and Certificate ([Mapping from oac-apps](lifecycle.md#mapping-from-oac-apps)). `stack`: the guest's MetalLB ingress pool |
| `values/clusters/nonprod-west-1.yaml` | ManagedCluster `nonprod-west-1` | Enforces the dc2 registry allowlist, the cluster's own MetalLB address pool, and user workload monitoring |
| `values/clusters/aws-prod.yaml` | ManagedCluster `aws-prod` | Values for an AWS test cluster: the OpenShift Data Foundation to Loki storage chain and additional operator installs. Rename it to the target ManagedCluster name before use, its header lists the fleet file it expects |

CI renders every element with the files the example fleet files in `tools/validator/testdata/clusters/` list ([Validation](validation.md)). None of them lists `vmware.yaml` or `payments.yaml`, and no example cluster is named `aws-prod`, so CI reads those files only when a fleet file in `fleet/` uses them.

## Worked example

`fleet/prod-east-1.yaml` lists the files for `prod-east-1`:

```yaml
revision: main
valueFiles:
  - environments/prod.yaml
  - datacenters/dc1.yaml
  - platforms/aws.yaml
```

For the `infra-nodes` element, the ApplicationSet creates the Application `infra-nodes-prod-east-1` with these files:

```yaml
valueFiles:
  - values.yaml
  - ../../values.yaml
  - ../../values/environments/prod.yaml
  - ../../values/datacenters/dc1.yaml
  - ../../values/platforms/aws.yaml
  - ../../values/clusters/prod-east-1.yaml
```

The environment and datacenter files set nothing for this element. The merged `stack.infraNodes`, trimmed:

```yaml
enabled: true                 # clusters/prod-east-1.yaml
toggles:
  mcp: true                   # element default
  config: true                # element default
  aws: true                   # platforms/aws.yaml
  vmware: false               # platforms/aws.yaml
  ready: true                 # element default
  workloads: false            # element default
  restore: false              # element default
config:
  replicas: 1                 # clusters/prod-east-1.yaml
  zones:                      # clusters/prod-east-1.yaml, replaces the default []
    - us-east-1a
    - us-east-1b
    - us-east-1c
  instanceType: m6i.2xlarge   # clusters/prod-east-1.yaml
  # every other config key keeps its element default
```

`valuesObject` then adds the one-cluster `selector`, `selectedName` and `selectedId`.

The dc1 hub carries ACM's `local-cluster` label, so it reads `fleet/hubs/<hubName>.yaml` ([The hub](applicationset.md#the-hub)). With `hubName: acm-dc1`, `fleet/hubs/acm-dc1.yaml`:

```yaml
revision: main
valueFiles:
  - environments/prod.yaml
  - datacenters/dc1.yaml
  - platforms/baremetal.yaml
  - acm/acm-dc1.yaml
```

For `node-feature-discovery`, the Application is `node-feature-discovery-acm-dc1`:

```yaml
valueFiles:
  - values.yaml
  - ../../values.yaml
  - ../../values/environments/prod.yaml
  - ../../values/datacenters/dc1.yaml
  - ../../values/platforms/baremetal.yaml
  - ../../values/acm/acm-dc1.yaml
  - ../../values/clusters/acm-dc1.yaml
```

`values/clusters/acm-dc1.yaml` turns the element on. On the hub, `hubName` takes the place of the ManagedCluster name, `local-cluster`, in the Application name and the cluster file, so `values/clusters/local-cluster.yaml` is never read.
