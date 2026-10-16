# Rolling out changes

A cluster runs what Git holds at the revision its [fleet file](applicationset.md#fleet-files) pins: the element charts, their policy-library pins and every values file. To roll out a change, commit it and point fleet files at a revision that contains it.

## Onboarding a cluster

A cluster either already exists and is imported, or a hub builds it from its fleet file. To have a hub build it, add `hub`, `version` and `install` to the fleet file in step 2 and skip step 1: the hub creates the cluster, ACM imports it, and the steps below then apply as written ([Cluster lifecycle](lifecycle.md)).

1. Import the cluster into ACM. The GitOpsCluster then imports it into Argo CD ([Install PolicyStack](install.md#install-policystack)).
2. Open a pull request to the default branch that adds `fleet/<cluster>.yaml`, named exactly like the ManagedCluster ([Fleet files](applicationset.md#fleet-files)). The hub's file is `fleet/hubs/<hubName>.yaml` ([The hub](applicationset.md#the-hub)).

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main
    valueFiles:
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/aws.yaml
    ```

3. Add `values/clusters/<cluster>.yaml`, named exactly like the ManagedCluster, at the revision the fleet file pins. When that is the default branch, add it in the same pull request. It enables the elements this cluster runs beyond what its other layers enable. A misnamed file is skipped without an error ([Missing files](values.md#missing-files)). The hub's file is `values/clusters/<hubName>.yaml` ([Order](values.md#order)).

Import and the pull request can come in either order. Land step 3 before or with step 2: Applications rendered before the cluster's values file exists leave that layer out, which can briefly enable an element the file turns off. Once the cluster is imported and its fleet file is on the default branch, the ApplicationSet creates one Application per element, named `<element>-prod-east-1`, in `openshift-gitops`, within about 6 minutes. An element no layer enables syncs with no resources.

```sh
oc get applications.argoproj.io -n openshift-gitops | grep prod-east-1
```

## Promoting and rolling back

A cluster pinned to a branch picks up every commit pushed to it. A cluster pinned to a tag or SHA changes only when its fleet file's `revision` changes. Promote with a pull request that edits `revision`:

```diff
 # fleet/prod-east-1.yaml
-revision: v1.4.0
+revision: v1.5.0
 valueFiles:
```

The PR's [Render Diff](workflows.md#render-diff) comment shows what the new revision changes in the cluster's Policies.

Roll back with `git revert` of that commit, or with a pull request that sets the previous revision. Both go through the same review as any change to `fleet/` ([Protection](applicationset.md#protection)).

Every Application for the cluster switches to the new revision. Application names do not include the revision, so Argo CD updates the same Policies in place. The [Placement](policies.md#placement) selector matches only the cluster's `name` label, so the policies stay bound to the cluster while Argo CD syncs.

An element that exists at the new revision but not the old one gets a new Application. An element missing at the new revision loses its Application and its Policies ([Removing an element](#removing-an-element)).

## Disabling an element

Set `enabled: false` under the element's key in a layer that outranks every layer enabling it ([Order](values.md#order)). `values/environments/prod.yaml` enables `dns`, to turn it off on `prod-east-1` only, add this to `values/clusters/prod-east-1.yaml`:

```yaml
stack:
  dns:
    enabled: false
```

To turn off one part of an element instead, set its [toggle](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles).

Once the change reaches the cluster's revision, the element renders nothing and automated sync prunes its objects from `policy`, since the ApplicationSet sets `allowEmpty`. ACM then removes its policies from the managed cluster ([What happens on the managed cluster](#what-happens-on-the-managed-cluster)). A failed render prunes nothing: the Application reports a `ComparisonError` with sync status `Unknown`, and its Policies stay as they were.

## Removing an element

Delete `stack/<element>`. The Applications carry the resources finalizer, so when a cluster's revision no longer has the directory, the ApplicationSet deletes the element's Application, Argo CD deletes its Policies, PolicySets, Placement and PlacementBinding from `policy`, and ACM removes the policies from the managed cluster. Clusters pinned to older revisions keep the element until their pin moves.

An Application is also deleted when:

- The cluster's fleet file is deleted ([Offboarding a cluster](#offboarding-a-cluster)).
- The cluster is detached. The GitOpsCluster deletes the cluster's Argo CD secret once its ManagedCluster is gone.
- The cluster's Argo CD secret, or its `clusterID` label, disappears for any other reason ([Pausing removals](#pausing-removals)).
- The ApplicationSet is deleted ([Teardown](install.md#teardown)).

A fleet file that does not parse, or a revision that does not exist, deletes nothing. The ApplicationSet changes no Application until it is fixed ([Revision](applicationset.md#revision)).

## Offboarding a cluster

Delete the cluster's fleet file in a pull request. The ApplicationSet deletes all of its Applications, and with them its Policies ([Removing an element](#removing-an-element)).

For a cluster whose fleet file sets `hub`, the file also decides whether the cluster survives:

- **Destroy.** Set `state: absent`, wait for the Policy `absent-lifecycle-<cluster>` to be Compliant, then delete the file ([Destroy](lifecycle.md#destroy)). Detaching deletes the Day-2 Applications and their Policies. The governance add-on is already gone, so nothing is pruned on the cluster.
- **Abandon.** Delete the file, or remove `hub`. The hub deletes the lifecycle Policies and the cluster keeps running, unmanaged ([Abandon](lifecycle.md#abandon)). Deleting the file also deletes the Day-2 Policies, and templates that set `pruneObjectBehavior` prune on the still attached cluster.

Do not reuse the cluster's name while its fleet file exists. A cluster imported under that name gets the old file's configuration.

## What happens on the managed cluster

Removing a policy from the hub removes it from the managed cluster. What it created there stays unless a ConfigurationPolicy template sets `pruneObjectBehavior`, which acts only when the template is `enforce` and the setting reached the managed cluster before the removal:

| Case | Effect |
|---|---|
| `pruneObjectBehavior: None` (default) | Every object stays. Patched fields keep their values, nothing is reverted. |
| `inform` | Nothing is deleted. |
| `DeleteIfCreated` | Deletes only the objects this ConfigurationPolicy instance created. A recreated policy, or one on a re-imported cluster, adopts the existing objects and does not delete them. |
| `DeleteAll` | Deletes every object the template manages, including ones that existed before the policy. |
| OperatorPolicy | The operator, Subscription, CSV, CRDs and CRs stay. A Subscription the policy set to manual approval, because `versions` is pinned or `upgradeApproval` is `None`, stays manual, so upgrades stop. An unpinned `Automatic` one keeps upgrading, unmanaged. |
| Gatekeeper constraints and constraint templates | Deleted. |
| Cluster detached, or governance add-on removed | Nothing is pruned. |
| Cluster disconnected | Pruning runs when it reconnects. |

- Set `pruneObjectBehavior` in its own commit, and remove the policy only after that commit reached the managed cluster.
- Renaming an element, a policy, a template or `hubName` deletes the old objects and creates new ones. With prune set, that deletes the objects on the managed cluster and creates them again.
- The validator rejects prune on a template with any dependency: `waitForOperator`, `extraDependencies`, the policy's `dependencies`, or a place after another template or policy under `orderManifests` or `orderPolicies` (POLICY033, [Validation](validation.md)). ACM deletes a template whenever its dependency is unmet, for example during an operator upgrade, and prune then deletes its objects.
- Keep the Placement's default tolerations. With `placement.tolerations: []`, a disconnected cluster drops out of the Placement and loses its policies.

## Pausing removals

Before an ACM upgrade or work on the Argo CD cluster secrets, stop the ApplicationSet from deleting Applications. The upgrade to ACM 2.17 can replace a cluster's secret with a renamed one and briefly drop the cluster's Applications, and with them its Policies.

```sh
oc patch applicationset policystack -n openshift-gitops --type merge \
  -p '{"spec":{"syncPolicy":{"applicationsSync":"create-update"}}}'
```

The ApplicationSet still creates and updates Applications, but deletes none. The OpenShift GitOps operator starts the ApplicationSet controller without `--policy`, so the per-ApplicationSet setting applies. `helm upgrade` leaves the field in place. Remove it when done:

```sh
oc patch applicationset policystack -n openshift-gitops --type json \
  -p '[{"op":"remove","path":"/spec/syncPolicy/applicationsSync"}]'
```

## Operator upgrades

Each `operatorPolicies` entry renders an OperatorPolicy named after its key ([Operator Policy Options](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#operator-policy-options), [Version pinning](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#version-pinning)). `upgradeApproval` is `None` or `Automatic`. ACM rejects `Manual` and `subscription.installPlanApproval`. `upgradeApproval` and `versions` decide upgrades:

| `upgradeApproval` | `versions` | Effect when enforced |
|---|---|---|
| `Automatic` | empty | The Subscription is set to automatic approval. OLM installs every upgrade the channel offers. |
| `Automatic` | listed | The Subscription is set to manual approval. The policy approves an InstallPlan only when the CSV it installs is in `versions` or equals `subscription.startingCSV`. |
| `None` | either | The policy approves the initial install only, filtered by `versions` and `subscription.startingCSV`. It never approves an upgrade, even to a CSV in `versions`. |

An informing policy approves nothing, it reports an installed CSV missing from a non-empty `versions` as noncompliant.

`versions` entries are exact CSV names, with no ranges or wildcards. An entry can hold several comma-separated names (ACM 2.14+) or a template (ACM 2.13+).

The hub's own ACM and GitOps follow the same rules once their elements are enabled ([Hand the operators to PolicyStack](install.md#hand-the-operators-to-policystack)).

Elements ship unpinned, with `upgradeApproval: Automatic` and `versions: []`. Pin in a layer. `operatorPolicies` is a map keyed by entry name, so the layer sets only the fields it changes ([Merge rules](values.md#merge-rules)). To pin metallb on every prod cluster:

```yaml
# values/environments/prod.yaml
stack:
  metallb:
    operatorPolicies:
      metallb:
        versions:
          - <installed CSV>
        subscription:
          startingCSV: <installed CSV>
```

`oc get csv -n metallb-system` on the managed cluster shows the installed CSV. On a cluster that does not have the operator yet, OLM resolves the Subscription to the channel's latest CSV, and the policy does not approve it unless it is listed. `startingCSV` installs the pinned CSV there instead. Leave it out when the pin is the channel head.

A layer can only override keys the element defines. The validator rejects a new key as POLICY032 ([Validation](validation.md)), and the render fails if the key has no `policyRef`.

When a newer CSV is available, the OperatorPolicy status names it in a message that starts `an InstallPlan to update to [<csv>]`. OLM picks that CSV from the channel's upgrade graph, the policy only approves or holds it, so a CSV in `versions` installs only once OLM offers it. Bump in two steps:

1. Add the new CSV to `versions` next to the old one. Promote the revision that has it one cluster at a time, by editing each cluster's fleet file ([Promoting and rolling back](#promoting-and-rolling-back)).
2. Once every cluster runs the new CSV, remove the old one, move `startingCSV` to the new one, and promote again.

Dropping the old CSV in the same change makes each cluster noncompliant until its upgrade finishes.

To unpin, set `versions: []` in a layer that outranks the pin, or delete the pin. To turn an operator off on some clusters, set `enabled: false` on its key or `toggles.<key>: false` ([Disabling an element](#disabling-an-element)). Never set the key to `null`. Disable the entries that name it in `waitForOperator` in the same layer, or the render fails. This stops managing the operator but does not uninstall it.