# Global Hub

How PolicyStack runs when ACM hubs sit under a [Multicluster Global Hub](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.17/html/multicluster_global_hub/index).

## What Global Hub does

Global Hub is an operator on an ACM hub, the global hub. The ACM hubs it imports are managed hubs. An agent on each managed hub streams that hub's Policies, compliance, events and ManagedClusters through Kafka into PostgreSQL. Grafana dashboards read them there. Global Hub can also move clusters between managed hubs (`ManagedClusterMigration`, Technology Preview).

It does not push configuration. Global policy distribution was removed in Global Hub 1.7. Delivery stays with GitOps and the global hub's own ACM.

Use Global Hub 1.8.2 ([RHSA-2026:68515](https://access.redhat.com/errata/RHSA-2026:68515)) or 1.7.3 ([RHSA-2026:67842](https://access.redhat.com/errata/RHSA-2026:67842)) or later. Earlier releases let one managed hub spoof another hub's compliance data ([CVE-2026-71576](https://access.redhat.com/security/cve/CVE-2026-71576)). Global Hub 1.8 pairs with ACM 2.17, 1.7 with 2.16.

## Import modes

The label `global-hub.open-cluster-management.io/deploy-mode` on a managed hub's ManagedCluster sets how the global hub imports it. Without the label the hub gets no Global Hub agent.

| | `default` | `hosted` |
|---|---|---|
| Managed hub's klusterlet runs | On the managed hub | On the global hub |
| Managed hub has `local-cluster` | No. Requires `disableHubSelfManagement: true` | Yes |
| Global hub's Policies reach the managed hub | Yes | No. The webhook turns off the policy and application addons |
| Who configures the managed hub | The global hub, through `fleet/<managed-hub>.yaml` | The managed hub, through `fleet/hubs/<managed-hub>.yaml` |
| Who builds the managed hub's clusters | The global hub, on the managed hub ([Lifecycle](#lifecycle)) | The managed hub |
| `hypershift` engine on the managed hub | No. Local hosting needs `local-cluster` | Yes |
| When to set the label | Any time | Before the import. The webhook ignores clusters already imported |

Hosted mode also needs the global hub to manage itself, an import kubeconfig that never expires, and network access from the global hub to each managed hub's API. Backup and restore of the managed hub is not supported.

## Layout

Install PolicyStack on the global hub and on every managed hub, each with its own `hubName` ([Install PolicyStack](install.md#install-policystack)). All hubs read the same `fleet/`. Each hub's ApplicationSets only see the ManagedClusters that hub imports, so a fleet file's Day-2 Applications render on one hub and its lifecycle Application on one hub. For a cluster of a default-mode managed hub these differ: Day-2 renders on the managed hub, lifecycle on the global hub.

```text
fleet/
  hubs/gh.yaml      # the global hub
  hub-a.yaml        # default-mode managed hub, configured by gh
  hubs/hub-b.yaml   # hosted-mode managed hub, configured by hub-b
  edge-1.yaml       # hub: hub-a, built by gh on hub-a
  edge-2.yaml       # hub: hub-b, built by hub-b
```

Default mode only:

- Set `stack.acm.config.selfManaged: false` in the managed hub's values. Left at `true`, the `acm` element turns `local-cluster` back on and breaks the import.
- Do not name a cluster after the hub that manages it. The global hub's Policies for `hub-a` land in namespace `hub-a` on `hub-a`, where a spoke named `hub-a` would collide.

Hosted mode only: do not create `fleet/<managed-hub>.yaml`. The global hub would render Policies that never reach the hub.

Name each managed hub's ManagedCluster on the global hub after its `hubName`. This is required: `hub` in its clusters' fleet files must match both, `hubName` in hosted mode and the ManagedCluster name in default mode. Global Hub also uses the ManagedCluster name as the hub's identity in its dashboards.

The validator cannot see the `deploy-mode` label. It accepts a `hub` that names any fleet file in `fleet/` or `fleet/hubs/`, so a `hub` pointing at a cluster that is not a default-mode managed hub passes and builds nothing.

## Lifecycle

`policystack-lifecycle` has two generators ([Lifecycle generator](applicationset.md#lifecycle-generator)):

1. Fleet files whose `hub` is this hub's `hubName`, only when this hub manages itself. Placed on its `local-cluster`.
2. Fleet files whose `hub` is a ManagedCluster of this hub labeled `deploy-mode=default`. Placed on that managed hub.

A hosted-mode hub and the global hub build their own clusters with the first. A default-mode hub has no `local-cluster`, renders nothing, and its parent builds for it with the second. ACM delivers the Policies to the managed hub, where every lookup and `fromSecret` resolves. The credential Secret and the BMC Secret therefore live on the managed hub, as they do in hosted mode.

`hypershift` is not supported on a default-mode hub. The HyperShift operator is not installed there, so `present-lifecycle-<cluster>` stays NonCompliant. Use `hive` or `siteconfig`, or hosted mode.

Changing a managed hub's mode is a detach and re-import: detach it from the global hub, flip `selfManaged`, move its file between `fleet/<hub>.yaml` and `fleet/hubs/<hub>.yaml`, and re-import it with the new label. `hive` and `siteconfig` clusters survive and are adopted by the other hub, because no lifecycle Policy sets `pruneObjectBehavior` and `hub` stays the same ([Abandon](lifecycle.md#abandon)). `hypershift` clusters cannot change mode.

## Do not

- Label a PolicyStack hub `global-hub.open-cluster-management.io/hub-role`. Hub HA copies its ApplicationSets, Applications, Policies and `ArgoCD` CR to the standby hub.
- Enable the ACM gitops-addon or Argo CD Agent on a managed hub. They replace `openshift-gitops`, which runs that hub's PolicyStack.
- Reuse a cluster name across hubs. `fleet/` is shared, and migration needs unique names.
- Move a cluster that lifecycle builds with `ManagedClusterMigration`. It moves the ManagedCluster, not its Policies or Applications, and `hypershift` clusters cannot migrate. Imported clusters migrate cleanly: their fleet file is keyed by name, and the target hub picks it up.
