# Rolling out changes

A cluster runs what Git holds at the `revision` its [fleet file](../reference/fleet.md) pins. To ship a change, commit it and point fleet files at a revision that has it. Adding a cluster is in [First cluster](../start/first-cluster.md), operator versions in [Operator versions](operators.md).

## Reviewing a change

A PR that changes any fleet cluster's rendered Policies gets a [Render Diff](../reference/ci.md#render-diff) comment: per cluster, what the merge adds, changes or removes at its `revision`. On a same-repo PR to the default branch, no comment means no fleet cluster's render changes. Fork PRs get the result in the run's job summary. Changes under `install/` are not rendered ([Upgrading PolicyStack](upgrading.md#install-chart)).

## Promoting and rolling back

A branch `revision` picks up every push. A tag or SHA changes only when `revision` does. Promote with a PR; its Render Diff comment shows the Policy changes.

```diff
 # fleet/prod-east-1.yaml
-revision: v1.4.0
+revision: v1.5.0
```

Roll back with `git revert`, or a PR that sets the old revision. Both need the usual `fleet/` review ([Protection](../reference/fleet.md#protection)).

- Application names don't include the revision, so Argo CD updates the same Policies in place.
- The [Placement](../concepts/policies.md#placement) matches only the cluster's `name` label, so policies stay bound during the sync.
- An element added at the new revision gets an Application. One missing loses it and its Policies ([Removing an element](#removing-an-element)).

## Dry run

Set `global.remediationAction: inform` in any layer ([Global keys](../concepts/values.md#global-keys)). Every Policy for clusters reading that layer, lifecycle included, reports instead of enforcing. To preview a revision on one cluster, add this to its file at the revision it reads and remove it once the Policies look right:

```yaml
# values/clusters/prod-east-1.yaml
global:
  remediationAction: inform # (1)!
```

1.  `enforce` is rejected. Remove the key to go back to each template's own setting. `InformOnly` gates stay inform either way.

On a branch pin, add the key in the PR with the change. A tag or SHA pin reads the cluster file at the pin, so a key added at HEAD never reaches the cluster. To promote `v1.4.0` to `v1.5.0`:

1.  Commit the key on a branch from `v1.5.0` and push it.
2.  Pin that commit's SHA and check the Policies.
3.  Pin `v1.5.0`. It lacks the key, so enforcement resumes.

!!! note
    Inform undoes nothing already applied. A Policy waiting on another stays Pending, so on a new cluster only the start of each dependency chain reports.

!!! warning "Dry run stops enforcement, not removal"
    Argo CD still prunes and self-heals, and the resources finalizer still deletes the Policies of an element the revision drops ([Removing an element](#removing-an-element)). A dropped or renamed Policy whose template sets `pruneObjectBehavior` is deleted with its last `enforce` spec, so its objects go too ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)). Check the "removed" counts in the PR's Render Diff first.

## Disabling an element

Set `enabled: false` in a layer that outranks every layer enabling it ([Order](../concepts/values.md#order)). `environments/prod.yaml` enables `dns`; to turn it off on one cluster:

```yaml
# values/clusters/prod-east-1.yaml
stack:
  dns:
    enabled: false
  remediations:
    policies:
      samples:
        enabled: false # (1)!
```

1.  `enabled: false` on a policy, template or operator turns off one part of an element ([Merge rules](../concepts/values.md#merge-rules)).

The element renders nothing, sync prunes it from the hub (`allowEmpty` is set), and ACM removes its policies from the cluster ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)). A failed render prunes nothing: the Application shows `ComparisonError` / `Unknown` and the Policies stay.

## Removing an element

Delete `stack/<element>` and promote. When a cluster's revision no longer has it, the ApplicationSet deletes the Application and its Policies. Clusters pinned to older revisions keep it until their pin moves. Other triggers that delete Applications: [What deletes an Application](../concepts/removal.md#what-deletes-an-application).

## Offboarding a cluster

Delete the fleet file in a PR. Its Applications and Policies go with it. If the file sets `hub`, choose what happens to the cluster:

=== "Destroy"

    Set `state: absent`, wait for Policy `absent-lifecycle-<cluster>` to be Compliant, then delete the file ([Destroy](lifecycle.md#destroy)).

=== "Abandon"

    Delete the file, or remove `hub`. The cluster keeps running, unmanaged ([Abandon](lifecycle.md#abandon)). Deleting the file also deletes the Day-2 Policies, and templates with `pruneObjectBehavior` prune on the still-attached cluster.

!!! warning
    Don't reuse a cluster name while its fleet file exists. The new cluster gets the old configuration.

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
