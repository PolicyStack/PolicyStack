# user-workload-monitoring - Policy Library Documentation

> User Workload Monitoring: Prometheus, Thanos Ruler and Alertmanager for application metrics

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `userWorkloadMonitoring` | User Workload Monitoring: Prometheus, Thanos Ruler and Alertmanager for application metrics |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | AU Audit and Accountability | Default category classifications |
| Controls | AU-6 Audit Review, Analysis, and Reporting | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `storageClass` | `` | Default StorageClass for every UWM component. Omit to use the cluster default. |
| `prometheus` | `(dict)` |  |
| `thanosRuler` | `(dict)` |  |
| `alertmanager` | `(dict)` |  |

## Policies

### 📋 Policy: config
> Enable and configure User Workload Monitoring

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | AU Audit and Accountability (default) | Category classifications |
| Controls | AU-6 Audit Review, Analysis, and Reporting (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: enable
> Enable UWM and write its configuration

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-enable` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Merges into an existing ConfigMap read on the managed cluster |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/uwm-enable.yaml` | inherited | Template configuration |


---

### 📋 Policy: ready
> Reports NonCompliant until the UWM StatefulSets are ready

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
| `config-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | AU Audit and Accountability (default) | Category classifications |
| Controls | AU-6 Audit Review, Analysis, and Reporting (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: statefulsets
> UWM StatefulSet readiness

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ready-statefulsets` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/uwm-ready.yaml` | inherited | Template configuration |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 2 |
| Configuration Policies | 2 |
| Operator Policies | 0 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **4** |