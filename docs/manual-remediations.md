# manual-remediations - Policy Library Documentation

> Opinionated cluster remediations applied as policy

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `manualRemediations` | Opinionated cluster remediations applied as policy |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SI System and Information Integrity | Default category classifications |
| Controls | CM-2 Baseline Configuration, SI-7 Software, Firmware, and Information Integrity | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `registries` | `True` | Restrict the registries the cluster may pull and import from |
| `remove-samples` | `True` | Remove the Samples Operator and its imagestreams |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `allowedRegistriesForImport` | `(list)` | Registries permitted for `oc import-image` |
| `allowedRegistries` | `(list)` | Registries the cluster may pull from at runtime |

## Policies

### 📋 Policy: registries
> Restrict image registries the cluster may use

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `registries-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `high` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SI System and Information Integrity (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SI-7 Software, Firmware, and Information Integrity (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: allowlist
> Image registry allowlist

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `registries-allowlist` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/allowed-image-registries.yaml` | inherited | Template configuration |


---

### 📋 Policy: remove-samples
> Remove the Samples Operator

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `remove-samples-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `low` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SI System and Information Integrity (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SI-7 Software, Firmware, and Information Integrity (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: samples
> Samples Operator management state

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `remove-samples-samples` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/remove-samples-operator.yaml` | inherited | Template configuration |


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