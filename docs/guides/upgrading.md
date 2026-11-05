# Upgrading PolicyStack

Each part of PolicyStack upgrades on its own path:

| Part | How |
|---|---|
| Elements, lifecycle, values | [Merge upstream](#merge-upstream) into the fork, then move fleet `revision` pins ([Rolling out changes](rollout.md)) |
| ACM and GitOps | Through the elements after [handover](../start/install.md#hand-the-operators-to-policystack). Before it, OLM upgrades within the installed channel. |
| The install chart | `helm upgrade` ([Install chart](#install-chart)) |

## Merge upstream

Your fleet pins your own tags. Fetch upstream without its tags, so a `v0.2.0` from upstream never shadows yours:

```sh
git remote add upstream https://github.com/PolicyStack/PolicyStack.git
git config remote.upstream.tagOpt --no-tags
git fetch upstream
```

Merge `upstream/main` through a PR. Its [Render Diff](../reference/ci.md#render-diff) comment shows the Policy changes for every cluster pinned to the default branch.

## Install chart

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
