# advanced-cluster-management - Policy Library Documentation

> Advanced Cluster Management hub: MultiClusterHub, observability and search

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `advancedClusterManagement` | "Advanced Cluster Management hub: MultiClusterHub, observability and search" |
| Enabled | `False` | element - enable it in values/acm/*.yaml. |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management | Default category classifications |
| Controls | CM-2 Baseline Configuration | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `observability` | `False` | Long-term metric storage via Thanos. Requires ODF on the same cluster. |
| `search` | `False` | Persistent storage for ACM Search |
| `addons` | `False` | Tune the policy addons pushed to managed clusters |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `availabilityConfig` | `High` | High or Basic. Basic runs single replicas and suits lab hubs. |
| `selfManaged` | `True` | Whether the hub also manages itself as `local-cluster` |
| `siteConfig` | `False` | Enable the SiteConfig component, needed for cluster provisioning |
| `search` | `(dict)` | ACM Search database storage |
| `observability` | `(dict)` | Observability sizing |
| `addons` | `(dict)` | stock concurrency and rate limits, which are the usual bottleneck at scale. |

## Policies

### 📋 Policy: install
> Install and manage the ACM operator

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
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: acm
> Advanced Cluster Management operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-acm` | Operator policy identifier |
| Namespace | `open-cluster-management` | Target namespace for operator |
| Display Name | `Advanced Cluster Management for Kubernetes` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `advanced-cluster-management` | Operator package name |
| Channel | `release-2.17` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: hub
> Deploy the MultiClusterHub

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `hub-<release>` | Full policy name including release |
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
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: mch
> MultiClusterHub

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `hub-mch` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Gating:**

- Waits for operator `acm` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/multiclusterhub.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `multiclusterhub` | Parameter value |
| `namespace` | `open-cluster-management` | Parameter value |
| `observabilityNamespace` | `open-cluster-management-observability` | Parameter value |
| `observabilityBucket` | `acm-observability` | Parameter value |


---

### 📋 Policy: observability
> Long-term metric storage

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `observability-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `hub-<release>` | `Policy` | `Compliant` |
| `ready-openshift-data-foundation-<cluster>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: bucket
> Observability namespace and Thanos bucket

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `observability-bucket` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acm-observability-ns.yaml` | inherited | Template configuration |
| `converters/acm-observability-bucket.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `multiclusterhub` | Parameter value |
| `namespace` | `open-cluster-management` | Parameter value |
| `observabilityNamespace` | `open-cluster-management-observability` | Parameter value |
| `observabilityBucket` | `acm-observability` | Parameter value |

###### ⚙️ Config: mco
> MultiClusterObservability and its object storage config

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `observability-mco` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Gating:**

- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `observability-bucket` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acm-observability.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `multiclusterhub` | Parameter value |
| `namespace` | `open-cluster-management` | Parameter value |
| `observabilityNamespace` | `open-cluster-management-observability` | Parameter value |
| `observabilityBucket` | `acm-observability` | Parameter value |


---

### 📋 Policy: search
> ACM Search persistent storage

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `search-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `hub-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: search-storage
> Search database storage

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `search-search-storage` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acm-search.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `multiclusterhub` | Parameter value |
| `namespace` | `open-cluster-management` | Parameter value |
| `observabilityNamespace` | `open-cluster-management-observability` | Parameter value |
| `observabilityBucket` | `acm-observability` | Parameter value |


---

### 📋 Policy: addons
> Policy addon tuning

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `addons-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `hub-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: addon-tuning
> Policy addon concurrency and resources

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `addons-addon-tuning` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Two objects per configured addon |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acm-addon-tuning.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `name` | `multiclusterhub` | Parameter value |
| `namespace` | `open-cluster-management` | Parameter value |
| `observabilityNamespace` | `open-cluster-management-observability` | Parameter value |
| `observabilityBucket` | `acm-observability` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 5 |
| Configuration Policies | 5 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **11** |