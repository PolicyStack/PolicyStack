# Policies

Elements render their ACM objects with the [policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md) chart, its README is the field reference. PolicyStack fixes three inputs. The values key an element reads, the release name every object name builds on, and the selector that targets one cluster.

## Element values

An element reads `stack.<key>`, where `<key>` is the camelCase form of the chart name in its `Chart.yaml`. The `user-workload-monitoring` chart reads `stack.userWorkloadMonitoring`. Files in the [values cascade](values.md#order) use the same keys, so one cluster file configures many elements and each chart reads only its own key. The validator reports a mismatched key as POLICY090.

Every chart also reads the top-level keys `policyNamespace`, `selector` and `placement`. Nothing renders unless `stack.<key>.enabled` is true.

## Rendered objects

Names build on the release, which is the Application name ([Applications](applicationset.md#applications)). In the table, `<policy>`, `<set>` and `<name>` are the `name` fields of `policies[]`, `policySets[]` and the template lists. An entry is enabled when its `enabled` field is true, unless its [toggle](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles) overrides it.

| Object | Name | Rendered when |
|---|---|---|
| `Policy` | `<policy>-<release>` | The policy is enabled and at least one enabled template names it in `policyRef`. |
| `ConfigurationPolicy`, `CertificatePolicy` template | `<policy>-<name>` | The entry is enabled and the Policy its `policyRef` names renders. It is embedded in that Policy. |
| `OperatorPolicy` template | `<policy>-<name>` | The entry is enabled and the Policy its `policyRef` names renders. It adds two ConfigurationPolicy templates: `<policy>-<name>-ns` for the operator namespace and `<policy>-<name>-status`, an inform check that the operator's CSV reached `Succeeded`. |
| `PolicySet` | `<set>-<release>` | The set is enabled and its `policies` list is not empty. Members resolve to `<policy>-<release>`. |
| `Placement` | `<release>` | `disablePlacements` is not true. |
| `PlacementBinding` | `<release>` | `disablePlacements` is not true and there is a subject to bind. Subjects are the rendered Policies, or the rendered PolicySets when `usePolicySetsPlacements` is true. |

Policies, PolicySets, Placements and PlacementBindings live on the hub in `policyNamespace`, set to `policy` in the root [`values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values.yaml). The chart ignores `policies[].namespace`, although its README lists it. Templates have no namespace of their own. They sit inside the Policy's `policy-templates`, and ACM creates them in the cluster's namespace on the managed cluster.

## Placement

The chart turns the injected `selector` ([Injected values](applicationset.md#injected-values)) into the Placement's required cluster selector. It holds one expression, on the `name` label, so each Placement selects only its own cluster. The rendered predicate for `nonprod-west-1`:

```yaml
spec:
  predicates:
  - requiredClusterSelector:
      labelSelector:
        matchExpressions:
        - key: name
          operator: In
          values:
            - nonprod-west-1
```

A top-level `placement:` key in any cascade file merges into that spec ([Placement overview](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#placement-overview)).

An ACM Placement selects only clusters in ManagedClusterSets bound to its namespace. The appset chart binds `global` into `policyNamespace` ([Install the ApplicationSet](install.md#install-the-applicationset)).

The `lifecycle/` chart is the exception ([Cluster lifecycle](lifecycle.md)). Its Policies build clusters on the hub, so its `values.yaml` sets `selector` to one `Exists` expression on the `local-cluster` label, and no `selector` is injected. Every lifecycle release lands on the hub, where ACM requires template names to be unique across all Policies, so the chart sets the policy-library option [`suffixTemplateNames`](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#root-component-options). Its template names are `<policy>-<name>-<cluster>`, such as `present-credentials-hcp-agent`.

## Naming limits

ACM replicates each Policy into the cluster's namespace on the hub, and names and labels the copy `<policyNamespace>.<policy>-<release>`. A label value holds at most 63 characters. With `policyNamespace: policy`, `<policy>-<release>` gets 56, and the release spends part of that on the element and cluster names. A long element name shrinks the budget of every policy in that element, and a long cluster name that of every policy on that cluster, so keep policy names short.

The lifecycle release is `lifecycle-<cluster>` and its longest Policy is `present`, so the replicated name is `policy.present-lifecycle-<cluster>`: 25 characters plus the cluster name, which leaves the cluster name at most 38. Its longest template, `present-credentials-<cluster>`, is shorter than 63 for any such name.

Validator rule POLICY001 holds `<policyNamespace>.<policy>-<release>`, `<policyNamespace>.<set>-<release>` and every template name (`<policy>-<name>`) to 63 characters ([Rules](validation.md#rules)).

Worked example: metallb on `nonprod-west-1`, release `metallb-nonprod-west-1`. The cluster file turns on the `addressing` toggle, `quota` stays toggled off and renders nothing.

| Entry | Hub Policy | Replicated name | Characters |
|---|---|---|---|
| `install` | `install-metallb-nonprod-west-1` | `policy.install-metallb-nonprod-west-1` | 37 |
| `config` | `config-metallb-nonprod-west-1` | `policy.config-metallb-nonprod-west-1` | 36 |
| `addressing` | `addressing-metallb-nonprod-west-1` | `policy.addressing-metallb-nonprod-west-1` | 40 |

Templates. the OperatorPolicy `install-metallb` with its `install-metallb-ns` and `install-metallb-status`, then `config-metallb-instance`, `addressing-pools`, `addressing-l2`, `addressing-bgp` and `addressing-peers`. The Placement and PlacementBinding are both `metallb-nonprod-west-1`.

Render it from the repo root. The release name stands in for the Application name and `baseline.yaml` for the injected selector ([Validation](validation.md)).

```sh
helm dependency update stack/metallb
helm template metallb-nonprod-west-1 stack/metallb \
  -f values.yaml \
  -f values/environments/nonprod.yaml \
  -f values/datacenters/dc2.yaml \
  -f values/clusters/nonprod-west-1.yaml \
  -f tools/validator/testdata/baseline.yaml
```

## Library reference

Chart README: [Policy dependencies](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#policy-dependencies), [Depending on another component](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#depending-on-another-component), [Toggles](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles), [Raw object templates](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#raw-object-templates), [Placement overview](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#placement-overview).
