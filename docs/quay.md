# quay - Policy Library Documentation

> Red Hat Quay container registry

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `quay` | Red Hat Quay container registry |
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
| `configure` | `False` | Run the one-shot bootstrap Job and publish the console link |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `components` | `(dict)` | supplying it yourself, e.g. an external Postgres or object store. |
| `featureUserInitialize` | `True` | Allow the first user to be created through the initialize API |
| `browserApiCallsXhrOnly` | `False` | Restrict browser API calls to XHR |
| `featureUserCreation` | `False` | Allow self-service user creation |
| `superUsers` | `(list)` | Quay superusers |
| `adminUser` | `quayadmin` | Superuser the bootstrap Job initialises |
| `initialUser` | `quaydevel` | First non-admin user the bootstrap Job creates |
| `initialOrg` | `devel` | Organization the bootstrap Job creates |
| `initialOrgEmail` | `devel@myorg.com` | Contact address for that organization (must differ from the admin's) |
| `initialRepo` | `example` | Repository the bootstrap Job creates in that organization |
| `bootstrapImage` | `image-registry.openshift-image-registry.svc:5000/openshift/cli:latest` | Image used to run the bootstrap Job |

## Policies

### 📋 Policy: install
> Install and manage the Quay operator

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

###### 🔧 Operator: quay
> Red Hat Quay operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-quay` | Operator policy identifier |
| Namespace | `openshift-operators` | Target namespace for operator |
| Display Name | `Red Hat Quay` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `quay-operator` | Operator package name |
| Channel | `stable-3.17` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: deploy
> Deploy the Quay registry

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `deploy-<release>` | Full policy name including release |
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

###### ⚙️ Config: registry
> Quay namespace, config bundle and QuayRegistry

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `deploy-registry` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Waits for operator `quay` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/quay-namespace.yaml` | inherited | Template configuration |
| `converters/quay-config-bundle.yaml` | inherited | Template configuration |
| `converters/quay-registry.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `quay-enterprise` | Parameter value |
| `name` | `registry` | Parameter value |
| `configBundleSecret` | `quay-registry-config-bundle` | Parameter value |


---

### 📋 Policy: configure
> Bootstrap the registry and publish its console link

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `configure-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `low` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `deploy-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: bootstrap
> Bootstrap Job, its RBAC, and the console link

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `configure-bootstrap` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/quay-bootstrap-sa.yaml` | inherited | Template configuration |
| `converters/quay-bootstrap-role.yaml` | inherited | Template configuration |
| `converters/quay-bootstrap-rolebinding.yaml` | inherited | Template configuration |
| `converters/quay-bootstrap-job.yaml` | inherited | Template configuration |
| `converters/quay-host-configmap.yaml` | inherited | Template configuration |
| `converters/quay-console-link.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `namespace` | `quay-enterprise` | Parameter value |
| `name` | `registry` | Parameter value |
| `bootstrapName` | `create-admin-user` | Parameter value |
| `routeName` | `registry-quay` | Parameter value |
| `consoleText` | `Red Hat Quay Enterprise Registry` | Parameter value |
| `consoleImageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 3 |
| Configuration Policies | 2 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **6** |