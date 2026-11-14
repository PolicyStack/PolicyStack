# Launcher

The launcher, `tools/policystack`, creates your repository, adds elements to it and updates it to
newer upstream releases. It runs copier in process, one copy per scope (the framework, or one
element), and records each in that scope's `.copier-answers.yml`
([Answers files](../guides/upgrading.md#answers-files)). This page lists its commands, flags, exit
codes, the state it keeps, and the `upgrades/` formats upstream writes for it. Walkthroughs:
[Installation](../start/install.md#create-your-repository) and
[Upgrading PolicyStack](../guides/upgrading.md).

## Requirements

| Needs | Why |
|---|---|
| [mise](https://mise.jdx.dev) | Installs uv and Python at the versions `mise.toml` pins (`mise install`), and runs the tasks below |
| git 2.43 or later | `git merge-file --object-id`, used to carry your edits into files upstream renamed. Not checked at start |
| Network access to upstream's Git and to PyPI | Upstream through `git`, so `url.insteadOf` mirrors apply. PyPI once: uv installs copier 9.19.1 and the other packages `tools/policystack/uv.lock` pins into `tools/policystack/.venv`. Without either: [Disconnected](../start/install.md#disconnected) |

Tested on Linux only.

## Tasks

| Task | Runs |
|---|---|
| `mise run ps -- <command> [args]` | The launcher. The `--` stops mise from reading the launcher's flags |
| `mise run update [args]` | `ps update [args]`, then `validate` only if the update exited `0`. Non-zero if either fails |
| `mise run validate` | `make -C tools/validator validate BASE_BRANCH="${BASE_BRANCH:-main}"` ([Running it](../develop/validation.md#running-it)) |

mise runs every task from the directory that holds `mise.toml`, wherever you call it. Give
`install` an absolute path.

## Commands

### install

```text
policystack install [--src URL] [--to vX.Y.Z] [--pre] [--elements a,b] [--examples values/x.yaml,...] DIR
```

Your repository doesn't exist yet, so `install` runs as `mise run ps -- install` in the launcher
fetched from an upstream release ([Create your repository](../start/install.md#create-your-repository)).

| Flag | Default | Effect |
|---|---|---|
| `DIR` | Required | The new repository. Must be empty or absent, and not inside a Git work tree |
| `--src` | `https://github.com/PolicyStack/PolicyStack.git` | Upstream. Recorded as `_src_path` in every answers file. To use a mirror, keep this default and rewrite it with `url.insteadOf`. copier reads a URL as git only if it ends in `.git` (or starts `git@`, `git://` or `git+`), so another URL fails with `Local template must be a directory` |
| `--to` | A picker; without a terminal, the newest release that isn't blocked | The release to copy. May name a prerelease, `vX.Y.Z-rc.N`, without `--pre` |
| `--pre` | Off | Also list prereleases in the picker, and take the newest of them as the default without a terminal |
| `--elements` | A picker. Required without a terminal; may be `''` | Comma-separated elements |
| `--examples` | A picker; without a terminal, none | Comma-separated files from upstream's `values/` |

It copies the framework, then each element in its own scope: `acm`, `gitops` and `upgrade`
(always, locked in the picker), the ones you picked, every element a picked example file turns on
(`stack.<key>.enabled: true`), and every hard dependency of those, transitively. A hard dependency
is an `element:` named by a policy that doesn't ship `enabled: false`. One named only by such a
disabled policy is listed under `not added` and not copied. The picked example files are copied
byte for byte and are yours from then on. Then `git init -b main` and one commit,
`chore: PolicyStack <tag>`. Nothing is pushed.

A failure or Ctrl-C before the commit removes `DIR`.

### add

```text
mise run ps -- add [element...]
```

Copies elements, plus every hard dependency not already in `stack/`, at the framework's release,
the `_commit` in `/.copier-answers.yml`. Without names it opens a picker of the elements you don't
have; without a terminal, names are required. One commit on the branch you are on,
`feat: add <elements> (<tag>)`.

Refuses: an element already in `stack/`, an unknown name (with the closest match), and an element
that first ships after your framework's release, naming that release:

```text
policystack: newel first ships in v1.1.0; update the framework first: mise run update --to v1.1.0
```

### update

```text
mise run ps -- update [--to vX.Y.Z] [--pre] [element...]
```

| Argument | Default | Effect |
|---|---|---|
| `--to` | A picker. Required without a terminal | The release to move to. Your current release updates only elements behind the framework. May name a prerelease without `--pre` |
| `--pre` | Off | Also list prereleases in the picker. From a prerelease, the releases after it are listed either way |
| `element...` | A picker of the changed elements, all ticked; without a terminal, every changed element | Elements to update. The framework always moves |

An element is changed when a file of it differs upstream between its `_commit` and the target, or
when the target is a new major. Each plan row:

| Text | Means |
|---|---|
| `N files changed` | Files upstream added, changed, renamed or deleted in that scope between your release and the target |
| `you modified M` | Files copied at your release that you have edited or deleted since |
| `K overlap -> may conflict` | Files in both counts |
| `BREAKING` | A commit touching the scope has `!` or a `breaking:` subject ([Versions](compatibility.md#versions)) |

An element must move, and is locked in the picker, when the target is a new major or when a pending
upgrade step rewrites its keys. Naming a subset that leaves one out is refused.

The plan also lists pending [upgrade steps](#upgrade-steps) and the files they rewrite, unchanged
elements, elements removed or added upstream, elements you wrote, missing hard dependencies, and
these notes:

| Note | When |
|---|---|
| `hub chart changed: before merging, run on every hub from this branch: helm upgrade ...` | `install/` changed ([Install chart](../guides/upgrading.md#install-chart)) |
| `major release: follow docs/guides/upgrading.md#majors ...` | The target is a new major ([Majors](../guides/upgrading.md#majors)) |

Refuses, before any change:

- Any uncommitted change or untracked file in the repository, unmerged paths, or a conflict marker
  (a line starting `<<<<<<< `) committed in any file.
- A target that is blocked, past a required stop, or not a release or prerelease after yours.
- A release tag that moved upstream since this machine last fetched it, or a release commit that
  carries a second tag ([Releases](../contributing.md#releases)).
- Branch `policystack/update-<tag>` existing while you are not on it.
- On a major, a tracked element upstream removed. Delete it, or only its answers file to keep it as
  yours, commit, and rerun.
- An upgrade step that can't be applied to every file it matches.

## Exit codes

| Exit | Meaning |
|---|---|
| `0` | Done, or nothing to update |
| `1` | Refused before any change; you answered no at `Proceed?`; copier refused; or `update` stopped on conflicts. Scopes committed before the stop stay on the branch |
| `2` | Bad arguments |
| `130` | Ctrl-C. The scope in progress is put back; `install` removes `DIR` |

## Branch and commits

`update` creates branch `policystack/update-<tag>` from the branch you are on and commits there, in
this order. It never pushes.

| Commit | Holds |
|---|---|
| `chore(policystack): upgrade steps for <tag>` | Files under `values/` and `fleet/` that upgrade steps rewrote |
| `chore(policystack): update framework to <tag>` | Every framework file and `/.copier-answers.yml`. Never `stack/`, `values/` or `fleet/` |
| `chore(policystack): update <element> to <tag>` | `stack/<element>/` only. One per element |

Rerun on that branch after a stop, and it continues with the scopes left
([Resolving conflicts](../guides/upgrading.md#resolving-conflicts)). If the update is refused or
interrupted before its first commit, the launcher switches back to the branch you were on and
deletes the new one.

## State

| Path | Holds |
|---|---|
| `$XDG_CACHE_HOME/policystack/<hash>.git`, default `~/.cache/policystack/` | A bare copy of upstream's tags, one per upstream URL. Fetched without force or prune, so a release tag that moved upstream is refused for as long as it lives. Delete it only if the moved tag is right |
| `.git/policystack-update` | Written while a scope is applied and removed after. If the run is killed, the next run refuses and prints the `git reset` and `git clean` that undo the half-applied scope |
| `tools/policystack/.venv` | The launcher's Python environment, made by uv. Ignored by Git |

## Upgrade steps

A release that renames or moves a values key can ship a step that rewrites your `values/` and
`fleet/` on the update branch, as its own commit for you to review. Steps are framework files,
`upgrades/<name>.yaml`, read at the target release. A step is pending when the target has it and
your current release doesn't. Pending steps run in file-name order, all in memory, before anything
is written. A release without `upgrades/` has no steps.

```yaml title="upgrades/2026-11-10-user-monitoring-retention.yaml"
summary: user-monitoring config.prometheus.retention becomes config.prometheus.retentionTime
files: ["values/**/*.yaml", "fleet/**/*.yaml"]
ops:
  - rename: stack.userMonitoring.config.prometheus.retention
    to: retentionTime
manual: |
  Rename stack.userMonitoring.config.prometheus.retention to retentionTime in the files listed.
```

| Key | Meaning |
|---|---|
| `summary` | Required. One line, shown in the plan |
| `files` | Globs from the repository root, such as `values/**/*.yaml`. Every match must be under `values/` or `fleet/`. Directories and untracked files are skipped |
| `ops` | Applied in order to every file. `rename: <path>` with `to: <new last key>`, or `move: <path>` with `to: <full new path>`. A path is dotted, or a list for keys that contain dots. `*` matches any key; `$N` in `to` is what the Nth `*` matched: `move: stack.*.toggles.*`, `to: stack.$1.toggles.$2.enabled`. A file where no path matches is left as it is |
| `stop` | `true` makes the first release that ships this step a [required stop](#required-stops) |
| `manual` | What to do by hand. Printed when a file can't be rewritten. A step without `ops` changes nothing, and the plan prints `manual` as the instruction: `by hand:`, or `before the next release:` with `stop: true` |

Only the changed key is edited: comments, quoting, blank lines and the rest of the file stay as they
were. Every element whose `stack.<key>` an op names must move in the same update (`stack.*` names all
of them). Keys inherited through a YAML `<<` merge are not matched.

The whole update is refused, before anything is written, when a pattern is absolute or contains
`..`, a match is a symlink or outside `values/` and `fleet/`, a file doesn't parse or can't be
rewritten without changing other lines, or a target key is already set.

## Blocked releases

A release found broken after it shipped can't be fixed in place, because tags never move. Upstream
lists it in `upgrades/blocked.yaml` of a later release instead. The launcher reads the file at the
newest release, shows each listed release with its reason, and refuses it as a target for `install`
and `update`.

```yaml title="upgrades/blocked.yaml"
- to: v1.2.0
  message: odf on v1.2.0 waits on newel forever
  fixedIn: v1.2.1 # optional; shown as "; use v1.2.1"
```

## Required stops

`update` must land on each required stop before going further. Later releases show
`update to <stop> first (required stop)` and can't be picked. `install` ignores stops.

- The last release of each major: majors are taken one at a time
  ([Majors](../guides/upgrading.md#majors)).
- The first release that ships a step with `stop: true`.

A blocked release is never a stop. For a major, the stop moves to the newest release of that major
that isn't blocked.
