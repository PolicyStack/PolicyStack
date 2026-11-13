# policy-library

Helm library chart that renders ACM Policies, Placements and PolicySets for a PolicyStack element.
An element drops manifests into `converters/<policy>/<template>.yaml`; `values.yaml` holds settings and
exceptions.

## Usage

```yaml
# Chart.yaml
dependencies:
  - name: policy-library
    alias: stack
    version: "^4.0.0"                            # floor: the lowest 4.x the element needs
    repository: "file://../../lib/policy-library"  # relative to the element directory
```

```yaml
# templates/policy.yaml
{{- include "policy-library.render" . -}}
```

`alias: stack` turns on `values.schema.json`, which validates `.Values.stack`. Without the alias nothing is
validated. Unknown keys and wrong types fail the render:

```
stack:
- at '/myElement': additional properties 'enabeld' not allowed
```

Values live under `stack.<chartName>`, with the chart name camelCased (`node-feature-discovery` →
`nodeFeatureDiscovery`). Helm adds `stack.global`, so don't `range` over `.Values.stack`.

## Layout

```
my-element/
├── Chart.yaml
├── values.yaml
├── templates/policy.yaml
└── converters/
    ├── install/
    │   └── my-element-instance.yaml   # Policy install, template my-element-instance
    ├── config/
    │   ├── settings.yaml              # Policy config, template settings
    │   └── _labels.yaml               # partial: not discovered, usable through file
    └── _shared/                       # partial directory: not discovered
        └── monitoring.yaml
```

```yaml
stack:
  myElement:
    enabled: true
    default:
      remediationAction: enforce
      severity: medium
      categories: [CM Configuration Management]
    config:                                  # .Config in every converter
      image: registry.example.com/app:1.0
    policies:                                # only for overrides
      install:
        templates:
          my-element-instance:
            waitForOperator: my-operator
      config:
        templates:
          monitoring:
            file: _shared/monitoring         # reuse a file
            config: {retention: 7d}          # merged over config for this template
    operatorPolicies:                        # always render into Policy install
      my-operator:
        namespace: my-operator
        clusterMonitoring: true
        subscription: {name: my-operator, channel: stable}
```

## Discovery

- **Policies** are the directories under `converters/`, the keys of `policies`, and `install` when
  `operatorPolicies` is non-empty. They render in name order.
- **Templates** of a policy are `converters/<policy>/*.yaml` plus the keys of `policies.<policy>.templates`.
  They render in name order, after any listed in `orderManifests`.
- A directory or file whose name starts with `_` is a partial and never discovered. Files directly under
  `converters/` and `.yml` files aren't discovered either.
- Each template is one ConfigurationPolicy named `<policy>-<template>`.
- `templates.<t>.file` replaces the discovered file: a path under `converters/` without `.yaml`, or a list of
  them. A list concatenates the objects of each file.
- Every policy, template, operator and policySet renders unless it sets `enabled: false`.
- A policy with nothing enabled renders no Policy and no PlacementBinding subject.

These fail the render, so a values layer can't name something that doesn't exist:

| Condition | Error |
|---|---|
| Template with no file | `policies.<p>.templates.<t> has no file` |
| `file` that doesn't exist or is empty | `converters/<path>.yaml, a file of <p>-<t>, is missing or empty` |
| Policy with no templates | `policies.<p> has no templates` |
| `orderManifests` entry that isn't a template | `policies.<p>.orderManifests names "<t>"` |
| `orderManifests` entry listed twice | `policies.<p>.orderManifests lists "<t>" twice` |
| `install` template with `waitForOperator` under `policies.install.orderManifests` | `policies.install.orderManifests chains operators after templates` |
| Operator without `subscription.name` | `operatorPolicies.<k> has no subscription.name` |
| `waitForOperator` naming no enabled operator | `waitForOperator "<k>" on policy "<p>" matches no enabled entry` |
| `waitForOperator` with `policies.install.enabled: false` | `waitForOperator "<k>" on policy "<p>" needs policies.install, which is disabled` |

The `has no file`, `has no templates` and `orderManifests` checks run before `enabled` is applied, so disabling
a template that doesn't exist fails too.

