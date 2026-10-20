# Upgrades

Set `version` in the cluster's fleet file and merge. On a minor bump, change `channel` too.

```diff
 # fleet/prod-east-1.yaml
 revision: v3.0.0
-version: 4.20.8
+version: 4.20.12
 channel: stable-4.20
```

| Key | Effect |
|---|---|
| `version` | OpenShift release `x.y.z` the cluster runs. Unset, nothing upgrades it. Required on a cluster a hub builds. |
| `channel` | Update channel. Unset, the cluster's channel is left alone. |

Both keys live only in the fleet file (`fleet/<cluster>.yaml`, or `fleet/hubs/<hubName>.yaml` for a hub). The validator rejects them in a values layer (POLICY034), so the clusters a merge upgrades are exactly the fleet files it changes, and the [Render Diff](workflows.md#render-diff) lists each one.

!!! note
    `version` must be a full `x.y.z`. Unquoted, `version: 4.20` is the number `4.2`; the validator rejects any value YAML reads as a number (POLICY050).

A wave is one commit that bumps several fleet files:

```diff
 # fleet/nonprod-west-1.yaml
-version: 4.20.8
+version: 4.21.2
-channel: stable-4.20
+channel: stable-4.21
 # fleet/prod-east-1.yaml
-version: 4.20.8
+version: 4.21.2
-channel: stable-4.20
+channel: stable-4.21
```

## Who upgrades what

Each field has one writer.

=== "Built by a hub with the hypershift engine"

    `lifecycle-<cluster>` on the hub writes HostedCluster `spec.release` and `spec.channel`, then each NodePool's `release` once the control plane completes. See [Cluster lifecycle](lifecycle.md#upgrades).

    The `upgrade` element is inert on the guest (Infrastructure `controlPlaneTopology: External`), since HyperShift owns its ClusterVersion.

=== "Any other cluster, hubs included"

    The `upgrade` element on the cluster writes ClusterVersion `spec.channel`, `spec.desiredUpdate` and `spec.upstream`.

    An imported hosted cluster is the exception: the element is inert on any HyperShift guest, so PolicyStack does not upgrade it.

## upgrade element

Enabled by default. Renders nothing until the fleet file sets `version` or `channel`, or a layer sets `config.upstream`.

| Template | Policy (renders when) | Mode | Does |
|---|---|---|---|
| `channel-pin` | `channel-upgrade-<cluster>` (`channel` or `config.upstream` set) | enforce | Sets ClusterVersion `spec.channel` and `spec.upstream`, each only when set. Not held by the guard. |
| `upgrade-guard` | `upgrade-upgrade-<cluster>` (`version` set) | inform | NonCompliant unless `version` is in `availableUpdates`, is already the desired version, or is at or below the current version. The message lists the ClusterVersion conditions holding it. |
| `upgrade-trigger` | same | enforce | After the guard is Compliant, sets `desiredUpdate.version`, only while `version` is newer than current and no update is progressing. Clears `image` and `force` left by `oc`/console; leaves `architecture`. |
| `upgrade-progress` | same | inform | NonCompliant until the cluster has completed `version`. The message lists the ClusterVersion conditions. |

- A version the update service does not offer is held at the guard.
- Downgrades are never attempted.

!!! note "Brownfield clusters"
    The CVO ignores `desiredUpdate.version` while `image` is set, and `oc adm upgrade` and the console leave the running image there. The trigger therefore clears `image` and `force`: a pending `oc`/console request that has not started is replaced while the fleet `version` is newer, and a hand-set `force` does not survive.

`channel-pin` sets `spec.upstream` from `stack.upgrade.config.upstream`. Empty (the default) leaves the cluster's value alone; a cluster with none uses the public graph. On a local OpenShift Update Service, set it in a values layer:

```yaml
# values/datacenters/dc1.yaml
stack:
  upgrade:
    config:
      upstream: https://osus.example.internal/api/upgrades_info/v1/graph
```

!!! warning
    Removing the key leaves the last URL on the cluster. To go back to the public graph, set `upstream: https://api.openshift.com/api/upgrades_info/v1/graph`. `toggles.channel` gates `upstream` too.

Watch it:

```sh
# on the hub: each template's latest message
oc get policy -n prod-east-1 pstack.upgrade-upgrade-prod-east-1 \
  -o jsonpath='{range .status.details[*]}{.templateMeta.name}{": "}{.history[0].message}{"\n"}{end}'
oc get clusterversion version -o jsonpath='{.status.history[0]}'    # on the cluster
```

??? info "Migrating from `config.targetVersion` / `config.channel`"
    Both keys are gone; the render fails while any layer sets them. In the commit that bumps `revision`, delete them from the values layer and set `version` and `channel` in the fleet file:

    ```diff
     # values/clusters/prod-east-1.yaml
     stack:
       upgrade:
    -    config:
    -      targetVersion: 4.20.12
    -      channel: stable-4.20
    ```

    ```diff
     # fleet/prod-east-1.yaml
    +version: 4.20.12
    +channel: stable-4.20
    ```

## Pausing

Turn a toggle off in any layer the cluster reads:

```yaml
# values/clusters/prod-east-1.yaml
stack:
  upgrade:
    toggles:
      upgrade: false # (1)!
      channel: false # (2)!
```

1.  Deletes `upgrade-upgrade-<cluster>`. An update already started on the cluster keeps running.
2.  Stops managing the channel and upstream.

A hosted cluster is paused by leaving `version` where it is.

??? question "Why not ClusterCurator or TALM?"
    ClusterCurator runs an upgrade as a one-shot Job: a failure is not retried, and a Policy owning the ClusterCurator cannot tell a finished upgrade from a new one. TALM's ClusterGroupUpgrade is also one-shot and needs its own inform policies. ClusterVersion is declarative, so a Policy can keep enforcing it.
