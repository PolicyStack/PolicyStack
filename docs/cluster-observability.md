# cluster-observability - Policy Library Documentation

> Cluster Observability Operator for monitoring, logging and tracing UIs

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `clusterObservability` | Cluster Observability Operator for monitoring, logging and tracing UIs |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management | Default category classifications |
| Controls | CM-2 Baseline Configuration | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Policies

### 📋 Policy: install
> Install and manage Cluster Observability Operator

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
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: ns-monitoring
> Adds openshift.io/cluster-monitoring label to the operator namespace

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
| `namespace` | `openshift-cluster-observability-operator` | Parameter value |

##### Operator Policies

###### 🔧 Operator: coo
> Cluster Observability Operator for monitoring, logging and tracing UIs

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-coo` | Operator policy identifier |
| Namespace | `openshift-cluster-observability-operator` | Target namespace for operator |
| Display Name | `Cluster Observability Operator` | Must match the CSV displayName so the generated status check can find it |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Pin upgrades by listing approved CSVs under `versions:` instead of changing this |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `cluster-observability-operator` | Operator package name |
| Channel | `stable` | Update channel |
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