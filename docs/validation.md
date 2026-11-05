# Validation

`tools/validator` renders every element in `stack/` for each fixture cluster, building the values
cascade like the ApplicationSet ([Order](values.md#order)), and checks sources and output. Needs the
Go and Helm in `mise.toml` (`mise install`).

## Running it

CI runs the same Make targets, from the repo root:

```sh
make -C tools/validator ci # (1)!
```

1.  `lint`, `test` and `validate`. With `CI` set, findings print as GitHub annotations.

While writing an element, run it alone ([Workflow](elements.md#workflow)):

```sh
make -C tools/validator preview ELEMENT=metallb CLUSTER=nonprod-west-1 # (1)!
make -C tools/validator validate ELEMENT=metallb # (2)!
```

1.  Renders the element turned on and prints `metallb.yaml`, the Policies, and
    `metallb.evaluated.yaml`, the objects ACM would apply with ACM templates resolved on an empty
    cluster. Without `CLUSTER`, every fixture is rendered into a temporary directory, or `PREVIEW=<dir>` (relative to `tools/validator`).
2.  `validate` for one chart. Repo-wide rules such as POLICY091 still read every element.

| Target | Runs |
| ------ | ---- |
| `validate [ELEMENT=<chart>]` | Builds, fetches the CRDs (`crds`), then three passes: the fixtures plus `tools/sample-element/` with POLICY081 skipped; every rule once per file in `testdata/profiles/`, sample element included ([ACM templates](#acm-templates)); `fleet/` when it has files. POLICY071 runs in the first pass only, POLICY052 in the last only |
| `preview ELEMENT=<chart> [CLUSTER=<fleet file>]` | Renders the chart turned on with `--render-dir` and `--evaluate`, running only RENDER000 and POLICY081 |
| `crds` | Downloads the [CRD schemas](#crd-schemas) into `tools/validator/.crds/` when a pin changed |
| `test` | `crds`, then `go test -race ./...`. Tests that need Helm, the CRDs or a fetched policy-library skip without them, and fail under `CI`. A rule ID missing from the [Rules](#rules) table fails |
| `lint` | golangci-lint (`.golangci.yml`, gofmt and goimports included; `make fmt` fixes formatting), `go mod tidy -diff`, `go tool govulncheck` |
| `deps` | `helm dependency update` for every chart that uses policy-library. Optional: `validate` fetches on demand |
| `consumers LIBRARY=<dir>` | `validate` against an unreleased policy-library checkout ([Chart changes](contributing.md#chart-changes)) |

`validate` passes `--extra-values tools/validator/testdata/baseline.yaml`. `--extra-values` files
override the whole cascade, except in `lifecycle/`, which ships its own
`selector`. `baseline.yaml` supplies the `selector` the ApplicationSet normally injects
([Placement](policies.md#placement)). Without it `helm template` fails on any enabled element:

```yaml title="tools/validator/testdata/baseline.yaml"
selector:
  matchExpressions:
    - key: name
      operator: Exists
```

!!! note
    `helm dependency update` runs unless `charts/` already holds `<name>-<version>.tgz` for every
    dependency. Bumping policy-library to an unpublished version fails the fetch, and RENDER000
    reports the element unrendered. Test an unpublished chart with `consumers` instead.

## Fixtures

`tools/validator/testdata/clusters` holds example [fleet files](applicationset.md#fleet-files). The
file name is the cluster name: `hubs/acm-dc1.yaml` renders as `acm-dc1` and reads
`values/clusters/acm-dc1.yaml`. A fixture that sets `hub` also renders `lifecycle/` as
`lifecycle-<cluster>`, with the fixture itself appended to the cascade ([Cluster lifecycle](lifecycle.md)).

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
    | `name-budget-30-characters-long` | The 30-character name every element must fit ([Naming limits](policies.md#naming-limits)) |

## Output

```text
error POLICY022 [metallb] reads config key pools, which is not in stack.metallb.config of the element's values.yaml or the config of a template rendering it
    at /path/to/policystack/stack/metallb/converters/addressing/pools.yaml:1:30
warning POLICY021 [metallb] converters/metallb-cr.yaml is never rendered: it is not converters/<policy>/<template>.yaml of a template and no templates.<t>.file names it
    at /path/to/policystack/stack/metallb/converters/metallb-cr.yaml:1

1 error(s), 1 warning(s)
```

A clean run prints `ok: no findings`. Findings on rendered output also name the fixture cluster.

| Exit | Meaning |
| ---- | ------- |
| `0` | No errors. Warnings alone pass |
| `1` | Errors, or warnings with `--severity warning` |
| `2` | The validator failed, e.g. Helm not on `PATH` or an unknown `--only`/`--skip` rule ID |

| Flag | Effect |
| ---- | ------ |
| `--element` | Check and render one chart, by chart name. Repo-wide rules still read every element and report only this one |
| `--cluster` | Render one fleet file, by cluster name |
| `--evaluate` | With `--render-dir`, also write `<chart>.evaluated.yaml`: the objects each ConfigurationPolicy applies, ACM templates resolved as POLICY081 does |
| `--only` / `--skip` | Comma-separated rule IDs to run / skip |
| `--severity warning` | Fail on warnings too |
| `--github` | GitHub Actions annotations |
| `--fixtures-dir` | Fleet files to render, hubs in its `hubs/`. Default `tools/validator/testdata/clusters`. CI passes `fleet` when it exists |
| `--lifecycle-dir` | Lifecycle chart, default `<repo>/lifecycle` |
| `--base-branch` | Branch `HEAD` lands on, default `$GITHUB_BASE_REF`. POLICY052 resolves a `revision` naming it to `HEAD`, the PR's merge commit |
| `--render-dir` | Writes `<dir>/<fleet file path without .yaml>/<element>.yaml`, a failed render as `# RENDER000:` lines. [Render Diff](workflows.md#render-diff) diffs two of these |

`--help` lists the rest. `resolve-revision [--branch <b> --at <commit>] -- <revision>` prints the
commit POLICY052 resolves a `revision` to, exit 1 when it does not resolve.

## ACM templates

Helm treats escaped ACM templates (`{{ "{{" }} ... {{ "}}" }}`) as strings. POLICY081 resolves them
with go-template-utils, the library ACM uses, against an empty cluster. The hub pass also holds the
operator-owned objects hub templates `index` into (`hubObjects` in `acmtemplates.go`), so their
absence is not reported:

1. `{{hub ... hub}}` as the policy propagator does. Lookups stay in the policy namespace unless the
   policy sets `hubTemplateOptions.serviceAccountName`.
2. `{{ ... }}` as the config-policy-controller does: `object-templates-raw` whole, each
   `object-templates` entry separately with `skipObject`.

It fails on an undefined function (in any branch), a failed call such as `index` on nil, a hub
lookup outside the policy namespace, a kind the fake cluster does not serve (misspelled kind or
apiVersion), and output with `<no value>` or an object missing `apiVersion`/`kind`.

A missing object is not a failure: on a real cluster another policy or operator creates it.
Evaluation stops there, so branches that need existing objects are parsed but not run. Guard such
lookups with `dig` and `skipObject`:

```yaml title="stack/acs/converters/central/console.yaml"
href: '{{ "{{" }} $host := (lookup "route.openshift.io/v1" "Route" "stackrox" "central").spec.host {{ "}}" }}{{ "{{" }} skipObject (not $host) {{ "}}" }}https://{{ "{{" }} $host {{ "}}" }}'
```

!!! tip "Looking up a new kind"
    Add it to `kinds` in `tools/validator/internal/acmtemplates/acmtemplates.go`.

Fixtures leave most elements off, so CI evaluates against `tools/validator/testdata/profiles`, which
turn every element and optional piece on. Add a new element, and each `enabled: false` policy,
template or operator, to `all-on.yaml`, or to `variants.yaml` if it conflicts with one there.
`TestProfileCoverage` fails until every converter with ACM templates renders under some profile.

```yaml title="tools/validator/testdata/profiles/all-on.yaml"
stack:
  acs:
    enabled: true
    policies:
      central:
        enabled: true
        templates:
          console:
            enabled: true
```

Every rule runs on the profiles, so an optional piece needs its input map there too, or
POLICY082 fails its empty Policy. A change to a lifecycle converter's ACM template needs a test
case in `tools/validator/internal/acmtemplates`, which resolves them against fake hub objects.

## CRD schemas

POLICY080 checks rendered objects against the CRDs ACM 2.16, the floor, serves. The checks run
in-process (`k8s.io/apiextensions-apiserver`) the same way the API server's do: prune unknown fields,
apply defaults, validate. `make crds` fetches the CRDs at the SHAs pinned in `tools/validator/Makefile`:

| Repo | Branch | CRDs |
|---|---|---|
| `stolostron/governance-policy-propagator` | `release-2.16` | Policy, PlacementBinding, PolicySet |
| `stolostron/config-policy-controller` | `release-2.16` | ConfigurationPolicy, OperatorPolicy |
| `stolostron/cert-policy-controller` | `release-2.16` | CertificatePolicy |
| `open-cluster-management-io/api` | `release-1.2` (v1.2.0, what the propagator requires) | Placement |

Nothing is committed. `--crds-dir` overrides `tools/validator/.crds`. Without CRDs, POLICY080 is
skipped with a warning, or fails the run when `CI` is set. CI caches the directory keyed on the
Makefile. The run logs how many objects were validated.

!!! note "Bumping and adding"
    Renovate moves each `*_SHA` to its branch head. Moving to a new ACM floor means changing the
    `branch=` comments and SHAs by hand. To add a CRD, add its raw URL to `CRD_URLS`, plus a
    `# renovate:` comment and `*_SHA` line for a new repo. Resolve the SHA with
    `git ls-remote https://github.com/<repo> refs/heads/<branch>`.

## Rendered output

POLICY001, POLICY033, POLICY037, POLICY080 and POLICY082 read what Helm rendered from the whole
cascade, so a values layer that enables a template or changes an action is checked like the
element's `values.yaml`. A finding points at the last cascade file that sets the field on the entry,
else the file defining the entry, else the template's converter. With no entry rendering the object,
it points at the fleet file (POLICY080: the element's `values.yaml`).

!!! warning "POLICY037: destructive templates on platform objects"
    `mustonlyhave` and `mustnothave` on the objects in the [Rules](#rules) table replace or delete
    what the installer and operators wrote. Use `musthave`, or make a gate `InformOnly`, which ACM
    never enforces whatever the Policy says. To keep one on purpose, annotate its ConfigurationPolicy
    with the reason:

    ```yaml
    metadata:
      annotations:
        validator.policystk.io/allow-destructive: "<reason>"
    ```

    policy-library sets no ConfigurationPolicy annotations from values yet. Until it does, the only
    opt-out is `--skip POLICY037`.

## Rules

!!! info "policy-library 4 rule changes"
    Removed: POLICY010 (`policyRef`), POLICY020 (missing `templateNames` converter) and POLICY036
    (template action mismatch). Discovery and the `InformOnly`-only template action make them
    impossible. Reworked: POLICY021 reports orphan files, POLICY022 reads `.Config`, POLICY030 checks
    the 4.0 keys, POLICY031 flags the 3.x shape, POLICY035 checks `policies`/`templates` layer keys,
    POLICY038 defaults `source` to `redhat-operators`, POLICY052 accepts `_hypershift/`.

--8<-- "tools/validator/README.md:rules"
