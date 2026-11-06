# CI workflows

PolicyStack's GitHub Actions workflows live in `.github/workflows/`. They run in
upstream PolicyStack and, unchanged, in your fork, the copy of PolicyStack your hubs read.
For an adopter, the checks that cover real clusters are the `fleet/` pass of
[Policy Validate](#policy-validate) and [Render Diff](#render-diff); the rest guard the framework.

| Workflow | Trigger | Fails when | In your fork |
| -------- | ------- | ---------- | ------------ |
| [CI](#ci) (`ci.yml`) | Every PR and `main` push | A hook, the validator, a validator test, the policy-library lint or tests, the doc generator tests or the site build fails | Runs. Its `publish` job is skipped |
| [PR Title](#pr-title) (`pr-title.yml`) | Every PR, including title edits | The title is not a Conventional Commit | Runs |
| [Policy Validate](#policy-validate) (`validate.yml`) | Called by CI | A validator error, a bad fleet pin, a broken install chart, a failing test | Runs, including the `fleet/` pass |
| [Render Diff](#render-diff) (`render-diff.yml`) | PRs into the default branch changing `stack/`, `lifecycle/`, `lib/`, `values/`, `values.yaml`, `fleet/`, `tools/validator/` or Render Diff itself | Never: it only comments | Comments only when `fleet/` has files, so only in a fork |

A new push to a PR cancels its running CI and Render Diff checks. CI and Policy Validate can also be
started from the Actions tab.

## Required checks

A required check is one GitHub won't let a PR merge without. `ci` and `pr-title` are the required
checks for `main`, set in a GitHub repository ruleset. Rulesets live in the repository settings, not
in the repo, so a fork doesn't inherit them: without one, a red PR still merges.

Enable Actions on the fork first (Actions tab): GitHub doesn't run workflows on a new fork, and a
required check that never runs blocks every PR. Then create a branch ruleset on the default branch
that requires the status checks `ci` and `pr-title`
([Set up your fork](../start/install.md#set-up-your-fork) has the steps, plus code owner review on
`fleet/`). The ruleset matches checks by job name, so renaming `ci` or `pr-title` leaves PRs waiting
on a check that never reports.

`ci` is the last job in `ci.yml`. It always runs and fails if any job it needs failed, was cancelled
or was skipped ([alls-green](https://github.com/re-actors/alls-green)). A new job goes in its
`needs`, not in the ruleset.

!!! note
    No gating workflow has a `paths` filter. A required check whose workflow is skipped by `on.paths`
    never reports, so the PR waits on it forever.

## CI (`ci.yml`) { #ci }

| Job | Runs |
| --- | ---- |
| `lint` | `prek run --all-files --show-diff-on-failure`: the [hooks](#hooks) in `.pre-commit-config.yaml` |
| `validate` | [Policy Validate](#policy-validate) |
| `library` | policy-library in `lib/`: `ct lint --config lib/ct.yaml`, which fails a change to `lib/policy-library` without a `version` bump and runs `helm lint --strict`; then the helm-unittest suites in `lib/tests` (`untt --strict`), failing on a snapshot that is not committed ([Library changes](../contributing.md#library-changes)) |
| `docs` | Doc generator tests (`python -m unittest tools/test_doc_generator.py`), then `zensical build --strict` |
| `ci` | The required check over `lint`, `validate`, `library` and `docs` |
| `publish` | `main` pushes in `PolicyStack/PolicyStack` only, after `ci`: the site to `gh-pages` |

Tool versions come from `mise.toml`. Run the same checks locally: [Setup](../contributing.md#setup).

### Hooks

prek is a pre-commit-compatible hook runner, installed by `mise install`. It reads
`.pre-commit-config.yaml`; you don't need pre-commit itself.

| Hook | Checks |
| ---- | ------ |
| `check-yaml`, `trailing-whitespace`, `end-of-file-fixer`, `check-merge-conflict`, `check-added-large-files` | prek builtins |
| `actionlint` (workflow syntax), `zizmor` (workflow security audit) | Workflows. zizmor runs offline |
| `shellcheck` | Shell scripts |
| `ruff-check`, `ruff-format` | `tools/*.py` |
| `validator-lint` | `make -C tools/validator lint`: golangci-lint, `go mod tidy -diff`, govulncheck |
| `element-readmes` | Regenerates `stack/*/README.md` ([Element reference](../develop/elements.md#element-reference)) |
| `lychee` | Link checker: relative links in `docs/`, offline |

Every hook skips `templates/`, `converters/` and `__snapshot__/`: they are YAML only once rendered.

A hook that rewrites a file (`trailing-whitespace`, `end-of-file-fixer`, `ruff-format`,
`element-readmes`) fails. Commit what it wrote:

```sh
prek run --all-files   # rewrites what it can
git add -u             # stage the rewritten files; add a new element's README by path
```

!!! tip
    `prek install` runs the hooks on every commit.

### Documentation site

The `docs` job builds `docs/` with `zensical build --strict`, which fails on a relative `.md` link or
anchor that doesn't resolve, or a missing snippet file. On `main`, `publish` pushes `site/` to the
`gh-pages` branch as a single commit, using a GitHub App. The site is unversioned: for an older
revision, read `docs/` at that revision on GitHub.

!!! warning "Not checked by strict mode"
    Absolute links, `nav` and the content of included snippets
    ([Docs layout](../contributing.md#docs-layout)).

## PR Title

Squash merges make the PR title the commit subject and the release-note line, so it must match:

```text
<type>(<scope>)!: <subject>
```

`type` is one of `feat fix docs chore ci refactor test perf build revert`. `(<scope>)` (lowercase,
digits, `-`) and `!` (breaking) are optional.

```text
feat(validator): real POLICY080       # ok
fix!: rename hubName                  # ok
Revert "feat: add metallb"            # fails: retitle to revert: feat: add metallb
```

Editing the title re-runs the check. In a fork, PR titles become the notes of your own release
tags the same way (`gh release create --generate-notes`); if you don't cut releases, drop
`pr-title` from your ruleset.

## Policy Validate

Runs the [validator](../develop/validation.md) and its tests. Findings are annotated on the PR at
the file and line to fix. Errors fail the job, warnings only annotate.

The job runs twice. The leg that counts uses the Helm in `mise.toml`, 3.18.4: Argo CD renders every
element with the Helm bundled in OpenShift GitOps, and OpenShift GitOps 1.18, the oldest PolicyStack
supports, bundles Helm 3.18.4, so
a pass here means the hub renders it too. The second leg uses Helm 4.2.1 to preview the next GitOps
Helm. It is advisory (`continue-on-error`): its failure shows on the PR but doesn't fail `ci`. It
becomes required when Argo CD 3.5 ships in OpenShift GitOps or Helm 3 reaches end of life on
2027-02-10, whichever comes first.

```sh
make -C tools/validator validate   # (1)!
make -C tools/validator test       # (2)!
```

1.  Three validator passes ([What `validate` runs](../develop/validation.md#what-validate-runs)).
    POLICY080 needs the [CRDs](../develop/validation.md#crd-schemas), cached between runs.
2.  Last, so a failing test does not hide the annotations. Helm 3 leg only.

!!! note
    The Helm floor in `mise.toml` moves only when the supported OpenShift GitOps floor does
    ([Installation](../start/install.md)). Renovate does not bump it.

### Your fleet (POLICY052)

The `fleet/` pass renders your fleet files from the PR's tree, not each file's `revision`: a cluster
pinned to tag `v1.0` is validated against the PR's code. POLICY052 checks the pin itself: that it
resolves and that its commit has what the fleet file uses ([Revision](fleet.md#revision)). To see what a pinned cluster actually gets, read
[Render Diff](#render-diff).

### Install chart (POLICY071)

`install/` is run by hand on each hub, so nothing else renders it. The check fails when:

- `helm lint install` fails.
- `helm template install` succeeds with an empty `hubName` or `gitRepo`. Neither has a usable
  default: `hubName` picks the hub's own fleet file and the clusters it builds, `gitRepo` is the fork Argo CD reads.
- `policyNamespace` differs between `install/values.yaml` and `values.yaml`, which the elements read.
- An ApplicationSet (or its Application template), the GitOpsCluster or its Placement lacks
  `velero.io/exclude-from-backup: "true"`. ACM backup restores onto a passive hub, where a restored
  ApplicationSet would rebuild the old hub's clusters ([Hub disaster recovery](../guides/lifecycle.md#hub-disaster-recovery)).
- `install/operators` doesn't render the ACM and GitOps operators plus a MultiClusterHub
  ([Installation](../start/install.md)).

## Render Diff

Comments on the PR with what the merge changes in each `fleet/` cluster's rendered Policies. Upstream
ships no fleet files, so this only does something in a fork. It never blocks a merge.

```markdown
### Fleet render: 3 cluster(s) change

> [!WARNING]
> `edge-3` pins `v1.4`, which is not a tag, branch or commit. Merging stops Argo CD from generating Applications for every cluster until it is fixed.

| Cluster | Revision | Changes |
|---|---|---|
| `prod-east-1` | `main` | user-monitoring: 1 Policy changed |
| `nonprod-west-1` | `v1.0.0` → `main` | metallb: **render failed** |
| `edge-2` | new at `main` | 6 elements, 21 objects added |
```

### What it renders

Each fleet file is rendered at its own `revision`, resolved as POLICY052 does,
once before and once after the merge (`.github/scripts/render-diff.sh`). A cluster is rendered only
when its resolved commit or its fleet file changes. So a cluster pinned to `main` or `HEAD` shows the
PR's changes, and a cluster pinned to a tag shows changes only when the PR edits its fleet file
(`revision`, `valueFiles`, `version`, `channel`, `install`, `hub`). `lifecycle/` is rendered for
fleet files that set `hub`. `install/` is not rendered: a change there reaches every cluster at the
next `helm upgrade` ([Install chart](../guides/upgrading.md#install-chart)).

It renders with the [baseline](../develop/validation.md#baseline) selector, so Placements show
`name Exists`, not the per-cluster selector. It checks only that the render succeeds: findings come
from Policy Validate.

### Reading the comment

Below the table, each cluster gets one collapsed block holding a diff per
changed element, or the Helm error. `render fixed` means the element failed at the cluster's revision
before the merge and renders now. An element that fails the same way on both sides is not listed. A
cluster added or removed shows totals only. A warning above the table flags a fleet file whose
`revision` doesn't resolve or that the validator can't load at all. If it merges anyway, the
ApplicationSet can't build its list of Applications: it deletes nothing, but no change reaches any
cluster until the fleet file is fixed ([Generators](../concepts/architecture.md#generators)).

No fleet files, or no cluster whose render changes, means no comment. The comment is sticky: a later
push that changes nothing deletes it.

!!! info "Limits"
    - Comments stop near 60,000 characters. The full patch is the run's `render-diff.patch` artifact.
    - A PR from another repository (a contributor's own fork of yours) gets a read-only token, so
      the result goes to the job summary instead of a comment.
    - About 30 seconds per distinct commit rendered.

## Dependencies

Renovate is a bot that opens dependency update PRs. Its config is the org preset
`renovate-config.json` in [`PolicyStack/.github`](https://github.com/PolicyStack/.github);
`renovate.json` only extends it. Renovate runs only where its GitHub App is installed. In a fork you
can leave it off: tool bumps arrive when you
[merge upstream](../guides/upgrading.md#merge-upstream).

| Pinned in | What |
| --------- | ---- |
| `mise.toml` | Every CLI tool, Go and Python |
| `tools/validator/go.mod` | Go libraries and `govulncheck` (`go tool`) |
| `tools/requirements*.txt` | PyYAML, Zensical |
| `.github/workflows/` | Actions, by commit SHA |
| `tools/validator/Makefile` | The CRD source SHAs ([CRD schemas](../develop/validation.md#crd-schemas)) |

policy-library is not a dependency Renovate moves: it is `lib/policy-library` in this repo, and
elements declare a floor on it ([Library changes](../contributing.md#library-changes)).
