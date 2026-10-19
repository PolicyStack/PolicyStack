# Global Hub

Running PolicyStack when ACM hubs sit under a [Multicluster Global Hub](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.17/html/multicluster_global_hub/index).

## What Global Hub does

Global Hub is an operator on one ACM hub, the **global hub**. The ACM hubs it imports are **managed hubs**. An agent on each managed hub streams its Policies, compliance, events and ManagedClusters through Kafka into PostgreSQL, and Grafana reads them there. It can also move clusters between managed hubs (`ManagedClusterMigration`, Technology Preview).

It does not push configuration. Global policy distribution was removed in 1.7. PolicyStack and each hub's ACM still deliver everything.

!!! warning "Minimum version"
    Use Global Hub 1.8.2 ([RHSA-2026:68515](https://access.redhat.com/errata/RHSA-2026:68515)) or 1.7.3 ([RHSA-2026:67842](https://access.redhat.com/errata/RHSA-2026:67842)) or later. Earlier releases let one managed hub spoof another's compliance data ([CVE-2026-71576](https://access.redhat.com/security/cve/CVE-2026-71576)). 1.8 pairs with ACM 2.17, 1.7 with 2.16.

## Import modes

A label on the managed hub's ManagedCluster (on the global hub) picks the mode. No label, no Global Hub agent.

```yaml
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: hub-a # (1)!
  labels:
    global-hub.open-cluster-management.io/deploy-mode: default # (2)!
```

1.  Must match the managed hub's `hubName`. Default mode finds the hub's files by this name, hosted mode by `hubName`, so matching them means a mode change moves no file. Global Hub also shows this name in its dashboards.
2.  `default` or `hosted`.

| | `default` | `hosted` |
|---|---|---|
| Managed hub's klusterlet runs on | The managed hub | The global hub |
| Managed hub has `local-cluster` | No (`disableHubSelfManagement: true`) | Yes |
| Global hub's Policies reach the managed hub | Yes | No. The webhook disables the policy and application addons |
| Reads `fleet/hubs/<managed-hub>.yaml` | Global hub | Managed hub |
| Builds the managed hub's clusters | Global hub, placed on the managed hub ([Lifecycle](#lifecycle)) | Managed hub |
| `hypershift` engine on the managed hub | No | Yes |
| When to set the label | Any time | Before import. Already-imported clusters are ignored |

??? info "Hosted mode requirements"
    - The global hub manages itself.
    - The import kubeconfig never expires.
    - The global hub reaches each managed hub's API.
    - Backup and restore of the managed hub is not supported.

## Layout

Install PolicyStack on the global hub and on every managed hub, each with its own `hubName` ([Install PolicyStack](install.md#install-policystack)). All hubs read the same `fleet/`. A hub's ApplicationSets only see the ManagedClusters it imports, so each fleet file renders Day-2 on one hub and lifecycle on one hub. For a cluster under a default-mode hub those differ: Day-2 renders on the managed hub, lifecycle on the global hub.

```text
fleet/
  hubs/gh.yaml      # the global hub
  hubs/hub-a.yaml   # default-mode managed hub, read by gh
  hubs/hub-b.yaml   # hosted-mode managed hub, read by hub-b
  edge-1.yaml       # hub: hub-a, built by gh on hub-a
  edge-2.yaml       # hub: hub-b, built by hub-b
```

=== "Default mode"

    Turn off `local-cluster` in the managed hub's values. Left at `true`, the `acm` element turns it back on and breaks the import.

    ```yaml
    # values/acm/hub-a.yaml, listed in fleet/hubs/hub-a.yaml
    stack:
      acm:
        config:
          selfManaged: false
    ```

    Then:

    1. Commit `fleet/hubs/hub-a.yaml`.
    2. Label the ManagedCluster.

        ```sh
        oc label managedcluster hub-a global-hub.open-cluster-management.io/deploy-mode=default
        ```

    !!! danger "The label picks the fleet file"
        With the label, the global hub reads `fleet/hubs/hub-a.yaml`. Without it, `fleet/hub-a.yaml`. Removing the label deletes the hub's Day-2 Policies ([Removing an element](rollout.md#removing-an-element)).

    - Do not name a managed cluster after the hub that manages it. The global hub's Policies for `hub-a` land in namespace `hub-a` on `hub-a`, where a managed cluster `hub-a` would collide.
    - A hub the global hub builds also has `fleet/hub-a.yaml` for its lifecycle keys. After import, its upgrade `version` and `channel` go in `fleet/hubs/hub-a.yaml`.

=== "Hosted mode"

    Set the label in the ManagedCluster before importing. Nothing else changes on the managed hub: it keeps `local-cluster` and reads its own `fleet/hubs/hub-b.yaml`.

    !!! warning
        Do not create `fleet/hub-b.yaml` for Day-2. The global hub would render Policies that never reach the hub.

## Lifecycle

`policystack-lifecycle` has two generators ([Lifecycle generator](applicationset.md#lifecycle-generator)):

| Generator | Matches fleet files whose `hub` is | Places the Policy on |
|---|---|---|
| 1 | This hub's `hubName`, if this hub has `local-cluster` | `local-cluster` |
| 2 | A ManagedCluster of this hub labeled `deploy-mode=default` | That managed hub |

The global hub and hosted-mode hubs build their own clusters with generator 1. A default-mode hub has no `local-cluster`, so it renders nothing and the global hub builds for it with generator 2.

```yaml
# fleet/edge-1.yaml, rendered by gh, Policy placed on hub-a
revision: main
hub: hub-a
version: 4.20.8
channel: stable-4.20
install:
  engine: hive
  credential:
    namespace: hive-credentials
    name: aws                     # (1)!
  baseDomain: aws.example.com
  hive:
    platform:
      aws:
        region: us-east-2
```

1.  The Secret must exist on hub-a, not the global hub.

ACM delivers the Policies to the managed hub, where every lookup and `fromSecret` resolves. The credential and BMC Secrets live on the managed hub in both modes.

!!! note "No `hypershift` on a default-mode hub"
    The HyperShift operator is not installed there, so `present-lifecycle-<cluster>` stays NonCompliant. Use `hive`, `siteconfig`, or hosted mode.

??? info "Changing a managed hub's mode"
    1. Detach the hub from the global hub.
    2. Flip `stack.acm.config.selfManaged`.
    3. Re-import with the new label.

    Fleet files stay where they are. `hive` and `siteconfig` clusters survive and the other hub adopts them: no lifecycle Policy sets `pruneObjectBehavior` and `hub` is unchanged ([Abandon](lifecycle.md#abandon)). `hypershift` clusters cannot change mode.

## Do not

- Label a PolicyStack hub `global-hub.open-cluster-management.io/hub-role`. Hub HA copies its ApplicationSets, Applications, Policies and `ArgoCD` CR to the standby.
- Enable the ACM gitops-addon or Argo CD Agent on a managed hub. They replace `openshift-gitops`, which runs that hub's PolicyStack.
- Reuse a cluster name across hubs. `fleet/` is shared and migration needs unique names.
- Migrate a lifecycle-built cluster with `ManagedClusterMigration`. It moves the ManagedCluster, not its Policies or Applications, and `hypershift` clusters cannot migrate. Imported clusters migrate cleanly: the target hub picks up the same `fleet/<cluster>.yaml`.
