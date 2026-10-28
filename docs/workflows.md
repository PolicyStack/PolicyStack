# CI Workflows

| Workflow (`.github/workflows/`) | Trigger | Fails when |
| -------- | ------- | ---------- |
| [Policy Validate](#policy-validate) (`validate.yml`) | Changes to `stack/`, `lifecycle/`, `install/`, `values/`, `values.yaml`, `fleet/`, `tools/sample-element/`, `tools/validator/` | Validator error, bad fleet pin, broken install chart, failing test |
| [Documentation Check](#documentation-check) (`docs-check.yml`) | Changes to `stack/` or the doc generator | An element README is stale |
| [Render Diff](#render-diff) (`render-diff.yml`) | PRs to the default branch changing `stack/`, `lifecycle/`, `values/`, `values.yaml`, `fleet/` | Not a gate |
| [Update Documentation](#update-documentation) (`docs-update.yml`) | `update-docs` label on a PR | Not a gate |
| [Documentation Site](#documentation-site) (`docs-site.yml`) | Every PR and `main` push | A relative link, anchor or snippet does not resolve |

Checks run on PRs and pushes to `main`. A new push to a PR cancels its running checks. Policy Validate, Documentation Check and Documentation Site can also be started from the Actions tab.

## Policy Validate

Findings are annotated on the PR at the file and line to fix. Errors fail the job, warnings only annotate.

```sh
# Every validator run also gets --repo-root .; steps 2 and 4 also get --extra-values tools/validator/testdata/baseline.yaml
make -C tools/validator lint build                                                  # (1)!
policystack-validator --github --include-sample-element --skip POLICY080,POLICY081  # (2)!
policystack-validator --github --extra-values "tools/validator/testdata/baseline.yaml,$profile" --only RENDER000,POLICY081 # (3)!
policystack-validator --github --fixtures-dir fleet --skip POLICY080                # (4)!
# fleet pin check, install chart check (below)
make -C tools/validator test                                                        # (5)!
```

1.  gofmt, vet, build. Go version comes from `tools/validator/go.mod`.
2.  Fixture clusters plus `tools/sample-element/`. POLICY080 is always skipped: it ignores every kind the chart emits.
3.  Once per file in `tools/validator/testdata/profiles/`. They turn every element on, so every converter's ACM templates are evaluated ([ACM templates](validation.md#acm-templates)). Runs even if step 2 failed.
4.  Only when `fleet/*.yaml` or `fleet/hubs/*.yaml` exist. Renders the PR's tree, not each file's `revision` ([Render Diff](#render-diff) does that).
5.  Last, so a failing test does not hide the annotations.

**Fleet pin check.** Each `revision` must be a tag, branch or commit SHA (5 to 40 hex characters) whose commit contains what the file uses, including every `valueFiles` entry. A pin to the PR's base branch is checked against the merge commit. A `values/clusters/<name>.yaml` with no `fleet/<name>.yaml` or `fleet/hubs/<name>.yaml` is a warning, since a tag-pinned cluster's file lands first ([Onboarding](rollout.md#onboarding-a-cluster)).

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

**Install chart check.** Runs even if an earlier step failed. `helm lint install` passes, an empty `hubName` or `gitRepo` fails the install, `policyNamespace` matches between `install/values.yaml` and `values.yaml`, and `install/operators` renders the ACM and GitOps operators plus a MultiClusterHub ([Installation](install.md)).

!!! note
    `HELM_VERSION` pins Helm in `validate.yml` and `render-diff.yml`. Keep them equal, on the major the hub's Argo CD bundles. Dependabot does not update it.

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

Each fleet file renders at its own `revision`, before and after the merge (`.github/scripts/render-diff.sh`). A cluster pinned to `main` or `HEAD` shows the PR. One pinned to a tag changes only when its pin moves.

!!! info "Limits"
    - Comments stop near 60,000 characters. The full patch is the run's `render-diff.patch` artifact.
    - Fork and Dependabot PRs have a read-only token and get the result in the job summary.
    - About 30 seconds per distinct commit rendered.

## Documentation Check

```sh
python -m unittest tools/test_doc_generator.py
python tools/doc-generator.py --check  # (1)!
```

1.  Regenerates each `stack/<element>/README.md` in memory and compares.

When `Verify Documentation` fails, fix it either way:

=== "Locally"

    ```sh
    python tools/doc-generator.py
    git add stack/*/README.md
    ```

=== "Label"

    Add the `update-docs` label to the PR.

## Update Documentation

On the `update-docs` label, regenerates `stack/*/README.md`, commits any change as `docs: update documentation [auto-generated]`, pushes it to the PR branch, and removes the label. A `GITHUB_TOKEN` push starts no workflows, so it dispatches Documentation Check and Policy Validate itself. Fork PRs are skipped.

## Documentation Site

Builds `docs/` with `zensical build --strict`. On `main`, a second job publishes `site/` to the `gh-pages` branch as a single commit, using a GitHub App. The site is unversioned: for an older revision, read `docs/` at that revision on GitHub.

```sh
pip install -r tools/requirements-docs.txt
zensical serve            # live preview, run from the repo root
zensical build --strict   # the CI check
```

!!! warning "Not checked by strict mode"
    Absolute links, `nav` in `zensical.toml`, and snippet content. Keep the rules section of `tools/validator/README.md` free of relative links, and restart `serve` after editing it.

??? info "One-time publish setup"
    `GITHUB_TOKEN` cannot bypass rulesets and its pushes do not start a Pages build, hence the App.

    | Where | Setting |
    | ----- | ------- |
    | Org Settings > GitHub Apps | No webhook, only **Contents: Read and write**, installed on this repository only |
    | Settings > Environments > `github-pages` | Deployment branches: `main` and `gh-pages` (the Pages build from `gh-pages` deploys to this environment too). Variable `DOCS_APP_CLIENT_ID`, secret `DOCS_APP_PRIVATE_KEY` |
    | Settings > Rules > Rulesets | On `gh-pages`: Restrict creations, updates, deletions, Block force pushes. Bypass: the App only, Always allow |
    | Settings > Pages | Deploy from branch `gh-pages`, root. Custom domain `policystk.io`, Enforce HTTPS |
    | Org Settings > Pages | Verified domain `policystk.io` |
    | Cloudflare DNS | `policystk.io` A and AAAA to GitHub's Pages IPs, `www` CNAME to `policystack.github.io`, DNS only. Proxied needs SSL mode Full (strict) |

    Keep all four rules: every publish force-pushes. The domain lives in `site_url` and the `CNAME` the publish job writes; change both together.

## Dependencies

Actions are pinned to commit SHAs. Dependabot opens weekly PRs for actions, the validator's Go modules, and `tools/requirements*.txt`.
