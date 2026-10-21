# Validation

`tools/validator` renders every element in `stack/` for each fixture cluster, building the values
cascade like the ApplicationSet ([Order](values.md#order)), and checks sources and output. Needs Go
and the Helm version CI pins ([Policy Validate](workflows.md#policy-validate)). kubeconform is
optional; without it POLICY080 is skipped.

## Running it

The CI commands, from the repo root:

```sh
make -C tools/validator lint test build # (1)!
tools/validator/bin/policystack-validator --repo-root . \
  --extra-values tools/validator/testdata/baseline.yaml \
  --include-sample-element --skip POLICY080,POLICY081 # (2)!
for p in tools/validator/testdata/profiles/*.yaml; do # (3)!
  tools/validator/bin/policystack-validator --repo-root . --only RENDER000,POLICY081 \
    --extra-values "tools/validator/testdata/baseline.yaml,$p"
done
```

1.  Format check, vet, Go tests, build.
2.  Validates the fixtures. [Policy Validate](workflows.md#policy-validate) explains the skips.
3.  Evaluates every converter's ACM templates, see [ACM templates](#acm-templates).

`--extra-values` files override the whole cascade, except in `lifecycle/`, which ships its own
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
    reports the element unrendered.

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
    | `hcp-retired`, `hive-retired`, `sno-retired` | `state: absent` |
    | `name-budget-30-characters-long` | The 30-character name every element must fit ([Naming limits](policies.md#naming-limits)) |

## Output

```text
error POLICY020 [metallb] configPolicies[0] "metallb-instance" references templateNames[0] "metallb-crd" but converters/metallb-crd.yaml does not exist
    at /path/to/policystack/stack/metallb/values.yaml:107:19
warning POLICY021 [metallb] converters/metallb-cr.yaml is not referenced by any templateNames[].name
    at /path/to/policystack/stack/metallb/converters/metallb-cr.yaml:1

1 error(s), 1 warning(s)
```

A clean run prints `ok: no findings`. Findings on rendered output also name the fixture cluster.

| Exit | Meaning |
| ---- | ------- |
| `0` | No errors. Warnings alone pass |
| `1` | Errors, or warnings with `--severity warning` |
| `2` | The validator failed, e.g. Helm not on `PATH` |

| Flag | Effect |
| ---- | ------ |
| `--only` / `--skip` | Comma-separated rule IDs to run / skip |
| `--severity warning` | Fail on warnings too |
| `--github` | GitHub Actions annotations |
| `--fixtures-dir` | Fleet files to render, hubs in its `hubs/`. Default `tools/validator/testdata/clusters`. CI passes `fleet` when it exists |
| `--lifecycle-dir` | Lifecycle chart, default `<repo>/lifecycle` |
| `--render-dir` | Writes `<dir>/<fleet file path without .yaml>/<element>.yaml`, a failed render as `# RENDER000:` lines. [Render Diff](workflows.md#render-diff) diffs two of these |

`--help` lists the rest.

## ACM templates

Helm treats escaped ACM templates (`{{ "{{" }} ... {{ "}}" }}`) as strings. POLICY081 resolves them
against an empty cluster with go-template-utils, the library ACM uses:

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

```yaml title="stack/acs/converters/acs-console-link.yaml"
href: '{{ "{{" }} $host := (lookup "route.openshift.io/v1" "Route" "{{ .Parameters.namespace }}" "central").spec.host {{ "}}" }}{{ "{{" }} skipObject (not $host) {{ "}}" }}https://{{ "{{" }} $host {{ "}}" }}'
```

!!! tip "Looking up a new kind"
    Add it to `kinds` in `tools/validator/internal/acmtemplates/acmtemplates.go`.

Fixtures leave most elements off, so CI evaluates against `tools/validator/testdata/profiles`, which turn every element and toggle on. Add a
new element or toggle to `all-on.yaml`, or to `variants.yaml` if it conflicts with one there.
`TestProfileCoverage` fails until every converter with ACM templates renders under some profile.

```yaml title="tools/validator/testdata/profiles/all-on.yaml"
stack:
  acs:
    enabled: true
    toggles:
      console: true
```

Profiles enable toggles without their input maps, so POLICY082 fires there; CI runs only RENDER000
and POLICY081 on them. A change to a lifecycle converter's ACM template needs a test case in
`tools/validator/internal/acmtemplates`, which resolves them against fake hub objects.

## Rules

--8<-- "tools/validator/README.md:rules"
