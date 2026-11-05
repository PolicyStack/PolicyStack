# Removal and pruning

"Prune" means two different things in PolicyStack:

| | Argo CD prune | ACM `pruneObjectBehavior` |
|---|---|---|
| Where it acts | Hub | Managed cluster |
| What it deletes | Hub objects that left the render: Policies, Placement, PlacementBinding | Objects a ConfigurationPolicy manages: Namespaces, Deployments, CRs |
| Set by | The ApplicationSet (`prune: true` on every Application) | You, per template. Off by default. |

Argo CD prune always runs. ACM prune is off unless you set it, and no shipped element or lifecycle engine sets it. **By default, removing a policy, a template, an element or a cluster's fleet file leaves every object it created on the managed cluster.** The procedures are in [Rolling out changes](../guides/rollout.md).

## The deletion chain

Each step runs in a different controller, and each has a place to check:

1. **Argo CD** deletes Policy `<policy>-<release>` from `pstack` on the hub, or updates it without the removed template. `oc get policies.policy.open-cluster-management.io -n pstack`
2. **ACM on the hub** deletes the replicated copy `pstack.<policy>-<release>` from the cluster's namespace on the hub, or updates it. `oc get policies.policy.open-cluster-management.io -n <cluster>`
3. **The governance add-on** on the managed cluster deletes its copy and the ConfigurationPolicies and OperatorPolicies inside it, or only the removed template. `oc get configurationpolicies,operatorpolicies -n <cluster>` on the managed cluster
4. **The config-policy controller** on the managed cluster deletes the objects those ConfigurationPolicies managed, only if [`pruneObjectBehavior`](#what-happens-on-the-managed-cluster) allows it. Otherwise the objects stay, unmanaged.

Argo CD never deletes the replicated copies itself: it doesn't track them (`copyPolicyMetadata: false`, [Deletion](architecture.md#deletion)).

## Everyday removals

Most removals change what one Application renders. The Application stays; Argo CD prunes what left the render. Renaming an element replaces the Application ([What deletes an Application](#what-deletes-an-application)).

| Change | Hub | Managed cluster |
|---|---|---|
| Element `enabled: false` | Every Policy, the Placement and the PlacementBinding are pruned. The Application stays, with zero resources (`allowEmpty`). | Every Policy of the element is removed. |
| Policy `enabled: false`, or its `converters/<policy>/` deleted together with its `policies.<policy>` entry in the element's values.yaml and every layer | That Policy is pruned. | That Policy and its templates are removed. |
| Template `enabled: false`, or its converter file deleted together with its `templates.<template>` entry | The Policy is updated without it; if it was the Policy's last enabled template, the Policy is pruned. | That ConfigurationPolicy is removed. |
| An object drops out of a template's output: an entry removed from a `config` map, an `{{- if }}` that stops emitting it, a renamed `metadata.name` | The Policy is updated. | The ConfigurationPolicy stays; that object is no longer managed. With prune set, it is deleted. |
| Renamed policy | Old Policy pruned, new one created. | Old ConfigurationPolicy removed, new one created. With prune set, the objects are deleted and recreated. |
| Renamed template | The Policy is updated: the old ConfigurationPolicy is removed and a new one created. | Old ConfigurationPolicy removed, new one created. With prune set, the objects are deleted and recreated. |

ACM prune fires when a policy change stops managing an object, not only when the policy is deleted. In metallb, the `pools` template emits one IPAddressPool per `config.ipAddressPools` entry; on a template like that with prune set, deleting an entry deletes that object on the cluster. (metallb itself can't set prune: every template waits on something, and the CI check [POLICY033](#setting-prune) rejects prune there.)

A file deleted while values still name it fails the render (`has no file`). A render that fails removes nothing: the Application shows `ComparisonError`, Argo CD neither syncs nor prunes, and the cluster keeps the last Policies it had ([Applications](architecture.md#applications)).

## What deletes an Application

Every 3 minutes the `policystack` ApplicationSet regenerates one Application for each `stack/<element>` directory, for each cluster that has a fleet file, read at that cluster's revision ([ApplicationSets](architecture.md)). It deletes an Application it no longer generates, and the Application's finalizer deletes its Policies, Placement and PlacementBinding first ([Deletion](architecture.md#deletion)). Any of these deletes an Application:

| Trigger | See |
|---|---|
| The cluster's revision no longer has `stack/<element>`. Clusters pinned to older revisions keep it until their pin moves. | [Removing an element](../guides/rollout.md#removing-an-element) |
| Fleet file deleted, or renamed, which is the same as deleting it: the file name must equal the ManagedCluster name. | [Offboarding a cluster](../guides/rollout.md#offboarding-a-cluster) |
| `stack/<element>` renamed, or `hubName` changed. The Application name changes, so the old one is deleted and a new one created. | [The hub](architecture.md#the-hub) |
| Global Hub only: a managed hub's `deploy-mode` label, which makes the global hub read its fleet file from `fleet/hubs/`, is removed. Its Day-2 Applications are deleted unless `fleet/<hub>.yaml` exists, and every `lifecycle-<cluster>` Application with `hub: <hub>` is deleted, abandoning those clusters. | [Global Hub](../guides/global-hub.md) |
| Cluster detached. ACM's GitOpsCluster deletes its Argo CD cluster secret, which the ApplicationSet lists clusters from. | [ApplicationSets](architecture.md) |
| The Argo CD cluster secret or its `clusterID` label disappears, for example during an ACM upgrade. | [Pausing removals](../guides/rollout.md#pausing-removals) |
| ApplicationSet deleted | [Teardown](../start/install.md#teardown) |

`applicationsSync: create-update` on the ApplicationSet pauses every row except deleting the ApplicationSet; creates and updates continue ([Pausing removals](../guides/rollout.md#pausing-removals)). Don't rename an element or `hubName` while paused: the old Policies stay next to the new ones and their template names collide.

A broken fleet file deletes nothing: it stops all generation on the hub until fixed ([One bad fleet file stops the hub](overview.md#one-bad-fleet-file-stops-the-hub)).

## What happens on the managed cluster

When the cluster deletes a ConfigurationPolicy (step 3 of [the chain](#the-deletion-chain)), the controller acts on the version it last received (step 4). It deletes the objects only if `pruneObjectBehavior` is not `None` **and** the Policy enforces and the template isn't `InformOnly` ([inform, enforce and InformOnly](acm.md#inform-enforce-and-informonly)).

| Case | Effect |
|---|---|
| `None` (default), or the Policy is `inform` | Everything stays. Fields that `musthave` or `mustonlyhave` changed on objects that existed before keep their last enforced values; nothing reverts them. |
| `DeleteIfCreated` | Deletes the objects this ConfigurationPolicy created, as recorded in its status. One recreated while its objects still exist (cluster re-imported, or a rename made while prune was off) records them as pre-existing and later deletes nothing. |
| `DeleteAll` | Deletes every object the template manages, including ones that existed before the policy, such as a shared namespace. A deleted Namespace takes every workload and PVC in it. |
| OperatorPolicy | `pruneObjectBehavior` doesn't apply. Operator, CSV, CRDs and CRs stay. With `versions` pinned or `upgradeApproval: None` ([OperatorPolicy](acm.md#operatorpolicy)), the OperatorPolicy left the Subscription on manual InstallPlan approval, so with the policy gone nothing approves upgrades and they stop. Unpinned, OLM keeps upgrading along the channel. To uninstall, see [Stopping and uninstalling](../guides/operators.md#stopping-and-uninstalling). |
| Cluster detached, or governance add-on removed | Nothing is deleted: the controller that would prune is gone. |
| Cluster disconnected when the Policy is removed | Prunes when it reconnects. |

Gatekeeper constraints in a converter follow the rows above; ACM's Gatekeeper-specific removal rule doesn't apply, because they're wrapped in a ConfigurationPolicy.

### Setting prune

Set it per template in a values layer; it works whatever shape the converter has ([Converter shapes](../develop/elements.md#converter-shapes)).

```yaml
stack:
  <element>:
    policies:
      <policy>:
        templates:
          <template>:
            pruneObjectBehavior: DeleteIfCreated
```

!!! danger "Prune deletes live objects"
    - **Set it in its own commit**, and remove the policy only after that commit reached the cluster. "Reached" is per cluster: in its pinned revision, synced, and replicated. Check with `oc get policies.policy.open-cluster-management.io pstack.<policy>-<release> -n <cluster> -o yaml | grep pruneObjectBehavior` on the hub. If prune and the removal land together, the cluster never sees prune and deletes nothing.
    - **Renames delete and recreate.** Renaming an element, policy, template or `hubName`, or changing `policyNamespace`, deletes the old ConfigurationPolicy and its objects, then recreates the objects from scratch. Data in a deleted PVC, namespace or CR is gone.
    - **Keep the Placement's default tolerations.** With `placement.tolerations: []`, a cluster that drops off the network loses its Policies and prunes on reconnect, then gets them back: a network blip becomes an outage ([Tolerations](policies.md#tolerations)).

POLICY033 fails CI on prune that can't work or would fire by accident ([Validator rules](../reference/rules.md)):

- **Any dependency.** ACM deletes a waiting template whenever its dependency is unmet ([Dependencies and ignorePending](acm.md#dependencies-and-ignorepending)), so prune would delete its objects every time, for example during each operator upgrade. Waiting keys: [Waiting and ordering](elements.md#waiting-and-ordering).
- **Not enforced.** ACM prunes only under `enforce`. Not flagged while a [dry run](../guides/rollout.md#dry-run) forces `inform`.
- **`DeleteAll`** is a warning, reported without failing CI: prefer `DeleteIfCreated`.

If you add prune in your fork, never add it to `stack/cim` or `lifecycle/`: deleting the AgentServiceConfig deletes the assisted-service volumes, and deleting a lifecycle Policy must never delete a cluster.

### Deleting objects without prune

Turn the template into a deletion first, then remove it:

1. Set `stack.<element>.policies.<policy>.templates.<template>.complianceType: mustnothave` under an enforced Policy:

    ```yaml
    # values/clusters/prod-east-1.yaml
    stack:
      metallb:
        policies:
          config:
            templates:
              metallb-instance:
                complianceType: mustnothave
    ```

2. Wait until `pstack.config-metallb-prod-east-1` is Compliant in namespace `prod-east-1` on the hub: the objects are gone. Every cluster must reach that state at its own pin before you go on.
3. Disable or delete the template.

!!! warning
    If the template includes the Namespace, `mustnothave` deletes it and everything in it.

This only works for converters that emit plain documents. Open the converter: if it emits `- complianceType:` entries, it is a raw list, and the values key is ignored without an error (metallb's `addressing` templates are). For those, disable the template and delete the objects with `oc delete`. For operators, use the OperatorPolicy's own `mustnothave` ([Stopping and uninstalling](../guides/operators.md#stopping-and-uninstalling)).
