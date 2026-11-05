# Rolling out changes

How a change gets from a commit to your clusters, how to preview it, and how to take things away. Adding a cluster is in [First cluster](../start/first-cluster.md), operator versions in [Operator versions](operators.md).

Rollout follows from two facts ([Fleet files](../concepts/overview.md#fleet-files) has the full model):

1. Argo CD reads `fleet/<cluster>.yaml` only from the default branch, so any edit to it, a pin move included, takes effect when it merges there.
2. Everything else (`stack/`, `values.yaml`, `values/`) is read at the commit the file's `revision` names, so each cluster can run a different commit.

## Choosing a pin

| `revision` | A change reaches the cluster | Use for |
|---|---|---|
| Branch, such as `main` | Within minutes of every merge to that branch, with no further step | Lab and nonprod clusters you want current |
| Tag, such as `v1.5.0` | Only when a fleet PR moves `revision` to a tag that has it | Production: each move is reviewed and gets its own [Render Diff](#reviewing-a-change) |
| Commit SHA | Same as a tag | One-off pins, such as a [dry run](#dry-run) |

A common setup: nonprod pins `main`, prod pins tags, and a fleet PR promotes prod one cluster or one wave at a time.

!!! note "What a pin does not freeze"
    Operators left on `versions: []` upgrade whenever their channel ships a new CSV ([Operator versions](operators.md)). The fleet keys `version` and `channel` apply when the fleet file merges, whatever the pin ([OpenShift upgrades](upgrades.md)).

!!! warning "A branch pin bypasses `fleet/` review"
    Protecting `fleet/` ([Set up your fork](../start/install.md#set-up-your-fork)) only gates pin moves. A cluster with `revision: main` takes every merge to `stack/`, `values/` and `values.yaml` without a `fleet/` reviewer. That includes `install` keys that resize hub-built clusters, or that the engine refuses, which then blocks that cluster's later changes ([Immutable fields](../concepts/lifecycle.md#immutable-fields)). Pin production to tags, or also require review on `values/` and `stack/`.

Tags are yours to create: Argo CD reads only your fork, and upstream's tags are deliberately not fetched into it ([Merge upstream](upgrading.md#merge-upstream)). Tag the commit you want to release:

```sh
git tag v1.5.0 <sha>
git push origin v1.5.0
```

Argo CD follows a tag wherever it points. Anyone with push rights can move or delete it, which changes every cluster pinned to it, or stops generation if it is gone ([When a revision breaks](../reference/fleet.md#when-a-revision-breaks)). Protect `v*` with a GitHub tag ruleset that blocks updates and deletion, or pin SHAs.

## Reviewing a change

On a PR to the default branch, the [Render Diff](../reference/ci.md#render-diff) job renders every fleet cluster at its `revision` before and after the merge and comments what each cluster's Policies gain, lose or change. Read it before you merge: it is the only place a PR shows its effect on real clusters.

It shows only clusters whose rendered output changes. A change to `stack/` or `values/` doesn't appear for clusters pinned to a tag; it appears later, in the PR that moves their pin.

No comment means no fleet cluster changes, but only if the **Render diff** check ran and passed. It is also absent when:

- the render job failed: the comment job depends on it, so check the run;
- the PR touches none of `stack/`, `lifecycle/`, `values/`, `values.yaml`, `fleet/` (`install/` is never rendered, see [Upgrading PolicyStack](upgrading.md#install-chart));
- the PR targets a branch other than the default branch;
- your fork has no fleet files yet;
- the PR comes from another repository, such as a contributor's fork of your fork: its result goes to the run's job summary instead.

## Promoting and rolling back

Promote a tag- or SHA-pinned cluster with a fleet PR. Its Render Diff shows the Policy changes for that cluster:

```diff
 # fleet/prod-east-1.yaml
-revision: v1.4.0
+revision: v1.5.0
```

Moving a pin updates the cluster in place. Application names are `<element>-<cluster>` and carry no revision, so Argo CD patches the existing Policies rather than deleting and recreating them: nothing is removed from the cluster and compliance history continues. The Placement selects the cluster by its `name` label, which doesn't change with the revision, so the Policies stay bound throughout. An element that exists only at the new revision gets a new Application. One missing there loses its Application and its Policies ([Removing an element](#removing-an-element)).

Roll back the same way you rolled forward:

| Cluster pins | Roll back by | Review |
|---|---|---|
| A tag or SHA | Setting the old `revision` in the fleet file, or reverting the pin commit | `fleet/` review applies |
| A branch | `git revert` of the commit that changed `stack/` or `values/` | Whatever protects those paths. `fleet/` review does not apply. |

!!! warning "What a rollback leaves on the cluster"
    - Templates default to `complianceType: musthave`, which only adds and patches. Fields the newer revision added stay unless the template is `mustonlyhave`; objects it added stay unless the template sets `pruneObjectBehavior` ([ConfigurationPolicy](../concepts/acm.md#configurationpolicy)).
    - A Policy that exists only in the newer revision is deleted. What it created stays, unless its template sets `pruneObjectBehavior` ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)).
    - OLM cannot downgrade an operator. Putting the old CSV back in `versions` leaves the Policy NonCompliant: `ClusterServiceVersion (<csv>) is not an approved version` ([Operator versions](operators.md)).
    - Rolling back past the commit that added an element deletes that element's Application and Policies.

## Dry run

A dry run makes every Policy report what it would change instead of changing it. Set `global.remediationAction: inform` in any layer ([Global keys](../concepts/values.md#global-keys)). policy-library, the Helm library every element renders through ([How an element is built](../concepts/elements.md)), writes it into every Policy's `spec.remediationAction`, lifecycle Policies included, and ACM applies a Policy's action to all its templates, OperatorPolicies too. Nothing is created, patched, deleted or approved.

Read the result per Policy ([Compliance states](../concepts/acm.md#compliance-states)):

- **Compliant**: enforcing would change nothing.
- **NonCompliant**: enforcing would change something. The template's status message names the objects that differ:

    ```sh
    oc get policies.policy.open-cluster-management.io -n prod-east-1  # replicated Policies, on the hub
    oc describe policies.policy.open-cluster-management.io -n prod-east-1 pstack.<policy>-<element>-prod-east-1
    ```

To dry-run one cluster, put the key in its cluster file, at the revision the cluster reads:

```yaml
# values/clusters/prod-east-1.yaml
global:
  remediationAction: inform # (1)!
```

1.  `enforce` is rejected by the schema. Remove the key to return to each Policy's own action. Templates set `remediationAction: InformOnly` stay inform either way: they are checks other templates wait on, and ACM never enforces them.

How you get the key to the cluster depends on its pin:

=== "Branch pin"

    Two PRs. The first adds the key together with the change; check the Policies once they evaluate. The second removes the key, and enforcement starts.

=== "Tag pin"

    A tag-pinned cluster reads its cluster file at the tag, so a key added on the default branch never reaches it. To preview `v1.5.0` on a cluster at `v1.4.0`:

    1.  Branch from `v1.5.0`, commit the key, and push the branch.
    2.  Fleet PR: pin that commit's SHA. Check the Policies.
    3.  Fleet PR: pin `v1.5.0`. It lacks the key, so enforcement resumes.

    Keep the branch until step 3 merges. A commit no branch or tag reaches can stop being fetchable, which stops generation ([When a revision breaks](../reference/fleet.md#when-a-revision-breaks)).

!!! warning "Limits of a dry run"
    - It does not stop removal. Only Policies that still exist at the new revision get the inform update. A Policy the new revision drops or renames is deleted from the hub, and its copy on the cluster was last seen as `enforce`. If its template sets `pruneObjectBehavior`, its objects are deleted ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)). Argo CD also keeps pruning and self-healing as usual. Check the "removed" counts in the PR's Render Diff first.
    - It undoes nothing already applied.
    - On a new cluster it previews only the first Policy of each dependency chain. A Policy that depends on another ([Dependencies](../concepts/acm.md#dependencies-and-ignorepending)) needs it Compliant; under inform nothing is applied, so the first stays NonCompliant and the rest stay Pending without evaluating. In `metallb`, `config` waits on `install` and `addressing` waits on `config`, so only `install` reports.
    - An OperatorPolicy under inform installs nothing, but an installed operator whose Subscription is `Automatic` keeps upgrading: OLM approves those InstallPlans itself ([Operator versions](operators.md)).

## Disabling an element

Set `enabled: false` in a layer listed after every layer that enables the element ([Order](../concepts/values.md#order)). `values/clusters/<cluster>.yaml` is always last, so it can turn off anything for one cluster. `environments/prod.yaml` enables `dns`; to turn it off on `prod-east-1`:

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

1.  `enabled: false` on a policy, template or operator turns off one part of an element ([How an element is built](../concepts/elements.md)).

What happens:

1.  The element renders nothing. Its Application stays and syncs to zero objects; `allowEmpty` lets it, where Argo CD would otherwise refuse to prune an Application down to nothing ([Applications](../concepts/architecture.md#applications)).
2.  Argo CD prunes the element's Policies, Placement and PlacementBinding from the hub.
3.  ACM removes the policies from the cluster. **What they created stays**, unless a template sets `pruneObjectBehavior` ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)).

If the render fails, nothing is pruned: the Application shows `ComparisonError` and sync status `Unknown`, and the last Policies stay in place until the error is fixed ([When an Application fails](../concepts/architecture.md#when-an-application-fails)).

## Removing an element

To drop an element from the repository:

1.  Delete `stack/<element>`.
2.  In the same PR, delete every `stack.<elementKey>` block (the element name in camelCase, `cert-manager` → `certManager`) under `values/` and in `values.yaml`. CI checks every values file, listed in a fleet file or not, and fails a key that names no element (POLICY035).
3.  Promote as usual. When a cluster's revision no longer has the element, the ApplicationSet deletes that cluster's Application and its Policies. Clusters pinned to older revisions keep it until their pin moves.

Disabling and deleting both delete the Policies and leave the objects. Disabling is per cluster and reversible with one line. Deleting removes it everywhere, cluster by cluster as pins move. Other events that delete Applications: [What deletes an Application](../concepts/removal.md#what-deletes-an-application).

## Offboarding a cluster

Delete `fleet/<cluster>.yaml` in a PR, and `values/clusters/<cluster>.yaml` with it (otherwise CI warns that no fleet file reads it). The cluster's Applications and Policies are deleted, and what the Policies created stays unless a template prunes. The cluster stays imported in ACM; detach it there if you want it gone.

If the file sets `hub`, the hub built the cluster and you choose what happens to it ([Cluster lifecycle](../concepts/lifecycle.md)):

=== "Destroy"

    Set `state: absent`, wait for Policy `absent-lifecycle-<cluster>` to be Compliant, then delete the file ([Destroy](lifecycle.md#destroy)).

=== "Abandon"

    Delete the file, or remove `hub`, `state` and `install` together (POLICY050 rejects `state` or `install` without `hub`). Either way `lifecycle-<cluster>` and its Policies are deleted, the cluster keeps running and stays in ACM, and lifecycle never changes or destroys it again ([Abandon](lifecycle.md#abandon)). Removing the keys keeps its element Applications. Deleting the file deletes those too, and templates with `pruneObjectBehavior` prune on the still-attached cluster.

!!! warning "Don't reuse a cluster name while its fleet file exists"
    The hub finds a cluster's fleet file by ManagedCluster name only. Any cluster imported under that name immediately gets Applications for every element the file and its layers enable.

## Pausing removals

The element Applications follow the cluster Secrets in `openshift-gitops`: if a cluster's Secret, or its `clusterID` label, disappears even briefly, the ApplicationSet deletes that cluster's Applications and their Policies. Pause deletions before an ACM upgrade or any work on those Secrets. An ACM upgrade that changes how GitOpsCluster builds cluster Secrets can replace one under a new name.

Pause both ApplicationSets. `policystack-lifecycle` pairs fleet files with the hub's own Secret, and losing its Applications abandons every cluster the hub built.

```sh
# pause deletes; creates and updates continue
for a in policystack policystack-lifecycle; do
  oc patch applicationset $a -n openshift-gitops --type merge \
    -p '{"spec":{"syncPolicy":{"applicationsSync":"create-update"}}}'
done
```

While paused, a deleted fleet file or element keeps its Applications and Policies. They are deleted when you resume.

Resume once every ManagedCluster has a Secret with a `clusterID`:

```sh
oc get managedclusters
oc get secrets -n openshift-gitops -l argocd.argoproj.io/secret-type=cluster -L clusterID
for a in policystack policystack-lifecycle; do
  oc patch applicationset $a -n openshift-gitops --type json \
    -p '[{"op":"remove","path":"/spec/syncPolicy/applicationsSync"}]'
done
```

!!! note
    `helm upgrade` of the install chart does not remove the patch; resume by hand. The field has no effect if the Argo CD ApplicationSet controller runs with an explicit `--policy`, which takes precedence.
