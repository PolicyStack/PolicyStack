# CI Workflows

The GitHub Actions workflows live in `.github/workflows/`. The checks run on pull requests and on
pushes to `main`. Update Documentation fixes stale element READMEs on request. Render Diff comments on
pull requests.

| Workflow | Runs on | Fails when |
| -------- | ------- | ---------- |
| Policy Validate (`validate.yml`) | Changes to `stack/`, `lifecycle/`, `install/`, `tools/sample-element/`, `values/`, `values.yaml`, `fleet/`, `tools/validator/` | Validator tests fail, the validator reports an error, the install charts check fails, or a fleet file's `revision` does not resolve to a commit with `stack/`, plus `lifecycle/converters/hypershift/` for a file that sets `hub` and an `upgrade` element that reads `version` for a file that sets `version` or `channel` |
| Documentation Check (`docs-check.yml`) | Changes to `stack/`, the doc generator or its tests | Generator tests fail or a `stack/<element>/README.md` is out of date |
| Render Diff (`render-diff.yml`) | PRs to the default branch changing `stack/`, `lifecycle/`, `values/`, `values.yaml` or `fleet/`, in a repo with fleet files | Not a gate. Fails only when the validator does not build |
| Update Documentation (`docs-update.yml`) | `update-docs` label added to a PR | Not a gate, it fixes what Documentation Check reports |
| Documentation Site (`docs-site.yml`) | Every PR and push to `main` | The strict build finds a relative `.md` link or anchor that does not resolve, or a missing snippet |

The checks also run from the Actions tab (`workflow_dispatch`). A newer push to the same PR cancels
the run in progress.

## Policy Validate

1. Checks gofmt, vets and builds `tools/validator` (`make -C tools/validator lint build`) with the
   Go version from `tools/validator/go.mod`.
2. Runs the validator against the fixture clusters with `--github` and `--include-sample-element`,
   so `tools/sample-element/` stays valid. Each finding is annotated on the PR diff at the file and line
   to fix. Errors fail the job, warnings are annotated only.
