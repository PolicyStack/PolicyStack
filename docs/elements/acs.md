# Advanced Cluster Security

The [`acs`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/acs) element installs Red Hat Advanced Cluster Security (ACS) and registers clusters with it. ACS has two parts:

- **Central**: the UI, API, database and scanners. It runs once, on the ACM hub.
- **Secured cluster**: the agents on every cluster you want protected. Sensor watches the cluster and connects to Central; Collector and Admission Controller run alongside it.

A new secured cluster proves itself to Central once, with a **cluster registration secret (CRS)**. You generate one CRS on the hub, following Red Hat's [CRS procedure](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_security_for_kubernetes/4.11/html/installing/installing-rhacs-on-red-hat-openshift), and the element copies it to every cluster. Sensor uses it to register, gets its own certificates back and renews them itself, so the CRS expiring, being replaced or revoked never affects a cluster that has already registered.

## Requirements

- ACS 4.9 or later. CRS registration is GA from 4.10; on 4.9 it is Technology Preview but works with this element.
- A default StorageClass on the hub. Central's database PVC `stackrox-db` uses it; without one Central never starts and `central-central-ready` stays NonCompliant.
- Generating a CRS needs the ACS `Admin` role.
- Every secured cluster must reach Central's Route on the hub on port 443.

## Turn it on

`enabled: true` installs the operator. `policies.central` and `policies.secured` are the element's two optional Policies ([How an element is built](../concepts/elements.md)); turn on the role each cluster plays. Both ship off.

