# Installation

PolicyStack runs on the ACM hub. You install it once per hub with Helm, from [`install/`](https://github.com/PolicyStack/PolicyStack/tree/main/install). After that, everything is driven from Git. Run every command from the root of your fork, logged in to the hub as a cluster administrator.

1. [Check the prerequisites](#prerequisites)
2. [Install the operators](#install-the-operators) (new hubs only)
3. [Install PolicyStack](#install-policystack)
4. [Add the hub's fleet file](#add-the-hubs-fleet-file)
5. [Hand the operators to PolicyStack](#hand-the-operators-to-policystack) (later, once the stack is proven)

| Placeholder | Meaning | Example |
|---|---|---|
| `<hub>` | The hub's name in PolicyStack (`hubName`). Picks `fleet/hubs/<hub>.yaml`. Unique per hub reading the repo, including a DR hub. | `acm-dc1` |
| `<org>/<fork>` | Your GitHub org and fork of PolicyStack | `acme/policystack` |

## Prerequisites

- `oc`, and `helm` 3.14 or later (3.18 or later for `--take-ownership`).
- A fork of PolicyStack, cloned locally. Argo CD reads your fleet files and values from it; upstream ships none.
- A hub that already runs OpenShift GitOps must use its default Argo CD instance, `openshift-gitops`.
- A hub that builds clusters from fleet files needs ACM 2.16 or later ([Cluster lifecycle prerequisites](lifecycle.md#prerequisites)).

```sh
git clone https://github.com/<org>/<fork>.git && cd <fork>
helm version --short                # v3.14.0 or later
oc auth can-i '*' '*' --all-namespaces   # yes
```

??? info "Private fork: add a repository secret"
    Argo CD needs credentials to read a private repo ([Argo CD docs](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repositories)):

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

    1.  A personal access token with read access to the fork.

    Apply it after `openshift-gitops` exists, before [Install PolicyStack](#install-policystack).

## Install the operators

=== "New hub"

    Installs ACM and OpenShift GitOps and creates the MultiClusterHub. The loop retries every 30 seconds while the operators install their CRDs. If it never ends, stop it and read the `oc` error, usually an unreachable catalog.

    ```sh
    until helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
          | oc apply -f - && oc get argocd/openshift-gitops -n openshift-gitops; do sleep 30; done
    ```

    Verify. The MultiClusterHub takes about 10 minutes:

    ```sh
    oc wait mch/multiclusterhub -n open-cluster-management --for=jsonpath='{.status.phase}'=Running --timeout=20m
    # multiclusterhub.operator.open-cluster-management.io/multiclusterhub condition met
    oc wait argocd/openshift-gitops -n openshift-gitops --for=jsonpath='{.status.phase}'=Available --timeout=10m
    # argocd.argoproj.io/openshift-gitops condition met
    ```

    These are the same objects the `acm` and `gitops` elements enforce, so [handover](#hand-the-operators-to-policystack) adopts them in place. The chart installs the channel head and refuses `versions` or `upgradeApproval` pins; pin after handover ([Operator upgrades](rollout.md#operator-upgrades)).

    ??? info "Single node or disconnected hub"
        Add a values layer to `helm template` with `-f`:

        ```yaml
        # values/acm/<hub>.yaml
        stack:
          acm:
            config:
              availabilityConfig: Basic  # (1)!
        ```

        1.  Single replicas, for lab and single node hubs. The default is `High`.

        ```sh
        until helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
              -f values/acm/<hub>.yaml | oc apply -f - && oc get argocd/openshift-gitops -n openshift-gitops; do sleep 30; done
        ```

        On a disconnected hub, also add the layer that sets `global.catalogSources`, such as `-f values/datacenters/<dc>.yaml` ([Disconnected clusters](disconnected.md)).

=== "Hub that already runs ACM/GitOps"

    Skip this step. Your operators keep running as they are. Run the compatibility checks under [Hand the operators to PolicyStack](#hand-the-operators-to-policystack) before enabling the `acm` or `gitops` element.

## Install PolicyStack

Creates the ApplicationSets, the `pstack` policy namespace, the cluster import into Argo CD and its RBAC.

```sh
helm install policystack ./install -n openshift-gitops \
  --set hubName=<hub> --set gitRepo=https://github.com/<org>/<fork>.git \
  --description "$(git rev-parse --short HEAD)"
```

Both values are required and stored in the release; upgrades reuse them. `--description` records the commit, which [upgrades](#upgrading) compare against.

Verify:

```sh
oc get applicationsets -n openshift-gitops
# NAME                    AGE
# policystack             10s
# policystack-lifecycle   10s
oc get secrets -n openshift-gitops -l argocd.argoproj.io/secret-type=cluster
# one secret per ManagedCluster, including the hub
```

| Object | Name | Namespace | Purpose |
|---|---|---|---|
| ApplicationSet | `policystack` | `openshift-gitops` | One Application per element per cluster ([Applications](applicationset.md#applications)) |
| ApplicationSet | `policystack-lifecycle` | `openshift-gitops` | One Application per cluster this hub builds ([Lifecycle generator](applicationset.md#lifecycle-generator)) |
| Namespace | `pstack` | | Holds the Policies |
| ManagedClusterSetBinding | `global` | `openshift-gitops`, `pstack` | Lets Placements select every cluster |
| Placement | `global` | `openshift-gitops` | Every cluster, tolerating `unreachable` and `unavailable` |
| GitOpsCluster | `argo-acm-importer-global` | `openshift-gitops` | Creates an Argo CD cluster secret per selected cluster |
| ClusterRole, ClusterRoleBinding | `openshift-gitops-policy-admin` | | Lets the Argo CD application controller manage Policies, PolicySets, Placements and PlacementBindings, the only objects elements render |

!!! warning "Objects outlive the release"
    Every object carries `helm.sh/resource-policy: keep`. `helm uninstall policystack` leaves PolicyStack running. To remove it, see [Teardown](#teardown).

If Helm reports an object that already exists, such as a hand-made `global` binding, rerun with `--take-ownership` (Helm 3.18 or later) to adopt it.

## Add the hub's fleet file

The hub gets Applications only once its fleet file is on the default branch ([The hub](applicationset.md#the-hub)):

```yaml
# fleet/hubs/<hub>.yaml
revision: main  # (1)!
valueFiles:  # (2)!
  - environments/prod.yaml
  - datacenters/dc1.yaml
  - acm/<hub>.yaml
```

1.  Branch, tag or SHA the hub's Applications render from ([Revision](applicationset.md#revision)).
2.  Layers under `values/`, lowest precedence first ([Order](values.md#order)). `values/clusters/<hub>.yaml` is always added last.

Merge it to the default branch. Within about 6 minutes, each element under `stack/` gets an Application named `<element>-<hub>`:

```sh
oc get applications.argoproj.io -n openshift-gitops | grep <hub>
# acm-<hub>      Synced   Healthy
# gitops-<hub>   Synced   Healthy
# ...
```

An element no layer enables syncs with no resources. Managed clusters follow the same pattern with `fleet/<cluster>.yaml` ([Onboarding a cluster](rollout.md#onboarding-a-cluster)).

## Hand the operators to PolicyStack

Once the rest of the stack is proven on the hub, let the `acm` and `gitops` elements own the operators.

???+ warning "Existing hub: check compatibility first"
    An element adopts an operator only when the namespace, OperatorGroup name and Subscription name (which must equal the package) match the element.

    ```sh
    oc get subscriptions.operators.coreos.com -A \
      -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,PACKAGE:.spec.name,CHANNEL:.spec.channel \
      | grep -E 'advanced-cluster-management|openshift-gitops-operator'
    oc get operatorgroups -n <NS>   # <NS> from the first command
    ```

    | Element | Namespace | OperatorGroup | Subscription |
    |---|---|---|---|
    | `acm` | `open-cluster-management` | `open-cluster-management` | `advanced-cluster-management` |
    | `gitops` | `openshift-gitops-operator` | `openshift-gitops-operator` | `openshift-gitops-operator` |

    | Mismatch | Fix |
    |---|---|
    | NAME differs from PACKAGE | Leave that element off. The OperatorPolicy would create a second Subscription. |
    | NS differs, such as GitOps in `openshift-operators` | Set `namespace` and `operatorGroup.name` to match, or leave the element off. Otherwise OLM installs the operator twice. |
    | Console-created OperatorGroup, `<ns>-xxxxx` | Set `operatorGroup.name` to it. |
    | Older channel | Set `subscription.channel` to it, then move one minor at a time. |

    Overrides go in `values/acm/<hub>.yaml`:

    ```yaml
    stack:
      gitops:
        operatorPolicies:
          gitops:
            namespace: openshift-operators
            operatorGroup:
              name: global-operators
    ```

```yaml
# values/acm/<hub>.yaml, listed in fleet/hubs/<hub>.yaml valueFiles
stack:
  acm:
    enabled: true
  gitops:
    enabled: true
    toggles:
      instance: false  # (1)!
      link: false  # (2)!
```

1.  PolicyStack never reconciles the Argo CD instance that delivers it. A bad value there would break the path that fixes it.
2.  The console link depends on `instance`.

!!! warning
    Do not run [Install the operators](#install-the-operators) after handover. It resets any channel the hub layer overrides.

ACM and GitOps upgrades now follow the hub's fleet `revision` like any other element ([Operator upgrades](rollout.md#operator-upgrades)).

## Upgrading

| Part | How |
|---|---|
| Elements, lifecycle, values | Merge upstream into the fork, then move fleet `revision` pins ([Rolling out changes](rollout.md)) |
| ACM and GitOps | Through the elements after [handover](#hand-the-operators-to-policystack). Before it, OLM upgrades within the installed channel. |
| The install chart | `helm upgrade`, below |

??? info "Upgrade the install chart"
    The ApplicationSets are not pinned per cluster: a change under `install/templates/` reaches every cluster on the hub at once. Upgrade only when the `git diff` below prints changes:

    ```sh
    helm history policystack -n openshift-gitops   # DESCRIPTION of the deployed revision = <last>
    git diff <last>..HEAD -- install/
    ```

    If the deployed revision's DESCRIPTION reads `Rollback to <n>`, use revision `<n>`'s DESCRIPTION as `<last>`.

    ```sh
    helm upgrade policystack ./install -n openshift-gitops --reset-then-reuse-values \
      --description "$(git rev-parse --short HEAD)"
    ```

    `--reset-then-reuse-values` takes the new chart defaults and keeps `hubName` and `gitRepo`. Roll back with `helm rollback policystack <revision> -n openshift-gitops`.

??? info "From the appset chart"
    Hubs installed with the old `appset` release and `gitops-prereq/` move over once, with Helm 3.18 or later. `<ns>` is the namespace of the `appset` release.

    1. Check the stored manifest keeps all four objects. The ApplicationSets gained `keep` on 2026-10-13. Below 4, run `helm upgrade appset` once, with its usual values, from a commit before `install/` existed.

        ```sh
        helm get manifest appset -n <ns> | grep -c 'helm.sh/resource-policy'   # 4
        ```

    2. Move the release. Reusing the stored values keeps `hubName` exact; a different one renames and recreates every hub Application.

        ```sh
        helm get values appset -n <ns> -o yaml > appset-values.yaml
        helm uninstall appset -n <ns>
        helm install policystack ./install -n openshift-gitops --take-ownership -f appset-values.yaml \
          --description "$(git rev-parse --short HEAD)"
        oc get applicationset policystack -n openshift-gitops \
          -o jsonpath='{.metadata.annotations.meta\.helm\.sh/release-name}'   # policystack
        ```

        Nothing is deleted. If the old ApplicationSet had `preserveResourcesOnDeletion: true`, its Applications gain the resources finalizer, and an Application left OutOfSync on a disabled element prunes on its next automated sync.

    3. Drop the cluster-admin binding the old install created. Skip this if the `openshift-gitops` instance also syncs Applications outside PolicyStack; they may rely on it.

        ```sh
        oc auth can-i create policies.policy.open-cluster-management.io -n pstack \
          --as=system:serviceaccount:openshift-gitops:openshift-gitops-argocd-application-controller   # yes
        oc get clusterrolebinding -o wide | grep 'ClusterRole/cluster-admin' | grep openshift-gitops-argocd-application-controller
        oc adm policy remove-cluster-role-from-user cluster-admin -z openshift-gitops-argocd-application-controller \
          -n openshift-gitops --rolebinding-name=<name>   # <name> from the previous command
        ```

        !!! danger
            Without `--rolebinding-name`, the last command edits every cluster-admin binding. Never delete `openshift-gitops-policy-admin`: nothing recreates it before the next `helm upgrade`.

    4. Sync the Applications and check they stay Synced.

## Teardown

```sh
oc delete applicationset policystack policystack-lifecycle -n openshift-gitops
```

!!! danger "Deletes every Policy"
    This deletes every Application and every Policy they rendered ([Removing an element](rollout.md#removing-an-element)). Clusters this hub built keep running, unmanaged ([Abandon](lifecycle.md#abandon)). Add `--cascade=orphan` to keep the Applications and Policies.