3. Runs it again with `--only RENDER000,POLICY081` for each profile in
   `tools/validator/testdata/profiles`, which turn every element on, so every converter's ACM
   templates are evaluated ([ACM templates](validation.md#acm-templates)). A finding both profiles
   report is annotated once. This step runs even when step 2 fails, so the PR shows its annotations
   too.
4. If `fleet/*.yaml` or `fleet/hubs/*.yaml` exist, runs the validator again with
   `--fixtures-dir fleet`, so a fork checks its fleet files. It renders the PR's `stack/` and
   `values/` for each file, not the tree at the file's `revision`; [Render Diff](#render-diff) renders
   each at its `revision`. Upstream ships no fleet files and skips this step.
5. Checks every fleet file's `revision`. It must be a tag, a branch or a commit SHA of 5 to 40 hex
   characters, the forms Argo CD resolves, and point at a commit that contains `stack/`. A file that
   sets `hub` also needs `lifecycle/converters/hypershift/`, and one that sets `version` or `channel`
   an `upgrade` element that reads them; older revisions ignore both keys. A pin to the PR's base
   branch is checked against the merge commit. Each failure is annotated on the fleet file. The checkout fetches
   full history (`fetch-depth: 0`) so tags and branches resolve.
6. Checks the install charts, which nothing else renders: `helm lint install`, `hubName` and
   `gitRepo` fail an install when empty, `policyNamespace` in `install/values.yaml` equals the root
   `values.yaml`, and `install/operators` renders two Subscriptions and a MultiClusterHub from the
   `acm` and `gitops` element values ([Installation](install.md)). Runs even when an earlier step fails.
7. Runs the validator's tests (`make -C tools/validator test`), last so a failing test does not hide
   the annotations.

The pin check catches typos and pins like `gh-pages`. It cannot catch a branch deleted after the
merge, which fails generation for every cluster. See [Revision](applicationset.md#revision).

Helm is pinned by `HELM_VERSION` in the workflow. Keep it on the major the hub's Argo CD uses.
POLICY080 is skipped. The chart emits only Policy, PolicySet, Placement and PlacementBinding, and
the rule skips those kinds, so it would check nothing. Step 2 skips POLICY081, which step 3 runs on
every converter. The fleet step keeps it.

To reproduce locally, see [Validation](validation.md).

## Render Diff

Comments on the PR with what the merge changes in each real cluster's rendered Policies. Only
clusters in `fleet/` count: a repo without fleet files, or a PR that changes no cluster's render,
gets no comment.

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

Below the table, each cluster has a collapsed block with the diff of each changed element, or the
Helm error of a failed render. Changes count rendered objects by kind and name. An element that
failed on the base branch and renders now shows `render fixed`, one that fails the same way on both
sides shows nothing. A cluster added or removed gets a total and no diff.

A fleet file whose `revision` does not resolve, or whose tree the validator cannot load, is a
warning above the table instead of a row.

How it works, in `.github/scripts/render-diff.sh`:

1. Reads the fleet files on the base branch (before) and in the PR merge commit (after).
2. Resolves each `revision` like the pin check: tag, branch, then SHA. The default branch or `HEAD`
   means the base tip before and the merge commit after, so a cluster pinned to `main` shows the PR
   and a cluster pinned to a tag changes only when its pin moves. A cluster with the same file and
   commit on both sides is skipped.
3. Renders each remaining fleet file in a worktree at its commit with
   `policystack-validator --only RENDER000 --render-dir`.
4. Compares the two sides per element and posts the comment, edited on every push and deleted when
   nothing changes.

The comment stops at about 60,000 characters, under GitHub's 65,536 limit. The full patch is the
`render-diff.patch` artifact of the run.

PRs from forks and Dependabot get a read-only token, so they get the result in the job summary
instead. The job that holds the write token runs no code from the PR.

Placements carry the `Exists` selector from `tools/validator/testdata/baseline.yaml`, standing in
for the one the ApplicationSet injects. Both sides get the same one, so it never shows as a change.

Every rendered commit gets a fresh worktree and runs `helm dependency update` for each chart.
Expect about half a minute per distinct commit.

## Documentation Check

1. Runs the doc generator tests (`python -m unittest tools/test_doc_generator.py`).
2. Runs `python tools/doc-generator.py --check`, which regenerates every element README in memory and
   compares it with `stack/<element>/README.md`, ignoring the `Generated:` timestamp.

If an element README is stale, the `Verify Documentation` check fails with an annotation saying how
to fix it:

- run `python tools/doc-generator.py` and commit `stack/*/README.md`, or
- add the `update-docs` label to the PR.

## Update Documentation

Runs when the `update-docs` label is added to a PR from a branch in this repository.

1. Regenerates `stack/*/README.md` from the PR branch, skipping READMEs that differ only in the
   `Generated:` timestamp.
2. If anything changed, commits it as `github-actions[bot]` with the message
   `docs: update documentation [auto-generated]` and pushes to the PR branch.
3. A push made with `GITHUB_TOKEN` does not start workflows, so it dispatches Documentation Check and
   Policy Validate on the branch to check the new commit.
4. Removes the label.

PRs from forks are skipped. Their token is read-only. Run the generator locally instead.

## Documentation Site

Builds this site from `docs/` with `zensical build --strict`. PRs only build.

On `main`, a separate job publishes the build. It pushes `site/` plus `.nojekyll` to the `gh-pages`
branch as a single commit, authenticated as a GitHub App. Pages serves the site from the root of
`gh-pages` (Settings > Pages > Deploy from a branch).

Only that App can change `gh-pages`. `GITHUB_TOKEN` cannot be a ruleset bypass actor, and its pushes
do not start a Pages build. One-time setup:

| Where | Setting |
| ----- | ------- |
| Org Settings > GitHub Apps | An App with no webhook and only **Contents: Read and write**, installed on this repository only |
| Settings > Environments > `gh-pages-publish` | Deployment branches: `main` only. Variable `DOCS_APP_CLIENT_ID`, secret `DOCS_APP_PRIVATE_KEY` |
| Settings > Rules > Rulesets | Branch ruleset on `gh-pages`: Restrict creations, Restrict updates, Restrict deletions, Block force pushes. Bypass list: the App only, Always allow |

Keep all four rules in that ruleset: every publish force-pushes, and the App bypasses only the rules
of rulesets that list it. Leave the `github-pages` environment as it is; the Pages build runs from
`gh-pages`.

The site is unversioned and built from `main`. For a cluster pinned to an older revision, read
`docs/` at that revision on GitHub.

To build locally, run from the repo root, where snippet paths resolve:

```sh
pip install -r tools/requirements-docs.txt
zensical serve            # live preview
zensical build --strict   # the CI check
```

`serve` does not watch the validator README, restart it after editing the rules table. Strict mode
does not check absolute links, `nav` entries in `zensical.toml`, or content included through
snippets. Keep the rules section of `tools/validator/README.md` free of relative links.

## Dependencies

Actions are pinned to commit SHAs, with the version in a trailing comment. Dependabot
(`.github/dependabot.yml`) opens weekly PRs for the actions, the validator's Go modules and the
Python pins in `tools/requirements.txt` and `tools/requirements-docs.txt` (the same `/tools` pip
entry). The Helm version in `validate.yml` is not covered and is updated by hand.
