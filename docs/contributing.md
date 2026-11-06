# Contributing

This page is for changes to PolicyStack itself, sent upstream: the framework (`install/`,
`lifecycle/`, `lib/`, `tools/`, `.github/`, `docs/`) and the starter elements in `stack/`. Running
PolicyStack from your own repository and taking upstream releases into it is covered in
[Upgrading PolicyStack](guides/upgrading.md); elements you write or eject there are yours and need
nothing here.

## Repositories and versions

| Repository | Holds | Versioned by |
|---|---|---|
| [PolicyStack](https://github.com/PolicyStack/PolicyStack) | The framework, the starter elements and the policy-library chart (`lib/`) | Release tags `vX.Y.Z`, one tag line for the framework and every element, cut by release-please ([Releases](#releases)). Adopters copy and update from them with copier. policy-library has its own chart version in `lib/policy-library/Chart.yaml` ([Library changes](#library-changes)) |
| [PolicyStack/.github](https://github.com/PolicyStack/.github) | Org defaults, including the Renovate preset | Not versioned |

Element versions, deprecating a key or an element, and when to contribute instead of ejecting:
[Versions and deprecation](develop/elements.md#versions-and-deprecation). What each version
promises adopters: [Compatibility](reference/compatibility.md).

"Release" means two things in these docs: the Helm release name Argo CD gives each element on
each cluster (`<element>-<cluster>`), and a PolicyStack GitHub release.

## Setup

Tool versions live in `mise.toml`. [mise](https://mise.jdx.dev) installs each at the version CI
uses: Go, Helm, Python, copier, prek (the hook runner) and the linters.

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

Before opening a PR, run what each job of [CI](reference/ci.md#ci) and
[Upstream](reference/ci.md#upstream) runs:

```sh
prek run --all-files                       # lint
make -C tools/validator validate           # validate (Helm floor leg)
make -C tools/validator lint test          # validator
helm dependency update --skip-refresh lib/tests/fixtures/policy-stack-test && untt --strict lib/tests/fixtures/policy-stack-test   # library
python -m unittest tools/test_doc_generator.py && zensical build --strict   # docs
```

The `library` job also runs `ct lint` (the `version` bump check, [Library changes](#library-changes)).
mise does not install chart-testing, so that step runs in CI only.

The first `validate` needs network access to download Go modules; the CRDs are committed.
`make lint` also fetches the Go vulnerability database (govulncheck).
CI also runs `validate` with Helm 4.2.1; that leg is advisory
([Policy Validate](reference/ci.md#policy-validate)).

## Pull requests

- The title is a Conventional Commit, such as `feat(validator): real POLICY080`. Squash merges make it
  the commit subject and the release-note line ([PR Title](reference/ci.md#pr-title)).
- A breaking change is anything that changes the framework's [Public API](reference/compatibility.md#public-api),
  a stable element's values keys or Policy names for existing users, a stable lifecycle engine, or
  anything [Lifecycle](reference/compatibility.md#lifecycle) holds for a major at every maturity.
  Mark it with `!` in the title: release-please reads it from the squash commit and bumps the major
  ([Releases](#releases)).
  Any other breaking change to a preview element or a preview lifecycle engine ships in a minor:
  no `!`, and a subject that starts with `breaking:` and says what to change, such as
  `feat(odf): breaking: rename config.pools to config.ipAddressPools` or
  `feat(lifecycle): breaking: ...`. The notes then list it under `odf:` with a subject starting
  `breaking:`, which adopters search for
  ([Element stability](reference/compatibility.md#element-stability)).
- `ci`, `upstream` and `pr-title` must pass, plus one approval from `.github/CODEOWNERS`: any
  member of `@PolicyStack/maintainers`.
- Update `docs/` in the same PR.
- A fix for a bad render or policy that reached a cluster adds a test that would have caught it:
  extend the validator rule that covers it, or add a rule. Check files are named by topic, not rule
  ID: `grep -l POLICY033 tools/validator/internal/checks/*.go` finds `prune.go`, tests in
  `prune_test.go`. ACM template evaluation is in `internal/acmtemplates/`.
- A new rule needs a row in the rules table of `tools/validator/README.md`
  ([Validator rules](reference/rules.md) includes it; `make test` fails without it) and in the phase
  table of [What it checks](develop/validation.md#what-it-checks), which nothing tests.

## Validator rules

A rule ID is never reused, even after its rule is removed. The severity is the `Severity` of the
`Finding` the rule's `Run` returns. A new rule, or a new error case in an existing rule, that can fail
a repository that passes today ships as a warning for at least one framework minor before it
becomes an error. The release notes of the minor that adds the warning and of the minor that makes
it an error say so. A rule that prevents an outage or a security
problem may be an error from the start. The comment above `All()` in
`tools/validator/internal/checks/checks.go` carries the same rule for whoever adds one.

## Releases

release-please cuts every release. There is one package, the whole repository, and one tag line,
`vX.Y.Z`: each release holds the framework and every element. On each `main` push, the `release`
job of [Upstream](reference/ci.md#upstream) opens or updates one release PR. Merging it tags its
merge commit and creates the GitHub release with the notes from the PR.
`.release-please/config.json` configures the package, and `.release-please/manifest.json` records
the last release.

| Tag | Counts commits that change | Version written to |
|---|---|---|
| `vX.Y.Z` | Anything except `.release-please/`, `docs/`, `diagrams/`, `values/`, `lib/tests/` and the validator fixtures | `CHANGELOG.md`, and the `version` lines of `install/Chart.yaml` and `lifecycle/Chart.yaml` marked `x-release-please-version` |

The squash commit's type sets the bump: `fix` a patch, `feat` a minor, `!` a major. `docs`, `chore`,
`ci`, `test` and `refactor` commits release nothing. The scope becomes the release-note prefix, so
scope an element change with the element's name, such as `feat(odf): add config.flexibleScaling`:
the notes then list it as `odf: add config.flexibleScaling`, which is how adopters find the changes
to the elements they track. Element `Chart.yaml` `version` lines are never bumped.

`exclude-paths` matches whole directories only. Some files copier leaves out of copies sit outside
those directories: `README.md`, `zensical.toml`, `.github/workflows/upstream.yml`,
`tools/test_doc_generator.py` and the rest of the upstream-only list in `copier.yml`. A `fix` or
`feat` that changes only those still cuts a release, and adopters get an update that changes
nothing but the version. Title those PRs `docs`, `ci`, `test` or `chore`.

A new element needs no release setup. `tools/create-element.sh` adds copier's answers template, and
the element ships in the next release.

## Library changes

policy-library is the Helm library chart every element renders through. It lives in
`lib/policy-library`, with its helm-unittest suites in `lib/tests`. Every chart that uses it declares
it the same way:

```yaml title="stack/gitops/Chart.yaml"
dependencies:
  - name: policy-library
    alias: stack
    version: "^4.0.0"                              # floor: the lowest 4.x this chart needs
    repository: "file://../../lib/policy-library"  # relative to the chart directory
```

Those charts are each element, `lifecycle/` (`file://../lib/policy-library`),
`tools/sample-element/` and the validator's test chart. `make deps` loops over `CHARTS` in
`tools/validator/Makefile`, so a new chart outside `stack/` must be added there.

Helm resolves `file://` at render time. Argo CD's repo server and the validator package
`lib/policy-library` into the chart's `charts/` from the same Git revision they render. `charts/` and
`Chart.lock` are gitignored. A cluster pinned to a tag therefore renders the library as it was at
that tag, and a library change reaches a cluster in the same commit as the element changes that use
it. Nothing is published, and there is no chart repo to reach.

`version` is a floor, not a pin. `^4.2.0` accepts any 4.x at or above 4.2.0, and the chart always
gets whatever `lib/` holds. If that does not satisfy the constraint, Helm fails with a generic
`can't get a valid version for 1 subchart(s)`; POLICY060 reports the same mistake with the fix
([Validator rules](reference/rules.md)).

A library change is one PR:

1. Change `lib/policy-library`, and in the same PR the elements, validator and docs that depend on it.
2. Bump `version` in `lib/policy-library/Chart.yaml`. Patch: a fix, which may change output only
   where it was a bug. Minor: a new optional key or define. Major: anything removed or renamed, or
   any other change to output for unchanged values
   ([policy-library versions](reference/compatibility.md#policy-library-versions)). `ct lint`
   in the [Upstream](reference/ci.md#upstream) `library` job fails a PR that changes
   `lib/policy-library` without a bump.
3. An element that needs the new minor raises its floor to `^4.N.0`. The others keep theirs.
4. Run the suites and commit the updated snapshots (`lib/tests/**/__snapshot__`). A snapshot diff
   for values that don't use a new key means the change is a major, unless the PR names the bug
   the diff fixes.

    ```sh
    helm dependency update --skip-refresh lib/tests/fixtures/policy-stack-test
    untt --strict lib/tests/fixtures/policy-stack-test  # -u rewrites snapshots
    make -C tools/validator validate test
    ```

A major moves every floor to `^5.0.0` in that same PR, with the element and values changes it needs,
and is a breaking change (`!`). It also adds `5` to the list in `install/templates/_helpers.tpl`,
keeping `4`. Both ApplicationSets pass that list to every Application as `policystack.libraryMajors`,
and policy-library fails the render of a revision whose library major the hub's install chart doesn't
list. A cluster pinned to a 4.x revision keeps rendering on a hub upgraded to the new install chart,
and a cluster moved to 5.x fails, naming the fix, on a hub that hasn't been upgraded.
`TestTemplatePatch` fails when the list lacks the major in `lib/policy-library/Chart.yaml`.
To roll back, revert the PR; fleet revisions from before it keep
rendering the library they had.

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
