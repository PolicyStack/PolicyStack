# servicemesh3-ambient - Policy Library Documentation

> Service Mesh 3 in ambient mode, with Kiali, OpenTelemetry and Tempo

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `servicemesh3Ambient` | Service Mesh 3 in ambient mode, with Kiali, OpenTelemetry and Tempo |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | SC System and Communications Protection | Default category classifications |
| Controls | SC-8 Transmission Confidentiality and Integrity | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `tracing` | `False` | Deploy Tempo and wire the mesh and Kiali to it. Requires ODF on the same cluster. |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `istioVersion` | `v1.28-latest` | Istio version the sail operator installs |
| `pilotRequests` | `(dict)` | Resource requests for istiod |
| `tracing` | `(dict)` |  |

## Policies

### 📋 Policy: mesh
> Service Mesh 3 control plane, CNI and ztunnel

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `mesh-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `install-servicemesh3operator-<cluster>` | `Policy` | `Compliant` |
| `install-kiali-<cluster>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | SC System and Communications Protection (default) | Category classifications |
| Controls | SC-8 Transmission Confidentiality and Integrity (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: namespaces
> Mesh namespaces

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `mesh-namespaces` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/namespace-istio-system.yaml` | inherited | Template configuration |
| `converters/namespace-istio-cni.yaml` | inherited | Template configuration |
| `converters/namespace-ztunnel.yaml` | inherited | Template configuration |
| `converters/network-cluster.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `tracingNamespace` | `tracing-system` | Parameter value |
| `tempoBucket` | `tempo-bucket-odf` | Parameter value |

###### ⚙️ Config: control-plane
> IstioCNI, Istio and ztunnel

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `mesh-control-plane` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `mesh-namespaces` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/istiocni-default.yaml` | inherited | Template configuration |
| `converters/istio-default.yaml` | inherited | Template configuration |
| `converters/ztunnel-default.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `tracingNamespace` | `tracing-system` | Parameter value |
| `tempoBucket` | `tempo-bucket-odf` | Parameter value |

###### ⚙️ Config: observability
> Kiali, telemetry and monitors

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `mesh-observability` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `mesh-control-plane` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/kiali.yaml` | inherited | Template configuration |
| `converters/telemetry-enable-prometheus-metrics.yaml` | inherited | Template configuration |
| `converters/servicemonitor-istiod-monitor.yaml` | inherited | Template configuration |
| `converters/podmonitor-istio-ztunnel-monitor.yaml` | inherited | Template configuration |
| `converters/clusterrolebinding-kiali-monitoring-rbac.yaml` | inherited | Template configuration |
| `converters/opentelemetrycollector-otel.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `tracingNamespace` | `tracing-system` | Parameter value |
| `tempoBucket` | `tempo-bucket-odf` | Parameter value |


---

### 📋 Policy: tracing
> Tempo tracing backend for the mesh

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `tracing-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `mesh-<release>` | `Policy` | `Compliant` |
| `install-tempo-<cluster>` | `Policy` | `Compliant` |
| `install-opentelemetry-<cluster>` | `Policy` | `Compliant` |
| `ready-openshift-data-foundation-<cluster>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | SC System and Communications Protection (default) | Category classifications |
| Controls | SC-8 Transmission Confidentiality and Integrity (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: tempo-storage
> Tempo namespace, bucket and credentials

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `tracing-tempo-storage` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/namespace-tracing-system.yaml` | inherited | Template configuration |
| `converters/objectbucketclaim-tempo-bucket-odf.yaml` | inherited | Template configuration |
| `converters/secret-tempo-s3-secret.yaml` | inherited | Template configuration |
| `converters/configmap-tempo-s3-ca-bundle.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `tracingNamespace` | `tracing-system` | Parameter value |
| `tempoBucket` | `tempo-bucket-odf` | Parameter value |

###### ⚙️ Config: tempostack
> TempoStack

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `tracing-tempostack` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `tracing-tempo-storage` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/tempostack-tempo-servicemesh.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `tracingNamespace` | `tracing-system` | Parameter value |
| `tempoBucket` | `tempo-bucket-odf` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 2 |
| Configuration Policies | 5 |
| Operator Policies | 0 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **7** |