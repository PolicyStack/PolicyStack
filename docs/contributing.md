# Contributing

## Setup

```sh
git clone https://github.com/<you>/PolicyStack.git && cd PolicyStack
mise install    # every tool at the version CI uses (mise.toml)
prek install    # optional: run the hooks on each commit
```

Before opening a PR, run what CI runs ([CI](workflows.md#ci)):

```sh
prek run --all-files
make -C tools/validator ci
```

## Pull requests

- The title is a Conventional Commit ([PR Title](workflows.md#pr-title)). Squash merges make it the commit subject and the release-note line.
- Mark a breaking change with `!` and the `breaking` label, which lists it first in the release notes: anything that changes element keys, values layout or fleet files for existing users.
- `ci` and `pr-title` must pass, plus one approval from `.github/CODEOWNERS`.
- Update `docs/` in the same PR. A fix for an incident adds a regression case to the rule's tests.

## Releases

PolicyStack releases are `v0.x.y` tags with generated notes (`.github/release.yml`), cut by hand from `main`. `v1.0.0` comes once the element, values and fleet contract is frozen.

```sh
gh release create v0.2.0 --target main --generate-notes
```

!!! note
    The tag ruleset lets only maintainers create, move or delete `v*` tags. Fleet files pin these tags ([Revision](applicationset.md#revision)).

## Forks

Your fleet pins your own tags. Fetch upstream without its tags, so a `v0.2.0` from upstream never shadows yours:

```sh
git remote add upstream https://github.com/PolicyStack/PolicyStack.git
git config remote.upstream.tagOpt --no-tags
git fetch upstream
```

## Chart changes

The policy-library chart lives in [PolicyStack-chart](https://github.com/PolicyStack/PolicyStack-chart). Every element, `lifecycle/` and `tools/sample-element/` pins it.

1. Chart PR. Its `consumers` job runs [Policy Validate](workflows.md#policy-validate) on PolicyStack `main` against the chart under test. Green, or red by design for a `!` PR.
2. Merge. chart-releaser publishes `policy-library-<version>`.
3. Renovate opens one PR that bumps every pin.
4. PolicyStack `ci` gates it.

Test a chart checkout before opening the chart PR:

```sh
make -C tools/validator consumers LIBRARY="$PWD/../PolicyStack-chart/charts/policy-library"
```

It packages the checkout as each chart's pinned version, runs `validate`, then deletes the packages.

!!! warning
    `LIBRARY` must be absolute: make runs from `tools/validator`.

**Rollback.** A chart release is never withdrawn: ship a patch, and exclude the bad version in Renovate. On PolicyStack, revert the Renovate PR, or move a fleet file's `revision` back to the previous tag.
