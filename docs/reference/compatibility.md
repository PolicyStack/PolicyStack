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
| Framework | `install/`, `lifecycle/`, `lib/`, `tools/`, the copied workflows and hooks, the root `values.yaml`, plus the surfaces in [Public API](#public-api) | `vX.Y.Z`, recorded in `/.copier-answers.yml` | A breaking change is a framework major, except where [Maturity](#maturity) lets a preview lifecycle engine change in a minor. You MUST NOT modify these paths: every framework update has to merge your edits, and a modified framework is not supported. |
| Stable element | Each `stack/<element>` whose `Chart.yaml` sets `policystk.io/stability: stable` (`acm`, `gitops`, `upgrade`), unmodified | `vX.Y.Z`, the release it was copied from, recorded in `stack/<element>/.copier-answers.yml` | A breaking change is a framework major ([Element stability](#element-stability)). |
| Preview element | Every other upstream element, `policystk.io/stability: preview` | Same | A framework minor MAY change values keys, Policy names or output, except the Policy names in [Public API](#public-api) item 10. The release notes say how. |
| Yours | `fleet/`, `values/`, `.github/CODEOWNERS`, `README.md`, and every element you modified, ejected or wrote | Yours | None. Upstream never writes these after the first copy. |

An element counts as unmodified while its files equal the tag in its answers file. Once you edit
one, it is yours: updates still arrive, and your edits are merged with them
([Resolving conflicts](../guides/upgrading.md#resolving-conflicts)), but upstream supports it only
if the problem reproduces on the unmodified element. To compare:

```sh
copier copy --vcs-ref "$(sed -n 's/^_commit: //p' stack/odf/.copier-answers.yml)" -d element=odf \
  https://github.com/PolicyStack/PolicyStack.git /tmp/odf
diff -r -x .copier-answers.yml -x charts -x Chart.lock /tmp/odf stack/odf # (1)!
```

1.  `charts/` and `Chart.lock` are what `helm dependency update` writes, gitignored.

The tiers and paths in your repository: [The repository](../concepts/overview.md#the-repository).

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
    `element`, and the tag format `vX.Y.Z`. A rule may be narrowed or retired in a minor; a change
    that makes it fail a repository that passes today follows [Deprecation](#deprecation). A retired
    ID is never reused.
10. **Stable elements.** Their `config` keys and types, the names of their `policies`,
    `templates` and `operatorPolicies` entries, Policy names other elements depend on, which
    entries are on by default, and the enforcement defaults `enabled`, `remediationAction`,
    `complianceType` and `pruneObjectBehavior`. Also the names of other elements' Policies that a
    stable element depends on, whatever their stability: today `odf` `ready`, which `acm`
    observability waits for.

### Not covered

These MAY change in any release:

- Output of preview elements, even for unchanged values.
- Internal library defines, and the internals of `tools/*.py` and `tools/*.sh`.
- Validator message text. Rule IDs are covered; wording is not.
- New validator warnings. A warning never fails a PR.
- This documentation, and the example files under upstream's `values/`, which copier doesn't copy.
- A change a security fix can't make compatibly. It is a `fix` without `!`, with a subject that
  starts with `breaking:`, and the release notes say what changed and why.

## Versions

The contract applies from `v1.0.0`. Prereleases (`vX.Y.Z-rc.N`) carry no promise: anything may
change before the release they precede.

Upstream has one tag line, `vX.Y.Z`. Every release tag holds the framework and every element, and
release-please cuts it from the Conventional Commit types merged since the last one
([Releases](../contributing.md#releases)). The release notes list every `feat`, `fix`, `perf` and
`revert`, each element change under the element's name (`odf:`), and every `!` change under its
own heading. A breaking change to a preview element, or one [Maturity](#maturity) allows in a minor
for a preview engine, has no `!`: its subject starts with `breaking:` and says what to change, under
the element's name or `lifecycle`, so searching for `breaking:` finds every one.

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

- **Major:** anything in [Public API](#public-api) removed, renamed or given a new meaning; a
  breaking change to a stable element or a stable engine or platform; anything
  [Lifecycle](#lifecycle) holds for a major at every maturity; a policy-library major; a naming or
  `policyNamespace` change.
- **Minor:** a new optional key, injected value, fleet key, lifecycle engine or platform, validator
  rule, or a library minor; a new element; a compatible change to a stable element; any change to a
  preview element, breaking ones included; an incompatible change [Maturity](#maturity) allows for a
  preview engine.
- **Patch:** a fix.

A floor rise in [Support](#support) is a minor.

### Element stability

`policystk.io/stability` in an element's `Chart.yaml` says what upstream promises about it. The
element's generated `README.md` shows it near the top. An element you write may leave it out.

```yaml title="stack/acm/Chart.yaml"
annotations:
  policystk.io/stability: stable
```

| Stability | Elements | A breaking change ships in |
|---|---|---|
| `stable` | `acm`, `gitops`, `upgrade` | A framework major only |
| `preview` | Every other upstream element, and every element `tools/create-element.sh` creates | Any framework minor, except the Policy names in [Public API](#public-api) item 10. The release notes say what to change |

For a stable element, breaking means any of:

- A `config` key removed, renamed, or narrowed to fewer types or values.
- A policy, template or operator entry renamed.
- An enforcement default flipped.
- The library floor raised to a new library major.

A new key, a new policy or template that ships off or inform-only, a moved operator channel or
version default, or a library floor raised within the major is a framework minor. A fix that
leaves output for unchanged values identical, except for the bug, is a patch.

A preview element becomes stable in a framework minor, by changing its annotation. A stable element
never goes back to preview within a major.

### policy-library versions

- **Patch:** a fix. Output for unchanged values changes only where it was a bug: it contradicts
  the [library reference](https://github.com/PolicyStack/PolicyStack/blob/main/lib/policy-library/README.md#reference),
  or ACM rejects or misapplies it. The PR names the bug.
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

The **Values keys** and **Elements** rules below bind the Public API, stable elements and stable
engines. A preview element SHOULD deprecate a key for one minor when the old key can keep working,
and MAY rename or remove keys in any minor. `install` keys follow [Lifecycle](#lifecycle) at every
maturity.

- **Values keys:** a deprecated key keeps working, and the schema and POLICY035 keep accepting it,
  for at least one minor and at least 90 days. It is removed only in a major. The key's comment
  ends with `Deprecated: use <key>; removed in <version>`, and POLICY039 warns on every layer and
  fleet file that still sets it ([Deprecating a key](../develop/elements.md#deprecating-a-key-or-an-element)).
- **Elements:** a deprecated element sets `deprecated: true` in its `Chart.yaml` and keeps working
  until a release removes it: any later minor for a preview element, a major for a stable one.
  POLICY092 warns on every values file that turns it on. Once it is removed, `copier update` on it
  fails: [eject](../guides/upgrading.md#skip-freeze-eject-roll-back) it to keep it as yours, or
  remove it.
- **Validator rules:** a new rule, or a new error case of an existing rule, that can fail a
  repository that passes today ships as a warning for at least one minor before it becomes an
  error. A rule that prevents an outage or a security problem MAY be an error at once. Rule IDs are
  never reused.
- **Fleet keys and injected values:** only added within a major, at every maturity, `install` keys
  of preview engines included. A new fleet key is optional, and leaving it out keeps the old
  behaviour.

Fixes, security fixes included, land only in the latest minor. Older minors and majors get no
backports: to get a fix, update. For a preview element, that update brings every change to it since
your release, breaking ones included.

## Policy names

ACM can't rename a Policy. A changed name deletes the old Policy and creates a new one, which
restarts its compliance history. What the cluster already has stays, unless the template sets `pruneObjectBehavior`
([Removal and pruning](../concepts/removal.md)).

- Changing the naming scheme or `policyNamespace` is a framework major.
- Renaming a policy, template or operator entry of a stable element is a framework major. The release
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

For a stable engine or platform:

- **Patch:** a fix.
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

Stable engines and platforms follow the rules above. Apart from the rules that hold at every
maturity, a preview one MAY change incompatibly in a framework minor: its converters, behaviour and
defaults not fixed at create. The release notes say what to change. Promotion to stable happens in
a minor once its build, upgrade, scale and destroy paths have been checked on live clusters.

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
    Pin hub-built clusters to tags, and read the lifecycle section of every framework release's
    notes before moving their pins ([Rolling out changes](../guides/rollout.md#choosing-a-pin)).

## How it is enforced

| Rule | Check |
|---|---|
| An element gets the library it needs | Helm's `^4.N.0` constraint on the `file://` dependency. Helm's error is generic (`can't get a valid version for 1 subchart(s)`); POLICY060 names the fix |
| The library version moves with every library change | `ct lint --check-version-increment` in upstream CI. It checks that `version` rose, not by how much: the `lib/tests` snapshot diff and review decide patch, minor or major |
| Fleet pins stay inside the skew window | POLICY052 reads `/.copier-answers.yml` at each `revision`: newer than the branch's major, or more than one major older, fails. Yours to check: that an N-1 pin is the latest N-1 release, and that hubs are upgraded before pins move |
| A hub never renders a library major it wasn't built for | The install chart injects `policystack.libraryMajors`; policy-library fails the render when its own major isn't listed |
| Answers files say what was copied | POLICY061: every `_commit` is a `vX.Y.Z` tag, each element's names its own directory, and no element's is newer than the root's or on another major |
| Stability is `stable` or `preview` | `tools/doc-generator.py`, run by the `element-readmes` hook, fails on any other value. It skips elements with a `.copier-answers.yml`, so in your repository it checks only elements you wrote or ejected |
| A breaking change gets a major | Review only. The PR author marks it with `!`, a reviewer checks it, and release-please bumps the major. Upstream has no fleet files, so Render Diff shows the change only in your repository: on the update PR for clusters pinned to the default branch, and on the PR that moves a tag pin |
| Deprecation windows | Review only. POLICY039 warns on a deprecated key, but doesn't check the version its comment names |
| A new error ships as a warning first | Review only |
| A preview break says `breaking:` | Review only. `pr-title` accepts any subject |
| Tags never move | GitHub tag rulesets on `v*` in upstream |
| Conflict markers never merge | The `check-merge-conflict` hook in `ci`, over every file except `tools/validator/crds/`, and the render in `validate` |

## Support

PolicyStack is community-supported. Red Hat supports OpenShift, ACM and OpenShift GitOps, not
PolicyStack.

| PolicyStack | OpenShift | ACM | OpenShift GitOps | Helm |
|---|---|---|---|---|
| 1.x | 4.20 or later | 2.16 or later | 1.18 or later (Argo CD 3.1) | 3.18.4, as bundled in GitOps 1.18. Helm 4 is tested, advisory |

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
