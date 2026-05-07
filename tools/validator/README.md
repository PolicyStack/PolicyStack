# policystack-validator

CI-friendly validator for PolicyStack ACM-policy charts.

Renders every element under `stack/` for a set of fixture `ManagedCluster`
manifests, walking the same values cascade that `appset/templates/appset.yaml`
applies at runtime, and runs structural checks against the result.

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

| ID         | Severity | What it catches |
|------------|----------|-----------------|
| POLICY001  | error    | Policy/PolicySet ACM-replicated name `<ns>.<name>-<release>` > 63 chars; ConfigurationPolicy/OperatorPolicy/CertificatePolicy `metadata.name` (`<parent>-<sub>`) > 63 chars |
| POLICY002  | error    | Duplicate rendered policy `metadata.name` within a `policyNamespace` |
| POLICY010  | error    | `policyRef` points to nonexistent or disabled parent policy |
| POLICY020  | error    | `templateNames[].name` has no matching `converters/<name>.yaml` |
| POLICY021  | warning  | Converter file not referenced by any `templateNames[].name` |
| POLICY030  | error    | Invalid enum: severity / remediationAction / complianceType / upgradeApproval |
| POLICY040  | error    | `policySets[].policies[]` references a name not in `policies[]` |
| POLICY050  | error    | Duplicate `<category>.<priority>` labels on a fixture cluster |
| POLICY060  | warning  | `policy-library` version drift across element `Chart.yaml` files |
| POLICY070  | error    | `helm lint` non-zero |
| POLICY080  | error    | `kubeconform` schema check on rendered manifests |
| POLICY090  | error    | `Chart.yaml` name → camelCase mismatch with single key under `stack:` |
| RENDER000  | error    | `helm template` failed |

## Fixtures

`testdata/clusters/*.yaml` ships canonical `ManagedCluster` manifests. Override
with `--fixtures-dir`. Labels of the form
`config.<base-domain>/<category>.<priority>=<value>` drive the cascade
(matches `appset.yaml`).

### Baseline values

`policy-library` dereferences `.Values.selector.matchExpressions` without a
`hasKey` guard. In production that comes from `values/clusters/<cluster>.yaml`,
but a CI fixture cluster typically doesn't match a real per-cluster file in
your repo, so the cascade lacks `selector` and helm panics.

Pass `--extra-values testdata/baseline.yaml` (or your own file) to inject a
default `selector` into every cascade. The shipped baseline sets a permissive
`Exists` selector that will render but not match any real cluster.

```sh
./bin/policystack-validator --extra-values tools/validator/testdata/baseline.yaml --repo-root .
```

Alternatively, name your fixtures so `metadata.name` matches a real
`values/clusters/<name>.yaml` in your repo and the cluster's own values
will supply `selector`.

## Required tools

- `helm` (required)
- `kubeconform` (optional — POLICY080 is skipped if absent)
