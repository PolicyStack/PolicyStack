# Upgrades

Every cluster upgrades the same way: change `version`, and `channel` on a minor bump, in its fleet file and merge.

```diff
 # fleet/prod-east-1.yaml
 revision: v3.0.0
-version: 4.20.8
+version: 4.20.12
 channel: stable-4.20
```

| Key | Effect |
|---|---|
| `version` | The OpenShift release, `x.y.z`, the cluster runs. Unset, nothing upgrades the cluster. Required on a cluster a hub builds. |
| `channel` | The update channel. Unset, the cluster's channel is left alone. |

Both go only in the fleet file. The validator rejects them in a values layer, so the clusters a merge upgrades are the fleet files it changes, and the [render diff](workflows.md#render-diff) shows each one. A wave is one commit that bumps several fleet files. Quote a value YAML reads as a number: unquoted, `version: 4.20` is read as `4.2`, and the validator rejects it.

## Who upgrades what

| Cluster | Upgraded by | Written |
|---|---|---|
| Built by a hub with the `hypershift` engine | `lifecycle-<cluster>` on the hub | HostedCluster `spec.release` and `spec.channel`, then each NodePool's `release` ([Cluster lifecycle](lifecycle.md#upgrades)) |
| Any other cluster, the hubs included | The `upgrade` element on the cluster | ClusterVersion `spec.channel`, `spec.desiredUpdate` and `spec.upstream` |

Each field has one writer. `upgrade` does nothing on a hosted control plane guest, one whose Infrastructure reports `controlPlaneTopology: External`, since HyperShift owns its ClusterVersion.

## upgrade

The element is enabled by default and renders nothing until the fleet file sets `version` or `channel`. It renders two Policies, `channel-upgrade-<cluster>` when `channel` is set and `upgrade-upgrade-<cluster>` when `version` is set:

| Template | Mode | Does |
|---|---|---|
| `channel-pin` | enforce | Sets the ClusterVersion channel. |
| `upgrade-guard` | inform | NonCompliant unless `version` is in `availableUpdates`, is the desired version, or is at or below the current version. |
| `upgrade-trigger` | enforce | Sets `desiredUpdate.version` once the guard is Compliant, only while `version` is newer than the current version and no update is progressing. Never `image` or `force`. |
| `upgrade-progress` | inform | NonCompliant until the cluster has completed `version`. |

A version the cluster's update service does not offer stays held at the guard. Downgrades are never attempted. `upgrade-trigger` also sets `spec.upstream` to `stack.upgrade.config.upstream`, `api.openshift.com` by default: a cluster on a local OpenShift Update Service sets it in a values layer.

`config.targetVersion` and `config.channel` are gone: the render fails while a layer sets them. Move them to the fleet file's `version` and `channel` in the commit that bumps `revision`.

Watch it:

```sh
oc get policy -n policy upgrade-upgrade-prod-east-1
oc get clusterversion version -o jsonpath='{.status.history[0]}'   # on the cluster
```

## Pausing

Set the toggle in any layer that the cluster reads, such as `values/clusters/<cluster>.yaml`:

```yaml
stack:
  upgrade:
    toggles:
      upgrade: false
```

This deletes `upgrade-upgrade-<cluster>`, and an update already started continues on the cluster. `channel: false` stops managing the channel. A hosted cluster is paused by leaving `version` where it is.

## Why not ClusterCurator or TALM

ClusterCurator runs an upgrade as a one-shot Job: a failure is not retried, and a Policy that owns the ClusterCurator cannot tell a finished upgrade from a new one. TALM's ClusterGroupUpgrade is one-shot too and needs its own inform policies. ClusterVersion is declarative, so a Policy can keep enforcing it.
