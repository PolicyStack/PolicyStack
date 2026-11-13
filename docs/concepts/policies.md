# Placement and naming

Every enabled element renders, for each cluster, its Policies, one Placement and one PlacementBinding, all in `pstack` on the hub. The Placement and PlacementBinding are named `<element>-<cluster>`, which is also the Helm release and the Argo CD Application name ([Applications](architecture.md#applications)). A Placement picks clusters; a PlacementBinding attaches Policies to it ([ACM policies in brief](acm.md)). Here every Placement picks exactly one cluster.

[Rendered names](elements.md#rendered-names) lists every name an element renders; [Naming limits](#naming-limits) covers how long they can be.

## Placement

The `policystack` ApplicationSet knows each Application's cluster from the Argo CD cluster secret it generated it from, so it writes the cluster's selector into the Application's `valuesObject` (Helm values inline in the Application, which outrank every values file, [Order](values.md#order)). policy-library, the Helm library chart every element depends on, turns each entry of the `selector.matchExpressions` map into one expression of the Placement's predicate: `name In (<cluster>)`. ACM sets the `name` label on every ManagedCluster to its own name.

=== "Injected"

    ```yaml
    # Application metallb-nonprod-west-1, spec.source.helm
    valuesObject:
      selector:
        matchExpressions:
          name: {key: name, operator: In, values: ["nonprod-west-1"]}
    ```

=== "Rendered"

    ```yaml
    apiVersion: cluster.open-cluster-management.io/v1beta1
    kind: Placement
    metadata:
      name: metallb-nonprod-west-1
      namespace: pstack
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
    ---
    apiVersion: policy.open-cluster-management.io/v1
    kind: PlacementBinding
    metadata:
      name: metallb-nonprod-west-1
      namespace: pstack
    placementRef:
      name: metallb-nonprod-west-1
      kind: Placement
      apiGroup: cluster.open-cluster-management.io
    subjects: # (2)!
    - apiGroup: policy.open-cluster-management.io
      kind: Policy
      name: addressing-metallb-nonprod-west-1
    - apiGroup: policy.open-cluster-management.io
      kind: Policy
      name: config-metallb-nonprod-west-1
    - apiGroup: policy.open-cluster-management.io
      kind: Policy
      name: install-metallb-nonprod-west-1
    ```

    1.  Added unless a layer sets `placement.tolerations` ([Tolerations](#tolerations)).
    2.  Every Policy this release renders. An element that renders no Policy gets no PlacementBinding.

A Placement selects one cluster because each Application renders from that cluster's own values cascade at that cluster's own revision. Its Policies carry that cluster's settings, so they must reach that cluster and no other. Group clusters with values layers ([Values cascade](values.md)), never with Placement labels.

On the hub, the selector holds the hub's ManagedCluster name, usually `local-cluster`, while the release name uses `hubName`: Placement `metallb-acm-dc1` selects `name In (local-cluster)` ([The hub](architecture.md#the-hub)).

`selector` belongs to the ApplicationSet. A values file that sets it fails CI with POLICY035 (`top-level key "selector" is read by nothing`).

To see the real Placement for a cluster, check out the revision the cluster's fleet file pins, then pass the cluster's layers in fleet-file order: root `values.yaml`, each `valueFiles` entry (here `environments/nonprod.yaml` and `datacenters/dc2.yaml`), then `values/clusters/<cluster>.yaml`. Without the three `--set` lines the render fails with `nil pointer evaluating interface {}.matchExpressions`, because only the ApplicationSet supplies `selector`.

```sh
helm dependency update stack/metallb # (1)!
helm template metallb-nonprod-west-1 stack/metallb \
  -f values.yaml \
  -f values/environments/nonprod.yaml \
  -f values/datacenters/dc2.yaml \
  -f values/clusters/nonprod-west-1.yaml \
  --set selector.matchExpressions.name.key=name \
  --set selector.matchExpressions.name.operator=In \
  --set 'selector.matchExpressions.name.values={nonprod-west-1}'
```

1.  Packages `lib/policy-library` into the gitignored `charts/`. Re-run it after a change to `lib/`.

The validator's preview (`make -C tools/validator preview ELEMENT=<element>`, [Preview](../develop/validation.md#preview)) supplies `operator: Exists` instead, so its Placement selects every cluster. The preview is meant to do that.

### Tolerations

The default tolerations keep a cluster's Policies bound while ACM taints it `unreachable` (stopped reporting) or `unavailable` (reports itself unavailable), so an outage changes nothing. Without them:

1. The Placement drops the cluster and ACM removes its Policies.
2. When the cluster reconnects, it deletes them locally, then they come back.
3. Templates with `pruneObjectBehavior` delete their objects on reconnect and recreate them when the Policies return ([Removal and pruning](removal.md#what-happens-on-the-managed-cluster)). Without prune (the default) the objects stay, but the cluster drops out of the hub's compliance view while offline.

Keep them. `placement.tolerations: []` in any layer removes them, and nothing validates it.

### Cluster sets

A Placement only selects clusters from ManagedClusterSets bound to its own namespace by a ManagedClusterSetBinding. ACM's built-in set `global` contains every ManagedCluster. The [install chart](../reference/install.md) creates ManagedClusterSetBinding `global` in `pstack`, and never deletes it, so removing PolicyStack doesn't unbind Placements still in use ([Deletion safety](../reference/install.md#deletion-safety)). If it is missing, every Placement selects nothing and no Policy reaches any cluster, while Argo CD still shows every Application Synced.

Check with `oc get managedclustersetbinding global -n pstack`. Without it, each Placement's `PlacementSatisfied` condition is False with reason `NoManagedClusterSetBindings`. The hub's Application `policystack`, which syncs the install chart, recreates it within about 3 minutes.

### Overriding the Placement

A top-level `placement:` key in any values layer merges into the generated spec ([policy-library Placement](https://github.com/PolicyStack/PolicyStack/blob/main/lib/policy-library/README.md#placement)). It applies to every element's Placement for every cluster reading that layer. POLICY080 checks the rendered Placement against its CRD (unknown fields, types), but nothing checks what the keys do, so a wrong selector shows up only on the hub.

| Key | Effect |
|---|---|
| `claimSelector`, `celSelector` | ANDed into the generated predicate. Can only narrow the selection to this cluster or to nothing, so use them as a guard, such as "only on AWS clusters". |
| `predicates` | Appended as extra predicates, which ACM ORs with the generated one. Almost always a mistake (below). |
| `tolerations` | Replaces the default tolerations. |
| Any other `Placement.spec` field | Passed through. Fields such as `numberOfClusters` or `spreadPolicy` change nothing with one cluster. |

```yaml
# values/platforms/aws.yaml: only place Policies on clusters that really are AWS
placement:
  claimSelector:
    matchExpressions:
    - {key: platform.open-cluster-management.io, operator: In, values: [AWS]}
```

If the guard excludes the cluster, its Policies stop reaching it, which is a removal ([Removal and pruning](removal.md)).

!!! danger "`placement.predicates` sends one cluster's Policies to other clusters"
    ```yaml
    # values/environments/prod.yaml: wrong
    placement:
      predicates:
      - requiredClusterSelector:
          labelSelector:
            matchLabels: {environment: prod}
    ```

    Every Application that reads this layer renders a Placement with two predicates, `name In (<its cluster>)` OR `environment=prod`. Each prod cluster then also receives every other prod cluster's Policies, built from their values. Their template names collide with its own, which ACM rejects (`Template name must be unique`).

### Lifecycle

The `lifecycle-<cluster>` Application's Policies build a cluster by creating objects on the hub, so its Placement selects the ManagedCluster of the hub that the fleet file's `hub:` names, not the cluster being built ([Cluster lifecycle](lifecycle.md)). The `policystack-lifecycle` ApplicationSet injects that selector, as for elements.

## Where Policies live

Argo CD creates every Policy, Placement and PlacementBinding in `pstack` on the hub, the `policyNamespace` set in the root [`values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/values.yaml). The install chart creates the namespace and never deletes it. ACM then copies each Policy to every cluster its Placement selects ([ACM policies in brief](acm.md)):

| Copy | Cluster | Namespace | Name | Look with |
|---|---|---|---|---|
| Root Policy, from Git | Hub | `pstack` | `<policy>-<release>` | `oc get policies.policy.open-cluster-management.io -n pstack` |
| Replicated Policy | Hub | `<cluster>`, `local-cluster` for the hub | `pstack.<policy>-<release>` | `oc get policies.policy.open-cluster-management.io -n <cluster>` |
| Policy and its ConfigurationPolicies, OperatorPolicies | Managed cluster | `<cluster>`, `local-cluster` on the hub | Policy as above; ConfigurationPolicy `<policy>-<template>`, OperatorPolicy `install-<key>` | `oc get configurationpolicies,operatorpolicies -n <cluster>` |

Leave `policyNamespace` at `pstack`. Changing it renames every Policy, which deletes and recreates all of them ([Removal and pruning](removal.md)), and shifts the name budget below. `install/values.yaml` must hold the same value, because the install chart creates that namespace and binds the `global` cluster set into it; CI fails POLICY071 if the two differ.

## Naming limits

ACM names the replicated copy `<policyNamespace>.<policy>-<release>`. Its admission webhook rejects a root Policy whose namespace and name together exceed 62 characters (63 with the dot), so Argo CD's sync fails with `the combined length of the policy namespace and name cannot exceed 62 characters` and the Policy is never created. CI fails POLICY001 before merge. A ManagedCluster can't be renamed, so a cluster name that is too long means re-importing it under a shorter one.

```text
pstack.addressing-metallb-nonprod-west-1
│      │          │       └ cluster: you choose it
│      │          └ element: fixed by the element
│      └ policy: fixed by the element
└ policyNamespace (7 with the dot, leaves 56)
```

| Policy | Replicated name | Characters |
|---|---|---|
| `install` | `pstack.install-metallb-nonprod-west-1` | 37 |
| `config` | `pstack.config-metallb-nonprod-west-1` | 36 |
| `addressing` | `pstack.addressing-metallb-nonprod-west-1` | 40 |

!!! warning "Keep cluster names and `hubName` to 30 characters"
    You only control the last part. `hubName` counts because it takes the cluster's place in the hub's release names. The longest policy and element pair shipped today, `pstack.ingress-cert-cert-manager-`, is 33 characters, which leaves 30. Element authors keep to that: an element name is at most 17 characters, policy plus element at most 24 ([Naming](../develop/elements.md#naming)).

CI (POLICY001) checks every element against a 30-character cluster name, and your fleet files against the elements each one enables ([Fixtures](../develop/validation.md#fixtures)). No check rejects a long cluster name by itself, so a 35-character cluster passes until a PR enables a longer-named element.

### Template and lifecycle names

ConfigurationPolicy and OperatorPolicy names carry no cluster name, so cluster length doesn't affect them. They must still be unique across every Policy on one cluster: Policy names such as `config`, `install` and `ready` are shared by many elements, so authors pick distinctive template names (`metallb-instance`, not `instance`), and CI fails a clash between two elements (POLICY003, [Rendered names](elements.md#rendered-names)). They are also capped at 63 characters (POLICY001).

The exception is `lifecycle/`: it places one release per built cluster on the hub, so it appends the cluster (`suffixTemplateNames`): `present-credentials-<cluster>`. Its longest names, `pstack.present-lifecycle-<cluster>` and `present-credentials-<cluster>`, fit a 38-character cluster, but a built cluster also gets element Applications once imported ([Cluster lifecycle](lifecycle.md)), so keep it to 30.
