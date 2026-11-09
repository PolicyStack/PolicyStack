# Fleet files

A fleet file, `fleet/<cluster>.yaml`, enrolls one cluster: while it exists on the default branch, the hub creates that cluster's Applications, and its keys pick the Git `revision` they render and the values layers they read. What that means for delivery (why the file is read at `HEAD` while everything else is read at `revision`, how long a merge takes, what one broken file stops) is in [How PolicyStack works](../concepts/overview.md#fleet-files).

Upstream ships no live fleet files; your repository adds them. Working examples are the validator fixtures in [`tools/validator/testdata/clusters/`](https://github.com/PolicyStack/PolicyStack/tree/main/tools/validator/testdata/clusters).

=== "Imported cluster"

    ```yaml
    # fleet/prod-east-1.yaml
    revision: main # (1)!
    channel: stable-4.20 # (2)!
    valueFiles: # (3)!
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/aws.yaml
    ```

    1.  Branch, tag or SHA. Every Application for this cluster renders `stack/`, `values.yaml` and `values/` from this commit ([Revision](#revision)).
    2.  Changes the cluster: the `upgrade` element, the one element enabled by default, enforces this as the cluster's ClusterVersion `spec.channel` as soon as the file merges ([OpenShift upgrades](../guides/upgrades.md)). Leave it out to keep the cluster's current channel.
    3.  Paths under `values/`, lowest precedence first ([Order](../concepts/values.md#order)). `values/clusters/prod-east-1.yaml` is always added last, so it is not listed. Any file under `values/` works: a custom layer such as `tenants/payments.yaml` is just a file you list.

=== "Hub-built cluster"

    ```yaml
    # fleet/hcp-kubevirt.yaml
    revision: main
    hub: acm-dc1 # (1)!
    version: 4.20.8 # (2)!
    valueFiles:
      - environments/nonprod.yaml
      - platforms/kubevirt.yaml # (3)!
    ```

    1.  Names the hub that builds this cluster by its `hubName`, the name the hub got at install (`--set hubName=acm-dc1` on the pointer) ([Cluster lifecycle](../concepts/lifecycle.md)). CI requires `fleet/hubs/acm-dc1.yaml` to exist, so a typo here can't leave the cluster with no hub to build or destroy it.
    2.  Required with `hub`: it is the release the hub installs.
    3.  Sets `install` (platform KubeVirt, two workers) for every cluster that lists it. A fleet file's own `install` is merged over it ([Install keys](lifecycle.md#install-keys)).

=== "Hub"

    ```yaml
    # fleet/hubs/acm-dc1.yaml
    revision: v1.4.0
    installRevision: v1.5.0 # (1)!
    valueFiles: # (2)!
      - environments/prod.yaml
      - datacenters/dc1.yaml
      - platforms/baremetal.yaml
      - acm/acm-dc1.yaml # (3)!
    ```

    1.  Optional. The revision of the hub's own install, `install/`, at least as new as `revision` ([installRevision](#installrevision)). Without it the install follows the default branch.
    2.  The hub is a cluster like any other: every element these layers enable, at the action they set, runs on the hub too.
    3.  Settings for the hub's own `acm` and `gitops` elements ([Hand the operators to PolicyStack](../start/install.md#hand-the-operators-to-policystack)). Listed last, so only `values/clusters/acm-dc1.yaml` overrides it. Hubs in one datacenter, such as an active hub and its DR hub, can list the same file.

## File name

`fleet/<cluster>.yaml`, where `<cluster>` is the ManagedCluster name exactly (`oc get managedclusters`). A `.yml` file, a different case or a typo enrolls nothing, and CI doesn't fail it. Keep cluster names within the [30-character budget](../concepts/policies.md#naming-limits), because Policy names are built from them. A hub's file is `fleet/hubs/<hubName>.yaml` ([Hub file](#hub-file)).

## Keys

Three ApplicationSets on the hub read fleet files ([ApplicationSets](../concepts/architecture.md)). `policystack` creates the element Applications that configure a running cluster and reads `revision`, `valueFiles`, `version` and `channel`. `policystack-lifecycle` builds and destroys clusters and reads every key, but only in files that set `hub`. `policystack-install`, the pointer, reads only the hub's own file and its `installRevision` ([Install chart](install.md#the-pointer)).

| Key | Required | Effect |
|---|---|---|
| `revision` | Yes | Branch, tag or SHA that every Application of the cluster renders from ([Revision](#revision)). |
| `valueFiles` | No | Values layers, as paths relative to `values/` (`environments/prod.yaml`, not `values/environments/prod.yaml`), lowest precedence first. `values/clusters/<cluster>.yaml` is added after them automatically. An entry missing at the revision is skipped silently by Argo CD, which is why CI fails it (POLICY052). |
| `hub` | No | `hubName` of the hub that builds the cluster. Without it, import the cluster yourself. Removing it, with `state` and `install`, abandons a built cluster ([Abandon](../guides/lifecycle.md#abandon)). |
| `state` | No | `present` (default) or `absent`. `absent` destroys the cluster ([Destroy](../guides/lifecycle.md#destroy)). Requires `hub`. |
| `install` | No | What to build: engine, platform, nodes ([Install keys](lifecycle.md#install-keys)). Values layers can set `install` too, for keys shared by many clusters; the fleet file's `install` is applied above all of them. Elements ignore it. Requires `hub`. |
| `version` | With `hub` | OpenShift release, `x.y.z`. With `hub` it picks the release the hub installs; leaving it out fails the `lifecycle-<cluster>` render with `version is required`. After install, and on every imported cluster or hub, a release newer than the running one starts an upgrade: the hub upgrades HyperShift-built clusters, the `upgrade` element every other cluster, including those built by `hive` or `siteconfig` ([OpenShift upgrades](../guides/upgrades.md)). |
| `channel` | No | Update channel. On HyperShift-built clusters the hub sets HostedCluster `spec.channel`; everywhere else the `upgrade` element enforces ClusterVersion `spec.channel`. Unset, the cluster's channel is left alone. |
| `installRevision` | No, hub files only | Branch, tag or SHA of the hub's install, `install/` ([installRevision](#installrevision)). Unset, the default branch. |

`version` and `channel` are accepted only in fleet files; POLICY034 rejects them in a values layer. That way the clusters a merge upgrades are exactly the fleet files it changes, each one listed in the PR's [Render Diff](ci.md#render-diff).

`hubName` exists because every hub's own ManagedCluster is named `local-cluster` by default (the MultiClusterHub can set `localClusterName`), so `fleet/local-cluster.yaml` could not tell hubs apart ([The hub](../concepts/architecture.md#the-hub)).

### What CI rejects

POLICY050 checks every fleet file before merge, in your repository's CI and in `make -C tools/validator validate` ([Validator rules](rules.md)). It fails on:

- a YAML error, an unknown key, or a missing or empty `revision`;
- an unquoted value Argo CD would read as a number or boolean ([Quote numbers and booleans](#quote-numbers-and-booleans));
- a `valueFiles` entry that is null, listed twice, or not a file under `values/`;
- a `version` that is not `x.y.z`, or `install.version` (use the top-level `version`);
- an empty `hub`, `version` or `channel`;
- a `hub` with no `fleet/hubs/<hub>.yaml`, or a `hub` that names the cluster itself;
- `state` other than `present` or `absent`, or an `install` that is not a map;
- `state` or `install` without `hub`;
- `hub`, `state` or `install` in a hub file, or `installRevision` in any other file;
- a cluster with `hub` whose name is not a DNS label, since the hub names the HostedCluster after it.

POLICY052 then checks `revision` and `installRevision` themselves ([Revision](#revision), [installRevision](#installrevision)).

### Quote numbers and booleans

!!! warning
    Argo CD reads fleet files as YAML 1.1 into untyped values, then templates them as text. `revision: 1.10` becomes the number `1.1`, `revision: 20261003` becomes `2.0261003e+07`, `revision: on` becomes `true`. Each names a ref that doesn't exist, which stops Application generation ([Revision](#revision)). Write `"1.10"`, `"20261003"`, `"on"`. The same applies to `version`: `version: 4.20` is the number `4.2`. POLICY050 catches every case.

## Revision

`revision` is a branch (slashes work, `release/2026-11`), a tag, `HEAD` (the default branch), or a commit SHA: all 40 characters, or at least the first 7, in lowercase. Argo CD tries a full SHA first, then a branch or tag of that name, then an abbreviated SHA. If a branch and a tag share a name, which one Argo CD picks is undefined, so never reuse a name. A branch follows every push to it; a tag or SHA changes only when you edit `revision`. Which to use and how to promote between them: [Rolling out changes](../guides/rollout.md#choosing-a-pin).

POLICY052 fails a PR when the revision doesn't resolve, or when its commit lacks what the file uses:

```yaml title="fleet/prod-east-1.yaml"
revision: v1.4.0  # (1)!
hub: acm-dc1      # (2)!
version: 4.20.8   # (3)!
```

1.  `v1.4.0` must contain `stack/` and every `valueFiles` entry. A typo, or a branch such as `gh-pages`, fails.
2.  The commit must contain `lifecycle/`, the chart that builds clusters. CI checks for `lifecycle/converters/_hypershift/`, present in every revision that can build clusters, whatever the engine.
3.  The commit must contain the `upgrade` element (`stack/upgrade`). It ships enabled. If a layer sets `stack.upgrade.enabled: false`, `version` and `channel` do nothing on any cluster except HyperShift-built ones, whose upgrades the hub runs.

Two rules read oddly at first:

- **A pin to the PR's base branch is checked against the merge commit.** In a PR to `main`, `revision: main` resolves to what `main` will be after the merge. A PR that adds both a fleet file and the values it lists therefore passes.
- **A `values/clusters/<name>.yaml` that no `fleet/<name>.yaml` or `fleet/hubs/<name>.yaml` reads is a warning, not an error.** For a tag-pinned cluster, `values/clusters/<name>.yaml` is merged and tagged before the fleet PR that pins that tag, so for a while no fleet file reads it ([First cluster](../start/first-cluster.md)). Once your repository has its first fleet file, every file in `values/clusters/` that no fleet file reads shows this warning. Delete those you don't use.

### installRevision

A hub file's `installRevision` is the revision at which the hub's own Argo CD syncs `install/`: the pointer, the two ApplicationSets that generate every Application, and their supporting objects ([Install chart](install.md)). Unset, the hub follows the default branch, and a framework update reaches it as soon as it merges. Pin a tag to upgrade hubs one at a time:

```yaml title="fleet/hubs/acm-lab.yaml"
revision: v1.4.0 # (1)!
installRevision: v1.5.0 # (2)!
```

1.  A hub that pins its install pins its own Day-2 too: `revision: main` would take each framework update before the install does, and POLICY052 fails the update PR.
2.  Move it to the next tag on a lab hub first, then on the others. Remove it to follow the default branch again.

It is separate from `revision` because the install serves every cluster on the hub, not just the hub itself. A hub renders each cluster's Applications with its own install, whatever `revision` that cluster pins, so the install must be at least as new as every revision the hub renders ([Hub and revision skew](compatibility.md#hub-and-revision-skew)). A hub with a tag `installRevision` can therefore only render clusters pinned to that tag or an older one. POLICY052 checks it where Git says which hub renders a cluster:

| Fleet file | Rendered by | A `revision` newer than that hub's install |
|---|---|---|
| `fleet/hubs/<child>.yaml` of a hub another hub builds, so its `fleet/<child>.yaml` sets `hub: <parent>` | The parent, its Global Hub | Error |
| Any other `fleet/hubs/<hub>.yaml` | That hub, or its Global Hub if imported in default mode | Warning |
| `fleet/<cluster>.yaml` with `hub:` | The hub it names, and that hub's parent when the parent builds it | Error |
| `fleet/<cluster>.yaml` without `hub:` | Whichever hub imported it | Warning, against the oldest hub install |

It also fails an `installRevision` that doesn't resolve, or whose commit has no `install/templates/install.yaml`: a release before the pointer existed would leave the hub's install no longer following Git. One whose commit records no framework release only warns, and its clusters go unchecked.

### When a revision breaks

CI checks the pin only at merge. A branch deleted later, including a PR branch GitHub deletes on merge, stops Application generation for the whole hub ([One bad fleet file stops the hub](../concepts/overview.md#one-bad-fleet-file-stops-the-hub)). Pin tags, SHAs or protected branches.

```sh
# the generation error, if any
oc get applicationset policystack policystack-lifecycle -n openshift-gitops \
  -o jsonpath='{range .items[*]}{.metadata.name}: {.status.conditions[?(@.type=="ErrorOccurred")].message}{"\n"}{end}'
```

## Hub file

A hub reads `fleet/hubs/<hubName>.yaml`, never `fleet/<cluster>.yaml`. It may not set `hub`, `state` or `install`: a hub cannot build itself, and `policystack-lifecycle` never reads `fleet/hubs/`. The file must exist before the hub's install: the pointer reads it, and deleting it freezes the hub's install ([Install chart](install.md#the-pointer)).

A managed hub imported into a Global Hub in default mode has no `local-cluster` of its own. The global hub reads `fleet/hubs/<cluster>.yaml` for it, where `<cluster>` is the managed hub's ManagedCluster name on the global hub: it renders the managed hub's Day-2 from it and installs PolicyStack on it, whose pointer reads the same file ([Global Hub](../guides/global-hub.md#layout)).

Files in any other subdirectory, such as `fleet/prod/edge-1.yaml`, are read by nothing, and CI doesn't flag them.

## Labels

PolicyStack reads five labels and ignores every other. The ApplicationSets read them from the cluster's Argo CD Secret in `openshift-gitops`, which ACM's GitOpsCluster creates from the ManagedCluster; the Placements read `name` and `deploy-mode` from the ManagedCluster itself. You only ever set the last one.

| Label | Set by | Effect |
|---|---|---|
| `clusterID` | ACM | Selects the cluster's Argo CD Secret. A cluster without it gets no Applications. |
| `local-cluster` | ACM, on the hub | Switches to the hub file ([Hub file](#hub-file)) and uses `hubName` instead of the cluster name for the Application names and the `values/clusters/` file. The Placement still selects the ManagedCluster by its real name. |
| `name` | ACM | The Placement selects the cluster by it ([Placement](../concepts/policies.md#placement)). |
| `apps.open-cluster-management.io/cluster-name` | ACM GitOpsCluster, on the Secret only | Matches `hub` to a managed hub imported into a Global Hub in default mode ([Global Hub](../guides/global-hub.md#lifecycle)). |
| `global-hub.open-cluster-management.io/deploy-mode` | You, when importing a managed hub into a Global Hub, or `install.labels` on a hub the global hub builds | `default` switches that hub to `fleet/hubs/<cluster>.yaml` and installs PolicyStack on it ([Global Hub](../guides/global-hub.md)). |

PolicyStack installs no RBAC around these labels. Anyone who can label ManagedClusters or edit Secrets in `openshift-gitops` can change what a cluster runs, for example by adding `local-cluster` to a spoke so it reads the hub's file. Keep those rights to hub admins. Who may change `fleet/` itself is set in your repository ([Protect the repository](../start/install.md#protect-the-repository)).
