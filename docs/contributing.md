# Contributing

This page is for changes to PolicyStack itself, sent upstream: the framework (`install/`,
`lifecycle/`, `tools/`, `.github/`, `docs/`), the starter elements in `stack/`, and the
policy-library chart. Running PolicyStack from your own fork and taking upstream changes into it is
covered in [Upgrading PolicyStack](guides/upgrading.md#merge-upstream); changes you make to elements
in that fork are yours and need nothing here.

## Repositories and versions

| Repository | Holds | Versioned by |
|---|---|---|
| [PolicyStack](https://github.com/PolicyStack/PolicyStack) | The framework and the starter elements | `vX.Y.Z` git tags with release notes ([Releases](#releases)). Nothing consumes them |
| [PolicyStack-chart](https://github.com/PolicyStack/PolicyStack-chart) | The policy-library Helm chart every element depends on | Chart semver (`4.0.0`), published at `https://policystack.github.io/PolicyStack-chart`. Every chart pins one exact version ([Chart changes](#chart-changes)) |
| [PolicyStack/.github](https://github.com/PolicyStack/.github) | Org defaults, including the Renovate preset | Not versioned |

An element's own `Chart.yaml` `version` (`0.1.0`) is never bumped: Argo CD renders elements from a
Git path and never packages them.

"Release" means three things in these docs: the Helm release name Argo CD gives each element on
each cluster (`<element>-<cluster>`), a PolicyStack GitHub release, and a chart release.

## Setup

Tool versions live in `mise.toml`. [mise](https://mise.jdx.dev) installs each at the version CI
uses: Go, Helm, Python, prek (the hook runner) and the linters.

```sh
git clone https://github.com/<you>/PolicyStack.git && cd PolicyStack
mise install    # run `mise trust` first if mise says the config is not trusted
eval "$(mise activate bash)"   # put this in your shell rc; see `mise activate --help` for others
pip install -r tools/requirements.txt -r tools/requirements-docs.txt
prek install    # optional: run the hooks on each commit
```

`mise install` only downloads the tools. Until `mise activate` puts them on `PATH`, `prek` and `go`
are missing, and `make` silently uses whatever `helm` your system has, which may not render the way
the hub does. The hooks call the mise tools directly, so they need the same `PATH`.

Before opening a PR, run what each [CI](reference/ci.md#ci) job runs:

```sh
prek run --all-files                       # lint
make -C tools/validator validate test      # validate (Helm floor leg)
python -m unittest tools/test_doc_generator.py && zensical build --strict   # docs
```

The first `validate` needs network access: it downloads Go modules, policy-library and the CRDs.
`prek run` also fetches the Go vulnerability database (govulncheck, in the `validator-lint` hook).
CI also runs `validate` with Helm 4.2.1; that leg is advisory
([Policy Validate](reference/ci.md#policy-validate)).

## Pull requests

- The title is a Conventional Commit, such as `feat(validator): real POLICY080`. Squash merges make it
  the commit subject and the release-note line ([PR Title](reference/ci.md#pr-title)).
- A breaking change is anything that changes element values keys, the `values/` layout or fleet
  file keys for existing users. Mark it with `!` in the title; a maintainer adds the `breaking`
  label. The title check only needs `!`; the release notes group by label, so without the label the
  change is listed with the rest.
- `ci` and `pr-title` must pass, plus one approval from `.github/CODEOWNERS`: any member of
  `@PolicyStack/maintainers`.
- Update `docs/` in the same PR.
- A fix for a bad render or policy that reached a cluster adds a test that would have caught it:
  extend the validator rule that covers it, or add a rule. Check files are named by topic, not rule
  ID: `grep -l POLICY033 tools/validator/internal/checks/*.go` finds `prune.go`, tests in
  `prune_test.go`. ACM template evaluation is in `internal/acmtemplates/`.
- A new rule needs a row in the rules table of `tools/validator/README.md`
  ([Validator rules](reference/rules.md) includes it; `make test` fails without it) and in the phase
  table of [What it checks](develop/validation.md#what-it-checks), which nothing tests.

## Releases

A PolicyStack release is a `v0.x.y` tag with notes generated from the merged PR titles
(`.github/release.yml`), cut by hand from `main`. It builds no artifact and nothing consumes it:
adopters merge `upstream/main` and fetch upstream without its tags, so their fleet files pin their
own fork's tags ([Merge upstream](guides/upgrading.md#merge-upstream)). `v1.0.0` comes once the
contract is frozen: element values keys, the `values/` layout and fleet file keys, for the framework
and unmodified elements.

```sh
gh release create v0.2.0 --target main --generate-notes
```

!!! note
    A tag ruleset lets only maintainers create, move or delete `v*` tags, so release history can't
    be rewritten. An adopter protects their own fork's tags the same way, since those are what its
    fleet files pin.

## Chart changes

The policy-library chart lives in
[PolicyStack-chart](https://github.com/PolicyStack/PolicyStack-chart). A pin is the exact `version` of
the `policy-library` dependency (alias `stack`) in a `Chart.yaml`. Every chart in `CHARTS` of
`tools/validator/Makefile` pins it: each element, `lifecycle/`, `tools/sample-element/` and the
validator's test chart. `make deps` and `make consumers` loop over that list, so a new chart that
uses policy-library outside `stack/` must be added there, or chart PRs never test it.

When a chart PR merges, chart-releaser creates a GitHub release holding the `.tgz` and updates the
index Helm reads. As soon as the version is published, Renovate, a bot, opens one PR that moves
every pin at once.

=== "Minor or patch"

    1. **Chart PR.** Bump `version` in `charts/policy-library/Chart.yaml` (chart-testing's
       `ct lint` fails without it, and chart-releaser publishes only versions with no release yet)
       and commit the updated helm-unittest snapshots (`tests/**/__snapshot__`). Its `consumers`
       job runs [Policy Validate](reference/ci.md#policy-validate) on PolicyStack `main` against
       the chart under test. It must be green.
    2. **Merge.** chart-releaser publishes `policy-library-<version>`.
    3. **Renovate** opens one PR that bumps every pin.
    4. **PolicyStack `ci`** gates it.

=== "Major"

    Renovate can't land a major: the pins and the element changes the new major needs must be one PR.

    1. **Chart PR with `!`.** `consumers` is red by design: it renders PolicyStack `main`, still
       written for the old major, against the new chart. A maintainer merges it through the ruleset
       bypass.
    2. **Merge.** chart-releaser publishes it. Nothing moves: every pin still names the old major.
    3. **Renovate's major PR.** Renovate opens a PR with only the pins; its `ci` fails because the
       elements aren't migrated. Close it: Renovate then ignores that major until the pins move by
       hand, and minors and patches arrive as usual.
    4. **Migration PR:** pins, elements, values layers, validator and docs in one change. You can
       prepare it before step 2 and test it locally against the chart checkout (below). If you open
       it before step 2, its `ci` stays red until the chart is published: `helm dependency update`
       can't fetch an unpublished version, so RENDER000 reports every element unrendered.
    5. **Merge.** Upstream has no clusters, so nothing moves here. In a fork, clusters pinned to its
       default branch move when it merges upstream, and tag-pinned clusters when their `revision`
       does.

Test a chart checkout before opening the chart PR, with PolicyStack-chart cloned next to
PolicyStack, from the PolicyStack root:

```sh
make -C tools/validator consumers LIBRARY="$PWD/../PolicyStack-chart/charts/policy-library"
```

It packages the checkout under each chart's pinned version, so the validator renders it instead of
the published chart, and runs `validate` (not `lint` or `test`). On exit it deletes every
`charts/policy-library-*.tgz` in those charts, so the next `validate` downloads the published chart
again.

!!! warning
    `LIBRARY` must be absolute: make runs from `tools/validator`.

**Rollback.** A chart release is never withdrawn. Argo CD downloads the pinned version every time it
renders, so any fleet `revision` whose `Chart.yaml` pins a removed version would stop rendering. Ship
a patch, and exclude the bad version in the Renovate preset in PolicyStack/.github (a
`packageRules` entry for `policy-library` with `allowedVersions`). On PolicyStack, revert the
Renovate or migration PR; in a fork, move a fleet file's `revision` back to the previous tag.

## Adding a lifecycle engine

An engine is the tool the hub builds a cluster with: `hypershift`, `hive` or `siteconfig`
([Cluster lifecycle](concepts/lifecycle.md)). Each fleet file that sets `hub` gets an Application
`lifecycle-<cluster>` rendering `lifecycle/`. Its `templates/policy.yaml` picks the
`stack.<engine>` block (a component) of `lifecycle/values.yaml` that `install.engine` names, and
turns on its `present` or `absent` Policy by `state`. That Policy's templates run in order, each
rendering one converter. Adding an engine adds converters, one component and schema keys; the
ApplicationSet and the fleet keys don't change.

HyperShift as the worked example:

```text
lifecycle/
├── converters/
│   ├── guard.yaml, manifests.yaml, detach.yaml,   # shared by every engine
│   │   managedcluster.yaml, destroy.yaml
│   └── _hypershift/
│       ├── root.yaml           # the HostedCluster by identity alone
│       ├── credentials.yaml    # pull secret, SSH key, etcd encryption key
│       ├── hostedcluster.yaml  # the cluster: HostedCluster and NodePools
│       ├── ready.yaml          # gate: waits for the rollout
│       └── cleanup.yaml        # deletes what credentials copied, then install.manifests
├── values.yaml                 # stack.hypershift: the present and absent Policies
└── values.schema.json          # install.engine enum and per-engine keys
```

Rendered for fixture `hcp-kubevirt`, Policy `present-lifecycle-hcp-kubevirt` holds, in order,
`present-guard-hcp-kubevirt` (`InformOnly`), `present-credentials-hcp-kubevirt`,
`present-manifests-hcp-kubevirt`, `present-cluster-hcp-kubevirt` (enforced) and
`present-ready-hcp-kubevirt` (`InformOnly`).

1.  **Converters in `converters/_<e>/`.** The `_` keeps policy-library from discovering the
    directory as a Policy `<e>` of every component. Without it, every lifecycle render runs those
    converters, which fails the render for clusters of the other engines (`install.siteconfig.nodes
    is required`) or ships an extra enforced Policy `<e>-lifecycle-<cluster>`. Shared converters sit directly in `converters/` for the same reason
    ([Partials](develop/elements.md#partials-and-file-reuse)). An engine needs:
    - `root.yaml`: the object the engine creates first and deletes to destroy the cluster, with only
      `apiVersion`, `kind`, name and namespace. The shared `destroy` step, the `destroyed` gate and
      the ownership check read it.
    - `credentials.yaml`, the cluster converter, a ready gate (`ready.yaml`; Hive's is
      `installed.yaml`) and `cleanup.yaml`, which ends with `include "lifecycle.cleanupManifests"` so
      the hub objects from `install.manifests` are deleted once the cluster is gone.

    Some fields can't change after create: Hive rejects `provisioning.releaseImage` changes. Read
    those back from the live object, so a later `version` bump never produces a rejected update:

    ```yaml title="lifecycle/converters/_hive/clusterdeployment.yaml"
    {{ "{{-" }} $releaseImage := dig "spec" "provisioning" "releaseImage" {{ $image | quote }} (lookup "hive.openshift.io/v1" "ClusterDeployment" {{ $c | quote }} {{ $c | quote }}) {{ "}}" }}
    ```

2.  **Ownership** ([Ownership](concepts/lifecycle.md#ownership)). In the cluster converter, merge
    the `lifecycle.marker` helper (`lifecycle/templates/_helpers.tpl`, like the other `lifecycle.*`
    helpers below) into the root object's annotations. If the engine creates the ManagedCluster
    itself, mark that too: Hive writes the marker on it, SiteConfig passes it through
    `extraAnnotations`. When the root object doesn't exist, as after a failed create, the guard
    falls back to the ManagedCluster, and an unmarked one reads as an imported cluster that every
    step refuses to touch. Start every enforced converter except `manifests` with
    `include "lifecycle.refuse"`, before its first object entry (Helm variables may come first). It
    fails the step in the same evaluation as the write, so a stale guard result can never let a
    step touch a cluster lifecycle didn't build. Because it is an ACM template, the step renders as
    `object-templates-raw`; `TestEnforceIsRaw`
    (`tools/validator/internal/acmtemplates/ownership_test.go`) checks that. An engine that can
    adopt an existing object (SiteConfig's ClusterDeployment) also adds a branch to
    `lifecycle.foreign`.

3.  **Component in `values.yaml`.** Copy an existing `stack.<engine>` block whole and change its
    `file:` paths. Keep:
    - `enabled: true`. Without it policy-library renders nothing and reports no error: the cluster
      is never built.
    - `suffixTemplateNames: true`. Every lifecycle release lands on the hub, so without the
      `-<cluster>` suffix two clusters' templates share a name and ACM rejects the second.
    - `file:` on every template: there is no `converters/present/`, so a template without one fails
      the render.
    - The template names of the block you copied (hypershift and hive: `guard`, `credentials`,
      `manifests`, `cluster`, `ready`; `guard`, `detach`, `detached`, `destroy`, `destroyed`,
      `cleanup`; siteconfig's absent chain has no `detach`/`detached`) and `orderManifests`, which
      makes each wait for the one before. `TestOwnership`
      (`tools/validator/internal/acmtemplates/ownership_test.go`) finds the guard and skips `ready`
      and `manifests` by name.
    - `default.remediationAction: enforce` with every gate `InformOnly`, which ACM keeps inform under
      a Policy-level `enforce`. No `pruneObjectBehavior`: it lets a Policy delete a cluster.

4.  **Schema.** Add `<e>` to the `install.engine` enum in `values.schema.json`. For engine settings,
    add an `install.<e>` object and an `allOf` entry that requires its keys when `engine` is `<e>`
    and sets `maxProperties: 0` otherwise, as `hive` and `siteconfig` do. `install` allows no other
    keys, so a fleet file setting `install.<e>` fails the render until the schema has it. POLICY090
    fails when the enum and the `stack` keys differ.

5.  **Validator.** Add every kind the converters `lookup` to `kinds` in
    `tools/validator/internal/acmtemplates/acmtemplates.go`, or POLICY081 fails. Add a present and
    an absent fixture to `tools/validator/testdata/clusters/`, to the want lists of
    `TestLoadDir_testdata` and `TestResolve_testdata`, and to the [fixture list](develop/validation.md#fixtures).
    Add a `TestOwnership` row (fixtures and the root's `apiVersion`, `kind` and namespace) and an
    `<e>_test.go` in `internal/acmtemplates`: profiles never reach `lifecycle/`.

6.  **Engine-specific code.** Check each and extend it if the engine needs it:
    `lifecycle.releaseImage` picks `-x86_64` for `siteconfig` and `-multi` for every other engine;
    POLICY052 checks only that a pinned revision has `lifecycle/converters/_hypershift/`, so a fleet
    file pinned to a revision without the new engine passes CI and then fails to render; POLICY051
    reads only `install.siteconfig.nodes`.

7.  **Docs.** An example in [Building clusters](guides/lifecycle.md), a row in
    [Pick an engine](concepts/lifecycle.md#pick-an-engine), and its tab or column in every table of
    [Lifecycle keys](reference/lifecycle.md).

An engine that upgrades from the hub writes `version` into its objects on every evaluation, as
`hypershift` does with the HostedCluster and NodePool release. One that only installs uses `version`
at create, reads the field back afterwards, and leaves upgrades to the `upgrade` element, which sets
ClusterVersion on the cluster from the fleet file's `version` and `channel`
([OpenShift upgrades](guides/upgrades.md)).

## Docs layout

Write for a first-time reader: model first, every rule with its reason, worked examples, failure
modes. Explain a concept in full on one page and link it everywhere else.

Each page belongs to one section:

| Section | Holds |
| ------- | ----- |
| Home (`index.md`) | What PolicyStack is, the diagram, where each section starts |
| Get started (`start/`) | The path from an empty hub to the first configured cluster, in order |
| Concepts (`concepts/`) | How PolicyStack works and why, in reading order: overview, ACM policies, elements, values, placement, lifecycle, removal, ApplicationSets. No procedures |
| Guides (`guides/`) | One operator task per page. Link Concepts for the model and Reference for key lists |
| Elements (`elements/`) | One page per `stack/<el>` that needs more than its README. No settings tables |
| Develop (`develop/`) | Writing, previewing and validating elements |
| Reference (`reference/`) | Complete key, rule and workflow tables, each opening with what it is for. No narrative |
| Contributing (`contributing.md`) | Changing PolicyStack itself |

- Element READMEs (`stack/*/README.md`) are generated ([Hooks](reference/ci.md#hooks)) and only on
  GitHub, not the site. An element with a page here sets `home:` in its `Chart.yaml` to the page URL,
  and its README links it.
- A new page needs a `nav` entry in `zensical.toml`. Strict mode doesn't check `nav`: a missing
  entry leaves the page out of the menu, and an entry pointing at a missing file ships as a dead
  link.
- A moved page or section needs a `redirect_maps` entry in `zensical.toml`.
- `reference/rules.md` includes the rules section of `tools/validator/README.md`. A relative link
  resolves against the including page, not the README, so it breaks on one side: keep that section
  free of relative links.
- Preview from the repo root, where snippet paths resolve:

    ```sh
    zensical serve            # live preview; restart it after editing the README rules table
    zensical build --strict   # what CI runs, after python -m unittest tools/test_doc_generator.py
    ```
