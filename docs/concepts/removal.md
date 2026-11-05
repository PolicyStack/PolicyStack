# Removal and pruning

Argo CD deletes hub objects; ACM then removes the policies from managed clusters. What a policy created stays unless its template prunes. The procedures are in [Rolling out changes](../guides/rollout.md).

## What deletes an Application

Deleting an Application deletes its Policies, PolicySets, Placement and PlacementBinding (resources finalizer). Applications are deleted when:

| Trigger | See |
|---|---|
| The cluster's revision no longer has `stack/<element>`. Clusters pinned to older revisions keep it until their pin moves. | [Removing an element](../guides/rollout.md#removing-an-element) |
| Fleet file deleted | [Offboarding a cluster](../guides/rollout.md#offboarding-a-cluster) |
| Cluster detached (GitOpsCluster deletes its Argo CD secret) | |
| Argo CD secret or its `clusterID` label disappears | [Pausing removals](../guides/rollout.md#pausing-removals) |
| ApplicationSet deleted | [Teardown](../start/install.md#teardown) |

A fleet file that doesn't parse, or a missing revision, deletes nothing ([Revision](../reference/fleet.md#revision)).

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

- POLICY033 rejects prune on a template with any dependency (`waitForOperator`, `extraDependencies`, the policy's `dependencies`, or any template but the first in a Policy that sets `orderManifests`). ACM deletes a template while a dependency is unmet, such as during an operator upgrade, and prune would delete its objects ([Validator rules](../reference/rules.md)).
- Keep the Placement's default tolerations. With `placement.tolerations: []`, a disconnected cluster drops out and loses its policies.
