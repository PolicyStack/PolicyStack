# Validation

`tools/validator` is a Go program that renders every element (a Helm chart under `stack/`,
[How an element is built](../concepts/elements.md)) the way Argo CD will, then checks the sources
and the result against the [validator rules](../reference/rules.md). CI runs it on every PR
([Policy Validate](../reference/ci.md#policy-validate)). This page is for element authors and for
anyone fixing a finding.

The validator exists because most PolicyStack mistakes are silent or show up late, on a managed cluster:

- Argo CD skips a values file that doesn't exist, so a typo in a
  [fleet file](../reference/fleet.md)'s `valueFiles` (the list of values files a cluster reads)
  silently drops that layer (POLICY050, POLICY052).
- A converter (a template file under an element's `converters/`) that reads a `config` key nobody
  set renders `null`, not an error (POLICY022).
- The hub accepts any typo inside a Policy's `policy-templates`, such as
  `remediationAction: enforec`; it only fails on the managed cluster (POLICY080,
  [CRD schemas](#crd-schemas)).
- ACM templates (the escaped `{{ "{{" }} ... {{ "}}" }}` parts of a converter) are resolved by ACM
  after Helm, so `helm template` never runs them (POLICY081).
- ACM rejects a Policy whose hub copy, `pstack.<policy>-<element>-<cluster>`, is over 63 characters. The name
  grows with the cluster name, so it only fails for long cluster names (POLICY001,
  [Naming limits](../concepts/policies.md#naming-limits)).

## What it checks

The validator never talks to a cluster. It stands in for the fleet with **fixtures**: made-up fleet
files in `tools/validator/testdata/clusters` (see [Fixtures](#fixtures)). For each element and each
fixture it builds the same values cascade the ApplicationSet builds
([Order](../concepts/values.md#order)) and runs `helm template` with it. It also renders
`lifecycle/`, the chart a hub uses to build and destroy clusters, for fixtures that set `hub:` (the
hub that builds them), as release `lifecycle-<cluster>`
([Cluster lifecycle](../concepts/lifecycle.md)).

Every rule runs in one of three phases:

| Phase | Runs | Reads | Rules |
|---|---|---|---|
| Chart | Once per element | The element's own `Chart.yaml`, `values.yaml` and `converters/` | POLICY021, 022, 030, 031, 040, 070, 090 |
| Cluster | Once per element and fixture | The `helm template` output for that cascade | POLICY001, 002, 032, 033, 034, 037, 050, 080, 081, 082, RENDER000 |
| Repo | Once | Every element, every file under `values/`, every fleet file, `install/`, the copier answers files | POLICY003, 011, 035, 038, 039, 051, 052, 060, 061, 071, 091, 092 |

Cluster-phase rules see the whole cascade, so a values layer that turns a template on or changes its
action is checked the same as the element's own `values.yaml`.

## Running it

You need the tools from `mise.toml` on your `PATH` ([Setup](../contributing.md#setup)), plus `make`
and `git`. The first run builds the validator, which downloads its Go modules (or reads them from
`GOPROXY`); after that it runs offline: the CRDs are committed in `tools/validator/crds/`
([CRD schemas](#crd-schemas)), and policy-library, the Helm library chart every element depends on,
comes from `lib/policy-library` in the same checkout
([Library changes](../contributing.md#library-changes)).

While writing an element ([Workflow](elements.md#workflow)):

```sh
make -C tools/validator preview ELEMENT=metallb CLUSTER=nonprod-west-1 # (1)!
make -C tools/validator validate ELEMENT=metallb # (2)!
```

1.  Renders one element for one fixture and prints it. `ELEMENT` is the chart name (`stack/<name>`).
    `CLUSTER` is the fixture's file name without directory or `.yaml`: `CLUSTER=acm-dc1` for
    `hubs/acm-dc1.yaml`.
2.  Every rule, for one element. Repo-phase rules still load everything but report only this
    element's findings, so findings that belong to no element (POLICY052, POLICY071) don't show.

Before opening a PR, run `validate`, which is what `ci` runs in your repository. In your
repository, `mise run validate` runs it with `BASE_BRANCH=main`, so a fleet file pinned to `main`
resolves to your working branch, and `mise run update` runs it after every update
([Taking an update](../guides/upgrading.md#taking-an-update)). If your default branch has another
name, set it: `BASE_BRANCH=prod mise run validate`. For a PR to upstream PolicyStack, run its
validator CI instead (every CI job: [Setup](../contributing.md#setup)):

```sh
make -C tools/validator ci # (1)!
```

1.  `lint`, `validate`, then `test`. Upstream only: `test` reads upstream's test data, which copier
    leaves out of your repository.

### Preview

`preview` writes a temporary values file holding only `stack.<key>.enabled: true` (`<key>` is
`ELEMENT` in camelCase: `user-monitoring` → `userMonitoring`) and appends it, with
[`baseline.yaml`](#baseline), on top of the fixture's cascade. Only the element switch is forced:
policies, templates and operators shipped `enabled: false` stay off unless the fixture's layers turn
them on. To preview one, set its `enabled: true` in the fixture's `values/clusters/<fixture>.yaml`
for the run. It runs only RENDER000 and POLICY081, so it prints output even when other rules would
fail (run `validate` for those). It prints two files:

- `<element>.yaml`: the Placement, PlacementBinding and Policies after Helm. The Placement selects
  `name Exists` (from the baseline), not the single cluster Argo CD would target
  ([Placement](../concepts/policies.md#placement)).
- `<element>.evaluated.yaml`: the objects each ConfigurationPolicy would apply, with ACM templates
  resolved on an empty cluster ([ACM templates](#acm-templates)). Each object is headed by its
  ConfigurationPolicy name and `complianceType`. A template that fails is listed first as
  `# POLICY081 <name>: <error>`. An object whose template looks up something the empty cluster lacks
  is written as rendered, marked `looks up an object the cluster lacks`. OperatorPolicies are not
  expanded; only their `-ns` and `-status` ConfigurationPolicies appear.

```yaml title="nonprod-west-1/metallb.evaluated.yaml (excerpt)"
---
# addressing-pools (mustonlyhave)
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: services
  namespace: metallb-system
spec:
  addresses:
  - 10.20.30.100-10.20.30.120
  autoAssign: true
---
# install-metallb-ns (musthave)
apiVersion: v1
kind: Namespace
metadata:
  name: metallb-system
```

Without `CLUSTER`, preview renders every fixture and prints only the paths it wrote. The files go to
a new temporary directory, which is not deleted, or to `PREVIEW=<dir>` (relative to
`tools/validator`). `ELEMENT=my-element` previews the [sample element](elements.md#sample-element).

copier leaves the fixtures out of your repository, so there `preview` renders your fleet files
instead, and `CLUSTER` names one of them (`CLUSTER=<hubName>` for `fleet/hubs/<hubName>.yaml`).

### What `validate` runs

`validate` builds the binary, then runs the validator in three passes. All of them run, and any
failure fails the target. Passes 1 and 2 render the fixtures, so they run only where
`tools/validator/testdata/clusters` exists: upstream has it, an adopter's copy doesn't.

1.  **Fixtures.** Every element plus `tools/sample-element/`, which `tools/create-element.sh` copies
    for every new element and so must stay valid. Skips POLICY052, because fixture revisions are
    not real pins, and POLICY081, which pass 2 covers. It runs POLICY071, the
    [install chart](../reference/install.md) check.
2.  **Profiles.** The all-on run, then one run per file in `tools/validator/testdata/profiles/`.
    Together they turn every element and optional piece on, so POLICY081 and POLICY001 see every
    converter ([Profiles](#profiles)).
3.  **Your fleet.** Only when `fleet/*.yaml` or `fleet/hubs/*.yaml` exists, which upstream never
    has. It renders the real fleet files and is the only pass that runs POLICY052 against real
    `revision` pins. Without fixtures, it also runs POLICY071, so an adopter's copy checks
    `install/` once it has a fleet file.

Upstream, a clean run prints `ok: no findings` three times (fixtures, all-on, `variants`). An
adopter's copy prints it once, for `fleet/`.

| Target | Runs |
| ------ | ---- |
| `validate [ELEMENT=<chart>] [BASE_BRANCH=<branch>]` | The three passes above. `BASE_BRANCH` passes `--base-branch` (see the flags below) |
| `preview ELEMENT=<chart> [CLUSTER=<name>] [PREVIEW=<dir>]` | [Preview](#preview) |
| `crds` | Refetches the committed [CRD schemas](#crd-schemas) in `tools/validator/crds/` from the pinned SHAs. Run it by hand after a pin changes, then commit the result |
| `test` | Upstream only. `go test -race ./...`. Tests that need Helm skip without it. A rule ID missing from the rules table in `tools/validator/README.md` (which [Validator rules](../reference/rules.md) includes) fails |
| `lint` | golangci-lint (`.golangci.yml`, gofmt and goimports included; `make fmt` fixes formatting), `go mod tidy -diff`, `go tool govulncheck` |
| `ci` | Upstream only. `lint`, `validate`, `test` |
| `deps` | `helm dependency update` for every chart that uses policy-library. Optional: `validate` runs it for any chart whose `charts/` archive is older than `lib/` |

`CI` is the environment variable GitHub Actions sets. Leave it unset locally. When it is set, findings
print as GitHub annotations (`--github`), missing CRDs fail the run instead of skipping POLICY080, and
tests that would skip fail instead. `test` and `lint` check the validator itself, so only upstream's
CI runs them, and `test` fails in a copy, which lacks upstream's test data ([Upstream](../reference/ci.md#upstream)).

!!! note
    Every run starts with `helm dependency update` on each chart, once per run, with
    `--skip-refresh` when every dependency is `file://`. It packages `lib/policy-library` into the chart's `charts/`, so the render always uses the library
    in your checkout, even when its version has not changed. When `lib/policy-library` does not
    satisfy the chart's `version` floor, the update fails, RENDER000 reports the element
    unrendered, and POLICY060 says what to change.

### Baseline

In production the ApplicationSet adds `selector`, the label selector that makes the element's
Placement pick its one cluster ([Placement](../concepts/policies.md#placement)), to every element's
values. Outside Argo CD
nothing does, and policy-library reads it unconditionally, so an enabled element fails:

```text
error RENDER000 [metallb/nonprod-west-1] hint: cascade did not provide `selector`. ...
... at <$root.Values.selector.matchExpressions>: nil pointer evaluating interface {}.matchExpressions
```

`validate` and `preview` pass `--extra-values tools/validator/testdata/baseline.yaml`, which supplies
a selector matching every cluster. A disabled element renders nothing, so it never hits this.

```yaml title="tools/validator/testdata/baseline.yaml"
selector:
  matchExpressions:
    - key: name
      operator: Exists
```

`--extra-values` files are appended after `values/clusters/<cluster>.yaml`, so they win over every
cascade file. They are never applied to `lifecycle/`, which ships its own `selector`. Argo CD
passes the whole fleet file to `lifecycle/` as values, so the validator appends the fixture there
instead. For elements, the validator passes the fixture's `version` and `channel` as the
ApplicationSet does.

## Reading the output

Each finding is `<severity> <rule> [<element>[/<cluster>]] <message>`, then `at <file>:<line>:<col>`:

```text
error POLICY022 [metallb] reads config key pools, which is not in stack.metallb.config of the element's values.yaml or the config of a template rendering it
    at /path/to/policystack/stack/metallb/converters/addressing/pools.yaml:7:19
error POLICY033 [metallb] policies.addressing.templates.l2: pruneObjectBehavior with a dependency (the Policy's dependencies): ACM deletes this template whenever the dependency is unmet, and prune deletes its objects
    at /path/to/policystack/values/clusters/nonprod-west-1.yaml:28:34
warning POLICY021 [metallb] converters/metallb-cr.yaml is never rendered: it is not converters/<policy>/<template>.yaml of a template and no templates.<t>.file names it
    at /path/to/policystack/stack/metallb/converters/metallb-cr.yaml:1

2 error(s), 1 warning(s)
```

Each points at the file to change: POLICY022 at the converter that reads the missing key, POLICY033
at the layer that set `pruneObjectBehavior`, POLICY021 at the stray file. Fixes for these and other
common findings: [Fixing common findings](../reference/rules.md#fixing-common-findings).

Only findings about one fixture name it: POLICY001, POLICY002, POLICY050, POLICY051, POLICY082 and
RENDER000. POLICY033, POLICY037, POLICY080 and POLICY081 leave it out on purpose, so one mistake is
reported once rather than once per fixture.

A finding on rendered output points at the last cascade file that sets the field on the entry (a
`policies.<p>`, `policies.<p>.templates.<t>` or `operatorPolicies.<k>` key), else the file that
defines the entry, else the template's converter. That is how the POLICY033 above lands on the
cluster's values file, not the element. If no entry renders the object, it points at the fleet
file; POLICY080 points at the element's `values.yaml` instead.

| Exit | Meaning |
| ---- | ------- |
| `0` | No errors. Warnings alone pass |
| `1` | Errors, or warnings with `--severity warning` |
| `2` | The validator failed, e.g. Helm not on `PATH` or an unknown `--only`/`--skip` rule ID |

??? info "Flags, for running the binary by hand"
    `make` fixes the flags it passes, so these only matter when you run
    `tools/validator/bin/policystack-validator --repo-root .` yourself. `--help` lists the rest.

    | Flag | Effect |
    | ---- | ------ |
    | `--element` | Check and render one chart, by chart name |
    | `--cluster` | Render one fleet file, by cluster name |
    | `--evaluate` | With `--render-dir`, also write `<chart>.evaluated.yaml` |
    | `--only` / `--skip` | Comma-separated rule IDs to run / skip |
    | `--severity warning` | Fail on warnings too |
    | `--github` | GitHub Actions annotations |
    | `--fixtures-dir` | Fleet files to render, hubs in its `hubs/`. Default `tools/validator/testdata/clusters`; pass 3 passes `fleet` |
    | `--extra-values` | Comma-separated values files appended to every cascade but `lifecycle/`'s ([Baseline](#baseline)) |
    | `--all-on` | Turns on every element and every policy, template and operator in it, ranked below `--extra-values` ([Profiles](#profiles)) |
    | `--lifecycle-dir` | Lifecycle chart, default `<repo>/lifecycle` |
    | `--base-branch` | The branch a PR merges into, default `$GITHUB_BASE_REF`. POLICY052 resolves a `revision` naming it to `HEAD`, the PR's merge commit |
    | `--render-dir` | Writes `<dir>/<fleet file path without .yaml>/<element>.yaml`, a failed render as `# RENDER000:` lines. [Render Diff](../reference/ci.md#render-diff) diffs two of these |

    Locally `$GITHUB_BASE_REF` is empty, so a fleet file with `revision: main` resolves to
    `origin/main` as of your last `git fetch`, not your working tree, and a branch you haven't pushed
    fails POLICY052. Pass `--base-branch main` (`BASE_BRANCH=main` with `make`) to make `main` mean
    `HEAD`; a repository with no `origin` remote needs it. POLICY052 also needs full history: tags
    and `origin/*` branches.

    `tools/validator/bin/policystack-validator resolve-revision [--branch <b> --at <commit>] -- <revision>`
    prints the commit POLICY052 resolves a `revision` to, exit 1 when it does not resolve.

## Fixtures

`tools/validator/testdata/clusters` holds example [fleet files](../reference/fleet.md) for fake
clusters that exist only for CI. Nothing is deployed. They list real files under `values/`, so a
change to a shared layer such as `values/environments/prod.yaml` changes what every fixture listing
it renders.

!!! note "Upstream only"
    copier leaves `tools/validator/testdata/clusters` and upstream's `values/` out of your
    repository, so the fixture and profile passes don't run there, and nothing in your `values/` is
    read by a fixture.

The file name is the cluster name: `hubs/acm-dc1.yaml` renders as `acm-dc1` and reads
`values/clusters/acm-dc1.yaml`. A fixture that sets `hub` names the hub that builds it with
`lifecycle/` ([Cluster lifecycle](../concepts/lifecycle.md)), so the validator also renders
`lifecycle/` for it.

Upstream contributors add a fixture for a new fleet shape, such as a new lifecycle engine. In your
repository, pass 3 checks your `fleet/` files; you don't add fixtures for them. `TestLoadDir_testdata`
(`internal/fixtures`) and `TestResolve_testdata` (`internal/cascade`) pin the exact list, so add the
new file to both.

??? info "Fixture list"
    | Fixture | Covers |
    | ------- | ------ |
    | `prod-east-1`, `nonprod-west-1` | Imported managed clusters, `version` and `channel` |
    | `hubs/acm-dc1` | The hub the lifecycle fixtures name |
    | `hcp-kubevirt`, `hcp-agent` | HyperShift |
    | `hive-aws` | Hive on AWS |
    | `hive-vsphere` | Hive on vSphere, one failure domain, DHCP |
    | `sno-edge` | SiteConfig single-node |
    | `bm-ha` | SiteConfig three masters and a worker |
    | `hcp-retired`, `hive-retired`, `sno-retired` | `state: absent` |
    | `name-budget-30-characters-long` | The 30-character name every element must fit ([Naming limits](../concepts/policies.md#naming-limits)) |

## ACM templates

A converter's escaped `{{ "{{" }} ... {{ "}}" }}` parts are ACM templates: Helm prints them as text,
and ACM resolves hub templates on the hub and the rest on the managed cluster
([Helm templates and ACM templates](../concepts/elements.md#helm-templates-and-acm-templates),
[ACM policies in brief](../concepts/acm.md#hub-and-managed-cluster-templates)). `helm template`
never runs them.

POLICY081 runs both stages with go-template-utils, the library ACM uses, against a fake cluster:

1. Hub templates, as the propagator does. Lookups stay in the Policy's namespace, plus the
   cluster's own ManagedCluster, unless the Policy sets `hubTemplateOptions.serviceAccountName`.
2. Managed templates, as the config-policy-controller does: `object-templates-raw` as a whole, each
   `object-templates` entry separately with `skipObject`.

The fake cluster holds no objects and serves only the kinds listed in `kinds` in
`tools/validator/internal/acmtemplates/acmtemplates.go`. The fake hub also holds the
operator-owned objects listed in `hubObjects` there: today one, Route `stackrox/central`, because
`stack/acs/converters/secured/agents.yaml` does `index` into it on the hub.

It fails on an undefined function (in any branch), a failed call such as `index` on nil, a hub
lookup outside the Policy's namespace, a kind the fake cluster does not serve (a misspelled kind or
apiVersion), and output with `<no value>` or an object missing `apiVersion`/`kind`:

```text
error POLICY081 [acs] central-console (managed template): resolves to <no value>: a field the template reads is missing
    at /path/to/policystack/stack/acs/converters/central/console.yaml:10
```

**Missing objects.** On an empty cluster every lookup misses. How that ends depends on the function:

- `fromSecret`, `fromConfigMap` and `copySecretData` of a missing object stop evaluation. That is not
  a failure, because on a real cluster another policy or operator creates the object. Code after it
  is parsed but not run, so only an undefined function there is caught; a bad `index` or missing
  field after a `fromSecret` shows up only on the cluster.
- `lookup` of a missing object returns an empty map and evaluation continues, so what you do with the
  result decides. These fail the same way on a real cluster before the object exists:

| Template | Result |
|---|---|
| `$host := (lookup ... "central").spec.host` then `https://$host` | POLICY081: `resolves to <no value>` |
| `$host := index (lookup ... "central") "spec" "host"` | POLICY081: `index of nil pointer` |
| `$host := dig "spec" "host" "" (lookup ... "central")` | Passes, `$host` is `""` |
| `.spec.host` as above, then `skipObject (not $host)` before printing it | Passes, the object is skipped until the Route exists |

ACS uses the last form, because a ConsoleLink with an empty `href` is worse than none:

```yaml title="stack/acs/converters/central/console.yaml"
href: '{{ "{{" }} $host := (lookup "route.openshift.io/v1" "Route" "stackrox" "central").spec.host {{ "}}" }}{{ "{{" }} skipObject (not $host) {{ "}}" }}https://{{ "{{" }} $host {{ "}}" }}'
```

!!! tip "Looking up a new kind, or indexing a new hub object"
    Add the kind to `kinds` in `acmtemplates.go`, or POLICY081 fails with `one or more API resources
    are not installed`. A hub template that indexes into an operator-owned hub object needs that
    object in `hubObjects`.

## Profiles

Fixtures leave most elements off, and every optional piece (a policy, template or operator shipped
`enabled: false`, [Optional pieces](elements.md#optional-pieces)) stays off, so most
converters never render under the fixtures. Pass 2 of `validate` turns everything on so POLICY081
and POLICY001 reach every converter:

- **All-on run.** `--all-on` turns on every element under `stack/` (and `tools/sample-element` with
  `--include-sample-element`) and every policy, template and operator in it, over any layer that
  turns one off. The validator builds this from the elements at run time, so a new element or
  `enabled: false` entry needs nothing added. `tools/validator/testdata/inputs.yaml` is layered on
  top: the inputs below, and the few choices all-on can't make, such as infra-nodes `restore`.
- **One run per file in `tools/validator/testdata/profiles/`**, each layered alone over the baseline,
  without `--all-on`. `variants.yaml` holds settings that can't be on together with the all-on run.
  Example: infra-nodes `workloads` and `restore` exclude each other, so `variants.yaml` turns
  `workloads` off and `restore` on.

Every rule but POLICY052 and POLICY071 runs on these, so an optional piece also needs its input. A
converter that loops over a `config` map renders nothing when the map is empty
([Converter shapes](elements.md#converter-shapes)): metallb's `addressing/pools.yaml` ranges over
`config.ipAddressPools`. With the map empty it renders `object-templates-raw: []`, and a Policy
whose templates are all empty is reported Compliant by ACM while checking nothing, which POLICY082
fails. `inputs.yaml` therefore sets `metallb.config.ipAddressPools` and the other maps:

```yaml title="tools/validator/testdata/inputs.yaml"
stack:
  metallb:
    config:
      ipAddressPools: {internal: {addresses: [192.168.100.1-192.168.100.20]}}
```

A Policy with only some empty templates passes.

`TestProfileCoverage`, run by `make test`, fails for a converter under `stack/` with ACM templates
that neither the all-on run nor a profile renders. Profiles never reach `lifecycle/` (extra values are not applied there), so a
change to a lifecycle converter's ACM template needs a test case in
`tools/validator/internal/acmtemplates` (`hypershift_test.go`, `hive_test.go`,
`siteconfig_test.go`, ...), which resolves it against fake hub objects so the branches for an
existing cluster run too.

## CRD schemas

The hub's API server validates a Policy, Placement and PlacementBinding at sync, but not the
ConfigurationPolicy, OperatorPolicy or CertificatePolicy inside `policy-templates`: the Policy CRD
declares `objectDefinition` with `x-kubernetes-preserve-unknown-fields`. A bad field there passes
the hub and only fails on the managed cluster. POLICY080 runs the CRD validation for those embedded
objects at PR time.

It checks rendered objects against the CRDs ACM 2.16 serves, the oldest release PolicyStack
supports. The checks run in-process (`k8s.io/apiextensions-apiserver`) the way the API server's do:
prune unknown fields, apply defaults, validate. `InformOnly` is read as `inform`, because the
ConfigurationPolicy CRD's enum lacks it and ACM's framework add-on translates it. The CRDs are
committed in `tools/validator/crds/` (7 files, about 130 KB), fetched by `make crds` at the SHAs
pinned in `tools/validator/Makefile`:

| Repo | Branch | CRDs |
|---|---|---|
| `stolostron/governance-policy-propagator` | `release-2.16` | Policy, PlacementBinding, PolicySet |
| `stolostron/config-policy-controller` | `release-2.16` | ConfigurationPolicy, OperatorPolicy |
| `stolostron/cert-policy-controller` | `release-2.16` | CertificatePolicy |
| `open-cluster-management-io/api` | `release-1.2` (v1.2.0, what the propagator requires) | Placement |

Committed, they need no network at validation time, which keeps disconnected sites working.
`--crds-dir` overrides the directory. Without CRDs, POLICY080 is skipped with a warning, or fails the
run when `CI` is set. The run logs how many objects were validated.

??? note "Bumping and adding"
    Upstream, Renovate moves each `*_SHA` to its branch head. Moving to a new ACM floor means
    changing the `branch=` comments and SHAs by hand. To add a CRD, add its raw URL to `CRD_URLS`, plus a
    `# renovate:` comment and `*_SHA` line for a new repo. After any of these, run `make crds` and
    commit `tools/validator/crds/`: upstream CI refetches the pinned files and fails while they
    differ, so a Renovate SHA bump stays red until someone does. Resolve the SHA with
    `git ls-remote https://github.com/<repo> refs/heads/<branch>`.

## Destructive templates

POLICY037 fails a template that is `mustonlyhave` or `mustnothave`
([ConfigurationPolicy](../concepts/acm.md#configurationpolicy)) on an object OpenShift owns (list in
the [POLICY037](../reference/rules.md#rules) row): enforced, it wipes what the installer and
operators wrote. Use `musthave`.

A template that must stay `mustonlyhave` or `mustnothave` can only be `InformOnly` (reported, never
enforced); an element has no way to enforce it. The validator also honours
`validator.policystk.io/allow-destructive: "<reason>"` on a ConfigurationPolicy, but policy-library
can't set it today.
