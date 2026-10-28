# Writing an element

An element is a Helm chart under `stack/` that renders ACM policies through the
[policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md)
chart. Every option of every policy type is in that README.

## Naming

- Lowercase kebab-case. Directory name and chart `name` must match: the Application and Helm
  release take the directory name ([Applications](applicationset.md#applications)), the values key
  takes the chart name, and `element:` dependencies and the validator assume both are equal.
- Short: the common abbreviation (`acm`, `odf`) or a plain noun (`logging`). No `openshift-`
  prefix, no `-operator` suffix ([Naming limits](policies.md#naming-limits)).
- Must not start with another element's name plus `-`. `gitops` on cluster `dev-c` and
  `gitops-dev` on cluster `c` would both be release `gitops-dev-c` (POLICY091).
- `lifecycle` is reserved.

!!! warning
    Element and policy names are part of every object name. Renaming one recreates its Policies on
    every cluster.

## Creating an element

```sh
./tools/create-element.sh cert-manager "Certificate management"
# Created stack/cert-manager, values key stack.certManager. ...
```

It prompts for missing arguments, refuses a name that is not kebab-case or already exists, and
copies [`tools/sample-element/`](https://github.com/PolicyStack/PolicyStack/tree/main/tools/sample-element)
to `stack/<name>/`, replacing `my-element` with the name and `myElement` with its camelCase form.
The result passes the validator as is. Then:

1. Set the operator's `namespace`, `operatorGroup` and `subscription` in `values.yaml`.
2. Replace `converters/<name>-configmap.yaml` with one file per `templateNames` entry. A missing
   converter fails (POLICY020), an unused one warns (POLICY021).
3. Describe each setting with `# @desc:` and [generate the README](#element-reference).
4. Run the [validator](validation.md).
5. Turn it on for one nonprod cluster in `values/clusters/<cluster>.yaml`.

## Layout

The files of `tools/sample-element/`. It has one of each piece: an operator under an `install`
policy, a `config` policy that depends on it, a toggle, and one converter.

=== "Chart.yaml"

    ```yaml
    apiVersion: v2
    name: my-element # (1)!
    description: Example element. tools/create-element.sh copies it to stack/<name>.
    type: application
    version: 0.1.0
    dependencies:
      - name: policy-library
        alias: stack # (2)!
        version: "3.4.0"
        repository: "https://policystack.github.io/PolicyStack-chart"
    ```

    1.  Same as the directory name.
    2.  The alias puts values under `stack:` and lets the library [schema](values.md#schema) check them.

=== "values.yaml"

    ```yaml
    stack:
      # @description: Example element. tools/create-element.sh copies it to stack/<name>.
      myElement: # (1)!
        # @desc: Master control to enable/disable all policies in this element
        enabled: false # (2)!

        # @description: Optional pieces, switchable per cluster
        toggles:
          # @desc: Apply the ConfigMap configured below
          config: true # (3)!

        # @description: Settings the converters read. Maps merge across the values cascade.
        config:
          # @desc: ConfigMap data, e.g. {logLevel: info}
          data: {} # (4)!

        # @description: Default policy metadata applied unless overridden per-policy
        default:
          categories:
            - CM Configuration Management
          controls:
            - CM-2 Baseline Configuration
          standards:
            - NIST SP 800-53

        policies:
          - name: install
            enabled: true
            description: "Install the operator"
            remediationAction: inform # (5)!
            severity: medium
            disabled: false

          - name: config
            enabled: true
            description: "Configure the operator"
            remediationAction: inform
            severity: medium
            disabled: false
            # @desc: ACM holds this policy Pending until install is compliant
            dependencies:
              - name: install

        operatorPolicies:
          my-element:
            enabled: true
            description: "Example operator"
            displayName: "Example Operator"
            policyRef: install # (6)!
            severity: medium
            remediationAction: inform
            complianceType: musthave
            namespace: my-element
            operatorGroup:
              name: my-element
            subscription:
              channel: stable
              name: my-element-operator
              source: redhat-operators
              sourceNamespace: openshift-marketplace
            upgradeApproval: Automatic
            # @desc: Allowed CSV names; empty allows whatever the channel offers
            versions: [] # (7)!

        configPolicies:
          - name: my-element-config # (8)!
            enabled: true
            description: "Example ConfigMap"
            policyRef: config
            complianceType: musthave
            remediationAction: inform
            severity: medium
            # @desc: Wait for the operator's CSV, and stay compliant while waiting
            waitForOperator: my-element
            ignorePending: true
            templateNames:
              - name: my-element-configmap # (9)!
    ```

    1.  camelCase of the chart name ([Element values](policies.md#element-values)).
    2.  Off by default. A layer in `values/` turns it on.
    3.  Overrides `enabled` of the entry named `config`.
    4.  Site data. Converters read it, layers set it.
    5.  New policies start in `inform`.
    6.  The policy this operator renders into.
    7.  Pin versions in layers, not here ([Operator upgrades](rollout.md#operator-upgrades)).
    8.  Prefixed with the element name: template names must be unique across all Policies on a cluster.
    9.  Renders `converters/my-element-configmap.yaml`.

=== "converters/my-element-configmap.yaml"

    ```yaml
    {{- /* One object per converter. Site data comes from config, not hard-coded values. */ -}}
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-element
      namespace: my-element
    data:
      {{- toYaml .Values.stack.myElement.config.data | nindent 2 }}
    ```

=== "templates/policy.yaml"

    ```yaml
    {{- include "policy-library.render" . -}}
    ```

=== ".gitignore"

    ```
    Chart.lock
    *.tgz
    charts/
    ```

A converter is the manifest for one `configPolicies[].templateNames` entry, rendered with Helm's
`tpl`. It reads `.Values`, and `.Parameters` (that config policy's `templateParameters`) when the
config policy sets `enableTemplateParameters: true`. `README.md` is
[generated](#element-reference).

## Conventions

From [`stack/metallb`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/metallb):

| Convention | metallb |
| ---------- | ------- |
| Off by default | `enabled: false`; a [values layer](values.md) turns it on |
| [Toggles](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles) for optional pieces | `toggles.quota` and `toggles.addressing` ship `false`; a cluster flips one in one line instead of restating the `policies` list |
| A `config` map for site data | Converters read `config.ipAddressPools`, `l2Advertisements`, `bgpPeers`, `quota`. Maps merge ([Merge rules](values.md#merge-rules)), so a cluster sets only its keys |
| Install, then configure | `config` depends on `install`, `addressing` on `config`. ACM holds each `Pending` until the previous one is compliant ([dependencies](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#policy-dependencies)) |
| Wait for the CSV | `metallb-instance` sets `waitForOperator: metallb` and `ignorePending: true`, so it waits for the operator without going noncompliant ([waiting for an operator](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#waiting-for-an-operator)) |
| `rawTemplate` when the object count varies | Pool, advertisement and peer converters emit one object per map key. An empty map renders an empty list ([raw object templates](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#raw-object-templates)) |
| Short names | Element, policy, config and operator policy names end up in length-limited object names ([Naming limits](policies.md#naming-limits)) |

A cluster turning it on, `values/clusters/nonprod-west-1.yaml`:

```yaml
stack:
  metallb:
    enabled: true
    toggles:
      addressing: true
    config:
      ipAddressPools:
        services: # (1)!
          addresses:
            - 10.20.30.100-10.20.30.120
          autoAssign: true
      l2Advertisements:
        services:
          ipAddressPools:
            - services
```

1.  Map key is the object name, value is the `IPAddressPool` spec.

The `rawTemplate` converter that reads it, `converters/metallb-ipaddresspools.yaml`:

```yaml
{{- range $name, $spec := .Values.stack.metallb.config.ipAddressPools }}
- complianceType: mustonlyhave
  objectDefinition:
    apiVersion: metallb.io/v1beta1
    kind: IPAddressPool
    metadata:
      name: {{ $name }}
      namespace: {{ $.Parameters.namespace }} # (1)!
    spec:{{ toYaml $spec | nindent 6 }}
{{- end }}
```

1.  From the `pools` config policy's `templateParameters`.

## Good practices

- **One product per element.** Clusters turn an element on as a unit. An operator and its
  configuration belong together, unrelated settings do not.
- **Prefix config and operator policy names with the element.** Two elements that both put a
  `ns-monitoring` config policy under an `install` policy render the same template name
  `install-ns-monitoring` and collide on the cluster (POLICY003). metallb uses `metallb-instance`.
- **Start in `inform`.** Roll out to a nonprod revision, read compliance, then switch to `enforce`.
- **Pin operator versions in layers.** Ship `versions: []`, set it per environment or cluster
  ([Operator upgrades](rollout.md#operator-upgrades)).
- **Try changes on one cluster first.** Push a branch, then pin a nonprod cluster's fleet file to it
  with a pull request to the default branch ([Promoting and rolling back](rollout.md#promoting-and-rolling-back)).

!!! warning "Policy-level `remediationAction` overrides inform gates"
    ACM applies it to every template in the Policy but `InformOnly` ones, so `enforce` there
    enforces readiness checks and `mustnothave` signals. Set gates to `InformOnly` (config policies
    only), as lifecycle, upgrade and storage-nodes do, or leave the Policy's action unset and set it
    on each config and operator policy, as acs and cert-manager do.
    `global.remediationAction: inform` still overrides all of them.

!!! warning "`pruneObjectBehavior` decides whether removal deletes objects"
    Set it only on templates without dependencies (POLICY033), in its own commit, so it reaches
    the cluster before the removal
    ([What happens on the managed cluster](rollout.md#what-happens-on-the-managed-cluster)).

!!! danger "Keep the branch until the pin moves back"
    A fleet file pinned to a deleted branch freezes the Applications of every cluster
    ([Revision](applicationset.md#revision)).

### Secrets

Values live in Git. Read runtime secrets on the hub with an ACM hub template, escaped so Helm
passes it through. From
[`stack/acs/converters/acs-sync-crs.yaml`](https://github.com/PolicyStack/PolicyStack/blob/main/stack/acs/converters/acs-sync-crs.yaml):

```yaml
data: '{{ "{{" }}hub copySecretData "{{ .Parameters.hubNamespace }}" "cluster-registration-secret" hub{{ "}}" }}'
```

`copySecretData` copies every key of `data`, not the type, labels or annotations. `fromSecret` reads one key and returns `''` when it is missing.

Hub lookups are limited to the policy namespace. To read another, point the policy at a
ServiceAccount in the policy namespace that can read it, as acs does:

```yaml
policies:
  - name: secured # (1)!
    hubTemplateOptions:
      serviceAccountName: acs-hub-templates
```

1.  Trimmed; see `stack/acs/values.yaml`.

The validator evaluates escaped templates as POLICY081 ([ACM templates](validation.md#acm-templates)).

## Element reference

`stack/<element>/README.md` is generated from `values.yaml` and its `@description` / `@desc`
comments. Do not edit it by hand.

```sh
pip install -r tools/requirements.txt
python tools/doc-generator.py          # regenerate all READMEs
python tools/doc-generator.py --check  # report stale ones, write nothing
```

Commit the README with the `values.yaml` change. CI runs the check on pull requests, and the
`update-docs` label regenerates READMEs on the PR branch
([Documentation Check](workflows.md#documentation-check)). Flags and comment syntax:
[tools/README.md](https://github.com/PolicyStack/PolicyStack/blob/main/tools/README.md#doc-generatorpy).
