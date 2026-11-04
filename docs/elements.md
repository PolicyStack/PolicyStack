# Writing an element

An element is a Helm chart under `stack/` that renders ACM policies through
[policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md).
Manifests go in `converters/<policy>/<template>.yaml`. `values.yaml` holds curated settings and the
exceptions to the [defaults](#what-you-leave-out). The library README lists every key.

## Workflow

1.  **Scaffold.**

    ```sh
    tools/create-element.sh demo "Demo element"
    ```

    Copies the [sample element](#sample-element) to `stack/<name>/`, renames `my-element` and
    `myElement`, and generates the README. Refuses a name that isn't kebab-case, is over 17 characters
    or collides with another element (POLICY091).

2.  **Drop manifests into `converters/<policy>/`.** Each directory is a Policy, each file in it one
    ConfigurationPolicy named `<policy>-<file>`. Set the operator's `namespace`, `subscription` and
    `displayName` (the CSV display name the status check matches) under `operatorPolicies`.

3.  **Move site data into `config`.** Converters read it as `.Config`. Expose curated, typed settings
    and build the manifest in the converter; don't pass a CR spec through values. Comment each key:
    the comment is its README description.

4.  **Preview.**

    ```sh
    make -C tools/validator preview ELEMENT=demo CLUSTER=nonprod-west-1
    ```

    Turns the element on and prints the rendered Policies and `<element>.evaluated.yaml`, the objects
    ACM would apply with ACM templates resolved on an empty cluster. Without `CLUSTER` it renders every
    fixture into a temporary directory, or `PREVIEW=<dir>` (relative to `tools/validator`).

5.  **Validate.**

    ```sh
    make -C tools/validator validate ELEMENT=demo
    ```

    Every [rule](validation.md#rules), this element only. Add it to
    `tools/validator/testdata/profiles/all-on.yaml`, or CI never evaluates its ACM templates
    ([ACM templates](validation.md#acm-templates)).

6.  **Enable on one cluster.** Set `enabled: true` in `values/clusters/<cluster>.yaml` of a nonprod
    cluster and pin its fleet file to your branch
    ([Promoting and rolling back](rollout.md#promoting-and-rolling-back)). Read compliance, then widen.

## Sample element

`tools/sample-element/`, included from source: one operator, one converter reading `.Config`, one
optional template. `README.md` is [generated](#element-reference).

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

- `enabled: false`: a [values layer](values.md) turns the element on.
- `default.remediationAction: inform`: switch to `enforce` once compliance reads clean.
- `waitForOperator`: the template waits for the CSV to succeed and stays compliant meanwhile.
- `versions: []`: pin per environment in layers ([Operator upgrades](rollout.md#operator-upgrades)).
- `alias: stack` puts values under `stack:` and turns on the library [schema](values.md#schema).
- `templates/policy.yaml` is one line, `include "policy-library.render"`. Leave it.

## What you leave out

| Key | Resolves to |
|---|---|
| `enabled` on a policy, template or operator | on |
| `policies` | one Policy per `converters/` directory, plus `install` when there are operators |
| `policies.<p>.templates` | one template per `converters/<p>/*.yaml` |
| `remediationAction` | `policies.<p>` → `default` → `inform` |
| `severity` | template or operator → `policies.<p>` → `default` → `low` |
| `categories`, `controls`, `standards` | `policies.<p>` → `default` → none |
| `complianceType` | `musthave` |
| `ignorePending` | `true` with `waitForOperator` |
| `subscription.source`, `sourceNamespace` | `redhat-operators`, `openshift-marketplace` |
| `upgradeApproval` | `Automatic` |
| `operatorGroup.name` | `subscription.name`, all namespaces |
| `displayName` | `subscription.name` |

!!! warning "Template `remediationAction` accepts only `InformOnly`"
    Every template takes its Policy's action. Mark readiness checks and `mustnothave` signals
    `InformOnly` so an `enforce` Policy never enforces them. `global.remediationAction: inform` still
    overrides every Policy.

## Converter shapes

A converter is a Helm template rendered with the chart root plus `.Config` (element `config` merged
with `templates.<t>.config`). The trimmed output picks the field:

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

=== "Documents"

    One `object-templates` entry per column-0 `---` document. Empty documents are dropped.

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

    !!! note
        A leading `---` and full-line `#` comments are fine. A `---` with text after it on the line
        (`--- # c`) is not a separator, so the document starts with `-` and goes raw.

=== "Raw list"

    Output starting with `-`, `[` or `{{` goes verbatim to `object-templates-raw`. Use it when the
    managed cluster decides the object count. It must be the template's only output.

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

Empty output renders `object-templates-raw: []`; a Policy with nothing else fails POLICY082.

## Helm or ACM: who evaluates what

| Data | Evaluated by | Write |
|---|---|---|
| `values.yaml`, layers, `.Config` | Helm, when Argo CD renders | `{{ .Config.x }}` |
| Objects on the managed cluster (`lookup`, `fromConfigMap`) | ACM on the managed cluster, every evaluation | `{{ "{{" }} lookup ... {{ "}}" }}` |
| Hub secrets and cluster labels (`copySecretData`, `.ManagedClusterLabels`) | ACM on the hub, when the Policy propagates | `{{ "{{" }}hub ... hub{{ "}}" }}` |

Escape only what ACM evaluates: Helm prints `{{ "{{" }}` as `{{`. Anything known from values belongs
to Helm. `make preview` shows both stages: the Policy after Helm, `.evaluated.yaml` after ACM.

## Partials and file reuse

Files and directories starting with `_` are never discovered. `templates.<t>.file` points a template
at one, or at a list of files whose objects are concatenated. From
[`stack/compliance`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/compliance), three
templates from one file, each with its own `config`:

```yaml
templates:
  high:
    file: findings/_findings
    config:
      severity: "compliance.openshift.io/check-severity=high"
  medium:
    file: findings/_findings
    severity: medium
    config:
      severity: "compliance.openshift.io/check-severity=medium"
```

## Optional pieces

Ship a policy, template or operator with `enabled: false`. A layer turns it on in one line,
`policies.install.templates.my-element-netpol.enabled: true`, and the README lists it under
*Optional pieces*. Give it its input defaults in `config`, or the enabled Policy may render empty.

## Naming

- Lowercase kebab-case, directory name equal to chart `name`. The values key is its camelCase.
- At most 17 characters: `pstack.install-<element>-<cluster>` must fit 63 with a 30-character
  cluster. Policy name plus element name: 24 at most ([Naming limits](policies.md#naming-limits)).
- Not another element's name plus `-`, nor a prefix of one: `gitops` on cluster `dev-c` and
  `gitops-dev` on cluster `c` share release `gitops-dev-c` (POLICY091). `lifecycle` is reserved.
- Prefix template files with the element (`my-element-instance`). Template names are unique across
  every Policy on a cluster (POLICY003).
- No `openshift-` prefix or `-operator` suffix.

!!! warning
    Element, policy and template names are part of every object name. Renaming one recreates its
    Policies on every cluster.

## Good practices

- **One product per element.** Clusters turn an element on as a unit.
- **Start in `inform`.** Roll out, read compliance, then `enforce`.
- **Compliance annotations only for controls the element implements.** There is no fallback.

!!! warning "`pruneObjectBehavior` decides whether removal deletes objects"
    Set it only on templates without dependencies (POLICY033), in its own commit, so it reaches the
    cluster before the removal
    ([What happens on the managed cluster](rollout.md#what-happens-on-the-managed-cluster)).

!!! danger "Keep the branch until the pin moves back"
    A fleet file pinned to a deleted branch freezes the Applications of every cluster
    ([Revision](applicationset.md#revision)).

## Secrets

Values live in Git. Read runtime secrets on the hub with a hub template. From
[`stack/acs/converters/secured/tls.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/stack/acs/converters/secured/tls.yaml):

```yaml
data: '{{ "{{" }}hub copySecretData "stackrox" "cluster-registration-secret" hub{{ "}}" }}'
```

`copySecretData` copies every key of `data`, not the type, labels or annotations. `fromSecret` reads
one key and returns `''` when it is missing.

Hub lookups stay in the policy namespace. To read another, point the Policy at a ServiceAccount in
the policy namespace that can read it:

```yaml
policies:
  secured:
    hubTemplateOptions:
      serviceAccountName: acs-hub-templates
```

POLICY081 evaluates escaped templates ([ACM templates](validation.md#acm-templates)).

## Element reference

`stack/<element>/README.md` is generated. Don't edit it.

```sh
pip install -r tools/requirements.txt
python tools/doc-generator.py          # all elements
python tools/doc-generator.py --check  # report stale ones, write nothing
```

| Source | Shown as |
|---|---|
| `Chart.yaml` `description` | Lead paragraph |
| `Chart.yaml` `home` | Guide link, for elements with a page under `docs/` |
| Comment above `config:` | Settings intro |
| Comment above a key under `config` | That row's description |
| `description:` of a policy, template or operator | Policy line or template row |
| `enabled: false` entries | *Optional pieces* table |

A blank line between a comment and its key keeps the comment out, for maintainer notes. The
`element-readmes` hook regenerates READMEs and fails CI when one is stale ([Hooks](workflows.md#hooks)).
