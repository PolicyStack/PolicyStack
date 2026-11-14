# Upgrading PolicyStack

Your repository was created by the launcher from an upstream release tag ([Installation](../start/install.md#create-your-repository)). Upgrading means running `mise run update`, which merges a newer release into a branch, the framework first and then each element you pick, one commit each, and validates the result. You open a PR, then roll it out like any other change. No bot, script or hosted service is involved, and nothing is pushed for you. Argo CD never reads upstream.

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
| Install chart (`install/`) | `helm upgrade`, run by you on each hub | When you run it, for every cluster on that hub at once ([Install chart](#install-chart)) |

## Checking for updates

Run `mise run ps -- update` in a terminal. It fetches upstream's tags and lists the releases you can move to. Press Ctrl-C at any prompt, or answer no at `Proceed?`, and nothing changes. Nothing checks for releases on its own: run it when you plan updates, or watch upstream's [releases](https://github.com/PolicyStack/PolicyStack/releases) on GitHub.

## Taking an update

Start from an up-to-date default branch with nothing uncommitted. The launcher refuses any uncommitted change or untracked file, anywhere in the repository:

```sh
git switch main && git pull
mise run update # (1)!
```

1.  `mise run ps -- update`, then, if that exits `0`, `mise run validate` on the result ([Running it](../develop/validation.md#running-it)). Without a terminal, pass the release: `mise run update --to v1.1.0`.

The samples below come from a test upstream with releases `v1.0.0` to `v2.0.0`.

**1. Pick the release.** Newest first. Your current release is listed last, for elements still behind the framework:

```text
current: v1.0.0
? Release (Use arrow keys)
   - v2.0.0 (update to v1.1.0 first (required stop))
   - v1.2.1  required stop: last v1 release before v2 (update to v1.1.0 first (required stop))
   - v1.2.0 (update to v1.1.0 first (required stop); blocked: odf on v1.2.0 waits on newel forever; use v1.2.1)
 » v1.1.0  required stop: lifecycle hub objects carry lifecycle.policystk.io/hub
   v1.0.0  current: elements behind it only
```

Rows marked `-` can't be picked:

- A **required stop** is a release you must land on before going further: the last release of each major, so majors go one at a time, and any release whose upgrade step must finish first ([Required stops](../reference/launcher.md#required-stops)). Here `v1.1.0` holds back everything after it. Take it, merge and roll it out as its step says, then update again.
- A **blocked** release is one upstream found broken after it shipped. Its reason names the release to use instead ([Blocked releases](../reference/launcher.md#blocked-releases)).

**2. Pick the elements.** Only elements upstream changed between their release and yours are listed, all ticked, with what changed:

```text
? Elements to update (type to filter by name, <space> toggle, <enter> done)
 » ● odf                       1 file changed
   ● storage-nodes             1 file changed
   ● user-monitoring (locked)  2 files changed, BREAKING
```

A row can also say `you modified 2, 1 overlap -> may conflict`. `you modified` counts the element's files you edited or deleted since it was copied. `overlap` counts the files you and upstream both changed: those may conflict ([Resolving conflicts](#resolving-conflicts)). `BREAKING` means a commit to that element is marked `!` or `breaking:`: read its release notes. A `(locked)` row must move with this release, because the release is a new major or an upgrade step rewrites that element's keys. Untick an element to leave it on its release; the framework always moves.

**3. Read the plan.** It lists everything the update will do, then asks `Proceed?`:

```text
PolicyStack v1.0.0 -> v1.1.0
  framework: 2 files changed, BREAKING
  upgrade step 2026-11-10-user-monitoring-retention.yaml: user-monitoring config.prometheus.retention becomes config.prometheus.retentionTime
  upgrade step 2026-11-11-hub-annotation.yaml: lifecycle hub objects carry lifecycle.policystk.io/hub
    before the next release: Move every fleet file with hub: to this release and wait for lifecycle-<cluster> to be
      Compliant before taking a newer release.
  upgrade steps rewrite: values/clusters/prod-east-1.yaml
  [x] odf                  1 file changed
  [x] storage-nodes        1 file changed
  [x] user-monitoring      2 files changed, BREAKING
  unchanged: acm, gitops, infra-nodes, machine-health, upgrade
  new upstream (mise run ps -- add): newel
? Proceed? (y/N)
```

| Line | Means | You |
|---|---|---|
| `upgrade step <file>: ...` | The release renames or moves a values key, or needs a step by hand ([Upgrade steps](#upgrade-steps)) | Do any `by hand:` or `before the next release:` instruction |
| `upgrade steps rewrite: ...` | Your files the steps edit, in their own commit | Review that commit |
| `new upstream` | Elements this release adds | `mise run ps -- add <element>` after the merge, if you want one |
| `removed upstream (yours to delete)` | Elements this release no longer ships. Yours from now on | Delete it, or keep it as yours ([Element updates](#element-updates)) |
| `note: <element> at <tag> needs: ...` | An updated element now needs an element you don't have | Add it |
| `hub chart changed: ...` | `install/` changed | Upgrade every hub from this branch before merging ([Install chart](#install-chart)) |
| `major release: ...` | A new major | [Majors](#majors) |

**4. Proceed.** The launcher creates branch `policystack/update-v1.1.0` and commits each scope on it, upgrade steps first, then the framework, then each element:

```text
updating framework (1/4)
updating odf (2/4)
updating storage-nodes (3/4)
updating user-monitoring (4/4)
done on branch policystack/update-v1.1.0, nothing pushed
```

```sh
git log --format=%s main..
# chore(policystack): update user-monitoring to v1.1.0
# chore(policystack): update storage-nodes to v1.1.0
# chore(policystack): update odf to v1.1.0
# chore(policystack): update framework to v1.1.0
# chore(policystack): upgrade steps for v1.1.0
```

A framework update takes about 15 seconds, an element a few. If it stops on a conflict, see [Resolving conflicts](#resolving-conflicts). Then `validate` runs: fix what it reports on the branch, the same as [step 2](#framework-updates) below.

**5. Open the PR.** Review each commit's diff, then push the branch and open a PR. `ci` and [Render Diff](#reading-render-diff) run on it like on any PR, and `pr-title` needs a Conventional Commit title, such as `chore(policystack): update to v1.1.0`.

```sh
git push -u origin policystack/update-v1.1.0
```

!!! tip "Give the framework its own PR"
    It has steps before the merge ([Framework updates](#framework-updates)). Untick every element, merge the framework, then run `mise run update` again and pick your current release (`current: elements behind it only`) for the elements. One PR per element keeps each Render Diff to that element.

### Upgrade steps

A release that renames or moves a values key, which only a major does for a stable element, can ship an upgrade step: a file in upstream's `upgrades/` that says which key moves where. The launcher applies every step between your release and the target to your `values/` and `fleet/`, as the first commit on the update branch. It edits only the key: comments, quoting and the rest of the file stay. It never pushes and never touches a file Git doesn't track, so the change reaches clusters only when you merge the PR. A file it can't rewrite safely refuses the whole update before anything is written, and the message says what to change by hand. Format: [Upgrade steps](../reference/launcher.md#upgrade-steps).

!!! example "The step in the plan above, on your layer"

    ```diff
       userMonitoring:
         config:
           prometheus:
    -        retention: 15d # keep two weeks
    +        retentionTime: 15d # keep two weeks
    ```

## Framework updates

The framework update applies upstream's changes between your `_commit` and the new release to every framework file, and moves `_commit`. It never touches `stack/`, `fleet/` or `values/`; only upgrade steps, in their own commit, write the last two. A file upstream deleted is deleted in your repository too.

Open the PR as in [Taking an update](#taking-an-update), then:

1.  **Read the release notes** of every release between your `_commit` and the new tag ([releases](https://github.com/PolicyStack/PolicyStack/releases)). A note about hubs, the install chart or lifecycle applies to step 4.
2.  **Fix what `validate` and CI report.** The new validator runs against your fleet and values. A new error, such as [POLICY035](../reference/rules.md) for a key an element dropped, is fixed in the same PR. Warnings such as POLICY039 (a deprecated key) can wait until the version they name. A conflict means you edited a framework file ([Resolving conflicts](#resolving-conflicts)).
3.  **Read the [Render Diff](#reading-render-diff).**
4.  **Upgrade the install chart on every hub from the PR branch** ([Install chart](#install-chart)). The plan says `hub chart changed` when this applies.
5.  **Merge.**
6.  **Move tag-pinned clusters** to a revision that has the update ([Roll it out](#roll-it-out)).

!!! warning "Hubs before pins"
    Within a major, a hub is only guaranteed to render revisions up to its own install chart's release. Upgrade every hub before you move any pin to a revision with a newer framework. A pin moved first can fail its render on a hub still on the older install chart ([Hub and revision skew](../reference/compatibility.md#hub-and-revision-skew)). Clusters that follow the default branch move the moment you merge, which is why step 4 comes before step 5.

### Majors

Take majors one at a time. A hub on major N is only guaranteed to render revisions on N and on the last N-1 release ([Hub and revision skew](../reference/compatibility.md#hub-and-revision-skew)):

1.  Take the last release of your current major, for the framework and every element, and move every fleet pin to a revision that has it. The launcher makes that release a required stop.
2.  Take the next major for the framework and every element in one PR: upgrade every hub from the update's PR branch, then merge it. The launcher locks every element in, and POLICY061 fails the PR while any element is still on the older major.
3.  Move pins to the new major, cluster by cluster.

POLICY052 checks only the major: on the update PR, a fleet file whose `revision` is more than one major behind, or on a newer major than the branch, fails. It can't tell whether an N-1 pin is the last N-1 release, so that part of step 1 is yours to check.

## Element updates

An element update merges upstream's changes to the element since its `_commit` with yours and moves `_commit`. Nothing outside `stack/<element>/` changes. Elements always update to the framework's release, so an element never gets ahead of it.

- Read the element's entries in the release notes, prefixed with its name (`odf:`), and the Render Diff. For a preview element any minor may rename keys or Policies; search those entries for `breaking:`.
- An element upstream removed (POLICY092 warned while it was deprecated) shows as `removed upstream` and stays as it is. It still works on the same major. An update to a new major is refused until you delete it (`git rm -r stack/<element>`) or eject it to keep it as yours.
- An element you left out stays on its release and is offered again on the next update.

## Skip, freeze, eject, roll back

| To | Do |
|---|---|
| Skip a release | Pick a later one. A required stop can't be skipped. |
| Freeze an element | Untick it, or name only the others: `mise run update --to v1.3.0 odf`. It keeps working on newer framework releases of the same major. A framework major ends the freeze: the launcher locks it in, and POLICY061 fails until the element is on the new major. |
| Eject an element | `git rm stack/odf/.copier-answers.yml`. The launcher stops offering it (it lists it as `untracked (yours)`), the validator stops checking its answers file, and the element is yours ([Tiers](../reference/compatibility.md#tiers)). To track it again, delete the directory and add it again ([Add elements](../start/install.md#add-elements)). |
| Remove an element | `git rm -r stack/odf`. Its Applications and Policies are deleted ([Removing an element](rollout.md#removing-an-element)). |
| Roll back | `git revert` the update's merge commit. The answers files revert with it, so the next update offers the release again. Clusters pinned to a tag never had it. |

## Answers files

`.copier-answers.yml` at the root and in each tracked element is copier's record of what it copied: `_commit` is the release tag, `_src_path` the upstream URL, and `element` the element, empty for the framework. copier rebuilds the old release from `_commit` to work out what you changed, the launcher reads both to find newer releases and what changed, POLICY052 reads the root file at each fleet `revision` to check skew, and POLICY061 checks every one.

!!! danger "Never edit an answers file by hand"
    A `_commit` that doesn't match the files makes the next update merge against the wrong base: your changes are reported as conflicts, or upstream changes are skipped. Change it only with `mise run update`, or `git revert` an update. POLICY061 fails a `_commit` that isn't a release tag, an element answers file whose `element` isn't its directory, and an element newer than the framework.

## Resolving conflicts

An update merges three versions of each file: the old release, the new release and yours. Where you and upstream changed the same lines, it leaves both in the file, between conflict markers, and stops:

```text
updating storage-nodes (3/4)
stopped: conflicts in storage-nodes:
  stack/storage-nodes/README.md
resolve each file (git rm one you no longer need), then:
  git add -A -- stack/storage-nodes && git commit -m 'chore(policystack): update storage-nodes to v1.1.0'
  mise run update --to v1.1.0
```

!!! example "A conflict in `stack/odf/values.yaml`"

    ```yaml
    <<<<<<< before updating
          storageClassName: fast-ssd
    =======
          storageClassName: ocs-storagecluster-ceph-rbd
    >>>>>>> after updating
    ```

`before updating` is your repository, `after updating` is the new release. The scopes before the stop are already committed on the branch; nothing after it has started. To finish:

1.  `git status` lists each conflicted file under `Unmerged paths`. Edit each to what you want, deleting the markers. In a framework file, take `after updating`: a framework you modified is not supported, and the conflict comes back on every update. Don't touch the answers file.
2.  Run the two lines the launcher printed: the `git add ... && git commit` for that scope, then `mise run update --to <tag>`. On the update branch it continues with the scopes left.

The launcher refuses to commit a file with a conflict marker (a line starting `<<<<<<< `, from copier or any other merge), and refuses to start while any committed file has one. The `ci` check fails a marker too (the `check-merge-conflict` hook).

These cases need a decision, not only an edit. Each is listed as a conflict, and the printed `git add -A` stages whatever you leave:

| You | Upstream | `git status` shows | To keep yours | To take upstream's |
|---|---|---|---|---|
| Edited a file | Deleted it | `deleted by them`, your copy in place | Leave the file | `git rm` it |
| Deleted a file | Changed or renamed it | `deleted by us`, upstream's new copy in place | `git rm` it | Leave the file |
| Edited a file, and have a file of your own at its new name | Renamed it | Your edited file `deleted by them` under its old name, and the new name `both modified`: your own file against upstream's | Resolve the new name, moving in what you need from the old file, then `git rm` the old file | Take `after updating` at the new name, and `git rm` the old file |

A file you edited that upstream renamed gets your edit merged into the new name, with conflict markers only where the two overlap, unless you have a file of your own at the new name (the last row above). A file you deleted that upstream didn't change stays deleted.

A conflict in an element you edited means the edit and upstream's change overlap. If they keep overlapping, move the setting to a values layer, where updates don't merge into it, or [eject](#skip-freeze-eject-roll-back) the element.

!!! tip "Change settings in layers, not in elements"
    Every setting an element exposes can be set from `values/`. Editing an element's `values.yaml` or converters instead makes it yours to merge on every update.

## When an update stops

| What happened | The launcher | You |
|---|---|---|
| Ctrl-C, or an error mid-scope | Puts the scope back and exits `130` (or `1`). Committed scopes stay | Rerun |
| The process was killed mid-scope | Leaves `.git/policystack-update`. The next run refuses and prints `git reset -q --hard <sha> && git clean -fdq -- <paths>` | Run that line, then rerun |
| Branch `policystack/update-<tag>` exists and you aren't on it | Refuses | `git switch` to it and rerun to finish, or `git branch -D` it to start over |
| A release tag moved upstream | Refuses: tags never move | Ask upstream. Delete the cache directory it names only if the new tag is right |
| A release commit carries a second tag | Refuses: copier would record the other tag | Ask upstream |

Every exit code: [Exit codes](../reference/launcher.md#exit-codes).

## Without the launcher

copier runs from the launcher's environment ([Without the launcher](../start/install.md#without-the-launcher) defines the `copier` function). Commit each update before the next; copier refuses a path with uncommitted changes, and for the framework that path is the whole repository:

```sh
git switch -c update/v1.1.0
copier update --defaults --vcs-ref v1.1.0 .
git add -A && git commit -m "chore(policystack): update framework to v1.1.0"
copier update --defaults --vcs-ref v1.1.0 stack/odf
git add -A && git commit -m "chore(policystack): update odf to v1.1.0"
```

You lose what the launcher adds: required stops, blocked releases, upgrade steps, the plan, tag checks and the undo. copier itself also:

- Takes the newest release without `--vcs-ref`, which takes an element past your framework unless the framework is on the newest. POLICY061 fails it.
- Deletes a file you edited when upstream deleted it, keeps a file you deleted deleted even when upstream changed it, and drops your edit to a file upstream renamed, all without a conflict or a message. Check the diff for each.
- Sometimes ends a framework update with `OSError: [Errno 39] Directory not empty: '/tmp/copier._main.new_copy...'` and exit code `1`, after the update is applied. Delete the leftover directory and review as usual. Git's automatic housekeeping writing into copier's temporary clone causes it; prefix the command with `GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false` to avoid it.
- Prints `Make sure Git >= 2.24 is installed to improve updates.` on every update, whatever your Git version. Ignore it.

`copier check-update --quiet <scope>` exits `2` when a newer release exists for that scope.

## Reading Render Diff

On an update PR, [Render Diff](../reference/ci.md#render-diff) shows what the merge changes for every cluster pinned to the default branch. A framework update shows changes from `lib/` and `lifecycle/` across elements; an element update, that element only. Tag-pinned clusters show theirs on the PR that [moves their pin](#roll-it-out).

- `removes Policy` names a Policy the update deletes. A rename shows as a removal plus an addition: the Policy is recreated and its compliance history starts over ([Policy names](../reference/compatibility.md#policy-names)).
- **render failed** means the update breaks that cluster. The error is in the cluster's block. Fix it in the PR.
- `install/` is not rendered. Read its diff in the PR ([Install chart](#install-chart)).

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

`install/` holds the two ApplicationSets and their supporting objects ([Install chart](../reference/install.md)). No Argo CD Application reads it, so merging a framework update changes nothing on a hub until someone runs `helm upgrade` there. From then on, the new ApplicationSets generate the Applications of every cluster on that hub, whatever `revision` each pins. Render Diff does not render `install/`, so read its diff on the update PR.

The release's chart version is the framework release it was installed from. Compare it with the framework update's PR branch, or with the default branch once the update is merged:

```sh
git fetch origin && git switch --detach origin/<branch>   # the PR branch, or main
helm list -n openshift-gitops   # CHART: policystack-1.2.0
grep '^version' install/Chart.yaml
```

If they differ, upgrade, in the hub's kube context:

```sh
helm upgrade policystack ./install -n openshift-gitops --reset-then-reuse-values
```

`--reset-then-reuse-values` (Helm 3.14 or later) takes the new chart's defaults and keeps every value set at install or a later upgrade, normally just `hubName` and `gitRepo`.

Within a major the ApplicationSets only add injected values, so clusters pinned to older revisions keep rendering. The exception is a fleet file's `install`, passed through as written: a key newer than the cluster's `revision` fails that revision's lifecycle schema ([Lifecycle](../reference/compatibility.md#lifecycle)). A policy-library major is checked at render: the chart passes `policystack.libraryMajors`, and a revision built on a library major the hub's chart doesn't list fails its render with a ComparisonError that says to move the pin or upgrade the hub.

Roll back with `helm rollback policystack <revision> -n openshift-gitops`.
