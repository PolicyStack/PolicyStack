# Values cascade

Each Application renders its element chart with an ordered list of values files picked by the cluster's [fleet file](applicationset.md#fleet-files). Later files win. The list is built in [appset.yaml](https://github.com/PolicyStack/PolicyStack/blob/main/install/templates/appset.yaml).

## Order

=== "Managed cluster"

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main
    channel: stable-4.20
    valueFiles:
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/aws.yaml
    ```

    Application `infra-nodes-prod-east-1`, lowest precedence first:

    ```yaml
    valueFiles:
      - values.yaml                             # (1)!
      - ../../values.yaml                       # (2)!
      - ../../values/environments/prod.yaml     # (3)!
      - ../../values/datacenters/dc1.yaml
      - ../../values/platforms/aws.yaml
      - ../../values/clusters/prod-east-1.yaml  # (4)!
    valuesObject:                               # (5)!
      channel: "stable-4.20"
      selectedName: prod-east-1
      selectedId: <clusterID label>
      selector:                                 # (6)!
        matchExpressions:
          name: {key: name, operator: In, values: ["prod-east-1"]}
    ```

    1.  Element defaults. Paths are relative to `stack/<element>`.
    2.  Repo root, shared by every element. Sets `policyNamespace`.
    3.  One per `valueFiles` entry, in list order. Nothing sorts them.
    4.  Always last. Named after the ManagedCluster.
    5.  [Injected values](applicationset.md#injected-values). Ranks above every file.
    6.  Targets this one cluster. For the hub, `values` holds `local-cluster`, not `hubName`.

=== "Hub"

    The hub reads `fleet/hubs/<hubName>.yaml`, and `hubName` replaces `local-cluster` in the Application name and cluster values file ([The hub](applicationset.md#the-hub)).

    ```yaml
    # Application nfd-acm-dc1
    valueFiles:
      - values.yaml
      - ../../values.yaml
      - ../../values/environments/prod.yaml
      - ../../values/datacenters/dc1.yaml
      - ../../values/platforms/baremetal.yaml
      - ../../values/acm/acm-dc1.yaml
      - ../../values/clusters/acm-dc1.yaml   # never clusters/local-cluster.yaml
    ```

=== "Lifecycle"

    Same files, relative to `lifecycle/`: `values.yaml`, `../values.yaml`, `../values/<entry>`, `../values/clusters/<cluster>.yaml`. `valuesObject` holds the fleet file's `hub`, `state`, `version`, `channel` and `install` ([Values](lifecycle.md#values)). Elements ignore `install`, so one cluster values file holds both:

    ```yaml
    # values/clusters/hcp-agent.yaml
    install: ...   # lifecycle/ builds the cluster
    stack: ...     # elements configure it once imported
    ```

Merged `stack.infraNodes` for `prod-east-1`, trimmed:

```yaml
enabled: true                 # clusters/prod-east-1.yaml
toggles:
  aws: true                   # platforms/aws.yaml
  workloads: false            # element default
config:
  replicas: 1                 # clusters/prod-east-1.yaml, default 3
  zones:                      # clusters/prod-east-1.yaml, replaces default []
    - us-east-1a
    - us-east-1b
    - us-east-1c
  maxPods: 250                # element default
```

## Missing files

`ignoreMissingValueFiles: true` skips any listed file absent at the cluster's revision. Add a `values/clusters/` file only for clusters that change something.

!!! warning "A skipped file raises no error"
    The Application syncs from the remaining layers. Elements default to off, so it looks like an element that never turns on.

    | Cause | Validator |
    |---|---|
    | Typo in a `valueFiles` entry | POLICY050 |
    | Entry for a file the cluster's pinned `revision` lacks | [Fleet pin check](workflows.md#policy-validate) |
    | Cluster values file name differs from the ManagedCluster name | [Fleet pin check](workflows.md#policy-validate) (warning) |

Compare the Application's files with the repo at the cluster's revision:

```sh
oc get applications.argoproj.io infra-nodes-prod-east-1 -n openshift-gitops \
  -o jsonpath='{.spec.source.helm.valueFiles}'
