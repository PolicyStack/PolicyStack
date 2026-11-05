# Global Hub

Read this only if your ACM hubs are, or will be, imported into a [Multicluster Global Hub](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.17/html/multicluster_global_hub/index). PolicyStack does not need one: several hubs can share one fork, each installed with its own `hubName`.

Import each managed hub in **hosted mode** (a Global Hub import mode, unrelated to hosted control planes; see [Import modes](#import-modes)) unless you want the global hub to configure and build clusters for it. In hosted mode every hub keeps running PolicyStack for itself and nothing in the repo changes. Default mode removes the managed hub's `local-cluster`, rules out `hypershift` on it, and makes its hub templates resolve on the global hub.

## What Global Hub does

Global Hub is an operator on one ACM hub, the **global hub**. The ACM hubs it imports are **managed hubs**. An agent on each managed hub streams its Policies, compliance, events and ManagedClusters through Kafka into PostgreSQL, and Grafana reads them there. It can also move clusters between managed hubs (`ManagedClusterMigration`, Technology Preview).

It does not push configuration: global policy distribution was removed in 1.7. Each hub's PolicyStack and ACM still deliver everything.

!!! warning "Minimum version"
    Use Global Hub 1.8.2 ([RHSA-2026:68515](https://access.redhat.com/errata/RHSA-2026:68515)) or 1.7.3 ([RHSA-2026:67842](https://access.redhat.com/errata/RHSA-2026:67842)) or later. Earlier releases let one managed hub spoof another's compliance data ([CVE-2026-71576](https://access.redhat.com/security/cve/CVE-2026-71576)). 1.8 pairs with ACM 2.17, 1.7 with 2.16.

## What the mode decides

Terms used below:

- **Day-2**: the element Policies the `policystack` ApplicationSet renders for a cluster. **Lifecycle**: building and destroying clusters, rendered by `policystack-lifecycle` ([Cluster lifecycle](../concepts/lifecycle.md)).
- **`local-cluster`**: the ManagedCluster a hub creates for itself. PolicyStack needs it to find the hub's own fleet file, `fleet/hubs/<hubName>.yaml`, and to place the Policies that build clusters.
- **Klusterlet**: ACM's agent through which a hub manages a cluster.

A hub's ApplicationSets see only the ManagedClusters that hub manages: its GitOpsCluster turns each one into an Argo CD cluster secret, and the ApplicationSets pair those secrets with fleet files ([ApplicationSets](../concepts/architecture.md)). Global Hub only mirrors status, so the global hub has no secret for a cluster under a managed hub, and that cluster's Day-2 always renders on the managed hub. The import mode decides two other things: which hub renders the managed hub's own Day-2, and which hub builds clusters whose fleet `hub` names the managed hub.

## Import modes

You import a managed hub into the global hub's ACM like any cluster, following [Red Hat's Global Hub import procedure](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.17/html/multicluster_global_hub/index); PolicyStack does not do it. A label on its ManagedCluster, on the global hub, picks the mode. Without the label, the hub gets no Global Hub agent.

```yaml
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: hub-a # (1)!
  labels:
    global-hub.open-cluster-management.io/deploy-mode: default # (2)!
```

1.  Use the managed hub's `hubName`. In default mode the global hub knows the hub only by this name: it reads `fleet/hubs/<name>.yaml` and `values/clusters/<name>.yaml`, and fleet `hub:` keys must use it. hub-a's own PolicyStack install still needs a `hubName`, but in default mode hub-a has no `local-cluster`, so it renders nothing for itself and never reads that value. Set it to the same name anyway, so the files work in both modes and Global Hub's dashboards match.
2.  `default` or `hosted`.

| | `default` | `hosted` |
|---|---|---|
| Managed hub's klusterlet runs on | The managed hub | The global hub |
| Managed hub has `local-cluster` | No. ACM requires `disableHubSelfManagement: true` on its MultiClusterHub | Yes |
| Global hub's Policies reach the managed hub | Yes | No. Global Hub's admission webhook turns off the policy and application add-ons for that hub |
| Renders the managed hub's Day-2, from `fleet/hubs/<managed-hub>.yaml` | Global hub | Managed hub |
| Builds clusters whose `hub` is the managed hub | Global hub, Policies placed on the managed hub ([Lifecycle](#lifecycle)) | Managed hub |
| `hypershift` engine on the managed hub | No | Yes |
| When to set the label | Any time | Before import. An already-imported cluster is ignored |

??? info "Hosted mode requirements"
    - The global hub has its own `local-cluster` (the ACM default).
    - The kubeconfig you give at import stays valid: use a token that doesn't expire, such as a service-account token, not an `oc login` token. When it expires the global hub loses the managed hub until you regenerate `auto-import-secret` in the managed hub's namespace on the global hub.
    - The global hub reaches each managed hub's API.
    - Red Hat does not support backup and restore of a hosted-mode managed hub, which [Hub disaster recovery](lifecycle.md#hub-disaster-recovery) relies on.

## Layout

Install PolicyStack on the global hub and on every managed hub, all with the same `gitRepo` and each with its own `hubName` ([Install PolicyStack](../start/install.md#install-policystack)). All hubs read the same `fleet/`.

```text
fleet/
  hubs/gh.yaml      # the global hub; Day-2 rendered by gh
  hubs/hub-a.yaml   # default-mode managed hub; Day-2 rendered by gh
  hubs/hub-b.yaml   # hosted-mode managed hub; Day-2 rendered by hub-b
  edge-1.yaml       # hub: hub-a; Day-2 rendered by hub-a, built by gh on hub-a
  edge-2.yaml       # hub: hub-b; Day-2 rendered and built by hub-b
```

## Import a managed hub

=== "Hosted mode"

    Set the label on the ManagedCluster before importing. Nothing else changes: the managed hub keeps `local-cluster`, reads its own `fleet/hubs/hub-b.yaml` and builds its own clusters.

    !!! warning "No `fleet/hub-b.yaml`"
        The global hub has a cluster secret for hub-b with neither `local-cluster` nor the `default` label, so its Day-2 ApplicationSet looks for `fleet/hub-b.yaml`. If that file exists, the global hub creates `<element>-hub-b` Applications whose Policies never reach hub-b and never report compliance. A hub the global hub builds is imported as soon as it is built, before you could set the `hosted` label, so it can't use hosted mode: use default mode.

=== "Default mode"

    For hub-a:

    1. Install PolicyStack on hub-a and commit `fleet/hubs/hub-a.yaml`, as for any hub. An existing PolicyStack hub has both. hub-a's PolicyStack applies step 2 and keeps rendering Day-2 for hub-a's managed clusters.
    2. Turn off self-management in hub-a's layer. The `acm` element must be on, which means [handing the operators to PolicyStack](../start/install.md#hand-the-operators-to-policystack) first:

        ```yaml
        # values/acm/hub-a.yaml, listed in fleet/hubs/hub-a.yaml
        stack:
          acm:
            enabled: true
            config:
              selfManaged: false
        ```

        Merge. hub-a's own PolicyStack sets `disableHubSelfManagement: true` and ACM removes `local-cluster`. With it go hub-a's Applications for itself (the Policies are deleted, what they created stays) and its `lifecycle-<cluster>` Applications: clusters with `hub: hub-a` are abandoned, not destroyed, until step 4. `oc get managedcluster local-cluster` on hub-a returns `NotFound` when done.

        From here until step 4, nothing enforces hub-a's own Policies or runs its builds. Do steps 3 and 4 together.

    3. Import hub-a into the global hub ([Red Hat's procedure](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.17/html/multicluster_global_hub/index)).
    4. Label it on the global hub:

        ```sh
        oc label managedcluster hub-a global-hub.open-cluster-management.io/deploy-mode=default
        ```

        The global hub now reads `fleet/hubs/hub-a.yaml`, creates `<element>-hub-a` Applications, and takes over building the clusters with `hub: hub-a`. It re-adopts the abandoned ones by their [ownership marker](../concepts/lifecycle.md#ownership).

    A hub the global hub builds with `hive` or `siteconfig` has two fleet files: `fleet/hub-a.yaml`, holding its lifecycle keys, and `fleet/hubs/hub-a.yaml`. Until the label is set, the global hub renders hub-a's Day-2 from `fleet/hub-a.yaml`; after, from `fleet/hubs/hub-a.yaml`. Both carry a `version`, and each drives [upgrades](upgrades.md) while the global hub reads it: `fleet/hub-a.yaml` before the label, `fleet/hubs/hub-a.yaml` after. Keep them equal, and label hub-a as soon as it is imported.

    !!! danger "The label picks the fleet file"
        With the label, the global hub reads `fleet/hubs/hub-a.yaml`; without it, `fleet/hub-a.yaml`. Removing the label deletes hub-a's Day-2 Policies and the `lifecycle-<cluster>` Applications for every cluster with `hub: hub-a`, which leaves those clusters abandoned ([What deletes an Application](../concepts/removal.md#what-deletes-an-application)). Restoring the label re-adopts them.

    !!! warning "Hub templates resolve on the global hub"
        An ACM hub template (`{{hub ... hub}}`) resolves on the hub that delivers the Policy. hub-a's own Day-2 Policies now come from the global hub, so the `acs` element on hub-a registers hub-a with the global hub's Central, and fails if the global hub runs none. Either run Central on the global hub, or keep `stack.acs.enabled: false` in hub-a's layer. hub-a's managed clusters still use hub-a's Central. `acs` is the only element that uses hub templates.

## Lifecycle

`policystack-lifecycle` has two generators, the parts of an ApplicationSet that each produce one Application per match ([Lifecycle generator](../concepts/architecture.md#lifecycle-generator)). "Placed on" means the lifecycle Placement selects that ManagedCluster, so ACM runs the build steps there.

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

ACM delivers the Policies to hub-a, and their `lookup` and `fromSecret` calls are managed-cluster template functions, evaluated on the cluster the Policy lands on. Lifecycle uses no hub templates, so credential and BMC Secrets live on the managed hub in both modes. To watch a default-mode build:

```sh
oc get policies.policy.open-cluster-management.io present-lifecycle-edge-1 -n pstack  # on the global hub
oc get configurationpolicy -n hub-a                # on hub-a: each step and its message
```

!!! note "No `hypershift` on a default-mode hub"
    ACM installs the HyperShift operator through the hypershift-addon on a hub's own `local-cluster`, which a default-mode hub does not have. `present-lifecycle-<cluster>` stays NonCompliant. Use `hive`, `siteconfig`, or hosted mode.

## Changing a managed hub's mode

Fleet files stay where they are. Detach means deleting hub-a's ManagedCluster on the global hub (`oc delete managedcluster hub-a`), ACM's way to remove a cluster from management.

`hive` and `siteconfig` clusters survive the change. Losing the hub that builds them (detaching from the global hub, or removing hub-a's `local-cluster`) deletes their `lifecycle-<cluster>` Applications, but no lifecycle template sets `pruneObjectBehavior`, so nothing is deleted on hub-a ([Removal and pruning](../concepts/removal.md)). The hub that takes over adopts them by their marker because `hub` is unchanged ([Abandon](lifecycle.md#abandon)). `hypershift` clusters of a hosted-mode hub keep it in hosted mode: destroy them first, since a default-mode hub can't run `hypershift`.

=== "Hosted to default"

    1. Detach hub-a from the global hub.
    2. Run steps 2 to 4 of [Default mode](#import-a-managed-hub). hub-a's own PolicyStack still runs, so it applies `selfManaged: false`.

=== "Default to hosted"

    hub-a has no `local-cluster`, so after detaching nothing delivers its Policies, and `selfManaged: true` renders no `disableHubSelfManagement` field that could undo the old setting. Flip it by hand:

    1. Detach hub-a from the global hub. Its Day-2 and lifecycle Applications on the global hub are deleted; its clusters are abandoned, not destroyed.
    2. Set `selfManaged: true`, or remove the key, in hub-a's layer and merge. Otherwise step 3 brings back hub-a's own PolicyStack, which removes `local-cluster` again.
    3. On hub-a:

        ```sh
        oc patch multiclusterhub multiclusterhub -n open-cluster-management --type merge \
          -p '{"spec":{"disableHubSelfManagement":false}}'
        ```

        `local-cluster` returns. hub-a's PolicyStack reads `fleet/hubs/hub-a.yaml` again and adopts its clusters.

    4. Re-import hub-a with the `hosted` label set before import.

## Do not

- Label a PolicyStack hub `global-hub.open-cluster-management.io/hub-role`. That turns on Hub HA, Global Hub's active/standby feature, which copies the ApplicationSets, Applications, Policies and `ArgoCD` CR to the standby, where they enforce the same Policies and build the same clusters. PolicyStack's recovery path is ACM backup plus a reinstall with the same `hubName` ([Hub disaster recovery](lifecycle.md#hub-disaster-recovery)).
- Enable the ACM gitops-addon or Argo CD Agent on a managed hub. They replace `openshift-gitops`, which runs that hub's PolicyStack.
- Name a managed cluster after the hub that manages it, in default mode. ACM puts a cluster's Policies in a namespace named after that cluster: the global hub's Policies for hub-a land in namespace `hub-a` on hub-a, and hub-a keeps the Policies for each of its own clusters in a namespace named after that cluster, so a managed cluster `hub-a` would share it.
- Reuse a cluster name across hubs. `fleet/` is shared, and migration needs unique names.
- Migrate a lifecycle-built cluster with `ManagedClusterMigration`. It moves the ManagedCluster, not its Policies or Applications, and `hypershift` clusters cannot migrate. Imported clusters migrate cleanly: the source hub deletes their Argo CD secret, and with it their Applications and Policies, pruning nothing on the cluster ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)). The target hub pairs its new secret with the same `fleet/<cluster>.yaml` and recreates them. If `acs` is enabled, run steps 2-3 of [Re-register a cluster](../elements/acs.md#re-register-a-cluster), since Central is now the target hub's.
