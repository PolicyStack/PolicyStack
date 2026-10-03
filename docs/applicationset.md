# Clusters and Applications

The ApplicationSet `policystack` in `openshift-gitops` on the hub creates one Argo CD Application per element for every managed cluster that has a fleet file. The fleet file picks the Git revision and the values files. The template is [`appset/templates/appset.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/appset/templates/appset.yaml).

## Fleet files

A fleet file declares one cluster. A managed cluster's file is `fleet/<cluster>.yaml`, named exactly like its ManagedCluster. The hub's file is `fleet/hubs/<hubName>.yaml` ([The hub](#the-hub)).

| Key | Required | Effect |
|---|---|---|
| `revision` | Yes | Branch, tag or commit SHA every Application for the cluster renders from ([Revision](#revision)). |
| `config` | No. The hub needs a `datacenter` entry | A map of `<category>.<priority>: <value>`. Each entry adds `values/<category>s/<value>.yaml` to the cascade. The priority orders the files ([Order](values.md#order)). On the hub the `datacenter` entry also names the Applications and the hub values files. |

No other keys are allowed. Config values follow label value syntax: at most 63 characters of alphanumerics, `-`, `_` and `.`, starting and ending with an alphanumeric. A category is alphanumerics, `-` and `_`.

Argo CD reads fleet files as YAML 1.1, so `revision` and every config value must be YAML strings. Quote anything YAML reads as a number or a boolean: `"1.10"`, `"20261003"`, `"on"`, `"yes"`. Unquoted, `revision: 1.10` pins `1.1`, and `platform.30: 1.10` loads `values/platforms/1.1.yaml`. The validator checks these rules ([Validation](validation.md)).

```yaml
# fleet/prod-east-1.yaml
revision: main
config:
  environment.10: prod
  datacenter.20: dc1
  platform.30: aws
```

Categories are dynamic. `tenant.40: payments` adds `values/tenants/payments.yaml`. The repo's priorities are environment `.10`, datacenter `.20`, platform `.30` and tenant `.40`. The config keys are the old `config.<baseDomain>/` label keys without the prefix ([Moving from cluster labels](rollout.md#moving-from-cluster-labels)).

Upstream ships no live fleet files, since a shipped file would configure a real cluster as soon as someone installs from upstream. The examples are the validator's fixtures in [`tools/validator/testdata/clusters/`](https://github.com/PolicyStack/PolicyStack/tree/main/tools/validator/testdata/clusters): `prod-east-1.yaml`, `nonprod-west-1.yaml` and `hubs/acm-dc1.yaml`.

The ApplicationSet reads fleet files only from the repo's default branch, as `HEAD`. Copies of `fleet/` on other branches have no effect. Changing the default branch re-points every cluster to the fleet files on the new branch, and a cluster without one there loses its Applications.

Fleet changes reach Argo CD within about 6 minutes: the ApplicationSet controller re-runs its generators every 3 minutes, and Argo CD caches the commit a branch points to for 3 minutes. An [ApplicationSet webhook](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Git/#webhook-configuration) makes changes immediate, except the fix for a failed generation, which still waits for the 3 minute retry.

PolicyStack still reads three labels. ACM sets all of them, and nobody sets them by hand:

| Label | Set by | Effect |
|---|---|---|
| `clusterID` | ACM, on OpenShift clusters | Selects the cluster's Argo CD secret and becomes `selectedId`. A cluster without it gets no Applications. |
| `local-cluster` | ACM, on the hub | Switches the cluster to the hub's fleet file, hub naming and hub values files. |
| `name` | ACM | The ManagedCluster name. The Placement selector matches on it ([Placement](policies.md#placement)). |

### Revision

`revision` is a branch, a tag or a commit SHA. Branch names with `/` work.

If a fleet file this hub reads does not parse, has no `revision`, or pins a revision that does not exist, ApplicationSet generation fails. Argo CD then creates, updates and deletes no Applications for any cluster until the file is fixed. The ApplicationSet's `ErrorOccurred` condition holds the error:

```sh
oc get applicationset policystack -n openshift-gitops \
  -o jsonpath='{.status.conditions[?(@.type=="ErrorOccurred")].message}'
```

Pin tags, commit SHAs, protected branches, or a branch you keep until its pin moves off. A pinned branch that is deleted freezes the whole fleet, and GitHub deletes a pull request's branch on merge when automatic deletion of head branches is on.

The generator reads `HEAD` rather than `main`, so it follows a rename of the default branch.

### The hub

The hub's own ManagedCluster carries ACM's `local-cluster` label. Its fleet file is `fleet/hubs/<hubName>.yaml`, where `hubName` is an appset chart value set once per hub, `acm-dc1` by default ([The appset chart](#the-appset-chart)).

```yaml
# fleet/hubs/acm-dc1.yaml
revision: main
config:
  environment.10: prod
  datacenter.20: dc1
  platform.30: baremetal
```

`hubName` is a chart value because nothing on the hub gives it a stable, unique name:

- Every hub's own ManagedCluster is named `local-cluster` by default, so the cluster name cannot tell two hubs reading this repo apart.
- The datacenter that names the hub Applications is inside the file, so it cannot be used to find the file.
- ACM's `localClusterName` (ACM 2.14 and later) changes only at install, or after turning off hub self-management.
- OpenShift's `clusterID` is a UUID that changes when a hub is rebuilt.

So each hub's identity is set once in its appset release, and its configuration stays in Git. `helm get values appset` shows a hub's `hubName`. `hubs/` is a subdirectory so a hub's file cannot collide with a managed cluster of the same name.

A second hub reading the same repo, including a passive or disaster recovery hub, needs its own `--set hubName=<name>` on every `helm install` and `helm upgrade` ([Install the ApplicationSet](install.md#install-the-applicationset)). `helm upgrade` drops `--set` values it is not given again, so the hub falls back to the default and applies the first hub's configuration to itself.

The hub has two names:

| Name | Comes from | Used for |
|---|---|---|
| `hubName` | The appset chart | The path of the hub's fleet file |
| `acm-<datacenter>` | The `datacenter` entry in the hub's fleet file | Application names, `values/acm/acm-<datacenter>.yaml`, `values/clusters/acm-<datacenter>.yaml` and `selectedName` |

With one hub per datacenter, set `hubName` to `acm-<datacenter>` so both names match. Hubs in the same datacenter need different `hubName` values but share hub values files.

### Protection

PolicyStack reads no labels people set by hand. Short of editing the three ACM labels above, which RBAC on ManagedClusters guards, `oc label` cannot change what a cluster runs. Whoever can merge to `fleet/` on the default branch decides it instead. The repo ships no CODEOWNERS file, because owners differ per fork. Add:

- A `CODEOWNERS` entry for `/fleet/`.
- A ruleset on the default branch that requires a pull request with code owner review, and blocks force pushes and deletion.

```text
# .github/CODEOWNERS
/fleet/ @<org>/<team>
```

To require code owner review only for production clusters, own `/fleet/prod-*.yaml` and `/fleet/hubs/` instead of `/fleet/`.

## Generators

The generator, as rendered with the default `hubName`:

```yaml
generators:
  - matrix:
      generators:
        - matrix:
            generators:
              - clusters:
                  selector:
                    matchExpressions:
                      - key: clusterID
                        operator: Exists
              - git:
                  repoURL: 'https://github.com/PolicyStack/PolicyStack.git'
                  revision: HEAD
                  pathParamPrefix: fleet
                  files:
                  - path: 'fleet/{{if index .metadata.labels "local-cluster"}}hubs/acm-dc1{{else}}{{.name}}{{end}}.yaml'
        - git:
            repoURL: 'https://github.com/PolicyStack/PolicyStack.git'
            revision: '{{.revision}}'
            directories:
            - path: 'stack/*'
```

1. A clusters generator. Every Argo CD cluster secret with a `clusterID` label.
2. A git files generator. The cluster's fleet file at `HEAD`, `fleet/hubs/<hubName>.yaml` on a cluster labeled `local-cluster` and `fleet/<cluster>.yaml` otherwise. The file's keys become parameters. `pathParamPrefix: fleet` moves the file's path parameters under `fleet`, because a matrix keeps the first child's value when two children set the same parameter, and the file's `path` would otherwise replace the directory's.
3. A git directories generator. Every directory under `stack/` in `gitRepo`, at the fleet file's `revision`.

The inner matrix pairs each cluster with its fleet file. A cluster without one pairs with nothing. The outer matrix pairs each cluster with the directories, and each cluster and directory pair becomes one Application. The directory list comes from the cluster's own revision, so an element added on one branch reaches only clusters on a revision that contains it. Every directory gets an Application whether the element is enabled or not. A disabled element renders no objects.

The GitOpsCluster from [Import clusters into Argo CD](install.md#import-clusters-into-argo-cd) creates one cluster secret per managed cluster. Clusters without a fleet file are imported but get no Applications.

## Applications

| Field | Value |
|---|---|
| Name | `<element>-<cluster>`, or `<element>-acm-<datacenter>` on a cluster labeled `local-cluster` |
| Project | `default` |
| Source | `stack/<element>` in `gitRepo` at the cluster's revision, rendered by Helm with the [values cascade](values.md#order) |
| Destination | The hub, `https://kubernetes.default.svc`, namespace `open-cluster-management` |
| Sync | Automated with `prune` and `selfHeal`, sync option `RespectIgnoreDifferences=true` |

`<element>` is the directory name and `<cluster>` the ManagedCluster name. Argo CD uses the Application name as the Helm release name, and every rendered object name derives from it ([Rendered objects](policies.md#rendered-objects), [Naming limits](policies.md#naming-limits)).

Argo CD deploys only to the hub. ACM delivers the policies to the managed clusters ([Placement](policies.md#placement)). The chart sets `policyNamespace` on every object it renders, so the destination namespace is unused.

Automated sync leaves `allowEmpty` unset, so it never prunes an Application down to no objects ([Disabling an element](rollout.md#disabling-an-element)).

The ApplicationSet sets `preserveResourcesOnDeletion: true`, so its Applications carry no resources finalizer and deleting one leaves its objects on the hub ([Removing an element](rollout.md#removing-an-element)).

## Injected values

Each Application passes these keys to Helm in `valuesObject`, built from the cluster's fleet file and its ACM labels. [Order](values.md#order) gives their precedence over the values files.

| Key | Value |
|---|---|
| `selected<Category>` | The value of the fleet file's config entry in that category, for example `selectedEnvironment`. When a category has several entries, the one that sorts last in [Order](values.md#order) wins. |
| `selected<Category>Values` | Every value of that category, as a list. |
| `selectedName` | The cluster name, or `acm-<datacenter>` on the hub. |
| `selectedId` | The `clusterID` label. |
| `selector.matchExpressions` | A map with one `In` expression on the `name` label, under the key `name`. |

Elements can read every key. The policy-library chart turns `selector` into each Placement's cluster selector ([Placement](policies.md#placement)). On the hub, `selectedName` is `acm-<datacenter>`, but the `name` expression holds the hub's ManagedCluster name. For `prod-east-1`:

```yaml
selectedDatacenter: dc1
selectedEnvironment: prod
selectedPlatform: aws
selectedDatacenterValues: [dc1]
selectedEnvironmentValues: [prod]
selectedPlatformValues: [aws]
selectedName: prod-east-1
selectedId: <clusterID>
selector:
  matchExpressions:
    name:
      key: name
      operator: In
      values:
        - "prod-east-1"
```

## The appset chart

The [appset chart](https://github.com/PolicyStack/PolicyStack/tree/main/appset) installs the ApplicationSet and reads these values:

| Value | Set in | Use |
|---|---|---|
| `gitRepo` | `appset/values.yaml` | The repository the git generators read and every Application renders from. |
| `hubName` | `appset/values.yaml`, or `--set` on each additional hub | Names this hub's fleet file, `fleet/hubs/<hubName>.yaml`. Default `acm-dc1` ([The hub](#the-hub)). |
| `policyNamespace` | Root `values.yaml` | The namespace the chart creates. Elements read the same value. Do not change it. |

The install command passes both values files ([Install the ApplicationSet](install.md#install-the-applicationset)). The chart also binds the `global` ManagedClusterSet into `policyNamespace`, so element Placements can select clusters ([Placement](policies.md#placement)).

[`appset/appset-noformatting.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/appset/appset-noformatting.yaml) is the ApplicationSet as `helm template` renders it with the default values, without the Helm escaping around Argo CD's template expressions. It sits outside `templates/`, so Helm never installs it.
