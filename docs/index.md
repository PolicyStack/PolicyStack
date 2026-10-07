# PolicyStack

PolicyStack manages OpenShift cluster configuration as Red Hat Advanced Cluster Management (ACM) policies kept in Git. Each directory under `stack/` is an element, a Helm chart that renders policies through the [policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md) chart. On the ACM hub, an Argo CD ApplicationSet creates one Application per element for every managed cluster that has a [fleet file](applicationset.md#fleet-files), which pins the cluster to a Git revision. Argo CD renders each Application at that revision and syncs it to the hub only. ACM delivers each policy to the single cluster it targets, where the governance add-on enforces it or reports compliance.

A fleet file that names a hub in `hub` also makes that hub build the cluster as a hosted control plane, and destroy it on `state: absent` ([Cluster lifecycle](lifecycle.md)).

```mermaid
flowchart LR
  subgraph repo["Git"]
    fl["fleet/<br>default branch"]
    el["stack/#lt;element#gt;/<br>element chart"]
    vf["values.yaml<br>values/**"]
    lc["lifecycle/<br>lifecycle chart"]
  end
  subgraph hub["ACM hub"]
    mc["ManagedClusters"]
    goc["GitOpsCluster"]
    sec["Argo CD cluster secrets"]
    appset["ApplicationSet"]
    app["Application #lt;element#gt;-#lt;cluster#gt;<br>syncs to the hub only"]
    pol["namespace policy<br>Policy, PolicySet,<br>Placement, PlacementBinding"]
    rep["namespace #lt;cluster#gt;<br>replicated Policy"]
    lcset["ApplicationSet<br>policystack-lifecycle"]
    lcapp["Application lifecycle-#lt;cluster#gt;"]
    hc["HostedCluster, NodePools<br>built by the hub's own<br>governance add-on"]
  end
  subgraph spoke["Managed cluster"]
    gov["governance add-on<br>enforces or reports"]
  end
  mc -->|"Placement global"| goc
  goc -->|"creates one per cluster"| sec
  sec -->|"imported clusters"| appset
  fl -->|"revision and valueFiles, at HEAD"| appset
  el -->|"directories stack/*"| appset
  appset -->|"one per element × cluster"| app
  el -->|"chart"| app
  vf -->|"valueFiles"| app
  app -->|"Helm render and sync"| pol
  pol -->|"Placement selects one cluster"| rep
  rep -->|"synced to the cluster"| gov
  gov -.->|"compliance status"| rep
  fl -->|"files with hub: #lt;hubName#gt;"| lcset
  lcset -->|"one per fleet file"| lcapp
  lc -->|"chart"| lcapp
  vf -->|"valueFiles"| lcapp
  lcapp -->|"Policies placed on the hub"| pol
  pol -->|"present or absent"| hc
  hc -->|"auto-import"| mc
```

## How a change reaches a cluster

1. Change an element, the root `values.yaml` or a file under `values/` on the branch or tag that the cluster's fleet file pins. Clusters pinned to other revisions are unaffected. A change to the fleet file itself counts only on the default branch.
2. Argo CD renders the element chart with the cluster's [values cascade](values.md#order) and syncs the Policy, PolicySet, Placement and PlacementBinding objects into the `policy` namespace on the hub.
3. The [Placement](policies.md#placement) selects only that cluster. ACM replicates each Policy into the cluster's namespace on the hub, and the governance add-on on the managed cluster applies it. `enforce` changes the cluster, `inform` only reports. Compliance status returns to the replicated Policy on the hub.

## Terms

| Term | Meaning |
|---|---|
| [Element](elements.md) | A directory under `stack/` holding a Helm chart that depends on the policy-library chart. Its values define the policies it renders. |
| [Fleet file](applicationset.md#fleet-files) | A file on the repo's default branch that declares one cluster: `fleet/<cluster>.yaml`, or `fleet/hubs/<hubName>.yaml` for a hub. It sets the cluster's revision and the values files it reads, and optionally the hub that builds the cluster. |
| [Hub](install.md#prerequisites) | The cluster that runs ACM and OpenShift GitOps. It runs the ApplicationSets and holds the Applications and Policies, and the control planes of the clusters it builds. By default ACM also manages the hub as `local-cluster`, so it can have a fleet file like any other managed cluster, at `fleet/hubs/<hubName>.yaml` ([The hub](applicationset.md#the-hub)). |
| [Managed cluster](rollout.md#onboarding-a-cluster) | A cluster that ACM manages and the GitOpsCluster imports into Argo CD, either imported by hand or built by a hub ([Cluster lifecycle](lifecycle.md)). It gets one Application per element once it has a fleet file. |
| [Release](applicationset.md#applications) | The Application name, which Argo CD also uses as the Helm release name. `<element>-<cluster>`, or `<element>-<hubName>` on the hub. Policy, PolicySet, Placement and PlacementBinding names are built from it. |
| [Revision](applicationset.md#revision) | The Git branch, tag or SHA in a cluster's fleet file. All of that cluster's Applications render from it. |
| [Values cascade](values.md#order) | The values files Argo CD passes to Helm for one Application, most of them listed in `valueFiles` in the cluster's fleet file. Later files override earlier ones. |