## Defaults

What you leave out:

| Key | Resolves from | Fallback |
|---|---|---|
| Policy `remediationAction` | `policies.<p>` → `default` | `inform` |
| Template `remediationAction` | `InformOnly` if set, otherwise the Policy's action | |
| `severity` | template or operator → `policies.<p>` → `default` | `low` |
| `categories`, `controls`, `standards` | `policies.<p>` → `default` | none |
| `complianceType` | template or operator | `musthave` |
| `ignorePending` | template or operator | `true` with `waitForOperator`, else unset |
| `disabled` | `policies.<p>` | `false` |
| `subscription.source` | operator | `redhat-operators` |
| `subscription.sourceNamespace` | operator | `openshift-marketplace` |
| `upgradeApproval` | operator | `Automatic` |
| `operatorGroup.name` | operator | `subscription.name`, cluster-scoped |
| `displayName` (CSV status check) | operator | `subscription.name` |

`global.remediationAction: inform` overrides every Policy's action, which ACM applies to all its templates: a
dry run for any layer. `enforce` is rejected by the schema.

> [!NOTE]
> Compliance annotations have no library fallback. Leave them out and the Policy carries none.

## Converters

A converter is a Helm template rendered with the chart's root context plus `.Config`: the element's `config`
merged with `templates.<t>.config`. `.Values`, `.Release` and `.Files` work as usual.

The rendered output picks the field:

| Trimmed output | Rendered as |
|---|---|
| Starts with `-` or `[` | `object-templates-raw`, verbatim |
| Starts with `{{` (an escaped ACM template) | `object-templates-raw`, verbatim |
| Empty | `object-templates-raw: []` |
| Anything else | `object-templates`, one entry per column-0 `---` document |

- Empty documents are dropped, so `{{- if }}` around a whole document is safe.
- Full-line YAML `#` comments stay in the output but don't pick the shape. A document of only comments is dropped.
- `complianceType`, `metadataComplianceType`, `recordDiff`, `recreateOption` and `objectSelector` apply to
  every document of the template.
- A raw list must be the template's only output. A raw file in a multi-entry `file` list fails the render.
- A raw converter emits the full `- complianceType: / objectDefinition:` entries itself, and ACM resolves the
  list as one string. Use it only when the managed cluster decides how many objects exist (ACM `range` over
  `lookup`), or whether an object or field exists and `skipObject` in a field can't express it; when
  several ACM statements compute a number or boolean; when objects share ACM variables or must apply all or
  nothing; or when objects need different `complianceType`. A Helm `range` or `if` renders documents.

Braces meant for ACM are escaped from Helm. Here Helm resolves the `range` over values at render time, and the
escaped `lookup` reaches the managed cluster:

```yaml
{{- range $zone, $cfg := .Config.zones }}
---
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  name: {{ $.Config.role }}-{{ $zone }}
  namespace: openshift-machine-api
  labels:
    machine.openshift.io/cluster-api-cluster: '{{ "{{" }} (lookup "config.openshift.io/v1" "Infrastructure" "" "cluster").status.infrastructureName {{ "}}" }}'
spec:
  replicas: {{ $cfg.replicas }}
{{- end }}
```

`disableTemplating` is unrelated. It sets `policy.open-cluster-management.io/disable-templates` for manifests
whose `{{ }}` belongs to another engine. It works on documents only: ACM never parses `object-templates-raw` with
templates disabled, so a raw list with `disableTemplating` fails the render.

## Operators

`operatorPolicies` is a map, so a layer can set one field of one operator (`operatorPolicies.oadp.versions`).
Every operator renders into Policy `install`, after the install templates, as three templates:
`install-<key>-ns`, `install-<key>-status` and the OperatorPolicy `install-<key>`. They take the install
Policy's action and severity; the status check is always `inform`.

- `clusterMonitoring: true` labels the Namespace `openshift.io/cluster-monitoring: "true"`.
- `global.catalogSources` maps a source, defaulted or explicit, to another CatalogSource, e.g.
  `redhat-operators: cs-redhat-operator-index-v4-20` for an oc-mirror catalog.
