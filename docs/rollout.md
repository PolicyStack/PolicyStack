# Rolling out changes

A cluster runs what Git holds at the `revision` its [fleet file](applicationset.md#fleet-files) pins. To ship a change, commit it and point fleet files at a revision that has it.

## Onboarding a cluster

1. Import the cluster into ACM. The GitOpsCluster adds it to Argo CD ([Install PolicyStack](install.md#install-policystack)).
2. Add `values/clusters/<cluster>.yaml` with the elements this cluster runs beyond its other layers.
3. PR `fleet/<cluster>.yaml` to the default branch:

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main # (1)!
    valueFiles: # (2)!
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/aws.yaml
    ```

    1.  Branch, tag or SHA.
    2.  Under `values/`, lowest precedence first. `values/clusters/prod-east-1.yaml` is added last ([Order](values.md#order)).

Within about 6 minutes there is one Application per element, named `<element>-<cluster>`. An element no layer enables still gets an Application, with no resources.

```sh
oc get applications.argoproj.io -n openshift-gitops | grep prod-east-1
```

!!! warning "Name both files exactly like the ManagedCluster"
    A misnamed values file is skipped silently ([Missing files](values.md#missing-files)). Commit it at the revision the fleet file pins (with the fleet file PR when that is the default branch), or the first render leaves it out and can briefly enable an element it turns off.

For a cluster the hub builds, skip step 1 and add `hub`, `version` and `install` to the fleet file ([Cluster lifecycle](lifecycle.md)). The hub itself uses `fleet/hubs/<hubName>.yaml` and `values/clusters/<hubName>.yaml` ([The hub](applicationset.md#the-hub)).

## Promoting and rolling back

A branch `revision` picks up every push. A tag or SHA changes only when `revision` does. Promote with a PR; its [Render Diff](workflows.md#render-diff) comment shows the Policy changes.

```diff
 # fleet/prod-east-1.yaml
-revision: v1.4.0
+revision: v1.5.0
```

Roll back with `git revert`, or a PR that sets the old revision. Both need the usual `fleet/` review ([Protection](applicationset.md#protection)).

- Application names don't include the revision, so Argo CD updates the same Policies in place.
- The [Placement](policies.md#placement) matches only the cluster's `name` label, so policies stay bound during the sync.
- An element added at the new revision gets an Application. One missing loses it and its Policies ([Removing an element](#removing-an-element)).

## Dry run

Set `global.remediationAction: inform` in any layer ([Global keys](values.md#global-keys)). Every Policy for clusters reading that layer, lifecycle included, reports instead of enforcing. To preview a revision on one cluster, add this in the same PR and remove it once the Policies look right:

```yaml
# values/clusters/prod-east-1.yaml
global:
  remediationAction: inform # (1)!
```

1.  `enforce` is rejected. Remove the key to go back to each template's own setting.

!!! note
    Inform undoes nothing already applied. A Policy waiting on another stays Pending, so on a new cluster only the start of each dependency chain reports.

## Disabling an element

Set `enabled: false` in a layer that outranks every layer enabling it ([Order](values.md#order)). `environments/prod.yaml` enables `dns`; to turn it off on one cluster:

```yaml
# values/clusters/prod-east-1.yaml
stack:
  dns:
    enabled: false
  remediations:
    toggles:
      samples: false # (1)!
```

1.  A [toggle](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles) turns off one entry of an element.

The element renders nothing, sync prunes it from the hub (`allowEmpty` is set), and ACM removes its policies from the cluster ([What happens on the managed cluster](#what-happens-on-the-managed-cluster)). A failed render prunes nothing: the Application shows `ComparisonError` / `Unknown` and the Policies stay.

## Removing an element

Delete `stack/<element>`. When a cluster's revision no longer has it, the ApplicationSet deletes the Application and the resources finalizer deletes its Policies, PolicySets, Placement and PlacementBinding. Clusters pinned to older revisions keep it until their pin moves.

Applications are also deleted when:

| Trigger | See |
|---|---|
| Fleet file deleted | [Offboarding a cluster](#offboarding-a-cluster) |
| Cluster detached (GitOpsCluster deletes its Argo CD secret) | |
| Argo CD secret or its `clusterID` label disappears | [Pausing removals](#pausing-removals) |
| ApplicationSet deleted | [Teardown](install.md#teardown) |

A fleet file that doesn't parse, or a missing revision, deletes nothing ([Revision](applicationset.md#revision)).

## Offboarding a cluster

Delete the fleet file in a PR. Its Applications and Policies go with it. If the file sets `hub`, choose what happens to the cluster:

=== "Destroy"

    Set `state: absent`, wait for Policy `absent-lifecycle-<cluster>` to be Compliant, then delete the file ([Destroy](lifecycle.md#destroy)). Nothing is pruned on the cluster: its governance add-on is already gone.

=== "Abandon"

    Delete the file, or remove `hub`. The hub deletes the lifecycle Policies and the cluster keeps running, unmanaged ([Abandon](lifecycle.md#abandon)). Deleting the file also deletes the Day-2 Policies, and templates with `pruneObjectBehavior` prune on the still-attached cluster.

!!! warning
    Don't reuse a cluster name while its fleet file exists. The new cluster gets the old configuration.

## What happens on the managed cluster

The policy goes away; what it created stays unless the template sets `pruneObjectBehavior`, is `enforce`, and that setting reached the cluster before the removal.

| Case | Effect |
|---|---|
| `None` (default) or `inform` | Everything stays, patched fields included. |
| `DeleteIfCreated` | Deletes objects this policy instance created. A recreated policy, or a re-imported cluster, adopts them and won't. |
| `DeleteAll` | Deletes every object the template manages, pre-existing ones too. |
| OperatorPolicy | Operator, CSV, CRDs and CRs stay. A pinned or `None` Subscription stays on manual approval, so upgrades stop. An unpinned one keeps upgrading. |
| Gatekeeper constraints and templates | Deleted. |
| Cluster detached, or governance add-on removed | Nothing pruned. |
| Cluster disconnected | Prunes on reconnect. |

!!! danger "Prune deletes live objects"
    Set `pruneObjectBehavior` in its own commit and remove the policy only after it reached the cluster. Renaming an element, policy, template or `hubName` deletes and recreates objects, which with prune set happens on the managed cluster too.

- POLICY033 rejects prune on a template with any dependency (`waitForOperator`, `extraDependencies`, the policy's `dependencies`, or a place after another template/policy under `orderManifests`/`orderPolicies`). ACM deletes a template while a dependency is unmet, such as during an operator upgrade, and prune would delete its objects ([Validation](validation.md)).
- Keep the Placement's default tolerations. With `placement.tolerations: []`, a disconnected cluster drops out and loses its policies.

## Pausing removals

Do this before an ACM upgrade or work on Argo CD cluster secrets. The ACM 2.17 upgrade can swap a cluster's secret for a renamed one and briefly drop its Applications and Policies.

```sh
# pause deletes; creates and updates continue
oc patch applicationset policystack -n openshift-gitops --type merge \
  -p '{"spec":{"syncPolicy":{"applicationsSync":"create-update"}}}'

# resume (helm upgrade does not remove it)
oc patch applicationset policystack -n openshift-gitops --type json \
  -p '[{"op":"remove","path":"/spec/syncPolicy/applicationsSync"}]'
```

## Operator upgrades

Each `operatorPolicies` entry renders an OperatorPolicy named after its key ([Operator Policy Options](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#operator-policy-options)).

| `upgradeApproval` | `versions` | When enforced |
|---|---|---|
| `Automatic` | empty | OLM installs every upgrade the channel offers. |
| `Automatic` | listed | Approves an InstallPlan only for a CSV in `versions` or `subscription.startingCSV`. |
| `None` | either | Approves the first install only, never an upgrade. |

- ACM rejects `Manual` and `subscription.installPlanApproval`.
- Inform approves nothing; it flags an installed CSV missing from a non-empty `versions`.
- `versions` holds exact CSV names, no wildcards. Comma-separated names need ACM 2.14+, templates 2.13+.
- The hub's ACM and GitOps follow the same rules once their elements are on ([Hand the operators to PolicyStack](install.md#hand-the-operators-to-policystack)).

Elements ship unpinned. `operatorPolicies` is a map, so a layer sets only what it changes ([Merge rules](values.md#merge-rules)):

```yaml
# values/environments/prod.yaml
stack:
  metallb:
    operatorPolicies:
      metallb: # (1)!
        versions:
          - <installed CSV> # (2)!
        subscription:
          startingCSV: <installed CSV> # (3)!
```

1.  Must be a key the element defines. A new key fails POLICY032 ([Validation](validation.md)) and the render.
2.  `oc get csv -n metallb-system` on the managed cluster.
3.  Without it, a fresh cluster resolves the channel head, which the pin blocks. Omit it when the pin is the head.

To bump, watch the OperatorPolicy status for `an InstallPlan to update to [<csv>]`. OLM picks that CSV from the upgrade graph; the policy only approves or holds it.

1. Add the new CSV next to the old one. Promote cluster by cluster ([Promoting and rolling back](#promoting-and-rolling-back)).
2. Once all clusters run it, drop the old CSV, move `startingCSV`, and promote again.

Swapping old for new in one change makes every cluster NonCompliant until it upgrades.

| Goal | Set on the key | Result |
|---|---|---|
| Unpin | `versions: []` in a higher layer, or delete the pin | Follows the channel |
| Stop managing | `enabled: false` or `toggles.<key>: false` (never `null`) | Stays installed |
| Uninstall | `complianceType: mustnothave` | Deletes Subscription, CSV, and an unused OperatorGroup. Keeps CRDs and namespace ([Removing an operator](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#removing-an-operator)) |

Stop managing and Uninstall both fail the render unless entries naming the key in `waitForOperator` are turned off in the same layer.

```yaml
# values/clusters/prod-east-1.yaml: uninstall metallb
stack:
  metallb:
    toggles:
      metallb-instance: false # (1)!
    operatorPolicies:
      metallb:
        complianceType: mustnothave # (2)!
```

1.  The instance waits on `metallb`, so it goes off in the same layer.
2.  `removalBehavior` changes what is deleted.
