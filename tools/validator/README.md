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

`--element <chart>` checks and renders one chart, `--cluster <name>` renders one fleet file, and
`--evaluate` writes `<chart>.evaluated.yaml` beside each `--render-dir` render: the objects its
ConfigurationPolicies apply, ACM templates resolved on an empty cluster as POLICY081 does.
`make validate ELEMENT=<chart>` and `make preview ELEMENT=<chart> [CLUSTER=<name>] [PREVIEW=<dir>]`
wrap them; preview turns the element on, renders into `PREVIEW` (relative to `tools/validator`, default
a new temp dir) and prints both files.

`resolve-revision` prints the commit Argo CD renders for a fleet file's `revision`, resolved as
POLICY052 does. Exit 1 when it does not resolve. `render-diff.sh` uses it:

```sh
./bin/policystack-validator resolve-revision --branch main --at <merge commit> -- v1.4.0
```

## Rules

<!-- --8<-- [start:rules] -->

| ID         | Severity | What it catches |
|------------|----------|-----------------|
| POLICY001  | error    | A rendered Policy or PolicySet whose ACM-replicated name `<ns>.<name>-<release>` is over 63 characters, or a policy-template (ConfigurationPolicy, OperatorPolicy with its `-ns`/`-status` templates, CertificatePolicy) whose `metadata.name` is. Reads the render, so a values layer that adds an entry is caught, and points at the last cascade file setting the entry, else at its converter |
| POLICY002  | error    | Duplicate policy-template `metadata.name` WITHIN one element, such as template `b-c` of policy `a` and template `c` of policy `a-b`. A template only `converters/` declares is reported at its converter |
| POLICY003  | error    | Two different elements render a policy-template with the same name. ACM requires template names to be unique across every Policy on a cluster, so template `ns-monitoring` of policy `install` in two elements is rejected on-cluster with "Template name must be unique". Covers disabled entries and the `-ns`/`-status` templates an operatorPolicy generates |
| POLICY011  | error    | dependency reference that can never be satisfied: unknown `dependencies`/`extraDependencies` target, unknown `element:`, `element:` on a template kind, `waitForOperator` naming no `operatorPolicies` entry, or a target policy policy-library never emits (no template or operator). Disabled entries count, since a layer may turn them on |
| POLICY021  | warning  | A converter policy-library never renders: a `.yml` file, a `.yaml` file no template renders (a top-level file no `templates.<t>.file` names, or `converters/<p>/<t>.yaml` replaced by the template's own `file`), or a partial (a path segment starting `_`) that no `file` names and no other chart file mentions by name |
| POLICY022  | error    | A converter reads a top-level config key missing from the element's `values.yaml`, which renders as null: `.Config.<key>` or `.Values.stack.<el>.config.<key>`, from `$` too, or `$v.<key>` with `$v` bound to either. `.Config` also takes the keys of `templates.<t>.config` of each template rendering the converter, or of any template in a `_` partial no template renders (pulled in with `tpl`). Only Helm actions count, not escaped ACM templates. Nested keys are not checked |
| POLICY030  | error/warning | Invalid enum: severity / remediationAction / complianceType / upgradeApproval (`None` or `Automatic`), in `default`, `policies.<p>` and `policies.<p>.templates.<t>`, whose `remediationAction` is only `InformOnly`. In `operatorPolicies` entries also: `subscription.installPlanApproval` set, a `versions` entry or `subscription.startingCSV` that is not a valid CSV name (templated entries skipped, comma-separated `versions` entries split), and a `name:` key, which is ignored (warning) |
| POLICY031  | error/warning | values keys policy-library never reads: `enable:` (element renders nothing) and `defaultPolicy:` (metadata silently dropped) are errors, `default.disabled` a warning. Also flags the policy-library 3.x shape (a `policies` list, `configPolicies` or `toggles`: run `tools/migrate-4.0.py`) and `operatorPolicies` as a list. A 3.x chart is still rendered (RENDER000) but no other rule runs on it |
| POLICY032  | error/warning | `stack.<el>.operatorPolicies` in a values layer: a key the element's `values.yaml` does not define, a list, or any POLICY030 operator problem. Reported once per layer file |
| POLICY033  | error/warning | A rendered ConfigurationPolicy's `pruneObjectBehavior` other than `None`: an invalid value, an effective `remediationAction` other than `enforce` (the Policy's overrides the template's unless it is `InformOnly`; skipped on a dry run, `global.remediationAction`), or any dependency (the template's `extraDependencies`, from `waitForOperator`, `extraDependencies` or `orderManifests`, or the Policy's `dependencies`), since ACM deletes the template whenever the dependency is unmet. `DeleteAll` is a warning |
| POLICY034  | error    | Top-level `version` or `channel` in a values layer: only the fleet file sets them. Reported once per layer file |
| POLICY035  | error    | A key in the root `values.yaml` or a file under `values/` that nothing reads: an unknown top-level or `global` key, a `stack` key that names no element, an element key policy-library's schema rejects (reported here at the layer; RENDER000 reports it once per element at its `values.yaml`), a `policies` key that names no policy of the element (a `policies` key, a `converters/` directory or `install`), a `templates` key that names no template of its policy and sets no `file`, or a top-level `config` key missing from the element's `values.yaml`. Checks every file, used by a fleet file or not |
| POLICY037  | error/warning | A rendered ConfigurationPolicy object-template that is `mustonlyhave` or `mustnothave` on an object OpenShift owns: ConfigMap `openshift-config/user-ca-bundle`, Secret `openshift-config/pull-secret`, ClusterVersion `version`, Proxy and Image `cluster` (`config.openshift.io`), or a Namespace starting `openshift-` or `kube-`. Enforced, it replaces or deletes platform state. `musthave` is not covered. Skips an `InformOnly` template and one annotated `validator.policystk.io/allow-destructive: <reason>`. `object-templates-raw` is read with its ACM template actions removed, so every branch counts; raw that is then not YAML is a warning |
| POLICY038  | error    | An `operatorPolicies` `subscription.source` (`redhat-operators` when unset), in an element's `values.yaml` or a values layer, that is not `redhat-operators`, `certified-operators`, `community-operators`, `redhat-marketplace` or a `global.catalogSources` key in any layer. OLM finds no such CatalogSource, so the operator never installs. Templated values are skipped |
| POLICY040  | error    | `policySets[].policies[]` names no policy of the element |
| POLICY050  | error    | Invalid fleet file: YAML error, unknown key, missing or empty `revision`, `revision`, `hub`, `state`, `version`, `channel` or `valueFiles` entry Argo CD reads as a number or boolean, `valueFiles` entry that is null, listed twice, or not a file under `values/`; `version` that is not `x.y.z`, `install.version`; empty `hub`, `version` or `channel`, `hub` with no `hubs/<hub>.yaml`, `hub` naming the cluster itself, `hub` in a hub file, `state` other than `present`/`absent`, `install` that is not a map, `state` or `install` without `hub`, or a cluster that sets `hub` whose name is not a DNS label |
| POLICY051  | error    | Two siteconfig nodes on one hub with the same `bootMACAddress` or `bmcAddress` (scheme aside), across fleet files that are not `state: absent`: two BareMetalHosts would drive one server |
| POLICY052  | error/warning | A fleet file's `revision`, resolved in git as Argo CD does (tag, `origin` branch, then a 5 to 40 character commit SHA; a PR's base branch is the merge commit), that does not resolve or whose commit lacks `stack/`, `lifecycle/converters/_hypershift/` (or the pre-4.0 `hypershift/`) when `hub` is set, an upgrade element when `version` or `channel` is set, or a `valueFiles` entry, which Argo CD skips. A `values/clusters/<name>.yaml` no fleet file reads is a warning. Needs git history (`fetch-depth: 0`). `make validate` runs it on `fleet/` only |
| POLICY060  | error    | `policy-library` version drift across element `Chart.yaml` files |
| POLICY070  | error    | `helm lint` non-zero |
| POLICY071  | error    | `install/`, which each hub runs by hand: `helm lint` fails, an empty `hubName` or `gitRepo` does not fail the install, `policyNamespace` differs from the root `values.yaml`, an ApplicationSet (or its Application template), GitOpsCluster or Placement lacks `velero.io/exclude-from-backup: "true"`, or `install/operators` does not render the ACM and GitOps operators and a MultiClusterHub from the element values, with policy-library's defaults for keys they omit |
| POLICY080  | error/warning | A rendered object that fails its CRD, checked as the API server would: an unknown field, a value outside an enum such as `remediationAction: enforec`, or a wrong type. Covers Policy, Placement, PlacementBinding and PolicySet, then each `policy-templates[].objectDefinition` (ConfigurationPolicy, OperatorPolicy, CertificatePolicy) with `InformOnly` read as `inform`, as the framework addon does. Findings name the values entry, without a cluster. A kind with no CRD is one warning per element |
| POLICY081  | error    | An escaped ACM template ACM would fail to resolve, resolved hub then managed on an empty cluster: an undefined function in any branch, a failed call such as `index` on nil, a hub lookup outside the policy namespace without `hubTemplateOptions.serviceAccountName`, a kind discovery does not serve, or output with `<no value>` or an object without `apiVersion`/`kind`. A missing object is not a failure. Findings name the sub-policy without its cluster suffix and carry no cluster, so a broken template reports once |
| POLICY082  | error    | A rendered Policy whose every template is a ConfigurationPolicy with no objects (`object-templates-raw` empty or `[]`, or no `object-templates`). ACM reports it Compliant without checking anything; policy-library renders `[]` when a raw converter's input map is empty. A Policy with only some empty templates passes |
| POLICY090  | error    | `Chart.yaml` name → camelCase mismatch with single key under `stack:`. For a chart whose `values.schema.json` has an `install.engine` enum (`lifecycle/`), a `stack:` key that is not an engine or an engine with no `stack:` key. Every other check then runs on each engine's component, and each fixture renders the one its `install.engine` selects |
| POLICY091  | error    | An element name equal to another's, or starting with another's followed by `-`. Release names join element and cluster with `-`, so `a` on cluster `b-c` and `a-b` on cluster `c` would share one Application and its Policies. `lifecycle/` takes part, which reserves `lifecycle` |
| RENDER000  | error    | `helm template` failed, or `helm dependency update` did. A chart whose dependencies were not fetched is not rendered, since Helm would use any `charts/` archive of the dependency, whatever its version. Reported once per chart |

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
| `bm-ha` | SiteConfig three masters and a worker, VIPs, static network |
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

`testdata/profiles/` turns on every element and optional template, `tools/sample-element` included, so
POLICY081 reaches every converter. `make validate` runs every rule on them, so an enabled template
needs its input map or POLICY082 fails. `variants.yaml` holds what `all-on.yaml` cannot set at the
same time. `TestProfileCoverage` fails for
a converter with ACM templates that no profile renders.

## Required tools

- `helm` (required)
- `curl`, for `make crds` (POLICY080 is skipped without the CRDs, and fails the run under `CI`)
