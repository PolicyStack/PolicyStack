# PolicyStack
PolicyStack manages OpenShift cluster configuration as ACM policies. Each directory under `stack/` is an element: a Helm chart that renders `Policy` objects through the [policy-library](https://github.com/PolicyStack/PolicyStack/tree/main/lib/policy-library) chart in `lib/`. On the ACM hub, an Argo CD ApplicationSet creates one Application per element for every managed cluster with a fleet file in Git, at the revision that file pins. Argo CD syncs the policies to the hub and ACM enforces them on each cluster.

Documentation: https://policystk.io/ (source in [`docs/`](docs/)). Each element's settings are listed in `stack/<element>/README.md`.
[docs/diagrams/architecture.drawio](docs/diagrams/architecture.drawio) (open in [draw.io](https://app.diagrams.net/#Uhttps%3A%2F%2Fraw.githubusercontent.com%2FPolicyStack%2FPolicyStack%2Frefs%2Fheads%2Fmain%2Fdocs%2Fdiagrams%2Farchitecture.drawio)) shows the whole process architecture diagram.

## Quick start

To use PolicyStack, create your own repository from one release `vX.Y.Z` with the launcher, `tools/policystack`. You need git and [mise](https://mise.jdx.dev/installing-mise.html); mise installs the rest. Install on the hub from your repository ([Installation](https://policystk.io/start/install/)):

1. Fetch only the launcher from a release, and create the repository with it:
   ```sh
   git clone --depth 1 --filter=blob:none --sparse --branch v1.0.0 \
     https://github.com/PolicyStack/PolicyStack.git ~/policystack
   git -C ~/policystack sparse-checkout set tools/policystack
   cd ~/policystack && mise trust && mise install
   mise run ps -- install ~/my-fleet
   cd ~/my-fleet && mise trust && mise install
   ```
   The launcher asks for the release and the elements; `acm`, `gitops` and `upgrade` are always included. Push the repository somewhere Argo CD can read. Take later releases with `mise run update`, from your repository ([Upgrading PolicyStack](https://policystk.io/guides/upgrading/)).
2. Install ACM and OpenShift GitOps, unless the hub already runs them:
   ```sh
   until helm template install/operators -f stack/acm/values.yaml -f stack/gitops/values.yaml \
         | oc apply -f - && oc get argocd/openshift-gitops -n openshift-gitops; do sleep 30; done
   oc wait mch/multiclusterhub -n open-cluster-management --for=jsonpath='{.status.phase}'=Running --timeout=20m
   ```
3. Install PolicyStack, naming the hub and your repository:
   ```sh
   helm install policystack ./install -n openshift-gitops \
     --set hubName=<hub> --set gitRepo=https://github.com/<org>/<repo>.git
   ```
4. Commit the hub's fleet file, `fleet/hubs/<hub>.yaml`, with `revision: main`.
5. Commit a fleet file for a managed cluster to the repo's default branch, `fleet/prod-east-1.yaml` for the ManagedCluster `prod-east-1` ([Fleet files](https://policystk.io/reference/fleet/)):
   ```yaml
   revision: main
   valueFiles:
     - environments/prod.yaml   # a layer you write under values/
   ```

Every element ships disabled except `upgrade`, which acts only on a fleet file's `version` and `channel` ([Upgrades](https://policystk.io/guides/upgrades/)). Enable an element in a values layer, such as `values/clusters/prod-east-1.yaml` for the cluster above; see [Values cascade](https://policystk.io/concepts/values/).

To have the hub build a hosted cluster instead of importing one (a preview feature, see [Compatibility](https://policystk.io/reference/compatibility/#maturity)), meet the [lifecycle prerequisites](https://policystk.io/guides/lifecycle/#prerequisites) and name the hub in the cluster's fleet file, `fleet/hcp-kubevirt.yaml`:
```yaml
revision: main
hub: acm-dc1
version: 4.20.8
valueFiles:              # layers you write, or copy from upstream's values/
  - environments/nonprod.yaml
  - platforms/kubevirt.yaml
```
