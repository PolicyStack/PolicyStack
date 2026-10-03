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

Set `gitRepo` and `hubName` in [`appset/values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/appset/values.yaml). `gitRepo` is the repository Argo CD reads ([The appset chart](applicationset.md#the-appset-chart)). `hubName` names this hub's fleet file, `fleet/hubs/<hubName>.yaml`, because every hub's own ManagedCluster is named `local-cluster` by default ([The hub](applicationset.md#the-hub)). Then install the chart, passing the root `values.yaml` for `policyNamespace`:

```sh
helm install appset ./appset -f ./appset/values.yaml -f values.yaml
```

Every additional hub that reads the same repo, including a passive or disaster recovery hub, needs its own `hubName`. Otherwise it applies the first hub's configuration to itself. Pass it on every `helm install` and `helm upgrade` of that hub, since `helm upgrade` drops `--set` values it is not given again:

```sh
helm install appset ./appset -f ./appset/values.yaml -f values.yaml --set hubName=<name>
helm upgrade appset ./appset -f ./appset/values.yaml -f values.yaml --set hubName=<name>
```

The chart creates:

| Object | Name | Namespace | On `helm uninstall` |
|---|---|---|---|
| ApplicationSet | `policystack` | `openshift-gitops` | Deleted |
| Namespace | `policy` | cluster-scoped | Kept |
| ManagedClusterSetBinding | `global` | `policy` | Kept |

The namespace and the binding carry `helm.sh/resource-policy: keep`. The binding lets the element Placements in `policy` select clusters ([Placement](policies.md#placement)). Helm refuses to install over an existing object the release does not own. If a hand-made `global` ManagedClusterSetBinding already exists in `policy`, add `--take-ownership` (Helm 3.17 or later) to adopt it. Deleting it instead unbinds the Placements that use it until Helm recreates it.

`helm uninstall appset` deletes every Application with the ApplicationSet but leaves their Policies enforcing ([Removing an element](rollout.md#removing-an-element)).

Next: add a fleet file for the hub and each managed cluster ([Onboarding a cluster](rollout.md#onboarding-a-cluster)).
