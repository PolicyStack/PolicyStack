# nfd - Policy Library Documentation

> Node Feature Discovery Operator and instance for hardware feature labelling

Field reference: [policy-library](https://github.com/PolicyStack/PolicyStack-chart/tree/main/charts/policy-library)

*Generated: 2026-10-16 02:34:20*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `nfd` | Node Feature Discovery Operator and instance for hardware feature labelling |
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
> Install and configure the Node Feature Discovery Operator and instance

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

###### ⚙️ Config: nfd-ns-monitoring
> Adds openshift.io/cluster-monitoring label to operator namespace

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-nfd-ns-monitoring` | Configuration policy identifier |
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
| `namespace` | `openshift-nfd` | Parameter value |

###### ⚙️ Config: nfd-instance
> Deploy the NodeFeatureDiscovery custom resource

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-nfd-instance` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Waits for operator `node-feature-discovery` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/nfd-instance.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `operandImage` | `registry.redhat.io/openshift4/ose-node-feature-discovery-rhel9:v4.20` | Parameter value |

##### Operator Policies

###### 🔧 Operator: node-feature-discovery
> Node Feature Discovery Operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-node-feature-discovery` | Operator policy identifier |
| Namespace | `openshift-nfd` | Target namespace for operator |
| Display Name | `Node Feature Discovery Operator` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `nfd` | Operator package name |
| Channel | `stable` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 1 |
| Configuration Policies | 2 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **4** |