# mtv - Policy Library Documentation

> Migration Toolkit for Virtualization

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `mtv` | Migration Toolkit for Virtualization |
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
| `uiPlugin` | `True` | Expose the Forklift console plugin |
| `validation` | `True` | Run the validation service |
| `volumePopulator` | `True` | Enable the volume populator for disk transfer |

## Policies

### 📋 Policy: install
> Install and manage the Migration Toolkit for Virtualization operator

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

##### Configuration Policies

###### ⚙️ Config: ns-monitoring
> Adds openshift.io/cluster-monitoring label to the operator namespace

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-ns-monitoring` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/ns-monitoring-label.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-mtv` | Parameter value |

##### Operator Policies

###### 🔧 Operator: mtv
> Migration Toolkit for Virtualization

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-mtv` | Operator policy identifier |
| Namespace | `openshift-mtv` | Target namespace for operator |
| Display Name | `Migration Toolkit for Virtualization Operator` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `mtv-operator` | Operator package name |
| Channel | `release-v2.10` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: config
> Deploy the ForkliftController instance

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
| `install-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: forklift
> ForkliftController instance

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `config-forklift` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Waits for operator `mtv` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/forklift-controller.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `openshift-mtv` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 2 |
| Configuration Policies | 2 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **5** |