- `operatorGroup.name` reuses an existing OperatorGroup (`global-operators` in `openshift-operators`). A second
  OperatorGroup in one namespace breaks OLM.
- `enabled: false` stops managing the operator. It doesn't uninstall it.

### Removing an operator

`complianceType: mustnothave` uninstalls it through the OperatorPolicy's `removalBehavior` (ACM defaults:
`operatorGroups: DeleteIfUnused`, `subscriptions` and `clusterServiceVersions: Delete`,
`customResourceDefinitions: Keep`). No namespace template is rendered, so other operators in a shared namespace
are untouched. The status check reports NonCompliant until the CSV is gone. `waitForOperator` on it fails the
render. Remove the operands first.

### Version pinning

| `upgradeApproval` | `versions` | Effect |
|---|---|---|
| `Automatic` | empty | Installs and follows the channel |
| `Automatic` | set | Installs and upgrades only to listed CSVs or `subscription.startingCSV` |
| `None` | any | Initial install only; never approves an upgrade |

- Entries are exact CSV names (`oadp-operator.v1.4.2`). Comma-separated entries need ACM 2.14+.
- Set `subscription.startingCSV` when the pin isn't the channel head.
- Bump in two steps: list old and new, promote through the layers, then drop the old and move `startingCSV`.
- `Manual` isn't valid; ACM rejects `subscription.installPlanApproval`.

## Ordering and dependencies

| Key | Renders |
|---|---|
| `policies.<p>.dependencies` | `Policy.spec.dependencies` |
| `templates.<t>.extraDependencies`, `operatorPolicies.<k>.extraDependencies` | `policy-templates[].extraDependencies` |
| `waitForOperator: <key>` (string or list) | Dependency on `install-<key>-status`, plus `ignorePending: true` |
| `policies.<p>.orderManifests: [t1, t2]` | `t1`, `t2` render first in that order; every template after them, then the operators, waits on the one before |

Operators chain as `-ns` → OperatorPolicy → `-status`, so the next entry waits for the CSV to reach
`Succeeded`. An operator that waits on another needs a key that sorts after it, or the chain deadlocks.
With `policies.install.orderManifests`, an `install` template that sets `waitForOperator` would deadlock the
same way and fails the render. Put ordered templates in another policy.

Dependency entry fields:

| Field | Meaning | Default |
|---|---|---|
| `name` | Name as written in values; the chart applies its naming rules | required |
| `kind` | `Policy`, `ConfigurationPolicy`, `OperatorPolicy`, or any kind with `apiVersion` | `Policy` in `dependencies`, `ConfigurationPolicy` in `extraDependencies` |
| `compliance` | `Compliant`, `NonCompliant`, `Pending` | `Compliant` |
| `policyRef` | Policy owning a template-kind target | the declaring policy |
| `element` | Policy of another element on the same cluster: `<name>-<element>-<cluster>` | |
| `release` | Policy of an exact release: `<name>-<release>` | `.Release.Name` |
| `raw` | Use `name` verbatim | `false` |
| `namespace` | Override | `policyNamespace` for `Policy`, otherwise none |
| `apiVersion` | Override | from `kind` |

Precedence for `kind: Policy` is `raw` > `release` > `element` > `.Release.Name`. `element` takes the cluster
from `Release.Name` minus `<chart>-`, which is how PolicyStack's ApplicationSet names releases, and fails the
render otherwise. It's only valid for `kind: Policy`: template kinds live in each cluster's namespace and carry
no release suffix.

## Reference

### Root

