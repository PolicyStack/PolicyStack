# advanced-cluster-security - Policy Library Documentation

> Red Hat Advanced Cluster Security: Central, secured clusters and security policies

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `advancedClusterSecurity` | "Red Hat Advanced Cluster Security: Central, secured clusters and security policies" |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | SI System and Information Integrity, CM Configuration Management | Default category classifications |
| Controls | SI-3 Malicious Code Protection, CM-6 Configuration Settings | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `central` | `False` | the declarative auth config, security policies and the init-bundle Job. |
| `secured` | `False` | Run the secured-cluster agents here. Usually every cluster, including the hub. |
| `console` | `True` | Publish a console link to Central. Only meaningful where Central runs. |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `scannerV4` | `Enabled` | Scanner V4 component state: Enabled, Default or Disabled |
| `egressConnectivity` | `Online` | Central egress posture: Online or Offline |
| `networkPolicies` | `` | Let the operator manage its NetworkPolicies: Enabled or Disabled. Empty leaves it unset. |
| `monitoring` | `None` | Wire ACS into OpenShift monitoring. Leave unset to use the operator default. |
| `scanner` | `(dict)` | Central scanner autoscaling |
| `auth` | `(dict)` | Authentication wired in declaratively at Central startup |
| `admissionControl` | `(dict)` | Admission control behaviour on secured clusters |
| `collector` | `(dict)` | Node collector |
| `bootstrapImage` | `image-registry.openshift-image-registry.svc:5000/openshift/cli:latest` | Image used by the init-bundle Job |
| `securityPolicies` | `(dict)` | SecurityPolicy objects, keyed by name. As values rather than built-ins they can be edited, extended or replaced per cluster. |

## Policies

### 📋 Policy: install
> Install and manage the ACS operator

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
| Categories | SI System and Information Integrity, CM Configuration Management (default) | Category classifications |
| Controls | SI-3 Malicious Code Protection, CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: acs
> Advanced Cluster Security operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-acs` | Operator policy identifier |
| Namespace | `rhacs-operator` | Target namespace for operator |
| Display Name | `Advanced Cluster Security for Kubernetes` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `rhacs-operator` | Operator package name |
| Channel | `stable` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: central
> Central, its declarative auth config and security policies

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `central-<release>` | Full policy name including release |
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
| Categories | SI System and Information Integrity, CM Configuration Management (default) | Category classifications |
| Controls | SI-3 Malicious Code Protection, CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: central
> Central and its declarative configuration

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `central-central` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Gating:**

- Waits for operator `acs` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-namespace.yaml` | inherited | Template configuration |
| `converters/acs-declarative-config.yaml` | inherited | Template configuration |
| `converters/acs-central.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |

###### ⚙️ Config: central-ready
> Central readiness

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `central-central-ready` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-central-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |

###### ⚙️ Config: policies
> SecurityPolicy objects

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `central-policies` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Raw Template | Enabled | One object per entry in config.securityPolicies |

**Gating:**

- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `central-central-ready` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-security-policies.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |

###### ⚙️ Config: bootstrap
> Init-bundle RBAC and Job

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `central-bootstrap` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Gating:**

- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `central-central-ready` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-bootstrap-sa.yaml` | inherited | Template configuration |
| `converters/acs-bootstrap-role.yaml` | inherited | Template configuration |
| `converters/acs-bootstrap-rolebinding.yaml` | inherited | Template configuration |
| `converters/acs-init-bundle-job.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |

###### ⚙️ Config: bundle-ready
> Init-bundle Job completion

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `central-bundle-ready` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-bundle-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |

###### ⚙️ Config: console
> Console link to Central

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `central-console` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-console-link.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |


---

### 📋 Policy: secured
> Secured-cluster agents and their TLS material

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `secured-<release>` | Full policy name including release |
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
| Categories | SI System and Information Integrity, CM Configuration Management (default) | Category classifications |
| Controls | SI-3 Malicious Code Protection, CM-6 Configuration Settings (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: tls
> Sensor, collector and admission-control TLS from the hub

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `secured-tls` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-namespace.yaml` | inherited | Template configuration |
| `converters/acs-sync-sensor.yaml` | inherited | Template configuration |
| `converters/acs-sync-collector.yaml` | inherited | Template configuration |
| `converters/acs-sync-admission-control.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |

###### ⚙️ Config: agents
> SecuredCluster agents

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `secured-agents` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Gating:**

- Waits for operator `acs` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `secured-tls` | `ConfigurationPolicy` | `Compliant` |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-secured-cluster.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |

###### ⚙️ Config: agents-ready
> SecuredCluster readiness

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `secured-agents-ready` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/acs-secured-ready.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `stackrox` | Parameter value |
| `hubNamespace` | `stackrox` | Parameter value |
| `centralName` | `stackrox-central-services` | Parameter value |
| `securedName` | `stackrox-secured-cluster-services` | Parameter value |
| `declarativeConfigMap` | `acs-declarative-configs` | Parameter value |
| `dbClaimName` | `stackrox-db` | Parameter value |
| `bootstrapName` | `create-cluster-init` | Parameter value |
| `jobName` | `create-cluster-init-bundle-v2` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 3 |
| Configuration Policies | 9 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **13** |