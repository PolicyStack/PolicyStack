# PolicyStack
PolicyStack manages OpenShift cluster configuration as ACM policies. Each directory under `stack/` is an element: a Helm chart that renders `Policy` objects through the [policy-library](https://github.com/PolicyStack/PolicyStack-chart/tree/main/charts/policy-library) chart. On the ACM hub, an Argo CD ApplicationSet creates one Application per element for every managed cluster with a fleet file in Git, at the revision that file pins. Argo CD syncs the policies to the hub and ACM enforces them on each cluster.

Documentation: https://policystack.github.io/PolicyStack/ (source in [`docs/`](docs/)). Each element's settings are listed in `stack/<element>/README.md`.
[diagrams/architecture.drawio](diagrams/architecture.drawio) (open in [draw.io](https://app.diagrams.net/#Uhttps%3A%2F%2Fraw.githubusercontent.com%2FPolicyStack%2FPolicyStack%2Frefs%2Fheads%2Fmain%2Fdiagrams%2Farchitecture.drawio)) shows the whole process architecture diagram.

## Quick start

From the root of your fork, logged in to the hub as a cluster administrator ([Installation](https://policystack.github.io/PolicyStack/install/)):

1. Install ACM and OpenShift GitOps, unless the hub already runs them:
   ```sh
   until helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
         | oc apply -f - && oc get argocd/openshift-gitops -n openshift-gitops; do sleep 30; done
   oc wait mch/multiclusterhub -n open-cluster-management --for=jsonpath='{.status.phase}'=Running --timeout=20m
   ```
2. Install PolicyStack, naming the hub and your fork:
   ```sh
   helm install policystack ./install -n openshift-gitops \
     --set hubName=<hub> --set gitRepo=https://github.com/<org>/<fork>.git \
     --description "$(git rev-parse --short HEAD)"
   ```
3. Commit the hub's fleet file, `fleet/hubs/<hub>.yaml`, with `revision: main`.
4. Commit a fleet file for a managed cluster to the repo's default branch, `fleet/prod-east-1.yaml` for the ManagedCluster `prod-east-1` ([Fleet files](https://policystack.github.io/PolicyStack/applicationset/#fleet-files)):
   ```yaml
   revision: main
   valueFiles:
     - environments/prod.yaml
     - datacenters/dc1.yaml
     - platforms/aws.yaml
   ```

Every element ships disabled except `upgrade`, which acts only on a fleet file's `version` and `channel` ([Upgrades](https://policystack.github.io/PolicyStack/upgrades/)). Enable an element in a values layer, such as `values/clusters/prod-east-1.yaml` for the cluster above; see [Values cascade](https://policystack.github.io/PolicyStack/values/).

To have the hub build a hosted cluster instead of importing one, meet the [lifecycle prerequisites](https://policystack.github.io/PolicyStack/lifecycle/#prerequisites) and name the hub in the cluster's fleet file, `fleet/hcp-kubevirt.yaml`:
```yaml
revision: main
hub: acm-dc1
version: 4.20.8
valueFiles:
  - environments/nonprod.yaml
  - platforms/kubevirt.yaml
```
