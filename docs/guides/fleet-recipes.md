# Fleet recipes

Enrolling a batch of clusters and moving a group of clusters to a new revision both edit many fleet files. Each fits in one ordinary fleet PR, and CI and Render Diff check every file in it. The single-cluster version is in [First cluster](../start/first-cluster.md), and the rules for pins are in [Rolling out changes](rollout.md).

The examples use four imported single-node clusters, `edge-01` to `edge-04`, onboarded at tag `v1.4.0` and later promoted to `v1.5.0`.

## Onboard many clusters in one PR

Each cluster needs its own fleet file, `fleet/<cluster>.yaml`, named exactly like its ManagedCluster. What the clusters have in common goes in shared layers that every fleet file lists. A `values/clusters/<cluster>.yaml` is needed only for a cluster that differs from the rest ([Layers](../concepts/values.md#layers)).

| Layer | Holds | Example |
|---|---|---|
| `environments/<env>.yaml` | What every cluster of an environment runs | `environments/prod.yaml` |
| `datacenters/<dc>.yaml` | Site settings: mirrors, DNS, base domain | `datacenters/dc1.yaml` |
| `platforms/<platform>.yaml` | The platform variant of the node elements | `platforms/baremetal.yaml` |
| `topologies/<topology>.yaml` | Facts about the node layout. Turns on no element ([Topology layers](../concepts/values.md#topology-layers)) | `topologies/single-node.yaml` |
| `clusters/<cluster>.yaml` | This cluster only. Read last, without being listed | `clusters/edge-03.yaml` |

Every directory except `clusters/` is a convention. List the layers lowest precedence first ([Order](../concepts/values.md#order)).

### Worked example

Four fleet files, identical except for the first line. It is a YAML comment that records the cluster's [ring](#promote-a-ring-in-one-pr): `# ring: canary` for `edge-01`, `# ring: ring-1` for the other three. PolicyStack reads no ring key, and POLICY050 fails an unknown key, so the ring lives in a comment.

```yaml
# fleet/edge-01.yaml
# ring: canary
revision: v1.4.0 # (1)!
valueFiles:
  - environments/prod.yaml # (2)!
  - topologies/single-node.yaml # (3)!
```

1.  The revision the ring already runs. Every layer listed here must exist at that commit (POLICY052).
2.  Your prod baseline. It turns elements on, and they enforce at once, so read it first or [dry run](#dry-run-the-first-merge) the batch.
3.  Sizing for one node. It turns nothing on, so listing it changes nothing on the cluster until a layer enables an element it configures.

Before you open the PR:

- Name each file after its ManagedCluster, at most 30 characters ([Naming limits](../concepts/policies.md#naming-limits)), ending in `.yaml`. A `.yml` file or a misspelled name enrolls nothing, and CI doesn't flag it ([File name](../reference/fleet.md#file-name)).
- Each `values/clusters/<cluster>.yaml` must exist at the revision the fleet file pins. With a branch pin, put it in the same PR. With a tag pin, merge it to `main` and tag before the fleet PR ([Land the cluster file before the fleet file](../start/first-cluster.md#onboard-the-cluster)).
- A layer that configures an element your repository doesn't have fails POLICY035 (`stack.<key> names no element`). `topologies/single-node.yaml` configures `image-registry`: `install` copies it with the example, but a layer you copy or write later needs `mise run ps -- add image-registry` first.

Then run `mise run validate` and open the PR.

### What CI checks

| Check | On each new fleet file |
|---|---|
| POLICY050 | Keys, quoting and `valueFiles` entries ([What CI rejects](../reference/fleet.md#what-ci-rejects)) |
| POLICY052 | The `revision` resolves and has `stack/` and every listed layer. `revision: main` is checked against the merge commit, so layers added in the same PR count ([Revision](../reference/fleet.md#revision)) |
| POLICY053 | A new file differs from the default branch, so when it pins a tag or SHA, CI also renders that commit with the new file ([Promotions](../reference/ci.md#promotions-policy053)) |
| POLICY035, RENDER000 | Every values key, and the render of every element for the cluster |
| POLICY001 | Every Policy name fits ACM's limit with this cluster name, for the elements its layers enable |

[Render Diff](../reference/ci.md#render-diff) adds one row per new cluster, `new at <revision>`, with the number of elements and objects it adds. A new cluster gets totals only, no per-element diff. Check the totals against what you expect: four clusters with the same layers and no cluster files should show the same totals.

### What happens on merge

Within about 6 minutes the `policystack` ApplicationSet picks up the new files ([Default branch only](../concepts/overview.md#default-branch-only)). Each cluster that is already imported and has its `clusterID` label gets one Application per element and its Policies. A file for a cluster that hasn't joined yet waits and takes effect when the cluster joins, so you can merge the batch before the last import finishes.

!!! danger "One bad file stops the whole hub"
    If any fleet file fails to parse, lacks `revision` or pins a revision that doesn't exist, the hub creates, updates and deletes no Application for any cluster until it is fixed ([One bad fleet file stops the hub](../concepts/overview.md#one-bad-fleet-file-stops-the-hub)). CI rejects all three before merge, so require `ci` in your ruleset ([Protect the repository](../start/install.md#protect-the-repository)).

### Dry run the first merge

To see what the batch would change before anything is enforced, keep a layer that holds only the dry-run key:

```yaml
# values/dry-run.yaml
global:
  remediationAction: inform
```

List it last in each new fleet file. Every Policy of those clusters then reports and changes nothing. Once the Policies have evaluated, a second fleet PR deletes the `- dry-run.yaml` line from each file, and enforcement starts. [Dry run](rollout.md#dry-run) lists what a dry run can't show, such as Policies that stay Pending on a new cluster.

A tag-pinned cluster reads the layer at its tag, and POLICY052 fails a fleet file that lists a layer its tag lacks. Commit `values/dry-run.yaml` once and tag that commit or a later one; every later tag has it.

## Promote a ring in one PR

A ring is a group of clusters that move to a new revision together, such as `canary`, `ring-1` and `ring-2`. PolicyStack has no ring object or key. A ring is a set of fleet files that pin the same revision, and you keep track of which files belong to it:

| Convention | Works with | Example |
|---|---|---|
| A comment on the first line | `grep`, so the one-liner below can pick the ring's files | `# ring: ring-1` |
| CODEOWNERS lines | GitHub review: the ring's owners must approve its promotion. CODEOWNERS can't read comments, so list each file | `/fleet/edge-02.yaml @acme/edge` |

Promote the canary first, in its own PR, and the next ring once the canary has run the new revision without problems.

### Steps

1.  Tag the commit you want the ring to run, and push the tag before you open the PR. CI resolves the pin from the tags it fetches. Protect `v*` tags from moves and deletion ([Choosing a pin](rollout.md#choosing-a-pin)).

    ```sh
    git tag v1.5.0 <sha>
    git push origin v1.5.0
    ```

2.  On a branch, change `revision` in every file of the ring, and only there:

    ```sh
    git switch -c promote-ring-1-v1.5.0
    grep -lx '# ring: ring-1' fleet/*.yaml \
      | xargs sed -E -i 's/^revision:[[:space:]]*[^#[:space:]]+/revision: "v1.5.0"/' # (1)!
    git diff --stat
    #  fleet/edge-02.yaml | 2 +-
    #  fleet/edge-03.yaml | 2 +-
    #  fleet/edge-04.yaml | 2 +-
    ```

    1.  `grep -lx` lists the files with that exact line. `sed` replaces only the value on the `revision:` line, so `valueFiles`, `version` and comments, a trailing one on that line included, stay as they are. The value is quoted, so a tag such as `1.10` stays a string ([Quote numbers and booleans](../reference/fleet.md#quote-numbers-and-booleans)). Add `fleet/hubs/*.yaml` to the `grep` if a hub is in the ring. On macOS, write `sed -E -i ''`.

3.  Run `mise run validate`, commit, and open the PR.

### What CI checks

The fleet pass renders the PR's tree. For the pin itself, POLICY052 checks that `v1.5.0` resolves and has `stack/` and every layer each file lists. POLICY053 then validates the commit: it groups the changed files by the commit they pin and renders `v1.5.0` once, with each file from the PR ([Promotions](../reference/ci.md#promotions-policy053)). A render failure or a safety rule (POLICY037, POLICY041, POLICY042) at the tag blocks the PR, and is reported at each file of the ring. A YAML error in a layer at the tag reads:

```text
error POLICY053 [image-registry/edge-02] revision "v1.5.0" fails RENDER000 at stack/image-registry/values.yaml: Error: failed to parse values/topologies/single-node.yaml: ...
    at fleet/edge-02.yaml
```

Other errors at the tag are warnings, so a rule newer than the tag never blocks a rollback to it.

### Reading Render Diff

Each cluster whose rendered Policies change gets a row whose Revision column reads `v1.4.0` → `v1.5.0`, with the changes per element. Clusters of one ring that list the same layers should show the same changes:

- A row that differs from the others points at that cluster's own `values/clusters/` file, or a layer only it lists.
- A cluster of the ring missing from the table renders the same at both tags.
- `removes Policy` names each Policy the promotion deletes from the hub, renamed ones included. What it created on the cluster stays ([What a rollback leaves on the cluster](rollout.md#promoting-and-rolling-back)).

After the merge, every cluster of the ring moves within about 6 minutes. Applications are updated in place, so compliance history continues ([Promoting and rolling back](rollout.md#promoting-and-rolling-back)).

### Roll back

Revert the promotion PR in a new PR: the **Revert** button on the merged PR opens one. By hand, on a new branch:

```sh
git revert -m 1 <merge sha>   # merged with a merge commit
git revert <sha>              # squash or rebase merge
```

The revert sets every file of the ring back to `v1.4.0` and goes through the same `fleet/` review and checks. OLM doesn't downgrade operators, and fields and objects that `musthave` templates added stay on the clusters ([What a rollback leaves on the cluster](rollout.md#promoting-and-rolling-back)).
