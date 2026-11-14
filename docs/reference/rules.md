# Validator rules

Every rule the validator (`tools/validator`) runs, by ID. You land here from a finding such as
`error POLICY033 [metallb] ...`, in your terminal or as an annotation on a PR line. What the
validator does, how to run it and how to read a finding: [Validation](../develop/validation.md).

`POLICYnnn` rules check element sources, values layers, fleet files and the render.
`RENDER000` means `helm template` itself failed, so no rule that reads the render ran for that
element and cluster: fix it first.

An `error` fails the run (exit 1) and with it the `ci` check, which blocks the merge
once your repository requires it ([Required checks](ci.md#required-checks)). A `warning` is reported
but passes. Rows marked `error/warning` say which case is which.

A finding points at the file and line to change. A rule that reads the render points at
the values layer that set the field, so a mistake in `values/` is reported there, not in the
element ([Reading the output](../develop/validation.md#reading-the-output)).

CI runs `make -C tools/validator validate`, whose flags are fixed in the
Makefile, so a PR can't skip a rule. `--skip`/`--only` work only when you run the binary by hand. The
one opt-out in the repo is POLICY037's: make the template `InformOnly`
([Destructive templates](../develop/validation.md#destructive-templates)).

??? info "Terms the table uses"
    | Term | Meaning |
    |---|---|
    | Element | A chart under `stack/` ([How an element is built](../concepts/elements.md)) |
    | Entry | A key under `policies`, `policies.<p>.templates` or `operatorPolicies` in an element's values |
    | Policy-template, sub-policy | A ConfigurationPolicy, OperatorPolicy or CertificatePolicy inside a Policy's `spec.policy-templates` ([ACM policies in brief](../concepts/acm.md)) |
    | Converter | A file under an element's `converters/`, rendered into a template |
    | Partial | A converter whose file or directory name starts with `_`: never a template on its own, only rendered when a template's `file` names it or another chart file reads it |
    | Values layer | A values file under `values/` that a cluster reads: listed in its fleet file's `valueFiles`, or its own `values/clusters/<cluster>.yaml` ([Values cascade](../concepts/values.md)) |
    | Cascade | The element's `values.yaml`, then the root `values.yaml`, then the cluster's layers in order; later files win ([Order](../concepts/values.md#order)) |
    | Fleet file | `fleet/<cluster>.yaml` or `fleet/hubs/<hub>.yaml` ([Fleet files](fleet.md)) |
    | Replicated name | `<namespace>.<Policy name>`, the name ACM gives the copy of a Policy in the cluster's namespace on the hub |
    | `InformOnly`, gate | A template `remediationAction` ACM never enforces. A gate is an `InformOnly` template other steps wait on |
    | Release | `<element>-<cluster>` (`<element>-<hubName>` on the hub): the Application and Helm release name. Policy and Placement names end with it |
    | PolicySet | An ACM object grouping Policies for reporting |
    | `object-templates-raw` | A ConfigurationPolicy's objects as one templated string, used when a converter's output starts with an ACM template or a list ([Converter shapes](../develop/elements.md#converter-shapes)) |
    | Fixture | A made-up fleet file in `tools/validator/testdata/clusters` the validator renders ([Fixtures](../develop/validation.md#fixtures)) |
    | Dry run | `global.remediationAction: inform` in a layer: every Policy reports instead of changing ([Dry run](../guides/rollout.md#dry-run)) |
    | Discovery | The API kinds the validator's fake cluster serves ([ACM templates](../develop/validation.md#acm-templates)) |
    | Framework add-on | The part of ACM's governance add-on on the managed cluster that reads each Policy's templates ([ACM policies in brief](../concepts/acm.md)) |
    | Engine, component | A lifecycle builder (`hypershift`, `hive`, `siteconfig`), one `stack.<engine>` block in `lifecycle/values.yaml` ([Cluster lifecycle](../concepts/lifecycle.md)) |

## Rules

--8<-- "tools/validator/README.md:rules"

## Fixing common findings

| Rule | Usual cause | Fix |
|---|---|---|
| RENDER000 | `helm template` failed | Read the Helm error under the finding. A `hint:` line names the usual cause: a missing `selector` (when running the binary by hand, pass the [baseline](../develop/validation.md#baseline)) or a values key the chart reads that no file sets. `dependencies not fetched` means `helm dependency update` could not package `lib/policy-library`: POLICY060 names the `Chart.yaml` fix |
| POLICY001 | A policy or template name too long for a 30-character cluster name | Shorten it ([Naming limits](../concepts/policies.md#naming-limits)) |
| POLICY002 | Two names collide on one cluster. Also reported across elements: element `b` with policy `x-a` and element `a-b` with policy `x` both render Policy `x-a-b-<cluster>`, which POLICY091 doesn't catch | Rename one policy or template |
| POLICY003 | Two elements ship a template of the same name | Rename one. ACM requires template names unique across every Policy on a cluster |
| POLICY011 | A typo in a dependency name, or a policy a fleet file turns on that depends on an element this repository doesn't have, such as `acm`'s `observability` on `odf` | Fix the name. For a missing element, add it ([Add elements](../start/install.md#add-elements)) or leave the policy off on every cluster |
| POLICY021 | A file directly under `converters/`, or an unused partial | Move it to `converters/<policy>/<template>.yaml`, name it in a template's `file`, or delete it. A `_` partial must be named by a `file` or read by another chart file |
| POLICY022 | A converter reads `.Config.<key>` the element's `values.yaml` doesn't define | Add the key under `config:` with a default and a comment |
| POLICY030 | A misspelled enum, or `subscription.installPlanApproval` | Fix the spelling. ACM's OperatorPolicy rejects `installPlanApproval`: use `upgradeApproval` and `versions` ([Operator versions](../guides/operators.md)) |
| POLICY033 | `pruneObjectBehavior` on a template that isn't enforced or has a dependency | Remove `pruneObjectBehavior` or the dependency ([Removal and pruning](../concepts/removal.md)) |
| POLICY034 | `version` or `channel` in a values layer | Move it to the cluster's fleet file |
| POLICY035 | A typo in `values/`: a key nothing reads | Fix the key the message names |
| POLICY037 | `mustonlyhave` or `mustnothave` on an object OpenShift owns | `musthave`, or `InformOnly` ([Destructive templates](../develop/validation.md#destructive-templates)) |
| POLICY038 | A `subscription.source` that is not a default catalog | Fix the name, or declare it under `global.catalogSources` ([Disconnected clusters](../guides/disconnected.md)) |
| POLICY039 | A layer or fleet file still sets a key an element deprecated | Set the replacement the message names instead, before the version it names. A finding at an element's `values.yaml` is a malformed note: write it as `Deprecated: use <key>; removed in <version>` ([Deprecating a key](../develop/elements.md#deprecating-a-key-or-an-element)) |
| POLICY050 | A broken fleet file | Fix the key the message names. `install.version` is an error whatever its value: use the top-level `version` |
| POLICY052 | A `revision` that doesn't resolve, a commit missing what the fleet file uses, or one on a framework major the hubs don't render | Push the branch or tag first. The "upgrade element" is `stack/upgrade`: a fleet file setting `version` or `channel` needs a `revision` that has it. Locally, `main` means `origin/main` ([flags](../develop/validation.md#reading-the-output)). A newer major: merge the framework update and upgrade the hubs first. Two or more majors older: move the `revision` forward ([Majors](../guides/upgrading.md#majors)) |
| POLICY060 | A `Chart.yaml` copied from outside `stack/`, written by hand, or with a floor above `lib/policy-library` | Set what the message names. A floor above the library means the library change this chart needs is missing, or `lib/policy-library/Chart.yaml` `version` was not bumped with it ([Library changes](../contributing.md#library-changes)) |
| POLICY061 | A hand-run `copier copy` or `copier update` from a branch or commit instead of a release tag, into the wrong directory, or past the framework; or an element left on an older major than the framework | Delete the element and `mise run ps -- add` it again, or `mise run update`, which takes elements at the framework's tag and moves every element on a major. Never edit an answers file by hand ([Answers files](../guides/upgrading.md#answers-files)) |
| POLICY070 | `helm lint` failed on the element with only its own `values.yaml`, where it is off | A chart structure error; render problems are RENDER000 |
| POLICY080 | A misspelled field or enum value in a rendered object | Fix the field the message names |
| POLICY081 | An ACM template that fails on an empty cluster | [ACM templates](../develop/validation.md#acm-templates) |
| POLICY082 | An optional piece turned on with an empty input map, so its Policy checks nothing | Give it its `config` input ([Profiles](../develop/validation.md#profiles)) |
| POLICY090 | `values.yaml` has no single `stack.<key>` matching the chart name in camelCase (`user-monitoring` → `userMonitoring`), so policy-library reads a different key and renders nothing | Rename the key. The engine part applies only to `lifecycle/` |
| POLICY092 | A layer turns on an element marked `deprecated: true` | Move to the element that replaces it, then remove `enabled: true` from the file the finding names |
