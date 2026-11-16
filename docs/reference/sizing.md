# Hub sizing

Argo CD settings for hubs with many clusters. PolicyStack does not manage the hub's Argo CD instance, `openshift-gitops` ([Enable the elements](../start/install.md#enable-the-elements)), so you set these once by hand on the `ArgoCD` resource. They apply to the whole instance, not only to PolicyStack's Applications.

## What grows

The `policystack` ApplicationSet creates one Application per element per cluster, so a hub with 100 clusters and 15 elements runs 1,500 Applications, plus one `lifecycle-<cluster>` per cluster it builds ([ApplicationSets](../concepts/architecture.md)). Every Application renders on the hub, so all the load lands on one Argo CD instance:

| Component | Work | Grows with |
|---|---|---|
| ApplicationSet controller | Every 3 minutes, renders every Application and writes a status that lists each one. | Applications |
| Application controller | Compares each Application with Git about every 3 minutes. Every 10 seconds, counts the Applications of every cluster secret. | Applications; clusters × Applications |
| Repo server | Runs `helm template` when no cached render matches: a commit to a file the render reads ([Render cache](../concepts/architecture.md#render-cache)), a Redis restart, or after 24 hours. | Applications on a changed revision |
| Server | Holds every Application in memory for the UI and CLI. | Applications |

Count yours with `oc get applications.argoproj.io -n openshift-gitops --no-headers | wc -l`.

## Every hub

Two settings cost nothing at small scale and remove the first two limits a growing hub hits:

| Setting | Component | Why |
|---|---|---|
| `ARGO_CD_UPDATE_CLUSTER_INFO_TIMEOUT=60s` | Application controller | Counting Applications per cluster secret looks up every Application's destination once per cluster. PolicyStack reads none of the result. At 100 clusters with 15 elements, this costs about one CPU core on GitOps 1.18 and 1.19 at the default 10 seconds. Once a minute cuts it by 6. |
| `ARGOCD_APPLICATIONSET_CONTROLLER_MAX_RESOURCES_STATUS_COUNT=5000` | ApplicationSet controller | `policystack`'s status lists every Application, about 180 bytes each. At about 8,600 Applications the object exceeds etcd's 1.5 MiB request limit and the status write fails. The controller then stops creating, updating and deleting Applications ([Stalled ApplicationSet](#stalled-applicationset)). The cap shortens only the list. |

Check the existing environment first. A merge patch replaces each `env` list, so add any entries it prints to the patch:

```sh
oc get argocd openshift-gitops -n openshift-gitops \
  -o jsonpath='{.spec.controller.env}{"\n"}{.spec.applicationSet.env}{"\n"}'

oc patch argocd openshift-gitops -n openshift-gitops --type merge -p '
spec:
  controller:
    env:
      - name: ARGO_CD_UPDATE_CLUSTER_INFO_TIMEOUT
        value: 60s
  applicationSet:
    env:
      - name: ARGOCD_APPLICATIONSET_CONTROLLER_MAX_RESOURCES_STATUS_COUNT
        value: "5000"
'
```

The operator restarts the controller and ApplicationSet controller.

!!! warning "The timeout needs a unit"
    `ARGO_CD_UPDATE_CLUSTER_INFO_TIMEOUT` takes a duration between `10s` and `1m`. `60` or `5m` falls back to `10s`, with only a warning in the controller log.

## By fleet size

Starting points, from benchmarks of Argo CD's code rather than from a live hub. Watch the metrics in the last column and adjust.

| Applications | Setting | Value | Watch |
|---|---|---|---|
| Over 750 (about 50 clusters) | OpenShift GitOps version | 1.20 or later. Argo CD 3.3 counts Applications per cluster 4 to 7 times faster than the 3.1 in GitOps 1.18 and 1.19. | Controller CPU |
| Over 1,500 | `spec.controller.appSync` and controller env `ARGOCD_RECONCILIATION_JITTER` | `600s` and `120s`; over 4,500 Applications, `900s` and `300s` | `argocd_app_reconcile`, `workqueue_depth` |
| Over 1,500 | `spec.repo.replicas`, `spec.repo.resources` | 2 replicas, 2Gi memory; over 4,500 Applications, 3 replicas with 2 CPU | `argocd_repo_pending_request_total`, repo-server OOMKilled |
| Over 3,000 | `spec.server.resources` memory limit | `512Mi`; over 7,500 Applications, `1Gi` | argocd-server OOMKilled |
| Over 4,500 | Controller env `ARGOCD_ENABLE_K8S_EVENT` | `OperationCompleted,StatusRefreshed` | Event creates by the controller |

What each one costs:

- **`appSync`** is how often each Application is compared with Git when nothing pushes a change. Every comparison writes the Application's status, so at 7,500 Applications the default 3 minutes is about 40 writes a second, close to the controller's API rate limit. A longer interval delays a commit to a branch-pinned `revision` by up to that long, unless a [webhook](#webhooks) reports pushes. Set it with `appSync`: the operator overrides `ARGOCD_RECONCILIATION_TIMEOUT` in `env`. Never set `0s`, which turns off comparison.
- **The jitter** spreads the comparisons over that many seconds instead of starting them together.
- **Repo server replicas** matter only for bursts. A commit to `lib/` or the root `values.yaml` re-renders every Application at that revision, about 0.15 CPU seconds each: 7,500 Applications take about 20 minutes on one core. Leave `ARGOCD_REPO_SERVER_PARALLELISM_LIMIT` unset.
- **`ARGOCD_ENABLE_K8S_EVENT`** limits the Kubernetes Events the controller creates. By default each sync creates about four, through the same API rate limit as status writes. The value above keeps sync results and deletion errors, and drops the sync-started and sync and health transition events from the UI's Events tab.

## Webhooks

Without webhooks, a push reaches Argo CD within about 6 minutes ([Fleet files](../concepts/overview.md#fleet-files)). Two webhooks shorten that, at different costs:

- **Argo CD webhook** (`/api/webhook` on the `openshift-gitops-server` Route): refreshes only the Applications whose source matches the push. Cheap at any size, and it offsets a longer `appSync`.
- **ApplicationSet webhook**: re-runs the ApplicationSets with Git caching off. Every push costs about two uncached Git revision lookups per cluster, which a large hub or a rate-limited Git server will feel. Above about 100 clusters, prefer the Argo CD webhook and let fleet changes wait for the 3-minute cycle.

## Only PolicyStack on the instance

Argo CD caches every resource type on the hub so it can track what its Applications own. An ACM hub has hundreds of types and many objects per cluster. If the instance runs only PolicyStack, restrict it to the kinds PolicyStack renders:

```sh
oc patch argocd openshift-gitops -n openshift-gitops --type merge -p '
spec:
  resourceInclusions: |
    - apiGroups: [policy.open-cluster-management.io]
      kinds: [Policy, PolicySet, PlacementBinding]
    - apiGroups: [cluster.open-cluster-management.io]
      kinds: [Placement]
'
```

!!! danger "Every other Application loses the excluded kinds"
    Argo CD skips excluded kinds without an error. Another Application on this instance that renders a Deployment never applies it. Use this only on an instance nothing else uses.

Leave out `clusters` in each entry. Argo CD matches it against the in-cluster API server's internal address, not `https://kubernetes.default.svc`, so an entry with that URL matches nothing.

## Stalled ApplicationSet

| Symptom | Cause | Fix |
|---|---|---|
| New fleet files and elements get no Applications, and deleted ones keep theirs, while the ApplicationSet shows no `ErrorOccurred` condition. | The status exceeds etcd's request size. The ApplicationSet controller log shows `etcdserver: request is too large`. | Set `ARGOCD_APPLICATIONSET_CONTROLLER_MAX_RESOURCES_STATUS_COUNT` ([Every hub](#every-hub)). |

```sh
oc logs -n openshift-gitops deploy/openshift-gitops-applicationset-controller | grep 'request is too large'
```

## Not worth changing

- **Controller sharding.** Argo CD shards by destination cluster, and every PolicyStack Application targets the hub, so all of them stay on one shard.
- **Status and operation processors, client QPS.** The controller is CPU-bound well before it runs out of workers or API rate below about 9,500 Applications.
- **`ignoreResourceUpdates` for status.** Argo CD 3.x already ignores status-only updates by default.
- **`orphanedResources` on the `default` project.** Every change to an unmanaged object in an Application's destination namespace, `open-cluster-management`, would then refresh every PolicyStack Application. ACM's own pods, leases and ConfigMaps live there.
