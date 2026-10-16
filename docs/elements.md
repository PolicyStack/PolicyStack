# Writing an element

An element is a Helm chart under `stack/` that renders ACM policies through the
[policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md)
chart.

Name it in lowercase kebab-case and keep the directory name and chart `name` identical. The
Application, and so the Helm release, takes the directory name
([Applications](applicationset.md#applications)). The values key comes from the chart name.
`element:` dependencies and the validator assume the two match.

Keep the name short: use the product's common abbreviation (`acm`, `odf`) or a plain noun
(`logging`), without an `openshift-` prefix or `-operator` suffix. The name must not start with
another element's name followed by `-`, since `gitops` on cluster `dev-c` and `gitops-dev` on
cluster `c` would both be release `gitops-dev-c` (POLICY091). `lifecycle` is reserved. Element
and policy names are part of every object name, so renaming one recreates its Policies on every
cluster.

## Creating an element

From the repo root:

```sh
./tools/create-element.sh <name> "<description>"
```

It prompts for whatever is not given, then copies
[`tools/sample-element/`](https://github.com/PolicyStack/PolicyStack/tree/main/tools/sample-element) to
`stack/<name>/`, replacing `my-element` with the name and `myElement` with its camelCase form
(`cert-manager` reads `stack.certManager`). It refuses a name that is not lowercase kebab-case or
already exists. The result passes the validator as is.

The sample is the [metallb](#conventions) pattern cut to one of each piece: an operator under an
`install` policy, a `config` policy that depends on it, a toggle, and one converter that reads
`config`. Policies start in `inform`. Then:

1. Set the operator's `namespace`, `operatorGroup` and `subscription` in `values.yaml`.
2. Replace `converters/<name>-configmap.yaml` with one file per `templateNames` entry. The validator
   fails on a missing converter (POLICY020) and warns on an unused one (POLICY021).
3. Describe each setting with `# @desc:` and generate the README ([Element reference](#element-reference)).
4. Run the [validator](validation.md).
5. Turn the element on for one nonprod cluster in its `values/clusters/<cluster>.yaml`.

Every option of every policy type is in the
[policy-library README](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md).

## Layout

| Path | Contents |
| ---- | -------- |
| `Chart.yaml` | Name, description and the `policy-library` dependency, aliased `stack` so its [schema](values.md#schema) checks the values |
| `templates/policy.yaml` | One line: `{{- include "policy-library.render" . -}}` |
| `values.yaml` | Defaults for every setting, under the key described in [Element values](policies.md#element-values) |
| `converters/<name>.yaml` | The manifest for one `configPolicies[].templateNames` entry, rendered with Helm's `tpl`. It reads `.Values`, and `.Parameters` (that config policy's `templateParameters`) when the config policy sets `enableTemplateParameters` |
| `README.md` | Generated [element reference](#element-reference) |
| `.gitignore` | Keeps `Chart.lock`, `charts/` and `*.tgz` out of Git |

## Conventions

The examples come from [`stack/metallb`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/metallb).
Its `values.yaml`, trimmed:

```yaml
stack:
  metallb:
    enabled: false
    toggles:
      quota: false
      addressing: false
    config:
      ipAddressPools: {}
      l2Advertisements: {}
      # bgpAdvertisements, bgpPeers, quota

    policies:
      - name: install
        enabled: true
      - name: config
        enabled: true
        dependencies:
          - name: install
      # quota
      - name: addressing
        enabled: true
        dependencies:
          - name: config

    operatorPolicies:
      metallb:
        enabled: true
        policyRef: install
        upgradeApproval: Automatic
        versions: []
        # namespace, operatorGroup, subscription

    configPolicies:
      - name: metallb-instance
        enabled: true
        policyRef: config
        waitForOperator: metallb
        ignorePending: true
        templateNames:
          - name: metallb-cr
      # resourcequota
      - name: pools
        enabled: true
        policyRef: addressing
        rawTemplate: true
        templateNames:
          - name: metallb-ipaddresspools
      # l2, bgp, peers
```

**Off by default.** The element ships with `enabled: false`. A layer in `values/` turns it on for the
clusters that need it ([Values cascade](values.md)).

**Toggles for optional pieces.** The `quota` and `addressing` policies ship toggled off, so a
cluster turns one on in one line instead of restating the `policies` list ([toggles](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#toggles)).

**A `config` map for site data.** Converters read addresses, peers and quotas from
`.Values.stack.metallb.config` rather than hard-coding them. Maps merge across the cascade
([Merge rules](values.md#merge-rules)), so a cluster file sets only its own keys.
`values/clusters/nonprod-west-1.yaml`:

```yaml
stack:
  metallb:
    enabled: true
    toggles:
      addressing: true
    config:
      ipAddressPools:
        services:
          addresses:
            - 10.20.30.100-10.20.30.120
          autoAssign: true
      l2Advertisements:
        services:
          ipAddressPools:
            - services
```

**Install, then configure.** The operator and the objects that need its CRDs live in separate
policies. `config` depends on `install`, and `addressing` depends on `config`, so ACM holds each
policy `Pending` until the one before it is compliant. `waitForOperator: metallb` makes
`metallb-instance` also wait for the operator's CSV, and `ignorePending: true` keeps that template's
`Pending` state from making its policy noncompliant
([policy dependencies](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#policy-dependencies),
[waiting for an operator](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#waiting-for-an-operator)).

**`rawTemplate` when the object count varies.** The pool, advertisement and peer converters range
over a `config` map and emit one object per key, so they set `rawTemplate: true`. An empty map
renders an empty list
([raw object templates](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#raw-object-templates)).

**Short names.** Element names, policy names and the names of their config and operator policies
end up in length-limited object names ([Naming limits](policies.md#naming-limits)).

## Good practices

- **One product per element.** Clusters turn an element on as a unit, and its name is in every
  object name. An operator and its configuration belong together, unrelated settings do not.
- **Put the element name in config and operator policy names.** ACM requires template names to be
  unique across every Policy on a cluster, so two elements that both define an `install` policy
  with a `ns-monitoring` config policy collide. metallb uses `metallb-instance`. The validator
  reports collisions as POLICY003.
- **Start in `inform`.** Ship a new policy with `remediationAction: inform`, roll it out to a
  nonprod revision, read its compliance, then switch to `enforce`.
- **Set `pruneObjectBehavior` only on templates without dependencies, in its own commit.** It
  decides whether removing a policy deletes what it created, and it must reach the cluster before
  the removal. The validator rejects it on a template with dependencies as POLICY033
  ([What happens on the managed cluster](rollout.md#what-happens-on-the-managed-cluster)).
- **Pin operator versions in layers, not in the element.** Ship `versions: []` and set it per
  environment or cluster where an unplanned upgrade would hurt
  ([Operator upgrades](rollout.md#operator-upgrades)).
- **Keep secrets out of values.** Values live in Git. Read runtime secrets on the hub with an ACM
  hub template, escaped so Helm passes it through:
  `'{{ "{{" }}hub fromSecret "<namespace>" "<secret>" "<key>" hub{{ "}}" }}'`
  ([example](https://github.com/PolicyStack/PolicyStack/blob/main/stack/acs/converters/acs-sync-collector.yaml)).
  Hub lookups are limited to the policy namespace. To read another, set
  `policies[].hubTemplateOptions.serviceAccountName` to a ServiceAccount in the policy namespace
  that can read it, as acs does. The validator evaluates escaped templates
  as POLICY081 ([ACM templates](validation.md#acm-templates)).
- **Try changes on one cluster first.** Push a branch, then pin a nonprod cluster's fleet file to it
  with a pull request to the default branch
  ([Promoting and rolling back](rollout.md#promoting-and-rolling-back)). Keep the branch until the
  pin moves back. A pin to a deleted branch freezes the Applications of every cluster
  ([Revision](applicationset.md#revision)).

## Element reference

`stack/<element>/README.md` is generated from the element's `values.yaml` and its `@description` and
`@desc` comments, do not edit it by hand. After changing `values.yaml`, run
`python tools/doc-generator.py` and commit the README, `--check` reports stale READMEs without
writing them. Flags and comment syntax are in
[tools/README.md](https://github.com/PolicyStack/PolicyStack/blob/main/tools/README.md#doc-generatorpy).

CI runs the check on pull requests, and the `update-docs` label regenerates the READMEs on the pull
request branch ([Documentation Check](workflows.md#documentation-check)).
