# Policies

Elements render ACM objects with the [policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md) chart. Its README is the field reference. PolicyStack fixes three inputs: the values key, the release name, and the cluster selector.

## Element values

An element reads `stack.<key>`, where `<key>` is the camelCase form of `name` in its `Chart.yaml`: `metallb` reads `stack.metallb`, `user-monitoring` reads `stack.userMonitoring`. One file in the [values cascade](values.md#order) can configure many elements:

```yaml title="values/clusters/nonprod-west-1.yaml (excerpt)"
stack:
  metallb:
    enabled: true        # (1)!
    toggles:
      addressing: true   # (2)!
  userMonitoring:
    enabled: true
    config:
      prometheus:
        retention: 72h
```

1.  Nothing in the element renders until this is true.
2.  Overrides `enabled` on the entry named `addressing`. See [Toggles](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles).

Charts also read the top-level `policyNamespace`, `selector` and `placement`.

!!! note
    In an element's `values.yaml`, a key that doesn't match the chart name renders nothing (POLICY090). In a cascade file, a stack key or toggle that names no element or entry is reported as POLICY035.

## Rendered objects

The release name is the Application name, `<element>-<cluster>` ([Applications](applicationset.md#applications)). Below, `<policy>`, `<set>` and `<name>` are the `name` of a `policies[]`, `policySets[]` or template entry, or the key of an `operatorPolicies` entry.

| Object | Name | Rendered when |
|---|---|---|
| `Policy` | `<policy>-<release>` | Enabled, and at least one enabled template names it in `policyRef`. |
| `ConfigurationPolicy`, `CertificatePolicy` | `<policy>-<name>` | Enabled, and its `policyRef` Policy renders. Embedded in that Policy. |
| `OperatorPolicy` | `<policy>-<name>` | Same as above. Adds `<policy>-<name>-ns` (operator namespace, omitted for `mustnothave`) and `<policy>-<name>-status` (inform check that the CSV reached `Succeeded`). |
| `PolicySet` | `<set>-<release>` | Enabled, and `policies` is not empty. Members resolve to `<policy>-<release>`. |
| `Placement` | `<release>` | `disablePlacements` is not true. |
| `PlacementBinding` | `<release>` | `disablePlacements` is not true and there is something to bind: the Policies, or the PolicySets when `usePolicySetsPlacements` is true. |

Everything except templates lives on the hub in `policyNamespace` (`pstack`, root [`values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values.yaml)). Templates have no namespace; ACM creates them in the cluster's namespace on the managed cluster. The chart ignores `policies[].namespace`, although its README lists it.

Render metallb for `nonprod-west-1` from the repo root. `baseline.yaml` stands in for the injected selector ([Validation](validation.md)).

```sh
helm dependency update stack/metallb
helm template metallb-nonprod-west-1 stack/metallb \
  -f values.yaml \
  -f values/environments/nonprod.yaml \
  -f values/datacenters/dc2.yaml \
  -f values/clusters/nonprod-west-1.yaml \
  -f tools/validator/testdata/baseline.yaml
```

`addressing` is toggled on; `quota` stays off and renders nothing:

```text
Placement         metallb-nonprod-west-1
PlacementBinding  metallb-nonprod-west-1
Policy            install-metallb-nonprod-west-1
  OperatorPolicy        install-metallb
  ConfigurationPolicy   install-metallb-ns
  ConfigurationPolicy   install-metallb-status
Policy            config-metallb-nonprod-west-1
  ConfigurationPolicy   config-metallb-instance
Policy            addressing-metallb-nonprod-west-1
  ConfigurationPolicy   addressing-pools
  ConfigurationPolicy   addressing-l2
  ConfigurationPolicy   addressing-bgp
  ConfigurationPolicy   addressing-peers
```

## Placement

The injected `selector` ([Injected values](applicationset.md#injected-values)) becomes one `In` expression on the `name` label, so each Placement selects only its own cluster:

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

A top-level `placement:` key in any cascade file merges into that spec ([Placement overview](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#placement-overview)). A Placement only sees ManagedClusterSets bound to its namespace; the install chart binds `global` into `policyNamespace` ([Install PolicyStack](install.md#install-policystack)).

??? info "Lifecycle chart"
    [`lifecycle/`](lifecycle.md) Policies build clusters on a hub, so its `selector` is an `In` expression on the hub's `name` ([Lifecycle generator](applicationset.md#lifecycle-generator)). The `local-cluster` `Exists` default in `lifecycle/values.yaml` only serves `helm lint` and the validator.

    Every lifecycle release lands on the hub, where template names must be unique across all Policies. The chart sets [`suffixTemplateNames`](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#root-component-options) and templates are named `<policy>-<name>-<cluster>`, such as `present-credentials-hcp-agent`.

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

POLICY001 holds `<policyNamespace>.<policy>-<release>`, `<policyNamespace>.<set>-<release>` and every template name to 63 characters ([Rules](validation.md#rules)).

!!! tip "30-character budget"
    Every element fits a cluster or hub name of 30 characters; most fit 35. CI checks every policy in each element's `values.yaml`, enabled or not, against fixture cluster `name-budget-30-characters-long`.

Lifecycle: the longest name is `pstack.present-lifecycle-<cluster>`, 25 characters plus the cluster, so cluster names can reach 38. The longest template, `present-credentials-<cluster>`, fits at that length.

## Library reference

Chart README: [Policy dependencies](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#policy-dependencies), [Depending on another component](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#depending-on-another-component), [Toggles](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles), [Raw object templates](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#raw-object-templates), [Placement overview](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#placement-overview).
