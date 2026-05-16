# openshift-pipelines - Policy Library Documentation

> OpenShift Pipelines (Tekton) Operator

*Generated: 2026-05-08 20:16:05*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `openshiftPipelines` | OpenShift Pipelines (Tekton) Operator |
| Enabled | `False` | Whether this component is enabled |

## Policies

### 📋 Policy: pipelines-install
> Install and manage the OpenShift Pipelines (Tekton) Operator

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `pipelines-install-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: openshift-pipelines
> OpenShift Pipelines (Tekton)

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `pipelines-install-openshift-pipelines` | Operator policy identifier |
| Namespace | `openshift-operators` | Target namespace for operator |
| Display Name | `Red Hat OpenShift Pipelines` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `openshift-pipelines-operator-rh` | Operator package name |
| Channel | `pipelines-1.21` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


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