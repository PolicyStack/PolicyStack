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

=== "Minor or patch"

    1. Chart PR. Its `consumers` job runs [Policy Validate](workflows.md#policy-validate) on PolicyStack `main` against the chart under test. It must be green.
    2. Merge. chart-releaser publishes `policy-library-<version>`.
    3. Renovate opens one PR that bumps every pin.
    4. PolicyStack `ci` gates it.

=== "Major"

    Renovate can't land a major: the bump needs the element changes in the same PR.

    1. Chart PR with `!`. `consumers` is red by design: `main` still has the old shape.
    2. Merge and release. No cluster moves; every pin still names the old major.
    3. Renovate opens a separate major PR with only the pins. Its `ci` fails. Close it.
    4. Open the migration PR: pins, elements, values layers, validator and docs in one change. Before the release, validate it with `consumers` against the chart checkout; its `ci` passes only once the release exists.
    5. Merge. Clusters pinned to `main` move with it; tag-pinned clusters move when their `revision` does.

Test a chart checkout before opening the chart PR:

```sh
make -C tools/validator consumers LIBRARY="$PWD/../PolicyStack-chart/charts/policy-library"
```

It packages the checkout as each chart's pinned version, runs `validate`, then deletes the packages.

!!! warning
    `LIBRARY` must be absolute: make runs from `tools/validator`.

**Rollback.** A chart release is never withdrawn: ship a patch, and exclude the bad version in Renovate. On PolicyStack, revert the Renovate or migration PR, or move a fleet file's `revision` back to the previous tag.

### Migrating to policy-library 4

ConfigurationPolicy names don't change (`<policy>-<template>`), so compliance history and anything matching on names hold. Every key change: [Upgrading from 3.x](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#upgrading-from-3x). In PolicyStack, `lifecycle/converters/<engine>/` also moves to `_<engine>/`, with templates named by `file:`.

Your own elements migrate the same way as upstream's. In a fork:

1. Merge the upstream release. Upstream elements arrive migrated. For one you changed, take upstream's copy and re-apply your change in the 4.0 shape:

    ```sh
    git rm -rq stack/<el> && git checkout upstream/main -- stack/<el>
    ```

2. Migrate everything still on 3.x: your elements and every values layer. It is idempotent and skips what is already 4.0.

    === "uv"

        ```sh
        uv run tools/migrate-4.0.py                    # the whole repo
        uv run tools/migrate-4.0.py stack/<el> values  # or only these paths
        ```

    === "pip"

        ```sh
        pip install 'ruamel.yaml>=0.18'
        python tools/migrate-4.0.py
        ```

3. Fix each note it prints by hand. Notes cover what it could not convert, such as a `policyRef` naming no policy.
4. Regenerate the READMEs and run CI locally:

    ```sh
    python tools/doc-generator.py
    prek run --all-files
    make -C tools/validator ci
    ```

5. Open the PR and read the [Render Diff](workflows.md#render-diff). Expected changes only:
    - the namespace monitoring ConfigurationPolicy becomes a label on the operator Namespace;
    - `Policy.spec.remediationAction` appears where it was unset;
    - gates go from `inform` to `InformOnly`;
    - a converter with per-object `complianceType` becomes an `object-templates-raw` list.
