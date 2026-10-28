# Advanced Cluster Security

Central runs on the ACM hub. Every secured cluster, the hub included, registers with one cluster registration secret (CRS) that you generate on the hub, following Red Hat's [CRS procedure](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_security_for_kubernetes/4.11/html/installing/installing-rhacs-on-red-hat-openshift).

```yaml
# values/acm/<hubName>.yaml
stack:
  acs:
    enabled: true
    toggles:
      central: true
      secured: true

# values/environments/<env>.yaml, or any layer every secured cluster reads
stack:
  acs:
    enabled: true
    toggles:
      secured: true
```

## How it works

- You generate the CRS once and apply it to `stackrox` on the hub as `cluster-registration-secret`, the name the operator reads. The hub's SecuredCluster uses it as is.
- The `secured` policy copies it to `stackrox/cluster-registration-secret` on every other secured cluster. `centralEndpoint` is the host of Central's Route on the hub, port 443.
- Each cluster uses the CRS once. Sensor then renews its own certificates, so expiring, replacing or revoking the CRS never affects a registered cluster.

!!! note "Requirements"
    - ACS 4.10+. CRS is Technology Preview on 4.9, which works.
    - Generating a CRS needs the `Admin` role.

## Generate the CRS

On the hub, once `central-central-ready` is Compliant:

```sh
export ROX_CENTRAL_ADDRESS=$(oc -n stackrox get route central -o jsonpath='{.spec.host}'):443
export ROX_API_TOKEN=<token with the Admin role> # (1)!
roxctl -e "$ROX_CENTRAL_ADDRESS" central crs generate fleet --valid-for 8760h --output crs.yaml
oc apply -n stackrox -f crs.yaml
rm crs.yaml
```

1.  Platform Configuration > Integrations > API Token. Or skip it and pass `--password` with the `admin` password from Secret `stackrox/central-htpasswd`.

!!! warning "Set `--valid-for`"
    Without it the CRS lasts 24 hours.

Anything that writes the same Secret works too, for example an ExternalSecret from your secret store.

## Health

| Policy | Compliant means |
|---|---|
| `central-crs-ready` | The hub holds the CRS with over 90 days left. Checked daily |
| `secured-tls` | The cluster holds the CRS. NonCompliant everywhere at once when the hub's copy is gone |
| `secured-agents-ready` | Sensor is ready. A refused or expired registration shows up here |

!!! warning "Not detected"
    A cluster still holding certificates from another Central (moved to another hub, Central reinstalled with a new CA) reports ready while it is disconnected. Check **Platform Configuration > Clusters**.

## Renew or rotate

When `central-crs-ready` turns NonCompliant, run [Generate the CRS](#generate-the-crs) again. The fleet copies follow. To revoke the old one, or after a leak, run `roxctl central crs revoke <id>`; the ID is on the Secret's `crs.platform.stackrox.io/id` annotation. Do not delete the Secret.

After a Central reinstall the CRS is no longer valid. Generate a new one.

## Re-register a cluster

Central refuses a CRS for any cluster name that has connected before.

| Event | Steps |
|---|---|
| Rebuilt or re-created under the same name, including lifecycle `state: absent` then re-added; SecuredCluster or `stackrox` namespace deleted | 1 |
| Moved to another hub; Central reinstalled | 2, 3 |
| Central restored from backup | None if sensor reconnects, else 1, 2, 3 |
| Still refused after step 1 | 2, 3 |
| Still refused after steps 2-3 | 1 |

1. Hub: delete the cluster in **Platform Configuration > Clusters**, or run `roxctl cluster delete --name <cluster>`.
2. Cluster: `oc -n stackrox get secret -o name | grep -E '/tls-cert-|/(sensor|collector|admission-control)-tls$' | xargs oc -n stackrox delete`
3. Cluster: `oc -n stackrox delete pod -l app=sensor`

## Migrating from init bundles

Registered clusters keep the certificates they already have, so they never re-register.

1. Remove `stack.acs.config.bootstrapImage` from every values layer. Otherwise POLICY035 fails the PR.
2. Make sure the hub and every cluster run ACS 4.9 or later. Lift any `operatorPolicies.acs.versions` pin below that.
3. Merge, then [generate the CRS](#generate-the-crs). Until you do, `central-crs-ready` and `secured-tls` are NonCompliant and nothing changes on the clusters.
4. Every cluster gets `cluster-registration-secret`. `centralEndpoint` loses a trailing space the old Job wrote, so sensor rolls out once on each cluster.
5. Wait until `central-crs-ready`, `secured-tls` and `secured-agents-ready` are Compliant everywhere.
6. Optional, on the hub: `oc -n stackrox delete job/create-cluster-init-bundle-v2 sa/create-cluster-init role/create-cluster-init rolebinding/create-cluster-init`.

!!! danger "Keep the old secrets"
    Leave `sensor-tls`, `collector-tls` and `admission-control-tls` on every cluster, and let the old init bundle expire rather than revoking it. Deleting them before `cluster-registration-secret` exists fails sensor with `InitBundleSecretsMissingError`.
