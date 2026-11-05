# Fleet files

One file per cluster: `fleet/<cluster>.yaml`, named exactly like its ManagedCluster. The hub uses `fleet/hubs/<hubName>.yaml` ([Hub file](#hub-file)).

=== "Imported cluster"

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main # (1)!
    channel: stable-4.20
    valueFiles: # (2)!
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/aws.yaml
    ```

    1.  Branch, tag or SHA. Every Application for this cluster renders from it.
    2.  Paths under `values/`, lowest precedence first ([Order](../concepts/values.md#order)). `values/clusters/prod-east-1.yaml` is always added last. Any file works, so a custom layer like `tenants/payments.yaml` is just a file you list.

=== "Hub-built cluster"

    ```yaml
    # fleet/hcp-kubevirt.yaml
    revision: main
    hub: acm-dc1 # (1)!
    version: 4.20.8
    valueFiles:
      - environments/nonprod.yaml
      - platforms/kubevirt.yaml
    ```

    1.  Needs `fleet/hubs/acm-dc1.yaml` to exist.

=== "Hub"

    ```yaml
    # fleet/hubs/acm-dc1.yaml
    revision: main
    valueFiles:
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/baremetal.yaml
      - acm/acm-dc1.yaml # (1)!
    ```

    1.  Hubs in one datacenter share a layer by listing it.

## Keys

| Key | Required | Effect |
|---|---|---|
| `revision` | Yes | Branch, tag or SHA every Application renders from ([Revision](#revision)). |
| `valueFiles` | No | Values layers, lowest precedence first. |
| `hub` | No | `hubName` of the hub that builds the cluster. Without it the cluster must be imported. |
| `state` | No | `present` (default) or `absent`, which destroys the cluster ([Destroy](../guides/lifecycle.md#destroy)). Requires `hub`. |
| `install` | No | The cluster to build ([Install keys](lifecycle.md#install-keys)). Requires `hub`. |
| `version` | With `hub` | OpenShift release, `x.y.z`. Bumping it upgrades ([OpenShift upgrades](../guides/upgrades.md)). |
| `channel` | No | Upgrade channel. |

The [validator](../develop/validation.md) enforces: no other keys, every `valueFiles` entry exists, and a hub file sets no `hub`, `state` or `install`. `version` and `channel` go only in fleet files ([OpenShift upgrades](../guides/upgrades.md)). `policystack` ignores the lifecycle keys.

!!! warning "Quote numbers and booleans"
    Argo CD reads fleet files as YAML 1.1. `revision: 1.10` pins `1.1`. Write `"1.10"`, `"20261003"`, `"on"`.

Fleet files are read only from the default branch (`HEAD`). `fleet/` on other branches does nothing. Changing the default branch re-points every cluster, and a cluster with no file there loses its Applications.

Changes land within about 6 minutes (3 minute generator requeue plus 3 minute branch cache). An [ApplicationSet webhook](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Git/#webhook-configuration) makes them immediate, except a fix for a failed generation, which waits for the 3 minute retry.

Upstream ships no live fleet files. Examples are the validator fixtures in [`tools/validator/testdata/clusters/`](https://github.com/PolicyStack/PolicyStack/tree/main/tools/validator/testdata/clusters).

## Revision

A branch (slashes work), tag or commit SHA. The [Render Diff](ci.md#render-diff) PR comment shows each cluster's Policies at its `revision` before and after merge.

CI checks each pin ([POLICY052](rules.md)): the `revision` must be a tag, branch or commit SHA (5 to 40 hex characters) whose commit contains what the file uses, including every `valueFiles` entry. A pin to the PR's base branch is checked against the merge commit. A `values/clusters/<name>.yaml` with no `fleet/<name>.yaml` or `fleet/hubs/<name>.yaml` is a warning, since a tag-pinned cluster's file lands first ([First cluster](../start/first-cluster.md)).

```yaml title="fleet/prod-east-1.yaml"
revision: v1.4.0  # (1)!
hub: acm-dc1      # (2)!
version: 4.20.8   # (3)!
```

1.  `v1.4.0` must contain `stack/` and each `valueFiles` entry. A typo or a branch like `gh-pages` fails.
2.  Also needs `lifecycle/converters/_hypershift/`.
3.  `version` or `channel` also needs an `upgrade` element that reads them.

!!! danger "One bad fleet file freezes this hub's fleet"
    If a fleet file this hub reads fails to parse, lacks `revision`, or pins a missing revision, `policystack` generation fails and no Application for any cluster is created, updated or deleted until it is fixed. A parse error stops `policystack-lifecycle` the same way. A bad `revision` there breaks only `lifecycle-<cluster>`.

```sh
oc get applicationset policystack -n openshift-gitops \
  -o jsonpath='{.status.conditions[?(@.type=="ErrorOccurred")].message}'
```

Pin tags, SHAs or protected branches. CI does not catch a pinned branch deleted after merge, and GitHub deletes PR branches on merge when automatic head branch deletion is on.

## Hub file

The hub reads `fleet/hubs/<hubName>.yaml`, never `fleet/<cluster>.yaml` ([The hub](../concepts/architecture.md#the-hub)). A managed hub imported into a Global Hub in default mode reads `fleet/hubs/<cluster>.yaml` ([Global Hub](../guides/global-hub.md#layout)). `hubs/` and other subdirectories are never lifecycle fleet files.

## Labels

PolicyStack reads five labels. You only ever set the last one.

| Label | Set by | Effect |
|---|---|---|
| `clusterID` | ACM | Selects the cluster's Argo CD secret, becomes `selectedId`. No label, no Applications. |
| `local-cluster` | ACM, on the hub | Switches to the hub file ([Hub file](#hub-file)). |
| `name` | ACM | Placement selector matches it ([Placement](../concepts/policies.md#placement)). |
| `apps.open-cluster-management.io/cluster-name` | ACM GitOpsCluster | Matches `hub` to a default-mode managed hub ([Lifecycle generator](../concepts/architecture.md#lifecycle-generator)). |
| `global-hub.open-cluster-management.io/deploy-mode` | You, on Global Hub import | `default` switches a managed hub to `fleet/hubs/<cluster>.yaml` ([Global Hub](../guides/global-hub.md)). |

## Protection

Other labels are ignored, and the five above are guarded by RBAC on ManagedClusters and `openshift-gitops` Secrets, so `oc label` cannot change what a cluster runs. Whoever merges to `fleet/` decides, including destroying clusters with `state: absent`.

Add a CODEOWNERS entry and a default branch ruleset requiring code owner review and blocking force push and deletion:

```text
# .github/CODEOWNERS
/fleet/ @<org>/<team>
```

To gate only production, own `/fleet/prod-*.yaml` and `/fleet/hubs/` instead.
