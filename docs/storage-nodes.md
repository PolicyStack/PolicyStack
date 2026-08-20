# storage-nodes - Policy Library Documentation

> Dedicated storage nodes for OpenShift Data Foundation

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `storageNodes` | Dedicated storage nodes for OpenShift Data Foundation |
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
| `aws` | `False` | Create storage MachineSets by cloning the worker MachineSet (AWS) |
| `vmware` | `False` | Clone each worker MachineSet per vSphere failure domain |
| `baremetal` | `False` | Label existing nodes as storage nodes (no MachineSets on bare metal) |
| `ready` | `True` | Report NonCompliant until the storage nodes are ready |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `replicas` | `3` | Storage nodes per zone. ODF expects three for a supported topology. |
| `zones` | `(list)` | Availability zones. Empty means "wherever the workers are". |
| `instanceType` | `` | AWS instance type. Empty inherits from the worker MachineSet. |
| `volumeSize` | `` | Root volume size in GiB. Empty inherits from the worker MachineSet. |
| `numCPUs` | `` | vSphere sizing. Empty inherits from the worker MachineSet. |
| `numCoresPerSocket` | `` |  |
| `memoryMiB` | `` |  |
| `nodes` | `(list)` | `replicas` nodes instead. |

## Policies

### 📋 Policy: aws
> Storage MachineSets on AWS

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `aws-<release>` | Full policy name including release |
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

###### ⚙️ Config: machinesets-aws
> Storage MachineSets cloned from the worker MachineSet

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
| `role` | `storage` | Parameter value |
| `namespace` | `openshift-machine-api` | Parameter value |
| `nodeLabels` | `{'node-role.kubernetes.io/infra': '', 'cluster.ocs.openshift.io/openshift-storage': ''}` | Parameter value |


---

### 📋 Policy: vmware
> Storage MachineSets on vSphere

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `vmware-<release>` | Full policy name including release |
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

###### ⚙️ Config: machinesets-vmware
> Storage MachineSets per vSphere failure domain

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
| `role` | `storage` | Parameter value |
| `namespace` | `openshift-machine-api` | Parameter value |
| `nodeLabels` | `{'node-role.kubernetes.io/infra': '', 'cluster.ocs.openshift.io/openshift-storage': ''}` | Parameter value |


---

### 📋 Policy: baremetal
> Storage node labels on bare metal

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baremetal-<release>` | Full policy name including release |
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

###### ⚙️ Config: labels
> Storage node labels

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baremetal-labels` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/storage-label-nodes.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `role` | `storage` | Parameter value |
| `namespace` | `openshift-machine-api` | Parameter value |
| `nodeLabels` | `{'node-role.kubernetes.io/infra': '', 'cluster.ocs.openshift.io/openshift-storage': ''}` | Parameter value |


---

### 📋 Policy: ready
> Reports NonCompliant until the storage nodes are ready

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
> Storage MachineSet readiness

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
| `role` | `storage` | Parameter value |
| `namespace` | `openshift-machine-api` | Parameter value |
| `nodeLabels` | `{'node-role.kubernetes.io/infra': '', 'cluster.ocs.openshift.io/openshift-storage': ''}` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 4 |
| Configuration Policies | 4 |
| Operator Policies | 0 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **8** |