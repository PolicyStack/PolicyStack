# Rolling out changes

A cluster runs what Git holds at the revision its [fleet file](applicationset.md#fleet-files) pins: the element charts, their policy-library pins and every values file. To roll out a change, commit it and point fleet files at a revision that contains it.

## Onboarding a cluster

1. Import the cluster into ACM. The GitOpsCluster then imports it into Argo CD ([Import clusters into Argo CD](install.md#import-clusters-into-argo-cd)).
2. Open a pull request to the default branch that adds `fleet/<cluster>.yaml`, named exactly like the ManagedCluster ([Fleet files](applicationset.md#fleet-files)). The hub's file is `fleet/hubs/<hubName>.yaml` ([The hub](applicationset.md#the-hub)).

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main
    config:
      environment.10: prod
      datacenter.20: dc1
      platform.30: aws
    ```

3. Add `values/clusters/<cluster>.yaml`, named exactly like the ManagedCluster, at the revision the fleet file pins. When that is the default branch, add it in the same pull request. It enables the elements this cluster runs beyond what its other layers enable. A misnamed file is skipped without an error ([Missing files](values.md#missing-files)). The hub uses `acm-<datacenter>` file names instead ([Order](values.md#order)).

Import and the pull request can come in either order. Land step 3 before or with step 2: Applications rendered before the cluster's values file exists leave that layer out, which can briefly enable an element the file turns off. Once the cluster is imported and its fleet file is on the default branch, the ApplicationSet creates one Application per element, named `<element>-prod-east-1`, in `openshift-gitops`, within about 6 minutes. An element no layer enables syncs with no resources.

```sh
oc get applications.argoproj.io -n openshift-gitops | grep prod-east-1
```

## Promoting and rolling back

A cluster pinned to a branch picks up every commit pushed to it. A cluster pinned to a tag or SHA changes only when its fleet file's `revision` changes. Promote with a pull request that edits `revision`:

```diff
 # fleet/prod-east-1.yaml
-revision: v1.4.0
+revision: v1.5.0
 config:
```

Roll back with `git revert` of that commit, or with a pull request that sets the previous revision. Both go through the same review as any change to `fleet/` ([Protection](applicationset.md#protection)).

Every Application for the cluster switches to the new revision. Application names do not include the revision, so Argo CD updates the same Policies in place. The [Placement](policies.md#placement) selector matches only the cluster's `name` label, so the policies stay bound to the cluster while Argo CD syncs.

An element that exists at the new revision but not the old one gets a new Application. An element missing at the new revision loses its Application, and its Policies stay behind ([Removing an element](#removing-an-element)).

## Disabling an element

Set `enabled: false` under the element's key in a layer that outranks every layer enabling it ([Order](values.md#order)). `values/environments/prod.yaml` enables `openshiftDns`, to turn it off on `prod-east-1` only, add this to `values/clusters/prod-east-1.yaml`:

```yaml
stack:
  openshiftDns:
    enabled: false
```

To turn off one part of an element instead, set its [toggle](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles).

Once the change reaches the cluster's revision, the element renders nothing. The ApplicationSet leaves `allowEmpty` unset, so automated sync refuses to prune every object of an Application. The Application goes OutOfSync and its policies keep enforcing. Sync it once with pruning, from the Argo CD UI or CLI:

```sh
argocd app sync openshift-dns-prod-east-1 --prune
```

Argo CD then deletes the element's objects from `policy`, and ACM removes its policies from the managed cluster. The objects those policies created stay on the managed cluster:

- An enforced ConfigurationPolicy deletes its objects only when it sets `pruneObjectBehavior`: `DeleteIfCreated` removes the objects it created, `DeleteAll` every object it manages. ACM's default is `None`. The chart passes `configPolicies[].pruneObjectBehavior` through, set it and let it sync before disabling the element.
- Deleting an OperatorPolicy leaves the operator installed.

## Removing an element

The ApplicationSet sets `preserveResourcesOnDeletion`, and its Applications carry no resources finalizer. Deleting an Application therefore leaves the element's objects in `policy`, its Policies stay bound to the cluster and keep enforcing, with nothing managing them. An Application is deleted when:

- `stack/<element>` does not exist at the cluster's revision.
- The cluster's fleet file is deleted ([Offboarding a cluster](#offboarding-a-cluster)).
- The ApplicationSet is deleted, for example by `helm uninstall appset`.

To remove an element:

1. Set `enabled: false` in the element's `values.yaml`, and remove every `enabled: true` for it from the root `values.yaml` and `values/`.
2. Move every cluster to a revision with that change, and sync each of the element's Applications with pruning ([Disabling an element](#disabling-an-element)) until it is Synced with no resources.
3. Delete `stack/<element>` in a later commit. Clusters pinned to older revisions keep the directory and its Application until their pin moves.

To clean up objects already orphaned, delete them from `policy` by name ([Rendered objects](policies.md#rendered-objects)).

## Offboarding a cluster

Deleting a cluster's fleet file deletes all of its Applications. Their Policies stay and keep enforcing ([Removing an element](#removing-an-element)). To stop managing a cluster without orphaning its Policies:

1. Set `enabled: false` in `values/clusters/<cluster>.yaml` for every element enabled on it, and sync those Applications with pruning ([Disabling an element](#disabling-an-element)).
2. Delete the cluster's fleet file in a pull request.

Do not reuse the cluster's name while its fleet file exists. A cluster imported under that name gets the old file's configuration.

## Operator upgrades

Each `operatorPolicies[]` entry renders an OperatorPolicy ([Operator Policy Options](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#operator-policy-options)). `upgradeApproval` and `versions` decide upgrades:

| `upgradeApproval` | `versions` | Effect when enforced |
|---|---|---|
| `Automatic` | empty | The Subscription is set to automatic approval. OLM installs every upgrade the channel offers. |
| `Automatic` | listed | The Subscription is set to manual approval. The policy approves an InstallPlan only when the CSV it installs is in `versions` or equals `subscription.startingCSV`. |
| `None` | either | The policy approves the initial install only, subject to `versions`. It never approves an upgrade. |

An informing policy approves nothing, it reports an installed CSV missing from a non-empty `versions` as noncompliant.

The metallb element ships with `upgradeApproval: Automatic` and no `versions`. To control its upgrades, add `versions` to its entry in [`stack/metallb/values.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/stack/metallb/values.yaml):

```yaml
stack:
  metallb:
    operatorPolicies:
      - name: metallb
        # other fields unchanged
        upgradeApproval: Automatic
        versions:
          - <installed CSV>
```

`oc get csv -n metallb-system` on the managed cluster shows the installed CSV. On a cluster that does not have the operator yet, OLM resolves the Subscription to the channel's latest CSV. If that CSV is not in `versions`, the policy does not approve the install. To install a listed version there, set `subscription.startingCSV` to it.

When a newer CSV is available, the OperatorPolicy status names it in a message that starts `an InstallPlan to update to [<csv>]`. Add that CSV name to `versions`, then promote the revision that has it one cluster at a time, by editing each cluster's fleet file ([Promoting and rolling back](#promoting-and-rolling-back)).

Keep `versions` in the element's `values.yaml`. `operatorPolicies` is a list, and lists replace across the cascade ([Merge rules](values.md#merge-rules)), so overriding `versions` from another layer means restating the whole list there.