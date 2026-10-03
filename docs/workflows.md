# CI Workflows

The GitHub Actions workflows live in `.github/workflows/`. The checks run on pull requests and on
pushes to `main`. Update Documentation fixes stale element READMEs on request.

| Workflow | Runs on | Fails when |
| -------- | ------- | ---------- |
| Policy Validate (`validate.yml`) | Changes to `stack/`, `values/`, `values.yaml`, `fleet/`, `tools/validator/` | Validator tests fail, the validator reports an error, or a fleet file's `revision` does not resolve to a commit with `stack/` |
| Documentation Check (`docs-check.yml`) | Changes to `stack/`, the doc generator or its tests | Generator tests fail or a `stack/<element>/README.md` is out of date |
| Update Documentation (`docs-update.yml`) | `update-docs` label added to a PR | Not a gate, it fixes what Documentation Check reports |
| Documentation Site (`docs-site.yml`) | Every PR and push to `main` | The strict build finds a relative `.md` link or anchor that does not resolve, or a missing snippet |

The checks also run from the Actions tab (`workflow_dispatch`). A newer push to the same PR cancels
the run in progress.

## Policy Validate

1. Checks gofmt, vets, tests and builds `tools/validator` (`make -C tools/validator lint test build`)
   with the Go version from `tools/validator/go.mod`.
2. Runs the validator against the fixture clusters with `--github`, so each finding is annotated on
   the PR diff at the file and line to fix. Errors fail the job, warnings are annotated only.
3. If `fleet/*.yaml` or `fleet/hubs/*.yaml` exist, runs the validator again with
   `--fixtures-dir fleet`, so a fork checks its fleet files. It renders the PR's `stack/` and
   `values/` for each file, not the tree at the file's `revision`. Upstream ships no fleet files and
   skips this step.
4. Checks every fleet file's `revision`. It must be a tag, a branch or a commit SHA of 5 to 40 hex
   characters, the forms Argo CD resolves, and point at a commit that contains `stack/`. Each failure is annotated on the fleet file. The checkout fetches
   full history (`fetch-depth: 0`) so tags and branches resolve.

The pin check catches typos and pins like `gh-pages`. It cannot catch a branch deleted after the
merge, which fails generation for every cluster. See [Revision](applicationset.md#revision).

Helm is pinned by `HELM_VERSION` in the workflow. Keep it on the major the hub's Argo CD uses.
POLICY080 is skipped. The chart emits only Policy, PolicySet, Placement and PlacementBinding, and
the rule skips those kinds, so it would check nothing.

To reproduce locally, see [Validation](validation.md).

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
