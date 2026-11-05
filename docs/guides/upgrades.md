# OpenShift upgrades

You set the OpenShift version each cluster should run in its fleet file. PolicyStack moves the cluster there, never backwards, and reports progress as Policy compliance on the hub.

## How it works

### The target is in the fleet file

`version` and `channel` are keys of `fleet/<cluster>.yaml` (`fleet/hubs/<hubName>.yaml` for a hub). Like every fleet key, they are read from the default branch, not at the cluster's `revision` ([Fleet files](../concepts/overview.md#fleet-files)), and the ApplicationSet passes them to each of the cluster's Applications as Helm values. A merged bump reaches the cluster within about 6 minutes even if the cluster pins an old tag. The `revision` only decides which version of the `upgrade` element's code runs.

| Fleet key | Effect |
|---|---|
| `version` | OpenShift release `x.y.z` the cluster should run. Unset, nothing upgrades the cluster. On a cluster a hub builds it is required, because it first picks the install image; the lifecycle render fails with `version is required` without it. Bumping it never reinstalls a `hive` or `siteconfig` cluster: the `upgrade` element upgrades it like any standalone cluster. |
| `channel` | Update channel, such as `stable-4.21`. Unset, the cluster's channel is left as it is. |

!!! note "Always the full `x.y.z`"
    `version: 4.20` is read as the number `4.2` ([Quote numbers and booleans](../reference/fleet.md#quote-numbers-and-booleans)). CI rejects any `version` that isn't `x.y.z` ([POLICY050](../reference/rules.md)). If one is merged without CI, the `upgrade-guard` check stays NonCompliant and nothing upgrades.

!!! warning "Not in a values layer"
    CI rejects `version:` or `channel:` at the root of a values file, outside `stack:` ([POLICY034](../reference/rules.md)). The fleet keys outrank every values file, so a layer's `version` would be ignored where the fleet file sets one and would silently upgrade every other cluster reading the layer, with no fleet diff to review. POLICY034 is a CI check: Argo CD itself would apply the layer value.

### The cluster must offer the target

On a standalone cluster `version` must be a direct update from the running version, listed under *Recommended updates* by `oc adm upgrade` on the cluster (ClusterVersion `status.availableUpdates`). That list comes from the update graph for the cluster's channel, served by Red Hat (`api.openshift.com`) or by your own OpenShift Update Service ([Update service](#update-service)). That is why a minor bump changes `channel` too.

### Who upgrades what

| Cluster | Upgraded by | Writes |
|---|---|---|
| Standalone: imported, built by `hive` or `siteconfig` ([Pick an engine](../concepts/lifecycle.md#pick-an-engine)), and every hub | The `upgrade` element, a Day-2 element on by default, running on the cluster | ClusterVersion `spec.channel`, `spec.upstream`, `spec.desiredUpdate` |
| Hosted control plane built by a hub (`hypershift` engine) | That hub's `present-lifecycle-<cluster>` Policy, rendered by Application `lifecycle-<cluster>` ([How a Policy builds a cluster](../concepts/lifecycle.md#how-a-policy-builds-a-cluster)) | HostedCluster and NodePool release ([Hosted control planes](#hosted-control-planes)) |
| Hosted control plane PolicyStack did not build | Nobody. Upgrade its HostedCluster on the hub that hosts it | |

## Upgrade a cluster

These steps are for standalone clusters. For a `hypershift`-built cluster do step 2 only and watch as in [Hosted control planes](#hosted-control-planes).

1. On the cluster, run `oc adm upgrade` and pick a version under *Recommended updates*. For a minor upgrade the list only shows the new minor once the channel moves: merge the `channel` change alone first (it upgrades nothing), then read the list again.
2. Edit the fleet file in a PR:

    ```diff
     # fleet/prod-east-1.yaml
     revision: v3.0.0
    -version: 4.20.8
    +version: 4.20.12
     channel: stable-4.20
    ```

    On a minor bump `channel` moves too (`stable-4.20` to `stable-4.21`), in this commit or already merged in step 1. The [Render Diff](../reference/ci.md#render-diff) comment lists the cluster, because its upgrade Policy changes.

3. Merge. Within about 6 minutes the cluster's `upgrade-upgrade-<cluster>` Policy (policy `upgrade` of element `upgrade`, see [Rendered names](../concepts/elements.md#rendered-names)) turns NonCompliant and the upgrade starts. [Watch it](#watch-an-upgrade).

Several fleet files bumped in one commit upgrade at the same time: nothing orders or staggers clusters. To stage a fleet, use one PR per group and open the next once the last group's Policies are Compliant.

## The upgrade element

The element is on by default for every cluster. It renders nothing, not even a Placement, until the fleet file sets `version` or `channel` or a layer sets `stack.upgrade.config.upstream`. With `version: 4.20.12` and `channel: stable-4.20` on cluster `prod-east-1` it renders two Policies:

| Policy | Renders when | Template (ConfigurationPolicy) | Action | Does |
|---|---|---|---|---|
| `channel-upgrade-prod-east-1` | `channel` or `stack.upgrade.config.upstream` set | `channel-pin` | enforce | Sets ClusterVersion `spec.channel` and `spec.upstream`, each only when set. Not held by the guard, so a disconnected cluster reaches its update service before the guard needs it. |
| `upgrade-upgrade-prod-east-1` | `version` set | `upgrade-guard` | InformOnly | NonCompliant unless `version` is in `availableUpdates`, is already the desired version, or is at or below the running version. |
| | | `upgrade-trigger` | enforce | Waits for the guard. Sets `desiredUpdate.version` only while `version` is newer than the running version and no update is progressing. Clears `image` and `force` ([Brownfield clusters](#brownfield-clusters)). |
| | | `upgrade-progress` | InformOnly | Waits for the trigger. NonCompliant until the cluster has completed `version`. |

- In the Policy names, `<cluster>` is the ManagedCluster name, or `hubName` on a hub.
- **InformOnly** is an action ACM keeps at inform even when the Policy says enforce, so the guard and progress checks never change the cluster ([inform, enforce and InformOnly](../concepts/acm.md#inform-enforce-and-informonly)).
- **Waits for** is an ACM dependency ([Waiting and ordering](../concepts/elements.md#waiting-and-ordering)). While the guard is NonCompliant the trigger is Pending and writes nothing. `ignorePending` counts Pending as Compliant, so the Policy's status shows the guard's reason, not a wall of Pending.
- The guard and progress templates show as `mustnothave` on ClusterVersion `version` in the console. That object always exists, so the check is a deliberate way to report NonCompliant. Both are InformOnly: nothing deletes your ClusterVersion.
- The NonCompliant message ends with the ClusterVersion conditions that explain it: always `Progressing`, `Failing` or `Invalid` when True, and `ReleaseAccepted` or `RetrievedUpdates` when False.

### Watch an upgrade

```sh
# on the hub: each template's latest message
oc get policies.policy.open-cluster-management.io -n prod-east-1 pstack.upgrade-upgrade-prod-east-1 \
  -o jsonpath='{range .status.details[*]}{.templateMeta.name}{": "}{.history[0].message}{"\n"}{end}'
# on the cluster
oc adm upgrade
```

For a hub, the namespace is `local-cluster` and the name uses `hubName`: `-n local-cluster pstack.upgrade-upgrade-<hubName>`.

| Phase | `upgrade-guard` | `upgrade-trigger` | `upgrade-progress` |
|---|---|---|---|
| Held | NonCompliant, with `Progressing` and, if they explain it, `RetrievedUpdates` or `ReleaseAccepted` | Pending | Pending |
| Running | Compliant | Compliant | NonCompliant, `...; Progressing: Working towards 4.20.12...` |
| Done | Compliant | Compliant | Compliant |

### When it does not move

| You see | Cause | Fix |
|---|---|---|
| Guard NonCompliant, minor bump | `channel` still names the old minor (or is unset), whose graph has no edge to `version` | Set `channel` to the new minor. |
| Guard NonCompliant, version is in the channel | `version` needs an intermediate hop, is only a conditional update (known risks, listed by `oc adm upgrade --include-not-recommended`), or `availableUpdates` is empty | Pick a version `oc adm upgrade` recommends; hop in several bumps. Conditional updates are not supported: keep `version` at the running version, run `oc adm upgrade --allow-not-recommended --to <version>` by hand, and set `version` to it once the update completes. Empty list: check the channel and the update service ([Update service](#update-service)). |
| Progress NonCompliant, the `ReleaseAccepted` message names a failed precondition | `Upgradeable=False` blocks a minor upgrade, often an admin-ack | Fix the cause on the cluster, such as setting the acknowledgement in ConfigMap `admin-acks` in `openshift-config` as the message says. There is no force for standalone clusters. |
| All Compliant, cluster runs an older `version` than the fleet's | Lowered or reverted `version`: a target at or below the running version counts as reached | OpenShift does not downgrade. Set `version` to what the cluster runs. A bump reverted after the update started still finishes. |
| All Compliant, cluster runs a newer version than the fleet's | Someone ran `oc adm upgrade --to` past the fleet `version`. The trigger only moves clusters forward to `version`, so it lets this through and nothing flags it | Set `version` to what the cluster runs. |

### Brownfield clusters

A brownfield cluster is one upgraded by hand before PolicyStack. The Cluster Version Operator (CVO) ignores `desiredUpdate.version` while `desiredUpdate.image` is set, and `oc adm upgrade` and the console leave the running image there. The trigger therefore writes `image: ''` and `force: false` next to the version, leaving `desiredUpdate.architecture` (set when a cluster moved to the multi-arch payload) as it is. While the fleet `version` is newer than the running one, a pending `oc` or console request that has not started is replaced, and a hand-set `force` (or `--to-image`) does not survive.

## Update service

`channel-pin` sets `spec.upstream` from `stack.upgrade.config.upstream`. Empty (the default) leaves the cluster's value alone; a cluster with none uses Red Hat's public graph. A disconnected cluster has no reachable graph, so `availableUpdates` stays empty and every bump holds at the guard. Point it at a local OpenShift Update Service (OSUS) in a values layer, and see [Disconnected clusters](disconnected.md#update-service) for the CA and mirrored images it also needs:

```yaml
# values/datacenters/dc1.yaml
stack:
  upgrade:
    config:
      upstream: https://osus.example.internal/api/upgrades_info/v1/graph
```

!!! warning
    Removing the key leaves the last URL on the cluster, since the template then stops setting the field. To go back to the public graph, set `upstream: https://api.openshift.com/api/upgrades_info/v1/graph`. `policies.channel.enabled: false` stops managing `upstream` too.

## Holding and pausing

To hold one cluster back, leave its `version` alone.

To stop PolicyStack writing ClusterVersion on a cluster altogether, turn the policies off in a layer the cluster reads:

```yaml
# values/clusters/prod-east-1.yaml
stack:
  upgrade:
    policies:
      upgrade:
        enabled: false # (1)!
      channel:
        enabled: false # (2)!
```

1.  Deletes `upgrade-upgrade-<cluster>`. What the trigger already wrote stays on ClusterVersion, so an update the trigger already requested still runs.
2.  Stops managing the channel and upstream. The cluster keeps the last values.

!!! warning "Pause before you bump"
    Values layers are read at the cluster's `revision`, but `version` is read at the default branch. A tag-pinned cluster sees the pause only once its `revision` includes it: tag the pause commit and move the pin in a fleet PR merged before the bump (same steps as the Tag pin tab in [Dry run](rollout.md#dry-run)). And the trigger writes `desiredUpdate` on its first evaluation after the bump, so a pause merged after the bump is usually too late.

`global.remediationAction: inform` in a layer also stops the trigger, along with every other Policy of the clusters reading that layer ([Dry run](rollout.md#dry-run)).

## Hosted control planes

A hosted control plane's ClusterVersion is owned by HyperShift, so the `upgrade` element's Policies report Compliant there and change nothing. For a `hypershift`-built cluster the hub's `present-lifecycle-<cluster>` Policy moves HostedCluster `spec.release` when `version` changes, then each NodePool once the control plane completes. Unlike the standalone path:

- With `channel` set it waits until `version` is in the HostedCluster's `status.version.availableUpdates`. Without `channel` it moves at once, with no availability check.
- It never applies a `version` below the control plane's desired version.
- There is no pause switch: leave `version` where it is.

Watching, held downgrades and forcing a blocked minor upgrade are in [Building clusters](lifecycle.md#upgrades).

??? question "Why not ClusterCurator or TALM?"
    ClusterCurator is ACM's resource for running an upgrade, with hooks, on a managed cluster as a one-shot Job. A failure is not retried, and a Policy owning the ClusterCurator cannot tell a finished upgrade from a new one. TALM (Topology Aware Lifecycle Manager) batches policy remediation across clusters with a ClusterGroupUpgrade, also one-shot, and needs its own inform policies. ClusterVersion is declarative, so a Policy can keep enforcing it.
