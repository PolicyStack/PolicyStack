# Clusters and Applications

Two ApplicationSets in `openshift-gitops` on the hub turn fleet files into Argo CD Applications:

| ApplicationSet | Creates | Template |
|---|---|---|
| `policystack` | One Application per element per cluster. Day-2 config. | [`appset.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/install/templates/appset.yaml) |
| `policystack-lifecycle` | One Application per fleet file this hub builds ([Cluster lifecycle](lifecycle.md), [Global Hub](global-hub.md)). | [`lifecycle.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/install/templates/lifecycle.yaml) |

## Fleet files

One file per cluster: `fleet/<cluster>.yaml`, named exactly like its ManagedCluster. The hub uses `fleet/hubs/<hubName>.yaml` ([The hub](#the-hub)).

=== "Imported cluster"

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main
    channel: stable-4.20
    valueFiles: # (1)!
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/aws.yaml
    ```

    1.  Paths under `values/`, lowest precedence first ([Order](values.md#order)). Any file works, so a custom layer like `tenants/payments.yaml` is just a file you list.

=== "Hub-built cluster"

    ```yaml
    # fleet/hcp-kubevirt.yaml
    revision: main
    hub: acm-dc1 # (1)!
    version: 4.20.8
    valueFiles:
      - environments/nonprod.yaml
      - platforms/kubevirt.yaml
    ```

    1.  Needs `fleet/hubs/acm-dc1.yaml` to exist.

| Key | Required | Effect |
|---|---|---|
| `revision` | Yes | Branch, tag or SHA every Application renders from ([Revision](#revision)). |
| `valueFiles` | No | Values layers, lowest precedence first. |
| `hub` | No | `hubName` of the hub that builds the cluster. Without it the cluster must be imported. |
| `state` | No | `present` (default) or `absent`, which destroys the cluster ([Destroy](lifecycle.md#destroy)). Requires `hub`. |
| `install` | No | The cluster to build ([Install keys](lifecycle-reference.md#install-keys)). Requires `hub`. |
| `version` | With `hub` | OpenShift release, `x.y.z`. Bumping it upgrades ([Upgrades](upgrades.md)). |
| `channel` | No | Upgrade channel. |

The [validator](validation.md) enforces: no other keys, every `valueFiles` entry exists, and a hub file sets no `hub`, `state` or `install`. `version` and `channel` go only in fleet files ([Upgrades](upgrades.md)). `policystack` ignores the lifecycle keys.

!!! warning "Quote numbers and booleans"
    Argo CD reads fleet files as YAML 1.1. `revision: 1.10` pins `1.1`. Write `"1.10"`, `"20261003"`, `"on"`.

Fleet files are read only from the default branch (`HEAD`). `fleet/` on other branches does nothing. Changing the default branch re-points every cluster, and a cluster with no file there loses its Applications.

Changes land within about 6 minutes (3 minute generator requeue plus 3 minute branch cache). An [ApplicationSet webhook](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Git/#webhook-configuration) makes them immediate, except a fix for a failed generation, which waits for the 3 minute retry.

Upstream ships no live fleet files. Examples are the validator fixtures in [`tools/validator/testdata/clusters/`](https://github.com/PolicyStack/PolicyStack/tree/main/tools/validator/testdata/clusters).

??? info "Migrating from `config.<baseDomain>` labels"
    `config.<baseDomain>/<category>.<priority>: <value>` becomes `<category>s/<value>.yaml`, in priority order.

    ```yaml
    # config.example.com/environment.1: prod
    # config.example.com/datacenter.2: dc1
    valueFiles:
      - environments/prod.yaml
      - datacenters/dc1.yaml
    ```

PolicyStack reads five labels. You only ever set the last one.

| Label | Set by | Effect |
|---|---|---|
| `clusterID` | ACM | Selects the cluster's Argo CD secret, becomes `selectedId`. No label, no Applications. |
| `local-cluster` | ACM, on the hub | Switches to the hub file ([The hub](#the-hub)). |
| `name` | ACM | Placement selector matches it ([Placement](policies.md#placement)). |
| `apps.open-cluster-management.io/cluster-name` | ACM GitOpsCluster | Matches `hub` to a default-mode managed hub ([Lifecycle generator](#lifecycle-generator)). |
| `global-hub.open-cluster-management.io/deploy-mode` | You, on Global Hub import | `default` switches a managed hub to `fleet/hubs/<cluster>.yaml` ([Global Hub](global-hub.md)). |

### Revision

A branch (slashes work), tag or commit SHA. The [Render Diff](workflows.md#render-diff) PR comment shows each cluster's Policies at its `revision` before and after merge.

!!! danger "One bad fleet file freezes this hub's fleet"
    If a fleet file this hub reads fails to parse, lacks `revision`, or pins a missing revision, `policystack` generation fails and no Application for any cluster is created, updated or deleted until it is fixed. A parse error stops `policystack-lifecycle` the same way. A bad `revision` there breaks only `lifecycle-<cluster>`.

```sh
oc get applicationset policystack -n openshift-gitops \
  -o jsonpath='{.status.conditions[?(@.type=="ErrorOccurred")].message}'
```

Pin tags, SHAs or protected branches. A deleted pinned branch freezes the fleet, and GitHub deletes PR branches on merge when automatic head branch deletion is on.

### The hub

The hub's ManagedCluster has `local-cluster`, so it reads `fleet/hubs/<hubName>.yaml`. `hubName` is set once at `helm install`, no default ([Install PolicyStack](install.md#install-policystack)).

```yaml
# fleet/hubs/acm-dc1.yaml
revision: main
valueFiles:
  - environments/prod.yaml
  - datacenters/dc1.yaml
  - platforms/baremetal.yaml
  - acm/acm-dc1.yaml # (1)!
```

1.  Hubs in one datacenter share a layer by listing it.

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
    Nothing on a hub is stable and unique. Every hub's ManagedCluster defaults to `local-cluster`. ACM's `localClusterName` (2.14+) changes only at install. `clusterID` changes on rebuild. `hubs/` keeps hub files from colliding with a managed cluster of the same name.

A Global Hub managed hub imported in default mode has no `local-cluster`; the global hub reads its file instead ([Global Hub](global-hub.md#layout)).

### Protection

Other labels are ignored, and the five above are guarded by RBAC on ManagedClusters and `openshift-gitops` Secrets, so `oc label` cannot change what a cluster runs. Whoever merges to `fleet/` decides, including destroying clusters with `state: absent`.

Add a CODEOWNERS entry and a default branch ruleset requiring code owner review and blocking force push and deletion:

```text
# .github/CODEOWNERS
/fleet/ @<org>/<team>
```

To gate only production, own `/fleet/prod-*.yaml` and `/fleet/hubs/` instead.

## Generators

`policystack`, rendered with `hubName=acm-dc1`:

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

An element added on a branch reaches only clusters pinned to a revision containing it. Disabled elements still get an Application that renders nothing.

### Lifecycle generator

`policystack-lifecycle`, rendered with `hubName=acm-dc1`:

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
3.  Fleet files naming a managed hub imported in default mode ([Global Hub](global-hub.md#lifecycle)). Fleet files come first because Argo CD templates a child generator's fields but not its `selector`.
4.  A `hub` that is not a DNS label matches nothing instead of failing the whole ApplicationSet.

Files without `hub` match neither, so imported clusters get no lifecycle Application. The cluster secret's `.name` is the ManagedCluster the Policies are placed on.

## Applications

| Field | `policystack` | `policystack-lifecycle` |
|---|---|---|
| Name | `<element>-<cluster>`, `<element>-<hubName>` on the hub | `lifecycle-<cluster>` |
| Source | `stack/<element>` at the cluster's revision | `lifecycle/` at the fleet file's revision |
| Values | [Cascade](values.md#order) | Same |
| Destination | Hub, `open-cluster-management` | Same |
| Sync | Automated, `prune`, `selfHeal`, `allowEmpty`, `RespectIgnoreDifferences=true` | Automated, `prune`, `selfHeal` |

The Application name is the Helm release name, which every object name derives from ([Naming limits](policies.md#naming-limits)). Argo CD deploys only to the hub; ACM delivers policies to clusters ([Placement](policies.md#placement)). Objects land in `policyNamespace`; the destination namespace is unused. `allowEmpty` lets a disabled element prune to zero objects ([Disabling an element](rollout.md#disabling-an-element)).

Both ApplicationSets set `preserveResourcesOnDeletion: false`: deleting an Application deletes its hub objects ([Removing an element](rollout.md#removing-an-element)). Deleting a lifecycle Application leaves the cluster running unmanaged ([Abandon](lifecycle.md#abandon)).

!!! note "Deletion is bounded"
    A failed render prunes nothing (`ComparisonError`). A failed generation deletes nothing. Argo CD only deletes hub objects; ACM then removes policies from the cluster ([What happens on the managed cluster](rollout.md#what-happens-on-the-managed-cluster)).

From policy-library 3.1.0, `copyPolicyMetadata: false` keeps Argo CD tracking metadata off ACM's per-cluster Policy copies, so Argo CD never prunes them. Clusters pinned to an older chart still copy it.

### Render cache

Each Application lists what its render reads in [`manifest-generate-paths`](https://argo-cd.readthedocs.io/en/stable/operator-manual/high_availability/#manifest-paths-annotation). For `compliance-prod-east-1`:

```yaml
argocd.argoproj.io/manifest-generate-paths: '.;../../values.yaml;../../values/environments/prod.yaml;../../values/datacenters/dc1.yaml;../../values/platforms/aws.yaml;../../values/clusters/prod-east-1.yaml'
```

A new commit re-renders only Applications whose listed paths changed. Fleet file changes alter the spec and always re-render. Without a webhook this needs Argo CD 2.11 (OpenShift GitOps 1.13)+; older versions re-render every Application on every commit.

## Injected values

Passed in `valuesObject`, which ranks above every values file ([Order](values.md#order)). For `prod-east-1`:

```yaml
channel: "stable-4.20" # (1)!
selectedName: prod-east-1 # (2)!
selectedId: <clusterID>
selector:
  matchExpressions:
    name: # (3)!
      key: name
      operator: In
      values:
        - "prod-east-1"
```

1.  `version` and `channel`, only when the fleet file sets them.
2.  `hubName` on the hub.
3.  policy-library turns this into each Placement's selector. On the hub it holds the ManagedCluster name, not `hubName`.

Nothing else is injected. An element that needs the environment reads a key set in `environments/<env>.yaml`.

A lifecycle Application passes the fleet file's keys instead, so the fleet file's `install` overrides the cascade ([Values](lifecycle.md#values)):

```yaml
hub: "acm-dc1"
state: "present"   # default
version: "4.20.8"  # channel too, when set
install: {}        # default
selector:
  matchExpressions:
    hub: # (1)!
      key: name
      operator: In
      values:
        - "local-cluster"
```

1.  The ManagedCluster of the hub running the lifecycle Policies.

## The install chart

The [install chart](https://github.com/PolicyStack/PolicyStack/tree/main/install) installs both ApplicationSets, the GitOpsCluster and RBAC ([Install PolicyStack](install.md#install-policystack)).

| Value | Set in | Use |
|---|---|---|
| `gitRepo` | `--set` | Repo everything reads. Required. |
| `hubName` | `--set` | This hub's identity ([The hub](#the-hub)). Required. |
| `policyNamespace` | `install/values.yaml` | Must match the root `values.yaml` (CI checks). Do not change. |

Upgrade it as in [Upgrading](install.md#upgrading). Preview the render:

```sh
helm template policystack ./install --set hubName=acm-dc1 --set gitRepo=https://github.com/PolicyStack/PolicyStack.git
```
