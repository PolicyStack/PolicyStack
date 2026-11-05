# Install chart

Values the [install chart](https://github.com/PolicyStack/PolicyStack/tree/main/install) takes and the objects it creates.

| Value | Set in | Use |
|---|---|---|
| `gitRepo` | `--set` | Repo everything reads. Required. |
| `hubName` | `--set` | This hub's identity ([The hub](../concepts/architecture.md#the-hub)). Required. |
| `policyNamespace` | `install/values.yaml` | Must match the root `values.yaml` (CI checks). Do not change. |

| Object | Name | Namespace | Purpose |
|---|---|---|---|
| ApplicationSet | `policystack` | `openshift-gitops` | One Application per element per cluster ([Applications](../concepts/architecture.md#applications)) |
| ApplicationSet | `policystack-lifecycle` | `openshift-gitops` | One Application per cluster this hub builds ([Lifecycle generator](../concepts/architecture.md#lifecycle-generator)) |
| Namespace | `pstack` | | Holds the Policies |
| ManagedClusterSetBinding | `global` | `openshift-gitops`, `pstack` | Lets Placements select every cluster |
| Placement | `global` | `openshift-gitops` | Every cluster, tolerating `unreachable` and `unavailable` |
| GitOpsCluster | `argo-acm-importer-global` | `openshift-gitops` | Creates an Argo CD cluster secret per selected cluster |
| ClusterRole, ClusterRoleBinding | `openshift-gitops-policy-admin` | | Lets the Argo CD application controller manage Policies, PolicySets, Placements and PlacementBindings, the only objects elements render |

!!! note "Not in ACM backup"
    The ApplicationSets, their Applications, the Placement and the GitOpsCluster carry `velero.io/exclude-from-backup: "true"`. A restored `policystack-lifecycle` on a passive hub would build the old hub's clusters there.

!!! warning "Objects outlive the release"
    Every object carries `helm.sh/resource-policy: keep`. `helm uninstall policystack` leaves PolicyStack running. To remove it, see [Teardown](../start/install.md#teardown).

Preview the render:

```sh
helm template policystack ./install --set hubName=acm-dc1 --set gitRepo=https://github.com/PolicyStack/PolicyStack.git
```

Upgrade it as in [Upgrading PolicyStack](../guides/upgrading.md#install-chart).
