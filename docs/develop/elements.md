# Writing an element

Read [How an element is built](../concepts/elements.md) first: folders under `converters/` create
Policies and templates, and `values.yaml` only configures them. ACM terms (Policy, ConfigurationPolicy, OperatorPolicy,
`musthave`, inform/enforce) are explained in [ACM policies in brief](../concepts/acm.md).

Every key the library reads is in the
[policy-library README](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#reference).

## Before you start

| Need | For | Without it |
|---|---|---|
| `pip install -r tools/requirements.txt` (PyYAML) | `create-element.sh` and `tools/doc-generator.py` | The script creates `stack/<name>/`, then stops with `README not generated: ...`. Run the generator it names; don't rerun the script, which refuses an existing directory |
| `mise install`, then `mise activate` ([Setup](../contributing.md#setup)) | The Go, Helm and prek versions pinned in `mise.toml`. The validator and hooks need them | `make -C tools/validator` can't find `go`, or renders with a different Helm than Argo CD |
| `prek install` (optional) | Runs the repo hooks, including `element-readmes`, on each commit | A stale README fails CI instead of your commit |
| A nonprod cluster in your fleet. Its fleet file, `fleet/<cluster>.yaml`, says which Git revision and values layers it gets ([Fleet files](../concepts/overview.md#fleet-files)) | Steps 10-12 | You can't try the element on a cluster. Stop after step 9 and rely on CI |

policy-library is downloaded, not committed: `preview` and `validate` fetch it into
`stack/<name>/charts/` (gitignored), so the first run needs network access. Argo CD fetches it itself
when it renders.

## Workflow

1.  **Scaffold.**

    ```sh
    tools/create-element.sh demo "Demo element"
    ```

    Copies the [sample element](#sample-element) to `stack/demo/`, renames `my-element` to `demo`
    and `myElement` to its camelCase values key, and generates the README. It refuses a name that
    isn't kebab-case, is over 17 characters or collides with another element (POLICY091); see
    [Naming](#naming) for why.

    The copy is a working example: operator `demo` points at package `demo-operator`,
    which doesn't exist, and `converters/install/demo-instance.yaml` is a sample ConfigMap. Replace
    or delete both in steps 2 and 3.

2.  **Write converters.** A converter is one file, `converters/<policy>/<template>.yaml`. Each
    directory becomes a Policy, each file in it a ConfigurationPolicy named `<policy>-<template>`
    ([rendered names](../concepts/elements.md#rendered-names)). Put the manifests you want on the
    cluster in it, several separated by `---` if needed ([Converter shapes](#converter-shapes)).
    `converters/install/demo-instance.yaml` renders as:

    ```yaml title="Policy install-demo-<cluster> (excerpt)"
    kind: Policy
    metadata:
      name: install-demo-<cluster>          # Policy install of release demo-<cluster>
    spec:
      policy-templates:
        - objectDefinition:
            kind: ConfigurationPolicy
            metadata:
              name: install-demo-instance
            spec:
              object-templates:
                - complianceType: musthave      # the fields you list must match; others are left alone
                  objectDefinition:
                    apiVersion: v1
                    kind: ConfigMap
                    ...                         # your file, as written
    ```

    !!! warning "Pasting `oc get -o yaml` output"
        Strip `status` and every server-set metadata field: `uid`, `resourceVersion`, `generation`,
        `creationTimestamp`, `managedFields`, `ownerReferences` and the
        `kubectl.kubernetes.io/last-applied-configuration` annotation. ACM compares every field you
        list, and it can't set `status` or server-set fields, so a copied one keeps the template
        NonCompliant forever or ties it to one instance of the object.

    `.yml` files, files directly under `converters/` and names starting with `_` are not discovered
    ([partials](#partials-and-file-reuse)). POLICY021 warns when nothing renders one.

3.  **Declare operators.** Operators need no converter. Set `namespace`, `subscription.name`,
    `subscription.channel` and `displayName` under `operatorPolicies.<key>`. The library adds three templates to Policy
    `install`: `install-<key>-ns` (the Namespace), `install-<key>` (the OperatorPolicy, which
    creates the OperatorGroup and Subscription) and `install-<key>-status` (an inform check for a
    CSV with that `displayName` in phase `Succeeded`). Find the values on a cluster:

    ```sh
    oc get packagemanifests -n openshift-marketplace \
      -o custom-columns=NAME:.metadata.name,SOURCE:.status.catalogSource,DEFAULT:.status.defaultChannel | grep -i <product>
    # NAME is subscription.name, SOURCE subscription.source, DEFAULT the channel most clusters want
    oc get packagemanifest <package> -n openshift-marketplace \
      -o jsonpath='{range .status.channels[*]}{.name}: {.currentCSVDesc.displayName} {.currentCSVDesc.installModes[?(@.supported==true)].type}{"\n"}{end}'
    # per channel: displayName, then the supported install modes
    ```

    !!! danger "A wrong `displayName` never goes green"
        It defaults to `subscription.name`, which is rarely the display name. The status check then
        never matches: it stays NonCompliant, and every template that waits for the operator stays
        Pending, which `ignorePending` shows as Compliant. You see `install-<key>-status`
        NonCompliant on the hub while `oc get csv -n <namespace>` on the cluster shows the operator
        `Succeeded`. Compare that CSV's `spec.displayName` with yours.

    Without `operatorGroup.targetNamespaces` the OperatorGroup watches all namespaces. An operator
    whose install modes lack `AllNamespaces` fails to install that way: set
    `targetNamespaces: [<namespace>]` (`stack/compliance`). Installing into `openshift-operators`?
    Set `operatorGroup.name: global-operators` to reuse the existing one, because a second
    OperatorGroup in a namespace breaks OLM.

    No operator? Delete both `operatorPolicies` and the sample's `waitForOperator`. Deleting only
    `operatorPolicies` fails the render:
    `waitForOperator "demo" on policy "install" matches no enabled entry in operatorPolicies`.

4.  **Put configuration that needs the operator in its own Policy.** A CR, or anything in a
    namespace or CRD the operator creates, goes in its own Policy that depends on `install`, with
    `waitForOperator` on the template that needs the CRD
    ([waiting and ordering](../concepts/elements.md#waiting-and-ordering)).
    Policy `install` then means "operator installed", which other elements can wait on, and a layer
    can turn the configuration off without unmanaging the operator. The sample keeps its ConfigMap
    in `install` with `waitForOperator` only to stay one Policy; that works too. From
    `stack/metallb/values.yaml`:

    ```yaml
    policies:
      config:
        dependencies:
          - name: install           # Policy config-metallb-<cluster> stays Pending until install is Compliant
        templates:
          metallb-instance:
            waitForOperator: metallb  # this template waits for install-metallb-status
    ```

5.  **Move site data into `config`.** Site data is anything that differs between clusters or
    environments. Converters read it as `.Config`
    ([merge rules](../concepts/elements.md#settings-config-and-config)). Expose curated settings, a
    named key with a working default, and build the manifest in the converter. The sample does this
    with `logLevel`: `config.logLevel: info` in `values.yaml`, `logLevel: {{ .Config.logLevel | quote }}`
    in the converter. A layer then sets `logLevel: debug` instead of copying a CR spec. Nothing
    checks a setting's type. A map of name to spec is fine when the object is nothing but site data
    (`stack/metallb` `ipAddressPools`).

    `config` in the element's `values.yaml` is the contract:

    - Every top-level key a converter reads must be declared there, under the element's `config` or
      the `config` of each template that renders the converter (`stack/compliance` declares
      `severity` only per template). An undeclared key renders as null, and POLICY022 fails the PR.
    - A layer may only set declared top-level keys of the element's `config`. POLICY035 fails a typo
      that would otherwise do nothing. Keys below the top level are not checked.
    - The comment above each key becomes its row in the README.

6.  **Preview.**

    ```sh
    make -C tools/validator preview ELEMENT=demo CLUSTER=nonprod-west-1
    ```

    Turns the element on and prints `demo.yaml`, the Policies after Helm, and
    `demo.evaluated.yaml`, the objects ACM would apply after resolving ACM templates on an empty
    cluster.

    - `CLUSTER` names a fixture, a made-up fleet file in `tools/validator/testdata/clusters/`.
      Preview can't render your own clusters; a PR's Render Diff does (step 10). Without `CLUSTER`
      every fixture renders and only the file paths print. Output goes to a new temporary
      directory, which is left in place; `PREVIEW=<dir>` (relative to `tools/validator`) writes it
      there instead.
    - Only `stack.demo.enabled: true` is forced, so pieces shipped `enabled: false` stay off. To see
      one, turn it on temporarily in `values/clusters/nonprod-west-1.yaml`, which the fixture reads.

    What the two files show and their limits: [Preview](validation.md#preview).

7.  **Add it to the test profile.** Fixtures leave a new element off, so CI renders it only under
    `tools/validator/testdata/profiles/`. Add it to `all-on.yaml` under its values key, with every
    entry it ships `enabled: false` turned on, and a sample entry for any `config` map those entries
    range over (an empty map fails POLICY082, see [Converter shapes](#converter-shapes)). Otherwise
    CI never evaluates their ACM templates, and `TestProfileCoverage` fails for any converter that
    has some. Settings that conflict with one already in `all-on.yaml` go in `variants.yaml`
    ([Profiles](validation.md#profiles)).

    ```yaml title="tools/validator/testdata/profiles/all-on.yaml"
    stack:
      demo:               # the values key: cert-manager would be certManager
        enabled: true
        policies:
          install:
            templates:
              demo-netpol:
                enabled: true
    ```

8.  **Validate.**

    ```sh
    make -C tools/validator validate ELEMENT=demo
    ```

    Runs every [rule](../reference/rules.md) on this element. Repo-wide rules such as POLICY091
    still read every element. Each finding names the rule, element and source line;
    [Reading the output](validation.md#reading-the-output) explains them.

9.  **Regenerate the README** after any change to `Chart.yaml`, `values.yaml` or `converters/`:

    ```sh
    python tools/doc-generator.py --element demo
    ```

    With `prek install`, the `element-readmes` hook rewrites a stale README and fails the commit;
    `git add` it and commit again. CI fails on a stale README ([Element reference](#element-reference)).

10. **Pin a cluster to your branch.** On `feature/demo`, turn the element on in the cluster's own
    layer:

    ```yaml title="values/clusters/nonprod-west-1.yaml"
    stack:
      demo:
        enabled: true
    ```

    Push `feature/demo` to the repository Argo CD reads; a branch in a personal fork doesn't
    resolve. Then, from a separate branch, open a PR to the default branch that changes only the
    cluster's fleet file, and merge it:

    ```yaml title="fleet/nonprod-west-1.yaml"
    revision: feature/demo
    ```

    Fleet files are read only from the default branch, so an open PR changes nothing, and the same
    change on `feature/demo` would merge the element with it. Everything else, `stack/` and
    `values/` included, now comes from `feature/demo` for this cluster: the element and any other
    difference between its old revision and your branch. The PR's Render Diff comment shows exactly
    that ([Promoting and rolling back](../guides/rollout.md#promoting-and-rolling-back)). Each push to
    the branch reaches the cluster within minutes.

11. **Read the result** on the hub with
    `oc get policies.policy.open-cluster-management.io -n nonprod-west-1`; the element's Policies
    are `pstack.<policy>-demo-nonprod-west-1` ([Compliance states](../concepts/acm.md#compliance-states)).
    No `demo` Policies a few minutes after the merge means the render failed:
    `oc get application demo-nonprod-west-1 -n openshift-gitops -o jsonpath='{.status.conditions}'`
    shows the `ComparisonError`.

    The element starts in `inform`, which changes nothing on the cluster, so new config never reads
    Compliant there:

    - Objects that don't exist yet are NonCompliant. Inform creates nothing and approves no
      operator install.
    - Templates that wait for the operator are Pending, and `ignorePending` reports them as
      Compliant. That green means "not checked yet".

12. **Choose the default action and un-pin.** Check that every violation is one you expect, then
    pick how the element ships.
    `default.remediationAction: enforce` in its `values.yaml` changes every cluster that enables it
    at once; `inform` leaves each layer to opt in with
    `stack.demo.default.remediationAction: enforce`.

    Once everything is Compliant, update `feature/demo` from the default branch and, in the
    element's PR, set `revision` in `fleet/nonprod-west-1.yaml` back to the default branch. The
    merge then un-pins the cluster in the same commit (POLICY052 checks a pin to the base branch
    against the merge commit), so it doesn't matter if GitHub deletes the branch on merge. Never
    delete a branch while a fleet file pins it: `policystack` stops generating for every cluster on
    that hub ([When a revision breaks](../reference/fleet.md#when-a-revision-breaks)).

## Sample element

The tabs show the live files in `tools/sample-element/`, so they always match the repo: one
operator, one converter reading `.Config`, one optional template. `create-element.sh` renames
`my-element` to your name and `myElement` to your values key. `README.md` is
[generated](#element-reference).

=== "values.yaml"

    ```yaml
    --8<-- "tools/sample-element/values.yaml"
    ```

=== "converters/install/my-element-instance.yaml"

    ```yaml
    --8<-- "tools/sample-element/converters/install/my-element-instance.yaml"
    ```

=== "converters/install/my-element-netpol.yaml"

    ```yaml
    --8<-- "tools/sample-element/converters/install/my-element-netpol.yaml"
    ```

=== "Chart.yaml"

    ```yaml
    --8<-- "tools/sample-element/Chart.yaml"
    ```

- `Chart.yaml` and `templates/policy.yaml`: leave them as they are
  ([Anatomy](../concepts/elements.md#anatomy)). Named templates several converters share go in
  `templates/_helpers.tpl`; converters can `include` them (`stack/compliance`
  `compliance.waivedRules`).
- `versions: []` follows the channel. Pin per environment in layers
  ([Operator versions](../guides/operators.md)).

## What you leave out

Write a key only when its default is wrong for your element. An arrow means "the first one set
wins, left to right". Template means `policies.<p>.templates.<t>`, operator means
`operatorPolicies.<k>`.

| Key | Resolves to |
|---|---|
| `enabled` on the element | **off** unless `true`. Elements ship `false`; a layer turns them on |
| `enabled` on a policy, template or operator | on |
| `policies` | one Policy per `converters/` directory, plus `install` when there are operators |
| `policies.<p>.templates` | one template per `converters/<p>/*.yaml` |
| `remediationAction` | `policies.<p>` → `default` → `inform`. `global.remediationAction: inform` overrides it ([Dry run](../guides/rollout.md#dry-run)) |
| `severity` | template or operator → `policies.<p>` → `default` → `low` |
| `categories`, `controls`, `standards` | `policies.<p>` → `default` → none |
| template `complianceType` | `musthave` (fields you list must match; [all three](../concepts/acm.md#configurationpolicy)). `mustonlyhave` also removes fields you don't list; `mustnothave` deletes the object under enforce |
| template or operator `ignorePending` | `false`; `true` when `waitForOperator` is set, so a template waiting on the operator counts as Compliant |
| operator `subscription.source`, `subscription.sourceNamespace` | `redhat-operators`, `openshift-marketplace` |
| operator `upgradeApproval` | `Automatic` |
| operator `operatorGroup.name` | `subscription.name`, watching all namespaces |
| operator `displayName` | `subscription.name` |

!!! warning "Template `remediationAction` accepts only `InformOnly`"
    It keeps one template reporting inside an `enforce` Policy
    ([inform, enforce and InformOnly](../concepts/acm.md#inform-enforce-and-informonly)). Set it on
    every template that only reports: readiness checks and `mustnothave` signals. Any other value
    fails the render: `remediationAction must be one of the following: "InformOnly", "", null`.

## Converter shapes

The library runs each converter through Helm with the usual `.Values`, `.Release` and `.Files`,
plus `.Config`. It then looks at the trimmed **output**, not your source, to pick the
ConfigurationPolicy field:

=== "One object"

    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-element
      namespace: my-element
    data:
      logLevel: {{ .Config.logLevel | quote }}
    ```

    Renders one `object-templates` entry: `complianceType: musthave` and this ConfigMap as its
    `objectDefinition`.

=== "Documents"

    One `object-templates` entry per `---` document. Empty documents are dropped. With
    `config.maps: {a: {k: "1"}, b: {k: "2"}}` this renders two entries, ConfigMaps `a` and `b`:

    ```yaml
    {{- range $name, $data := .Config.maps }}
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: {{ $name }}
      namespace: my-element
    data: {{- toYaml $data | nindent 2 }}
    ---
    {{- end }}
    ```

    The file starts with `{{- range`, but Helm consumes that action, so the output starts with
    `apiVersion` and this is not raw.

    !!! note
        A `---` must start the line and stand alone to separate documents. A leading `---` and
        full-line `#` comments are fine. `--- # c` is not a separator: at the top of the file it
        makes the output start with `-`, so it is treated as raw; between objects it stays inside
        the previous document and the render fails with a YAML parse error. Put the comment on its
        own line.

=== "Raw list"

    Output starting with `-`, `[` or `{{` goes verbatim into `object-templates-raw`, a string ACM
    evaluates on the managed cluster. Use it when the cluster decides how many objects there are.
    In practice a leading `{{` after Helm comes from an escaped ACM template
    ([Helm templates and ACM templates](../concepts/elements.md#helm-templates-and-acm-templates)):

    ```yaml
    {{ "{{-" }} range (lookup "v1" "Namespace" "" "" "team=a").items {{ "}}" }}
    - complianceType: musthave
      objectDefinition:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: my-element
          namespace: {{ "{{" }} .metadata.name {{ "}}" }}
    {{ "{{-" }} end {{ "}}" }}
    ```

    Renders `object-templates-raw: |` followed by this block with the escapes removed
    (`{{- range (lookup ...` on the first line). You write each `complianceType` yourself, so the
    template's `complianceType`, `metadataComplianceType`, `recordDiff`, `recreateOption` and
    `objectSelector` don't apply. A raw list must be the template's only output; next to another
    document, or in a `file` list, it fails with
    `renders a raw object list next to other documents`.

Output that is empty renders `object-templates-raw` as the string `[]` (an empty raw list). ACM
reports a ConfigurationPolicy with no objects as Compliant without checking anything, so a Policy
whose templates are all empty fails POLICY082. The usual cause is a converter ranging over a
`config` map whose default is `{}` (`stack/metallb` `ipAddressPools`): give the map an entry
wherever the template is enabled, and in the test profile (step 7).

### Escaping

Every ACM `{{ }}` in a converter is escaped from Helm as `{{ "{{" }} ... {{ "}}" }}`; why, and the
errors an unescaped one gives, are in
[Helm templates and ACM templates](../concepts/elements.md#helm-templates-and-acm-templates). To
use a setting inside an ACM expression, leave that part unescaped so Helm fills it in first:
`'{{ "{{" }} lookup "v1" "ConfigMap" "{{ .Config.namespace }}" "x" {{ "}}" }}'` reaches ACM as
`{{ lookup "v1" "ConfigMap" "my-ns" "x" }}`. Escaped, ACM receives `.Config`, which it doesn't
have, and POLICY081 fails. `make preview` shows both stages: `<element>.yaml` after Helm,
`.evaluated.yaml` after ACM.

## Partials and file reuse

When and why to share a converter is in
[Partials and reuse](../concepts/elements.md#partials-and-reuse). To do it:

1.  Name the shared file or its directory with a leading `_` (`converters/findings/_findings.yaml`,
    `converters/_shared/`), so discovery doesn't turn it into a template or Policy of its own.
2.  Point each template at it with `templates.<t>.file`: the path under `converters/` without
    `.yaml` (`file: findings/_findings`), or a list whose objects are concatenated
    (`file: [_shared/acs-namespace, secured/tls]`). Give each template its own `config` if the
    partial should render differently.
3.  Preview. A path that doesn't exist fails with
    `converters/<path>.yaml, a file of <p>-<t>, is missing or empty`. A raw converter can't be in
    a list ([Converter shapes](#converter-shapes)).

`file` also replaces a discovered `converters/<p>/<t>.yaml`; POLICY021 then warns that the file is
unused.

## Optional pieces

Ship a policy, template or operator with `enabled: false` for things only some clusters want. A
layer turns it on with its full path:

```yaml title="values/clusters/nonprod-west-1.yaml"
stack:
  demo:
    policies:
      install:
        templates:
          demo-netpol:
            enabled: true
```

The README lists it under *Optional pieces* with that path. Give every input it reads a working
default in `config`. A `config` map with no sensible default stays `{}`: say in its comment that
the layer turning the piece on must fill it, or CI fails POLICY082 for that cluster
([Converter shapes](#converter-shapes)).

## Secrets

Values live in Git, so secrets don't. A converter reads them on the hub at runtime with a hub
template, `{{hub ... hub}}`
([Hub and managed-cluster templates](../concepts/acm.md#hub-and-managed-cluster-templates)). The
propagator re-resolves it when the Secret changes, so a rotation reaches clusters without a
commit. `stack/acs/converters/secured/tls.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: '{{ "{{" }}hub if eq (index .ManagedClusterLabels "local-cluster") "true" hub{{ "}}" }}{{ "{{" }} skipObject {{ "}}" }}{{ "{{" }}hub end hub{{ "}}" }}cluster-registration-secret'
  namespace: stackrox
type: Opaque
data: '{{ "{{" }}hub copySecretData "stackrox" "cluster-registration-secret" hub{{ "}}" }}'
```

The `name` line skips the object on the hub itself (label `local-cluster: "true"`): there the hub
template emits `{{ skipObject }}`, which the cluster then runs, because the source Secret already
lives on the hub and copying it onto itself would overwrite it.

- `copySecretData` copies every key of the hub Secret's `data`, not its type, labels or
  annotations.
- `fromSecret <ns> <name> <key>` returns one key's base64 value, so it belongs under `data`. A
  missing key returns `''`; a missing Secret is a template error.
- By default hub templates may only read the policy namespace (`pstack`). `stack/acs` reads
  `stackrox`, so its Policy runs them as a ServiceAccount:

    ```yaml
    policies:
      secured:
        hubTemplateOptions:
          serviceAccountName: acs-hub-templates
    ```

    You create that ServiceAccount in `pstack` and give it read access to exactly what the
    templates read. ACS does it in its `central` Policy, which ships `enabled: false` and is turned
    on only in the hub's layer (`values/acm/<hub>.yaml`), so it runs on the hub where `pstack` is.
    Its `hub-access` template (`stack/acs/converters/central/hub-access.yaml`) grants get/list/watch
    on the one Secret and Route, plus ManagedClusters for `.ManagedClusterLabels`. Without
    `serviceAccountName` the propagator refuses the lookup and POLICY081 fails; without the RBAC
    the hub denies it.

## Naming

Element, policy and template names end up in every object name, and ACM caps names at 63
characters ([Naming limits](../concepts/policies.md#naming-limits)).

- **Lowercase kebab-case, directory name equal to the chart `name`**
  ([why](../concepts/elements.md#anatomy)). The values key is its camelCase
  (`cert-manager` → `certManager`).
- **At most 17 characters.** `pstack.install-<element>-<cluster>` must fit 63 with a 30-character
  cluster name. Policy name and element name together are at most 24 characters, not counting the
  hyphens (`ingress-cert` + `cert-manager` = 24), so `pstack.<policy>-<element>-` leaves 30 for the
  cluster.
- **Not `<another element>-...`, and no other element `<yours>-...`.** `gitops` on cluster `dev-c`
  and `gitops-dev` on cluster `c` would both be release `gitops-dev-c` (POLICY091). `lifecycle` is
  taken: the `lifecycle/` chart's releases are `lifecycle-<cluster>`.
- **Distinctive template names.** `<policy>-<template>` must be unique on a cluster across all
  elements ([why](../concepts/elements.md#rendered-names)). In shared Policy names (`config`,
  `ready`, `install`) name templates after the product (`metallb-instance`, not `instance`).
  Operator keys count: they render `install-<key>`.
- **No `openshift-` prefix or `-operator` suffix.** An element names the product, and both spend
  the 17-character budget for nothing.

!!! warning "Renaming recreates Policies"
    Renaming an element or policy deletes its Policies on every cluster and creates new ones;
    renaming a template replaces its ConfigurationPolicy inside the same Policy. Either way, objects
    on the cluster stay unless the template sets `pruneObjectBehavior`
    ([Removal and pruning](../concepts/removal.md#what-happens-on-the-managed-cluster)).

## Good practices

- **One product per element.** Clusters turn an element on as a unit.
- **`categories`, `controls`, `standards` only for controls the element implements.** The library
  adds none, and the Governance dashboard reports what you claim
  ([annotations](../concepts/acm.md#severity-and-compliance-annotations)).
- **`pruneObjectBehavior` in its own commit, before any removal**, because landed together the
  cluster never sees prune and deletes nothing. Never on a template with dependencies (POLICY033,
  [why](../concepts/removal.md#setting-prune)).

## Element reference

`stack/<element>/README.md` is generated from `Chart.yaml`, `values.yaml` and `converters/`. Don't
edit it.

```sh
pip install -r tools/requirements.txt
python tools/doc-generator.py                   # all elements
python tools/doc-generator.py --element <name>  # one element
python tools/doc-generator.py --check           # report stale ones, write nothing
```

| Source | Shown as |
|---|---|
| `Chart.yaml` `description` | Lead paragraph |
| `Chart.yaml` `home` | Link at the top of the README. Set it to the element's page on the docs site if it has one; leave it out otherwise |
| Comment above `config:` | Settings intro |
| Comment above a key under `config` | That row's description |
| `description:` of a policy, template or operator | Policy line or template row |
| `enabled: false` entries | *Optional pieces* table |

A blank line between a comment and its key keeps the comment out, for maintainer notes. When the
hook and CI run the generator: [Hooks](../reference/ci.md#hooks).
