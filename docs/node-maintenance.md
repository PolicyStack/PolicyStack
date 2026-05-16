# node-maintenance - Policy Library Documentation

> Node Maintenance Operator for safely cordoning and draining nodes

*Generated: 2026-05-08 20:16:05*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `nodeMaintenance` | Node Maintenance Operator for safely cordoning and draining nodes |
| Enabled | `False` | Whether this component is enabled |

## Policies

### 📋 Policy: node-maintenance-install
> Install and manage the Node Maintenance Operator

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `node-maintenance-install-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: node-maintenance-ns-monitoring
> Adds openshift.io/cluster-monitoring label to operator namespace

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `node-maintenance-install-node-maintenance-ns-monitoring` | Configuration policy identifier |
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
| `namespace` | `openshift-workload-availability` | Parameter value |

##### Operator Policies

###### 🔧 Operator: node-maintenance
> Node Maintenance Operator (Workload Availability)

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `node-maintenance-install-node-maintenance` | Operator policy identifier |
| Namespace | `openshift-workload-availability` | Target namespace for operator |
| Display Name | `Node Maintenance Operator` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `node-maintenance-operator` | Operator package name |
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