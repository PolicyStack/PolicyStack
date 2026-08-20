# openshift-data-foundation - Policy Library Documentation

> OpenShift Data Foundation: Ceph storage and the NooBaa object gateway

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `openshiftDataFoundation` | "OpenShift Data Foundation: Ceph storage and the NooBaa object gateway" |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CP Contingency Planning, SC System and Communications Protection | Default category classifications |
| Controls | CP-9 System Backup, SC-28 Protection of Information at Rest | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `noobaa-pvpool` | `False` | PV-pool backing store for NooBaa. Not needed when fronting a cloud object store. |
| `pdb-alert` | `True` | Suppress the permanent NooBaa PodDisruptionBudgetAtLimit alert |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `resourceProfile` | `balanced` | balanced, lean or performance |
| `defaultStorageClass` | `ocs-storagecluster-ceph-rbd` | StorageClass marked default cluster-wide. Every other class is marked non-default. |
| `flexibleScaling` | `None` | count is not a multiple of three. Leave unset to let ODF decide. |
| `csiOnAllNodes` | `True` | Run Ceph CSI node plugins on every node rather than only storage nodes |
| `multiCloudGateway` | `manage` | manage or ignore - whether ODF runs the NooBaa multicloud gateway |
| `deviceSet` | `(dict)` | The disks backing Ceph |
| `encryption` | `(dict)` | Encryption at rest |
| `network` | `(dict)` |  |
| `noobaa` | `(dict)` | NooBaa PV-pool sizing, used when the noobaa-pvpool toggle is on |

## Policies

### 📋 Policy: install
> Install and manage the ODF operator

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
| Categories | CP Contingency Planning, SC System and Communications Protection (default) | Category classifications |
| Controls | CP-9 System Backup, SC-28 Protection of Information at Rest (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: odf
> OpenShift Data Foundation operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-odf` | Operator policy identifier |
| Namespace | `openshift-storage` | Target namespace for operator |
| Display Name | `OpenShift Data Foundation` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `odf-operator` | Operator package name |
| Channel | `stable-4.20` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: cluster
> Deploy the StorageCluster

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `cluster-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `high` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `install-<release>` | `Policy` | `Compliant` |
| `ready-storage-nodes-<cluster>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CP Contingency Planning, SC System and Communications Protection (default) | Category classifications |
| Controls | CP-9 System Backup, SC-28 Protection of Information at Rest (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: storagecluster
> StorageCluster and Ceph CSI placement

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `cluster-storagecluster` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Gating:**

- Waits for operator `odf` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/storage-cluster.yaml` | inherited | Template configuration |
| `converters/ceph-csi-config.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `ocs-storagecluster` | Parameter value |
| `namespace` | `openshift-storage` | Parameter value |


---

### 📋 Policy: ready
> Reports NonCompliant until the StorageCluster is Available

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ready-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `inform` | Other elements depend on this policy by name; it is the "storage is usable" contract |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `cluster-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CP Contingency Planning, SC System and Communications Protection (default) | Category classifications |
| Controls | CP-9 System Backup, SC-28 Protection of Information at Rest (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: available
> StorageCluster readiness

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `ready-available` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/storage-cluster-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `ocs-storagecluster` | Parameter value |
| `namespace` | `openshift-storage` | Parameter value |


---

### 📋 Policy: config
> Default StorageClass, console plugin and NooBaa tuning

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
| `ready-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CP Contingency Planning, SC System and Communications Protection (default) | Category classifications |
| Controls | CP-9 System Backup, SC-28 Protection of Information at Rest (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: storageclass
> Default StorageClass selection

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-storageclass` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | One object per StorageClass the cluster has |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/default-storageclass.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `ocs-storagecluster` | Parameter value |
| `namespace` | `openshift-storage` | Parameter value |

###### ⚙️ Config: console
> ODF console plugin

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-console` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/odf-console-plugin.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `ocs-storagecluster` | Parameter value |
| `namespace` | `openshift-storage` | Parameter value |

###### ⚙️ Config: noobaa-pvpool
> NooBaa PV-pool backing store

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-noobaa-pvpool` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/noobaa-backing-store.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `ocs-storagecluster` | Parameter value |
| `namespace` | `openshift-storage` | Parameter value |

###### ⚙️ Config: pdb-alert
> Suppress the NooBaa PDB alert

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-pdb-alert` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/noobaa-pdb-alert.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `ocs-storagecluster` | Parameter value |
| `namespace` | `openshift-storage` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 4 |
| Configuration Policies | 6 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **11** |