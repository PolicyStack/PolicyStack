# Global Hub

The [`global-hub`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/global-hub) element installs Red Hat's Multicluster Global Hub operator on one ACM hub, the **global hub**, and creates its `MulticlusterGlobalHub`. From that object the operator deploys the Global Hub manager, Grafana, and its own Kafka (AMQ Streams) and PostgreSQL.

The element does not import managed hubs. What Global Hub does, and how importing a hub changes which hub renders what: [Global Hub](../guides/global-hub.md).

## Before you turn it on

| Need | Without it |
|---|---|
| ACM running on the hub | The operator has nothing to attach to. Grafana is only deployed once ACM's resources are ready |
| A default StorageClass, or `config.storageClass` | The Kafka and PostgreSQL volumes stay `Pending` and `ready` stays NonCompliant |
| ACM observability turned on first, if you want it (`stack.acm.policies.observability`) | Red Hat: once Global Hub is installed, a `MultiClusterObservability` enabled afterwards ignores later changes. Let `observability-acm-<hub>` go Compliant before enabling this element |

!!! warning "FIPS hubs"
    Red Hat lists a known issue ([ACM-12592](https://issues.redhat.com/browse/ACM-12592)): on a FIPS cluster the AMQ Streams operator keeps restarting out of memory. Red Hat's workaround raises its memory limit to at least 512M, but the Global Hub operator rewrites the AMQ Streams Subscription on every reconcile ([Disconnected](#disconnected)), so a limit set there does not hold. Check Red Hat's Global Hub release notes before enabling the element on a FIPS hub.

## Turn it on

Enable it in the global hub's layer only:

```yaml
# values/global-hub/gh.yaml, listed in fleet/hubs/gh.yaml
stack:
  globalHub:
    enabled: true
    config:
      availabilityConfig: Basic # (1)!
```

1.  A lab setting. Kafka keeps the broker count it was first created with, so choose before the first enable.

| Key under `stack.globalHub.config` | Default | Allowed | Sets |
|---|---|---|---|
| `availabilityConfig` | `High` | `High`, `Basic` | `High` runs the manager and Grafana with two replicas and Kafka with three brokers, each topic replicated three times. `Basic` runs one of each. Changing it later resizes the manager and Grafana but not Kafka |
| `retention` | `18m` | Years and months, in that order: `6m`, `1y`, `1y6m` | How long PostgreSQL keeps history. `m` is months, not minutes. Red Hat recommends at least one month |
| `storageClass` | `""` | Any StorageClass name | The class of the Kafka and PostgreSQL volumes. Empty uses the cluster default. Set it before the first enable and never change it: StatefulSet volume templates are immutable, so the operator's PostgreSQL update is rejected and `ready` stays NonCompliant until you set the old value back |

Any other `availabilityConfig` or `retention` fails the render. The CRD accepts any string for both. The operator compares `availabilityConfig` case-sensitively, giving two manager and Grafana replicas only for `High` and a one-broker Kafka only for `Basic`, so `basic` would mix the two. It rejects a malformed retention only at runtime.

## What it creates

| Policy | Templates | Creates |
|---|---|---|
| `install-global-hub-<hub>` | `install-global-hub-ns`, `install-global-hub`, `install-global-hub-status` | Namespace `multicluster-global-hub`, an OperatorGroup that targets only that namespace (the operator has no all-namespaces mode), and Subscription `multicluster-global-hub-operator-rh` on channel `release-1.8` |
| `config-global-hub-<hub>` | `config-global-hub` | `MulticlusterGlobalHub` `multiclusterglobalhub` in `multicluster-global-hub` |
| `ready-global-hub-<hub>` | `ready-global-hub` | Nothing. Reports NonCompliant until the instance is running ([Ready](#ready)) |

The template uses `musthave`: fields it doesn't list keep their CRD defaults (`enableMetrics` and `installAgentOnLocal` are `true`), and clearing `storageClass` leaves the old value on the object.

!!! warning "One instance per cluster"
    The operator reconciles only while exactly one `MulticlusterGlobalHub` exists cluster-wide. Do not create another from the console: with two, the operator stops managing both, and logs that only at debug level.

## Version

Channel `release-1.8` is Global Hub 1.8. Red Hat's ACM 2.17 Global Hub docs support it on global hubs and managed hubs running ACM 2.15 to 2.17, which covers PolicyStack's ACM 2.16 floor and the `acm` element's `release-2.17`. Global Hub 1.7 (`release-1.7`) supports only ACM 2.14 to 2.16.

Upgrade approval is `Automatic`, so the hub follows the channel head, which is past the 1.8.2 security minimum ([Global Hub](../guides/global-hub.md#what-global-hub-does)). The channel already holds the minor; only 1.8 z-streams arrive. Before moving ACM past 2.17, check Red Hat's Global Hub docs for that ACM version and set `operatorPolicies.global-hub.subscription.channel` in the same layer.

!!! danger "Don't pin this operator"
    Leave `versions` empty and `upgradeApproval` at `Automatic`. Either one makes the OperatorPolicy set the Global Hub Subscription to manual approval ([Pinning](../guides/operators.md#pinning)), and OLM then makes every InstallPlan in `multicluster-global-hub` manual. That includes the one for the AMQ Streams Subscription the Global Hub operator creates there, which no OperatorPolicy manages, so nothing approves it: Kafka never installs on a new hub, and on a running hub any InstallPlan that carries an AMQ Streams CSV is held.

!!! warning "After a Global Hub upgrade"
    - Red Hat's procedure says to add `global-hub.open-cluster-management.io/deploy-mode=default` back to every default-mode managed hub after upgrading Global Hub. In default mode that label decides which fleet file the global hub renders the managed hub from ([The label picks the fleet file](../guides/global-hub.md#import-a-managed-hub)). Check it with `oc get managedcluster -L global-hub.open-cluster-management.io/deploy-mode`.
    - Before changing the channel, delete every `ManagedClusterMigration` (`oc delete mcm -A --all`). Otherwise the upgrade can fail CRD validation with `[].spec.from: Required value`.

## Ready

`ready-global-hub` matches `status.phase: Running`. The operator sets it once the manager, PostgreSQL and Kafka are available, and Grafana when ACM is ready. A component that never becomes available leaves the phase at `Progressing`, never `Error`, so this check is where a broken install shows. On the global hub:

```sh
oc get mgh multiclusterglobalhub -n multicluster-global-hub -o jsonpath='{.status.components}' # (1)!
oc get pods,pvc -n multicluster-global-hub
oc get route multicluster-global-hub-grafana -n multicluster-global-hub # (2)!
```

1.  One entry per component, each with a `reason` and `message`.
2.  The Grafana dashboards, once `ready` is Compliant.

## Disconnected

The operator installs AMQ Streams itself, through a Subscription it creates in `multicluster-global-hub` (package `amq-streams`, channel `amq-streams-3.1.x`, catalog `redhat-operators`). It overwrites that Subscription on every reconcile, so editing it does nothing. When `global.catalogSources` maps `redhat-operators` to the mirrored catalog ([Site layer](../guides/disconnected.md#site-layer)), the element adds two annotations to the `MulticlusterGlobalHub` that point the Subscription at the same catalog. There is nothing to set.

Mirror both packages. The `imageset` subcommand sees only the Global Hub operator, so add `--package amq-streams:amq-streams-3.1.x` to the run that covers every cluster the mirror serves ([What to mirror](../guides/disconnected.md#what-to-mirror)).

## Turning it off

Turning off the element deletes its Policies but nothing they created: no template sets `pruneObjectBehavior` ([Removal and pruning](../concepts/removal.md#what-happens-on-the-managed-cluster)). The operator, the `MulticlusterGlobalHub`, Kafka, PostgreSQL and the stored history stay. Red Hat documents no uninstall procedure. Deleting the `MulticlusterGlobalHub` makes the operator remove Kafka, its volumes and the AMQ Streams operator before it releases the object; uninstall the Global Hub operator only after the object is gone. The PostgreSQL volume, which holds the history, stays in `multicluster-global-hub` until you delete it.
