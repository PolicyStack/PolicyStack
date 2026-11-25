# API Hardening

The [`api-hardening`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/api-hardening) element applies cluster-wide security settings that Red Hat documents as day-2 procedures: etcd encryption, the API server audit profile, TLS security profiles for the API server and the default ingress controller, console banners, and removing the `kubeadmin` user. It installs no operator. Every setting ships off, and you turn each one on by itself.

## Turn it on

Enabling the element alone renders no Policy. Turn on the settings you want, and change a value only where the default doesn't suit you:

```yaml
# values/environments/prod.yaml
stack:
  apiHardening:
    enabled: true
    config:
      apiServerTLSProfile: Modern
    policies:
      config:
        templates:
          etcd-encryption: {enabled: true}
          audit-profile: {enabled: true}
          apiserver-tls: {enabled: true}
      kubeadmin:
        enabled: true
```

| Template or Policy | Key under `stack.apiHardening.config` | Default | Allowed |
|---|---|---|---|
| `etcd-encryption` | `etcdEncryption` | `aesgcm` | `aesgcm`, `aescbc`, `identity` |
| `audit-profile` | `auditProfile` | `WriteRequestBodies` | `Default`, `WriteRequestBodies`, `AllRequestBodies` |
| `apiserver-tls` | `apiServerTLSProfile` | `Intermediate` | `Intermediate`, `Modern` |
| `ingress-tls` | `ingressTLSProfile` | `Intermediate` | `Intermediate`, `Modern` |
| `console-banners` | `consoleBanners` | `{}`, fill it ([Console banners](#console-banners)) | |
| Policy `kubeadmin` | | | |

Any other value fails the render once its template is on. Each section below says why a value Red Hat allows is left out.

!!! warning "Hosted control planes"
    Every template skips its objects on a hosted control plane (HCP) guest, a cluster whose `Infrastructure` reports `controlPlaneTopology: External`. There HyperShift writes the guest's `APIServer` and `kubeadmin` Secret from the HostedCluster on the management cluster and overwrites edits made in the guest. Set them on the HostedCluster instead: `spec.secretEncryption` for etcd encryption, `spec.configuration.apiServer` for audit and TLS, `spec.configuration.oauth` for identity providers, which also removes `kubeadmin`. The guest router follows the APIServer profile. The HostedCluster has no setting for console banners or a separate router profile, so this element applies neither on HCP. The skipped templates show Compliant.

## What it creates

| Policy | Templates | Writes |
|---|---|---|
| `config-api-hardening-<cluster>` | `config-etcd-encryption`, `config-audit-profile`, `config-apiserver-tls` | One field each of `APIServer` `cluster` |
| | `config-ingress-tls` | `spec.tlsSecurityProfile` of `IngressController` `default` in `openshift-ingress-operator` |
| | `config-console-banners` | One `ConsoleNotification` per `consoleBanners` entry |
| `kubeadmin-api-hardening-<cluster>` | `kubeadmin-removal`, `kubeadmin-absent` | Deletes Secret `kube-system/kubeadmin` ([kubeadmin](#removing-kubeadmin)) |

`APIServer` and `IngressController` are shared with other elements and with the cluster's own operators (`cert-manager` sets serving certificates, `infra-nodes` sets ingress node placement). Their templates use `musthave`, so each sets only its own field and leaves the rest alone. The element owns each `ConsoleNotification`, so `console-banners` uses `mustonlyhave`: a field removed from an entry is removed from the banner. Changing one of these fields rolls out new API server or router pods; expect several minutes of rolling restarts per change.

!!! note "Turning a template off doesn't revert"
    `musthave` never removes a field. A template turned off, or the element disabled, leaves the cluster on the last value it applied. To go back, keep the template on with the Red Hat default, `Default` for audit and `Intermediate` for the APIServer TLS profile, or `identity` to decrypt etcd, and turn it off once the cluster reports it. The ingress default is no profile, which follows the APIServer's; see [TLS security profiles](#tls-security-profiles).

## etcd encryption

Follows [Enabling etcd encryption](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/etcd/etcd-encrypt). Sets `APIServer` `spec.encryption.type`. The API servers then encrypt the values of Secrets, ConfigMaps, Routes, OAuth access tokens and OAuth authorize tokens. Names, namespaces and resource types stay in plain text.

- `aesgcm` and `aescbc` both use 32-byte keys that the cluster rotates every 7 days. Red Hat prefers neither.
- Changing between them re-encrypts everything automatically.
- `identity` decrypts.
- KMS is left out. It is Technology Preview in 4.20, needs a `TechPreviewNoUpgrade` cluster, and its API is still changing.

Encrypting or decrypting takes 20 minutes or longer, depending on the size of etcd. The policy goes Compliant as soon as the field is set, not when encryption finishes. Check progress with:

```sh
for r in openshiftapiserver kubeapiserver authentication.operator.openshift.io; do
  oc get $r -o=jsonpath='{range .items[0].status.conditions[?(@.type=="Encrypted")]}{.reason}{": "}{.message}{"\n"}{end}'
done
```

All three report `EncryptionCompleted` when done, or `DecryptionCompleted` after `identity`. `EncryptionInProgress` or `DecryptionInProgress` means it is still running. After a change between `aesgcm` and `aescbc` the condition stays `EncryptionCompleted` throughout, because data under the old key still counts as encrypted, so it can't tell you the re-encryption has finished.

!!! warning "Backups"
    Don't back up etcd until encryption completes, or the backup may be only partly encrypted. Restoring an encrypted backup needs its keys, which the backup writes to `static_kuberesources_<datetimestamp>.tar.gz`. Store that file apart from the snapshot.

## Audit profile

Follows [Configuring the audit log policy](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/security_and_compliance/audit-log-policy-config). Sets `APIServer` `spec.audit.profile`, which applies to the Kubernetes, OpenShift and OAuth API servers and the OAuth server.

| Profile | Logs |
|---|---|
| `Default` | Metadata of every request, plus OAuth token request bodies. What every cluster starts with |
| `WriteRequestBodies` | `Default` plus the body of every create, update, patch and delete |
| `AllRequestBodies` | `WriteRequestBodies` plus the body of every read. Most CPU, memory and disk I/O |

Secrets, Routes and OAuthClients are logged at metadata level whatever the profile. `None` is left out: it disables audit logging, which Red Hat advises against. Per-group `customRules` are left out too: `musthave` appends list entries, so it could never change or reorder a rule, and rules apply first match first.

The Kubernetes API server rolls out a new revision. It has finished when this prints `AllNodesAtLatestRevision`:

```sh
oc get kubeapiserver -o=jsonpath='{range .items[0].status.conditions[?(@.type=="NodeInstallerProgressing")]}{.reason}{"\n"}{.message}{"\n"}{end}'
```

## TLS security profiles

Follows [Configuring TLS security profiles](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/security_and_compliance/tls-security-profiles).

- `apiserver-tls` sets `APIServer` `spec.tlsSecurityProfile`. The Kubernetes API server, controller manager and scheduler, the OpenShift and OAuth API servers, the OAuth server, etcd and the Machine Config Server use it. The kubelet doesn't: it has its own profile in `KubeletConfig`.
- `ingress-tls` sets the default `IngressController`'s profile, which the router uses for `*.apps`.

An ingress controller without its own profile uses the API server's. So `apiserver-tls` alone changes the router too, and `ingress-tls` is only needed for a different router profile. Once `ingress-tls` has applied a profile, the router stops following the API server, even after the template is turned off. To make it follow again, turn the template off and remove the field:

```sh
oc patch ingresscontroller default -n openshift-ingress-operator --type=json \
  -p '[{"op":"remove","path":"/spec/tlsSecurityProfile"}]'
```

| Profile | Minimum TLS | For |
|---|---|---|
| `Intermediate` | 1.2 | Most clients. The default for both |
| `Modern` | 1.3 | Clients that all support TLS 1.3. Anything older can no longer connect |

`Old` is left out because it lowers the minimum to TLS 1.0. `Custom` is left out because `musthave` merges cipher lists: a cipher once applied could never be removed. Changing profile leaves the previous profile's empty key, such as `modern: {}`, on the object. The operators read only `type`, so it does nothing.

!!! note
    A profile's ciphers can change between OpenShift releases, so an upgrade may roll out the API servers or routers again.

The router reports the ciphers and minimum version in effect in `oc get ingresscontroller default -n openshift-ingress-operator -o jsonpath='{.status.tlsProfile}'`.

## Console banners

Follows [Creating custom notification banners](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/web_console/customizing-web-console#creating-custom-notification-banners_customizing-web-console). Each `consoleBanners` entry becomes a cluster-scoped `ConsoleNotification`, named by its key, shown on every console page:

```yaml
stack:
  apiHardening:
    config:
      consoleBanners:
        authorized-use:
          text: "Authorized use only. Activity is monitored."
          location: BannerTopBottom
          color: "#fff"
          backgroundColor: "#c9190b"
        maintenance:
          text: "Maintenance Saturday 02:00 UTC"
          link: {href: "https://status.example.com", text: "Status page"}
    policies:
      config:
        templates:
          console-banners: {enabled: true}
```

| Field | Required | Value |
|---|---|---|
| `text` | Yes | The message |
| `location` | No | `BannerTop` (default), `BannerBottom` or `BannerTopBottom` |
| `link` | No | `href`, which must start with `https://`, and `text`, both required |
| `color`, `backgroundColor` | No | Any CSS color |

Layers merge `consoleBanners` by key. A field removed from an entry, such as `link`, is removed from the banner. An entry removed from it, or set to `null` in a later layer, stays on the cluster unmanaged; delete it with `oc delete consolenotification <name>`. On a cluster installed without the Console capability the `ConsoleNotification` kind doesn't exist, and the template reports NonCompliant.

## Removing kubeadmin

Follows [Removing the kubeadmin user](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/authentication_and_authorization/removing-kubeadmin). The installer creates `kubeadmin` as a temporary cluster-admin. Red Hat's procedure deletes Secret `kube-system/kubeadmin`, which can't be undone, and only after two prerequisites. Deleting it before another user is cluster-admin means reinstalling the cluster.

`kubeadmin-removal` deletes the Secret only when the cluster meets both:

1.  `OAuth` `cluster` lists at least one identity provider.
2.  A ClusterRoleBinding to ClusterRole `cluster-admin` names a User or Group outside `system:` and other than `kube:admin`.

Until then it renders nothing. `kubeadmin-absent` reports NonCompliant for as long as the Secret exists, so the Policy shows what is pending instead of a misleading Compliant.

!!! danger "Check that someone can log in first"
    The element checks that the binding exists, not that it works. A binding to a Group nobody belongs to, or a user whose identity provider is misconfigured, passes. Log in through the identity provider as that cluster-admin before turning the Policy on.

- Clusters using external OIDC authentication have no OAuth identity providers, so `kubeadmin-removal` never deletes the Secret there and the Policy stays NonCompliant. Leave `kubeadmin` off for them.
- The installer's certificate-based `kubeconfig` still works after `kubeadmin` is gone. Keep it as the break-glass credential.
- Clusters built by Hive or the assisted installer keep a copy of the `kubeadmin` password on the hub (`spec.clusterMetadata.adminPasswordSecretRef`). It stops working once the Secret is deleted.
