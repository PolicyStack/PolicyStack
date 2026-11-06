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
| Framework | `install/`, `lifecycle/`, `lib/`, `tools/`, the copied workflows and hooks, the root `values.yaml`, plus the surfaces in [Public API](#public-api) | `vX.Y.Z`, recorded in `/.copier-answers.yml` | A breaking change is a framework major. You MUST NOT modify these paths: every framework update has to merge your edits, and a modified framework is not supported. |
| Stable element | Each `stack/<element>` whose `Chart.yaml` sets `policystk.io/stability: stable` (`acm`, `gitops`, `upgrade`), unmodified | `vX.Y.Z`, the release it was copied from, recorded in `stack/<element>/.copier-answers.yml` | A breaking change is a framework major ([Element stability](#element-stability)). |
| Preview element | Every other upstream element, `policystk.io/stability: preview` | Same | A framework minor MAY change values keys, Policy names or output. The release notes say how. |
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

Within a framework major these change only by addition. A key, value, name or behaviour listed
here is never removed, renamed or given a new meaning before the next major.

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
9.  **Tooling.** Validator rule IDs and what each checks, the make targets `validate`, `preview` and
    `deps`, `tools/create-element.sh`, the required checks `ci` and `pr-title`, the copier question
    `element`, and the tag format `vX.Y.Z`.
10. **Stable elements.** Their `config` keys and types, the names of their `policies`,
    `templates` and `operatorPolicies` entries, Policy names other elements depend on, which
    entries are on by default, and the enforcement defaults `enabled`, `remediationAction`,
    `complianceType` and `pruneObjectBehavior`.

### Not covered

These MAY change in any release:

- Output of preview elements, even for unchanged values.
- Internal library defines, and the internals of `tools/*.py` and `tools/*.sh`.
- Validator message text. Rule IDs are covered; wording is not.
- New validator warnings. A warning never fails a PR.
- This documentation, and the example files under upstream's `values/`, which copier doesn't copy.
- Anything a security fix has to change. The release notes say what and why.

## Versions

Upstream has one tag line, `vX.Y.Z`. Every release tag holds the framework and every element, and
release-please cuts it from the Conventional Commit types merged since the last one
([Releases](../contributing.md#releases)). The release notes list every change, element changes
under the element's name (`odf:`), and a breaking change under its own heading.

| Part | Version in | Recorded in your repository |
|---|---|---|
| Framework | The tag, and `install/Chart.yaml` and `lifecycle/Chart.yaml` `version` | `/.copier-answers.yml` `_commit` |
| Element | The tag only. `stack/<element>/Chart.yaml` `version` stays `0.1.0`: nothing reads it, and it changes no output | `stack/<element>/.copier-answers.yml` `_commit` |
| policy-library | `lib/policy-library/Chart.yaml` `version`, `4.x`. Ships inside a framework release | Nothing: it is part of the framework |

Each copy keeps its own answers file, so you can update the framework and each element at
different times. One rule orders them: an element's `_commit` MUST NOT be newer than the root
`_commit`. Update the framework first, then the elements. An element on an older release than the
framework is supported within the same major. After a framework major, update every element to it.
POLICY061 fails an element newer than the framework or on another major.

### Framework versions

- **Major:** anything in [Public API](#public-api) removed, renamed or given a new meaning; a
  breaking change to a stable element; a policy-library major; a naming or `policyNamespace`
  change.
- **Minor:** a new optional key, injected value, fleet key, lifecycle engine or platform, validator
  rule, or a library minor; a new element; a compatible change to a stable element; any change to a
  preview element, breaking ones included.
- **Patch:** a fix.

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
| `preview` | Every other upstream element, and every element `tools/create-element.sh` creates | Any framework minor. The release notes say what to change |

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

- **Minor:** a new optional key or define. Output for values that don't set it is unchanged.
- **Major:** anything removed or renamed, or different output for unchanged values. A library major
  ships only in a framework major, and never changes the naming scheme.
- Each element declares the lowest library it needs as a floor, `version: "^4.N.0"`, and Helm
  refuses to render it against an older one ([Library changes](../contributing.md#library-changes)).

`lib/policy-library-<N>` directories, one per library major, are reserved so that elements you
wrote could keep an older major after a framework major. None exist yet.

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

- **Values keys:** a deprecated key keeps working, and the schema and POLICY035 keep accepting it,
  for at least one minor and at least 90 days. It is removed only in a major. The key's comment
  ends with `Deprecated: use <key>; removed in <version>`, and POLICY039 warns on every layer and
  fleet file that still sets it ([Deprecating a key](../develop/elements.md#deprecating-a-key-or-an-element)).
- **Elements:** a deprecated element sets `deprecated: true` in its `Chart.yaml` and keeps working
  until a major removes it. POLICY092 warns on every values file that turns it on.
- **Validator rules:** a new rule, or a new error case of an existing rule, that can fail a
  repository that passes today ships as a warning for one minor and becomes an error in the next.
  A rule that prevents an outage or a security problem MAY be an error at once. Rule IDs are never
  reused.
- **Fleet keys and injected values:** only added within a major. A new fleet key is optional, and
  leaving it out keeps the old behaviour.

The previous framework major gets security fixes for 6 months after the next major is released.
Other fixes land only in the latest minor.

## Policy names

ACM can't rename a Policy. A changed name deletes the old Policy and creates a new one, which
restarts its compliance history. What the cluster already has stays, unless the template sets `pruneObjectBehavior`
([Removal and pruning](../concepts/removal.md)).

- Changing the naming scheme or `policyNamespace` is a framework major.
- Renaming a policy, template or operator entry of a stable element is a framework major. The release
  notes list every Policy it recreates.
- Element directories are never renamed. A replacement is a new element; the old one is deprecated
  and removed in a major.
- For every cluster that still has a fleet file, Render Diff lists each Policy a PR removes,
  renames included. A removed fleet file shows totals only ([Render Diff](ci.md#render-diff)).

## Lifecycle

`lifecycle/` is framework code and follows framework versions. "Engine" is `install.engine`;
"platform" is `install.platform` for `hypershift`, the cloud under `install.hive.platform` for
`hive`, and `install.siteconfig.templates` for `siteconfig` ([Lifecycle keys](lifecycle.md)).

For a stable engine or platform:

- **Patch:** a fix.
- **Minor:** a new optional key, engine or platform, or a validation every existing fleet file
  already passes.
- **Major:** an `install` or fleet key removed, renamed or given a new meaning; a new default for a
  key that is fixed at create (`engine`, `platform`, `services`, `namespace`); a new name for an
  object the hub creates (HostedCluster, NodePool `<cluster>-<pool>`, ClusterDeployment,
  ClusterInstance, namespaces).

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

Stable engines and platforms follow the rules above. A preview one MAY change incompatibly in a
framework minor; the release notes say what to change. Promotion to stable happens in a minor once
its build, upgrade, scale and destroy paths have been checked on live clusters.

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
| The library version moves with every library change | `ct lint --check-version-increment` in upstream CI |
| Fleet pins stay inside the skew window | POLICY052 reads `/.copier-answers.yml` at each `revision`: newer than the branch's major, or more than one major older, fails |
| A hub never renders a library major it wasn't built for | The install chart injects `policystack.libraryMajors`; policy-library fails the render when its own major isn't listed |
| Answers files say what was copied | POLICY061: every `_commit` is a `vX.Y.Z` tag, each element's names its own directory, and no element's is newer than the root's or on another major |
| Stability is `stable` or `preview` | `tools/doc-generator.py`, run by the `element-readmes` hook, fails on any other value |
| A breaking change gets a major | Conventional Commit `!`, read by release-please, plus Render Diff in review |
| Tags never move | GitHub tag rulesets on `v*` in upstream ([GitHub setup](../contributing.md#github-setup)) |
| Conflict markers never merge | The `check-merge-conflict` hook in `ci`, and the render in `validate` |

## Support

PolicyStack is community-supported. Red Hat supports OpenShift, ACM and OpenShift GitOps, not
PolicyStack.

| PolicyStack | OpenShift | ACM | OpenShift GitOps | Helm |
|---|---|---|---|---|
| 1.x | 4.20 or later | 2.16 or later | 1.18 or later (Argo CD 3.1) | 3.18.4, as bundled in GitOps 1.18. Helm 4 is tested, advisory |

A floor rises only after Red Hat ends maintenance for that version, announced in the release
before. Upstream supports the framework and unmodified elements. Report vulnerabilities
through [private vulnerability reporting](https://github.com/PolicyStack/PolicyStack/security/advisories/new).
