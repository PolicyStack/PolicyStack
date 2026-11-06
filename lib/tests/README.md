# Tests

`fixtures/policy-stack-test` is a consuming chart that renders `policy-library` through `file://`.
Its name camelCases to the `stack.policyStackTest` key. The [helm-unittest](https://github.com/helm-unittest/helm-unittest)
suites are in its `tests/`. After `mise install`, CI runs the same two commands:

```bash
helm dependency update lib/tests/fixtures/policy-stack-test # re-run after every library change
untt --strict lib/tests/fixtures/policy-stack-test         # add -u to rewrite snapshots
```

| Suite | Covers |
|---|---|
| `snapshot_test.yaml` | One golden render per `values*.yaml` |
| `contract_test.yaml` | The 4.0 element contract: discovery, `enabled`, defaults chain, `.Config`, shape detection, `orderManifests`, operator defaults |
| `fail_test.yaml` | Schema rejections, removed keys, and every `fail` call in the library |
| `library_test.yaml` | Template-name suffixes, `copyPolicyMetadata`, mustnothave operators, `hubTemplateOptions`, `policySets` |

`converters/base` and `converters/certs` are discovered in every case. `converters/_cases` is a partial
directory: tests reach its files only through `templates.<t>.file`.

> [!IMPORTANT]
> A snapshot diff is a behaviour change. If output changes for values that do not use a new key,
> the chart needs a major bump. Review `__snapshot__/` diffs like code and commit them.

`untt` 1.2.1 renders with the Helm 4 engine (v4.3.0), not the hub's Helm 3.18.4, so `fail_test.yaml`
matches errors with `errorPattern`, never exact text. `make validate` renders every element with the hub's Helm
from `mise.toml`.

CI also runs `ct lint --config lib/ct.yaml` (version bump, `Chart.yaml` schema, `helm lint --strict`).
