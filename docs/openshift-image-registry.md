# openshift-image-registry - Policy Library Documentation

> Internal image registry storage and configuration

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `openshiftImageRegistry` | Internal image registry storage and configuration |
| Enabled | `False` | Master control to enable/disable all policies in this element |

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
| `pvc` | `False` | Back the registry with an RWX PersistentVolumeClaim |
| `s3` | `False` | Back the registry with a NooBaa bucket from ODF |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `storageType` | `s3` | Must match the toggle you enabled above: pvc or s3 |
| `managementState` | `Managed` | Managed, Unmanaged or Removed |
| `replicas` | `2` | Registry replicas. Use at least 2 with RWX or S3 storage. |
| `rolloutStrategy` | `RollingUpdate` | RollingUpdate is only safe with RWX or S3; use Recreate with RWO |
| `pvc` | `(dict)` | Used when storageType is pvc |
| `s3` | `(dict)` | Used when storageType is s3 |

## Policies

### 📋 Policy: pvc
> PVC-backed registry storage

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `pvc-<release>` | Full policy name including release |
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

###### ⚙️ Config: claim
> Registry PVC

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `pvc-claim` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/registry-pvc.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-image-registry` | Parameter value |
| `claimName` | `image-registry-storage` | Parameter value |
| `trustBundleName` | `image-registry-s3-bundle` | Parameter value |


---

### 📋 Policy: s3
> S3-backed registry storage from ODF

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `s3-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
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
> Registry bucket

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `s3-bucket` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/registry-bucket.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-image-registry` | Parameter value |
| `claimName` | `image-registry-storage` | Parameter value |
| `trustBundleName` | `image-registry-s3-bundle` | Parameter value |

###### ⚙️ Config: bucket-ready
> Registry bucket is provisioned

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `s3-bucket-ready` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/registry-bucket-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-image-registry` | Parameter value |
| `claimName` | `image-registry-storage` | Parameter value |
| `trustBundleName` | `image-registry-s3-bundle` | Parameter value |

###### ⚙️ Config: credentials
> Registry S3 credentials and trust bundle

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `s3-credentials` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Gating:**

- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `s3-bucket-ready` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/registry-s3-secret.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-image-registry` | Parameter value |
| `claimName` | `image-registry-storage` | Parameter value |
| `trustBundleName` | `image-registry-s3-bundle` | Parameter value |


---

### 📋 Policy: config
> Point the registry at its storage and enable it

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

###### ⚙️ Config: registry
> Image registry configuration

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-registry` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/registry-config.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-image-registry` | Parameter value |
| `claimName` | `image-registry-storage` | Parameter value |
| `trustBundleName` | `image-registry-s3-bundle` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 3 |
| Configuration Policies | 5 |
| Operator Policies | 0 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **8** |