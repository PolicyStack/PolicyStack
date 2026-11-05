# Architecture

Two ApplicationSets in `openshift-gitops` on the hub turn [fleet files](../reference/fleet.md) into Argo CD Applications:

| ApplicationSet | Creates | Template |
|---|---|---|
| `policystack` | One Application per element per cluster. Day-2 config. | [`appset.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/install/templates/appset.yaml) |
| `policystack-lifecycle` | One Application per fleet file this hub builds ([Cluster lifecycle](../guides/lifecycle.md), [Global Hub](../guides/global-hub.md)). | [`lifecycle.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/install/templates/lifecycle.yaml) |

## How a change reaches a cluster

A commit lands on the revision a cluster pins ([First cluster](../start/first-cluster.md) walks through one). Clusters pinned to other revisions are unaffected. Then:

1. **Argo CD renders** the element with the cluster's [values cascade](values.md#order) and syncs Policy, PolicySet, Placement and PlacementBinding into `pstack` on the hub.
2. **ACM delivers.** The [Placement](policies.md#placement) selects only that cluster, and its governance add-on applies the policy: `enforce` changes the cluster, `inform` only reports. Compliance shows on the replicated Policy in the cluster's namespace on the hub.

## Generators

`policystack` pairs every cluster secret with its fleet file at `HEAD`, then with every `stack/*` directory at the cluster's `revision`. An element added on a branch reaches only clusters pinned to a revision containing it. Disabled elements still get an Application that renders nothing.

??? example "`policystack` generators, rendered with `hubName=acm-dc1`"

    ```yaml
    generators:
      - matrix:
          generators:
            - matrix:
                generators:
                  - clusters: # (1)!
                      selector:
                        matchExpressions:
                          - key: clusterID
                            operator: Exists
                  - git: # (2)!
                      repoURL: 'https://github.com/PolicyStack/PolicyStack.git'
                      revision: HEAD
                      pathParamPrefix: fleet # (3)!
                      files:
                      - path: 'fleet/{{if index .metadata.labels "local-cluster"}}hubs/acm-dc1{{else if eq (index .metadata.labels "global-hub.open-cluster-management.io/deploy-mode") "default"}}hubs/{{.name}}{{else}}{{.name}}{{end}}.yaml'
            - git: # (4)!
                repoURL: 'https://github.com/PolicyStack/PolicyStack.git'
                revision: '{{.revision}}'
                directories:
                - path: 'stack/*'
    ```

    1.  Every cluster secret with `clusterID`. The GitOpsCluster creates one per managed cluster.
    2.  The cluster's fleet file at `HEAD`. Its keys become parameters. No file, no Applications.
    3.  Stops the file's `path` from shadowing the element directory's `path` in the matrix.
    4.  Every `stack/*` directory at the cluster's `revision`. One Application per cluster and directory.

### Lifecycle generator

`policystack-lifecycle` reads fleet files with a `hub` key. Files without `hub` match neither generator, so imported clusters get no lifecycle Application. The cluster secret's `.name` is the ManagedCluster the Policies are placed on.

??? example "`policystack-lifecycle` generators, rendered with `hubName=acm-dc1`"

    ```yaml
    generators:
      - matrix: # (1)!
          generators:
            - clusters:
                selector:
                  matchExpressions:
                    - key: local-cluster
                      operator: Exists
            - git: &fleet
                repoURL: 'https://github.com/PolicyStack/PolicyStack.git'
                revision: HEAD
                files:
                - path: 'fleet/*.yaml'
                - path: 'fleet/*/*.yaml' # (2)!
                  exclude: true
              selector:
                matchExpressions:
                  - key: hub
                    operator: In
                    values:
                      - "acm-dc1"
      - matrix: # (3)!
          generators:
            - git: *fleet
              selector:
                matchExpressions:
                  - key: hub
                    operator: Exists
            - clusters:
                selector:
                  matchLabels:
                    apps.open-cluster-management.io/cluster-name: '{{ if regexMatch "^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$" (toString .hub) }}{{ .hub }}{{ end }}' # (4)!
                    global-hub.open-cluster-management.io/deploy-mode: default
    ```

    1.  Fleet files with `hub: acm-dc1`, paired with this hub's own cluster secret. A hub that does not manage itself builds nothing.
    2.  Argo CD's `*` crosses `/`. This drops `hubs/` and other subdirectories.
    3.  Fleet files naming a managed hub imported in default mode ([Global Hub](../guides/global-hub.md#lifecycle)). Fleet files come first because Argo CD templates a child generator's fields but not its `selector`.
    4.  A `hub` that is not a DNS label matches nothing instead of failing the whole ApplicationSet.

## Applications

| Field | `policystack` | `policystack-lifecycle` |
|---|---|---|
| Name | `<element>-<cluster>`, `<element>-<hubName>` on the hub | `lifecycle-<cluster>` |
| Source | `stack/<element>` at the cluster's revision | `lifecycle/` at the fleet file's revision |
| Values | [Cascade](values.md#order) | Same |
| Destination | Hub, `open-cluster-management` | Same |
| Sync | Automated, `prune`, `selfHeal`, `allowEmpty`, `RespectIgnoreDifferences=true` | Automated, `prune`, `selfHeal` |

The Application name is the Helm release name, which every object name derives from ([Naming limits](policies.md#naming-limits)). Argo CD deploys only to the hub; ACM delivers policies to clusters ([Placement](policies.md#placement)). Objects land in `policyNamespace`; the destination namespace is unused. `allowEmpty` lets a disabled element prune to zero objects ([Disabling an element](../guides/rollout.md#disabling-an-element)).

Both ApplicationSets set `preserveResourcesOnDeletion: false`: deleting an Application deletes its hub objects ([Removal and pruning](removal.md)). Deleting a lifecycle Application leaves the cluster running unmanaged ([Abandon](../guides/lifecycle.md#abandon)).

!!! note "Deletion is bounded"
    A failed render prunes nothing (`ComparisonError`). A failed generation deletes nothing. Argo CD only deletes hub objects; ACM then removes policies from the cluster ([What happens on the managed cluster](removal.md#what-happens-on-the-managed-cluster)).

`copyPolicyMetadata: false` keeps Argo CD tracking metadata off ACM's per-cluster Policy copies, so Argo CD never prunes them.

### Render cache

Each Application lists what its render reads in [`manifest-generate-paths`](https://argo-cd.readthedocs.io/en/stable/operator-manual/high_availability/#manifest-paths-annotation). For `compliance-prod-east-1`:

```yaml
argocd.argoproj.io/manifest-generate-paths: '.;../../values.yaml;../../values/environments/prod.yaml;../../values/datacenters/dc1.yaml;../../values/platforms/aws.yaml;../../values/clusters/prod-east-1.yaml'
```

A new commit re-renders only Applications whose listed paths changed. Fleet file changes alter the spec and always re-render.

## The hub

The hub's ManagedCluster has `local-cluster`, so it reads `fleet/hubs/<hubName>.yaml` ([Hub file](../reference/fleet.md#hub-file)). `hubName` is set once at `helm install`, no default ([Install PolicyStack](../start/install.md#install-policystack)).

`hubName` replaces the cluster name everywhere on the hub:

| | Managed cluster | Hub |
|---|---|---|
| Applications | `<element>-<cluster>` | `<element>-<hubName>` |
| Cluster values file | `values/clusters/<cluster>.yaml` | `values/clusters/<hubName>.yaml` |
| `selectedName` | `<cluster>` | `<hubName>` |

!!! warning
    Changing `hubName` renames the hub's Applications and every object they render.

Every hub reading the repo, DR hubs included, sets its own `hubName`. The chart fails without it rather than apply another hub's config. Check it with `helm get values policystack -n openshift-gitops`.

??? question "Why not use the cluster name?"
    Nothing on a hub is stable and unique. Every hub's ManagedCluster defaults to `local-cluster`. ACM's `localClusterName` changes only at install. `clusterID` changes on rebuild. `hubs/` keeps hub files from colliding with a managed cluster of the same name.

A Global Hub managed hub imported in default mode has no `local-cluster`; the global hub reads its file instead ([Global Hub](../guides/global-hub.md#layout)).

## Day-2 handoff

A cluster the hub builds gets Day-2 config the same way as an imported one:

1. The ManagedCluster appears. `hypershift`: the hypershift-addon creates it once the HostedCluster is available. `hive`, `siteconfig`: ACM imports the one the chart or SiteConfig created.
2. It joins `global`, the GitOpsCluster creates its Argo CD secret, and ACM adds the `clusterID` label ([Install chart](../reference/install.md)).
3. The Day-2 ApplicationSet pairs the secret with the same fleet file and creates `<element>-<cluster>` Applications. It ignores `hub`, `state` and `install`.

Labels in `hostedCluster.metadata.labels` are copied to the ManagedCluster once, at creation.

??? info "HyperShift auto-import disabled"
    Auto-import is on by default. `autoImportDisabled` in the `hypershift-addon-deploy-config` AddOnDeploymentConfig leaves a built cluster without Day-2 until you import it by hand.

## Terms

| Term | Meaning |
|---|---|
| [Element](../develop/elements.md) | A `stack/<element>/` Helm chart that depends on policy-library. Its `converters/<policy>/<template>.yaml` files and `operatorPolicies` define the policies; values configure them. |
| [Fleet file](../reference/fleet.md) | `fleet/<cluster>.yaml`, or `fleet/hubs/<hubName>.yaml` for the hub, on the default branch. Sets the revision, the values files and optionally the hub that builds the cluster. |
| [Hub](#the-hub) | The cluster running ACM and OpenShift GitOps. Holds the ApplicationSets, Applications, Policies and the control planes it builds. ACM manages it as `local-cluster`, so you give it a fleet file too, at `fleet/hubs/<hubName>.yaml`. |
| [Managed cluster](../start/first-cluster.md) | A cluster ACM manages, imported by hand or built by a hub. The GitOpsCluster imports it into Argo CD; it gets one Application per element once it also has a fleet file. |
| [Release](#applications) | The Application name, also the Helm release name: `<element>-<cluster>`, or `<element>-<hubName>` on the hub. Policy and Placement names derive from it. |
| [Revision](../reference/fleet.md#revision) | The branch, tag or SHA in a fleet file. All of the cluster's Applications render from it. |
| [Values cascade](values.md#order) | The ordered values files Helm gets for one Application. Later files override earlier ones. |
