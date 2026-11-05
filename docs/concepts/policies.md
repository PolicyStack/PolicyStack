# Policies and placement

Elements render ACM objects with the [policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md) chart. Its README is the field reference. PolicyStack fixes three inputs: the values key, the release name, and the cluster selector.

## Element values

An element reads `stack.<key>`, where `<key>` is the camelCase form of `name` in its `Chart.yaml`: `metallb` reads `stack.metallb`, `user-monitoring` reads `stack.userMonitoring`. One file in the [values cascade](values.md#order) can configure many elements:

```yaml title="values/clusters/nonprod-west-1.yaml (excerpt)"
stack:
  metallb:
    enabled: true        # (1)!
    policies:
      addressing:
        enabled: true    # (2)!
  userMonitoring:
    enabled: true
    config:
      prometheus:
        retention: 72h
```

1.  Nothing in the element renders until this is true.
2.  Turns on policy `addressing`, which the element ships with `enabled: false`. Policies, templates and operators are on unless they set `enabled: false` ([Discovery](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#discovery)).

Charts also read the top-level `policyNamespace`, `selector`, `placement` and `global` ([Global keys](values.md#global-keys)).

!!! note
    In an element's `values.yaml`, a key that doesn't match the chart name renders nothing (POLICY090). In a cascade file, a `stack`, `policies` or `templates` key that names no element, policy or template is reported as POLICY035.

## Rendered objects

The release name is the Application name, `<element>-<cluster>` ([Applications](architecture.md#applications)). An element's policies are the directories under `converters/`, the keys of `policies`, and `install` when it has `operatorPolicies`. A policy's templates are the files in its directory and the keys of its `templates`; `_` names are partials, never discovered ([Discovery](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#discovery)):

```text
stack/metallb/                  # excerpt
├── values.yaml                 # stack.metallb: config, defaults, overrides
└── converters/
    ├── config/
    │   └── metallb-instance.yaml   # ConfigurationPolicy config-metallb-instance
    └── addressing/
        ├── pools.yaml              # ConfigurationPolicy addressing-pools
        └── l2.yaml
```

Below, `<policy>`, `<template>` and `<set>` are those names, `<key>` an `operatorPolicies` key.

| Object | Name | Rendered when |
|---|---|---|
| `Policy` | `<policy>-<release>` | Enabled, with at least one enabled template or operator. |
| `ConfigurationPolicy` | `<policy>-<template>` | Enabled, and its Policy renders. Embedded in that Policy. |
| `OperatorPolicy` | `install-<key>` | Enabled. Always in Policy `install`, with `install-<key>-ns` (operator namespace, omitted for `mustnothave`) and `install-<key>-status` (inform check that the CSV reached `Succeeded`). |
| `PolicySet` | `<set>-<release>` | Enabled, and `policies` is not empty. Members resolve to `<policy>-<release>`. |
| `Placement` | `<release>` | `disablePlacements` is not true. |
| `PlacementBinding` | `<release>` | `disablePlacements` is not true and there is something to bind: the Policies, or the PolicySets when `usePolicySetsPlacements` is true. |

Everything except templates lives on the hub in `policyNamespace` (`pstack`, root [`values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values.yaml)). Templates have no namespace; ACM creates them in the cluster's namespace on the managed cluster.

Render metallb for `nonprod-west-1` from the repo root. `baseline.yaml` stands in for the injected selector ([Validation](../develop/validation.md)).

```sh
helm dependency update stack/metallb
helm template metallb-nonprod-west-1 stack/metallb \
  -f values.yaml \
  -f values/environments/nonprod.yaml \
  -f values/datacenters/dc2.yaml \
  -f values/clusters/nonprod-west-1.yaml \
  -f tools/validator/testdata/baseline.yaml
```

`addressing` is turned on; `quota` stays off and renders nothing. Policies and templates render in name order:

```text
Placement         metallb-nonprod-west-1
PlacementBinding  metallb-nonprod-west-1
Policy            addressing-metallb-nonprod-west-1
  ConfigurationPolicy   addressing-bgp
  ConfigurationPolicy   addressing-l2
  ConfigurationPolicy   addressing-peers
  ConfigurationPolicy   addressing-pools
Policy            config-metallb-nonprod-west-1
  ConfigurationPolicy   config-metallb-instance
Policy            install-metallb-nonprod-west-1
  ConfigurationPolicy   install-metallb-ns
  ConfigurationPolicy   install-metallb-status
  OperatorPolicy        install-metallb
```

## Placement

The injected `selector` ([Order](values.md#order)) becomes one `In` expression on the `name` label, so each Placement selects only its own cluster:

```yaml title="Placement metallb-nonprod-west-1"
spec:
  predicates:
  - requiredClusterSelector:
      labelSelector:
        matchExpressions:
        - key: name
          operator: In
          values:
          - nonprod-west-1
  tolerations: # (1)!
  - key: cluster.open-cluster-management.io/unreachable
    operator: Exists
  - key: cluster.open-cluster-management.io/unavailable
    operator: Exists
```

1.  Chart default. Policies stay bound while the cluster is disconnected.

A top-level `placement:` key in any cascade file merges into that spec ([Placement](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#placement)). A Placement only sees ManagedClusterSets bound to its namespace; the install chart binds `global` into `policyNamespace` ([Install chart](../reference/install.md)).

??? info "Lifecycle chart"
    [`lifecycle/`](../guides/lifecycle.md) Policies build clusters on a hub, so its `selector` is an `In` expression on the hub's `name` ([Lifecycle generator](architecture.md#lifecycle-generator)). The `local-cluster` `Exists` default in `lifecycle/values.yaml` only serves `helm lint` and the validator.

    Every lifecycle release lands on the hub, where template names must be unique across all Policies. The chart sets [`suffixTemplateNames`](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#stackelement) and templates are named `<policy>-<template>-<cluster>`, such as `present-credentials-hcp-agent`.

## Naming limits

ACM copies each Policy into the cluster's namespace on the hub, named and labelled `<policyNamespace>.<policy>-<release>`. Label values max out at 63 characters.

```text
pstack.addressing-metallb-nonprod-west-1
│      │          │       └ cluster
│      │          └ element
│      └ policy
└ policyNamespace (7 with the dot, leaves 56)
```

Policy, element and cluster names share those 56 characters. Keep policy names short.

| Entry | Replicated name | Characters |
|---|---|---|
| `install` | `pstack.install-metallb-nonprod-west-1` | 37 |
| `config` | `pstack.config-metallb-nonprod-west-1` | 36 |
| `addressing` | `pstack.addressing-metallb-nonprod-west-1` | 40 |

POLICY001 holds `<policyNamespace>.<policy>-<release>`, `<policyNamespace>.<set>-<release>` and every template name to 63 characters ([Validator rules](../reference/rules.md)).

!!! tip "30-character budget"
    Every element fits a cluster or hub name of 30 characters; most fit 35. CI renders every fixture, `name-budget-30-characters-long` included, with each `testdata/profiles` file, which turns elements and their optional policies on ([ACM templates](../develop/validation.md#acm-templates)).

Lifecycle: the longest name is `pstack.present-lifecycle-<cluster>`, 25 characters plus the cluster, so cluster names can reach 38. The longest template, `present-credentials-<cluster>`, fits at that length.
