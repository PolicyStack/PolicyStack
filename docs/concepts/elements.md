# How an element is built

An element is one product's configuration, packaged as a Helm chart under `stack/<element>/`:
`gitops` installs OpenShift GitOps and its Argo CD instance, `metallb` installs MetalLB and its
address pools. Argo CD on the hub renders every element once per cluster, as an Application named
`<element>-<cluster>`; a disabled element renders nothing. The output is ACM Policies in the `pstack`
namespace on the hub.
Nothing in an element is applied to a cluster directly: ACM delivers the Policies and enforces them
([ACM policies in brief](acm.md)).

The examples below follow
[`stack/gitops`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/gitops) from its files to
its Policies. To write an element, read this first, then follow [Writing an element](../develop/elements.md).

## Anatomy

```yaml
stack/gitops/
├── Chart.yaml                   # (1)!
├── values.yaml                  # (2)!
├── templates/policy.yaml        # (3)!
├── converters/                  # (4)!
│   ├── instance/
│   │   ├── argocd.yaml
│   │   └── ca-bundle.yaml
│   └── link/
│       └── argocd-console.yaml
└── README.md                    # (5)!
```

1.  `name: gitops`, equal to the directory name, and one dependency: the [policy-library](https://github.com/PolicyStack/PolicyStack/blob/main/lib/policy-library/README.md) chart with `alias: stack`, `version: "^4.0.0"` and `repository: file://../../lib/policy-library`. The library does all the rendering. It lives in this repo, so Helm takes it from `lib/` at the same Git revision as the element; `version` is the lowest 4.x the element needs ([Library changes](../contributing.md#library-changes)). Helm passes a subchart the values under its alias and checks them against the subchart's `values.schema.json`, so everything under `stack:` is validated: a misspelled key such as `enabeld` fails the render instead of being ignored.
2.  The element's defaults, all under `stack.gitops`. A chart named `user-monitoring` reads `stack.userMonitoring`: the library camelCases the chart name, because Go templates can't read a key containing `-` with dot syntax. Below, `<el>` stands for this key. A key that doesn't match the chart name renders nothing (POLICY090, a [validator rule](../reference/rules.md) your repository's CI runs).
3.  One line, `{{- include "policy-library.render" . -}}`, in nearly every element (`compliance` and `upgrade` add validation before it).
4.  The manifests. A **converter** is one file here: a Kubernetes manifest with Helm templating that "converts" the element's settings into the objects ACM enforces.
5.  Generated from `Chart.yaml`, `values.yaml` and `converters/` by `tools/doc-generator.py`. See [Reading an element's README](#reading-an-elements-readme).

Keep the directory name equal to the chart `name`. The ApplicationSet names the release
`<directory>-<cluster>`, and the library recovers the cluster by stripping `<chart name>-` from it.
With a mismatch the stripping fails, and any `element:` dependency ([Waiting and ordering](#waiting-and-ordering))
or `suffixTemplateNames` fails the render: `requires Release.Name (...) to start with the chart name`.

## Files create, values configure

Converter files create policies and templates; `values.yaml` only configures them, with two
exceptions: `operatorPolicies` and `templates.<t>.file`. The library scans
`converters/<policy>/<template>.yaml`. A folder name becomes a Policy, a file name (minus `.yaml`)
a **template**, and the file's contents the objects that template enforces. In ACM terms a template
is one entry in the Policy's `policy-templates`, here a ConfigurationPolicy named
`<policy>-<template>`. It is not a Helm template.

Rendering gitops for cluster `c1` (release `gitops-c1`: the Application and Helm release name,
`<element>-<cluster>`) gives:

```text
Placement         gitops-c1
PlacementBinding  gitops-c1                    # binds the three Policies to the Placement
Policy            install-gitops-c1            # from operatorPolicies.gitops
  ConfigurationPolicy   install-gitops-ns
  ConfigurationPolicy   install-gitops-status
  OperatorPolicy        install-gitops
Policy            instance-gitops-c1           # from converters/instance/
  ConfigurationPolicy   instance-argocd        #   argocd.yaml
  ConfigurationPolicy   instance-ca-bundle     #   ca-bundle.yaml
Policy            link-gitops-c1               # from converters/link/
  ConfigurationPolicy   link-argocd-console    #   argocd-console.yaml
```

The gitops `values.yaml` has entries for `instance`, `link` and their templates, but deleting every
one of them would still render the same three Policies. They only set options: descriptions,
dependencies, severity, per-template `config`. `policies.<p>` and `policies.<p>.templates.<t>`
configure what the folders created.

| You add | You get |
|---|---|
| `converters/<p>/<t>.yaml` | Template `<p>-<t>` in Policy `<p>-<release>`. The first file in a new folder creates the Policy. |
| `operatorPolicies.<k>` with `subscription.name`, in the element's `values.yaml` (a layer may only override existing entries; POLICY032) | Three templates in Policy `install-<release>`: `install-<k>-ns`, `install-<k>-status` and OperatorPolicy `install-<k>` ([The install Policy](#the-install-policy)). |
| `policies.<p>.templates.<t>.file` | Template `<p>-<t>` rendered from the file(s) named, and Policy `<p>` if it is new. The other values key that creates something ([Reusing files](#reusing-files)). A new policy belongs in the element's `values.yaml`; a layer may only add templates to existing policies (POLICY035). |
| Any other `policies` or `templates` key | Options on an entry that already exists. Nothing new. |

### Names that don't exist

A values layer (a values file under `values/` that the cluster reads; [Values cascade](values.md)) can't invent a policy or a template by naming it:
anything it names must be backed by a file, a `file` key or an operator, and the library fails the
render rather than ignore a typo. The checks run before `enabled` is applied, so `enabled: false` on a misspelled
template fails too. All errors below come from rendering gitops:

| In a values layer | Result |
|---|---|
| `policies.ghost.enabled: true` | `policies.ghost has no templates - add converters/ghost/<template>.yaml` |
| `policies.instance.templates.typo.enabled: false` | `policies.instance.templates.typo has no file - add converters/instance/typo.yaml or set file` |
| `policies.extra.templates.cm.file: instance/nope` | `converters/instance/nope.yaml, a file of extra-cm, is missing or empty` |
| `operatorPolicies.typo.namespace: x` | `operatorPolicies.typo has no subscription.name - a values layer may set an operator the element does not define` |
| `stack.gitops.enabeld: true` | Schema error: `gitops: Additional property enabeld is not allowed` |
| `stack.gitOps.enabled: true` (wrong element key) | **Renders nothing, no error.** Only POLICY035 in CI catches it. |
| `stack.gitops.config.bogus: 1` | Renders; the converters never read it. POLICY035 catches unknown top-level `config` keys, nothing checks deeper ones. |

A render error stops the whole Application: Argo CD shows a `ComparisonError` with the message, and
nothing for that element on that cluster syncs or prunes until the layer is fixed
([Applications](architecture.md#applications)). A typo in a layer stops this element on every
cluster that reads it; a schema error under `stack` stops every element there
([Schema](values.md#schema)). Your repository's CI ([Your repository's CI](overview.md#your-repositorys-ci)) runs the same
render on every pull request, so these normally fail there first
([Validation](../develop/validation.md)).

## The install Policy

`install` exists when `operatorPolicies` is non-empty. Each entry becomes three templates:

| Template | Kind | Does |
|---|---|---|
| `install-<k>-ns` | ConfigurationPolicy | Creates the operator's Namespace. Left out when `operatorPolicies.<k>.complianceType: mustnothave` (uninstall). |
| `install-<k>` | OperatorPolicy | Manages the Subscription, OperatorGroup and InstallPlan approval ([Operator versions](../guides/operators.md)). |
| `install-<k>-status` | ConfigurationPolicy | Checks that a CSV whose `spec.displayName` equals `operatorPolicies.<k>.displayName` (default `subscription.name`) reached `Succeeded`. It takes the Policy's action but names no CSV, so even under enforce it only reports. Templates with `waitForOperator: <k>` wait on it; if `displayName` doesn't match the CSV exactly (`oc get csv -n <namespace>`), it never passes and they never run. |

Files in `converters/install/` join the same Policy, rendered before the operators. The
[sample element](../develop/elements.md#sample-element) does both: its `my-element-instance`
template (a ConfigMap) and its operator both sit in Policy `install-my-element-<cluster>`. Turning off
`policies.install` therefore turns off every operator of the element.

Don't confuse this Policy with the `install/` chart or a fleet file's `install:` key
([Words with more than one meaning](overview.md#words-with-more-than-one-meaning)).

## Partials and reuse

Discovery (the scan of `converters/<policy>/<template>.yaml` above) skips two kinds of file, which
render only when a `templates.<t>.file` names them:

- **Partials**: a file or folder whose name starts with `_`, such as `findings/_findings.yaml` or
  `_shared/`. Prefix a file meant only for `file` with `_`; otherwise discovery also renders it as a
  template of its own.
- Files directly under `converters/`, such as `converters/guard.yaml` of the
  [lifecycle](lifecycle.md) chart.

A `.yml` file never renders: discovery skips it and `file` always appends `.yaml`. Rename it.

POLICY021 warns about a converter nothing renders.

### Reusing files

`templates.<t>.file` says "this template's contents come from this other file". It is a path under
`converters/` without `.yaml`, or a list of them, and it replaces the file discovery would have used.
Two uses:

=== "One file, several templates"

    [`stack/compliance`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/compliance)
    reports findings per severity. `high`, `medium` and `low` have no file of their own: `file`
    creates all three from one partial, each with its own `config`.

    ```yaml title="stack/compliance/values.yaml (excerpt)"
    policies:
      findings:
        templates:
          high:
            file: findings/_findings
            config:
              severity: "compliance.openshift.io/check-severity=high"   # (1)!
          medium:
            file: findings/_findings
            severity: medium                                            # (2)!
            config:
              severity: "compliance.openshift.io/check-severity=medium"
    ```

    1.  Read by the partial as `.Config.severity`, a label selector.
    2.  The ACM severity of ConfigurationPolicy `findings-medium`. Unrelated to `config.severity`.

=== "Several files, one template"

    [`stack/acs`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/acs) needs its
    namespace in both `central` and `secured`, because either can be on a cluster without the
    other. `central/central` is the file discovery would use anyway; the list puts the shared
    namespace in front of it, concatenating the objects of each file into one ConfigurationPolicy:

    ```yaml title="stack/acs/values.yaml (excerpt)"
    policies:
      central:
        templates:
          central:
            file: [_shared/acs-namespace, central/central]
      secured:
        templates:
          tls:
            file: [_shared/acs-namespace, secured/tls]
    ```

Rule of thumb: if you are about to copy a converter, use `file`. Otherwise add a file.

## Settings: config and .Config

`stack.<el>.config` holds the element's settings: the curated, typed values an admin is meant to
change, such as gitops `ha.enabled` or `rbacPolicies`. Converters read it as `.Config`. Values never
carry a raw CR spec: the converter builds the manifest from the settings, so layers stay small and
typed, and a CR API change is fixed once in the converter rather than in every layer.

For each template, `.Config` is the element `config` deep-merged with that template's own
`templates.<t>.config`, the template winning. gitops sets the console text this way:

```yaml title="stack/gitops/values.yaml (excerpt)"
stack:
  gitops:
    policies:
      link:
        templates:
          argocd-console:
            config:
              consoleText: Red Hat OpenShift GitOps   # .Config.consoleText in argocd-console.yaml only
```

Layers change `config` like any other values: maps merge key by key, lists are replaced whole
([Merge rules](values.md#merge-rules)). The library schema does not validate `config`. POLICY022
fails a converter that reads a top-level key missing from the element's `values.yaml`, and POLICY035
fails a layer that sets one.

## What renders

- **The element is off** until a values layer sets `stack.<el>.enabled: true`. Every shipped
  element is off except `upgrade`, which drives and reports OpenShift version upgrades and renders
  nothing until a fleet file sets `version` or `channel`, or a layer sets
  `stack.upgrade.config.upstream` ([OpenShift upgrades](../guides/upgrades.md)).
- **Everything inside it is on.** Every policy, template and operator renders unless it sets
  `enabled: false`. Elements ship a piece off when it needs per-cluster input, is one of several
  alternatives (platform, storage backend), or is risky to apply by default; the README lists those
  under *Optional pieces*.
- **`null` means unset.** A layer setting `enabled: null` on an entry an earlier layer turned off
  turns it back on.
- **A Policy with nothing enabled renders nothing.** Disable `instance-argocd` and
  `instance-ca-bundle` and Policy `instance-gitops-<cluster>` disappears. Its dependents are not
  adjusted: `link-gitops-<cluster>` still waits for it and stays Pending, so turn them off in the
  same layer (`policies.link.enabled: false`).
- **`operatorPolicies.<k>.enabled: false` stops managing the operator; it doesn't uninstall it.**
  It also fails the render while a template waits on that operator:
  `waitForOperator "gitops" on policy "instance" matches no enabled entry in operatorPolicies`.
  Turn the waiting templates off in the same layer
  ([Stopping and uninstalling](../guides/operators.md#stopping-and-uninstalling)).

A template whose converter renders no objects still renders, with `object-templates-raw: []` (the
string field converter output goes into when it starts with an ACM template or a list;
[Converter shapes](../develop/elements.md#converter-shapes)), and ACM reports it Compliant without
checking anything. POLICY082 fails a Policy whose templates are all
empty.

## Inform or enforce

Each Policy gets a `remediationAction`, the first one set of:

1. `policies.<p>.remediationAction`
2. `default.remediationAction` (`stack.<el>.default`, the element-wide defaults block)
3. `inform`

gitops sets `default.remediationAction: enforce`, so enabling it changes the cluster at once.

| Setting | Rule |
|---|---|
| Dry run | `global.remediationAction: inform` in any layer a cluster reads overrides every Policy of every element ([Global keys](values.md#global-keys), [Dry run](../guides/rollout.md#dry-run)) |
| Template action | Only `InformOnly`; any other value fails the schema ([inform, enforce and InformOnly](acm.md#inform-enforce-and-informonly)) |
| `severity` | template or operator → `policies.<p>` → `default` → `low` |

Full defaults: [What you leave out](../develop/elements.md#what-you-leave-out). What inform and
enforce do on the cluster: [ACM policies in brief](acm.md).

## Waiting and ordering

Policies and templates render in name order, but that is output order only. ACM evaluates them
independently unless something says to wait. Until a dependency is met, ACM does not create the
waiting template on the cluster and reports it **Pending**, and deletes it again if the dependency
later stops being met ([Dependencies and ignorePending](acm.md#dependencies-and-ignorepending)).

| Key | Waits for |
|---|---|
| `policies.<p>.dependencies: [{name: install}]` | Policy `install-<release>` of the same element to be Compliant. |
| `dependencies: [{name: ready, element: odf}]` | Policy `ready-odf-<cluster>`: another element on the same cluster. If odf isn't enabled there, that Policy never exists and the wait never ends. |
| `templates.<t>.extraDependencies: [{name: central-ready}]` | Template `central-ready` of the same Policy (`name` is the template key, rendered `<p>-central-ready`). |
| `templates.<t>.waitForOperator: <k>` | `install-<k>-status`, the operator's CSV reaching `Succeeded`. Also sets `ignorePending: true`, so the Policy reads Compliant while waiting. |
| `templates.<t>.ignorePending: true` | Adds no wait. While the template is Pending, the Policy's compliance ignores it, so the Policy can read Compliant. |
| `policies.<p>.orderManifests: [a, b, c]` | Chains every template of Policy `p`: `a`, `b`, `c` first, then the unlisted templates in name order, then its operators, each waiting for the one before to be Compliant. Used by [lifecycle](lifecycle.md) for its build steps. |

gitops chains its three Policies: `instance` depends on `install`, `link` on `instance`, and
`instance-argocd` waits for the operator, because the ArgoCD CR can't be created before its CRD
exists. The rendered `instance` Policy:

```yaml title="Policy instance-gitops-c1 (excerpt)"
spec:
  dependencies:
    - kind: Policy
      name: install-gitops-c1      # policies.instance.dependencies: [{name: install}]
      namespace: pstack
      compliance: Compliant
  policy-templates:
    - extraDependencies:           # waitForOperator: gitops
        - kind: ConfigurationPolicy
          name: install-gitops-status
          compliance: Compliant
      ignorePending: true
      objectDefinition:
        kind: ConfigurationPolicy
        metadata:
          name: instance-argocd
```

On a new cluster this reads: install the operator, wait for its CSV, then create the ArgoCD
instance, then the console link. Dependency entry fields (`kind`, `compliance`, `release`, `raw`)
are listed in the
[library README](https://github.com/PolicyStack/PolicyStack/blob/main/lib/policy-library/README.md#ordering-and-dependencies).

## Helm templates and ACM templates

Here "template" means a Go template expression `{{ }}`, not a policy template. A converter can hold
two kinds, evaluated at different times in different places:

| Data | Template expressions evaluated by | Write |
|---|---|---|
| `values.yaml`, layers, `.Config` | Helm, when Argo CD renders on the hub | `{{ .Config.x }}` |
| Objects on the managed cluster (`lookup`, `fromConfigMap`) | ACM's config-policy controller on the managed cluster, every evaluation | `{{ "{{" }} lookup ... {{ "}}" }}` |
| Hub objects and cluster labels (`copySecretData`, `.ManagedClusterLabels`) | ACM's policy propagator on the hub, before it copies the Policy to the cluster and again when an object it reads changes | `{{ "{{" }}hub ... hub{{ "}}" }}` |

Helm runs first and would evaluate ACM's braces too, so they are escaped: Helm prints
`{{ "{{" }}` as `{{`. Anything known from values belongs to Helm. The gitops console link uses both:

=== "Converter"

    ```yaml title="converters/link/argocd-console.yaml (excerpt)"
    text: {{ .Config.consoleText | quote }}
    href: '{{ "{{" }} $host := (lookup "route.openshift.io/v1" "Route" "{{ .Config.namespace }}" "openshift-gitops-server").spec.host {{ "}}" }}{{ "{{" }} skipObject (not $host) {{ "}}" }}https://{{ "{{" }} $host {{ "}}" }}'
    ```

=== "After Helm (in the Policy)"

    ```yaml
    text: "Red Hat OpenShift GitOps"
    href: '{{ $host := (lookup "route.openshift.io/v1" "Route" "openshift-gitops" "openshift-gitops-server").spec.host }}{{ skipObject (not $host) }}https://{{ $host }}'
    ```

    Helm filled in the text and the namespace. The rest is now an ACM template.

=== "On the managed cluster"

    ACM looks up the cluster's own Route and writes `href: https://<route host>`. Until the Route
    exists, `skipObject` leaves the ConsoleLink out instead of creating a broken link.

Forgetting to escape usually fails at render: Helm's `lookup` returns an empty map, so reading a
field from it fails (`nil pointer evaluating interface {}.<field>`), and Helm doesn't know ACM-only
functions (`function "fromSecret" not defined`). Some mistakes render silently instead: an
unescaped `.ManagedClusterName` becomes an empty string. Check the rendered Policy
([Validation](../develop/validation.md)).

Helm evaluates only converter files; strings from values are inserted as-is. So ACM templates inside
values, such as `policies.<p>.templates.<t>.customMessage.noncompliant` (the ConfigurationPolicy's violation
message), are written unescaped: `noncompliant: "{{ .DefaultMessage }}: ..."`. Reading hub secrets with
hub templates is covered in [Secrets](../develop/elements.md#secrets).

## Rendered names

`<release>` is the Application name, `<element>-<cluster>`; on the hub itself, `hubName` (the name
given to the hub's install, since ACM calls the hub `local-cluster`) stands in for the cluster name
([The hub](architecture.md#the-hub)).

| Object | Name | gitops on `c1` |
|---|---|---|
| Policy | `<policy>-<release>` | `instance-gitops-c1` |
| ConfigurationPolicy | `<policy>-<template>` | `instance-argocd` |
| OperatorPolicy | `install-<k>` | `install-gitops` |
| Placement, PlacementBinding | `<release>` | `gitops-c1` |

Everything lives in `pstack` on the hub; ConfigurationPolicies and OperatorPolicies are read inside
the Policy YAML there ([The objects](acm.md#the-objects)).

Template names must be unique per cluster ([ACM](acm.md#how-a-policy-reaches-a-cluster)) but carry
neither element nor cluster, so include the product in them ([Naming](../develop/elements.md#naming)).
CI fails clashes (POLICY002 within, POLICY003 between elements). Lifecycle places every cluster's release on the hub, so it appends the cluster
to its template names (`suffixTemplateNames`, see [Cluster lifecycle](lifecycle.md)). ACM also copies
each Policy as `pstack.<policy>-<release>`, which limits name lengths
([Naming limits](policies.md#naming-limits)).

!!! warning "Renaming deletes and recreates Policies"
    Element, policy and template names are part of every object name. Renaming one deletes its
    Policies and creates new ones on every cluster that has the element
    ([Removal and pruning](removal.md)).

## Reading an element's README

Each `stack/<element>/README.md` is generated from the element's files and lists what a values
layer can set. Browse them on [GitHub](https://github.com/PolicyStack/PolicyStack/tree/main/stack).

| Section | Shows | You set |
|---|---|---|
| Title and first paragraph | What the element does | `stack.<el>.enabled: true` |
| *Optional pieces* | Entries shipped `enabled: false` | The `Turn on with` key, under `stack.<el>` |
| *Settings* | Every `config` key with its default | `stack.<el>.config.<setting>` |
| *Operators* | Each operator's package, channel, source, namespace and pinned versions | `stack.<el>.operatorPolicies.<k>.versions` and the other `operatorPolicies` keys, where `<k>` is the Operator policy name minus `install-` ([Operator versions](../guides/operators.md)) |
| *Policies* | Each Policy's name on the hub (`<policy>-<element>-<cluster>`), its dependencies, and every template with its action, severity, waits and source file | `stack.<el>.policies.<p>...` |

In the *Policies* tables, `inform (always)` marks an `InformOnly` template, `(off)` an optional one,
and "compliant while pending" a template with `ignorePending`. The full list of keys is in the
[policy-library README](https://github.com/PolicyStack/PolicyStack/blob/main/lib/policy-library/README.md#reference).
