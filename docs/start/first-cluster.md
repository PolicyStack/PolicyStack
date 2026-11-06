# First cluster

Onboard an imported cluster and turn on one element. Onboarding is four hand-offs:

1. You import the cluster into ACM. The GitOpsCluster from [Installation](install.md#install-policystack) creates an Argo CD cluster secret for it.
2. You merge `fleet/<cluster>.yaml` to the default branch. It enrolls the cluster and names the revision and values layers it reads.
3. The `policystack` ApplicationSet creates one Application per element for the cluster. Each renders the element's Policies into `pstack` on the hub.
4. ACM copies each Policy to the cluster, where ACM's governance add-on (installed at import) applies it and reports compliance.

More: [How a change reaches a cluster](../concepts/overview.md#how-a-change-reaches-a-cluster).

PolicyStack must be [installed](install.md) on the hub. The hub doesn't need its own fleet file for this. The examples use `prod-east-1`; replace it with your cluster's name.

## Onboard the cluster

1.  Import the cluster into ACM ([Importing a cluster](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html/clusters/cluster_mce_overview#importing-a-target-managed-cluster-to-the-hub-cluster)). Use its ManagedCluster name everywhere below. Pick a name of 30 characters or fewer before importing. A ManagedCluster can't be renamed, and with a longer name some replicated Policy names (`pstack.<policy>-<element>-<cluster>`) exceed 63 characters, which ACM rejects, so those Policies never reach the cluster ([Naming limits](../concepts/policies.md#naming-limits)).

    Once the import finishes, ACM sets the `clusterID` label, and the cluster gets its Argo CD secret. A cluster without the label gets no Applications and no error. ACM sets `clusterID` from the OpenShift ClusterVersion once the cluster joins (AVAILABLE `True`). If it's missing, the import hasn't finished: check the conditions in `oc get managedcluster prod-east-1 -o yaml`.

    ```sh
    oc get managedcluster prod-east-1 -L clusterID
    # NAME          HUB ACCEPTED   ...   AVAILABLE   AGE   CLUSTERID
    # prod-east-1   true           ...   True        5m    1b2c3d4e-...
    oc get secrets -n openshift-gitops -l argocd.argoproj.io/secret-type=cluster,name=prod-east-1
    # one secret
    ```

2.  PR the fleet file to the default branch. For this walkthrough, list no layers. Merge once CI passes:

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main  # (1)!
    ```

    1.  Where `stack/` and `values/` are read. A branch makes the cluster follow every merge to it. A tag or SHA holds it there until you change this line, which is how changes are promoted cluster by cluster ([Rolling out changes](../guides/rollout.md#promoting-and-rolling-back)). Use `main` for a first or lab cluster.

    The fleet file itself is always read from the default branch, because it says which revision to read everything else at ([Fleet files](../concepts/overview.md#fleet-files)). Name it exactly like the ManagedCluster: a misnamed fleet file matches nothing and reports no error.

    CI rejects an invalid fleet file or a revision missing a listed layer (POLICY050, POLICY052), and Render Diff comments the Policies the merge adds ([Protect the repository](install.md#protect-the-repository)).

    ???+ example "Adding shared layers"
        List layers under `valueFiles`, lowest precedence first ([Values cascade](../concepts/values.md#order)):

        ```yaml
        # fleet/prod-east-1.yaml
        revision: main
        valueFiles:
          - environments/prod.yaml  # (1)!
          - platforms/aws.yaml      # (2)!
        ```

        1.  A layer you wrote, or copied from upstream's examples ([Example files](../concepts/values.md#example-files)). Upstream's `environments/prod.yaml` turns elements on and enforces them. Read a layer before you list it.
        2.  Picks the AWS variant of the node elements.

        Helm merges the element's `values.yaml`, the root `values.yaml`, each listed layer, then `values/clusters/prod-east-1.yaml`. Later files win: maps merge key by key, lists are replaced whole ([Merge rules](../concepts/values.md#merge-rules)).

        !!! warning "Land the cluster file before the fleet file"
            If `values/clusters/prod-east-1.yaml` turns off something a listed layer turns on, it must already be at the revision when the fleet file merges. Otherwise the first render enforces the element, and turning it off afterwards deletes the Policy but not what it changed ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)). With `remediations` from a copy of upstream's `environments/prod.yaml`, the Samples Operator stays `Removed`.

            - `revision: main`: put both files in one PR.
            - Tag or SHA: merge the cluster file to `main`, tag, then PR the fleet file pinning the new tag.

            A misnamed cluster file is skipped silently ([Missing files](../concepts/values.md#missing-files)).

## Check the Applications

Within about 6 minutes each directory under `stack/` has an Application named `<element>-<cluster>`. The ApplicationSet lists directories and can't read values, so an element no layer turns on still gets an Application. It renders nothing and shows Synced and Healthy.

```sh
oc get applications.argoproj.io -n openshift-gitops | grep prod-east-1  # (1)!
# acm-prod-east-1           Synced        Healthy
# cert-manager-prod-east-1  Synced        Healthy
# ...
```

1.  ACM installs a second `applications` kind, so use the full name.

??? failure "No Applications"
    Check in order:

    1. The ManagedCluster has the `clusterID` label and a cluster secret exists (step 1).
    2. `fleet/prod-east-1.yaml` is on the default branch, named exactly like the ManagedCluster.
    3. The ApplicationSet's error. A YAML error, a missing `revision` or a revision that doesn't exist stops Application changes for every cluster on the hub ([One bad fleet file stops the hub](../concepts/overview.md#one-bad-fleet-file-stops-the-hub)):

        ```sh
        oc get applicationset policystack -n openshift-gitops \
          -o jsonpath='{.status.conditions[?(@.type=="ErrorOccurred")].message}'
        ```

## Enable an element

Each element's README lists its values key, settings and Policies: [`stack/gitops/README.md`](https://github.com/PolicyStack/PolicyStack/blob/main/stack/gitops/README.md) for `gitops` ([Reading an element's README](../concepts/elements.md#reading-an-elements-readme)). On `prod-east-1`, `gitops` installs the OpenShift GitOps operator, configures that cluster's own Argo CD instance in `openshift-gitops`, and adds a console link. That Argo CD is for the cluster's workloads; the hub's Argo CD still runs PolicyStack.

The key is the element's chart name in camelCase under `stack` (`gitops` here, `userMonitoring` for `user-monitoring`), because Go templates can't read a key containing `-` with dot syntax ([Anatomy](../concepts/elements.md#anatomy)). Create the cluster file if it's missing. Its name must match the ManagedCluster exactly, or it is skipped silently:

```yaml
# values/clusters/prod-east-1.yaml
stack:
  gitops:
    enabled: true
```

!!! warning "Enabling enforces at once"
    Shipped elements default to `enforce`, so the operator installs as soon as the change syncs. To see what an element would do first, add a dry run to the same file. Every Policy for this cluster then reports only, until you remove it ([Dry run](../guides/rollout.md#dry-run)):

    ```yaml
    global:
      remediationAction: inform
    ```

    Under the dry run expect `install` NonCompliant (operator not installed) and `instance` and `link` Pending, since they wait on `install`; the Application shows Degraded because some Policies aren't Compliant ([Check compliance](#check-compliance)). Remove the key to enforce ([What a dry run can't show](../guides/rollout.md#dry-run)).

Turning the element off later deletes its Policies but leaves the operator and Argo CD instance on the cluster ([What happens on the managed cluster](../concepts/removal.md#what-happens-on-the-managed-cluster)).

Commit where the cluster reads it. With a branch pin, PR to that branch. With a tag or SHA, merge to `main`, tag, and move `revision` in the fleet file. A change reaches only clusters that read the file and pin a revision containing it, so this one reaches only `prod-east-1`. A shared layer such as `environments/prod.yaml` reaches every cluster that lists it.

### What you created

Application `gitops-prod-east-1` renders into `pstack`:

| Object | Name | From |
|---|---|---|
| Policy | `install-gitops-prod-east-1` | `operatorPolicies.gitops` in the element's values: the operator |
| Policy | `instance-gitops-prod-east-1` | `converters/instance/`: the Argo CD instance and its CA bundle. Waits for `install`. |
| Policy | `link-gitops-prod-east-1` | `converters/link/`: the console link. Waits for `instance`. |
| Placement, PlacementBinding | `gitops-prod-east-1` | Selects the cluster whose `name` label is `prod-east-1` |

A Policy is named `<policy>-<element>-<cluster>` ([Rendered names](../concepts/elements.md#rendered-names)). ACM copies each into the hub namespace named after the cluster as `pstack.<name>`. That copy is what the cluster applies, and where its compliance is reported ([How a Policy reaches a cluster](../concepts/acm.md#how-a-policy-reaches-a-cluster)).

## Check compliance

```sh
oc get applications.argoproj.io gitops-prod-east-1 -n openshift-gitops
# NAME                 SYNC STATUS   HEALTH STATUS
# gitops-prod-east-1   Synced        Healthy
oc get policies.policy.open-cluster-management.io -n prod-east-1
# NAME                                 REMEDIATION ACTION   COMPLIANCE STATE   AGE
# pstack.install-gitops-prod-east-1    enforce              Compliant          3m
# pstack.instance-gitops-prod-east-1   enforce              Compliant          3m
# pstack.link-gitops-prod-east-1       enforce              Compliant          3m
```

Synced means the Policies on the hub match Git. Health is Argo CD's summary of their compliance: Healthy when every Policy is Compliant, Degraded when one is NonCompliant or Pending, Progressing until ACM reports a status. The ACM console shows the same under **Governance**. `enforce` changes the cluster to match; `inform` only reports ([Compliance states](../concepts/acm.md#compliance-states)).

On the cluster itself:

```sh
oc --context prod-east-1 get csv -n openshift-gitops-operator   # PHASE Succeeded
oc --context prod-east-1 get argocd -n openshift-gitops         # the instance
```

??? note "Pending"
    A Policy waiting on another is Pending. `instance` waits until `install` is Compliant, which happens once the operator has installed. `link` waits for `instance`. Expect a few minutes. Pending for more than about 10 minutes means `install` is not Compliant; read its message as below.

??? failure "NonCompliant"
    Read each template's last message on the cluster's copy:

    ```sh
    oc get policies.policy.open-cluster-management.io pstack.instance-gitops-prod-east-1 -n prod-east-1 \
      -o jsonpath='{range .status.details[*]}{.templateMeta.name}{": "}{.history[0].message}{"\n"}{end}'
    ```

??? failure "Application shows ComparisonError"
    The render failed, so Argo CD syncs and prunes nothing for that Application ([When an Application fails](../concepts/architecture.md#when-an-application-fails)). A misspelled key fails the element's schema:

    ```sh
    oc get applications.argoproj.io gitops-prod-east-1 -n openshift-gitops -o jsonpath='{.status.conditions[*].message}'
    # ... values don't meet the specifications of the schema(s) in the following chart(s):
    # stack:
    # - gitops: Additional property enabeld is not allowed
    ```

## Next steps

- [How an element is built](../concepts/elements.md): where Policies and their names come from.
- [Rolling out changes](../guides/rollout.md): promote, roll back, remove an element.
- [Building clusters](../guides/lifecycle.md): let the hub build the cluster. Skip the import and add `hub`, `version` and `install` to the fleet file.
