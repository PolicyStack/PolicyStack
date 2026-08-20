# workload-partitioning - Policy Library Documentation

> PerformanceProfile pinning platform workloads to reserved CPUs

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `workloadPartitioning` | PerformanceProfile pinning platform workloads to reserved CPUs |
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
| `reservedCpus` | `` | CPUs reserved for platform/infrastructure workloads, e.g. "0-1" |
| `isolatedCpus` | `` | CPUs left isolated for application workloads, e.g. "2-31" |
| `nodeSelector` | `(dict)` | Nodes the profile applies to. Defaults to control-plane nodes. |
| `machineConfigPoolSelector` | `(dict)` | MachineConfigPool the profile applies to. Defaults to nodeSelector, then master. |
| `numaTopologyPolicy` | `` | Kubelet NUMA topology policy, e.g. single-numa-node |
| `realTimeKernel` | `False` | Install the real-time kernel |
| `globallyDisableIrqLoadBalancing` | `False` | Disable IRQ load balancing across the isolated set |
| `hugepages` | `(dict)` | Hugepage allocation, e.g. {defaultSize: 1G, pages: [{size: 1G, count: 4}]} |

## Policies

### 📋 Policy: config
> Workload partitioning PerformanceProfile

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

###### ⚙️ Config: performance-profile
> PerformanceProfile for workload partitioning

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-performance-profile` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/performance-profile.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `workload-partitioning` | Parameter value |


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