# File Integrity

The [`file-integrity`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/file-integrity) element installs the File Integrity Operator, which runs AIDE (Advanced Intrusion Detection Environment) on every Linux node. AIDE records checksums, permissions and SELinux labels of the files under each node's `/boot`, `/root`, `/usr` and `/etc`. It rescans every 15 minutes and reports any file that changed since. Nodes with changes show in Governance: `findings-file-integrity-<cluster>` turns NonCompliant and names them.

## Turn it on

```yaml
# values/environments/prod.yaml
stack:
  fileIntegrity:
    enabled: true
```

Defaults ([README](https://github.com/PolicyStack/PolicyStack/blob/main/stack/file-integrity/README.md#settings)):

| Key under `stack.fileIntegrity.config` | Default |
|---|---|
| `gracePeriod` | `900` seconds between scans |
| `aideExclusions` | `/etc/machine-config-daemon/previous-logs` ([Exclusions](#exclusions)) |

The AIDE pods tolerate every taint, so control plane, infra, storage and other dedicated nodes are all scanned. Windows nodes are skipped. Red Hat supports RHCOS nodes only, all on the same RHCOS version.

!!! warning "Hosted control planes"
    Red Hat doesn't document the operator on hosted clusters, and it is untested there. Leave the element off on HCP guests.

## What it creates

| Policy | Templates | Waits for |
|---|---|---|
| `install-file-integrity-<cluster>` | Namespace `openshift-file-integrity`, the OperatorPolicy (package `file-integrity-operator`, channel `stable`) and its status check ([The install Policy](../concepts/elements.md#the-install-policy)) | |
| `config-file-integrity-<cluster>` | `config-file-integrity`, in this order: the namespace's `pod-security.kubernetes.io/enforce: privileged` label, ConfigMap `file-integrity-aide`, FileIntegrity `file-integrity` | `install` Compliant |
| `findings-file-integrity-<cluster>` | `findings-file-integrity-active`, `findings-file-integrity-nodes` ([Reading Governance](#reading-governance)) | `config` Compliant |

One FileIntegrity covers every node. Per-role instances would scan schedulable control plane nodes twice, because those nodes are workers too.

## How AIDE is configured

The operator ships a default AIDE configuration. A FileIntegrity's `spec.config` replaces that default whole; you can't add to it. The element follows Red Hat's custom-configuration procedure:

1. ConfigMap `file-integrity-aide` holds the operator's default, copied from operator 1.5.0, followed by one `!<path>` rule per `aideExclusions` entry.
2. The FileIntegrity points at it.
3. The operator rewrites the database paths, prefixes `/hostroot` to each path, and stores the result in ConfigMap `file-integrity`, which the AIDE pods read.

Edit neither ConfigMap on the cluster. ACM restores `file-integrity-aide`, and the operator overwrites `file-integrity`.

!!! note "Operator upgrades"
    Clusters keep the 1.5.0 copy of the default until an element release updates it, even after the operator changes its own. Red Hat changed the default once between 1.3.4 and 1.5.0. If every node goes `Errored` right after an operator upgrade, the operator has moved to a newer AIDE that rejects the copy. [Pin the operator](../guides/operators.md#pinning) to the previous version until an element release refreshes it.

### Exclusions

The machine config daemon copies its log to `/etc/machine-config-daemon/previous-logs` on every restart. The operator's default doesn't exclude that path. Without the element's default exclusion, every daemon restart would make a node `Failed` for a change nobody made.

```yaml
stack:
  fileIntegrity:
    config:
      aideExclusions:
        /etc/myapp/cache: "Rewritten by myapp on start. OPS-123"
```

The key is the host path without `/hostroot`. It must start with `/` and contain no spaces, or the render fails. It is an AIDE regular expression matched from the start, so `/etc/myapp` also excludes `/etc/myapp2` and everything below both. End it with `$` to match one file. Layers merge the map, and `null` in a later layer removes an entry, the default one included.

!!! warning "A configuration change re-initializes"
    Any change to the ConfigMap, whether from `aideExclusions` or from an element release that refreshes the default, makes the operator rebuild the database on every node from the files as they are at that moment. A change made before the rebuild is never reported.

## Reading Governance

| Template | NonCompliant means |
|---|---|
| `findings-file-integrity-active` | The FileIntegrity isn't `Active`: `Pending` (no daemon set yet), `Initializing` (database being rebuilt) or `Error` (AIDE couldn't run on a node; see the next row) |
| `findings-file-integrity-nodes` | A node's latest scan is `Failed` (files changed) or `Errored` (AIDE couldn't run). ACM's message names the FileIntegrityNodeStatus objects, `file-integrity-<node>` |

`Initializing` is expected for a few minutes in three cases:

- after install, because the operator always runs one re-init when it switches to the custom configuration
- after a configuration change
- on each node after a Machine Config Operator update

The operator pauses scans during an MCO update and re-initializes afterwards, so a MachineConfig rollout raises no `Failed` nodes.

The operator also fires the `NodeHasIntegrityFailure` alert in cluster monitoring.

## After a failure

1.  Read what changed:

    ```sh
    oc get fileintegritynodestatus file-integrity-<node> -n openshift-file-integrity \
      -o jsonpath='{.lastResult.resultConfigMapName}'
    oc get cm <failure-cm> -n openshift-file-integrity -o jsonpath='{.data.integritylog}'
    ```

    Logs over 1 MB are stored as a base64 gzip archive, marked by the annotation `file-integrity.openshift.io/compressed`. Extract those with:

    ```sh
    oc get cm <failure-cm> -n openshift-file-integrity -o json | jq -r '.data.integritylog' | base64 -d | gunzip
    ```

2.  Treat an unexpected change as an incident. A path that changes routinely belongs in [Exclusions](#exclusions). To accept a one-off change, re-initialize the failed nodes:

    ```sh
    oc annotate fileintegrities/file-integrity -n openshift-file-integrity file-integrity.openshift.io/re-init-on-failed=
    ```

    `file-integrity.openshift.io/re-init=` re-initializes every node. The finding clears after the next scan.

3.  Once it's handled, delete the failure ConfigMap with `oc delete cm <failure-cm> -n openshift-file-integrity`. The operator keeps failure ConfigMaps as a record and never deletes them.

!!! danger "Never manage the re-init annotations from a policy"
    The operator removes them once the re-init starts. An enforced policy would add them back on every evaluation and rebuild the database in a loop, so no change would ever be reported.
