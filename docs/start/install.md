# Installation

PolicyStack runs on the ACM hub: a Helm release from [`install/`](https://github.com/PolicyStack/PolicyStack/tree/main/install) points the hub's Argo CD at your fork, and ACM delivers the Policies Argo CD renders, hub objects that describe a cluster's configuration ([ACM policies in brief](../concepts/acm.md), [what the release creates](#install-policystack)).

- An **element** is a Helm chart under `stack/` that turns settings for one product into Policies. Every element ships off.
- A **layer** is a values file under `values/` that turns elements on and configures them.
- A **fleet file**, `fleet/<cluster>.yaml`, enrolls one cluster and lists the layers it reads. The hub is itself a managed cluster, enrolled by `fleet/hubs/<hub>.yaml`.

Full model: [How PolicyStack works](../concepts/overview.md).

1. [Check the prerequisites](#prerequisites)
2. [Set up your fork](#set-up-your-fork)
3. [Install the operators](#install-the-operators) (new hubs only)
4. [Install PolicyStack](#install-policystack)
5. [Add the hub's fleet file](#add-the-hubs-fleet-file)
6. [Hand the operators to PolicyStack](#hand-the-operators-to-policystack) (later, once the stack is proven)

Run every command from the root of your fork's clone, logged in to the hub as a cluster administrator.

| Placeholder | Meaning | Example |
|---|---|---|
| `<hub>` | The hub's name in PolicyStack (`hubName`). Every hub's own ManagedCluster is called `local-cluster`, so PolicyStack needs a unique name to pick the hub's files ([The hub](../concepts/architecture.md#the-hub)). Lowercase DNS label, at most 30 characters ([Naming limits](../concepts/policies.md#naming-limits)). | `hub-east` |
| `<org>/<fork>` | Your GitHub org and repository | `acme/policystack` |

!!! warning "Choose `hubName` once"
    Changing it deletes and recreates the hub's own Applications and Policies, and abandons the clusters this hub built until their fleet files name the new hub ([The hub](../concepts/architecture.md#the-hub)). Two active hubs with one `hubName` apply the same hub file and build the same clusters.

## Prerequisites

- A GitHub repository for your fork. Argo CD can read any Git server, but the CI checks, CODEOWNERS and rulesets below are GitHub-specific.
- `oc`, and `helm` 3.14 or later on your workstation: [upgrades](../guides/upgrading.md#install-chart) use `--reset-then-reuse-values`, added in 3.14. You need 3.17 or later only if the install hits an [ownership conflict](#install-policystack). Your local `helm` runs only the install chart. Argo CD renders elements with the Helm bundled in GitOps (3.18.4 in 1.18), and CI uses the same version.
- Supported floors: ACM 2.16, OpenShift 4.20, OpenShift GitOps 1.18. Cluster lifecycle has extra requirements ([Cluster lifecycle prerequisites](../guides/lifecycle.md#prerequisites)).
- The hub manages itself: MultiClusterHub self-management on, the ACM default. It creates the hub's ManagedCluster (`local-cluster` by default), which carries the `local-cluster` label. Without it the hub never picks up `fleet/hubs/<hub>.yaml`, gets no Applications and builds no clusters.
- A hub that already runs OpenShift GitOps must use its default Argo CD instance, `openshift-gitops`. The install chart creates its objects in that namespace and grants Policy rights to that instance's application controller; another instance has neither.
- The hub is not a passive DR hub. On a passive DR hub, install PolicyStack only at takeover ([Hub disaster recovery](../guides/lifecycle.md#hub-disaster-recovery)).

```sh
helm version --short                     # v3.14.0 or later
oc auth can-i '*' '*' --all-namespaces   # yes
oc get managedclusters -l local-cluster=true
# NAME            HUB ACCEPTED   ...   AVAILABLE
# local-cluster   true           ...   True
```

The last command applies to a hub that already runs ACM. A new hub creates `local-cluster` in [Install the operators](#install-the-operators).

## Set up your fork

Argo CD reads only your fork. Upstream ships no fleet files and no live configuration.

1.  Copy [PolicyStack](https://github.com/PolicyStack/PolicyStack) into your org and clone it. GitHub doesn't allow a private fork of a public repository, so a private copy is a new repository:

    === "Public"

        Fork it on GitHub, then:

        ```sh
        git clone https://github.com/<org>/<fork>.git && cd <fork>
        ```

    === "Private"

        Create an empty private repository, then:

        ```sh
        git clone https://github.com/PolicyStack/PolicyStack.git <fork> && cd <fork>
        git remote rename origin upstream && git config remote.upstream.tagOpt --no-tags
        git remote add origin https://github.com/<org>/<fork>.git && git push -u origin main
        ```

        [Merge upstream](../guides/upgrading.md#merge-upstream) works the same, through the `upstream` remote. Argo CD needs a [repository secret](#install-policystack) to read it.

    You edit `fleet/` (which clusters exist, which revision each reads, which layers it gets) and `values/` (the layers). You may add, change or delete elements under `stack/`; a changed element is yours to maintain. `install/`, `lifecycle/`, `tools/` and `.github/` come from upstream when you [merge upstream](../guides/upgrading.md#merge-upstream) ([The repository](../concepts/overview.md#the-repository)).

2.  Decide what to do with the example files under `values/` ([Example files](../concepts/values.md#example-files)). They do nothing until a fleet file lists them, except `values/clusters/<name>.yaml`, which the hub or cluster of that name reads automatically, with no fleet-file entry. The shipped `acm-dc1.yaml` and `prod-east-1.yaml` turn elements on, so don't use those names for real clusters, or delete the files. Once `fleet/` has a file, the fork's CI (step 3) warns (POLICY052) about every `values/clusters/<name>.yaml` that no fleet file reads.

3.  Enable GitHub Actions on the fork. GitHub doesn't run workflows on a new fork until you enable them in its **Actions** tab. These checks then run on every PR:

    | Check | What it does for you |
    |---|---|
    | `ci` | Runs the [validator](../develop/validation.md), the repo's CI checker. It renders every element the way Argo CD will and checks the result. Once `fleet/` exists it also checks each fleet file and its `revision` (POLICY050, POLICY052) and every values key (POLICY035). Also runs the lint hooks from `.pre-commit-config.yaml` (a hook that rewrites a file fails and prints the diff) and the docs build. |
    | `pr-title` | The PR title must be a Conventional Commit, such as `feat: enable gitops on prod-east-1`. |
    | Render Diff | On PRs to the default branch, comments the Policies each fleet cluster gains, loses or changes. It only reports and never blocks. |

    Details: [CI workflows](../reference/ci.md), [Validator rules](../reference/rules.md).

4.  Protect `fleet/`. A fleet file decides which revision and layers a cluster reads, and a fleet file with `hub:` and `state: absent` makes the hub delete that cluster ([Cluster lifecycle](../concepts/lifecycle.md)). Clusters pinned to a branch also take every merge to `values/` and `stack/` on that branch, so if production follows `main`, own those paths too, or pin production to tags ([Rolling out changes](../guides/rollout.md#choosing-a-pin)). Replace upstream's `* @PolicyStack/maintainers` line in `CODEOWNERS`: it names a team your org doesn't have, so no review would be requested.

    ```text
    # .github/CODEOWNERS
    /fleet/ @<org>/<team>
    ```

    The team needs write access to the fork. To protect only production, own `/fleet/prod-*.yaml` and `/fleet/hubs/` instead.

    Then add a ruleset on the default branch (**Settings > Rules > Rulesets**). A fork doesn't inherit upstream's, and without one nothing blocks a merge:

    - Require a pull request with code owner review.
    - Require the status checks `ci` and `pr-title`. The picker lists only checks that have run, so open one PR first (the fleet PR in [Add the hub's fleet file](#add-the-hubs-fleet-file) works), or type the names.
    - Block force pushes and deletion.

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
    Argo CD needs credentials to read a private repo ([Argo CD docs](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repositories)). Without them the ApplicationSets can't read the fork and create no Applications.

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
      url: https://github.com/<org>/<fork>.git
      username: <user>
      password: <token>  # (1)!
    ```

    1.  A fine-grained personal access token with read-only Contents access to this one repository.

    Apply it with `oc apply -f -`. Never commit it to the fork.

```sh
helm install policystack ./install -n openshift-gitops \
  --set hubName=<hub> --set gitRepo=https://github.com/<org>/<fork>.git \
  --description "$(git rev-parse --short HEAD)"
```

Both values are required and stored in the release; upgrades reuse them. `--description` records the commit, which [upgrades](../guides/upgrading.md#install-chart) compare against. The release creates ([Install chart](../reference/install.md)):

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

PR it to the default branch and merge. Within about 6 minutes (the ApplicationSet polls Git every 3 minutes) every directory under `stack/` gets an Application named `<element>-<hub>`, 43 with the shipped catalog. Each shows Synced and Healthy and renders nothing, because every element ships off except `upgrade`, which acts only when a fleet file sets `version` or `channel`.

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
    Every shipped element enforces by default, so the hub changes as soon as the PR merges. The upstream example layers turn elements on ([Example files](../concepts/values.md#example-files)). List only layers you wrote or reviewed, or put `global.remediationAction: inform` in `values/clusters/<hub>.yaml` to report without changing anything ([Dry run](../guides/rollout.md#dry-run)).

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
