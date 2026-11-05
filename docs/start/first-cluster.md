# First cluster

Onboard an imported cluster and turn on an element. The hub must already run PolicyStack and have its fleet file ([Installation](install.md)). The examples use `prod-east-1`.

## Onboard the cluster

1. Import the cluster into ACM. The GitOpsCluster adds it to Argo CD ([Install PolicyStack](install.md#install-policystack)).
2. Optional: add `values/clusters/prod-east-1.yaml` if this cluster differs from its layers. [Enable an element](#enable-an-element) adds one.
3. PR `fleet/prod-east-1.yaml` to the default branch ([Fleet files](../reference/fleet.md)):

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main # (1)!
    valueFiles: # (2)!
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/aws.yaml
    ```

    1.  Branch, tag or SHA.
    2.  Under `values/`, lowest precedence first. `values/clusters/prod-east-1.yaml` is added last ([Order](../concepts/values.md#order)).

!!! warning "Name both files exactly like the ManagedCluster"
    A misnamed values file is skipped silently ([Missing files](../concepts/values.md#missing-files)). Commit it at the revision the fleet file pins (with the fleet file PR when that is the default branch), or the first render leaves it out and can briefly enable an element it turns off.

Within about 6 minutes there is one Application per element, named `<element>-<cluster>`. An element no layer enables still gets an Application, with no resources.

```sh
oc get applications.argoproj.io -n openshift-gitops | grep prod-east-1
```

## Enable an element

Commit to the revision the cluster pins. Here, turning on `gitops`:

```diff
# values/clusters/prod-east-1.yaml
 stack:
+  gitops:
+    enabled: true
```

Clusters pinned to other revisions are unaffected. Argo CD renders the element and syncs its policies into `pstack` on the hub; ACM delivers them to the cluster ([Architecture](../concepts/architecture.md)).

## Check compliance

`enforce` changes the cluster, `inform` only reports. Compliance shows on the replicated Policy in the cluster's namespace on the hub, and under Governance in the ACM console:

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

## Next steps

- [Rolling out changes](../guides/rollout.md): promote, roll back, remove an element.
- [Cluster lifecycle](../guides/lifecycle.md): let the hub build the cluster. Skip the import and add `hub`, `version` and `install` to the fleet file.
