# Values cascade

Every Application renders its element chart with an ordered list of values files. The cluster's [fleet file](applicationset.md#fleet-files) chooses the files, and later files override earlier ones. The ApplicationSet builds the list in the `templatePatch` of [appset.yaml](https://github.com/PolicyStack/PolicyStack/blob/main/appset/templates/appset.yaml).

## Order

Lowest precedence first. Paths are relative to `stack/<element>`, the Application's source path.

1. `values.yaml`: the element defaults.
2. `../../values.yaml`: the repo root, shared by every element. It sets `policyNamespace`.
3. `../../values/<category>s/<value>.yaml`, one per `<category>.<priority>: <value>` entry under `config` in the cluster's fleet file, in sort-key order. The directory is the category name plus `s`: `tenant` maps to `values/tenants/`.
4. The cluster's own files:
    - On the hub (`local-cluster` label): `../../values/acm/acm-<datacenter>.yaml`, then `../../values/clusters/acm-<datacenter>.yaml`.
    - On any other cluster: `../../values/clusters/<cluster>.yaml`, where `<cluster>` is the ManagedCluster name.
5. `valuesObject`, the keys the ApplicationSet [injects](applicationset.md#injected-values). Argo CD ranks it above every values file.

The sort key for a config entry is `printf "%05s-%s-%s"` of its priority, category and value. The keys sort as text (`sortAlpha`):

- The priority is zero-padded to five characters, so `10` becomes `00010` and priorities 1 to 99999 sort numerically. Longer numbers get no padding and sort as text: `100000` loads before `20000`.
- Equal priorities sort by category name, then by value: `region.40` loads before `tenant.40`.
- Environment and datacenter have no fixed slots. They sort like any other category, so `datacenter.5` loads before `environment.10`.
- A config entry with no `.<priority>` suffix adds no file.

[Fleet files](applicationset.md#fleet-files) lists the repo's priority convention.

## Missing files

The ApplicationSet sets `ignoreMissingValueFiles: true`, so Argo CD skips any listed file that does not exist at the cluster's revision. A layer needs a file only where it changes something.

The same setting hides mistakes. Each of these drops a layer without an error, and the Application syncs from the remaining files:

- A typo in a config entry's category or value in the fleet file, which points at a file that does not exist.
- A cluster file whose name differs from the ManagedCluster name.

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
├── platforms/      aws.yaml  baremetal.yaml  vmware.yaml
├── tenants/        payments.yaml
├── acm/            acm-dc1.yaml
└── clusters/       acm-dc1.yaml  aws-prod.yaml  nonprod-west-1.yaml  prod-east-1.yaml
```

## Example files

The files under [values/](https://github.com/PolicyStack/PolicyStack/tree/main/values) configure the example clusters. They are a starting point, not a complete configuration. Every element ships with `enabled: false`, and the layer that should run an element turns it on. The Selected by column shows the `config` entry in a cluster's fleet file that adds the file.

| File | Selected by | Holds |
|---|---|---|
| `values/environments/prod.yaml` | `environment.10: prod` | The security and compliance baseline for every prod cluster: cert-manager, External Secrets, the Compliance Operator, manual remediations and cluster DNS |
| `values/environments/nonprod.yaml` | `environment.10: nonprod` | Placeholder, sets nothing |
| `values/datacenters/dc1.yaml` | `datacenter.20: dc1` | Placeholder, sets nothing |
| `values/datacenters/dc2.yaml` | `datacenter.20: dc2` | Site facts only: the registry allowlist through the dc2 mirror. Turns no element on |
| `values/platforms/aws.yaml` | `platform.30: aws` | Toggles for the node elements: clone the worker MachineSet |
| `values/platforms/vmware.yaml` | `platform.30: vmware` | Toggles for the node elements: clone each worker MachineSet, one per vSphere failure domain |
| `values/platforms/baremetal.yaml` | `platform.30: baremetal` | Toggles for the node elements: no infra MachineSets, and storage nodes are existing nodes labeled in place |
| `values/tenants/payments.yaml` | `tenant.40: payments` | A custom category: the GitOps operator without its instance, plus a payments team Argo CD |
| `values/acm/acm-dc1.yaml` | The hub in dc1 | Reserved for the hub's own ACM and GitOps elements. Left commented out so PolicyStack does not reconcile its own delivery path |
| `values/clusters/acm-dc1.yaml` | The hub in dc1 | The hub's elements, and a commented list of the elements left off and why |
| `values/clusters/prod-east-1.yaml` | ManagedCluster `prod-east-1` | Infra nodes, machine health checks, an update channel pin and user workload monitoring |
| `values/clusters/nonprod-west-1.yaml` | ManagedCluster `nonprod-west-1` | Enforces the dc2 registry allowlist, the cluster's own MetalLB address pool, and user workload monitoring |
| `values/clusters/aws-prod.yaml` | ManagedCluster `aws-prod` | Values for an AWS test cluster: the OpenShift Data Foundation to Loki storage chain and additional operator installs. Rename it to the target ManagedCluster name before use, its header lists the fleet file it expects |

CI renders every element with the files the example fleet files in `tools/validator/testdata/clusters/` select ([Validation](validation.md)). None of them selects `vmware.yaml`, `payments.yaml` or `aws-prod.yaml`, so CI reads them only when a fleet file in `fleet/` selects them.

## Worked example

`fleet/prod-east-1.yaml` selects the files for `prod-east-1`:

```yaml
revision: main
config:
  environment.10: prod
  datacenter.20: dc1
  platform.30: aws
```

The sort keys are `00010-environment-prod`, `00020-datacenter-dc1` and `00030-platform-aws`. For the `infra-nodes` element, the ApplicationSet creates the Application `infra-nodes-prod-east-1` with these files:

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

`valuesObject` then adds the one-cluster `selector` and the `selected*` keys.

The dc1 hub carries ACM's `local-cluster` label, so it reads `fleet/hubs/<hubName>.yaml` ([The hub](applicationset.md#the-hub)). With `hubName: acm-dc1`, `fleet/hubs/acm-dc1.yaml`:

```yaml
revision: main
config:
  environment.10: prod
  datacenter.20: dc1
  platform.30: baremetal
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

`values/clusters/acm-dc1.yaml` turns the element on. The hub's Applications and values files are named `acm-` plus the `datacenter` value in its fleet file, not its ManagedCluster name or `hubName`, so `values/clusters/local-cluster.yaml` is never read. `hubName` only locates the fleet file. Setting it to `acm-<datacenter>` gives the fleet file and the values files the same name.
