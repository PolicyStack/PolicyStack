# Upgrading PolicyStack

Argo CD reads your fork, never upstream (`PolicyStack/PolicyStack`) ([The repository](../concepts/overview.md#the-repository)). Upgrading means merging upstream's `main` into your fork's default branch, then rolling the result out like any other change. Upstream's `v0.x.y` release tags only mark points on `main` and carry notes listing the merged PRs; read them before you merge, but merge `main`, not a tag.

Each part reaches your clusters on its own path:

| Part | Delivered by | Reaches clusters |
|---|---|---|
| Elements, `lifecycle/`, [policy-library](../concepts/elements.md) (`lib/`) | Argo CD, from your fork at each fleet file's `revision` | Clusters pinned to the default branch, often the hub: within minutes of the merge. Clusters pinned to a tag or SHA: when you [move the pin](#roll-it-out). |
| Validator and CI | GitHub Actions in your fork | On the merge PR itself |
| ACM and GitOps operators | The `acm` and `gitops` elements, after [handover](../start/install.md#hand-the-operators-to-policystack) | Like any element, at the hub's `revision` ([ACM and GitOps](#acm-and-gitops)) |
| Install chart (`install/`) | `helm upgrade`, run by you on each hub | When you run it, for every cluster on that hub at once ([Install chart](#install-chart)) |

A merge can carry:

- Changes to policy-library in `lib/policy-library`. Argo CD renders each element with the library from the same revision, so a cluster gets them when its `revision` reaches the merge, together with the element changes that need them. Nothing to mirror: the library comes with your fork.
- Element default changes: operator channels (`stack/acm/values.yaml` ships `release-2.17`), newly enabled policies, renamed or removed values keys.
- New validator rules, which apply to your fleet and values on the merge PR.
- Edits to upstream's example `values/` files ([Example files](#example-values-files)).

## Merge upstream

Once per clone, add upstream without its tags:

```sh
git remote add upstream https://github.com/PolicyStack/PolicyStack.git
git config remote.upstream.tagOpt --no-tags
```

`git fetch` otherwise copies upstream's tags into your clone, and a later `git push --tags` publishes them to your fork. An upstream `v0.2.0` then takes that name in your fork: a fleet `revision: v0.2.0` renders upstream's commit, and your own `git tag v0.2.0` fails.

Then, for each upgrade:

```sh
git fetch origin && git fetch upstream
git switch -c merge-upstream origin/main
git merge upstream/main     # resolve conflicts, then git commit
git push -u origin merge-upstream
```

Open a PR with a Conventional Commit title such as `chore: merge upstream`, which the `pr-title` check requires ([CI workflows](../reference/ci.md)).

- The `ci` check runs upstream's new validator against your fleet and values. Fix new findings in the same PR, for example [POLICY035](../reference/rules.md) for a layer key an element no longer has.
- The [Render Diff](../reference/ci.md#render-diff) comment shows the Policy changes for every cluster pinned to the default branch. Tag-pinned clusters show theirs on the PR that moves their pin.
- Conflicts in `.github/CODEOWNERS`: keep your lines.

!!! danger "Merge commit, never squash"
    A squash commit has no upstream parent, so the next `git merge upstream/main` starts from the old merge base and conflicts again on everything already merged. If your fork's settings allow only squash merges, allow merge commits.

### Example values files

Upstream ships example layers (`values/environments/prod.yaml`, `values/datacenters/dc1.yaml`, `values/clusters/prod-east-1.yaml` and others). If your real clusters use the same paths, upstream's edits to them merge into your config, silently when they don't conflict. Keep your real layers at paths upstream does not ship, or delete the examples: an upstream change to a deleted file then stops the merge as a modify/delete conflict, and `git rm <path>` keeps it deleted.

## Roll it out

Clusters pinned to the default branch already run the merge. For the rest, tag the merge commit in your fork and move pins cluster by cluster ([Promoting and rolling back](rollout.md#promoting-and-rolling-back)):

```sh
git switch main && git pull
git tag v1.5.0 && git push origin v1.5.0
```

## ACM and GitOps

Before handover, `install/operators` created the Subscriptions with automatic approval on the channel `stack/acm/values.yaml` and `stack/gitops/values.yaml` named at the time. OLM upgrades within that channel; a new channel from upstream does not reach the hub. Hand the operators over, or rerun [Install the operators](../start/install.md#install-the-operators) with the new values.

After handover, the channel is `subscription.channel` of the element's `operatorPolicies` entry. An upstream merge that changes it, or your own layer, moves the hub's Subscription to that channel once the hub's fleet `revision` includes it. Nothing steps through minors for you, and ACM upgrades only from the previous minor. To go from `release-2.15` to `release-2.17`, set `release-2.16` in the hub's layer, wait until `oc get csv -n open-cluster-management` shows the 2.16 CSV `Succeeded`, then set `release-2.17` or take the upstream merge. Pin or hold versions as in [Operator versions](operators.md).

## Install chart

`install/` holds the two ApplicationSets and their supporting objects ([Install chart](../reference/install.md)). No Argo CD Application reads it, so merging changes nothing until someone runs `helm upgrade` on each hub. From then on, the new ApplicationSets generate the Applications of every cluster on that hub, whatever `revision` each pins. Render Diff does not render `install/`, so read its diff before upgrading.

Run it right after the merge. The ApplicationSets pass each Application only root-level Helm values, such as `version`, `channel` and `selector`, which no element or lifecycle schema rejects, so tag-pinned clusters on an older revision keep rendering. The reverse is not guaranteed: elements from the merge that read a value only the new chart passes fail with a ComparisonError on branch-pinned clusters until the hub is upgraded.

1. Update your clone, and find the commit the hub runs:

    ```sh
    git switch main && git pull
    helm history policystack -n openshift-gitops
    ```

    The DESCRIPTION of the `deployed` revision is the short SHA passed with `--description` at install or the last upgrade; call it `<last>`. If it reads `Rollback to <n>`, use revision `<n>`'s DESCRIPTION. If it reads `Install complete` or `Upgrade complete`, no SHA was recorded: take the last commit to `install/` before its UPDATED date, `git log -1 --until=<date> --format=%h -- install/`.

2. Check what changed. `install/operators` is not part of the release:

    ```sh
    git diff <last>..HEAD -- install/ ':!install/operators'
    ```

    If it prints nothing, the hub needs no upgrade.

3. Upgrade, in the hub's kube context:

    ```sh
    helm upgrade policystack ./install -n openshift-gitops --reset-then-reuse-values \
      --description "$(git rev-parse --short HEAD)"
    ```

    `--reset-then-reuse-values` (Helm 3.14 or later) takes the new chart's defaults and keeps every value set at install or a later upgrade, normally just `hubName` and `gitRepo`.

Roll back with `helm rollback policystack <revision> -n openshift-gitops`.