=== "Hub"

    In a [layer](../concepts/values.md#layers) only the hub reads: `values/clusters/<hubName>.yaml` (the hub's [`hubName`](../concepts/overview.md#words-with-more-than-one-meaning)), or a file such as `values/acm/<hubName>.yaml` that only hub fleet files list ([Hub file](../reference/fleet.md#hub-file)). Turning `central` on for any other cluster starts a second Central there that no cluster registers with.

    ```yaml
    stack:
      acs:
        enabled: true
        policies:
          central:
            enabled: true
          secured: # (1)!
            enabled: true
    ```

    1.  Optional. It makes ACS monitor the hub itself. Other managed clusters don't need it.

=== "Secured clusters"

    In any layer those clusters read, for example `values/environments/prod.yaml`.

    ```yaml
    stack:
      acs:
        enabled: true
        policies:
          secured:
            enabled: true
    ```

Central must run on the ACM hub: the `secured` Policy reads the CRS and Central's Route from the hub ([How the CRS reaches each cluster](#how-the-crs-reaches-each-cluster)). Until `central` runs there and you have [generated the CRS](#generate-the-crs), `secured-tls` on every secured cluster is NonCompliant with a `template-error`.

Settings (auth, scanner sizing, SecurityPolicies, admission control) are in the element's [README](https://github.com/PolicyStack/PolicyStack/blob/main/stack/acs/README.md#settings). The defaults:

- OpenShift login, with the `cluster-admins` group mapped to the ACS `Admin` role and no role for other users.
- Three SecurityPolicies: no privilege escalation, no root user, no shell spawning.
- Admission control enforcement off, because the webhook sits in every deployment's path. The operator's own default is on.

## What it creates

`<cluster>` is the ManagedCluster name, or `hubName` on the hub ([Rendered names](../concepts/elements.md#rendered-names)). Every Policy enforces. Templates marked (InformOnly) only report, even inside an enforcing Policy ([inform, enforce and InformOnly](../concepts/acm.md#inform-enforce-and-informonly)).

| Policy | Where | Templates |
|---|---|---|
| `install-acs-<cluster>` | Every cluster with `enabled: true` | `install-acs-ns`, `install-acs` (OperatorPolicy, package `rhacs-operator`, channel `stable`), `install-acs-status` (reports only, [The install Policy](../concepts/elements.md#the-install-policy)) |
| `central-acs-<hubName>` | The hub | `central-central` (namespace `stackrox`, Central `stackrox-central-services`, auth config), `central-central-ready` (InformOnly), `central-policies` (SecurityPolicies, after `central-central-ready`), `central-crs-ready` (InformOnly), `central-hub-access`, `central-console` (console link) |
| `secured-acs-<cluster>` | Every secured cluster | `secured-tls` (the CRS copy), `secured-agents` (SecuredCluster `stackrox-secured-cluster-services`, after `secured-tls`), `secured-agents-ready` (InformOnly) |

`central` and `secured` stay Pending until `install-acs-<cluster>` is Compliant, so the operator is running before any ACS resource is written.

### How the CRS reaches each cluster

`secured-tls` holds a Secret whose data is the hub template `{{hub copySecretData "stackrox" "cluster-registration-secret" hub}}`. ACM resolves hub templates on the hub while it copies the Policy to each cluster ([ACM policies in brief](../concepts/acm.md)), so the CRS never sits in Git:

1. The ACM policy propagator reads `stackrox/cluster-registration-secret` on the hub, as ServiceAccount `acs-hub-templates`. `central-hub-access` creates that ServiceAccount, a Role in `stackrox` that can read only this Secret and the `central` Route, and a ClusterRole to read ManagedClusters (the hub skip below reads their labels).
2. It writes the data, encrypted, into the cluster's copy of `secured-acs-<cluster>`. The cluster decrypts it and creates `stackrox/cluster-registration-secret`. The hub skips this: its own SecuredCluster reads the original.
3. `secured-agents` fills the SecuredCluster's `centralEndpoint` the same way, from the hub's Route `stackrox/central` plus `:443`, and its `clusterName` from the ManagedCluster name. You set neither. A missing Route is an error, never an empty endpoint.

The propagator watches what it read. Replacing the hub's Secret re-copies it to every cluster without a commit.

## First run

1. Merge the values above to the revision the hub's fleet file pins (move a tag or SHA pin, [Promoting and rolling back](../guides/rollout.md#promoting-and-rolling-back)). The operator installs, then Central deploys on the hub.
2. Wait for `central-central-ready` to be Compliant: in the console, **Governance > Policies > `central-acs-<hubName>`**.
3. [Generate the CRS](#generate-the-crs).
4. Each secured cluster gets the copy, then its SecuredCluster, and turns Compliant.

Until step 3 these are not Compliant, and that is expected:

| Template | State | Why |
|---|---|---|
| `central-crs-ready` | NonCompliant | No CRS on the hub yet |
| `secured-tls`, on every secured cluster | NonCompliant | The hub template can't find the Secret |
| `secured-agents` | Pending | Waits for `secured-tls`, so no SecuredCluster exists yet |
| `secured-agents-ready` | NonCompliant | No SecuredCluster, no Sensor |

They clear on their own once the Secret exists.

## Generate the CRS

Run this from a shell logged in to the hub with `oc`. `roxctl` is the ACS CLI; install it first from Red Hat's ACS docs.

```sh
export ROX_ENDPOINT=$(oc -n stackrox get route central -o jsonpath='{.spec.host}'):443 # (1)!
export ROX_API_TOKEN=<token with the Admin role> # (2)!
roxctl central crs generate "fleet-$(date +%Y%m%d)" --valid-for 8760h --output crs.yaml # (3)!
oc apply -n stackrox -f crs.yaml
rm crs.yaml
```

1.  Every `roxctl` command on this page reads it. The Route serves Central's own certificate, so on `x509: certificate signed by unknown authority` also `export ROX_CA_CERT_FILE=<file>` holding key `ca.pem` of Secret `stackrox/central-tls`.
2.  **Platform Configuration > Integrations > API Token**. Or skip it and pass `--password` with the `admin` password from Secret `stackrox/central-htpasswd`.
3.  The CRS's name in Central. Central refuses a name already used by a CRS that isn't revoked, so the date keeps renewals unique. 8760h is one year.

`crs.yaml` is a Secret named `cluster-registration-secret`, the name the operator reads, with annotations that include `crs.platform.stackrox.io/name`, `id` and `expires-at`.

!!! warning "Set `--valid-for`"
    Without it the CRS lasts 24 hours and `central-crs-ready` is NonCompliant from the start ([Health](#health)).

Any other way of writing the Secret works, an ExternalSecret from your secret store for example, as long as it also writes the `crs.platform.stackrox.io/expires-at` annotation (with the External Secrets Operator: `spec.target.template.metadata.annotations`). Without it, `central-crs-ready` stays NonCompliant forever.

## Health

| Template | Policy | Compliant means |
|---|---|---|
| `central-crs-ready` | `central-acs-<hubName>` | The hub holds the CRS with over 90 days left, so a CRS valid for 90 days or less never is. Checked daily, every 10 minutes while NonCompliant. A one-year CRS turns NonCompliant about 9 months after you generate it |
| `secured-tls` | `secured-acs-<cluster>` | The cluster holds the copy. Turns NonCompliant on every cluster at once when the hub's Secret is gone |
| `secured-agents-ready` | `secured-acs-<cluster>` | The SecuredCluster is deployed and Sensor is ready. A refused or expired registration shows up here, because Sensor never becomes ready |

!!! warning "Not detected"
    A cluster still holding certificates from another Central (moved to another hub, Central reinstalled with a new CA) reports ready while it is disconnected. Check **Platform Configuration > Clusters** in ACS.

## Renew or rotate

When `central-crs-ready` turns NonCompliant:

1. Note the current CRS's name: `oc -n stackrox get secret cluster-registration-secret -o jsonpath='{.metadata.annotations.crs\.platform\.stackrox\.io/name}'`. Applying the new one overwrites it.
2. Run [Generate the CRS](#generate-the-crs) again. The copies on every cluster follow.
3. Optional: revoke the old one with `roxctl central crs revoke <name>`, with the exports from [Generate the CRS](#generate-the-crs). Do this right away after a leak. `roxctl central crs list` shows every CRS with its expiry.

!!! danger "Never delete the Secret"
    Replace it with `oc apply` instead. Registration doesn't need it again, but the operator reads `cluster-registration-secret` on every reconcile, expired or not. Without it, or the old init bundle secrets, the operator fails to reconcile the SecuredCluster (`InitBundleSecretsMissingError`), so setting changes stop reaching the agents. A deleted copy comes back at the next `secured-tls` evaluation; a deleted hub Secret turns `secured-tls` NonCompliant on every cluster and breaks the hub's own SecuredCluster.

After a Central reinstall the CRS is no longer valid. Generate a new one.

## Re-register a cluster

Central refuses a CRS for a cluster name that has connected before. In ACS, each cluster is named after its ManagedCluster; the hub is `local-cluster` unless ACM's `localClusterName` changed it.

A refused cluster shows `secured-agents-ready` NonCompliant, and the Sensor pod's CRS init container crash-loops (`oc -n stackrox get pods -l app=sensor`).

| Event | Steps |
|---|---|
| Rebuilt or re-created under the same name, including [lifecycle](../guides/lifecycle.md#destroy) `state: absent` then re-added; SecuredCluster or `stackrox` namespace deleted | 1 |
| Moved to another hub; Central reinstalled | 2, 3 |
| Central restored from backup | None if Sensor reconnects, else 1, 2, 3 |
| Still refused | The steps you skipped, in order 1, 2, 3 |

1. Hub: delete the cluster's old record so Central accepts the name again. **Platform Configuration > Clusters**, or `roxctl cluster delete --name <cluster>` with the exports from [Generate the CRS](#generate-the-crs).
2. Cluster: delete the certificates Sensor already holds, so it falls back to the CRS. `oc -n stackrox get secret -o name | grep -E '/tls-cert-|/(sensor|collector|admission-control)-tls$' | xargs oc -n stackrox delete`
3. Cluster: restart Sensor so it registers. `oc -n stackrox delete pod -l app=sensor`

Step 2 is safe only once `cluster-registration-secret` is on the cluster; without it Sensor has nothing to register with.

## Turning it off

Turning off `secured`, `central` or the element deletes their Policies but nothing they created: no template sets `pruneObjectBehavior`, the ACM setting that deletes a template's objects with its Policy ([Removal and pruning](../concepts/removal.md#what-happens-on-the-managed-cluster)). The operator, Central or SecuredCluster, the `stackrox` namespace and the CRS copy stay, and the cluster stays registered in Central. Re-enabling it picks up the existing SecuredCluster and Sensor keeps its registration. If the SecuredCluster or `stackrox` namespace was deleted in the meantime, or the cluster was rebuilt, follow [Re-register a cluster](#re-register-a-cluster).

## Clusters registered with init bundles

An init bundle is ACS's older registration method: TLS Secrets (`sensor-tls`, `collector-tls`, `admission-control-tls`) applied to each cluster. This section is for clusters that already run ACS agents registered with one, for example from before PolicyStack. New installs skip it. The element manages Central `stackrox-central-services` and SecuredCluster `stackrox-secured-cluster-services` in `stackrox`; yours must have those names, or the element creates second ones.

1. Check the names: `oc get central,securedcluster -A`. Both must be in `stackrox` with the names above.
2. Make sure the hub and every cluster run ACS 4.9 or later. Lift any `operatorPolicies.acs.versions` pin below that ([Operator versions](../guides/operators.md#pinning)).
3. [Turn the element on](#turn-it-on), previewing step 4 with a [dry run](../guides/rollout.md#dry-run) first, then merge and [generate the CRS](#generate-the-crs). Until you generate it, `central-crs-ready` and `secured-tls` are NonCompliant and the SecuredCluster isn't touched.
4. Every cluster gets `cluster-registration-secret`, then the element's SecuredCluster settings merge into yours; the hub's Central gets the element's Central settings, auth config and SecurityPolicies the same way. Any field that differs changes at that point: admission control enforcement turns off unless `config.admissionControl.enabled: true`.
5. Wait until `central-crs-ready`, `secured-tls` and `secured-agents-ready` are Compliant everywhere.

!!! danger "Keep the old secrets"
    Leave `sensor-tls`, `collector-tls` and `admission-control-tls` on every cluster, and let the old init bundle expire rather than revoking it: revoking cuts off every cluster still using its certificates. They are the cluster's only credentials until `cluster-registration-secret` exists; deleting them first fails Sensor with `InitBundleSecretsMissingError`.
