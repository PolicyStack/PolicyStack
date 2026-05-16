# cert-manager - Policy Library Documentation

> cert-manager Operator (Red Hat build) for X.509 certificate lifecycle management

*Generated: 2026-05-08 20:16:05*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `certManager` | cert-manager Operator (Red Hat build) for X.509 certificate lifecycle management |
| Enabled | `False` | Whether this component is enabled |

## Policies

### 📋 Policy: cert-manager-install
> Install and manage the cert-manager Operator

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `cert-manager-install-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: cert-manager-ns-monitoring
> Adds openshift.io/cluster-monitoring label to operator namespace

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `cert-manager-install-cert-manager-ns-monitoring` | Configuration policy identifier |
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
| Name | `cert-manager-install-cert-manager` | Operator policy identifier |
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

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 1 |
| Configuration Policies | 1 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **3** |