| Key | Description |
|---|---|
| `policyNamespace` | Namespace for every object. Required. |
| `selector.matchExpressions.<key>` | Label expression (`key`, `operator`, `values`) for the Placement. A map, so layers merge. |
| `placement` | Extra `Placement.spec`. See [Placement](#placement). |
| `global.remediationAction` | `inform` makes every Policy inform. |
| `global.catalogSources` | Map of `subscription.source` to the CatalogSource to use. |

### `stack.<element>`

| Key | Description |
|---|---|
| `enabled` | Must be `true` for anything to render. |
| `config` | Element settings, read as `.Config`. Not validated. |
| `default` | `remediationAction`, `severity`, `categories`, `controls`, `standards`. |
| `policies.<p>` | Policy options, below. |
| `operatorPolicies.<k>` | Operator options, below. |
| `policySets[]` | `name`, `enabled`, `description`, `policies` (policy names). |
| `usePolicySetsPlacements` | Bind the PolicySets instead of the Policies. |
| `disablePlacements` | Render no Placement or PlacementBinding. |
| `suffixTemplateNames` | Append `-<cluster>` to every template name, for releases that all land on one cluster (the hub). |

### `policies.<p>`

| Key | Description |
|---|---|
| `enabled` | `false` drops the Policy. |
| `description` | `description` annotation. |
| `remediationAction` | `inform` or `enforce`. |
| `severity`, `categories`, `controls`, `standards` | Override `default`. |
| `disabled` | ACM-side `spec.disabled`. |
| `dependencies` | Dependency entries. |
| `orderManifests` | Template names that render first and chain the policy. |
| `hubTemplateOptions.serviceAccountName` | ServiceAccount resolving `{{hub ... hub}}` lookups outside the policy namespace. |
| `templates.<t>` | Template options, below. |

### `policies.<p>.templates.<t>`

| Key | Description |
|---|---|
| `enabled` | `false` drops the template. |
| `file` | Path under `converters/` without `.yaml`, or a list. |
| `config` | Merged over the element `config` for this template. |
| `description` | `description` annotation. |
| `remediationAction` | `InformOnly` only. |
| `severity` | Overrides the policy's. |
| `complianceType` | `musthave`, `mustonlyhave`, `mustnothave`. |
| `metadataComplianceType`, `recordDiff`, `recreateOption`, `objectSelector` | Per object-template, passed through. |
| `namespaceSelector` | For namespaced objects without a namespace. |
| `pruneObjectBehavior` | `None`, `DeleteIfCreated`, `DeleteAll`. |
| `customMessage.compliant`, `customMessage.noncompliant` | Quote them when they hold templates. |
| `evaluationInterval.compliant`, `evaluationInterval.noncompliant` | Polling intervals. |
| `disableTemplating` | Stop ACM from evaluating `{{ }}`. |
| `waitForOperator` | Operator key, or list. |
| `ignorePending` | Compliant while waiting on dependencies. |
| `extraDependencies` | Dependency entries. |

### `operatorPolicies.<k>`

| Key | Description |
|---|---|
| `enabled` | `false` stops managing it. |
| `namespace` | Install namespace. |
| `subscription.name` | Package. Required. |
| `subscription.channel` | Channel. |
| `subscription.source`, `subscription.sourceNamespace` | Catalog. |
| `subscription.startingCSV` | First CSV to install. |
| `subscription.config` | Subscription config (tolerations, env, ...). |
| `operatorGroup.name`, `operatorGroup.targetNamespaces` | OperatorGroup. |
| `versions` | Allowed CSV names. |
| `upgradeApproval` | `Automatic` or `None`. |
| `complianceType` | `musthave` or `mustnothave`. |
| `removalBehavior` | `operatorGroups`, `subscriptions`, `clusterServiceVersions`, `customResourceDefinitions`. |
| `clusterMonitoring` | Label the Namespace for cluster monitoring. |
| `displayName` | CSV display name for the status check. |
| `description`, `severity` | As for templates. |
| `waitForOperator`, `ignorePending`, `extraDependencies` | As for templates. |

## Placement

Each release renders one Placement and, when something is bound, one PlacementBinding, both named after the
release in `policyNamespace`.

```yaml
selector:
  matchExpressions:
    environment:
      key: environment
      operator: In
      values: [dev]

placement:
  claimSelector:          # ANDed with selector
    matchExpressions:
      - key: platform.open-cluster-management.io
        operator: In
        values: [AWS]
  celSelector:            # ANDed with selector, ACM 2.14+
    celExpressions:
      - managedCluster.metadata.labels["vendor"] == "OpenShift"
  predicates: []          # ORed with the generated predicate
  # tolerations: []       # omit to tolerate unreachable/unavailable
  clusterSets: [global]   # any other Placement.spec field passes through
```

- Without `tolerations`, the Placement tolerates `cluster.open-cluster-management.io/unreachable` and
  `unavailable`, so Policies stay bound while a cluster is disconnected. `[]` drops disconnected clusters.
- A Placement only selects clusters from ManagedClusterSets bound to `policyNamespace`.
- Every Policy sets `copyPolicyMetadata: false`, so ACM doesn't copy Argo CD tracking metadata onto replicas.

## Public API

Within a major, changes only by addition, and a patch may change output only to fix a bug. The full rules are in [Compatibility](https://policystk.io/reference/compatibility/#policy-library-versions). `lib/tests/` pins the output.

| Surface | Contract |
|---|---|
| Defines | `policy-library.render` (with the `.component` override), `policy-library.clusterSuffix` and `policy-library.enabled` (takes the entry, returns `"true"` or `""`). Every other `policy-library.*` define is internal and may change in any release. |
| Values | Every key in `values.schema.json`. A new key is a minor. |
| Converters | Discovery, `.Config` and the shape rules above. |
| Output | What existing values render. A new key may change output only when it is set, and a fix only where the output was a bug. |

## Compatibility

Floors match PolicyStack's: ACM 2.16 and Helm 3.18.4 (OpenShift GitOps 1.18). The chart lives in the
PolicyStack repo and ships with it: a change bumps `version` in `Chart.yaml` in the same PR, and an element
that needs the change raises its `^4.N.0` floor.

| Version | Change |
|---|---|
| 1.1.0 | `components` renamed to `stack` |
| 2.0.0 | Placement replaces PlacementRule |
| 3.0.0 | `operatorPolicies` is a map |
| 3.1.0 | `copyPolicyMetadata: false` on every Policy |
| 3.2.0 | Schema rejects unknown keys and wrong types |
| 4.0.0 | Converter discovery, maps, defaults chain, shape detection. See below. |
| 4.1.0 | Fails the render when `policystack.libraryMajors`, which PolicyStack's install chart injects, is set and lacks the library's major |
| 4.1.1 | Public API wording matches PolicyStack's compatibility rules. No output change |
| 4.1.2 | Fails the render when `disableTemplating` is set on a raw list, which ACM would never evaluate |

## Upgrading from 3.x

Rewrite each element and its values layers as the table shows, then review the Render Diff.
ConfigurationPolicy names don't change.

| 3.x | 4.0 |
|---|---|
| `policies[]` | `policies.<p>` map |
| `configPolicies[]` with `policyRef` and `templateNames` | `converters/<p>/<t>.yaml`, options in `policies.<p>.templates.<t>` |
| Several `templateNames` | One file with `---`, or `file: [a, b]` |
| `rawTemplate: true` | Output starting with `-` |
| `templateParameters`, `enableTemplateParameters`, `.Parameters` | `templates.<t>.config`, `.Config` |
| `.Values.stack.<element>.config` in converters | `.Config` |
| `toggles.<name>` | `enabled: false` on the policy, template or operator |
| `enabled: true` on entries | Omit |
| `orderPolicies` | `policies.<p>.dependencies` |
| `orderManifests: true` | `policies.<p>.orderManifests: [<templates>]` |
| `operatorPolicies.<k>.policyRef` | Removed; operators render in `install` |
| `operatorPolicies.<k>.remediationAction` | `policies.install.remediationAction` |
| Template `remediationAction: inform` under an enforce Policy | `InformOnly` |
| Template `remediationAction: enforce` | Omit; it takes the Policy's |
| `certificatePolicies[]` | Removed |
| `policies[].namespace` | Removed (was unused) |
| Namespace monitoring-label converter | `clusterMonitoring: true` |

> [!WARNING]
> `Policy.spec.remediationAction` is always rendered, `inform` when nothing sets it, and every template takes
> it. A template that enforced under a Policy with no action now informs: set `policies.<p>.remediationAction`
> or `default.remediationAction`.
