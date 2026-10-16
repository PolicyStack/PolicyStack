# Installation

PolicyStack runs on the ACM hub. Everything in [`install/`](https://github.com/PolicyStack/PolicyStack/tree/main/install) is run once per hub; Argo CD reads nothing there. Run the commands from the root of your fork, logged in to the hub as a cluster administrator.

## Prerequisites

- `oc`, and `helm` 3.14 or later.
- On a hub that already runs OpenShift GitOps: its default Argo CD instance, `openshift-gitops`. The install chart grants its application controller access and the instance runs the ApplicationSets.
- A fork of PolicyStack. Argo CD reads fleet files and values from it, and upstream ships none. A private fork needs an [Argo CD repository secret](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repositories) in `openshift-gitops`.
- On a hub that builds clusters from fleet files: ACM 2.16 or later and the rest of [Cluster lifecycle prerequisites](lifecycle.md#prerequisites).

## Install the operators

Skip this on a hub that already runs ACM and OpenShift GitOps ([Existing hubs](#existing-hubs)).

```sh
until helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
      | oc apply -f - && oc get argocd/openshift-gitops -n openshift-gitops; do sleep 30; done
oc wait mch/multiclusterhub -n open-cluster-management --for=jsonpath='{.status.phase}'=Running --timeout=20m
oc wait argocd/openshift-gitops -n openshift-gitops --for=jsonpath='{.status.phase}'=Available --timeout=10m
```

The `install/operators` chart renders the Namespace, OperatorGroup and Subscription for each operator, plus the MultiClusterHub, from the `acm` and `gitops` element values. They are the objects those elements enforce, so [handing the operators over](#hand-the-operators-to-policystack) later adopts them in place. The chart installs the channel head and fails on a `versions` or `upgradeApproval` pin; pin after handover ([Operator upgrades](rollout.md#operator-upgrades)). The loop re-applies until ACM installs the MultiClusterHub CRD and the GitOps operator creates its default Argo CD instance. The MultiClusterHub takes about 10 minutes. A loop that never finishes is a real error, such as an unreachable catalog: stop it and read the `oc` output.

To override a value, such as `stack.acm.config.availabilityConfig: Basic` on a single node hub, add `-f values/acm/<hub>.yaml`.

### Existing hubs

The `acm` and `gitops` elements adopt an operator only when its namespace, its OperatorGroup name and its Subscription name, which must equal the package, match the element. Check before enabling either element:

```sh
oc get subscriptions.operators.coreos.com -A \
  -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,PACKAGE:.spec.name,CHANNEL:.spec.channel \
  | grep -E 'advanced-cluster-management|openshift-gitops-operator'
oc get operatorgroups -n <NS>
```

- NAME differs from PACKAGE: leave that element off. The OperatorPolicy would create a second Subscription.
- NS differs from the element's, such as GitOps in `openshift-operators`: set `stack.<acm|gitops>.operatorPolicies.<acm|gitops>.namespace` and `operatorGroup.name` to match, or leave that element off. Otherwise OLM gets a second install of the operator.
- A console-created OperatorGroup, `<ns>-xxxxx`: set `stack.<acm|gitops>.operatorPolicies.<acm|gitops>.operatorGroup.name` to it in `values/acm/<hub>.yaml`.
- An older channel: set `subscription.channel` to it, then move one minor at a time.

## Install PolicyStack

```sh
helm install policystack ./install -n openshift-gitops \
  --set hubName=<hub> --set gitRepo=https://github.com/<org>/<fork>.git \
  --description "$(git rev-parse --short HEAD)"
```

`hubName` names the hub's fleet file, `fleet/hubs/<hubName>.yaml`, and its Applications ([The hub](applicationset.md#the-hub)). Every hub reading the same repo, including a disaster recovery hub, needs its own. Both values are required and stored in the release, so they are typed only here.

The chart creates:

| Object | Name | Namespace | Purpose |
|---|---|---|---|
| ApplicationSet | `policystack`, `policystack-lifecycle` | `openshift-gitops` | Applications per element and cluster, and per built cluster ([Clusters and Applications](applicationset.md)) |
| Namespace | `policy` | | Holds the Policies |
| ManagedClusterSetBinding | `global` | `openshift-gitops`, `policy` | Lets the Placements select every cluster |
| Placement | `global` | `openshift-gitops` | Every cluster, tolerating `unreachable` and `unavailable` |
| GitOpsCluster | `argo-acm-importer-global` | `openshift-gitops` | Creates an Argo CD cluster secret for each cluster the Placement selects |
| ClusterRole, ClusterRoleBinding | `openshift-gitops-policy-admin` | | Lets the Argo CD application controller manage Policies, PolicySets, Placements and PlacementBindings, the only objects elements render |

Every object carries `helm.sh/resource-policy: keep`, so `helm uninstall policystack` leaves PolicyStack running. If Helm reports an object that already exists, such as a hand-made `global` binding, add `--take-ownership` (Helm 3.18 or later).

Each managed cluster then has an Argo CD cluster secret:

```sh
oc get secrets -n openshift-gitops -l argocd.argoproj.io/secret-type=cluster
```

Next: add a fleet file for the hub and each managed cluster ([Onboarding a cluster](rollout.md#onboarding-a-cluster)).

## Hand the operators to PolicyStack

Once the stack is proven on the hub, enable the elements in `values/acm/<hub>.yaml`, and list `acm/<hub>.yaml` under `valueFiles` in `fleet/hubs/<hub>.yaml` ([The hub](applicationset.md#the-hub)):

```yaml
stack:
  acm:
    enabled: true
  gitops:
    enabled: true
    toggles:
      instance: false
      link: false
```

ACM and GitOps upgrades then follow the hub's fleet `revision` like any other element ([Operator upgrades](rollout.md#operator-upgrades)). `instance` stays off so PolicyStack never reconciles the Argo CD instance that delivers it, and `link` depends on it. Do not run [Install the operators](#install-the-operators) again afterwards: it would reset a channel the hub layer overrides.

## Upgrading

| Part | How |
|---|---|
| Elements, lifecycle, values | Merge upstream into the fork, then move fleet `revision` pins ([Rolling out changes](rollout.md)) |
| ACM and GitOps | Through the elements after [handover](#hand-the-operators-to-policystack). Before it, OLM upgrades within the installed channel |
| The install chart | `helm upgrade`, below |

The ApplicationSets are not pinned per cluster: a change under `install/templates/` reaches every cluster on the hub. Upgrade only when `git diff <last>..HEAD -- install/` is not empty, where `<last>` is the DESCRIPTION of the deployed revision in `helm history policystack -n openshift-gitops`. After `Rollback to <n>`, it is revision `<n>`'s.

```sh
helm upgrade policystack ./install -n openshift-gitops --reset-then-reuse-values \
  --description "$(git rev-parse --short HEAD)"
```

`--reset-then-reuse-values` takes the new chart defaults and keeps `hubName` and `gitRepo`. Roll back with `helm rollback policystack <revision> -n openshift-gitops`.

### From the appset chart

Hubs installed with the old `appset` release and `gitops-prereq/` move over once, with Helm 3.18 or later. First check that the release's stored manifest keeps all four objects; the ApplicationSets gained `keep` on 2026-10-13. Below 4, run `helm upgrade appset` once from a commit before `install/` existed, with its usual values.

```sh
helm get manifest appset -n <ns> | grep -c 'helm.sh/resource-policy'   # 4
helm get values appset -n <ns> -o yaml > appset-values.yaml
helm uninstall appset -n <ns>
helm install policystack ./install -n openshift-gitops --take-ownership -f appset-values.yaml \
  --description "$(git rev-parse --short HEAD)"
oc get applicationset policystack -n openshift-gitops \
  -o jsonpath='{.metadata.annotations.meta\.helm\.sh/release-name}'   # policystack
```

Reusing the stored values keeps `hubName` exact; a different one renames and recreates every hub Application. Nothing is deleted. A hub whose ApplicationSet still had `preserveResourcesOnDeletion: true` gains the resources finalizer on its Applications, and an Application left OutOfSync on a disabled element prunes on its next automated sync.

Then drop the cluster-admin binding the old install step created. Skip this if the `openshift-gitops` instance also syncs Applications outside PolicyStack, since they may rely on it. Check the new role first, then remove only the cluster-admin binding:

```sh
oc auth can-i create policies.policy.open-cluster-management.io -n policy \
  --as=system:serviceaccount:openshift-gitops:openshift-gitops-argocd-application-controller   # yes
oc get clusterrolebinding -o wide | grep 'ClusterRole/cluster-admin' | grep openshift-gitops-argocd-application-controller
oc adm policy remove-cluster-role-from-user cluster-admin -z openshift-gitops-argocd-application-controller \
  -n openshift-gitops --rolebinding-name=<name>
```

Without `--rolebinding-name`, the last command edits every cluster-admin binding. Never delete `openshift-gitops-policy-admin`; nothing recreates it before the next `helm upgrade`. Afterwards, sync the Applications and check that they stay Synced.

## Teardown

```sh
oc delete applicationset policystack policystack-lifecycle -n openshift-gitops
```

This deletes every Application and every Policy they rendered ([Removing an element](rollout.md#removing-an-element)). Every cluster this hub built keeps running, unmanaged ([Abandon](lifecycle.md#abandon)). Add `--cascade=orphan` to keep the Applications and Policies.
