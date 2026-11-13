# Upgrading PolicyStack

Your repository was created with copier from an upstream release tag ([Installation](../start/install.md#create-your-repository)). Upgrading means running `copier update` to merge a newer release into a branch, the framework first and then each element you want, opening a PR, then rolling the result out like any other change. No bot, script or hosted service is involved. Argo CD never reads upstream.

## How releases are versioned

Upstream has one tag line, `vX.Y.Z`, semver. Each release tag holds the framework and every element. Your repository records which release each part came from, in its own answers file:

| Part | Recorded in | Covers |
|---|---|---|
| Framework | `/.copier-answers.yml` | `install/`, `lifecycle/`, policy-library in `lib/`, `tools/`, the CI workflows and hooks |
| Element | `stack/<element>/.copier-answers.yml` | That element only |

Because each part has its own record, you can update them at different times: the framework to `v1.3.0` this week, `odf` next month, `acm` not at all. One rule orders them: an element is never on a newer release than the framework, so take the framework first. An element on an older release than the framework keeps working within the same major ([Versions](../reference/compatibility.md#versions)).

A major breaks something you depend on, and its release notes say what to change. A stable element breaks only in a major; a preview element may break in a minor, with notes. Each element's `README.md` shows which it is ([Element stability](../reference/compatibility.md#element-stability)). Lifecycle engines and platforms are all preview.

Each part reaches your clusters on its own path:

| Part | Delivered by | Reaches clusters |
|---|---|---|
| Elements, `lifecycle/`, policy-library (`lib/`) | Argo CD, from your repository at each fleet file's `revision` | Clusters pinned to the default branch, often the hub: within minutes of the merge. Clusters pinned to a tag or SHA: when you [move the pin](#roll-it-out). |
| Validator and CI | GitHub Actions in your repository | On the update PR itself |
| ACM and GitOps operators | The `acm` and `gitops` elements, after [handover](../start/install.md#hand-the-operators-to-policystack) | Like any element, at the hub's `revision` ([ACM and GitOps](#acm-and-gitops)) |
| Install chart (`install/`) | Each hub's own Argo CD, at the hub file's `installRevision` | Hubs that follow the default branch: within minutes of the merge. Hubs with a tag `installRevision`: when you move it. Either way, for every cluster on that hub at once ([Install chart](#install-chart)) |

## Checking for updates

`copier check-update` compares one answers file's `_commit` with upstream's tags:

```sh
copier check-update .            # the framework
copier check-update stack/odf    # one element
```

```text
Current version is 1.0.0, latest version is 1.2.0.
```

To check the framework and every tracked element at once, loop over them. With `--quiet`, copier prints nothing and exits `2` when a newer release exists:

```sh
for d in . stack/*/; do
  [ -f "$d/.copier-answers.yml" ] || continue  # (1)!
  copier check-update --quiet "$d"; [ $? -ne 2 ] || echo "update available: $d"
done
```

1.  Elements you wrote or ejected have no answers file and are skipped.

copier asks the Git server in `_src_path` for its tags, so this works against an internal mirror too ([Disconnected clusters](disconnected.md#policystack-releases)). It only reads. Prereleases, such as `v1.3.0-rc.1`, are ignored unless you pass `--prereleases`.

Nothing checks for releases on its own. Run the loop when you plan updates, or watch upstream's [releases](https://github.com/PolicyStack/PolicyStack/releases) on GitHub.

## Taking an update

Use copier at the version `mise.toml` pins (`mise install`). Start from an up-to-date default branch:

```sh
git switch main && git pull
git switch -c update/v1.2.0
copier update --defaults .                                 # (1)!
git add -A && git commit -m "chore(deps): update framework to v1.2.0"
fw=$(sed -n 's/^_commit: //p' .copier-answers.yml)
copier update --defaults --vcs-ref "$fw" stack/odf          # (2)!
git add -A && git commit -m "chore(deps): update odf to $fw"
```

1.  The framework, to the newest release. Add `--vcs-ref vX.Y.Z` for another one.
2.  An element, to the framework's release. Without `--vcs-ref` copier takes the newest release, which is newer than your framework unless you just updated it to the newest.

Review each diff, resolve any conflict markers ([Resolving conflicts](#resolving-conflicts)), then push the branch and open a PR. `ci` and [Render Diff](#reading-render-diff) run on it like on any PR, and `pr-title` needs a Conventional Commit title such as the ones above.

`copier update` merges upstream's changes between your `_commit` and the new release with your edits, and moves `_commit`. One update covers any number of releases. It leaves the result uncommitted.

- copier refuses to update a path that has uncommitted changes in it. For the framework that path is the whole repository, so commit each update before the next.
- Give the framework its own PR when you can: it has steps before the merge ([Framework updates](#framework-updates)). Elements can share a PR, but one PR per element keeps each Render Diff to that element and lets you merge or drop each on its own.
- copier prints `Make sure Git >= 2.24 is installed to improve updates.` on every update, whatever your Git version. Ignore it.

!!! note "`OSError: [Errno 39] Directory not empty: '/tmp/copier._main.new_copy...'`"
    A framework update sometimes ends with this traceback and exit code 1. Git's automatic housekeeping is still writing to copier's temporary clone when copier deletes it. The update itself has been applied: `git status` shows the changes and `_commit` names the new release. Review it as usual, and delete the leftover directory. To avoid it, turn housekeeping off for the command:

    ```sh
    GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 \
      GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false copier update --defaults .
    ```

!!! warning "Always pass `--vcs-ref` for an element"
    Unless your framework is on the newest release, `copier update` without `--vcs-ref` takes the element past it. POLICY061 fails the PR, because an element newer than the framework may need framework code you don't have yet. Update the framework first, or take the element back with `--vcs-ref` and the framework's tag.

## Framework updates

`copier update .` applies upstream's changes between your `_commit` and the new release to every framework file, and moves `_commit`. It never touches `stack/`, `fleet/` or `values/`. A file upstream deleted is deleted in your repository too.

!!! warning "A framework file you deleted stays deleted"
    copier doesn't recreate it, and drops upstream's later changes to it without a conflict or a message. Delete a framework file only if you never want it back.

Open the PR as in [Taking an update](#taking-an-update), then:

1.  **Read the release notes** of every release between your `_commit` and the new tag ([releases](https://github.com/PolicyStack/PolicyStack/releases)). A note about hubs, the install chart or lifecycle applies to every hub once it takes the new install.
2.  **Fix what CI reports.** The PR runs the new validator against your fleet and values. A new error, such as [POLICY035](../reference/rules.md) for a key an element dropped, is fixed in the same PR. Warnings such as POLICY039 (a deprecated key) can wait until the version they name. A conflict means you edited a framework file ([Resolving conflicts](#resolving-conflicts)).
3.  **Read the [Render Diff](#reading-render-diff).** It includes each hub's install.
4.  **Merge.** Hubs that follow the default branch take the new install, and clusters that follow it take the new framework, within minutes ([Install chart](#install-chart)).
5.  **Move hubs with a tag `installRevision`** to a tag of the merge, one hub at a time. Such a hub pins its own `revision`, and every fleet file it renders, to tags too: POLICY052 fails the update PR for one that follows the default branch ([installRevision](../reference/fleet.md#installrevision)).
6.  **Move tag-pinned clusters** to a revision that has the update ([Roll it out](#roll-it-out)).

!!! warning "Hubs before pins"
    Within a major, a hub is only guaranteed to render revisions up to its own install's release. Move every hub's install before you move any pin to a revision with a newer framework. A pin moved first can fail its render on a hub still on the older install ([Hub and revision skew](../reference/compatibility.md#hub-and-revision-skew)). POLICY052 fails such a pin where the fleet file says which hub renders it, and warns otherwise ([installRevision](../reference/fleet.md#installrevision)).

### Majors

Take majors one at a time. A hub on major N is only guaranteed to render revisions on N and on the last N-1 release ([Hub and revision skew](../reference/compatibility.md#hub-and-revision-skew)):

1.  Take the last release of your current major, for the framework and every element, and move every fleet pin to a revision that has it.
2.  Take the next major for the framework and every element in one PR, and merge it. Hubs that follow the default branch take the new install at once; move the others' `installRevision`. POLICY061 fails the PR while any element is still on the older major.
3.  Move pins to the new major, cluster by cluster.

POLICY052 checks only the major: on the update PR, a fleet file whose `revision` is more than one major behind, or on a newer major than the branch, fails. It can't tell whether an N-1 revision is the last N-1 release, so step 1 is yours to check. `copier update` without `--vcs-ref` takes the newest release, whatever its major. For step 1, or when the newest release is two majors ahead, name the tag from the [releases page](https://github.com/PolicyStack/PolicyStack/releases): `copier update --defaults --vcs-ref v1.9.2 .`.

## Element updates

`copier update stack/odf` merges upstream's changes to `odf` since your `_commit` with yours and moves `_commit`. Nothing outside `stack/odf/` changes.

- Read the element's entries in the release notes, prefixed with its name (`odf:`), and the Render Diff. For a preview element any minor may rename keys or Policies; search those entries for `breaking:`.
- If the element needs a newer policy-library than your framework has, its render fails and [POLICY060](../reference/rules.md) says so. Taking the element at the framework's release, as above, avoids it.
- A file upstream deleted is deleted from `stack/odf/` too, even if you edited it, without a conflict. Check the PR diff for deleted files and move any edit you need into a values layer.
- An element upstream removed (POLICY092 warned while it was deprecated) can't be updated: `copier update` fails with a `FileNotFoundError` naming `stack/<element>` in copier's temporary clone. [Eject](#skip-freeze-eject-roll-back) it to keep it as yours, or remove it.
- Elements update independently, so merge their PRs in any order.

## Skip, freeze, eject, roll back

| To | Do |
|---|---|
| Skip a release | Don't take it, or take a later one with `--vcs-ref`. `copier check-update` keeps reporting the newest release until you take it. |
| Freeze an element | Don't run `copier update` on it. It keeps working on newer framework releases of the same major. A framework major ends the freeze: POLICY061 fails until the element is on the new major. |
| Eject an element | `git rm stack/odf/.copier-answers.yml`. copier and the check loop stop tracking it, the validator stops checking its answers file, and the element is yours ([Tiers](../reference/compatibility.md#tiers)). `copier update stack/odf` then fails with a Python traceback ending in `TypeError: Template not found`. To track it again, delete the directory and copy it again ([Add elements](../start/install.md#add-elements)). |
| Remove an element | `git rm -r stack/odf`. Its Applications and Policies are deleted ([Removing an element](rollout.md#removing-an-element)). |
| Roll back | `git revert` the update's merge commit. The answers file reverts with it, so `copier check-update` reports the update again. Clusters pinned to a tag never had it. |

## Answers files

`.copier-answers.yml` at the root and in each tracked element is copier's record of what it copied: `_commit` is the release tag, `_src_path` the upstream URL, and `element` the element, empty for the framework. copier rebuilds the old release from `_commit` to work out what you changed, `copier check-update` reads both to find newer releases, POLICY052 reads the root file at each fleet `revision` to check skew, and POLICY061 checks every one.

!!! danger "Never edit an answers file by hand"
    A `_commit` that doesn't match the files makes the next update merge against the wrong base: your changes are reported as conflicts, or upstream changes are skipped. Change it only with `copier update`, or `git revert` an update. POLICY061 fails a `_commit` that isn't a release tag, an element answers file whose `element` isn't its directory, and an element newer than the framework.

## Resolving conflicts

copier merges three versions of each file: the old release, the new release and yours. Where you and upstream changed the same lines, it leaves both in the file, between conflict markers:

!!! example "A conflict in `stack/odf/values.yaml`"

    ```yaml
    <<<<<<< before updating
          storageClassName: fast-ssd
    =======
          storageClassName: ocs-storagecluster-ceph-rbd
    >>>>>>> after updating
    ```

`before updating` is your repository, `after updating` is the new release. The `ci` check fails while a marker is left (the `check-merge-conflict` hook), so a conflicted PR can't merge. Resolve them before you commit:

1.  `git diff --check` lists every file and line with a marker.
2.  Edit each file to what you want, deleting the markers. In a framework file, take `after updating`: a framework you modified is not supported, and the conflict comes back on every update.
3.  Run `git diff --check` again until it prints nothing, then commit. Don't touch the answers file.

A conflict in an element you edited means the edit and upstream's change overlap. If they keep overlapping, move the setting to a values layer, where updates never reach it, or [eject](#skip-freeze-eject-roll-back) the element.

!!! tip "Change settings in layers, not in elements"
    Every setting an element exposes can be set from `values/`. Editing an element's `values.yaml` or converters instead makes it yours to merge on every update.

## Reading Render Diff

On an update PR, [Render Diff](../reference/ci.md#render-diff) shows what the merge changes for every cluster pinned to the default branch. A framework update shows changes from `lib/` and `lifecycle/` across elements; an element update, that element only. Tag-pinned clusters show theirs on the PR that [moves their pin](#roll-it-out).

- `removes Policy` names a Policy the update deletes. A rename shows as a removal plus an addition: the Policy is recreated and its compliance history starts over ([Policy names](../reference/compatibility.md#policy-names)).
- **render failed** means the update breaks that cluster. The error is in the cluster's block. Fix it in the PR.
- A hub whose install changes shows a `policystack-install` row: the install as its pointer renders it, with placeholder `gitRepo` ([Install chart](#install-chart)).

## Roll it out

Clusters pinned to the default branch already run the update. For the rest, tag the merge commit in your repository and move pins cluster by cluster ([Promoting and rolling back](rollout.md#promoting-and-rolling-back)):

```sh
git switch main && git pull
git tag v1.5.0 && git push origin v1.5.0
```

These are your repository's tags, unrelated to upstream's. POLICY052 reads the framework release from each pinned commit's answers file, not from the tag name.

## ACM and GitOps

Before handover, `install/operators` created the Subscriptions with automatic approval on the channel `stack/acm/values.yaml` and `stack/gitops/values.yaml` named at the time. OLM upgrades within that channel; a new channel from an element update does not reach the hub. Hand the operators over, or rerun [Install the operators](../start/install.md#install-the-operators) with the new values.

After handover, the channel is `subscription.channel` of the element's `operatorPolicies` entry. An element update that changes it, or your own layer, moves the hub's Subscription to that channel once the hub's fleet `revision` includes it. Nothing steps through minors for you, and ACM upgrades only from the previous minor. To go from `release-2.15` to `release-2.17`, set `release-2.16` in the hub's layer, wait until `oc get csv -n open-cluster-management` shows the 2.16 CSV `Succeeded`, then set `release-2.17` or take the `acm` update. Pin or hold versions as in [Operator versions](operators.md).

## Install chart

`install/` holds the pointer, the two ApplicationSets and their supporting objects ([Install chart](../reference/install.md)). Each hub's own Argo CD syncs it, through the Application `policystack`, at the hub file's `installRevision`, or at the default branch when it sets none ([installRevision](../reference/fleet.md#installrevision)). From then on, the new ApplicationSets generate the Applications of every cluster on that hub, whatever `revision` each pins.

| Hub file | The hub takes a framework update |
|---|---|
| No `installRevision` | When the update merges. Every such hub at once. |
| `installRevision: <tag>` | When you move the tag. Move a lab hub first, then the rest. |

Check what a hub runs:

```sh
oc get application.argoproj.io policystack -n openshift-gitops \
  -o jsonpath='{.status.sync.revision} {.status.sync.status} {.status.health.status}{"\n"}'
```

Within a major the ApplicationSets only add injected values, so clusters pinned to older revisions keep rendering. The exception is a fleet file's `install`, passed through as written: a key newer than the cluster's `revision` fails that revision's lifecycle schema ([Lifecycle](../reference/compatibility.md#lifecycle)). A policy-library major is checked at render: the install passes `policystack.libraryMajors`, and a revision built on a library major the hub's install doesn't list fails its render with a ComparisonError that says to move the pin or the hub's install.

Roll back by moving `installRevision` back to the previous tag, or reverting the merge for hubs that follow the default branch. A release that drops an install object leaves it on the hub ([Deletion safety](../reference/install.md#deletion-safety)); its release notes say what to delete.
