# openshift-compliance-operator - Policy Library Documentation

> Compliance Operator running STIG scans and applying remediations

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `openshiftComplianceOperator` | Compliance Operator running STIG scans and applying remediations |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CA Security Assessment and Authorization | Default category classifications |
| Controls | CA-2 Control Assessments, CA-7 Continuous Monitoring | Default control mappings |
| Standards | NIST SP 800-53, DISA STIG | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `remediate` | `False` | Apply the ComplianceRemediations listed in config.autoRemediate |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `profiles` | `(list)` | Profiles bound to the scan |
| `rawResultStorage` | `(dict)` | Where raw scan results are persisted |
| `autoRemediate` | `(list)` | ComplianceRemediation names to apply when the remediate toggle is on. Keeping the list in values rather than an out-of-band ConfigMap makes it reviewable in git. |

## Policies

### 📋 Policy: install
> Install and manage the Compliance Operator

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
| Categories | CA Security Assessment and Authorization (default) | Category classifications |
| Controls | CA-2 Control Assessments, CA-7 Continuous Monitoring (default) | Control mappings |
| Standards | NIST SP 800-53, DISA STIG (default) | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: compliance
> OpenShift Compliance Operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-compliance` | Operator policy identifier |
| Namespace | `openshift-compliance` | Target namespace for operator |
| Display Name | `Compliance Operator` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `compliance-operator` | Operator package name |
| Channel | `stable` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: scan
> Configure and bind the STIG scan

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `scan-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `high` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `install-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CA Security Assessment and Authorization (default) | Category classifications |
| Controls | CA-2 Control Assessments, CA-7 Continuous Monitoring (default) | Control mappings |
| Standards | NIST SP 800-53, DISA STIG (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: setting
> Default ScanSetting

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `scan-setting` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Waits for operator `compliance` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/scan-setting.yaml` | inherited | Template configuration |
| `converters/agg-patch-role.yaml` | inherited | Template configuration |
| `converters/agg-patch-rolebinding.yaml` | inherited | Template configuration |
| `converters/scan-binding.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-compliance` | Parameter value |
| `scanSetting` | `default` | Parameter value |
| `binding` | `stig` | Parameter value |


---

### 📋 Policy: ready
> Reports NonCompliant until the scan suite has finished

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ready-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `inform` | A status can only be observed, so this policy never enforces |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `scan-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CA Security Assessment and Authorization (default) | Category classifications |
| Controls | CA-2 Control Assessments, CA-7 Continuous Monitoring (default) | Control mappings |
| Standards | NIST SP 800-53, DISA STIG (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: suite
> Scan suite completion

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ready-suite` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/suite-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-compliance` | Parameter value |
| `binding` | `stig` | Parameter value |


---

### 📋 Policy: remediate
> Apply the configured compliance remediations

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `remediate-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `high` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `ready-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CA Security Assessment and Authorization (default) | Category classifications |
| Controls | CA-2 Control Assessments, CA-7 Continuous Monitoring (default) | Control mappings |
| Standards | NIST SP 800-53, DISA STIG (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: remediations
> Applied ComplianceRemediations

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `remediate-remediations` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |
| Raw Template | Enabled | One object per entry in config.autoRemediate |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/auto-remediate.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-compliance` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 4 |
| Configuration Policies | 3 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **8** |