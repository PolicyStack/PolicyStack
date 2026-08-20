# loki - Policy Library Documentation

> Loki log storage, the log forwarder and the console logging UI

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `loki` | Loki log storage, the log forwarder and the console logging UI |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | AU Audit and Accountability | Default category classifications |
| Controls | AU-6 Audit Review, Analysis, and Reporting, AU-9 Protection of Audit Information | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `forwarder` | `False` | Forward cluster logs into Loki. Requires the openshift-logging element. |
| `observability-ui` | `False` | Add the Logging view to the console. Requires the cluster-observability element. |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `size` | `1x.extra-small` | 1x.demo, 1x.pico, 1x.extra-small, 1x.small or 1x.medium |
| `storageClass` | `gp3-csi` | StorageClass for Loki's own PVCs (indexes and caches, not the object store) |
| `ingestionRate` | `8` | Sustained ingestion in MB/s per tenant |
| `ingestionBurstSize` | `16` | Burst allowance in MB |
| `queryTimeout` | `3m` | How long a query may run before it is cut off |
| `schemaEffectiveDate` | `2024-09-22` | Schema start date. Only change this when migrating schema versions. |
| `forwardedLogTypes` | `(list)` | Log types forwarded into Loki: application, infrastructure and/or audit |
| `collectorRoles` | `(list)` | ClusterRoles the collector needs to read those log types |

## Policies

### 📋 Policy: install
> Install and manage the Loki operator

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
| Categories | AU Audit and Accountability (default) | Category classifications |
| Controls | AU-6 Audit Review, Analysis, and Reporting, AU-9 Protection of Audit Information (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: loki
> Loki operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-loki` | Operator policy identifier |
| Namespace | `openshift-operators-redhat` | Target namespace for operator |
| Display Name | `Loki Operator` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `loki-operator` | Operator package name |
| Channel | `stable-6.5` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: stack
> Deploy the LokiStack and its object storage

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `stack-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `install-<release>` | `Policy` | `Compliant` |
| `ready-openshift-data-foundation-<cluster>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | AU Audit and Accountability (default) | Category classifications |
| Controls | AU-6 Audit Review, Analysis, and Reporting, AU-9 Protection of Audit Information (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: bucket
> Object storage bucket and the secret LokiStack reads it through

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `stack-bucket` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/loki-bucketclaim.yaml` | inherited | Template configuration |
| `converters/loki-bucket-secret.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-logging` | Parameter value |
| `stackName` | `logging-lokistack` | Parameter value |
| `bucketName` | `loki-bucket-odf` | Parameter value |
| `bucketStorageClass` | `openshift-storage.noobaa.io` | Parameter value |
| `storageSecret` | `logging-loki-odf` | Parameter value |
| `collectorName` | `collector` | Parameter value |
| `forwarderName` | `logging` | Parameter value |

###### ⚙️ Config: lokistack
> LokiStack

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `stack-lokistack` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Waits for operator `loki` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `stack-bucket` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/loki-stack.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-logging` | Parameter value |
| `stackName` | `logging-lokistack` | Parameter value |
| `bucketName` | `loki-bucket-odf` | Parameter value |
| `bucketStorageClass` | `openshift-storage.noobaa.io` | Parameter value |
| `storageSecret` | `logging-loki-odf` | Parameter value |
| `collectorName` | `collector` | Parameter value |
| `forwarderName` | `logging` | Parameter value |


---

### 📋 Policy: forwarder
> Forward cluster logs into Loki

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `forwarder-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `stack-<release>` | `Policy` | `Compliant` |
| `install-openshift-logging-<cluster>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | AU Audit and Accountability (default) | Category classifications |
| Controls | AU-6 Audit Review, Analysis, and Reporting, AU-9 Protection of Audit Information (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: collector
> Collector ServiceAccount

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `forwarder-collector` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/collector-sa.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-logging` | Parameter value |
| `stackName` | `logging-lokistack` | Parameter value |
| `bucketName` | `loki-bucket-odf` | Parameter value |
| `bucketStorageClass` | `openshift-storage.noobaa.io` | Parameter value |
| `storageSecret` | `logging-loki-odf` | Parameter value |
| `collectorName` | `collector` | Parameter value |
| `forwarderName` | `logging` | Parameter value |

###### ⚙️ Config: collector-rbac
> Collector ClusterRoleBindings

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `forwarder-collector-rbac` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | One binding per configured collector role |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/collector-rbac.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-logging` | Parameter value |
| `stackName` | `logging-lokistack` | Parameter value |
| `bucketName` | `loki-bucket-odf` | Parameter value |
| `bucketStorageClass` | `openshift-storage.noobaa.io` | Parameter value |
| `storageSecret` | `logging-loki-odf` | Parameter value |
| `collectorName` | `collector` | Parameter value |
| `forwarderName` | `logging` | Parameter value |

###### ⚙️ Config: clusterlogforwarder
> ClusterLogForwarder

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `forwarder-clusterlogforwarder` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `forwarder-collector-rbac` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/log-forwarder.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-logging` | Parameter value |
| `stackName` | `logging-lokistack` | Parameter value |
| `bucketName` | `loki-bucket-odf` | Parameter value |
| `bucketStorageClass` | `openshift-storage.noobaa.io` | Parameter value |
| `storageSecret` | `logging-loki-odf` | Parameter value |
| `collectorName` | `collector` | Parameter value |
| `forwarderName` | `logging` | Parameter value |


---

### 📋 Policy: observability-ui
> Logging view in the OpenShift console

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `observability-ui-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `low` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `forwarder-<release>` | `Policy` | `Compliant` |
| `install-cluster-observability-<cluster>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | AU Audit and Accountability (default) | Category classifications |
| Controls | AU-6 Audit Review, Analysis, and Reporting, AU-9 Protection of Audit Information (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: uiplugin
> Console logging UI plugin

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `observability-ui-uiplugin` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/logging-ui-plugin.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-logging` | Parameter value |
| `stackName` | `logging-lokistack` | Parameter value |
| `bucketName` | `loki-bucket-odf` | Parameter value |
| `bucketStorageClass` | `openshift-storage.noobaa.io` | Parameter value |
| `storageSecret` | `logging-loki-odf` | Parameter value |
| `collectorName` | `collector` | Parameter value |
| `forwarderName` | `logging` | Parameter value |


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