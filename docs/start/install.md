# Installation

PolicyStack runs on the ACM hub: a Helm release from `install/` points the hub's Argo CD at your repository, and ACM delivers the Policies Argo CD renders, hub objects that describe a cluster's configuration ([ACM policies in brief](../concepts/acm.md), [what the release creates](#install-policystack)).

- An **element** is a Helm chart under `stack/` that turns settings for one product into Policies. Every element ships off.
- A **layer** is a values file under `values/` that turns elements on and configures them.
- A **fleet file**, `fleet/<cluster>.yaml`, enrolls one cluster and lists the layers it reads. The hub is itself a managed cluster, enrolled by `fleet/hubs/<hub>.yaml`.

You create your own repository with the launcher (`mise run ps`), which copies the framework and each element you want from an upstream release tag `vX.Y.Z` with [copier](https://copier.readthedocs.io), and records what it copied. Later, `mise run update` merges a newer release with your changes on a branch, for you to open as a PR ([Upgrading PolicyStack](../guides/upgrading.md)). Full model: [How PolicyStack works](../concepts/overview.md).

1. [Check the prerequisites](#prerequisites)
2. [Create your repository](#create-your-repository)
3. [Add elements](#add-elements)
4. [Protect the repository](#protect-the-repository)
5. [Install the operators](#install-the-operators) (new hubs only)
6. [Install PolicyStack](#install-policystack)
7. [Add the hub's fleet file](#add-the-hubs-fleet-file)
8. [Hand the operators to PolicyStack](#hand-the-operators-to-policystack) (later, once the stack is proven)

Once [Create your repository](#create-your-repository) has made it, run every command from the root of your repository's clone, logged in to the hub as a cluster administrator.

| Placeholder | Meaning | Example |
|---|---|---|
| `<hub>` | The hub's name in PolicyStack (`hubName`). Every hub's own ManagedCluster is called `local-cluster`, so PolicyStack needs a unique name to pick the hub's files ([The hub](../concepts/architecture.md#the-hub)). Lowercase DNS label, at most 30 characters ([Naming limits](../concepts/policies.md#naming-limits)). | `hub-east` |
| `<org>/<repo>` | Your GitHub org and repository | `acme/fleet` |

!!! warning "Choose `hubName` once"
    Changing it deletes and recreates the hub's own Applications and Policies, and abandons the clusters this hub built until their fleet files name the new hub ([The hub](../concepts/architecture.md#the-hub)). Two active hubs with one `hubName` apply the same hub file and build the same clusters.

## Prerequisites

- An empty Git repository, private or public. Argo CD can read any Git server, but the CI checks, CODEOWNERS and rulesets below are GitHub-specific.
- `git` 2.43 or later and [uv](https://docs.astral.sh/uv/getting-started/installation/) 0.13.8 or later on your workstation, to create the repository. uv runs the launcher with Python 3.14, which it downloads if none is installed, and the Python packages `tools/policystack/uv.lock` pins, which it downloads from PyPI ([Launcher requirements](../reference/launcher.md#requirements)).
- [mise](https://mise.jdx.dev), once the repository exists. It installs every tool your repository uses, uv and Python included, at the versions its `mise.toml` pins.
- `oc`, and `helm` 3.14 or later: [upgrades](../guides/upgrading.md#install-chart) use `--reset-then-reuse-values`, added in 3.14. You need 3.17 or later only if the install hits an [ownership conflict](#install-policystack). Your local `helm` runs only the install chart. Argo CD renders elements with the Helm bundled in GitOps (3.18.4 in 1.18), and CI uses the same version.
- Supported floors: ACM 2.16, OpenShift 4.20, OpenShift GitOps 1.18 ([Support](../reference/compatibility.md#support)). Cluster lifecycle has extra requirements ([Cluster lifecycle prerequisites](../guides/lifecycle.md#prerequisites)).
- The hub manages itself: MultiClusterHub self-management on, the ACM default. It creates the hub's ManagedCluster (`local-cluster` by default), which carries the `local-cluster` label. Without it the hub never picks up `fleet/hubs/<hub>.yaml`, gets no Applications and builds no clusters.
- A hub that already runs OpenShift GitOps must use its default Argo CD instance, `openshift-gitops`. The install chart creates its objects in that namespace and grants Policy rights to that instance's application controller; another instance has neither.
- The hub is not a passive DR hub. On a passive DR hub, install PolicyStack only at takeover ([Hub disaster recovery](../guides/lifecycle.md#hub-disaster-recovery)).

```sh
git --version                            # git version 2.43.0 or later
uv --version                             # uv 0.13.8 or later
helm version --short                     # v3.14.0 or later
oc auth can-i '*' '*' --all-namespaces   # yes
oc get managedclusters -l local-cluster=true
# NAME            HUB ACCEPTED   ...   AVAILABLE
# local-cluster   true           ...   True
```

The last command applies to a hub that already runs ACM. A new hub creates `local-cluster` in [Install the operators](#install-the-operators).

## Create your repository

Argo CD reads only your repository. Upstream ships no fleet files and no live configuration.

The launcher ships in every release, under `tools/policystack`. Your repository doesn't exist yet, so fetch just that directory from the newest upstream release and run it with uv:

```sh
url=https://github.com/PolicyStack/PolicyStack.git
tag=$(git ls-remote --tags --refs --sort=-v:refname "$url" 'v*' \
  | sed -n 's|.*refs/tags/\(v[0-9]*\.[0-9]*\.[0-9]*\)$|\1|p' | head -1) # (1)!
git clone -q --depth 1 --filter=blob:none --sparse --branch "$tag" "$url" ~/policystack-"$tag" # (2)!
git -C ~/policystack-"$tag" sparse-checkout set tools/policystack
uv run --frozen --no-dev --project ~/policystack-"$tag"/tools/policystack \
  policystack install ~/<repo> # (3)!
```

1.  The newest release. Prereleases such as `v1.3.0-rc.1` are skipped. The launcher it fetches can install any release; it asks which.
2.  One commit, and only the launcher's files: well under 1 MB. A Git server without partial clone support ignores `--filter` and sends the whole release, about 3 MB; the commands still work.
3.  Not inside `~/policystack-<tag>`: the launcher refuses a directory inside a Git work tree. `--frozen` installs the exact packages `uv.lock` pins; `--no-dev` leaves out the launcher's test tools.

In a terminal, the launcher asks three things. Arrow keys move, space ticks, Enter confirms, and typing filters a list by name:

| Prompt | Lists | Pick |
|---|---|---|
| `Release` | Every release, newest first. A release upstream later found broken is shown with its reason and can't be picked ([Blocked releases](../reference/launcher.md#blocked-releases)) | The newest |
| `Elements` | Every element with its stability. The line under the list shows the highlighted element's description and what it needs. `acm`, `gitops` and `upgrade` are ticked and locked | What you want now. [Add elements](#add-elements) works later too |
| `Example values files` | Upstream's `values/`, each with the elements it turns on | None, unless one fits ([Example files](#example-files)) |

It then prints what it will create and asks `Proceed?`. The same install without prompts, as a script would run it:

```sh
mise run ps -- install --to v1.0.0 --elements odf,loki \
  --examples values/clusters/prod-east-1.yaml ~/fleet
```

```text
PolicyStack v1.0.0 into /home/me/fleet
  framework: install/, lifecycle/, lib/, tools/, CI, mise.toml
  elements: acm, gitops, infra-nodes, loki, machine-health, odf, storage-nodes, upgrade, user-monitoring
    + infra-nodes enabled by values/clusters/prod-east-1.yaml
    + machine-health enabled by values/clusters/prod-east-1.yaml
    + storage-nodes needed by odf
    + user-monitoring enabled by values/clusters/prod-east-1.yaml
  not added, named only by policies that ship disabled; POLICY011 fails once a cluster enables one: loki -> coo, loki -> logging
  example files, yours from now on: values/clusters/prod-east-1.yaml
copying framework (1/10)
copying acm (2/10)
...
copying user-monitoring (10/10)
created /home/me/fleet: one commit on main, nothing pushed. Next (docs/start/install.md):
  git -C /home/me/fleet remote add origin <your repository URL> && git -C /home/me/fleet push -u origin main
  write fleet/hubs/<hub>.yaml and fleet/<cluster>.yaml, then: mise run validate
```

The `+` lines are elements you didn't pick that the install adds, and why ([Dependencies](#dependencies)). Each element takes a few seconds to copy. If anything fails, or you press Ctrl-C, the directory is removed and you can run it again. Every flag and refusal: [install](../reference/launcher.md#install).

Push it, and install your repository's own tools with mise. From here on, run the launcher from your repository with `mise run ps`; delete `~/policystack-<tag>`.

```sh
cd ~/<repo>
git remote add origin https://github.com/<org>/<repo>.git
git push -u origin main
mise trust && mise install
```

You now have the framework (`install/`, `lifecycle/`, `lib/`, `tools/`, the CI workflows and hooks, `mise.toml`, the root `values.yaml`), one directory per element under `stack/`, and a `.copier-answers.yml` at the root and in each element, which records the release copied there:

```yaml title=".copier-answers.yml"
# Changes here will be overwritten by Copier; NEVER EDIT MANUALLY
_commit: v1.0.0
_src_path: https://github.com/PolicyStack/PolicyStack.git
element: ''
```

`mise run update` reads `_commit` to know what to merge, and the validator reads it to check every fleet pin (POLICY052) and every element's release (POLICY061). Never edit it by hand ([Answers files](../guides/upgrading.md#answers-files)).

Left out: everything that stays upstream, such as the docs, the upstream README and CODEOWNERS, and the example `values/` files you didn't pick. Who owns each path from here on: [The repository](../concepts/overview.md#the-repository).

## Add elements

Each element is copied on its own into `stack/<element>/`, from the framework's release, and gets its own `.copier-answers.yml`. Every repository has `acm`, `gitops` and `upgrade`: [Install the operators](#install-the-operators) reads the first two, and `upgrade` applies a fleet file's `version` and `channel`. To add another later, from your repository's root:

```sh
mise run ps -- add local-storage # (1)!
```

```text
add at v1.0.0 (your framework release)
  elements: local-storage
copying local-storage (1/1)
committed; nothing pushed
```

1.  Without a name it lists the elements you don't have. It commits on the branch you are on, as `feat: add local-storage (v1.0.0)`, so open a PR for it like any other change.

An element may be older than your framework, never newer: a newer one may need framework code you don't have, and POLICY061 fails it. So `add` always copies at your framework's `_commit`, and refuses an element that first ships in a later release, naming the release to update the framework to first ([Framework updates](../guides/upgrading.md#framework-updates)).

Each element is either stable or preview, shown in the picker and at the top of its `README.md`. A preview element may change incompatibly in a framework minor ([Element stability](../reference/compatibility.md#element-stability)). Each element's settings are in its `README.md`.

Elements you write yourself also go under `stack/`, without an answers file ([Writing an element](../develop/elements.md)).

### Dependencies

Some elements depend on another: `odf`, for one, waits on Policies of `storage-nodes`. `install` and `add` copy every such dependency with the element that needs it (`+ storage-nodes needed by odf`).

A dependency named only by a policy that ships disabled is not copied: `loki`'s optional policies wait on `coo` and `logging`, and the summary lists them under `not added`. Once a fleet file enables such a policy without its element, CI fails it with [POLICY011](../reference/rules.md), naming it; add the element then. In your repository the validator checks elements and values only through the clusters in `fleet/` ([What `validate` runs](../develop/validation.md#what-validate-runs)).

### Example files

A picked example file is copied byte for byte into your `values/` and is yours from then on: updates never merge upstream's changes into it. It turns its elements on as soon as a fleet file lists it, which is why `install` copies the elements it enables. Read it before listing it ([Example files](../concepts/values.md#example-files)).

### Without the launcher

The launcher runs copier 9.19.1 from its own environment, so copier needs no install of its own. To run copier by hand, from your repository's root:

```sh
copier() { mise x -- uv run --frozen --no-dev --project tools/policystack copier "$@"; }
fw=$(sed -n 's/^_commit: //p' .copier-answers.yml)
copier copy --vcs-ref "$fw" -d element=odf https://github.com/PolicyStack/PolicyStack.git stack/odf # (1)!
git add stack/odf && git commit -m "feat: add odf ($fw)"
```

1.  For a new repository, define the function with `--project ~/policystack-<tag>/tools/policystack` and run `copier copy --defaults --vcs-ref <tag> "$url" ~/<repo>` without `-d element`. That copies the framework only: `cd ~/<repo>`, copy `acm`, `gitops` and `upgrade` with the command above, then `git init -b main` and commit.

!!! warning "Pass `--vcs-ref` with your framework's tag"
    Without it copier copies the newest release, which can be newer than your framework, and POLICY061 fails it. Delete the directory and copy it again with your framework's tag. copier also copies no dependencies and checks no tags.

## Protect the repository

1.  Protect `fleet/`. A fleet file decides which revision and layers a cluster reads, and a fleet file with `hub:` and `state: absent` makes the hub delete that cluster ([Cluster lifecycle](../concepts/lifecycle.md)). Clusters pinned to a branch also take every merge to `values/` and `stack/` on that branch, so if production follows `main`, own those paths too, or pin production to tags ([Rolling out changes](../guides/rollout.md#choosing-a-pin)). Write your own `CODEOWNERS`:

    ```text
    # .github/CODEOWNERS
    /fleet/ @<org>/<team>
    ```

    The team needs write access to the repository. To protect only production, own `/fleet/prod-*.yaml` and `/fleet/hubs/` instead.

2.  Add a ruleset on the default branch (**Settings > Rules > Rulesets**). Without one nothing blocks a merge:

    - Require a pull request with code owner review.
    - Require the status checks `ci` and `pr-title`. The picker lists only checks that have run, so open one PR first (the fleet PR in [Add the hub's fleet file](#add-the-hubs-fleet-file) works), or type the names.
    - Block force pushes and deletion.

    These checks run on every PR ([CI workflows](../reference/ci.md), [Validator rules](../reference/rules.md)):

    | Check | What it does for you |
    |---|---|
    | `ci` | Runs the lint hooks from `.pre-commit-config.yaml` (a hook that rewrites a file fails and prints the diff) and the [validator](../develop/validation.md). The validator renders every fleet cluster the way Argo CD will and checks the result, each fleet file and its `revision` (POLICY050, POLICY052), and every values key (POLICY035). |
    | `pr-title` | The PR title must be a Conventional Commit, such as `feat: enable gitops on prod-east-1`. |
    | Render Diff | On PRs to the default branch, comments the Policies each fleet cluster gains, loses or changes. It only reports and never blocks. |

!!! warning "Label rights change what clusters run"
    Anyone who can label ManagedClusters or edit the cluster Secrets in `openshift-gitops` can switch a cluster to another fleet file. Keep both rights to hub admins ([Labels](../reference/fleet.md#labels)).

## Install the operators

=== "New hub"

    Installs ACM and OpenShift GitOps and creates the MultiClusterHub. The chart renders a Namespace, OperatorGroup and Subscription for each operator (ACM on channel `release-2.17`, GitOps on `latest`, both with automatic approval) plus the MultiClusterHub. These are the same names and specs the `acm` and `gitops` elements enforce, so [handover](#hand-the-operators-to-policystack) later adopts them in place. `helm template | oc apply` leaves no Helm release, so after handover only the elements own these objects.

    ```sh
    until helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
          | oc apply -f - && oc get argocd/openshift-gitops -n openshift-gitops; do sleep 30; done
    ```

    The first passes fail with `no matches for kind "MultiClusterHub"` until ACM registers its CRD. The loop ends once the GitOps operator has created its default Argo CD instance, `openshift-gitops`. If it never ends, stop it and read the `oc` error. The usual cause is an unreachable catalog:

    ```sh
    oc get packagemanifest advanced-cluster-management openshift-gitops-operator -n openshift-marketplace
    ```

    `NotFound` means the `redhat-operators` catalog isn't reachable. On a disconnected hub, pass the `global.catalogSources` layer (below).

    Verify. The MultiClusterHub takes about 10 minutes:

    ```sh
    oc wait mch/multiclusterhub -n open-cluster-management --for=jsonpath='{.status.phase}'=Running --timeout=20m
    # multiclusterhub.operator.open-cluster-management.io/multiclusterhub condition met
    oc wait argocd/openshift-gitops -n openshift-gitops --for=jsonpath='{.status.phase}'=Available --timeout=10m
    # argocd.argoproj.io/openshift-gitops condition met
    ```

    The chart installs the newest version in each channel. It fails with `versions and upgradeApproval pins apply only after handover` if the values you pass pin a version, because a plain Subscription with automatic approval can't hold one. Pin after handover ([Operator versions](../guides/operators.md)).

    ??? info "Single node or disconnected hub"
        Write a values layer: a YAML file under `values/` that the hub's fleet file will list. Settings for the `acm` element go under `stack.acm`:

        ```yaml
        # values/acm/<hub>.yaml
        stack:
          acm:
            config:
              availabilityConfig: Basic  # (1)!
        ```

        1.  Single replicas, for lab and single node hubs. The default is `High`.

        Pass it with `-f`:

        ```sh
        until helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
              -f values/acm/<hub>.yaml | oc apply -f - && oc get argocd/openshift-gitops -n openshift-gitops; do sleep 30; done
        ```

        Commit this file and list it in the [hub's fleet file](#add-the-hubs-fleet-file). At handover the `acm` element enforces `availabilityConfig` on the MultiClusterHub, and without this layer it switches the hub to `High`.

        On a disconnected hub, also pass the layer that sets `global.catalogSources`, which points each Subscription at your mirrored catalog, such as `-f values/datacenters/<dc>.yaml` ([Disconnected clusters](../guides/disconnected.md#the-hub)).

=== "Hub that already runs ACM and GitOps"

    Skip this step. Your operators keep running as they are. Run the [compatibility checks](#existing-hub-check-compatibility) before enabling the `acm` or `gitops` element.

    If only one of them runs, install the other from OperatorHub following the Red Hat docs (GitOps: namespace `openshift-gitops-operator`, channel `latest`). Don't run this chart: it renders both, and would create a second Subscription for the one already installed.

## Install PolicyStack

??? info "Private repository: add a repository secret first"
    Argo CD needs credentials to read a private repo ([Argo CD docs](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repositories)). Without them the ApplicationSets can't read the repository and create no Applications.

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: policystack-repo
      namespace: openshift-gitops
      labels:
        argocd.argoproj.io/secret-type: repository
    stringData:
      type: git
      url: https://github.com/<org>/<repo>.git
      username: <user>
      password: <token>  # (1)!
    ```

    1.  A fine-grained personal access token with read-only Contents access to this one repository.

    Apply it with `oc apply -f -`. Never commit it.

```sh
helm install policystack ./install -n openshift-gitops \
  --set hubName=<hub> --set gitRepo=https://github.com/<org>/<repo>.git
```

Both values are required and stored in the release; upgrades reuse them. The release's chart version is the framework release it came from, which [upgrades](../guides/upgrading.md#install-chart) compare against. The release creates ([Install chart](../reference/install.md)):

| Object | Why |
|---|---|
| ApplicationSet `policystack` | One Application per element per cluster, named `<element>-<cluster>`. Each renders the element's Policies into `pstack`. |
| ApplicationSet `policystack-lifecycle` | One Application per cluster this hub builds. Creates nothing until a fleet file sets `hub: <hub>` ([Cluster lifecycle](../concepts/lifecycle.md)). |
| Namespace `pstack` | Holds the Policies. |
| GitOpsCluster `argo-acm-importer-global`, Placement `global` | Create the Argo CD cluster secret for every ManagedCluster ([Cluster secrets](../concepts/architecture.md#cluster-secrets)). |
| ManagedClusterSetBinding `global` | ACM lets a Placement choose only from cluster sets bound into its namespace. `global` is ACM's built-in set containing every ManagedCluster. Bound in `openshift-gitops` (for the GitOpsCluster) and `pstack` (for the element Placements). |
| ClusterRole and binding `openshift-gitops-policy-admin` | Let Argo CD manage Policies, Placements and PlacementBindings. |

Verify:

```sh
oc get applicationsets -n openshift-gitops
# NAME                    AGE
# policystack             10s
# policystack-lifecycle   10s
oc get secrets -n openshift-gitops -l argocd.argoproj.io/secret-type=cluster
# one secret per ManagedCluster, including the hub
```

No secret for the hub means it doesn't manage itself ([Prerequisites](#prerequisites)).

??? failure "`... exists and cannot be imported into the current release: invalid ownership metadata`"
    An object of the same name already exists. Hubs set up with the ACM GitOps guide often have ManagedClusterSetBinding `global` in `openshift-gitops` and ClusterRole/ClusterRoleBinding `openshift-gitops-policy-admin`. Rerun with `--take-ownership` (Helm 3.17 or later) to adopt them. Adoption replaces their spec with the chart's.

## Add the hub's fleet file

A fleet file enrolls a cluster: Argo CD reads it from the default branch, and it names the `revision` (branch, tag or SHA) at which `stack/` and `values/` are read, plus the values layers the cluster gets ([Fleet files](../concepts/overview.md#fleet-files)). The hub reads `fleet/hubs/<hub>.yaml`, never `fleet/<cluster>.yaml`.

Start with a file that lists no layers:

```yaml
# fleet/hubs/<hub>.yaml
revision: main  # (1)!
```

1.  A branch the cluster follows, or a tag or SHA it stays on ([Revision](../reference/fleet.md#revision)). Name your default branch if it isn't `main`.

PR it to the default branch and merge. Within about 6 minutes (the ApplicationSet polls Git every 3 minutes) every directory under `stack/` gets an Application named `<element>-<hub>`, one per element you copied. Each shows Synced and Healthy and renders nothing, because every element ships off except `upgrade`, which acts only when a fleet file sets `version` or `channel`.

```sh
oc get applications.argoproj.io -n openshift-gitops | grep <hub>  # (1)!
# acm-<hub>      Synced   Healthy
# gitops-<hub>   Synced   Healthy
# ...
```

1.  ACM installs a second `applications` kind, so use the full name.

Then turn elements on through layers. A layer sets `stack.<element>.enabled`; the key is the element name in camelCase ([Enable an element](first-cluster.md#enable-an-element)):

```yaml
# values/environments/hub.yaml
stack:
  certManager:
    enabled: true
```

List each layer under `valueFiles`, lowest precedence first ([Order](../concepts/values.md#order)). A `values/clusters/<hub>.yaml`, if you add one, is always read last, without being listed, and holds hub-only settings:

```yaml
# fleet/hubs/<hub>.yaml
revision: main
valueFiles:
  - environments/hub.yaml  # (1)!
```

1.  Add `acm/<hub>.yaml` too if you wrote it under [Single node or disconnected hub](#install-the-operators).

!!! warning "Listed layers apply at once, with `enforce`"
    Every shipped element enforces by default, so the hub changes as soon as the PR merges. If you copied a layer from upstream's examples ([Example files](../concepts/values.md#example-files)), read it first: they turn elements on. Or put `global.remediationAction: inform` in `values/clusters/<hub>.yaml` to report without changing anything ([Dry run](../guides/rollout.md#dry-run)).

Every `valueFiles` entry must exist at the revision. Argo CD skips a missing file silently, but CI fails the PR:

```text
error POLICY050 invalid fleet file: valueFiles "acm/hub-east.yaml": not a file under values/
error POLICY052 revision "main" has no values/acm/hub-east.yaml, which Argo CD skips
```

Commit the layer in the same PR, or leave the entry out until you need it.

??? failure "No Applications after 6 minutes"
    Read the ApplicationSet's error first:

    ```sh
    oc get applicationset policystack -n openshift-gitops \
      -o jsonpath='{.status.conditions[?(@.type=="ErrorOccurred")].message}'
    ```

    | Cause | Symptom |
    |---|---|
    | File name differs from `hubName`, or the file isn't on the default branch | No error. Nothing happens. |
    | YAML error, no `revision`, or a revision that doesn't exist | Error above. No cluster on this hub gets Application changes until it is fixed ([One bad fleet file stops the hub](../concepts/overview.md#one-bad-fleet-file-stops-the-hub)). |
    | Private repository without a repository secret | Error above: authentication or repository not found. |
    | Hub doesn't manage itself | No hub cluster secret ([Prerequisites](#prerequisites)). |

Check the Policies the hub got and whether they comply. The Policies Argo CD syncs from Git (root Policies) are in `pstack`. ACM copies each (a replicated Policy) into the namespace of the hub's ManagedCluster, which carries the per-cluster compliance ([Check compliance](first-cluster.md#check-compliance)). Both are empty until a layer enables an element:

```sh
oc get policies.policy.open-cluster-management.io -n pstack
oc get policies.policy.open-cluster-management.io -n local-cluster
```

Managed clusters follow the same pattern with `fleet/<cluster>.yaml` ([First cluster](first-cluster.md)).

## Hand the operators to PolicyStack

Optional. Without it, both operators keep their Subscriptions as installed (Install the operators sets automatic approval, so they upgrade on their own within their channels), and PolicyStack can't pin their versions. Handover enables the `acm` and `gitops` elements on the hub. Their `install` Policy, generated from each element's `operatorPolicies` ([The install Policy](../concepts/elements.md#the-install-policy)), then enforces the operators' Subscription and OperatorGroup: channel, approved versions and upgrades. The `acm` element's `hub` Policy enforces the MultiClusterHub.

Wait until the hub's other Applications have stayed Synced and Healthy through a few changes. PolicyStack is delivered by this ACM and this Argo CD, so a bad value in these two elements can break the path that would deliver the fix.

### Existing hub: check compatibility

The `install` Policy's OperatorPolicy (the ACM template that manages a Subscription and OperatorGroup, [OperatorPolicy](../concepts/acm.md#operatorpolicy)) names its Subscription after the operator package and finds it by namespace and that name. An element adopts an operator only when the namespace, OperatorGroup name and Subscription name match the element.

!!! warning
    A namespace or Subscription name mismatch makes the OperatorPolicy create a second Subscription.

1.  List the operators' Subscriptions, OperatorGroups and the MultiClusterHub:

    ```sh
    oc get subscriptions.operators.coreos.com -A \
      -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,PACKAGE:.spec.name,CHANNEL:.spec.channel \
      | grep -E 'advanced-cluster-management|openshift-gitops-operator'
    oc get operatorgroups -n <NS>   # <NS> from the first command
    oc get mch -A
    ```

2.  Compare them with the names each element expects:

    | Element | Namespace | OperatorGroup | Subscription |
    |---|---|---|---|
    | `acm` | `open-cluster-management` | `open-cluster-management` | `advanced-cluster-management` |
    | `gitops` | `openshift-gitops-operator` | `openshift-gitops-operator` | `openshift-gitops-operator` |

3.  Fix each mismatch in `values/acm/<hub>.yaml`, the same layer [the next step](#enable-the-elements) enables both elements in. Short keys in the Fix column are under `stack.<element>.operatorPolicies.<element>`, for example `stack.acm.operatorPolicies.acm.subscription.channel`.

    | Mismatch | Fix |
    |---|---|
    | NAME differs from PACKAGE | Leave that element off. The OperatorPolicy would create a second Subscription for the same package. |
    | NS differs, such as GitOps in `openshift-operators` | Set `namespace` and `operatorGroup.name` to match, or leave the element off. Otherwise the Policy creates a second Subscription and OperatorGroup in the element's namespace, which OLM refuses to install, and the Policy stays NonCompliant. |
    | Console-created OperatorGroup, `<ns>-xxxxx` | Set `operatorGroup.name` to it. Another name makes the Policy NonCompliant. |
    | Older channel | Set `subscription.channel` to it, then move one minor at a time. |
    | MultiClusterHub not `multiclusterhub` in `open-cluster-management` | Leave `acm` off. The `hub` Policy manages only that name. |
    | MultiClusterHub on `Basic` | Set `stack.acm.config.availabilityConfig: Basic`, or handover switches it to `High`. |

    For example, GitOps in `openshift-operators`:

    ```yaml
    # values/acm/<hub>.yaml
    stack:
      gitops:
        operatorPolicies:
          gitops:
            namespace: openshift-operators
            operatorGroup:
              name: global-operators
    ```

### Enable the elements

Enable both elements in the hub's layer, and list that layer in `fleet/hubs/<hub>.yaml` in the same PR:

```yaml
# values/acm/<hub>.yaml
stack:
  acm:
    enabled: true
  gitops:
    enabled: true
    policies:
      instance:
        enabled: false  # (1)!
      link:
        enabled: false  # (2)!
```

1.  `instance` and `link` are the `gitops` element's other Policies, from `converters/instance/` and `converters/link/` ([Reading an element's README](../concepts/elements.md#reading-an-elements-readme), [gitops README](https://github.com/PolicyStack/PolicyStack/blob/main/stack/gitops/README.md)). `instance` manages the Argo CD instance that delivers PolicyStack. Never let PolicyStack reconcile it: a bad value there breaks the path that fixes it.
2.  The console link depends on `instance`. Left on, it would stay Pending.

!!! warning "Don't rerun Install the operators after handover"
    `oc apply` writes the channel and automatic approval from whatever values you pass. The `install` Policy changes them back, but in between OLM can follow the wrong channel or approve an upgrade past a `versions` pin.

Verify:

```sh
oc get policies.policy.open-cluster-management.io -n local-cluster | grep -E 'acm|gitops'
# pstack.hub-acm-<hub>          enforce   Compliant
# pstack.install-acm-<hub>      enforce   Compliant
# pstack.install-gitops-<hub>   enforce   Compliant
oc get csv -n open-cluster-management   # the same CSV as before, not a reinstall
```

All three turn Compliant within a few minutes. NonCompliant on `install` means a name in the compatibility table doesn't match: read the template's message ([Check compliance](first-cluster.md#check-compliance)) and fix the override.

ACM and GitOps upgrades now follow the hub's fleet `revision` like any other element ([Operator versions](../guides/operators.md)).

## Teardown

```sh
oc delete applicationset policystack policystack-lifecycle -n openshift-gitops
```

This deletes every Application, and each Application deletes the Policies it rendered. ACM then stops applying them, but what they created on the clusters stays, operators included ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)). Clusters this hub built keep running, unmanaged ([Abandon](../guides/lifecycle.md#abandon)).

Every other chart object carries `helm.sh/resource-policy: keep`, so `helm uninstall` removes only the release record. Once `oc get policies.policy.open-cluster-management.io -n pstack` returns nothing, remove the rest. Skip any object you created yourself and adopted with `--take-ownership`:

```sh
helm uninstall policystack -n openshift-gitops
oc delete gitopscluster argo-acm-importer-global -n openshift-gitops  # (1)!
oc delete placement global -n openshift-gitops
oc delete managedclustersetbinding global -n openshift-gitops
oc delete clusterrolebinding,clusterrole openshift-gitops-policy-admin
oc delete namespace pstack
```

1.  ACM then removes the cluster secrets it created, unless another GitOpsCluster selects the same clusters.

!!! note "`--cascade=orphan` is not a way to stop"
    It deletes only the ApplicationSets. The Applications keep syncing from Git, so PolicyStack keeps managing every cluster. Use it to recreate the ApplicationSets without touching the Policies.

Next: [First cluster](first-cluster.md).
