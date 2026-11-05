# Install chart

The [install chart](https://github.com/PolicyStack/PolicyStack/tree/main/install) is the only part of PolicyStack you install by hand, once per hub ([Install PolicyStack](../start/install.md#install-policystack)). It installs no elements. It creates the two ApplicationSets that turn fleet files into Applications, the `pstack` namespace the Policies live in, the objects that give the ApplicationSets their list of clusters, and the RBAC Argo CD needs to write ACM policy objects. Everything after it comes from Git.

[ApplicationSets](../concepts/architecture.md) explains how the objects work together.

!!! note "Two charts under `install/`"
    `install/operators` is a separate chart, rendered once to install ACM and OpenShift GitOps on a new hub ([Install the operators](../start/install.md#install-the-operators)). It is not part of the `policystack` release.

## Values

| Value | Required | Set with | What it does | Changing it later |
|---|---|---|---|---|
| `gitRepo` | Yes | `--set` | Clone URL of your fork, such as `https://github.com/<org>/<fork>.git`. The `repoURL` of both ApplicationSets' git generators and of every Application. For a private fork it must equal the `url` of the Argo CD repository Secret that holds the fork's credentials ([Private fork](../start/install.md#install-policystack)). | `helm upgrade policystack ./install -n openshift-gitops --reset-then-reuse-values --set gitRepo=<url>` re-points every Application at once, so the new repo needs the same fleet files and every pinned revision. |
| `hubName` | Yes | `--set` | This hub's name in PolicyStack: picks `fleet/hubs/<hubName>.yaml` and `values/clusters/<hubName>.yaml`, names the hub's Applications `<element>-<hubName>`, and selects the fleet files whose `hub` key names this hub as the builder ([Fleet keys](../concepts/lifecycle.md#fleet-keys), [The hub](../concepts/architecture.md#the-hub)). Use a lowercase DNS label of at most 30 characters, because it ends up in Policy names ([Naming limits](../concepts/policies.md#naming-limits)). Unique per active hub reading the repo; a DR hub takes over the old hub's name. | Renames and recreates every hub Application and its Policies, and drops lifecycle Applications for `hub: <old>` ([Changing `hubName`](../concepts/architecture.md#several-hubs-one-repo)). |
| `policyNamespace` | Default `pstack` | `install/values.yaml` | Namespace the chart creates and binds the `global` set into. Elements read their own copy from the root `values.yaml`. | Leave at `pstack`. Changing it, even in both files, renames every Policy, so all are deleted and recreated ([Removal and pruning](../concepts/removal.md)). If `install/values.yaml` and the root `values.yaml` differ, elements render into a namespace this chart did not create or bind: their Applications fail to sync or their Placements select nothing. CI rule [POLICY071](rules.md) compares the two files; a `--set policyNamespace` at install bypasses it. |

The chart only checks that `gitRepo` and `hubName` are set. A `hubName` with uppercase letters or `_` installs, but the hub's Applications are never created: `policystack` shows an `ErrorOccurred` condition with the API server's invalid-name message. Fix it with `helm upgrade policystack ./install -n openshift-gitops --reset-then-reuse-values --set hubName=<valid>` and name the hub's files to match.

Both values are stored in the Helm release. Read them back with `helm get values policystack -n openshift-gitops`.

## Objects

Three different objects are called `global`: ACM's built-in ManagedClusterSet that contains every ManagedCluster, and this chart's ManagedClusterSetBindings and Placement that use it. The release and the first ApplicationSet are both named `policystack`.

| Object | Name | Namespace | Why it exists |
|---|---|---|---|
| ApplicationSet | `policystack` | `openshift-gitops` | One Application per element per cluster that has a fleet file and a `clusterID` label ([`policystack`](../concepts/architecture.md#policystack)). |
| ApplicationSet | `policystack-lifecycle` | `openshift-gitops` | One Application per cluster this hub builds, from fleet files with `hub: <hubName>` ([Lifecycle generator](../concepts/architecture.md#lifecycle-generator)). |
| Namespace | `pstack` (`policyNamespace`) | | Holds every Policy, PolicySet, Placement and PlacementBinding the Applications render. |
| ManagedClusterSetBinding | `global` | `openshift-gitops`, `pstack` | A Placement can only select clusters from sets bound into its own namespace. `openshift-gitops` is for the Placement below; `pstack` for every element Placement. |
| Placement | `global` | `openshift-gitops` | Selects every cluster for the GitOpsCluster. Tolerates `unreachable` and `unavailable`, so a cluster that loses contact keeps its Argo CD secret, and with it its Applications and Policies ([Cluster secrets](../concepts/architecture.md#cluster-secrets)). |
| GitOpsCluster | `argo-acm-importer-global` | `openshift-gitops` | Creates an Argo CD cluster secret, with the ManagedCluster's labels, for each cluster the Placement selects. The secrets are only the ApplicationSets' list of clusters: `argoServer.cluster` is `notused`, a placeholder for a required field nothing reads, and every Application deploys to the hub. |
| ClusterRole, ClusterRoleBinding | `openshift-gitops-policy-admin` | | The OpenShift GitOps application controller cannot create or change ACM policy objects. Grants it get, list, watch, create, update, patch and delete on `policies`, `configurationpolicies`, `certificatepolicies`, `operatorpolicies`, `policysets`, `placementbindings`, `placementrules`, `placements`, `placementdecisions` and the two status subresources, the role from ACM's ["Policy definitions with GitOps"](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/gitops/). Bound only to `openshift-gitops-argocd-application-controller`, which is why PolicyStack needs the default Argo CD instance. |

!!! warning "AppProject `default`"
    Every generated Application uses the `default` AppProject, which the chart does not create. OpenShift GitOps ships it unrestricted. If yours is restricted, allow your fork's URL as a source and `https://kubernetes.default.svc` with namespaces `open-cluster-management` and `pstack` as destinations, or every Application shows an `InvalidSpecError` saying the source repository or destination is not permitted in project `default`.

### Backup and restore

ACM's cluster backup (the hub backup and restore operator) continuously restores the active hub's objects onto a passive DR hub. The ApplicationSets, their Applications, the Placement and the GitOpsCluster carry `velero.io/exclude-from-backup: "true"` so the passive hub doesn't start PolicyStack while the active hub still runs: a restored `policystack-lifecycle` would build the active hub's clusters a second time. CI rule [POLICY071](rules.md) fails if the label goes missing.

The namespace, bindings and RBAC are not excluded and are harmless if restored. After a takeover, run `helm install` on the new hub with the `hubName` of the hub it replaces ([Hub disaster recovery](../guides/lifecycle.md#hub-disaster-recovery)).

### Uninstall

Every object carries `helm.sh/resource-policy: keep`, so `helm uninstall policystack -n openshift-gitops` removes only the release record and PolicyStack keeps running. Deleting the ApplicationSets deletes every Application and every Policy they rendered, and abandons the clusters this hub built: they keep running, unmanaged ([Destroy and abandon](../concepts/lifecycle.md#destroy-and-abandon)). Neither `helm uninstall` nor a failed `helm upgrade` can delete them. Teardown is a deliberate `oc delete` ([Teardown](../start/install.md#teardown)).

## Preview and upgrade

```sh
helm template policystack ./install --set hubName=acm-dc1 --set gitRepo=https://github.com/<org>/<fork>.git
```

Run from the root of your fork. A merged change under `install/` does nothing until you run `helm upgrade` on the hub. Then it reaches every cluster on that hub at once, because the ApplicationSets are not pinned to any fleet file's `revision`. Upgrade as in [Upgrading PolicyStack](../guides/upgrading.md#install-chart).
