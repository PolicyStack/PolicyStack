# Install chart

The [install chart](https://github.com/PolicyStack/PolicyStack/tree/main/install) puts PolicyStack on a hub. It installs no elements. It creates the pointer and the two ApplicationSets that turn fleet files into Applications, the `pstack` namespace the Policies live in, the objects that give the ApplicationSets their list of clusters, the RBAC Argo CD needs to write ACM policy objects, and the Policy that installs PolicyStack on Global Hub child hubs. Everything after it comes from Git.

The hub's own Argo CD syncs the chart from your repository. You apply one object by hand, once per hub: the **pointer** ([Install PolicyStack](../start/install.md#install-policystack)). A Global Hub child imported in `default` mode doesn't: its parent's Policy applies the pointer ([Child hubs](#child-hubs)).

[ApplicationSets](../concepts/architecture.md) explains how the objects work together.

!!! note "Two charts under `install/`"
    `install/operators` is a separate chart, rendered once to install ACM and OpenShift GitOps on a new hub ([Install the operators](../start/install.md#install-the-operators)). Argo CD never reads it.

## The pointer

The pointer is the ApplicationSet `policystack-install`. It reads one file, `fleet/hubs/<hubName>.yaml`, from the default branch and generates one Application, `policystack`, which renders `install/` with Helm and keeps it applied. That render includes the pointer itself, so after the first apply the pointer is managed from Git like everything else.

```text
ApplicationSet policystack-install   reads fleet/hubs/<hubName>.yaml at HEAD
  └─ Application policystack         renders install/ at the file's installRevision
       ├─ ApplicationSet policystack-install   (itself)
       ├─ ApplicationSets policystack, policystack-lifecycle
       └─ namespace, bindings, Placement, GitOpsCluster, RBAC, Policy policystack-hubs
```

The Application renders `install/` at the hub file's `installRevision`, or at the default branch when the file doesn't set one ([Install revision](fleet.md#installrevision)). A merged change under `install/`, which only a framework update brings, reaches every hub that follows the default branch within one Argo CD refresh, about 3 minutes.

!!! warning "The hub file must exist before the pointer"
    Without `fleet/hubs/<hubName>.yaml` on the default branch, the pointer generates nothing and the hub gets no install. Deleting the file later deletes the Application `policystack` but none of what it created ([Deletion safety](#deletion-safety)): the hub keeps running its last install, no longer updated from Git.

## Values

The pointer passes these to the chart. You set them once, on the command that applies the pointer.

| Value | Required | What it does | Changing it later |
|---|---|---|---|
| `gitRepo` | Yes | Clone URL of your repository, such as `https://github.com/<org>/<repo>.git`. The `repoURL` of the pointer, both ApplicationSets and every Application. For a private repository it must equal the `url` of the Argo CD repository Secret that holds its credentials ([Private repository](../start/install.md#install-policystack)). | Re-apply the pointer with the new URL. Every Application is re-pointed at once, so the new repository needs the same fleet files and every pinned revision. |
| `hubName` | Yes | This hub's name in PolicyStack: picks `fleet/hubs/<hubName>.yaml` and `values/clusters/<hubName>.yaml`, names the hub's Applications `<element>-<hubName>`, and selects the fleet files whose `hub` key names this hub as the builder ([Fleet keys](../concepts/lifecycle.md#fleet-keys), [The hub](../concepts/architecture.md#the-hub)). Use a lowercase DNS label of at most 30 characters, because it ends up in Policy names ([Naming limits](../concepts/policies.md#naming-limits)). Unique per active hub reading the repo; a DR hub takes over the old hub's name. | Renames and recreates every hub Application and its Policies, and drops lifecycle Applications for `hub: <old>` ([Changing `hubName`](../concepts/architecture.md#changing-hubname)). |
| `repoSecret` | For a private repository with child hubs | Name of the Argo CD repository Secret in `openshift-gitops` that holds `gitRepo`'s credentials. Child hubs get a copy under the same name ([Child hubs](#child-hubs)). | Re-apply the pointer. A cleared value leaves the copies on the children. |
| `policyNamespace` | Default `pstack`, in `install/values.yaml` | Namespace the chart creates and binds the `global` set into. Elements read their own copy from the root `values.yaml`. | Leave at `pstack`. Changing it, even in both files, renames every Policy, so all are deleted and recreated ([Removal and pruning](../concepts/removal.md)). CI rule [POLICY071](rules.md) checks that `install/values.yaml` and the root `values.yaml` agree. |

`parent` is set only by a parent's Policy, on a child's pointer. Never set it yourself. A child's `hubName` is its ManagedCluster name on the parent, and its `gitRepo` and `repoSecret` are the parent's: change them on the parent.

To change a value, apply the pointer again with every value, as in [Install PolicyStack](../start/install.md#install-policystack). The ApplicationSet controller regenerates the Application `policystack` within seconds. If Argo CD's self-heal restores the old pointer first, the read-back below still shows the old value: apply it again.

The chart only checks that `gitRepo` and `hubName` are set. A `hubName` with uppercase letters or `_` installs, but the hub's Applications are never created: ApplicationSet `policystack` shows an `ErrorOccurred` condition with the API server's invalid-name message. Re-apply the pointer with a valid name and name the hub's files to match.

Read the values back from the live pointer:

```sh
oc get applicationset policystack-install -n openshift-gitops \
  -o jsonpath='{.spec.template.spec.source.helm.valuesObject}'
```

## Objects

Three different objects are called `global`: ACM's built-in ManagedClusterSet that contains every ManagedCluster, and this chart's ManagedClusterSetBindings and Placement that use it.

| Object | Name | Namespace | Why it exists |
|---|---|---|---|
| ApplicationSet | `policystack-install` | `openshift-gitops` | The pointer. Generates the Application `policystack` that syncs this chart ([The pointer](#the-pointer)). Not rendered on a child hub, whose parent's Policy owns it. |
| ApplicationSet | `policystack` | `openshift-gitops` | One Application per element per cluster that has a fleet file and a `clusterID` label ([`policystack`](../concepts/architecture.md#policystack)). |
| ApplicationSet | `policystack-lifecycle` | `openshift-gitops` | One Application per cluster this hub builds, from fleet files with `hub: <hubName>` ([Lifecycle generator](../concepts/architecture.md#lifecycle-generator)). |
| Namespace | `pstack` (`policyNamespace`) | | Holds every Policy, PolicySet, Placement and PlacementBinding the element and lifecycle Applications render. |
| ManagedClusterSetBinding | `global` | `openshift-gitops`, `pstack` | A Placement can only select clusters from sets bound into its own namespace. `openshift-gitops` is for the two Placements below; `pstack` for every element Placement. |
| Placement | `global` | `openshift-gitops` | Selects every cluster for the GitOpsCluster. Tolerates `unreachable` and `unavailable`, so a cluster that loses contact keeps its Argo CD secret, and with it its Applications and Policies ([Cluster secrets](../concepts/architecture.md#cluster-secrets)). |
| GitOpsCluster | `argo-acm-importer-global` | `openshift-gitops` | Creates an Argo CD cluster secret, with the ManagedCluster's labels, for each cluster the Placement selects. The secrets are only the ApplicationSets' list of clusters: `argoServer.cluster` is `notused`, a placeholder for a required field nothing reads, and every Application deploys to the hub. |
| Policy, Placement, PlacementBinding | `policystack-hubs` | `openshift-gitops` | Installs PolicyStack on each child hub ([Child hubs](#child-hubs)). The only framework Policy outside `pstack`, because `copySecretData` reads only its own Policy's namespace. |
| ClusterRole, ClusterRoleBinding | `openshift-gitops-policy-admin` | | Lets Argo CD write what the chart and the Applications render ([RBAC](#rbac)). |

!!! warning "AppProject `default`"
    The Application `policystack` and every generated Application use the `default` AppProject, which the chart does not create. OpenShift GitOps ships it unrestricted. If yours is restricted, allow your repository's URL as a source, cluster-scoped resources, and `https://kubernetes.default.svc` with namespaces `openshift-gitops`, `open-cluster-management` and `pstack` as destinations. Otherwise every Application shows an `InvalidSpecError` saying the source repository or destination is not permitted in project `default`.

### RBAC

The default Argo CD instance's application controller, `openshift-gitops-argocd-application-controller`, applies everything. OpenShift GitOps grants it namespaces, RBAC objects and everything in `openshift-gitops`, but not ACM's policy and cluster objects. `openshift-gitops-policy-admin`, the role from ACM's ["Policy definitions with GitOps"](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/gitops/), adds them:

- `policies`, `configurationpolicies`, `certificatepolicies`, `operatorpolicies`, `policysets`, `placementbindings`, `placementrules`, `placements`, `placementdecisions` and the two status subresources: what the elements and lifecycle render.
- `gitopsclusters` and `managedclustersetbindings`: objects of this chart.
- `create` on `managedclustersets/bind` for the set `global` only. ACM admits a ManagedClusterSetBinding only from a user allowed to bind the set.

It is bound only to that service account, which is why PolicyStack needs the default Argo CD instance. Merge rights on `install/` therefore amount to admin rights on every hub, as merge rights on `values/` already do through ACM: protect both ([Protect the repository](../start/install.md#protect-the-repository)).

### Deletion safety

Argo CD applies a merged change to `install/` on its own, so no change there may delete what the chart created. Deleting either ApplicationSet would delete every Application and every Policy it generated. Three guards prevent it, and [POLICY071](rules.md) fails CI if any goes missing:

1. Every object carries `argocd.argoproj.io/sync-options: Prune=false,Delete=false`. Argo CD reads it from the live object, so a release that drops or renames a template leaves the old object in place, and deleting the Application deletes nothing. The release notes then give the `oc delete` command.
2. The Application `policystack` has no `resources-finalizer` and syncs with `prune: false`.
3. `helm.sh/resource-policy: keep` stays on every object, for hubs installed with `helm install` before 1.0.

A render error, such as a missing required value, makes Application `policystack` show a `ComparisonError` and applies nothing.

### Backup and restore

ACM's cluster backup (the hub backup and restore operator) continuously restores the active hub's objects onto a passive DR hub. The ApplicationSets, the Applications they generate, the Placements, the GitOpsCluster, and the Policy and PlacementBinding `policystack-hubs` carry `velero.io/exclude-from-backup: "true"`, so the passive hub doesn't start PolicyStack while the active hub still runs. A restored `policystack-lifecycle` would build the active hub's clusters a second time. POLICY071 fails if the label goes missing.

The namespace, bindings and RBAC are not excluded and are harmless if restored. After a takeover, apply the pointer on the new hub with the `hubName` of the hub it replaces ([Hub disaster recovery](../guides/lifecycle.md#hub-disaster-recovery)).

## Child hubs

A child hub is an ACM hub imported into this hub's [Multicluster Global Hub](../guides/global-hub.md) in `default` mode, labelled `global-hub.open-cluster-management.io/deploy-mode=default`. This hub already renders the child's own Day-2 from `fleet/hubs/<child>.yaml`. The Policy `policystack-hubs`, placed on every cluster with that label, adds the child's install:

- **The pointer**, with the child's ManagedCluster name as `hubName`, this hub's `gitRepo` and `repoSecret`, and this hub's name as `parent`. The child's own Argo CD then syncs `install/` from `fleet/hubs/<child>.yaml`, at its own `installRevision`. Because `parent` is set, the child's render leaves the pointer out, so the Policy is its only owner.
- **A copy of `repoSecret`**, kept equal to this hub's Secret, when `repoSecret` is set. Rotate the credential on this hub only.

The pointer's ConfigurationPolicy turns off managed-cluster templates, so the Argo CD templates in it reach the child as written. Only the hub template that names the child resolves, on this hub. `pruneObjectBehavior: None`: removing the label, the hub file or this hub never uninstalls a child.

| Situation | Result |
|---|---|
| Labelled cluster with no `fleet/hubs/<child>.yaml` | It gets a pointer that generates nothing. |
| GitOps operator not yet on the child | `policystack-hubs` is NonCompliant until the operator creates the ApplicationSet CRD, then applies. The Application `policystack` retries until ACM's CRDs exist. |
| Child imported in `hosted` mode | Global Hub turns off the governance add-on for it, so the Policy never reaches it. Install it by hand like any hub ([Global Hub](../guides/global-hub.md#import-modes)). |
| Pointer applied by hand on a `default`-mode child | The Policy adopts it. Use the same `hubName` as the child's ManagedCluster name. |

## Teardown

[Teardown](../start/install.md#teardown).
