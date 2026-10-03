# Writing an element

An element is a Helm chart under `stack/` that renders ACM policies through the
[policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md)
chart.

Name it in lowercase kebab-case and keep the directory name and chart `name` identical. The
Application, and so the Helm release, takes the directory name
([Applications](applicationset.md#applications)). The values key comes from the chart name.
`element:` dependencies and the validator assume the two match.

## Creating an element

From the repo root:

```sh
./tools/create-element.sh
```

The script prompts for a name and a description, then:

1. Copies `sample-element/` to `stack/<name>/`. If that directory exists and you confirm the
   overwrite, it is deleted first.
2. Sets `name` and `description` in `Chart.yaml`.
3. Renames the sample's `stack.myTest` key to the camelCase name. `security-baseline` reads
   `stack.securityBaseline`.

Then finish it by hand:

1. Delete `charts/` and `Chart.lock` from the new directory if they exist. They are gitignored, but
   the script copies whatever your checkout of `sample-element/` holds, and the validator renders
   with an existing archive instead of fetching the pinned library version.
2. Cut `values.yaml` down. The sample shows every option of every policy type, keep what the
   element uses, and keep `enabled: false`.
3. Replace `converters/example.yaml` with one file per `templateNames` entry. The validator fails on
   a missing converter (POLICY020) and warns on an unused one (POLICY021).
4. Describe each setting with `# @desc:` and generate the README ([Element reference](#element-reference)).
5. Run the [validator](validation.md).
6. Turn the element on for one nonprod cluster in its `values/clusters/<cluster>.yaml`.

## Layout

| Path | Contents |
| ---- | -------- |
| `Chart.yaml` | Name, description and the `policy-library` dependency |
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
      - name: metallb
        enabled: true
        policyRef: install
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
- **Set `pruneObjectBehavior` before the first enforce.** It decides whether disabling a policy
  deletes what it created, and a later change must sync before the element is disabled
  ([Disabling an element](rollout.md#disabling-an-element)).
- **Pin operator versions** where an unplanned upgrade would hurt
  ([Operator upgrades](rollout.md#operator-upgrades)).
- **Keep secrets out of values.** Values live in Git. Read runtime secrets on the hub with an ACM
  hub template, escaped so Helm passes it through:
  `'{{ "{{" }}hub fromSecret "<namespace>" "<secret>" "<key>" hub{{ "}}" }}'`
  ([example](https://github.com/PolicyStack/PolicyStack/blob/main/stack/advanced-cluster-security/converters/acs-sync-collector.yaml)).
  Helm and the validator see escaped templates as plain strings, so a mistake shows up only on the
  cluster, as a `template-error` violation.
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
