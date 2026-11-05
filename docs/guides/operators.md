# Operator versions

How to pin, upgrade, stop managing and uninstall the OLM operators elements install. The examples use the `metallb` element.

## How an element installs an operator

| Term | Meaning |
|---|---|
| Subscription | Asks OLM for an operator package from a catalog channel. |
| CSV | ClusterServiceVersion: one installed version of the operator, such as `metallb-operator.v4.20.0-<build>`. |
| InstallPlan | OLM's proposal to install or upgrade to a CSV; it runs only once approved. |
| OperatorGroup | Tells OLM which namespaces the operator watches. |

An element declares each operator under `stack.<element>.operatorPolicies.<key>` in its `values.yaml`. The element's README lists them under **Operators**, with package, channel and namespace ([`stack/metallb/README.md`](https://github.com/PolicyStack/PolicyStack/blob/main/stack/metallb/README.md#operators)). Each entry renders Policy `install`'s three templates, `install-<key>-ns`, OperatorPolicy `install-<key>` and the `install-<key>-status` check ([The install Policy](../concepts/elements.md#the-install-policy)). This page is about the OperatorPolicy.

The OperatorPolicy owns the Subscription's approval mode:

- Enforced, with `upgradeApproval: Automatic` (the default) and an empty `versions` list, it sets the Subscription to `installPlanApproval: Automatic`. OLM then installs every new CSV the channel offers as soon as it appears, on every cluster, whatever the cluster's fleet `revision`.
- Enforced in every other case, it sets `Manual` and approves InstallPlans itself, only for CSVs it allows. Under inform it leaves the Subscription's approval mode as it is.

Elements ship with `versions: []`, so by default every operator follows its channel.

## Choosing a behavior

Set these on `stack.<element>.operatorPolicies.<key>` in a values layer.

| Goal | Set | Result when enforced |
|---|---|---|
| Follow the channel | Nothing (default) | OLM installs every upgrade the channel offers. |
| Pin | `versions` and `subscription.startingCSV` ([Pinning](#pinning)) | Approves an InstallPlan only for a CSV in `versions` or `startingCSV`. |
| Unpin | `versions: []` and `subscription.startingCSV: ""` in a later layer, or delete the pin | Back to following the channel. `startingCSV` is a separate key, so `versions: []` alone leaves it set and a new cluster installs that old CSV first. |
| Install once | `upgradeApproval: None` | Approves the first install only, never an upgrade. |
| Change channel | `subscription.channel` ([Changing channel](#changing-channel)) | The Subscription moves to the new channel. |
| Stop managing | `enabled: false` ([Stopping and uninstalling](#stopping-and-uninstalling)) | The operator stays installed, unmanaged. |
| Uninstall | `complianceType: mustnothave` ([Stopping and uninstalling](#stopping-and-uninstalling)) | Deletes the Subscription, the CSV and an unused OperatorGroup. |

- The OperatorPolicy owns approval. `subscription.installPlanApproval` fails the render with a schema error. `upgradeApproval` accepts only `Automatic` or `None`: `Manual` renders, CI fails it (POLICY030), and on a cluster the Policy reports the OperatorPolicy's validation error. A manual policy is a `versions` pin or `upgradeApproval: None`.
- Under `inform`, including a [dry run](rollout.md#dry-run), nothing is approved or created. A new cluster gets no operator and the Policy reports NonCompliant. A non-empty `versions` also flags an installed CSV missing from it. An installed operator whose Subscription is `Automatic` keeps upgrading: OLM approves those InstallPlans itself.
- `versions` holds exact CSV names; wildcards are not supported.
- The hub's ACM and GitOps operators follow the same rules once their elements are on ([Hand the operators to PolicyStack](../start/install.md#hand-the-operators-to-policystack)).

## Pinning

`operatorPolicies` is a map, so a layer sets only the fields it changes ([Merge rules](../concepts/values.md#merge-rules)):

```yaml
# values/environments/prod.yaml
stack:
  metallb:
    operatorPolicies:
      metallb: # (1)!
        versions:
          - metallb-operator.v4.20.0-<build> # (2)!
        subscription:
          startingCSV: metallb-operator.v4.20.0-<build> # (3)!
```

1.  The key the element defines in its `values.yaml`. A layer cannot add an operator: a key the element doesn't define, such as the typo `metalb`, fails the render with `operatorPolicies.metalb has no subscription.name - a values layer may set an operator the element does not define`, and CI fails it as POLICY032 ([Validator rules](../reference/rules.md)). Add operators in the element.
2.  The CSV the clusters run: `oc get csv -n metallb-system` on a managed cluster.
3.  A new Subscription starts at the channel head, the newest CSV in the channel. If that isn't in `versions`, its InstallPlan is never approved, the operator never installs and the Policy stays NonCompliant. `startingCSV` makes OLM install the pinned CSV instead. Omit it only when the pin is the channel head.

!!! warning "A pin in a shared layer applies to every cluster reading it"
    A cluster running a CSV not in `versions` goes NonCompliant (`ClusterServiceVersion (<csv>) is not an approved version`) and its next upgrade is held. List every CSV the clusters in that layer run, or pin in each cluster file.

## Bumping

A pinned operator with an upgrade available stays **Compliant**, so nothing flags it. The held upgrade appears only in the compliance message, as `an InstallPlan to update to [<csv>] is available for approval`.

To bump a pinned operator:

1.  Find the held CSV in the message:

    ```sh
    # on the hub: the replicated install Policy of the element
    oc describe policies.policy.open-cluster-management.io -n prod-east-1 pstack.install-metallb-prod-east-1

    # on the managed cluster: the OperatorPolicy, in the namespace named after the cluster
    oc get operatorpolicy install-metallb -n prod-east-1 \
      -o jsonpath='{.status.conditions[?(@.type=="InstallPlanCompliant")].message}'
    ```

2.  Add that CSV, not a later one, next to the old one, and leave `startingCSV` alone. Promote it ([Promoting and rolling back](rollout.md#promoting-and-rolling-back)): the change is one commit, so clusters pinned to that branch get it at merge, and tag-pinned clusters get it one fleet PR at a time.
3.  Once every cluster reading the layer runs the new CSV, drop the old one, set `startingCSV` to the new one, and promote again.

Dropping the old CSV from both `versions` and `startingCSV` in one change makes every cluster still on it NonCompliant until it upgrades, and forever if the new CSV isn't its next step. Bumps don't roll back: OLM cannot downgrade, so removing the new CSV after clusters upgraded leaves them NonCompliant on the same message.

??? info "Why only the next CSV can be approved"
    OLM picks that CSV from the channel's upgrade graph: each CSV names the ones it replaces, so OLM offers only the next step from the running CSV, not the channel head. The OperatorPolicy only approves or holds it. Only the CSV in the message can be approved: listing a later one holds the upgrade forever, and reaching a target can take several bumps. One InstallPlan can carry the CSVs of several operators in the same namespace. On a `versions` pin the message ends with `but approval for [<csv>] is required`, listing every CSV in it that no OperatorPolicy in that namespace allows yet. Once all are allowed, the InstallPlan is approved.

    ```sh
    # each channel of a package and its newest CSV
    oc get packagemanifest <package> -n openshift-marketplace \
      -o jsonpath='{range .status.channels[*]}{.name}: {.currentCSV}{"\n"}{end}'
    ```

## Changing channel

Set `subscription.channel` in a layer. The OperatorPolicy updates the Subscription, and OLM offers the new channel's upgrade. An unpinned operator upgrades as soon as the new channel is set. With a pin, add the CSV it offers as in [Bumping](#bumping). Move one minor at a time: OLM's upgrade graph only links a CSV to the versions it replaces, so skipping a channel can leave no path from the running CSV, and the upgrade never comes.

```yaml
stack:
  lvm:
    operatorPolicies:
      lvms:
        subscription:
          channel: stable-4.21 # was stable-4.20
```

## Stopping and uninstalling

`enabled: false` stops managing the operator. It stays installed, and its Subscription keeps the approval mode the OperatorPolicy last set: a pinned or `None` operator stays on `Manual` and never upgrades again unless someone approves InstallPlans by hand; an unpinned one stays `Automatic` and keeps upgrading with nothing watching it. To manage it again, write `enabled: true` in a later layer or remove the `false` ([What renders](../concepts/elements.md#what-renders)).

`complianceType: mustnothave` uninstalls it through the OperatorPolicy's `removalBehavior`. The Namespace template is not rendered, so other operators in a shared namespace are untouched. `install-<key>-status` reads NonCompliant until the CSV is gone.

| `removalBehavior` key | Default | Other value |
|---|---|---|
| `operatorGroups` | `DeleteIfUnused` | `Keep` |
| `subscriptions` | `Delete` | `Keep` |
| `clusterServiceVersions` | `Delete` | `Keep` |
| `customResourceDefinitions` | `Keep` | `Delete`, which also deletes every resource of those CRDs cluster-wide |

!!! warning "Operands stay"
    Uninstalling deletes the operator but leaves what it manages. The MetalLB instance, IPAddressPools and other custom resources stay, with no controller and, with CRDs kept, nothing to clean them up. Templates that wait on the operator cannot prune them either: CI rejects `pruneObjectBehavior` on any template with a dependency (POLICY033).

To uninstall cleanly, remove the operands while the operator still runs to process their finalizers:

1.  PR: turn off the templates that create operands, so nothing recreates them. Their objects stay on the cluster.

    ```yaml
    # values/clusters/prod-east-1.yaml
    stack:
      metallb:
        policies:
          config:
            templates:
              metallb-instance:
                enabled: false # (1)!
          addressing:
            enabled: false # (2)!
    ```

    1.  The only template of Policy `config`, which waits on `metallb`. With it off, `config-metallb-<cluster>` is no longer rendered. `policies.config.enabled: false` does the same.
    2.  `addressing` depends on Policy `config`. The render doesn't check Policy dependencies, so if it were on it would still render and, on the cluster, sit Pending on a Policy that no longer exists. Turn off every Policy that depends on one you remove. `addressing` ships off; this line only matters where a layer turned it on.

2.  On the cluster, delete the operands: `oc delete metallb,ipaddresspools,l2advertisements,bgpadvertisements,bgppeers -n metallb-system --all`.
3.  PR: in the same layer as step 1, set the operator to `mustnothave`:

    ```yaml
    # values/clusters/prod-east-1.yaml
    stack:
      metallb:
        # policies: as in step 1
        operatorPolicies:
          metallb:
            complianceType: mustnothave
    ```

Keep both changes in one layer. The render checks each cluster's merged values and fails, for `enabled: false` as for `mustnothave`, while an enabled template still waits on the operator:

```text
waitForOperator "metallb" on policy "config" matches no enabled entry in operatorPolicies
waitForOperator "metallb" on policy "config" names an operator with complianceType mustnothave, which is being uninstalled
```