```

## Merge rules

Maps merge key by key. Lists replace.

=== "Map: set only what changes"

    ```yaml
    # values/clusters/nonprod-west-1.yaml
    stack:
      userMonitoring:
        config:
          prometheus:
            retention: 72h   # other prometheus keys keep their defaults
            storage: 20Gi
    ```

=== "List: restate everything"

    ```yaml
    # values/datacenters/dc2.yaml
    stack:
      remediations:
        config:
          allowedRegistries:
            - mirror.dc2.example.com:8443
            - quay.io                    # the defaults, restated
            - registry.redhat.io
            - image-registry.openshift-image-registry.svc:5000
    ```

`policies` and `configPolicies` are lists. Change them through maps instead:

| Change | Key |
|---|---|
| One policy on or off | [`toggles`](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles), e.g. `stack.remediations.toggles.registries` |
| A converter setting | `config` ([Conventions](elements.md#conventions)) |
| One operator field | `stack.metallb.operatorPolicies.metallb.versions`. `versions` is a list and replaces ([Operator upgrades](rollout.md#operator-upgrades)) |

## Schema

Each element aliases `policy-library` as `stack` in its `Chart.yaml`, so Helm checks the merged `stack` key against the library's [values.schema.json](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/values.schema.json) after every layer:

```text
Error: values don't meet the specifications of the schema(s) in the following chart(s):
stack:
- at '/certManager': additional properties 'enabeld' not allowed
```

Argo CD shows a ComparisonError and does not sync or prune. Every element validates the whole `stack` key, so one typo in a shared layer fails every Application reading it.

The validator's POLICY035 checks every file under `values/` for what the schema misses ([Validation](validation.md)):

| Mistake | Caught by |
|---|---|
| Unknown key in an element (`enabeld`) | schema, POLICY035 |
| Wrong type in an element | schema |
| Unknown top-level or `global` key | POLICY035 |
| Unknown element (`stack.certManagr`) | POLICY035 |
| Toggle that names no entry | POLICY035 |
| Top-level `config` key absent from the element's `values.yaml` | POLICY035 |
| Deeper `config` keys | nothing |

!!! warning
    Never set `skipSchemaValidation` on the ApplicationSet.

## Global keys

`global` applies from any layer: one cluster, a site or an environment.

| Key | Effect |
|---|---|
| `remediationAction` | `inform` makes every Policy, lifecycle included, report instead of enforce ([Dry run](rollout.md#dry-run)). `enforce` is rejected. |
| `catalogSources` | Maps a CatalogSource name to its replacement for every operator ([Disconnected clusters](disconnected.md)) |

```yaml
global:
  remediationAction: inform
  catalogSources:
    redhat-operators: cs-redhat-operator-index-v4-20
```

## Example files

[values/](https://github.com/PolicyStack/PolicyStack/tree/main/values) configures the example clusters. Any file under `values/` can be listed. Every element but `upgrade` ships `enabled: false`, so the layer that should run an element turns it on. `upgrade` acts only on the fleet file's `version` and `channel` ([Upgrades](upgrades.md)).

??? info "What each example file holds"
    | File | Holds |
    |---|---|
    | `environments/prod.yaml` | Prod baseline: cert-manager, External Secrets, Compliance Operator, remediations, DNS |
    | `environments/nonprod.yaml` | Nothing |
    | `datacenters/dc1.yaml` | `install.baseDomain` |
    | `datacenters/dc2.yaml` | Registry allowlist through the dc2 mirror |
    | `platforms/aws.yaml` | Node toggles: clone the worker MachineSet. `install`: Hive |
    | `platforms/vmware.yaml` | Node toggles: clone each MachineSet per failure domain. `install`: Hive |
    | `platforms/baremetal.yaml` | Node toggles: no MachineSets, label nodes. `install`: SiteConfig |
    | `platforms/kubevirt.yaml` | `install` only: `KubeVirt`, 2-node `workers` pool |
    | `platforms/agent.yaml` | `install` only: `Agent`, no default NodePool |
    | `tenants/payments.yaml` | GitOps operator plus a payments team Argo CD |
    | `acm/acm-dc1.yaml` | Hub ACM and GitOps, commented out ([why](install.md#hand-the-operators-to-policystack)) |
    | `clusters/acm-dc1.yaml` | Hub elements, and which are off and why |
    | `clusters/prod-east-1.yaml` | Infra nodes, machine health, user monitoring |
    | `clusters/hcp-agent.yaml` | `install`: hosted control plane service publishing. `stack`: guest MetalLB pool |
    | `clusters/nonprod-west-1.yaml` | Registry enforcement, MetalLB pool, user monitoring |
    | `clusters/aws-prod.yaml` | ODF to Loki chain. Rename to the real ManagedCluster name first |

    CI renders with the fleet files in `tools/validator/testdata/clusters/`. None lists `payments.yaml` and no cluster is named `aws-prod`, so CI reads those only when a file in `fleet/` uses them.
