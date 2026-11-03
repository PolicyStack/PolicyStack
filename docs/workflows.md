# CI Workflows

| Workflow (`.github/workflows/`) | Trigger | Fails when |
| -------- | ------- | ---------- |
| [CI](#ci) (`ci.yml`) | Every PR and `main` push | A hook, the validator, a validator test, the doc generator tests or the site build fails |
| [PR Title](#pr-title) (`pr-title.yml`) | Every PR, including title edits | The title is not a Conventional Commit |
| [Policy Validate](#policy-validate) (`validate.yml`) | Called by CI and by the PolicyStack-chart PR CI | Validator error, bad fleet pin, broken install chart, failing test |
| [Render Diff](#render-diff) (`render-diff.yml`) | PRs to the default branch changing `stack/`, `lifecycle/`, `values/`, `values.yaml`, `fleet/`, `tools/validator/` | Not a gate |

A new push to a PR cancels its running checks. CI and Policy Validate can also be started from the Actions tab.

## Required checks

`ci` and `pr-title` are the required checks for `main`. Both names are fixed: the ruleset requires them by name.

`ci` is the last job in `ci.yml`. It always runs and fails if any job it needs failed, was cancelled or was skipped ([alls-green](https://github.com/re-actors/alls-green)). A new job goes in its `needs`, not in the ruleset.

!!! note
    No gating workflow has a `paths` filter. A required check whose workflow is skipped by `on.paths` never reports, so the PR waits on it forever.

## CI

| Job | Runs |
| --- | ---- |
| `lint` | `prek run --all-files --show-diff-on-failure` over `.pre-commit-config.yaml` |
| `validate` | [Policy Validate](#policy-validate) |
| `docs` | Doc generator tests, then `zensical build --strict` |
| `ci` | The required check over `lint`, `validate` and `docs` |
| `publish` | `main` pushes in `PolicyStack/PolicyStack` only, after `ci`: the site to `gh-pages` |

Tool versions come from `mise.toml`. The same run locally:

```sh
mise install
prek run --all-files             # lint
make -C tools/validator ci       # lint, validate, test
```

### Hooks

| Hook | Checks |
| ---- | ------ |
| `check-yaml`, `trailing-whitespace`, `end-of-file-fixer`, `check-merge-conflict`, `check-added-large-files` | prek builtins. `templates/` and `converters/` are skipped: they are YAML only once rendered |
| `actionlint`, `zizmor` | Workflows. zizmor runs offline |
| `shellcheck` | Shell scripts |
| `ruff-check`, `ruff-format` | `tools/*.py` |
| `validator-lint` | `make -C tools/validator lint`: golangci-lint, `go mod tidy -diff`, govulncheck |
| `element-readmes` | Regenerates `stack/*/README.md` ([Element reference](elements.md#element-reference)) |
| `lychee` | Relative links in `docs/`, offline |

A hook that rewrites a file fails. Commit what it wrote:

```sh
prek run element-readmes   # or: python tools/doc-generator.py
git add stack/*/README.md
```

!!! tip
    `prek install` runs the hooks on every commit.

### Documentation site

The `docs` job builds `docs/` with `zensical build --strict`. On `main`, `publish` pushes `site/` to the `gh-pages` branch as a single commit, using a GitHub App. The site is unversioned: for an older revision, read `docs/` at that revision on GitHub.

```sh
pip install -r tools/requirements-docs.txt
zensical serve            # live preview, run from the repo root
zensical build --strict   # the CI check
```

!!! warning "Not checked by strict mode"
    Absolute links, `nav` in `zensical.toml`, and snippet content. Keep the rules section of `tools/validator/README.md` free of relative links, and restart `serve` after editing it.

## PR Title

The PR title must match:

```text
<type>(<scope>)!: <subject>
```

`type` is one of `feat fix docs chore ci refactor test perf build revert`. `(<scope>)` (lowercase, digits, `-`) and `!` (breaking) are optional.

```text
feat(validator): real POLICY080       # ok
fix!: rename hubName                  # ok
Revert "feat: add metallb"            # fails: retitle to revert: feat: add metallb
```

Editing the title re-runs the check.

## Policy Validate

Findings are annotated on the PR at the file and line to fix. Errors fail the job, warnings only annotate.

The job runs twice: with the Helm in `mise.toml` (3.18.4, what OpenShift GitOps 1.18 renders with) and with Helm 4.2.1. The 4.2.1 leg is advisory (`continue-on-error`) until Argo CD 3.5 ships in OpenShift GitOps or Helm 3 reaches end of life on 2027-02-10, whichever comes first.

```sh
make -C tools/validator validate   # (1)!
make -C tools/validator test       # (2)!
```

1.  Three validator passes ([Running it](validation.md#running-it)). Fixture clusters plus `tools/sample-element/` with POLICY081 skipped; every profile in `tools/validator/testdata/profiles/`, sample element included, with every rule ([ACM templates](validation.md#acm-templates)); POLICY080 needs the [CRDs](validation.md#crd-schemas), cached between runs; `fleet/` when it exists, rendering the PR's tree, not each file's `revision` ([Render Diff](#render-diff) does that).
2.  Last, so a failing test does not hide the annotations. Helm 3 leg only.

**Fleet pins (POLICY052).** Each `revision` must be a tag, branch or commit SHA (5 to 40 hex characters) whose commit contains what the file uses, including every `valueFiles` entry. A pin to the PR's base branch is checked against the merge commit. A `values/clusters/<name>.yaml` with no `fleet/<name>.yaml` or `fleet/hubs/<name>.yaml` is a warning, since a tag-pinned cluster's file lands first ([Onboarding](rollout.md#onboarding-a-cluster)).

```yaml title="fleet/prod-east-1.yaml"
revision: v1.4.0  # (1)!
hub: acm-dc1      # (2)!
version: 4.20.8   # (3)!
```

1.  `v1.4.0` must contain `stack/` and each `valueFiles` entry. A typo or a branch like `gh-pages` fails.
2.  Also needs `lifecycle/converters/hypershift/`.
3.  `version` or `channel` also needs an `upgrade` element that reads them.

!!! warning
    A pinned branch deleted after merge is not caught, and stops Argo CD generating Applications for every cluster. See [Revision](applicationset.md#revision).

**Install chart (POLICY071).** `helm lint install` passes, an empty `hubName` or `gitRepo` fails the install, `policyNamespace` matches between `install/values.yaml` and `values.yaml`, the ApplicationSets, their Applications, the GitOpsCluster and its Placement carry `velero.io/exclude-from-backup: "true"`, and `install/operators` renders the ACM and GitOps operators plus a MultiClusterHub ([Installation](install.md)).

**Chart PRs.** The PolicyStack-chart CI calls this workflow with `library-artifact`, an artifact holding the unpacked `charts/policy-library` directory. It checks out PolicyStack `main` and runs `make -C tools/validator consumers`, which renders every element, `lifecycle/` and the sample element against that chart at their pinned version. The test step is skipped: a chart PR that adds a key must not fail on PolicyStack's own tests. POLICY071 still runs; `install/` does not use policy-library.

!!! note
    The Helm floor in `mise.toml` moves only when the supported OpenShift GitOps floor does ([Installation](install.md)). Renovate does not bump it.

Run it locally: [Validation](validation.md).

## Render Diff

Comments on the PR with what the merge changes in each `fleet/` cluster's rendered Policies. No fleet files or no change means no comment.

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

Each cluster gets a collapsed diff per element below the table, or the Helm error. `render fixed` means it failed on the base branch and renders now. An element that fails the same way on both sides is not listed. A cluster added or removed shows totals only.

Each fleet file renders at its own `revision`, resolved as POLICY052 does, before and after the merge (`.github/scripts/render-diff.sh`). A cluster pinned to `main` or `HEAD` shows the PR. One pinned to a tag changes only when its pin moves.

!!! info "Limits"
    - Comments stop near 60,000 characters. The full patch is the run's `render-diff.patch` artifact.
    - Fork PRs have a read-only token and get the result in the job summary.
    - About 30 seconds per distinct commit rendered.

## Dependencies

Renovate opens the update PRs, from the org preset `renovate-config.json` in [`PolicyStack/.github`](https://github.com/PolicyStack/.github). `renovate.json` only extends it.

| Pinned in | What |
| --------- | ---- |
| `mise.toml` | Every CLI tool, Go and Python |
| `tools/validator/go.mod` | Go libraries and `govulncheck` (`go tool`) |
| `tools/requirements*.txt` | PyYAML, Zensical |
| `.github/workflows/` | Actions, by commit SHA |
| `stack/*/Chart.yaml`, `lifecycle/`, `tools/sample-element/` | policy-library, bumped together in one PR after a chart release ([Contributing](contributing.md#chart-changes)) |
