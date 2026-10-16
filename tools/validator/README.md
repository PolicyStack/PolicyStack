# policystack-validator

CI-friendly validator for PolicyStack ACM-policy charts.

Renders every element under `stack/` for a set of fixture fleet files, walking
the same values cascade that `install/templates/appset.yaml` applies at runtime,
and runs structural checks against the result. `lifecycle/` (override with
`--lifecycle-dir`) is checked like an element but rendered only for fleet files
that set `hub`, with the fleet file appended to the cascade the way
`install/templates/lifecycle.yaml` passes it in `valuesObject`.

## Usage

```sh
go build -o bin/policystack-validator ./cmd/policystack-validator
./bin/policystack-validator --repo-root ../..
```

In CI:

```sh
./bin/policystack-validator --repo-root . --github
```

## Rules

<!-- --8<-- [start:rules] -->

| ID         | Severity | What it catches |
|------------|----------|-----------------|
| POLICY001  | error    | Policy/PolicySet ACM-replicated name `<ns>.<name>-<release>` > 63 chars; ConfigurationPolicy/OperatorPolicy/CertificatePolicy `metadata.name` (`<parent>-<sub>`) > 63 chars |
| POLICY002  | error    | Duplicate rendered `metadata.name` WITHIN one element |
| POLICY003  | error    | Two different elements render a policy-template with the same name. ACM requires template names to be unique across every Policy on a cluster, so `policies[].name: install` + `configPolicies[].name: ns-monitoring` in two elements is rejected on-cluster with "Template name must be unique". Covers toggled-off entries and the `-ns`/`-status` templates an operatorPolicy generates |
| POLICY010  | error    | `policyRef` points to nonexistent or disabled parent policy |
| POLICY011  | error    | dependency reference that can never be satisfied: unknown `dependencies`/`extraDependencies` target, unknown `element:`, `element:` on a template kind, `waitForOperator` naming no enabled operator, or a target policy policy-library never emits (no enabled sub-policy attached) |
| POLICY020  | error    | `templateNames[].name` has no matching `converters/<name>.yaml`; `<name>` may contain `/`, as in `hypershift/ready` |
| POLICY021  | warning  | Converter file not referenced by any `templateNames[].name` |
| POLICY030  | error/warning | Invalid enum: severity / remediationAction / complianceType / upgradeApproval (`None` or `Automatic`). In `operatorPolicies` entries also: `subscription.installPlanApproval` set, a `versions` entry or `subscription.startingCSV` that is not a valid CSV name (templated entries skipped, comma-separated `versions` entries split), and a `name:` key, which is ignored (warning) |
| POLICY031  | error/warning | values keys policy-library never reads: `enable:` (element renders nothing) and `defaultPolicy:` (metadata silently dropped) are errors; `default.severity`/`.remediationAction`/`.disabled` are warnings. Also flags `rawTemplate: true` with more than one `templateNames` entry, and `operatorPolicies` as a list (policy-library 3.x needs a map) |
| POLICY032  | error/warning | `stack.<el>.operatorPolicies` in a values layer: a key the element's `values.yaml` does not define, a list, or any POLICY030 operator problem. Reported once per layer file |
| POLICY033  | error/warning | `configPolicies[].pruneObjectBehavior` other than `None`: an invalid value, an effective `remediationAction` other than `enforce` (a policy-level value overrides the template's), or any dependency (`waitForOperator`, `extraDependencies`, parent `dependencies`, `orderManifests` unless first template, `orderPolicies` unless first Policy), since ACM deletes the template whenever the dependency is unmet. `DeleteAll` is a warning |
| POLICY034  | error    | Top-level `version` or `channel` in a values layer: only the fleet file sets them. Reported once per layer file |
| POLICY040  | error    | `policySets[].policies[]` references a name not in `policies[]` |
| POLICY050  | error    | Invalid fleet file: YAML error, unknown key, missing or empty `revision`, `revision`, `hub`, `state`, `version`, `channel` or `valueFiles` entry Argo CD reads as a number or boolean, `valueFiles` entry that is null, listed twice, or not a file under `values/`; `version` that is not `x.y.z`, `install.version`; empty `hub`, `version` or `channel`, `hub` with no `hubs/<hub>.yaml`, `hub` in a hub file, `state` other than `present`/`absent`, `install` that is not a map, `state` or `install` without `hub`, or a cluster that sets `hub` whose name is not a DNS label |
| POLICY060  | warning  | `policy-library` version drift across element `Chart.yaml` files |
| POLICY070  | error    | `helm lint` non-zero |
| POLICY080  | error    | `kubeconform` schema check on rendered manifests |
| POLICY081  | error    | An escaped ACM template ACM would fail to resolve, resolved hub then managed on an empty cluster: an undefined function in any branch, a failed call such as `index` on nil, a hub lookup outside the policy namespace without `hubTemplateOptions.serviceAccountName`, a kind discovery does not serve, or output with `<no value>` or an object without `apiVersion`/`kind`. A missing object is not a failure. Findings name the sub-policy without its cluster suffix and carry no cluster, so a broken template reports once |
| POLICY090  | error    | `Chart.yaml` name → camelCase mismatch with single key under `stack:`. For a chart whose `values.schema.json` has an `install.engine` enum (`lifecycle/`), a `stack:` key that is not an engine or an engine with no `stack:` key. Every other check then runs on each engine's component, and each fixture renders the one its `install.engine` selects |
| POLICY091  | error    | An element name equal to another's, or starting with another's followed by `-`. Release names join element and cluster with `-`, so `a` on cluster `b-c` and `a-b` on cluster `c` would share one Application and its Policies. `lifecycle/` takes part, which reserves `lifecycle` |
| RENDER000  | error    | `helm template` failed |

<!-- --8<-- [end:rules] -->

## Fixtures

`testdata/clusters/` holds the example fleet files, laid out like `fleet/`:
`<cluster>.yaml` for spokes and `hubs/<hubName>.yaml` for hubs. The file name is
the cluster name, so a hub renders as its `hubName`. The `valueFiles` entries
drive the cascade in list order (matches `appset.yaml`).

| Fixture | Covers |
|---|---|
| `prod-east-1`, `nonprod-west-1` | Import-only spokes: a `channel` pin, and a `version` and `channel` upgrade |
| `hubs/acm-dc1` | The hub the `hcp-*` fixtures name |
| `hcp-kubevirt` | KubeVirt hosted cluster, defaults from `values/platforms/kubevirt.yaml` |
| `hcp-agent` | Agent hosted cluster with two NodePools, `channel` and `values/clusters/hcp-agent.yaml` |
| `hcp-retired` | `state: absent`, the destroy chain |
| `hive-aws` | Hive cluster on AWS, defaults from `values/platforms/aws.yaml` |
| `hive-retired` | Hive `state: absent` |
| `sno-edge` | SiteConfig single-node cluster, defaults from `values/platforms/baremetal.yaml` |
| `sno-retired` | SiteConfig `state: absent` |
| `name-budget-30-characters-long` | The 30-character cluster name every element must fit (POLICY001) |

Override with `--fixtures-dir`, for example `--fixtures-dir fleet` to check a
real fleet. Other files are ignored, `.yml` included.

### Baseline values

`policy-library` dereferences `.Values.selector.matchExpressions` without a
`hasKey` guard. In production the ApplicationSet injects `selector` in
`valuesObject`. Outside Argo CD nothing injects it, so the cascade lacks
`selector` and helm panics.

Pass `--extra-values testdata/baseline.yaml` (or your own file) to inject a
default `selector` into every cascade. The shipped baseline sets a permissive
`Exists` selector that will render but not match any real cluster. It is not
applied to `lifecycle/`, which ships its own `selector`.

```sh
./bin/policystack-validator --extra-values tools/validator/testdata/baseline.yaml --repo-root .
```

`--render-dir <dir>` also writes each render to `<dir>/<fleet file path without .yaml>/<chart>.yaml`,
a failed one as `# RENDER000:` comments. CI diffs two of these trees on PRs (`render-diff.yml`).

### Profiles

`testdata/profiles/` turns on every element and toggle, so POLICY081 reaches every converter.
`variants.yaml` holds what `all-on.yaml` cannot set at the same time. `TestProfileCoverage` fails for
a converter with ACM templates that no profile renders.

## Required tools

- `helm` (required)
- `kubeconform` (optional — POLICY080 is skipped if absent)
