# openshift-logging - Policy Library Documentation

> Element for the OpenShift Logging. This installs and configures OpenShift Logging.

*Generated: 2025-09-09 16:48:44*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `openshiftLogging` | OpenShift Logging element |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Values

Default configuration applied to all policies unless explicitly overridden

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management | Categories for organizing policies in ACM console and reports |
| Controls | CM-2 Baseline Configuration | Specific security controls addressed by these policies |
| Standards | NIST SP 800-53 | Compliance standards and frameworks |

## Policies

### 📋 Policy: openshift-logging-operator
> Policy for any operator installation

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `openshift-logging-operator-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Set to enforce as, despite enforce not working on OperatorPolicy objects, there are ConfigPolicies we will need to enforce. |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: openshift-logging
> Installs the OpenShift logging operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `openshift-logging-operator-openshift-logging` | Operator policy identifier |
| Namespace | `openshift-logging` | Target namespace for operator installation |
| Display Name | `Red Hat OpenShift Logging` | Human-friendly display name in OLM |
| Compliance Type | `musthave` | Operator must be present |
| Remediation | `enforce` | Automatically install and configure |
| Severity | `high` | Could be a high or medium severity depending on the security requirements. |
| Upgrade Approval | `Automatic` | Approval strategy for operator updates (Automatic/Manual) |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `openshift-logging` | Operator package name in catalog |
| Channel | `stable-6.2` | Update channel |
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