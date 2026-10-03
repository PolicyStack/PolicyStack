# PolicyStack
PolicyStack manages OpenShift cluster configuration as ACM policies. Each directory under `stack/` is an element: a Helm chart that renders `Policy` objects through the [policy-library](https://github.com/PolicyStack/PolicyStack-chart/tree/main/charts/policy-library) chart. On the ACM hub, an Argo CD ApplicationSet creates one Application per element for every managed cluster with a fleet file in Git, at the revision that file pins. Argo CD syncs the policies to the hub and ACM enforces them on each cluster.

Documentation: https://policystack.github.io/PolicyStack/ (source in [`docs/`](docs/)). Each element's settings are listed in `stack/<element>/README.md`.
[diagrams/architecture.drawio](diagrams/architecture.drawio) (open in [draw.io](https://app.diagrams.net/#Uhttps%3A%2F%2Fraw.githubusercontent.com%2FPolicyStack%2FPolicyStack%2Frefs%2Fheads%2Fmain%2Fdiagrams%2Farchitecture.drawio)) shows the whole process architecture diagram.

## Quick start

On the ACM hub, with the [prerequisites](https://policystack.github.io/PolicyStack/install/#prerequisites) in place:

1. Import the managed clusters into Argo CD:
   ```sh
   oc apply -f gitops-prereq/
   ```
2. Set `hubName` and `gitRepo` in `appset/values.yaml`.
3. Install the ApplicationSet:
   ```sh
   helm install appset ./appset -f ./appset/values.yaml -f values.yaml
   ```
4. Commit a fleet file for a managed cluster to the repo's default branch, `fleet/prod-east-1.yaml` for the ManagedCluster `prod-east-1` ([Fleet files](https://policystack.github.io/PolicyStack/applicationset/#fleet-files)):
   ```yaml
   revision: main
   config:
     environment.10: prod
     datacenter.20: dc1
     platform.30: aws
   ```

Every element ships disabled. Enable it in a values layer, such as `values/clusters/prod-east-1.yaml` for the cluster above; see [Values cascade](https://policystack.github.io/PolicyStack/values/).
