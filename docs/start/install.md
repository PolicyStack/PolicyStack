# Installation

PolicyStack runs on the ACM hub. You install it once per hub with Helm, from [`install/`](https://github.com/PolicyStack/PolicyStack/tree/main/install). After that, everything is driven from Git. Run every command from the root of your fork, logged in to the hub as a cluster administrator.

1. [Check the prerequisites](#prerequisites)
2. [Install the operators](#install-the-operators) (new hubs only)
3. [Install PolicyStack](#install-policystack)
4. [Add the hub's fleet file](#add-the-hubs-fleet-file)
5. [Hand the operators to PolicyStack](#hand-the-operators-to-policystack) (later, once the stack is proven)

| Placeholder | Meaning | Example |
|---|---|---|
| `<hub>` | The hub's name in PolicyStack (`hubName`). Picks `fleet/hubs/<hub>.yaml`. Unique per active hub reading the repo. A DR hub takes over the `hubName` of the hub it replaces ([Hub disaster recovery](../guides/lifecycle.md#hub-disaster-recovery)). | `acm-dc1` |
| `<org>/<fork>` | Your GitHub org and fork of PolicyStack | `acme/policystack` |

## Prerequisites

- `oc`, and `helm` 3.14 or later (3.18 or later for `--take-ownership`).
- A fork of PolicyStack, cloned locally. Argo CD reads your fleet files and values from it; upstream ships none.
- A hub that already runs OpenShift GitOps must use its default Argo CD instance, `openshift-gitops`.
- Protect `fleet/` before the first fleet file: whoever merges there builds and destroys clusters ([Protection](../reference/fleet.md#protection)).
- Supported floors: ACM 2.16, OpenShift 4.20, OpenShift GitOps 1.18. Argo CD renders with the Helm that GitOps bundles (3.18.4 in 1.18), and the validator evaluates ACM templates with the 2.16 engine. Cluster lifecycle has extra requirements ([Cluster lifecycle prerequisites](../guides/lifecycle.md#prerequisites)).

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

    These are the same objects the `acm` and `gitops` elements enforce, so [handover](#hand-the-operators-to-policystack) adopts them in place. The chart installs the channel head and refuses `versions` or `upgradeApproval` pins; pin after handover ([Operator versions](../guides/operators.md)).

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

        On a disconnected hub, also add the layer that sets `global.catalogSources`, such as `-f values/datacenters/<dc>.yaml` ([Disconnected clusters](../guides/disconnected.md#the-hub)).

=== "Hub that already runs ACM/GitOps"

    Skip this step. Your operators keep running as they are. Run the compatibility checks under [Hand the operators to PolicyStack](#hand-the-operators-to-policystack) before enabling the `acm` or `gitops` element.

## Install PolicyStack

Creates the ApplicationSets, the `pstack` policy namespace, the cluster import into Argo CD and its RBAC ([Install chart](../reference/install.md)).

```sh
helm install policystack ./install -n openshift-gitops \
  --set hubName=<hub> --set gitRepo=https://github.com/<org>/<fork>.git \
  --description "$(git rev-parse --short HEAD)"
```

Both values are required and stored in the release; upgrades reuse them. `--description` records the commit, which [upgrades](../guides/upgrading.md#install-chart) compare against.

Verify:

```sh
oc get applicationsets -n openshift-gitops
# NAME                    AGE
# policystack             10s
# policystack-lifecycle   10s
oc get secrets -n openshift-gitops -l argocd.argoproj.io/secret-type=cluster
# one secret per ManagedCluster, including the hub
```

If Helm reports an object that already exists, such as a hand-made `global` binding, rerun with `--take-ownership` (Helm 3.18 or later) to adopt it.

## Add the hub's fleet file

The hub gets Applications only once its fleet file is on the default branch ([The hub](../concepts/architecture.md#the-hub)):

```yaml
# fleet/hubs/<hub>.yaml
revision: main  # (1)!
valueFiles:  # (2)!
  - environments/prod.yaml
  - datacenters/dc1.yaml
  - acm/<hub>.yaml
```

1.  Branch, tag or SHA the hub's Applications render from ([Revision](../reference/fleet.md#revision)).
2.  Layers under `values/`, lowest precedence first ([Order](../concepts/values.md#order)). `values/clusters/<hub>.yaml` is always added last.

Merge it to the default branch. Within about 6 minutes, each element under `stack/` gets an Application named `<element>-<hub>`:

```sh
oc get applications.argoproj.io -n openshift-gitops | grep <hub>
# acm-<hub>      Synced   Healthy
# gitops-<hub>   Synced   Healthy
# ...
```

An element no layer enables syncs with no resources. Managed clusters follow the same pattern with `fleet/<cluster>.yaml` ([First cluster](first-cluster.md)).

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
    policies:
      instance:
        enabled: false  # (1)!
      link:
        enabled: false  # (2)!
```

1.  PolicyStack never reconciles the Argo CD instance that delivers it. A bad value there would break the path that fixes it.
2.  The console link depends on `instance`.

!!! warning
    Do not run [Install the operators](#install-the-operators) after handover. It resets any channel the hub layer overrides.

ACM and GitOps upgrades now follow the hub's fleet `revision` like any other element ([Operator versions](../guides/operators.md)).

## Teardown

```sh
oc delete applicationset policystack policystack-lifecycle -n openshift-gitops
```

!!! danger "Deletes every Policy"
    This deletes every Application and every Policy they rendered ([Removal and pruning](../concepts/removal.md)). Clusters this hub built keep running, unmanaged ([Abandon](../guides/lifecycle.md#abandon)). Add `--cascade=orphan` to keep the Applications and Policies.

Next: [First cluster](first-cluster.md).
