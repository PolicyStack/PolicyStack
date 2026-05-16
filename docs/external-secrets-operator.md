# external-secrets-operator - Policy Library Documentation

> External Secrets Operator (Red Hat) for syncing secrets from external KMS providers

*Generated: 2026-05-08 20:16:05*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `externalSecretsOperator` | External Secrets Operator (Red Hat) for syncing secrets from external KMS providers |
| Enabled | `False` | Whether this component is enabled |

## Policies

### 📋 Policy: eso-install
> Install and manage the External Secrets Operator

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `eso-install-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: external-secrets-ns-monitoring
> Adds openshift.io/cluster-monitoring label to operator namespace

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `eso-install-external-secrets-ns-monitoring` | Configuration policy identifier |
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
| `namespace` | `external-secrets-operator` | Parameter value |

##### Operator Policies

###### 🔧 Operator: external-secrets
> External Secrets Operator (Red Hat build)

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `eso-install-external-secrets` | Operator policy identifier |
| Namespace | `external-secrets-operator` | Target namespace for operator |
| Display Name | `External Secrets Operator` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `openshift-external-secrets-operator` | Operator package name |
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