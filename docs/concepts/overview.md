# How PolicyStack works

The rest of the docs assume you know what is in your repository, what PolicyStack creates on the hub, how a commit reaches a cluster, how fleet files work, what your repository's CI checks, and the terms below. Read this page before Installation.

## The repository

You create your own repository from upstream PolicyStack with the launcher (`mise run ps`) and pass its URL as `gitRepo` when you install the chart on the hub ([Installation](../start/install.md)). Upstream ships no fleet files, so it configures no cluster until you add yours. Several hubs can read one repository; each acts only on the fleet files of clusters it manages or builds, plus its own `fleet/hubs/<hubName>.yaml` ([Several hubs, one repo](architecture.md#several-hubs-one-repo)).

The launcher copies two kinds of things from upstream release tags with copier, each recorded in a `.copier-answers.yml`: the framework, once, at the repository root, and each element you want, into `stack/<element>/`. Everything else is yours. Each path belongs to one owner:

| Path | What it is | Owner | Updated by |
|---|---|---|---|
| `install/` | The Helm chart you run once per hub. It creates the two ApplicationSets, a GitOpsCluster that registers every ACM managed cluster with Argo CD (so the ApplicationSets can list them), the `pstack` namespace and RBAC ([Install chart](../reference/install.md)). `install/operators/` renders ACM, OpenShift GitOps and a MultiClusterHub for a hub that doesn't run them yet. | Framework | A framework update. Don't edit. |
| `lifecycle/` | The chart that builds and destroys clusters ([Cluster lifecycle](lifecycle.md)). | Framework | Same |
| `lib/` | policy-library, the Helm library chart every element and `lifecycle/` renders through ([How an element is built](elements.md)). | Framework | Same |
| `tools/` | The validator, the launcher (`tools/policystack`), the sample element, `create-element.sh` and the README generator. | Framework | Same |
| `.github/workflows/`, `.pre-commit-config.yaml`, `mise.toml` | CI workflows, lint hooks, pinned tool versions and the `ps`, `update` and `validate` tasks ([CI workflows](../reference/ci.md), [Launcher](../reference/launcher.md)). | Framework | Same |
| `upgrades/` | Present once a release ships one: upgrade steps and blocked releases. The launcher reads them from upstream, not from your copy ([Upgrade steps](../reference/launcher.md#upgrade-steps)). | Framework | Same |
| `/.copier-answers.yml` | The framework release you have, such as `_commit: v1.2.0`. | copier | A framework update. Never edit by hand. |
| `stack/<element>/` with a `.copier-answers.yml` | A tracked element, copied from a release tag `vX.Y.Z` no newer than the framework's. | Upstream, until you edit it | An element update, which merges upstream's changes with your edits. |
| `stack/<element>/` without one | An element you wrote, or one you ejected from tracking. | You | You |
| `fleet/` | One fleet file per cluster. | You | You, and upgrade steps on an update branch |
| `values/` | Values layers, such as `environments/prod.yaml` or `clusters/<cluster>.yaml`. | You | You, and upgrade steps on an update branch |
| `values.yaml` | Values every cluster gets. It holds only `policyNamespace: pstack`, the namespace the install chart creates. Don't edit it: changing that key renames, so deletes and recreates, every Policy ([Where Policies live](policies.md#where-policies-live)), and fleet-wide settings go in a layer ([Order](values.md#order)). | Framework | A framework update |
| `.github/CODEOWNERS`, `README.md` | Reviewers, your own notes. | You | You |

You take updates by hand: `mise run update` merges a newer release of the framework and the elements you pick into a branch, you open it as a PR, and the merge reaches clusters like any other ([Upgrading PolicyStack](../guides/upgrading.md)). Upstream promises compatibility only for the framework and for elements you haven't edited ([Compatibility](../reference/compatibility.md)). Settings belong in `values/`: an element you edit is yours to merge on every update.

Every element ships with `enabled: false` except `upgrade`, which renders nothing until a fleet file sets `version` or `channel`, or a layer sets `stack.upgrade.config.upstream`. Then it enforces them on merge: `version` starts an OpenShift upgrade ([OpenShift upgrades](../guides/upgrades.md)). A cluster gets nothing from an element until a values layer it reads sets `stack.<elementKey>.enabled: true`, where the key is the element name in camelCase: `cert-manager` reads `stack.certManager` ([How an element is built](elements.md)).

A layer that turns elements on enforces them as soon as the PR that lists the layer merges. Read a layer before listing it, or start with a [dry run](../guides/rollout.md#dry-run). A cluster always reads the `values/clusters/` file matching its name, without listing it.

## What lives on the hub

Every object Argo CD creates is on the hub. Only ACM changes managed clusters, through the Policies.

| Object | Where | One per | Created by |
|---|---|---|---|
| ApplicationSets `policystack` and `policystack-lifecycle` | `openshift-gitops` | Hub | `helm install policystack ./install` |
| GitOpsCluster `argo-acm-importer-global`, its Placement `global`, and its Argo CD cluster secrets | `openshift-gitops` | Secret per ManagedCluster, the hub included | The install chart, then ACM. The secrets only let the ApplicationSets list clusters ([ApplicationSets](architecture.md)). |
| ManagedClusterSetBinding `global` | `openshift-gitops`, `pstack` | Namespace | The install chart. Lets Placements there select clusters; without it they select nothing ([Cluster sets](policies.md#cluster-sets)). |
| Namespace `pstack`, ClusterRole and ClusterRoleBinding `openshift-gitops-policy-admin` | Cluster | Hub | The install chart. Lets Argo CD manage Policies, Placements and PlacementBindings. |
| Application `<element>-<cluster>` | `openshift-gitops` | Element directory per cluster with a fleet file, enabled or not. A disabled element's Application renders nothing. | `policystack` |
| Application `lifecycle-<cluster>` | `openshift-gitops` | Cluster this hub builds | `policystack-lifecycle` ([Cluster lifecycle](lifecycle.md)) |
| Policies, Placement, PlacementBinding | `pstack` | One Policy per policy the element defines: one per `converters/<policy>/` directory, plus `install` when it manages operators ([How an element is built](elements.md)). One Placement per enabled element per cluster; one PlacementBinding when at least one Policy renders | The Applications |
| Replicated Policies `pstack.<policy name>` | The cluster's namespace, named like the cluster | Policy per cluster it targets | ACM ([ACM policies in brief](acm.md)) |

For example, with the `gitops` element enabled for the managed cluster `prod-east-1`:

```text
Application   openshift-gitops/gitops-prod-east-1
├─ Placement         pstack/gitops-prod-east-1           selects prod-east-1 only
├─ PlacementBinding  pstack/gitops-prod-east-1
├─ Policy            pstack/install-gitops-prod-east-1   → prod-east-1/pstack.install-gitops-prod-east-1
├─ Policy            pstack/instance-gitops-prod-east-1  → prod-east-1/pstack.instance-gitops-prod-east-1
└─ Policy            pstack/link-gitops-prod-east-1      → prod-east-1/pstack.link-gitops-prod-east-1
```

`install` comes from the element's `operatorPolicies`, `instance` and `link` from its `converters/instance/` and `converters/link/` directories.

The Application name is also the Helm release name, and Policy names are `<policy>-<release>` ([Rendered names](elements.md#rendered-names)). On the hub itself, `hubName`, the name you give the hub at `helm install`, takes the cluster's place in the Application name, and the replicated Policies sit in the hub's ManagedCluster namespace, `local-cluster` by default ([The hub](architecture.md#the-hub)).

## How a change reaches a cluster

1. You merge a commit. A cluster whose fleet file pins a branch, such as `revision: main`, takes every commit to that branch. A cluster pinned to a tag or commit SHA takes nothing until its fleet file's `revision` changes.
2. For each Application whose inputs changed, Argo CD runs `helm template` on `stack/<element>` at the cluster's revision, with the cluster's values cascade: the element's `values.yaml`, the root `values.yaml`, each `valueFiles` entry, then `values/clusters/<cluster>.yaml` ([Values cascade](values.md#order)). It syncs the result into `pstack` and deletes the objects that dropped out of the render.
3. The Placement selects exactly one ManagedCluster, and ACM replicates each Policy into that cluster's namespace on the hub ([Placement and naming](policies.md)).
4. The governance add-on, ACM's policy controllers on the managed cluster, pulls the replicated Policy. Under `enforce` it creates or patches objects to match the Policy; under `inform` it only reports differences. The status goes back to the replicated Policy and the Policy in `pstack` ([ACM policies in brief](acm.md)).

=== "Branch pin"

    ```yaml title="fleet/prod-east-1.yaml"
    revision: main
    ```

    Merge a change to `values/clusters/prod-east-1.yaml` on `main`. It reaches the cluster within about 6 minutes ([Default branch only](#default-branch-only)).

=== "Tag pin"

    ```yaml title="fleet/prod-east-1.yaml"
    revision: v1.4.0
    ```

    Merge the change to `main`, tag it `v1.5.0`, then merge a second PR that sets `revision: v1.5.0`. Until that second merge, the cluster keeps rendering `v1.4.0`. Production clusters pinned this way change only through a reviewed edit under `fleet/` ([Rolling out changes](../guides/rollout.md#promoting-and-rolling-back)).

When a change doesn't arrive, check each stage in order:

| Stage | Check | Usual cause and effect |
|---|---|---|
| Generation | `oc get applicationset policystack -n openshift-gitops -o jsonpath='{.status.conditions[?(@.type=="ErrorOccurred")].message}'` | A bad fleet file stops generation for the whole hub ([One bad fleet file stops the hub](#one-bad-fleet-file-stops-the-hub)). |
| Application exists | `oc get applications.argoproj.io -n openshift-gitops \| grep <cluster>` | None: no fleet file named exactly like the ManagedCluster at `fleet/<cluster>.yaml` on the default branch (subdirectories and other branches are ignored), or the cluster's Argo CD secret lacks the `clusterID` label (`oc get secret -n openshift-gitops -l argocd.argoproj.io/secret-type=cluster -L clusterID`). ACM adds the label once the cluster has joined. |
| Render | `oc get application <element>-<cluster> -n openshift-gitops -o jsonpath='{.status.conditions[*].message}'` | A `ComparisonError` with the Helm error: a key the library schema rejects (an unknown key directly under `stack.<elementKey>`), a wrong type, a values entry that names no policy. Argo CD syncs and deletes nothing for that Application, so the cluster keeps its last good Policies. Other Applications are unaffected, except that a schema error under `stack` in a shared layer fails every Application, `lifecycle-<cluster>` included, of every cluster reading that layer ([When an Application fails](architecture.md#when-an-application-fails)). |
| Render is empty | `oc get policies.policy.open-cluster-management.io -n pstack \| grep <element>-<cluster>` (`<element>-<hubName>` for the hub) | No Policies: no layer the cluster reads enables the element, or a values file name is misspelled, which Argo CD skips silently ([Missing files](values.md#missing-files)). |
| Compliance | `oc get policies.policy.open-cluster-management.io -n <cluster>` (`local-cluster` for the hub) | Under `inform`, `NonCompliant` means the cluster differs and nothing will change it. Under `enforce`, `oc describe policies.policy.open-cluster-management.io pstack.<policy> -n <cluster>` shows each template's latest message ([Compliance states](acm.md#compliance-states)). |

A misspelled element key, or a misspelled top-level key under `config`, renders without error and is ignored; only POLICY035 in CI catches it. Nothing catches a misspelled nested `config` key ([Schema](values.md#schema)).

## Fleet files

A fleet file enrolls one cluster. `fleet/<cluster>.yaml` is named exactly like the cluster's ManagedCluster. A hub's own ManagedCluster is named `local-cluster` by default, so its name can't tell hubs apart. A hub, recognized by its `local-cluster` label, uses `fleet/hubs/<hubName>.yaml` instead, kept in its own directory so it can't collide with a managed cluster's file.

```yaml title="fleet/prod-east-1.yaml"
revision: main # (1)!
valueFiles: # (2)!
  - environments/prod.yaml
  - datacenters/dc1.yaml
  - platforms/aws.yaml
```

1.  The branch, tag or commit SHA every Application of this cluster renders `stack/` and `values/` from.
2.  Values layers under `values/`, lowest precedence first. `values/clusters/prod-east-1.yaml` is always added last.

The file decides three things: whether the cluster gets Applications at all, the revision it renders from, and the layers it reads. Optional keys set the OpenShift version and channel, or have a hub build the cluster ([Fleet file keys](../reference/fleet.md#keys)).

A fleet file does not import a cluster. The cluster must also exist in ACM, imported by you or built by a hub, and have joined. Without `hub: <hubName>`, which asks that hub to build the cluster ([Cluster lifecycle](lifecycle.md)), a fleet file for a cluster ACM doesn't manage does nothing. Once the cluster joins, its Applications appear and its Policies apply without any further commit, so a fleet file written ahead of an import takes effect at import.

### Default branch only

The ApplicationSets read fleet files from the latest commit of the default branch (Argo CD's `HEAD`). Everything else, `stack/` and `values/`, is read at the revision the file names. The revision is a key inside the file, so the file has to come from one fixed branch. It also means only merges to the default branch decide which revision each cluster renders: a commit on another branch can't re-pin a cluster. A cluster pinned to a branch still takes every commit to that branch.

What follows from that:

- `fleet/` on any other branch does nothing.
- Subdirectories other than `fleet/hubs/` are ignored. `fleet/prod/prod-east-1.yaml` enrolls nothing.
- Changing the repository's default branch re-points every cluster. A cluster with no file on the new default branch loses its Applications.
- Deleting a fleet file deletes the cluster's Applications and with them its Policies on the hub. ACM then removes the policies from the cluster; what they created stays unless the template sets `pruneObjectBehavior` ([Removal and pruning](removal.md)). A cluster a hub built is abandoned, not destroyed: it keeps running, unmanaged ([Cluster lifecycle](lifecycle.md)).

A fleet file change lands within about 6 minutes: the ApplicationSet re-reads Git every 3 minutes, and Argo CD remembers which commit a branch points to for 3 minutes. An [ApplicationSet webhook](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Git/#webhook-configuration) makes it immediate, except the fix for a failed generation, which waits for the 3-minute retry.

### One bad fleet file stops the hub

!!! danger "A broken fleet file freezes generation"
    `policystack` reads the fleet files of the clusters registered on this hub. If one of them fails to parse, lacks `revision`, or pins a revision that doesn't exist, generation fails: no Application for any cluster on this hub is created, updated or deleted until it is fixed. Existing Applications and Policies stay as they are.

    `policystack-lifecycle` parses every `fleet/*.yaml` (not `fleet/hubs/`) before it filters by hub, so one unparseable fleet file stops lifecycle on every hub reading the repository, and a fleet file naming this hub that lacks `revision` stops it on this hub. A `revision` that doesn't exist breaks only that cluster's `lifecycle-<cluster>` render.

Your repository's CI rejects these before merge ([Your repository's CI](#your-repositorys-ci)). It can't catch a pinned branch deleted after merge, so pin tags, commit SHAs or protected branches. Fleet keys, quoting rules and revision forms are in [Fleet files](../reference/fleet.md).

## Your repository's CI

On every pull request, the CI workflow runs Policy Validate, the [validator](../develop/validation.md), which renders every element the way the ApplicationSet does and checks the result ([Validator rules](../reference/rules.md), [CI workflows](../reference/ci.md)).

| Check | Catches | Blocks merge |
|---|---|---|
| `ci` | Policy Validate: invalid fleet files (POLICY050), pins that don't resolve, lack a file the fleet file uses or are on a framework major the hubs can't render (POLICY052), values keys nothing reads (POLICY035), render errors (RENDER000). Also fails if the lint hooks fail. | Once a ruleset requires it |
| `pr-title` | PR titles that aren't Conventional Commits, such as `feat: enable metallb on prod-east-1` | Once a ruleset requires it |
| Render Diff | Nothing; comments each fleet cluster's Policy changes, rendering each cluster at its own revision before and after the merge | Never |

Nothing is required until you add a ruleset: [Protect the repository](../start/install.md#protect-the-repository).

## Words with more than one meaning

=== "hub"

    | Term | Meaning |
    |---|---|
    | The hub | The cluster running ACM, OpenShift GitOps and PolicyStack. ACM also manages it as a cluster, named `local-cluster` by default. |
    | `hubName` | The hub's name in PolicyStack, set once at `helm install`. Picks `fleet/hubs/<hubName>.yaml` and `values/clusters/<hubName>.yaml`, and names the hub's Applications ([The hub](architecture.md#the-hub)). |
    | `hub:` in a fleet file | The `hubName` of the hub that builds and destroys that cluster ([Cluster lifecycle](lifecycle.md)). Leave it out for a cluster you import. |
    | `fleet/hubs/` | Hub fleet files ([Fleet files](#fleet-files)). |

=== "install"

    | Term | Meaning |
    |---|---|
    | `install/` chart | `helm install policystack ./install`, run once per hub ([Install chart](../reference/install.md)). |
    | `install/operators` | Renders ACM and OpenShift GitOps for a new hub ([Installation](../start/install.md)). |
    | The `install` Policy | The Policy an element gets when it manages operators, such as `install-gitops-prod-east-1` ([How an element is built](elements.md)). |
    | `install:` in a fleet file | The cluster a hub builds: engine, platform, nodes ([Lifecycle keys](../reference/lifecycle.md#install-keys)). |

=== "compliance"

    | Term | Meaning |
    |---|---|
    | Compliance state | ACM's status on every Policy: `Compliant`, `NonCompliant` or `Pending` ([ACM policies in brief](acm.md)). |
    | The `compliance` element | Installs the Compliance Operator, runs scans and reports their results as the compliance state of its Policies ([Compliance](../elements/compliance.md)). |

## Glossary

| Term | Meaning |
|---|---|
| Answers file | `.copier-answers.yml`, at the repository root for the framework and in each tracked `stack/<element>/`. Records the upstream release tag copier copied ([The repository](#the-repository)). |
| Application | An Argo CD object that renders one source with Helm and syncs the result. PolicyStack creates `<element>-<cluster>` and `lifecycle-<cluster>`, all with the hub as destination ([ApplicationSets](architecture.md)). |
| ApplicationSet | An Argo CD object that creates Applications from generators, here from cluster secrets, fleet files and `stack/*` directories ([ApplicationSets](architecture.md)). |
| Cluster secret | An Argo CD Secret registering a cluster. The GitOpsCluster creates one per ManagedCluster so the ApplicationSets can list them ([ApplicationSets](architecture.md)). |
| ConfigurationPolicy | A policy template listing Kubernetes objects that must, or must not, exist on the cluster ([ACM policies in brief](acm.md)). |
| Converter | A file `stack/<element>/converters/<policy>/<template>.yaml`. Its directory creates a Policy and the file one ConfigurationPolicy in it; the file's Helm output is the objects it checks or enforces ([How an element is built](elements.md)). |
| Day-2 | Configuring a cluster that already runs, which is what elements do. Building the cluster is lifecycle. A built cluster gets Day-2 config the same way as an imported one ([Day-2 handoff](lifecycle.md#day-2-handoff)). |
| Element | A Helm chart `stack/<element>/` that turns curated settings into the Policies for one product. Configured under `stack.<elementKey>` in values ([How an element is built](elements.md)). |
| `enforce`, `inform` | A Policy's remediation action. `enforce` changes the cluster to match; `inform` only reports. Every shipped element enforces by default ([ACM policies in brief](acm.md)). |
| Engine | How a hub builds a cluster: `hypershift` (hosted control plane on the hub), `hive` (installer on AWS, GCP, Azure or vSphere) or `siteconfig` (bare metal) ([Cluster lifecycle](lifecycle.md)). |
| Fleet file | `fleet/<cluster>.yaml`, or `fleet/hubs/<hubName>.yaml` for the hub, on the default branch. Enrolls the cluster and sets its revision and layers ([Fleet files](#fleet-files)). |
| Framework | Everything in your repository that comes from upstream except the elements: `install/`, `lifecycle/`, `lib/`, `tools/`, the CI workflows. Versioned `vX.Y.Z` ([Compatibility](../reference/compatibility.md)). |
| GitOpsCluster | ACM object that creates an Argo CD cluster secret per ManagedCluster; PolicyStack's is `argo-acm-importer-global` ([ApplicationSets](architecture.md)). |
| Governance add-on | ACM's policy controllers on each managed cluster. They pull its replicated Policies, apply or check them, and report status ([ACM policies in brief](acm.md)). |
| `hubName` | See [Words with more than one meaning](#words-with-more-than-one-meaning). |
| Layer | A values file under `values/` that a cluster reads, either listed in its fleet file's `valueFiles` or its own `values/clusters/<cluster>.yaml`. The root `values.yaml` is read before all layers. Directory names under `values/` are conventions ([Values cascade](values.md)). |
| Lifecycle | A hub building and destroying clusters from fleet files with `hub:`, through Policies that run on the hub ([Cluster lifecycle](lifecycle.md)). |
| ManagedCluster | ACM's record of a cluster. The hub has a namespace of the same name for it, holding its replicated Policies ([ACM policies in brief](acm.md)). |
| Managed cluster | A cluster ACM manages, imported by hand or built by a hub. It gets Applications once it has a fleet file ([First cluster](../start/first-cluster.md)). |
| OperatorPolicy | A policy template that installs and upgrades one OLM operator. An element's `operatorPolicies` create them in its `install` Policy ([ACM policies in brief](acm.md)). |
| Placement, PlacementBinding | ACM objects that choose clusters and bind Policies to them. Every PolicyStack Placement selects exactly one cluster ([Placement and naming](policies.md)). |
| Policy | ACM's unit of compliance on the hub. It wraps one or more policy templates and carries the remediation action ([ACM policies in brief](acm.md)). |
| policy-library | The Helm library chart every element depends on, aliased `stack`, in `lib/policy-library`. It discovers converters and renders the Policies and Placement ([How an element is built](elements.md)). |
| `pstack` | The hub namespace holding every Policy, Placement and PlacementBinding PolicyStack renders (`policyNamespace`) ([Placement and naming](policies.md)). |
| Release | The Application name, also the Helm release name: `<element>-<cluster>`, `<element>-<hubName>` on the hub. Policy, Placement and PlacementBinding names derive from it ([Rendered names](elements.md#rendered-names)). |
| Render | Argo CD running `helm template` for one Application. A failed render shows as `ComparisonError` and changes nothing ([How a change reaches a cluster](#how-a-change-reaches-a-cluster)). |
| Replicated Policy | ACM's copy of a Policy in the target cluster's namespace on the hub, `pstack.<policy name>`. Read a cluster's compliance there ([ACM policies in brief](acm.md)). |
| Revision | The branch, tag or commit SHA a fleet file pins. All of the cluster's Applications render from it ([Fleet files](#fleet-files)). |
| Template | One entry in a Policy: a ConfigurationPolicy or an OperatorPolicy ([ACM policies in brief](acm.md)). |
| Validator | The Go tool in `tools/validator` that renders and checks every element in CI ([Validation](../develop/validation.md)). |
| Values cascade | The ordered files Helm gets for one Application: the element's `values.yaml`, the root `values.yaml`, then every layer. Later files override earlier ones ([Values cascade](values.md)). |
