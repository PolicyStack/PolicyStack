# policystack-validator

CI-friendly validator for PolicyStack ACM-policy charts.

Renders every element under `stack/` for a set of fixture fleet files, walking
the same values cascade that `appset/templates/appset.yaml` applies at runtime,
and runs structural checks against the result. `lifecycle/` (override with
`--lifecycle-dir`) is checked like an element but rendered only for fleet files
that set `hub`, with the fleet file appended to the cascade the way
`appset/templates/lifecycle.yaml` passes it in `valuesObject`.

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
| POLICY020  | error    | `templateNames[].name` has no matching `converters/<name>.yaml` |
| POLICY021  | warning  | Converter file not referenced by any `templateNames[].name` |
| POLICY030  | error    | Invalid enum: severity / remediationAction / complianceType / upgradeApproval |
| POLICY031  | error/warning | values keys policy-library never reads: `enable:` (element renders nothing) and `defaultPolicy:` (metadata silently dropped) are errors; `default.severity`/`.remediationAction`/`.disabled` are warnings. Also flags `rawTemplate: true` with more than one `templateNames` entry |
| POLICY040  | error    | `policySets[].policies[]` references a name not in `policies[]` |
| POLICY050  | error    | Invalid fleet file: YAML error, unknown key, missing or empty `revision`, `revision`, `hub`, `state` or `valueFiles` entry Argo CD reads as a number or boolean, `valueFiles` entry that is null, listed twice, or not a file under `values/`; empty `hub`, `hub` with no `hubs/<hub>.yaml`, `hub` in a hub file, `state` other than `present`/`absent`, `install` that is not a map, `state` or `install` without `hub`, or a cluster that sets `hub` whose name is not a DNS label |
| POLICY060  | warning  | `policy-library` version drift across element `Chart.yaml` files |
| POLICY070  | error    | `helm lint` non-zero |
| POLICY080  | error    | `kubeconform` schema check on rendered manifests |
| POLICY090  | error    | `Chart.yaml` name → camelCase mismatch with single key under `stack:` |
| RENDER000  | error    | `helm template` failed |

<!-- --8<-- [end:rules] -->

## Fixtures

`testdata/clusters/` holds the example fleet files, laid out like `fleet/`:
`<cluster>.yaml` for spokes and `hubs/<hubName>.yaml` for hubs. The file name is
the cluster name, so a hub renders as its `hubName`. The `valueFiles` entries
drive the cascade in list order (matches `appset.yaml`).

| Fixture | Covers |
|---|---|
| `prod-east-1`, `nonprod-west-1` | Import-only spokes |
| `hubs/acm-dc1` | The hub the `hcp-*` fixtures name |
| `hcp-kubevirt` | KubeVirt hosted cluster, defaults from `values/platforms/kubevirt.yaml` |
| `hcp-agent` | Agent hosted cluster with two NodePools and `values/clusters/hcp-agent.yaml` |
| `hcp-retired` | `state: absent`, the destroy chain |

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

## Required tools

- `helm` (required)
- `kubeconform` (optional — POLICY080 is skipped if absent)
