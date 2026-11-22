# Compatibility

What upstream PolicyStack promises not to break, how each part is versioned, and what is checked
automatically. Read it before you take an update you can't review line by line, or before you
change a file copier manages. How updates arrive: [Upgrading PolicyStack](../guides/upgrading.md).

MUST, MUST NOT, SHOULD and MAY are used as in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

## Tiers

Every path in your repository belongs to one tier. The tier decides who changes it, which version
number it carries and what upstream promises about it.

| Tier | Paths | Version | Promise |
|---|---|---|---|
| Framework | `install/`, `lifecycle/`, `lib/`, `tools/`, `upgrades/`, the copied workflows and hooks, the root `values.yaml`, plus the surfaces in [Public API](#public-api) | `vX.Y.Z`, recorded in `/.copier-answers.yml` | A breaking change is a framework major, preview lifecycle engines included. You MUST NOT modify these paths: every framework update has to merge your edits, and a modified framework is not supported. |
| Stable element | Each `stack/<element>` whose `Chart.yaml` sets `policystk.io/stability: stable` (`acm`, `gitops`, `upgrade`), unmodified | `vX.Y.Z`, the release it was copied from, recorded in `stack/<element>/.copier-answers.yml` | A breaking change is a framework major ([Element stability](#element-stability)). |
| Preview element | Every other upstream element, `policystk.io/stability: preview` | Same | The same promise as a stable element. Preview says only that it isn't proven on live clusters ([Element stability](#element-stability)). |
| Yours | `fleet/`, `values/`, `.github/CODEOWNERS`, `README.md`, and every element you modified, ejected or wrote | Yours | None. Upstream never writes these after the first copy, with one exception: `mise run update` applies upstream's [upgrade steps](launcher.md#upgrade-steps) to `values/` and `fleet/`, as a separate commit on the update branch that you review before merging. |

An element counts as unmodified while its files equal the tag in its answers file. Once you edit
one, it is yours: updates still arrive, and your edits are merged with them
([Resolving conflicts](../guides/upgrading.md#resolving-conflicts)), but upstream supports it only
if the problem reproduces on the unmodified element. To compare:

```sh
mise x -- uv run --project tools/policystack copier copy -d element=odf \
  --vcs-ref "$(sed -n 's/^_commit: //p' stack/odf/.copier-answers.yml)" \
  https://github.com/PolicyStack/PolicyStack.git /tmp/odf
diff -r -x .copier-answers.yml -x charts -x Chart.lock /tmp/odf stack/odf # (1)!
```

1.  `charts/` and `Chart.lock` are what `helm dependency update` writes, gitignored.

The tiers and paths in your repository: [The repository](../concepts/overview.md#the-repository).

## Unchanged values

At every maturity, stable or preview, unchanged values render unchanged behaviour within a major.
New behaviour ships off, or behind a key that defaults to the old behaviour. This covers every
element, `lifecycle/`, `install/` and policy-library.

| Change | Within a major |
|---|---|
| A bug fix that changes what a cluster gets | Ships opt-in. Output that failed to render, or that the API server or ACM rejected, had no behaviour to keep and is fixed directly |
| A better default | Ships opt-in, its comment says `Default changes to <value> in <version>`, and POLICY043 warns whoever hasn't chosen. The major flips every such default together |
| A new policy, template or element | Ships off |
| A key, entry or Policy renamed or removed, or a template that stops accepting a value it took, ACM templates included | Not allowed: a major, after [Deprecation](#deprecation) |
| An operator channel or version default | MAY move in a minor, named in the release notes |
| A security fix with no compatible path | The one exception. A `fix` without `!`, with a subject that starts with `breaking:`, and release notes that say what changed and why |

Formatting, comments and `description` annotations are not behaviour. Upstream CI renders the last
release and each PR with the same values and fails on any other difference a maintainer hasn't
approved ([Compatibility](ci.md#compatibility)).

!!! example "Fixing a bug without breaking it"
    `musthave` only adds to lists, so removing a registry from `allowedRegistries` never revoked it.
    Replacing the template would also have removed every registry added by hand. Instead `allowlist`
    stays as it was, `allowlist-exact` ships off, and 2.0.0 turns it on.

## Public API

Within a framework major these change only by addition, except where an item says otherwise. A
key, value, name or behaviour listed here is never removed, renamed or given a new meaning before
the next major.

1.  **Install chart.** The values `gitRepo`, `hubName` and `policyNamespace`, the release name
    `policystack` in `openshift-gitops`, and the names of the objects it creates
    ([Install chart](install.md)).
2.  **Values the ApplicationSets inject.** `version`, `channel`, `selector` and
    `policystack.libraryMajors` into every Application; `hub`, `state` and `install` into
    lifecycle Applications ([Order](../concepts/values.md#order)).
3.  **Values cascade.** The element's `values.yaml`, the root `values.yaml`, each `valueFiles`
    entry, then `values/clusters/<cluster>.yaml`, in that order. A missing file is skipped.
4.  **Fleet files.** `fleet/<cluster>.yaml` and `fleet/hubs/<hubName>.yaml`, with the keys
    `revision`, `valueFiles`, `hub`, `state`, `install`, `version` and `channel`
    ([Fleet files](fleet.md)). The top-level key `apiVersion` is reserved; a file without it is v1.
5.  **Values schema.** The top-level keys `stack`, `global`, `placement`, `install` and
    `policyNamespace`, and every key policy-library's schema accepts under `stack.<element>`
    ([policy-library reference](https://github.com/PolicyStack/PolicyStack/blob/main/lib/policy-library/README.md#reference)).
6.  **Element authoring.** `include "policy-library.render" .` and its `.component` override,
    `policy-library.enabled`, `policy-library.clusterSuffix`, the `alias: stack` dependency,
    discovery of `converters/<policy>/<template>.yaml`, `_` partials, `templates.<t>.file`,
    `.Config` and the defaults chain ([How an element is built](../concepts/elements.md)). Every
    other `policy-library.*` define is internal.
7.  **Rendered names.** Applications `<element>-<cluster>` and `lifecycle-<cluster>`, Policies
    `<policy>-<release>`, ConfigurationPolicies `<policy>-<template>`, the `install-<operator>`
    templates, Placements, PlacementBindings and PolicySets, the `pstack` namespace, and the
    length budgets in [Naming limits](../concepts/policies.md#naming-limits)
    ([Rendered names](../concepts/elements.md#rendered-names)).
8.  **Labels and annotations.** The annotations `lifecycle.policystk.io/hub` and
    `validator.policystk.io/allow-destructive`, the `Chart.yaml` annotation
    `policystk.io/stability` and its values `stable` and `preview`, the label `velero.io/exclude-from-backup`, and the
    cluster labels in [Labels](fleet.md#labels).
9.  **Tooling.** Validator rule IDs, the make targets `validate`, `preview` and `deps`,
    `tools/create-element.sh`, the required checks `ci` and `pr-title`, the copier question
    `element`, and the tag format `vX.Y.Z`, with each release commit carrying only its own
    `vX.Y.Z` tag. The [launcher](launcher.md): its commands, flags and exit codes, the mise tasks
    `ps`, `update` and `validate`, and the formats of `upgrades/<name>.yaml` and
    `upgrades/blocked.yaml`. A rule may be narrowed or retired in a minor; a change that makes it
    fail a repository that passes today follows [Deprecation](#deprecation). A retired ID is never
    reused.
10. **Elements, at every maturity.** Their `config` keys and types, the names of their `policies`,
    `templates` and `operatorPolicies` entries, their Policy names, which entries are on by default,
    and the enforcement defaults `enabled`, `remediationAction`, `complianceType` and
    `pruneObjectBehavior`.

### Not covered

These MAY change in any release:

- Internal library defines, and the internals of `tools/*.py` and `tools/*.sh`.
- Formatting, comments and `description` annotations in rendered output.
- Validator and launcher message text, the update plan included. Rule IDs and exit codes are
  covered; wording is not.
- New validator warnings. A warning never fails a PR.
- This documentation, and the example files under upstream's `values/`. `install` copies the
  ones you pick, and from then on they are yours.
- A change a security fix can't make compatibly. It is a `fix` without `!`, with a subject that
  starts with `breaking:`, and the release notes say what changed and why.

## Versions

The contract applies from `v1.0.0`. Prereleases (`vX.Y.Z-rc.N`) carry no promise: anything may
change before the release they precede.

Upstream has one tag line, `vX.Y.Z`. Every release tag holds the framework and every element, and
release-please cuts it from the Conventional Commit types merged since the last one
([Releases](../contributing.md#releases)). The release notes list every `feat`, `fix`, `perf` and
`revert`, each element change under the element's name (`odf:`), and every `!` change under its
own heading. The only breaking change without `!` is the security exception in
[Unchanged values](#unchanged-values): its subject starts with `breaking:` and says what to change,
under the element's name or `lifecycle`, so searching for `breaking:` finds every one.

| Part | Version in | Recorded in your repository |
|---|---|---|
| Framework | The tag, and `install/Chart.yaml` and `lifecycle/Chart.yaml` `version` | `/.copier-answers.yml` `_commit` |
| Element | The tag only. `stack/<element>/Chart.yaml` `version` stays `0.1.0`: nothing reads it, and it changes no output | `stack/<element>/.copier-answers.yml` `_commit` |
| policy-library | `lib/policy-library/Chart.yaml` `version`, `4.x`. Ships inside a framework release | Nothing: it is part of the framework |

Each copy keeps its own answers file, so you can update the framework and each element at
different times. One rule orders them: an element's `_commit` MUST NOT be newer than the root
`_commit`. Update the framework first, then the elements. An element on an older release than the
framework keeps working within the same major; fixes to it still need an element update. After a
framework major, update every element to it. POLICY061 fails an element newer than the framework
or on another major.

### Framework versions

- **Major:** anything in [Public API](#public-api) removed, renamed or given a new meaning; a change
  to behaviour for unchanged values in any element, engine or platform
  ([Unchanged values](#unchanged-values)); anything [Lifecycle](#lifecycle) holds; a policy-library
  major; a naming or `policyNamespace` change.
- **Minor:** a new optional key that defaults to the old behaviour, injected value, fleet key,
  lifecycle engine or platform, validator rule, or a library minor; a new element, or a policy or
  template that ships off; a moved operator channel or version default.
- **Patch:** a fix that leaves behaviour for unchanged values as it was, or fixes output that failed
  to render or was rejected, and the security exception.

A floor rise in [Support](#support) is a minor.

### Element stability

`policystk.io/stability` in an element's `Chart.yaml` says what upstream promises about it. The
element's generated `README.md` shows it near the top. An element you write may leave it out.

```yaml title="stack/acm/Chart.yaml"
annotations:
  policystk.io/stability: stable
```

| Stability | Elements | Means | A breaking change ships in |
|---|---|---|---|
| `stable` | `acm`, `gitops`, `upgrade` | Proven | A framework major only |
| `preview` | Every other upstream element, and every element `tools/create-element.sh` creates | Not proven on live clusters: test it on one before production | A framework major only |

Breaking means any of:

- Behaviour for unchanged values changed ([Unchanged values](#unchanged-values)).
- A `config` key removed, renamed, or narrowed to fewer types or values.
- A policy, template or operator entry renamed.
- An enforcement default flipped.
- The library floor raised to a new library major.

A new key with the old behaviour as its default, a new policy or template that ships off, a moved
operator channel or version default, or a library floor raised within the major is a framework
minor.

A preview element becomes stable in a framework minor, by changing its annotation. A stable element
never goes back to preview within a major.

### policy-library versions

- **Patch:** a fix. Output for unchanged values changes only where it failed to render, or the API
  server or ACM rejected it. The PR names the bug. A fix to any other output is a new key that
  defaults to the old output, until the major.
- **Minor:** a new optional key or define. Output for values that don't set it is unchanged.
- **Major:** anything removed or renamed, or any other change to output for unchanged values. A
  library major ships only in a framework major, and never changes the naming scheme.
- Each element declares the lowest library it needs as a floor, `version: "^4.N.0"`, and Helm
  refuses to render it against an older one ([Library changes](../contributing.md#library-changes)).

Each framework release holds one library major, in `lib/policy-library`. Elements you wrote move
to a new library major when you take the framework major that ships it.

## Hub and revision skew

A hub runs one install chart, upgraded by hand, and renders every cluster at the revision its
fleet file pins. Those revisions can be on older framework releases than the hub.

- **Same major:** a hub on `X.Y` renders every revision from `X.0` to `X.Y`. Upgrade the hub before
  moving any pin to a newer release: an element from that release may read a value only the newer
  install chart injects.
- **Across majors:** a hub on `N` renders revisions on `N` and on the latest `N-1` release. Move every
  pin to the latest `N-1` release, upgrade the hubs to `N`, then move pins to `N`.

The order and commands: [Framework updates](../guides/upgrading.md#framework-updates).

## Deprecation

These rules bind the Public API and every element and engine, whatever its maturity. `install` keys
follow [Lifecycle](#lifecycle).

- **Values keys:** a deprecated key keeps working, and the schema and POLICY035 keep accepting it,
  for at least one minor and at least 90 days. It is removed only in a major. The key's comment
  ends with `Deprecated: use <key>; removed in <version>`, and POLICY039 warns on every layer and
  fleet file that still sets it ([Deprecating a key](../develop/elements.md#deprecating-a-key-or-an-element)).
- **Elements:** a deprecated element sets `deprecated: true` in its `Chart.yaml` and keeps working
  until a framework major removes it.
  POLICY092 warns on every values file that turns it on. Once it is removed, updates skip it and
  list it as `removed upstream`, and an update to a new major is refused until you
  [eject](../guides/upgrading.md#skip-freeze-eject-roll-back) it to keep it as yours, or remove it.
- **Defaults:** a default that will change keeps its value until the major. Its comment says
  `Default changes to <value> in <version>`, and POLICY043 warns wherever it is in use and no layer
  sets the key ([Changing a default](../develop/elements.md#changing-a-default)).
- **Validator rules:** a new rule, or a new error case of an existing rule, that can fail a
  repository that passes today ships as a warning for at least one minor before it becomes an
  error. A rule that prevents an outage or a security problem MAY be an error at once. Rule IDs are
  never reused.
- **Fleet keys and injected values:** only added within a major, at every maturity, `install` keys
  of preview engines included. A new fleet key is optional, and leaving it out keeps the old
  behaviour.

Fixes, security fixes included, land only in the latest minor. Older minors and majors get no
backports: to get a fix, update.

## Policy names

ACM can't rename a Policy. A changed name deletes the old Policy and creates a new one, which
restarts its compliance history. What the cluster already has stays, unless the template sets `pruneObjectBehavior`
([Removal and pruning](../concepts/removal.md)).

- Changing the naming scheme or `policyNamespace` is a framework major.
- Renaming a policy, template or operator entry of any element is a framework major. The release
  notes list every Policy it recreates.
- Element directories are never renamed. A replacement is a new element; the old one is deprecated
  and removed as in [Deprecation](#deprecation).
- For every cluster that still has a fleet file, Render Diff lists each Policy a PR removes,
  renames included. A removed fleet file shows totals only ([Render Diff](ci.md#render-diff)).

## Lifecycle

`lifecycle/` is framework code and follows framework versions. "Engine" is `install.engine`;
"platform" is `install.platform` for `hypershift`, the cloud under `install.hive.platform` for
`hive`, and `install.siteconfig.templates` for `siteconfig` ([Lifecycle keys](lifecycle.md)).

At every maturity, these change only in a framework major:

- The name or namespace of an object the hub creates: HostedCluster, NodePool `<cluster>-<pool>`,
  ClusterDeployment, ClusterInstance and their namespaces.
- The default of any key [Lifecycle keys](lifecycle.md) marks fixed at create, and of `engine`,
  `services` and `namespace`.
- An `install` key removed, renamed or given a new meaning. A preview engine that replaces a key
  deprecates the old one ([Deprecation](#deprecation)) and accepts both until the next major.

Fleet files are read at `HEAD`, and each revision's `lifecycle/` rejects `install` keys it doesn't
know. Set a new `install` key only on a fleet file whose `revision` has the release that added it.

For every engine and platform, whatever its maturity:

- **Patch:** a fix, as [Unchanged values](#unchanged-values) allows one.
- **Minor:** a new optional key, engine or platform, or a validation every existing fleet file
  already passes.
- **Major:** any other incompatible change, such as a converter that builds a different object
  from the same keys.

!!! danger "A renamed hub object can cost you a cluster"
    The lifecycle Policy finds what it built by name. Under a new name it creates a new object next
    to the old one: a second HostedCluster, ClusterDeployment or ClusterInstance, or a second
    NodePool, while the old one keeps running unmanaged. Getting back means destroying one of them
    by hand. A Policy rename costs compliance history; this can cost a cluster. Read the lifecycle
    section of a framework major's notes before any hub that builds clusters renders it.

Not covered at any maturity: `hostedCluster`, `nodePools`, `hive.installConfig` and
`hive.clusterDeployment`. They are merged into Red Hat's objects as you write them, and follow the
HyperShift and Hive APIs.

### Maturity

Every engine and platform follows the rules above. Preview says only that its build, upgrade, scale
and destroy paths haven't been checked on live clusters. Promotion to stable happens in a minor,
once they have.

| Engine | Platform | Maturity |
|---|---|---|
| `hypershift` | `KubeVirt` | Preview |
| `hypershift` | `Agent` | Preview |
| `hypershift` | `None` | Preview |
| `hive` | `aws` | Preview |
| `hive` | `gcp` | Preview |
| `hive` | `azure` | Preview |
| `hive` | `vsphere` | Preview |
| `siteconfig` | `ai` | Preview |
| `siteconfig` | `ibi` | Preview |

!!! warning "Every engine and platform is preview at 1.0"
    None is proven on live clusters. Pin hub-built clusters to tags, test a build and a destroy, and
    read the lifecycle section of a release's notes before moving their pins ([Rolling out changes](../guides/rollout.md#choosing-a-pin)).

## How it is enforced

| Rule | Check |
|---|---|
| An element gets the library it needs | Helm's `^4.N.0` constraint on the `file://` dependency. Helm's error is generic (`can't get a valid version for 1 subchart(s)`); POLICY060 names the fix |
| The library version moves with every library change | `ct lint --check-version-increment` in upstream CI. It checks that `version` rose, not by how much: the `lib/tests` snapshot diff and review decide patch, minor or major |
| Fleet pins stay inside the skew window | POLICY052 reads `/.copier-answers.yml` at each `revision`: newer than the branch's major, or more than one major older, fails. Yours to check: that an N-1 pin is the latest N-1 release, and that hubs are upgraded before pins move |
| A hub never renders a library major it wasn't built for | The install chart injects `policystack.libraryMajors`; policy-library fails the render when its own major isn't listed |
| Answers files say what was copied | POLICY061: every `_commit` is a `vX.Y.Z` tag, each element's names its own directory, and no element's is newer than the root's or on another major |
| Stability is `stable` or `preview` | `tools/doc-generator.py`, run by the `element-readmes` hook, fails on any other value. It skips elements with a `.copier-answers.yml`, so in your repository it checks only elements you wrote or ejected |
| Unchanged values, unchanged output | Upstream's `compat` job renders the last release and the PR with the release's values and fails on a new difference, unless a maintainer's label approves exactly it ([Compatibility](ci.md#compatibility)). It sees what the fixtures and scenarios set, and no runtime behaviour beyond its upgrade test |
| A breaking change gets a major | The PR author marks it with `!`, a reviewer checks it before adding the `output-change-<hash>` label the job prints, and release-please bumps the major |
| A default that will change warns first | POLICY043 reads the comment. Review checks that there is one |
| Deprecation windows | Review only. POLICY039 warns on a deprecated key, but doesn't check the version its comment names |
| A new error ships as a warning first | Review only |
| A security exception says `breaking:` | Review only. `pr-title` accepts any subject |
| Tags never move | GitHub tag rulesets on `v*` in upstream. The launcher refuses a release tag that moved since this machine last fetched it |
| A release commit carries only its tag | The launcher refuses a release whose commit carries a second tag, because copier would record that one. release-please tags one package |
| Conflict markers never merge | The launcher refuses to commit one or to start while one is committed; the `check-merge-conflict` hook in `ci`, over every file except `tools/validator/crds/`; and the render in `validate` |
| Majors one at a time | The launcher makes the last release of each major a required stop and moves every element on a major ([Required stops](launcher.md#required-stops)) |

## Support

PolicyStack is community-supported. Red Hat supports OpenShift, ACM and OpenShift GitOps, not
PolicyStack.

| PolicyStack | OpenShift | ACM | OpenShift GitOps | Helm |
|---|---|---|---|---|
| 1.x | 4.20 or later | 2.16 or later | 1.18 or later (Argo CD 3.1) | 3.18.4, as bundled in GitOps 1.18. Helm 4 is tested, advisory |

On the workstation that runs the [launcher](launcher.md#requirements): git 2.43 or later, make, and
mise, which installs the Python (3.14) and uv that `mise.toml` pins.

CI runs no cluster. It renders with Helm 3.18.4, plus Helm 4.2.1 as an advisory leg, and validates
the output against the ACM 2.16 CRDs. OpenShift and OpenShift GitOps are not tested. Later versions
are expected to work, and a problem on one is fixed like any other bug.

Supported means a bug that reproduces on the latest release, with the framework and the element
unmodified, on versions in the table above, is accepted as a bug. Fixes are best effort, with no
response time, and land only in the latest minor ([Deprecation](#deprecation)).

A floor rises in a framework minor, and only after Red Hat's Maintenance Support for that version
ends. For an OpenShift EUS release, that is the end of Maintenance Support, not of the EUS terms.
The minor before announces it: a maintainer adds the line to that release's GitHub notes and to the
table here. The Helm floor is the Helm the GitOps floor bundles, and rises with it. Until a floor
rises, a key, policy or template that needs a newer version ships off, and its comment or
`description` names the version. Operator channel and version defaults follow
[Element stability](#element-stability).

Report vulnerabilities through [private vulnerability reporting](https://github.com/PolicyStack/PolicyStack/security/advisories/new).
