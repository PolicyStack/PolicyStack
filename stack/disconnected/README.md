# disconnected - Policy Library Documentation

> Mirror registries, mirrored catalogs and CA trust for disconnected clusters

Field reference: [policy-library](https://github.com/PolicyStack/PolicyStack-chart/tree/main/charts/policy-library)

*Generated: 2026-10-19 17:44:57*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `disconnected` | Mirror registries, mirrored catalogs and CA trust for disconnected clusters. Pair it with global.catalogSources so operators install from the mirrored catalogs. On a hosted cluster the HostedCluster owns mirrors, image config and the proxy: keep only CatalogSources in clusterResources and turn registry-ca and trusted-ca off. |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection | Default category classifications |
| Controls | CM-2 Baseline Configuration, SC-7 Boundary Protection | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `resources` | `True` | Apply the objects in config.clusterResources |
| `registry-ca` | `True` | Trust the mirror registries' CAs for image pulls |
| `trusted-ca` | `True` | Add config.trustedCA to the cluster trust bundle through ConfigMap trusted-cas (Proxy trustedCA) |
| `operatorhub` | `True` | Disable the default OperatorHub catalog sources, which cannot be reached |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `clusterResources` | `(list)` | Objects from oc-mirror's working-dir/cluster-resources, applied as they are: ImageDigestMirrorSet, ImageTagMirrorSet, CatalogSource, ClusterCatalog and the release signature ConfigMaps |
| `registryCAs` | `(dict)` | Mirror registry CAs for image pulls, keyed by host, or host..port for a port (registry.example.com..8443). Values are PEM. |
| `trustedCA` | `` | PEM bundle added to the cluster trust bundle, e.g. the CA of an internal Git or Helm server. Appended to the CAs in user-ca-bundle, which is never changed. |

## Policies

### 📋 Policy: resources
> Mirror sets and mirrored catalogs from oc-mirror

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `resources-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `high` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SC-7 Boundary Protection (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: objects
> oc-mirror cluster resources

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `resources-objects` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/cluster-resources.yaml` | inherited | Template configuration |


---

### 📋 Policy: registry-ca
> Trust the mirror registries' CAs

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `registry-ca-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `high` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SC-7 Boundary Protection (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: trust
> Registry CA ConfigMap and image config

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `registry-ca-trust` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/registry-ca.yaml` | inherited | Template configuration |


---

### 📋 Policy: trusted-ca
> Add a CA bundle to the cluster trust bundle

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `trusted-ca-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `high` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SC-7 Boundary Protection (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: proxy
> trusted-cas ConfigMap and Proxy trustedCA

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `trusted-ca-proxy` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |
| Raw Template | Enabled | Emitted under object-templates-raw |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/trusted-ca.yaml` | inherited | Template configuration |


---

### 📋 Policy: operatorhub
> Disable the default OperatorHub catalog sources

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `operatorhub-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection (default) | Category classifications |
| Controls | CM-2 Baseline Configuration, SC-7 Boundary Protection (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: sources
> OperatorHub default sources

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `operatorhub-sources` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/operatorhub.yaml` | inherited | Template configuration |


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