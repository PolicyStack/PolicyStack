# machine-health-checks - Policy Library Documentation

> MachineHealthChecks for automatic replacement of unhealthy nodes

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `machineHealthChecks` | MachineHealthChecks for automatic replacement of unhealthy nodes |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CP Contingency Planning | Default category classifications |
| Controls | CP-10 System Recovery and Reconstitution | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `worker` | `(dict)` |  |
| `infra` | `(dict)` |  |
| `storage` | `(dict)` |  |
| `default` | `(dict)` |  |

## Policies

### 📋 Policy: config
> MachineHealthChecks per MachineSet

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
| Categories | CP Contingency Planning (default) | Category classifications |
| Controls | CP-10 System Recovery and Reconstitution (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: checks
> One MachineHealthCheck per discovered MachineSet

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-checks` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Object count depends on how many MachineSets the cluster has |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/machine-health-checks.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-machine-api` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 1 |
| Configuration Policies | 1 |
| Operator Policies | 0 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **2** |