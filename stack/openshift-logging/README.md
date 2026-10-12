# openshift-logging - Policy Library Documentation

> Element for the OpenShift Logging. This installs and configures OpenShift Logging.

Field reference: [policy-library](https://github.com/PolicyStack/PolicyStack-chart/tree/main/charts/policy-library)

*Generated: 2026-10-12 18:31:18*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `openshiftLogging` | OpenShift Logging element |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management | Categories for organizing policies in ACM console and reports |
| Controls | CM-2 Baseline Configuration | Specific security controls addressed by these policies |
| Standards | NIST SP 800-53 | Compliance standards and frameworks |

## Policies

### 📋 Policy: install
> Policy for any operator installation

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Set on the Policy, so it overrides every template's action, including the OperatorPolicy below. With inform, the operator is reported missing, not installed. |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: cluster-logging
> Installs the OpenShift logging operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-cluster-logging` | Operator policy identifier |
| Namespace | `openshift-logging` | Target namespace for operator installation |
| Display Name | `Red Hat OpenShift Logging` | Human-friendly display name in OLM |
| Compliance Type | `musthave` | Operator must be present |
| Remediation | `enforce` | Automatically install and configure |
| Severity | `high` | High or medium, depending on security requirements |
| Upgrade Approval | `Automatic` | Automatic approves upgrades allowed by versions; None approves the initial install only, never upgrades |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `cluster-logging` | Operator package name in catalog |
| Channel | `stable-6.5` | Update channel |
| Source | `redhat-operators` | Catalog source name |
| Source Namespace | `openshift-marketplace` | Namespace containing the catalog |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 1 |
| Configuration Policies | 0 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **2** |