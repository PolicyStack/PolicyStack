# infra-nodes - Policy Library Documentation

> Dedicated infra nodes, and moving platform workloads onto them

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `infraNodes` | Dedicated infra nodes, and moving platform workloads onto them |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management | Default category classifications |
| Controls | CM-6 Configuration Settings | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `mcp` | `True` | MachineConfigPool for the infra role |
| `config` | `True` | Kubelet configuration for the infra pool |
| `aws` | `False` | Create infra MachineSets by cloning the worker MachineSet (AWS) |
| `vmware` | `False` | Clone each worker MachineSet per vSphere failure domain |
| `ready` | `True` | Report NonCompliant until the infra nodes are ready |
| `workloads` | `False` | Move gitops, ingress, image registry and monitoring onto infra nodes |
| `restore` | `False` | turn it off again once the components have moved. |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `replicas` | `3` | Infra nodes per zone |
| `zones` | `(list)` | Availability zones. Empty means "wherever the workers are". |
| `instanceType` | `` | AWS instance type. Empty inherits from the worker MachineSet. |
| `volumeSize` | `` | Root volume size in GiB. Empty inherits from the worker MachineSet. |
| `numCPUs` | `` | vSphere sizing. Empty inherits from the worker MachineSet. |
| `numCoresPerSocket` | `` |  |
| `memoryMiB` | `` |  |
| `gitopsNamespace` | `openshift-gitops` | Namespace the GitopsService lives in |
| `maxPods` | `250` | Kubelet reservations for infra nodes |
| `systemReservedMemory` | `2Gi` |  |
| `systemReservedCpu` | `500m` |  |
| `kubeReservedMemory` | `1Gi` |  |
| `kubeReservedCpu` | `500m` |  |
| `evictionMemory` | `500Mi` |  |

## Policies

### 📋 Policy: mcp
> MachineConfigPool for infra nodes

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `mcp-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: pool
> Infra MachineConfigPool

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `mcp-pool` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/infra-mcp.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `infra` | Parameter value |


---

### 📋 Policy: config
> Infra kubelet configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `mcp-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: kubelet
> KubeletConfig for the infra pool

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-kubelet` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/node-kubelet-config.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `infra` | Parameter value |


---

### 📋 Policy: aws
> Infra MachineSets on AWS

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `aws-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `mcp-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: machinesets-aws
> Infra MachineSets cloned from the worker MachineSet

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `aws-machinesets-aws` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/machineset-aws.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `infra` | Parameter value |
| `namespace` | `openshift-machine-api` | Parameter value |
| `nodeLabels` | `{'node-role.kubernetes.io/infra': ''}` | Parameter value |


---

### 📋 Policy: vmware
> Infra MachineSets on vSphere

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `vmware-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `mcp-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: machinesets-vmware
> Infra MachineSets per vSphere failure domain

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `vmware-machinesets-vmware` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/machineset-vmware.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `infra` | Parameter value |
| `namespace` | `openshift-machine-api` | Parameter value |
| `nodeLabels` | `{'node-role.kubernetes.io/infra': ''}` | Parameter value |


---

### 📋 Policy: ready
> Reports NonCompliant until the infra nodes are ready

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ready-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `inform` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: nodes-ready
> Infra MachineSet readiness

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ready-nodes-ready` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/machineset-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `infra` | Parameter value |
| `namespace` | `openshift-machine-api` | Parameter value |
| `nodeLabels` | `{'node-role.kubernetes.io/infra': ''}` | Parameter value |


---

### 📋 Policy: workloads
> Move platform components onto infra nodes

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `workloads-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `ready-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: components
> Gitops, ingress and image registry pinned to infra

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `workloads-components` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/infra-gitops.yaml` | inherited | Template configuration |
| `converters/infra-ingress.yaml` | inherited | Template configuration |
| `converters/infra-registry.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `infra` | Parameter value |

###### ⚙️ Config: monitoring
> Monitoring stack pinned to infra

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `workloads-monitoring` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Merges into the existing cluster-monitoring-config rather than replacing it |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/infra-monitoring.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `infra` | Parameter value |
| `components` | `['alertmanagerMain', 'prometheusK8s', 'prometheusOperator', 'kubeStateMetrics', 'telemeterClient', 'openshiftStateMetrics', 'thanosQuerier', 'monitoringPlugin']` | Monitoring components to pin. Alertmanager and Prometheus are the heavy ones. |


---

### 📋 Policy: restore
> Move platform components back off infra nodes

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `restore-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: revert
> Platform components moved back off infra

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `restore-revert` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/infra-restore.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `infra` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 7 |
| Configuration Policies | 8 |
| Operator Policies | 0 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **15** |