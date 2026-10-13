# Installation

PolicyStack runs on the ACM hub. Run the commands below from the repository root, logged in to the hub as a cluster administrator.

## Prerequisites

- Red Hat Advanced Cluster Management on the hub.
- OpenShift GitOps, with its default Argo CD instance in `openshift-gitops`.
- The Argo CD application controller service account bound to `cluster-admin`. Argo CD creates the policy objects on the hub with this account:

    ```sh
    oc adm policy add-cluster-role-to-user cluster-admin \
      -z openshift-gitops-argocd-application-controller -n openshift-gitops
    ```

- `oc` and `helm`.
- On a hub that builds clusters from fleet files: ACM 2.16 or later and the rest of [Cluster lifecycle prerequisites](lifecycle.md#prerequisites).

## Import clusters into Argo CD

```sh
oc apply -f gitops-prereq/
```

[`gitops-prereq/`](https://github.com/PolicyStack/PolicyStack/tree/main/gitops-prereq) creates these objects in `openshift-gitops`:

| Object | Name | Purpose |
|---|---|---|
| ManagedClusterSetBinding | `global` | Binds ACM's `global` ManagedClusterSet, which holds every managed cluster, into `openshift-gitops`. |
| Placement | `global` | Selects every cluster in `global` and tolerates the `unreachable` and `unavailable` taints. |
| GitOpsCluster | `argo-acm-importer-global` | Creates an Argo CD cluster secret for each cluster the Placement selects. |

Each managed cluster then has an Argo CD cluster secret:

```sh
oc get secrets -n openshift-gitops -l argocd.argoproj.io/secret-type=cluster
```

## Install the ApplicationSet

Set `gitRepo` in [`appset/values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/appset/values.yaml), the repository Argo CD reads ([The appset chart](applicationset.md#the-appset-chart)). Then install the chart, passing the root `values.yaml` for `policyNamespace` and this hub's `hubName`. `hubName` names the hub's fleet file, `fleet/hubs/<hubName>.yaml`, and its Applications, because every hub's own ManagedCluster is named `local-cluster` by default ([The hub](applicationset.md#the-hub)):

```sh
helm install appset ./appset -f ./appset/values.yaml -f values.yaml --set hubName=<name>
helm upgrade appset ./appset -f ./appset/values.yaml -f values.yaml --set hubName=<name>
```

`hubName` has no default, and the chart fails without it. Every hub that reads the same repo, including a passive or disaster recovery hub, needs its own. Pass it on every `helm upgrade` too, since `helm upgrade` drops `--set` values it is not given again.

On an upgrade from a chart that set `preserveResourcesOnDeletion: true`, existing Applications gain the resources finalizer on their next reconcile, and an Application left OutOfSync on a disabled element prunes on its next automated sync.

The chart creates:

| Object | Name | Namespace | On `helm uninstall` |
|---|---|---|---|
| ApplicationSet | `policystack` | `openshift-gitops` | Kept |
| ApplicationSet | `policystack-lifecycle` | `openshift-gitops` | Kept |
| Namespace | `policy` | cluster-scoped | Kept |
| ManagedClusterSetBinding | `global` | `policy` | Kept |

Every object carries `helm.sh/resource-policy: keep`. The binding lets the element Placements in `policy` select clusters ([Placement](policies.md#placement)). Helm refuses to install over an existing object the release does not own. If a hand-made `global` ManagedClusterSetBinding already exists in `policy`, add `--take-ownership` (Helm 3.17 or later) to adopt it. Deleting it instead unbinds the Placements that use it until Helm recreates it.

`helm uninstall appset` therefore leaves every Application and Policy running. A later `helm install` under the release name `appset` adopts the objects. To tear PolicyStack down, delete the ApplicationSets:

```sh
oc delete applicationset policystack policystack-lifecycle -n openshift-gitops
```

This deletes every Application and every Policy they rendered ([Removing an element](rollout.md#removing-an-element)), and every cluster this hub built keeps running, unmanaged ([Abandon](lifecycle.md#abandon)). Add `--cascade=orphan` to delete only the ApplicationSets and keep the Applications and Policies.

Next: add a fleet file for the hub and each managed cluster ([Onboarding a cluster](rollout.md#onboarding-a-cluster)).
