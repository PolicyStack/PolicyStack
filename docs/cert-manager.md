# cert-manager - Policy Library Documentation

> cert-manager Operator (Red Hat build) for X.509 certificate lifecycle management

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `certManager` | cert-manager Operator (Red Hat build) for X.509 certificate lifecycle management |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection | Default category classifications |
| Controls | CM-2 Baseline Configuration, SC-12 Cryptographic Key Establishment and Management | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `ca` | `True` | Cluster CA issuer. On by default - it is the default TLS issuer for the cluster. |
| `api-cert` | `False` | Replace the external API serving cert (api.<domain>). Additive and SNI-matched. |
| `ingress-cert` | `False` | Replace the default Ingress wildcard cert (*.apps.<domain>). Higher blast radius. |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `caName` | `policystack-ca` | Name of the CA Certificate and the ClusterIssuer that signs from it |
| `caIssuer` | `(dict)` | Issuer/ClusterIssuer to make the CA an intermediate instead. |
| `caDuration` | `17520h0m0s` | CA lifetime. Roots are long-lived; the serving certs they sign rotate. |
| `caRenewBefore` | `2160h0m0s` | Renew the CA this far before expiry |
| `leafDuration` | `8760h` | Lifetime of the serving certificates |
| `leafRenewBefore` | `720h` | Renew serving certificates this far before expiry |
| `keyAlgorithm` | `ECDSA` | Private key algorithm for every certificate issued here |
| `keySize` | `384` | P-384 is FIPS 140-approved and suits FedRAMP High |
| `apiCert` | `(dict)` | API serving certificate settings |
| `ingressCert` | `(dict)` | Ingress wildcard certificate settings |

## Policies

### 📋 Policy: install
> Install and manage the cert-manager Operator

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SC-12 Cryptographic Key Establishment and Management (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: ns-monitoring
> Adds openshift.io/cluster-monitoring label to operator namespace

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-ns-monitoring` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/ns-monitoring-label.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `cert-manager-operator` | Parameter value |

##### Operator Policies

###### 🔧 Operator: cert-manager
> cert-manager Operator (Red Hat build)

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-cert-manager` | Operator policy identifier |
| Namespace | `cert-manager-operator` | Target namespace for operator |
| Display Name | `cert-manager Operator for Red Hat OpenShift` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `openshift-cert-manager-operator` | Operator package name |
| Channel | `stable-v1` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: ca
> Cluster CA ClusterIssuer used as the default TLS issuer

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ca-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `install-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SC-12 Cryptographic Key Establishment and Management (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: ca-issuer
> Cluster CA Certificate and ClusterIssuer

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ca-ca-issuer` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Object count varies with whether a self-signed root is needed |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/ca-issuer.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `caName` | `policystack-ca` | Parameter value |
| `caNamespace` | `cert-manager` | cert-manager's cluster-resource-namespace; the CA secret must live here |


---

### 📋 Policy: api-cert
> cert-manager-managed API server serving certificate

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `api-cert-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `ca-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SC-12 Cryptographic Key Establishment and Management (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: api-certificate
> API server serving certificate (additive, SNI-matched)

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `api-cert-api-certificate` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/api-certificate.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `certName` | `cert-manager-api-cert` | Parameter value |

###### ⚙️ Config: api-ready
> Reports NonCompliant while the API certificate is not Ready

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `api-cert-api-ready` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/api-cert-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `certName` | `cert-manager-api-cert` | Parameter value |


---

### 📋 Policy: ingress-cert
> cert-manager-managed default Ingress wildcard certificate

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ingress-cert-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `ca-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SC-12 Cryptographic Key Establishment and Management (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: ingress-certificate
> Default Ingress wildcard certificate (replaces *.apps)

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ingress-cert-ingress-certificate` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/ingress-certificate.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `certName` | `cert-manager-ingress-cert` | Parameter value |

###### ⚙️ Config: ingress-ready
> Reports NonCompliant while the Ingress certificate is not Ready

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ingress-cert-ingress-ready` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/ingress-cert-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `certName` | `cert-manager-ingress-cert` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 4 |
| Configuration Policies | 6 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **11** |