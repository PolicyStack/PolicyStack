# Contributing

## Setup

```sh
git clone https://github.com/<you>/PolicyStack.git && cd PolicyStack
mise install    # every tool at the version CI uses (mise.toml)
prek install    # optional: run the hooks on each commit
```

Before opening a PR, run what CI runs ([CI](reference/ci.md#ci)):

```sh
prek run --all-files
make -C tools/validator ci
```

## Pull requests

- The title is a Conventional Commit ([PR Title](reference/ci.md#pr-title)). Squash merges make it the commit subject and the release-note line.
- Mark a breaking change with `!` and the `breaking` label, which lists it first in the release notes: anything that changes element keys, values layout or fleet files for existing users.
- `ci` and `pr-title` must pass, plus one approval from `.github/CODEOWNERS`.
- Update `docs/` in the same PR. A fix for an incident adds a regression case to the rule's tests.

## Releases

PolicyStack releases are `v0.x.y` tags with generated notes (`.github/release.yml`), cut by hand from `main`. `v1.0.0` comes once the element, values and fleet contract is frozen.

```sh
gh release create v0.2.0 --target main --generate-notes
```

!!! note
    The tag ruleset lets only maintainers create, move or delete `v*` tags. Fleet files pin these tags ([Revision](reference/fleet.md#revision)).

## Chart changes

The policy-library chart lives in [PolicyStack-chart](https://github.com/PolicyStack/PolicyStack-chart). Every chart in `CHARTS` of `tools/validator/Makefile` pins it: each element, `lifecycle/`, `tools/sample-element/` and the validator's test chart.

=== "Minor or patch"

    1. Chart PR. Its `consumers` job runs [Policy Validate](reference/ci.md#policy-validate) on PolicyStack `main` against the chart under test. It must be green.
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

## Adding a lifecycle engine

An engine `<e>` is a component of `lifecycle/`. `templates/policy.yaml` renders whichever component `install.engine` names, so the ApplicationSet and fleet keys do not change.

1. Add `converters/_<e>/`, `_` so the library doesn't discover it as a policy ([Partials](develop/elements.md#partials-and-file-reuse)): objects to create, `root.yaml` (the root object by identity alone, for destroy and ownership), and its credentials. `lookup` back every field the engine rejects changes to. End the cleanup converter with `include "lifecycle.cleanupManifests"`.
2. Add `stack.<e>` to `values.yaml` with `present` and `absent` Policies following the [chains](reference/lifecycle.md#policy-chains): each template names its converter with `file:` (`_<e>/root`, or a shared one such as `guard`), and `orderManifests` lists the steps. Set `default.remediationAction: enforce` and every gate to `InformOnly`, which ACM keeps inform under a Policy-level `enforce`. No `pruneObjectBehavior`: it lets a Policy delete a cluster.
3. Add `<e>` to the `install.engine` enum in `values.schema.json`, with its keys. POLICY090 fails when the enum and `stack` keys differ ([Validator rules](reference/rules.md)).
4. Merge `lifecycle.marker` into the root object's annotations in the cluster converter, and start every enforced converter but `manifests` with `include "lifecycle.refuse"`. Add the engine to `TestOwnership`, a present and an absent fleet file to `tools/validator/testdata/clusters/` ([Fixtures](develop/validation.md#fixtures)), and an example to [Cluster lifecycle](guides/lifecycle.md). Add the engine to [Pick an engine](guides/lifecycle.md#pick-an-engine) and its tab or column to every table in [Lifecycle keys](reference/lifecycle.md).

An engine that upgrades from the hub maps `version` onto its objects, like `hypershift`. One that only installs uses `version` at create and leaves upgrades to `upgrade` ([OpenShift upgrades](guides/upgrades.md)).

## Docs layout

Each page belongs to one section:

| Section | Holds |
| ------- | ----- |
| Home (`index.md`) | What PolicyStack is, the diagram, where each section starts |
| Get started (`start/`) | The path from an empty hub to the first configured cluster, in order |
| Concepts (`concepts/`) | How PolicyStack works and why. No procedures |
| Guides (`guides/`) | One operator task per page. Link Reference for key lists |
| Elements (`elements/`) | One page per `stack/<el>` that needs more than its README. No settings tables |
| Develop (`develop/`) | Writing, previewing and validating elements |
| Reference (`reference/`) | Complete key, rule and workflow tables. No narrative |
| Contributing (`contributing.md`) | Framework maintenance |

- Element READMEs (`stack/*/README.md`) are generated ([Hooks](reference/ci.md#hooks)) and only on GitHub, not the site. An element with a page here sets `home:` in its `Chart.yaml` to the page URL, and its README links it.
- A new page needs a `nav` entry in `zensical.toml`.
- A moved page needs a `redirect_maps` entry in `zensical.toml`.
- Preview:

    ```sh
    pip install -r tools/requirements-docs.txt
    zensical serve            # live preview, run from the repo root
    zensical build --strict   # the CI check
    ```
