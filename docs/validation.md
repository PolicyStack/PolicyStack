# Validation

`tools/validator` renders every element under `stack/` for each fixture cluster in
`tools/validator/testdata/clusters`, building the list of values files the way the ApplicationSet
does ([Order](values.md#order)). It checks the element sources and the rendered output. The
fixtures are the example [fleet files](applicationset.md#fleet-files): `prod-east-1.yaml`,
`nonprod-west-1.yaml`, the hub's `hubs/acm-dc1.yaml`, and seven clusters the hub builds:
`hcp-kubevirt.yaml`, `hcp-agent.yaml`, `hive-aws.yaml`, `sno-edge.yaml`, and `hcp-retired.yaml`,
`hive-retired.yaml` and `sno-retired.yaml` (`state: absent`). A file is named
after its cluster, so `hubs/acm-dc1.yaml` renders as `acm-dc1` and reads
`values/clusters/acm-dc1.yaml`. Their files in `values/` are rendered too.

For each fixture that sets `hub`, it also renders the `lifecycle/` chart as `lifecycle-<cluster>`,
with the fixture file appended to the cascade in place of the ApplicationSet's `valuesObject`
([Cluster lifecycle](lifecycle.md)).

Neither Helm nor the checks evaluate the ACM templates the lifecycle chart escapes, such as the
HostedCluster upgrade hold or the ClusterDeployment and ClusterImageSet release image read-backs. The Go tests in
`tools/validator/internal/acmtemplates` render the fixtures and resolve those templates with
go-template-utils, the library the config-policy-controller uses, against fake hub objects. A change
to a lifecycle converter's ACM template needs a case there.

It needs Go and Helm. Use the Helm major version that CI pins
([Policy Validate](workflows.md#policy-validate)). kubeconform is optional, without it on `PATH`,
POLICY080 is skipped.

## Running it

From the repo root, the same commands CI runs, without `--github`:

```sh
make -C tools/validator lint test build
tools/validator/bin/policystack-validator --repo-root . --extra-values tools/validator/testdata/baseline.yaml --skip POLICY080
```

The first checks formatting, vets, tests and builds the validator. The second runs it.
[Policy Validate](workflows.md#policy-validate) explains the POLICY080 skip.

`baseline.yaml` supplies a stand-in `selector`. One `Exists` expression on the `name` label. The
chart builds each Placement from `selector.matchExpressions`, and outside Argo CD nothing injects
it, so `helm template` fails for an enabled element without one. In production the ApplicationSet
injects a selector for the one cluster instead ([Placement](policies.md#placement)). Files passed
with `--extra-values` take precedence over the whole cascade. They are not applied to `lifecycle/`,
which ships its own `selector`.

The validator runs `helm dependency update` on an element unless its `charts/` directory already holds
`<name>-<version>.tgz` for every dependency in `Chart.yaml`, so a policy-library bump fetches the new
archive and replaces the stale one.

## Output

A clean run prints `ok: no findings`. Otherwise each finding gives its severity, rule, element (and
fixture cluster, for checks on rendered output), and the file to fix, with the line where known:

```text
error POLICY020 [metallb] configPolicies[0] "metallb-instance" references templateNames[0] "metallb-crd" but converters/metallb-crd.yaml does not exist
    at /path/to/policystack/stack/metallb/values.yaml:107:19
error POLICY030 [kiali] policies[0].severity = "hgh", allowed: [low medium high critical]
    at /path/to/policystack/stack/kiali/values.yaml:22:19
warning POLICY021 [metallb] converters/metallb-cr.yaml is not referenced by any templateNames[].name
    at /path/to/policystack/stack/metallb/converters/metallb-cr.yaml:1

2 error(s), 1 warning(s)
```

| Exit code | Meaning |
| --------- | ------- |
| `0` | No errors, warnings alone pass |
| `1` | Errors, or warnings with `--severity warning` |
| `2` | The validator itself failed, for example Helm is not on `PATH` |

| Flag | Effect |
| ---- | ------ |
| `--only` | Comma-separated rule IDs to run, all others are skipped |
| `--skip` | Comma-separated rule IDs to skip |
| `--severity warning` | Fail on warnings as well as errors |
| `--github` | Write GitHub Actions annotations instead of terminal output |
| `--fixtures-dir` | Fleet files to render instead of the examples, hubs in its `hubs/`. CI passes `fleet` when the repo has fleet files |
| `--lifecycle-dir` | The lifecycle chart, default `<repo>/lifecycle` |
| `--render-dir` | Writes each render to `<dir>/<fleet file path without .yaml>/<element>.yaml`, a failed one as `# RENDER000:` lines, nothing for an empty one. [Render Diff](workflows.md#render-diff) diffs two of these |

`--help` lists the rest.

## Rules

--8<-- "tools/validator/README.md:rules"
