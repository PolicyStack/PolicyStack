# Logging

The [`logging`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/logging) element installs the Red Hat OpenShift Logging operator into `openshift-logging`. On its own it collects nothing. Its optional `forwarders` Policy sends logs to receivers outside the cluster: syslog, Splunk or any HTTP endpoint. Storing logs in the cluster is the [`loki`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/loki) element's job; the two are independent and can run together.

## Turn on forwarders

Each entry under `config.forwarders` becomes one ClusterLogForwarder, named after the entry, with one output and one pipeline. The layer that turns the Policy on fills the map:

```yaml
# values/environments/prod.yaml
stack:
  logging:
    enabled: true
    policies:
      forwarders:
        enabled: true
    config:
      forwarders:
        app-splunk:
          type: splunk
          url: https://splunk.example.com:8088
          logTypes: [application]
          index: openshift
          secret: {name: splunk-hec, token: hecToken}
```

| Key | Required | Meaning |
|---|---|---|
| `type` | yes | `syslog`, `splunk` or `http` |
| `url` | yes | `tls://`, `tcp://` or `udp://` with a port for syslog; `https://` or `http://` otherwise. Another scheme fails the render |
| `logTypes` | yes | Any of `application`, `infrastructure`, `audit`. Each one also grants the collector the matching `collect-<type>-logs` ClusterRole |
| `rfc` | | Syslog only. `RFC5424` (default) or `RFC3164` |
| `index` | | Splunk only. A literal index, or an operator template quoted as a string: `index: '{.log_type\|\|"main"}'`. Unset sends to the HEC token's default index |
| `secret` | Splunk | TLS material and credentials, see [Secrets](#secrets) |
| `enabled` | | `false` deletes the forwarder, see [Removing a forwarder](#removing-a-forwarder) |

The entry name is 3-54 lowercase letters, digits and hyphens, starting with a letter and ending with a letter or digit, and not `logging`, which is the loki element's ClusterLogForwarder. Any other key, a key that doesn't fit the type, or a missing required key fails the render with the path of the entry. A later layer clears an inherited key with `null`, so it can change an entry's type or authentication.

## Secrets

Values live in Git, so credentials don't. Create one Secret per receiver on the hub, in the policy namespace `pstack`. The `outputs` template copies its data to `openshift-logging/<name>-output` on each cluster with `copySecretData`, and the ClusterLogForwarder reads it from there. A change to the hub Secret reaches every cluster without a commit.

```sh
oc -n pstack create secret generic splunk-hec --from-literal=hecToken=<token>
```

`secret.name` names the hub Secret. Every other key under `secret` names the key inside it that feeds one ClusterLogForwarder field. Leave out what you don't use:

| `secret` key | Field | Types |
|---|---|---|
| `ca` | `tls.ca`, the CA that verifies the receiver | all |
| `certificate` and `key` | `tls.certificate` and `tls.key`, a client certificate for mutual TLS. Set both or neither | all |
| `token` | Splunk HEC token, or HTTP bearer token | splunk (required), http |
| `username` and `password` | HTTP basic authentication. Set both, and not with `token` | http |

!!! note
    - Syslog has no authentication; a syslog receiver identifies the cluster by its client certificate.
    - Without `ca`, the collector trusts the cluster's CA bundle, which includes the proxy's `trustedCA`. A receiver signed by a private CA needs `ca`.
    - The copy is `musthave`: a key deleted from the hub Secret stays on the cluster until you delete it there.

## What it creates

The Policy is `forwarders-logging-<cluster>`. It waits for `install` to be Compliant, so the operator and its ClusterRoles exist before anything is created. Every object lives in `openshift-logging` unless it is cluster-scoped.

| Template | Objects per enabled entry `<name>` |
|---|---|
| `forwarders-collectors` | ServiceAccount `<name>-collector`. ClusterRoleBinding `<name>-collect-<type>-logs` per `logTypes` entry |
| `forwarders-outputs` | Secret `<name>-output` when `secret.name` is set. Nothing waits for it, so a hub Secret that doesn't exist yet fails only this template, and the forwarder reading it shows in [Readiness](#readiness) |
| `forwarders-clusterlogforwarders` | ClusterLogForwarder `<name>`, `mustonlyhave`: a key removed from the entry is removed from the forwarder. Waits for `forwarders-collectors` |
| `forwarders-ready` | Nothing; `InformOnly` check, see [Readiness](#readiness) |
| `forwarders-removed` | Deletes what values no longer ask for, see below. Waits for `forwarders-clusterlogforwarders`, so a binding or Secret goes only after the forwarder stops using it |

`forwarders-clusterlogforwarders` waits for `forwarders-collectors` only on the first rollout. A forwarder or log type added later can reach the cluster before its ServiceAccount or binding; the operator then reports `Authorized` `False` and corrects itself on its next resync, within about 3 minutes.

The names never collide with the loki element, whose forwarder is ClusterLogForwarder `logging` with ServiceAccount `collector` and ClusterRoleBindings named after the ClusterRoles. Each forwarder has its own ServiceAccount, so each can read only its own log types.

## Readiness

`forwarders-ready` is NonCompliant until, for every enabled entry:

- ClusterLogForwarder `<name>` has condition `Ready` `True`, and
- DaemonSet `<name>`, which the operator creates, has at least one pod and every pod it schedules is ready. A DaemonSet that schedules no pod reports NonCompliant.

Both are needed. The operator sets `Ready` once it has written the DaemonSet, without waiting for pods, and leaves it `True` when the collector later fails to connect. The check is `InformOnly`, so it never changes the cluster, even when the Policy enforces.

!!! warning "Ready doesn't prove delivery"
    A wrong CA or an unreachable receiver shows only in the collector pod logs: `oc logs -n openshift-logging ds/<name>`. A spec the API server refuses, such as a syslog URL without a port, shows as a violation of `forwarders-clusterlogforwarders` in Governance, and no forwarder exists. A spec the operator rejects shows on the forwarder: `oc get clusterlogforwarder <name> -n openshift-logging -o jsonpath='{.status}'`, with condition `observability.openshift.io/ValidOutput-<name>` saying why.

## Removing a forwarder

Set `enabled: false` on the entry. Only the name matters; the other keys can stay or go:

```yaml
stack:
  logging:
    config:
      forwarders:
        app-splunk:
          enabled: false
```

Under `enforce`, `forwarders-removed` then deletes ClusterLogForwarder `<name>`, which stops its DaemonSet, plus ServiceAccount `<name>-collector`, Secret `<name>-output`, all three `<name>-collect-<type>-logs` ClusterRoleBindings and `metadata-reader-openshift-logging-<name>-collector`, a binding the operator creates and never deletes. Under `inform` it reports them NonCompliant instead. Once the cluster is Compliant you can delete the entry.

The same template keeps enabled entries tidy: dropping a type from `logTypes` deletes its binding, so the collector loses read access to it, and unsetting `secret` deletes `<name>-output`.

!!! danger "Deleting the entry doesn't delete the forwarder"
    An entry removed from `config.forwarders`, or the Policy turned off, stops managing its objects. The forwarder keeps sending logs, unmanaged. Disable the entry first, as above.

## STIG: audit logs over TLS

The [compliance element](compliance.md)'s `ocp4-stig` scan has two forwarding rules: `ocp4-stig-audit-log-forwarding-enabled` (part of CNTR-OS-000170 and CNTR-OS-000220) and `ocp4-stig-audit-log-forwarding-uses-tls` (part of CNTR-OS-000340 and CNTR-OS-000510). A syslog receiver over TLS with a client certificate passes both rules. The controls need more than the forwarder, such as FIPS mode on every node and RHCOS audit settings, which other rules check.

1.  On the hub, put the receiver's CA and the cluster's client certificate in one Secret:

    ```sh
    oc -n pstack create secret generic audit-syslog-tls \
      --from-file=ca-bundle.crt=siem-ca.pem \
      --from-file=tls.crt=client.crt \
      --from-file=tls.key=client.key
    ```

2.  Turn on the forwarder in the layer the STIG clusters share:

    ```yaml
    # values/environments/prod.yaml
    stack:
      logging:
        enabled: true
        policies:
          forwarders:
            enabled: true
        config:
          forwarders:
            audit-syslog:
              type: syslog
              url: tls://siem.example.com:6514
              logTypes: [audit]
              secret:
                name: audit-syslog-tls
                ca: ca-bundle.crt
                certificate: tls.crt
                key: tls.key
    ```

The cluster then runs:

```yaml
apiVersion: observability.openshift.io/v1
kind: ClusterLogForwarder
metadata:
  name: audit-syslog
  namespace: openshift-logging
spec:
  managementState: Managed
  serviceAccount:
    name: audit-syslog-collector          # bound to collect-audit-logs only
  outputs:
    - name: audit-syslog
      type: syslog
      syslog:
        url: tls://siem.example.com:6514
        rfc: RFC5424
      tls:
        ca: {key: ca-bundle.crt, secretName: audit-syslog-output}
        certificate: {key: tls.crt, secretName: audit-syslog-output}
        key: {key: tls.key, secretName: audit-syslog-output}
  pipelines:
    - name: audit-syslog
      inputRefs: [audit]
      outputRefs: [audit-syslog]
```

!!! warning "One plain-text URL fails the whole cluster"
    `ocp4-stig-audit-log-forwarding-uses-tls` reads every ClusterLogForwarder in `openshift-logging` and fails if any output URL is not `https://` or `tls://`. A second forwarder sending application logs over `tcp://` or `http://` fails the STIG rule too. Outputs without a URL, such as the loki element's, are ignored.

!!! warning "The audit forwarder must sort first"
    Compliance Operator content up to ComplianceAsCode 0.1.82 checks `ocp4-stig-audit-log-forwarding-enabled` against only the first ClusterLogForwarder in `openshift-logging`, in name order. With forwarders `app-splunk` and `audit-syslog`, it reads `app-splunk`, finds no `audit` input and fails. Until your content has the upstream fix, name the audit forwarder so it sorts before every other ClusterLogForwarder in the namespace, including loki's `logging`, or add `audit` to each one.

!!! note "Hosted control planes"
    On a hosted control plane guest the API server runs on the management cluster, so its audit log is not on the guest. `ocp4-stig-audit-log-forwarding-enabled` is not applicable there. `ocp4-stig-audit-log-forwarding-uses-tls` still checks every forwarder URL on the guest. The hosted API server's audit log is not covered by this element.
