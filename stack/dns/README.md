# dns - Policy Library Documentation

> Cluster DNS operand configuration

Field reference: [policy-library](https://github.com/PolicyStack/PolicyStack-chart/tree/main/charts/policy-library)

*Generated: 2026-10-16 02:34:19*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `dns` | Cluster DNS operand configuration |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management | Default category classifications |
| Controls | CM-2 Baseline Configuration | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `tolerations` | `(list)` | Tolerations for the DNS daemonset. The default tolerates every taint so that workloads on infra, storage and master nodes keep cluster DNS. |

## Policies

### 📋 Policy: config
> Cluster DNS node placement

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
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: tolerations
> DNS daemonset tolerations

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-tolerations` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/dns-tolerations.yaml` | inherited | Template configuration |


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