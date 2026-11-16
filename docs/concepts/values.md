# Values cascade

Everything a cluster runs is set in plain Helm values files under `values/`, called layers. Each cluster has a fleet file, `fleet/<cluster>.yaml`, that names the Git revision it runs and the layers it gets ([Fleet files](overview.md#fleet-files)). Argo CD renders every element (one product's configuration, packaged as a Helm chart under `stack/`; [How an element is built](elements.md)) for that cluster with the same ordered list. Later files win.

## Layers

A layer is an ordinary Helm values file anywhere under `values/`. Nothing is matched by name, label or directory: a cluster gets a layer only when its fleet file lists it in `valueFiles`, as a path relative to `values/`.

```yaml
# fleet/prod-east-1.yaml
revision: main
valueFiles:              # lowest precedence first
  - environments/prod.yaml
  - datacenters/dc1.yaml
  - platforms/aws.yaml
```

- **Directories are conventions.** `environments/`, `datacenters/`, `platforms/`, `tenants/` and `acm/` mean nothing to the tooling. Add `values/regions/emea.yaml` and list it, and it works the same way.
- **Only `values/clusters/<cluster>.yaml` is special.** `<cluster>` is the fleet file's name, which is the ManagedCluster name (a hub uses its `hubName`, see the Hub tab under [Order](#order)). The file is appended to every cluster's list automatically, always last, so it overrides every shared layer. Create it only for clusters that change something.
- **One file configures many elements.** Each element, enabled or not, gets its own Application per cluster, named `<element>-<cluster>`. A disabled one syncs nothing. Every Application receives the full list. An element reads only its own `stack.<elementKey>` block plus the shared top-level keys, so `values/clusters/prod-east-1.yaml` turns on `infraNodes`, `machineHealth` and `userMonitoring` in one file.
- **Elements are off until a layer turns them on.** Every element ships `enabled: false` except `upgrade`, which renders nothing until the fleet file sets `channel` or `version`, or a layer sets `stack.upgrade.config.upstream` ([OpenShift upgrades](../guides/upgrades.md)). Inside an enabled element it is the reverse: every policy, template and operator is on unless the merged values set `enabled: false` ([What renders](elements.md#what-renders)).

!!! info "Which revision a layer is read at"
    Fleet files are read at HEAD of the default branch. The element charts and every values file are read at the fleet file's `revision` ([Fleet files](overview.md#fleet-files)). An edit to `values/environments/prod.yaml` therefore reaches only the clusters that list it **and** pin a revision containing the commit. A cluster pinned to a tag or SHA does not see a later edit, not even to its own `values/clusters/` file, until its pin moves ([Promoting and rolling back](../guides/rollout.md#promoting-and-rolling-back)).

## What goes in a layer

| Top-level key | Read by | What it does |
|---|---|---|
| `stack` | Elements | One block per element, keyed by the element's chart name in camelCase: `cert-manager` is `stack.certManager`, `infra-nodes` is `stack.infraNodes`. Holds `enabled`, `config`, `policies`, `operatorPolicies` and `default` ([How an element is built](elements.md)). |
| `global` | Every element and `lifecycle/` | Fleet-wide switches: dry run and catalog remapping ([Global keys](#global-keys)). |
| `placement` | Every element and `lifecycle/` | Extra settings merged into every Placement they render, the lifecycle Placement on the hub included ([Placement](policies.md#placement)). Never set `placement.predicates`: ACM ORs it with the cluster selector and sends this cluster's Policies to other clusters. A `claimSelector` guard ([Overriding the Placement](policies.md#overriding-the-placement)) in a layer that a built cluster lists also narrows its lifecycle Placement, so it must match the hub too. |
| `install` | `lifecycle/` only | Defaults for clusters a hub builds ([Install keys](../reference/lifecycle.md#install-keys)). Elements never read it, so a cluster you imported into ACM yourself, rather than one a hub built, can list `platforms/aws.yaml`. Its `install` block goes unused. |
| `policyNamespace` | Every element | Set once in the repo root `values.yaml`: `pstack`, the hub namespace every Policy goes in. Don't change it: `install/` creates the namespace from its own copy in `install/values.yaml`, so if the two differ the Policies target a namespace that doesn't exist. The validator (`tools/validator`, which CI runs on every pull request; [Validation](../develop/validation.md)) fails POLICY071 when they differ. |
| `version`, `channel` | The `upgrade` element, from the fleet file only | Rejected in a layer (POLICY034, [why](../guides/upgrades.md#how-it-works)). Set them in the fleet file. |
| `selector` | policy-library, injected by the ApplicationSet ([Order](#order)) | Rejected in a layer (POLICY035). If merged anyway, it narrows every Placement. |
| `policystack` | policy-library, injected by the ApplicationSet ([Order](#order)) | Rejected in a layer (POLICY035). |

Any other top-level key is rejected by the validator (POLICY035).

Element settings sit under `stack` because policy-library, the shared chart that renders the Policies and runs in the element's own context, reads `.Values.stack.<elementKey>`. Each element's `Chart.yaml` adds the library with `alias: stack`, so Helm checks that key against the library's [values schema](#schema).

```yaml
# values/clusters/nonprod-west-1.yaml
stack:
  metallb:
    enabled: true            # turns the element on
    policies:
      addressing:
        enabled: true        # turns on a policy the element ships off
  userMonitoring:
    enabled: true
    config:
      prometheus:
        retention: 72h       # a converter setting
```

## Order

The `policystack` ApplicationSet on the hub (an Argo CD object that generates one Application per element per cluster from the fleet files; [ApplicationSets](architecture.md)) builds each Application's `spec.source.helm`. You never write it yourself; you write the fleet file and the layers. Below is what Argo CD hands Helm, lowest precedence first.

=== "Managed cluster"

    Application `infra-nodes-prod-east-1`, generated from the fleet file above plus `channel: stable-4.20`:

    ```yaml
    valueFiles:
      - values.yaml                             # (1)!
      - ../../values.yaml                       # (2)!
      - ../../values/environments/prod.yaml     # (3)!
      - ../../values/datacenters/dc1.yaml
      - ../../values/platforms/aws.yaml
      - ../../values/clusters/prod-east-1.yaml  # (4)!
    valuesObject:                               # (5)!
      channel: "stable-4.20"                    # (6)!
      policystack:
        libraryMajors: [4]                      # (7)!
      selector:                                 # (8)!
        matchExpressions:
          name: {key: name, operator: In, values: ["prod-east-1"]}
    ```

    1.  The element's defaults, `stack/infra-nodes/values.yaml`. Paths are relative to the element directory.
    2.  The repo root `values.yaml`, read by every Application of every cluster. It holds only `policyNamespace`. Keep it that way: put fleet-wide settings in a layer every fleet file lists, so a cluster can leave it out.
    3.  One entry per `valueFiles` entry, in list order. Nothing sorts them.
    4.  Always appended, always last, named after the ManagedCluster. Skipped when it doesn't exist ([Missing files](#missing-files)).
    5.  Injected values. Argo CD ranks `valuesObject` above every file, so no layer can override these keys. Nothing else is injected: an element that needs to know the environment reads a key that `environments/<env>.yaml` sets.
    6.  `version` and `channel`, only when the fleet file sets them. The `upgrade` element reads them ([OpenShift upgrades](../guides/upgrades.md)).
    7.  The policy-library majors this hub's install chart supports, set in `install/templates/_helpers.tpl`, not by you. policy-library fails the render when the revision's library major isn't listed, so a pin to a revision built for another major shows a ComparisonError that names the fix instead of rendering under ApplicationSets it wasn't built for ([Library changes](../contributing.md#library-changes)). A plain `helm template` sets nothing and checks nothing.
    8.  policy-library turns this into the selector of each Placement the element renders, so its Policies reach this one cluster ([Placement](policies.md#placement)). It is a map, so Helm would merge a layer's `selector` entries into it, ANDed with `name`. No layer can change `name`, because `valuesObject` outranks every file.

=== "Hub"

    ACM registers the hub as a ManagedCluster of itself, named `local-cluster` and labeled `local-cluster`. Every hub gets that name by default, so the name can't tell hubs apart. For that cluster the ApplicationSet uses `hubName` instead, the name you gave this hub at install (`--set hubName=`): it reads `fleet/hubs/<hubName>.yaml` instead of `fleet/<cluster>.yaml`, and uses `hubName` where it would use the cluster name in two places: the Application name and the cluster values file ([The hub](architecture.md#the-hub)). The Placement selector still uses the ManagedCluster name, `local-cluster` unless ACM's `localClusterName` changes it.

    ```yaml
    # fleet/hubs/acm-dc1.yaml
    revision: main
    valueFiles:
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/baremetal.yaml
      - acm/acm-dc1.yaml         # (1)!
    ```

    1.  Listed by hand, like any layer. `acm/` is a convention for hub-only settings, such as the `acm` and `gitops` elements that manage the hub's own ACM and Argo CD.

    ```yaml
    # Application nfd-acm-dc1: element nfd, which values/clusters/acm-dc1.yaml enables on this hub
    valueFiles:
      - values.yaml
      - ../../values.yaml
      - ../../values/environments/prod.yaml
      - ../../values/datacenters/dc1.yaml
      - ../../values/platforms/baremetal.yaml
      - ../../values/acm/acm-dc1.yaml
      - ../../values/clusters/acm-dc1.yaml   # never clusters/local-cluster.yaml
    valuesObject:
      policystack:
        libraryMajors: [4]
      selector:
        matchExpressions:
          name: {key: name, operator: In, values: ["local-cluster"]}
    ```

=== "Lifecycle"

    Only for clusters a hub builds; skip this tab if you import your clusters. A fleet file that sets `hub` (the `hubName` of the hub that should build this cluster) gets a lifecycle Application, `lifecycle-<cluster>`, which builds the cluster on that hub ([Cluster lifecycle](lifecycle.md)). It reads the same layers, relative to `lifecycle/`: `values.yaml`, `../values.yaml`, `../values/<entry>` for each `valueFiles` entry, then `../values/clusters/<fleet file name>.yaml`. The cluster file is always named after the fleet file: the `hubName` swap from the Hub tab never applies.

    The injected `valuesObject` also carries the fleet file's lifecycle keys.

    ```yaml
    valuesObject:
      hub: "acm-dc1"
      state: "present"   # (1)!
      version: "4.20.8"  # (2)!
      install: {}        # (3)!
      policystack:
        libraryMajors: [4]
      selector:
        matchExpressions:
          hub:           # (4)!
            key: name
            operator: In
            values:
              - "local-cluster"
    ```

    1.  `present` when the fleet file sets no `state`.
    2.  `channel` too, when set.
    3.  The fleet file's `install`, `{}` when it sets none. It ranks above every layer and merges into their `install` key by key: `platforms/agent.yaml` sets the platform and credential, and the fleet file adds `install.nodePools`.
    4.  The hub that builds the cluster, not the cluster itself ([Placement: Lifecycle](policies.md#lifecycle)).

    `lifecycle/` reads `install` and, under `stack`, only the block of the engine `install.engine` picks (`stack.hive`, `stack.hypershift` or `stack.siteconfig`), so one cluster file can hold both halves:

    ```yaml
    # values/clusters/hcp-agent.yaml
    install: ...   # lifecycle/ builds the cluster
    stack: ...     # elements configure it once it is imported
    ```

    The schema still checks the whole `stack` key in `lifecycle/` too, so a typo in an element block fails the lifecycle Application as well ([Schema](#schema)).

### Worked example

The layers for `prod-east-1` merge into this `stack.infraNodes`, trimmed:

```yaml
enabled: true                 # clusters/prod-east-1.yaml, default false
policies:
  aws:
    enabled: true             # platforms/aws.yaml, default false
  vmware:
    enabled: false            # platforms/aws.yaml and element default
  workloads:
    enabled: false            # element default
  restore:
    enabled: false            # element default
config:
  replicas: 1                 # clusters/prod-east-1.yaml, default 3
  zones:                      # clusters/prod-east-1.yaml, replaces default [] (lists replace, see Merge rules)
    - us-east-1a
    - us-east-1b
    - us-east-1c
  instanceType: m6i.2xlarge   # clusters/prod-east-1.yaml
  maxPods: 250                # element default
```

Application `infra-nodes-prod-east-1` renders, in `pstack` on the hub, Placement and PlacementBinding `infra-nodes-prod-east-1` and Policies `mcp-infra-nodes-prod-east-1`, `config-infra-nodes-prod-east-1`, `aws-infra-nodes-prod-east-1` and `ready-infra-nodes-prod-east-1`. `vmware`, `workloads` and `restore` render nothing because the merged values leave them `enabled: false`. Where those names come from: [Rendered names](elements.md#rendered-names).

## Missing files

Every Application sets `ignoreMissingValueFiles: true`: Argo CD skips any listed file that doesn't exist at the cluster's revision. It has to, because `values/clusters/<cluster>.yaml` is appended for every cluster and most clusters have none. The price is that a typo or a file the pinned revision lacks is skipped too.

!!! warning "A skipped file raises no error"
    The Application stays Synced and Healthy and renders from the remaining layers. What you see depends on what the lost file did:

    - It turned something on: that piece never appears.
    - It turned something off: the shared layer wins. If `values/clusters/prod-east-1.yaml` sets `remediations.enabled: false` and is skipped, remediations from `environments/prod.yaml` keeps enforcing.
    - It changed a setting: the lower layer's value applies. Skip `datacenters/dc2.yaml` on nonprod-west-1 and the registry allowlist reverts to the element defaults, so pulls from `mirror.dc2.example.com` fail.

    The [validator](../develop/validation.md), which CI runs on every pull request in your repository, catches most causes ([Validator rules](../reference/rules.md)):

    | Cause | Validator |
    |---|---|
    | Typo in a `valueFiles` entry | POLICY050 |
    | Entry for a file the cluster's pinned `revision` lacks | POLICY052 |
    | `values/clusters/` file no fleet file reads, such as a misnamed one | POLICY052 (warning: reported on the pull request, doesn't fail it; [Reading the output](../develop/validation.md#reading-the-output)) |

To check by hand, compare the files the Application lists with the files at the cluster's revision. Any listed `../../values/<path>` with no matching `values/<path>` in the second output was skipped:

```sh
oc get applications.argoproj.io infra-nodes-prod-east-1 -n openshift-gitops \
  -o jsonpath='{.spec.source.helm.valueFiles}'
git fetch origin
git ls-tree -r --name-only origin/main -- values/   # the fleet file's revision: branch, tag or SHA
```

## Merge rules

Helm merges the files in order:

- **Scalars:** the last file that sets the key wins.
- **Maps:** merge key by key, so a layer sets only what changes.
- **Lists:** replace whole. A layer can't append to a list; it restates every item it keeps.
- **Removing:** a higher layer can't take back a key a lower layer set. `null` doesn't restore the element default: the converter sees no value (Helm 3.17 and later keep the null under `stack`, earlier versions delete the key). To drop a `policies`, `templates` or `operatorPolicies` entry, set `enabled: false`. To drop a `config` map entry, such as a MetalLB pool, move it out of the shared layer into the cluster files that need it.

That is why collections a layer must extend are maps keyed by name: `policies`, `templates`, `operatorPolicies`, and element settings such as MetalLB's `ipAddressPools` or team-gitops' `teams`.

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

Common changes, each settable from any layer:

| Change | Key |
|---|---|
| An element on or off | `stack.remediations.enabled`. Off unless `true` |
| A policy on or off | `stack.remediations.policies.registries.enabled`. On unless `false` |
| A template on or off | `stack.gitops.policies.link.templates.argocd-console.enabled`. On unless `false` |
| An operator on or off | `stack.metallb.operatorPolicies.metallb.enabled`. `false` stops managing it; it stays installed ([Removal and pruning](removal.md)) |
| One element in inform | `stack.infraNodes.default.remediationAction: inform`. Every policy of the element that sets no action of its own |
| One policy in inform | `stack.infraNodes.policies.mcp.remediationAction: inform`. Beats the element's `default` |
| Everything in inform | `global.remediationAction: inform` ([Global keys](#global-keys)) |
| A converter setting | `stack.<el>.config.<key>`, or `policies.<p>.templates.<t>.config.<key>` for one template only. The template's `config` is merged over the element's ([How an element is built](elements.md)). Keys and defaults are in the element's `values.yaml` and README |
| One operator field | `stack.metallb.operatorPolicies.metallb.versions`. `versions` is a list and replaces ([Operator versions](../guides/operators.md)) |

Use `false` to turn a piece off, never `null`: null counts as unset, which turns a policy on and an element off ([What renders](elements.md#what-renders)).

## Schema

Helm merges every layer first, then validates the result once against policy-library's [values.schema.json](https://github.com/PolicyStack/PolicyStack/blob/main/lib/policy-library/values.schema.json), which covers the `stack` key and `global` (Helm passes top-level `global` to every subchart, the library included). A misspelled or mistyped library key fails the render. The wording depends on the Helm version:

=== "Helm 3.18.4 and earlier (OpenShift GitOps 1.18, CI)"

    ```text
    Error: values don't meet the specifications of the schema(s) in the following chart(s):
    stack:
    - certManager: Additional property enabeld is not allowed
    ```

=== "Helm 3.18.5 and later"

    ```text
    Error: values don't meet the specifications of the schema(s) in the following chart(s):
    stack:
    - at '/certManager': additional properties 'enabeld' not allowed
    ```

Argo CD shows the Application as `ComparisonError` and neither syncs nor prunes, so the hub keeps the Policies it had ([When an Application fails](architecture.md#when-an-application-fails)). Every element, and `lifecycle/`, validates the whole `stack` key, so one typo in a shared layer fails every Application that reads it.

!!! warning "Never set `skipSchemaValidation` on the ApplicationSet"
    The schema is the only render-time check that rejects unknown library keys such as `enabeld`. Without it `enabeld: false` merges silently and does nothing, and a policy you meant to turn off keeps enforcing. A loud `ComparisonError` that changes nothing on the hub is the safer failure.

Both checks run before merge. The validator renders every element on the pull request, so a schema failure fails CI (RENDER000, and POLICY035 at the layer that set the key). Argo CD runs the schema again at sync.

The schema checks library keys only. POLICY035 also checks every `.yaml` file under `values/` and the root `values.yaml`, used by a fleet file or not. Name layers `.yaml`: Argo CD reads a listed `.yml` file, but POLICY035 skips it.

| Mistake | Caught by |
|---|---|
| Unknown key in an element (`stack.certManager.enabeld`) | schema, POLICY035 |
| Wrong type for a library key (`enabled: "yes"`) | schema |
| `global.remediationAction: enforce` | schema |
| Unknown top-level or `global` key | POLICY035 |
| Unknown element (`stack.certManagr`) | POLICY035 |
| `global` nested under `stack` (`stack.global.remediationAction`): ignored by the render | POLICY035 |
| `policies` key that names no policy of the element | render, POLICY035 |
| `templates` key that names no template and sets no `file` | render, POLICY035 |
| `operatorPolicies` key that names no operator of the element | render, POLICY032 |
| Top-level `config` key absent from the element's `values.yaml` | POLICY035 |
| Deeper `config` keys, and the type of any `config` value | nothing: `replicas: three` renders and fails later, in the converter or when ACM applies the object |

The render errors name the fix, for example `policy-library: policies.mcp.templates.ghost has no file - add converters/mcp/ghost.yaml or set file` ([Names that don't exist](elements.md#names-that-dont-exist)).

## Global keys

`global` is a top-level key that policy-library reads for every element and for `lifecycle/`. One setting changes every Policy of every element, on the clusters that read the layer. Set it in a cluster file for one cluster, or in a shared layer for a site or an environment. Under `stack` it is ignored.

| Key | Effect |
|---|---|
| `remediationAction` | `inform` sets every Policy to report instead of enforce, lifecycle Policies included, overriding the element and policy settings ([Dry run](../guides/rollout.md#dry-run)). `enforce` is rejected by the schema: it would also enforce templates built only to report, such as readiness checks. To undo a shared layer's `inform` for one cluster, set `remediationAction: ""` in its cluster file; each Policy's own action applies again. |
| `catalogSources` | Maps an operator's CatalogSource name to its replacement, for every operator of every element ([Disconnected clusters](../guides/disconnected.md)) |

```yaml
global:
  remediationAction: inform
  catalogSources:
    redhat-operators: cs-redhat-operator-index-v4-20
```

## Example files

Upstream's [values/](https://github.com/PolicyStack/PolicyStack/tree/main/values) holds examples for the example clusters in the docs, and the layers the validator's test fleet files in `tools/validator/testdata/clusters/` read. Your `values/` starts with only the example files you picked at [install](../start/install.md#example-files), which are yours from then on: updates never merge upstream's changes into them. Only an [upgrade step](../reference/launcher.md#upgrade-steps) renames a key in them, in its own commit on the update branch. Copy another by hand if it fits, and read it first.

!!! warning "The examples enforce"
    Listing a layer applies it on the next sync, and the elements it turns on enforce, so a cluster that lists a copy of `environments/prod.yaml` changes at once. Start a new cluster with an empty `valueFiles`, or set `global.remediationAction: inform` in its cluster file first.

??? info "What each example file holds"
    | File | Holds |
    |---|---|
    | `environments/prod.yaml` | Prod baseline: cert-manager, External Secrets, Compliance Operator, remediations (samples on, registry allowlist off), DNS |
    | `environments/nonprod.yaml` | Nothing |
    | `datacenters/dc1.yaml` | `install.baseDomain` only, so nothing for an imported cluster |
    | `datacenters/dc2.yaml` | Registry allowlist through the dc2 mirror. Applies only where a cluster enables `remediations` |
    | `platforms/aws.yaml` | Node policies: clone the worker MachineSet. `install`: Hive |
    | `platforms/vmware.yaml` | Node policies: clone each MachineSet per failure domain. `install`: Hive |
    | `platforms/baremetal.yaml` | Node policies: no MachineSets, label nodes. `install`: SiteConfig |
    | `platforms/kubevirt.yaml` | `install` only: `KubeVirt`, 2-node `workers` pool |
    | `platforms/agent.yaml` | `install` only: `Agent`, no default NodePool |
    | `tenants/payments.yaml` | GitOps operator plus a payments team Argo CD |
    | `acm/acm-dc1.yaml` | Hub ACM and GitOps, commented out ([why](../start/install.md#hand-the-operators-to-policystack)) |
    | `clusters/acm-dc1.yaml` | Hub elements, and which are off and why (`cim` for SiteConfig clusters) |
    | `clusters/prod-east-1.yaml` | Infra nodes, machine health, user monitoring |
    | `clusters/hcp-agent.yaml` | `install`: hosted control plane service publishing. `stack`: guest MetalLB pool |
    | `clusters/nonprod-west-1.yaml` | Registry enforcement, MetalLB pool, user monitoring |
    | `clusters/aws-prod.yaml` | ODF to Loki chain. Rename to the real ManagedCluster name first |

    `tenants/payments.yaml` and `clusters/aws-prod.yaml` are only key-checked (POLICY035) until a fleet file lists them. Render errors in them show up on that pull request.
