# test-element - Policy Library Documentation

> something

*Generated: 2025-09-07 21:17:18*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `testElement` | Sample element demonstrating all policy types and documentation features |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Values

Default configuration applied to all policies unless explicitly overridden

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management, SC System and Communications Protection | Categories for organizing policies in ACM console and reports |
| Controls | CM-2 Baseline Configuration, SC-4 Information in Shared Resources | Specific security controls addressed by these policies |
| Standards | NIST SP 800-53, CIS Benchmark, PCI DSS | Compliance standards and frameworks |

## Policies

### 📋 Policy: baseline-security
> Enforces baseline security controls and configurations

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baseline-security-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Enable this policy and all its sub-policies |
| Severity | `high` | Critical priority for security team review |
| Remediation | `enforce` | Automatically remediate violations |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | SI System and Information Integrity, CM Configuration Management | Override default categories for this specific policy |
| Controls | SI-4 Information System Monitoring, CM-2 Baseline Configuration, CM-6 Configuration Settings | Security controls this policy addresses |
| Standards | NIST SP 800-53, NIST SP 800-171 | Compliance frameworks satisfied by this policy |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: pod-security-standards
> Configures Pod Security Standards for namespaces

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baseline-security-pod-security-standards` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/namespace-psa-labels.yaml` | inherited | Configure namespace labels for PSA |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `enforceLevel` | `baseline` | Enforcement level (privileged/baseline/restricted) |
| `enforceVersion` | `v1.29` | Version of pod security standards to use |
| `auditLevel` | `restricted` | Audit violations at this level |
| `warnLevel` | `restricted` | Warn about violations at this level |
| `exemptNamespaces` | `['kube-system', 'openshift-*']` | Exempted namespaces that need privileged access |

###### ⚙️ Config: rbac-configuration
> Configures RBAC rules and restrictions

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baseline-security-rbac-configuration` | Configuration policy identifier |
| Compliance Type | `mustnothave` | Compliance requirement type |
| Remediation | `inform` | Remediation action |
| Severity | `high` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/restrict-cluster-admin.yaml` | mustnothave | Prevent use of cluster-admin role |
| `converters/no-wildcard-verbs.yaml` | mustnothave | Restrict wildcard permissions |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `allowedAdminAccounts` | `['system:admin', 'system:serviceaccount:openshift-gitops:argocd-cluster-argocd-server']` | Allowed service accounts for admin access |
| `maxRoleBindings` | `20` | Maximum role bindings per namespace |

###### ⚙️ Config: image-security
> Enforces container image security policies

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baseline-security-image-security` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/image-signature-policy.yaml` | inherited | Require image signatures |
| `converters/allowed-registries.yaml` | inherited | Enforce registry allowlist |
| `converters/image-tag-policy.yaml` | inherited | Require specific image tags (no latest) |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `allowedRegistries` | `['registry.redhat.io', 'quay.io/openshift', 'gcr.io/distroless']` | List of trusted image registries |
| `requireSignatures` | `True` | Require image signatures |
| `blockedTags` | `['latest', 'edge', 'nightly']` | Blocked image tags |

##### Operator Policies

###### 🔧 Operator: acs-operator
> Runtime security and threat detection with ACS

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baseline-security-acs-operator` | Operator policy identifier |
| Namespace | `rhacs-operator` | Target namespace for operator |
| Display Name | `Advanced Cluster Security` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `high` | Severity level |
| Upgrade Approval | `Manual` | Manual approval for major version changes |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `rhacs-operator` | Operator package name |
| Channel | `stable` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |
| Starting CSV | `rhacs-operator.v4.4.0` | Specific version for consistency |

**Approved Versions:**
- `rhacs-operator.v4.4.0`
- `rhacs-operator.v4.4.1`
- `rhacs-operator.v4.4.2`

###### 🔧 Operator: openshift-gitops
> GitOps operator for declarative continuous deployment

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baseline-security-openshift-gitops` | Operator policy identifier |
| Namespace | `openshift-gitops-operator` | Target namespace for operator |
| Display Name | `OpenShift GitOps` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `openshift-gitops-operator` | Operator package name |
| Channel | `latest` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |

##### Certificate Policies

###### 🔐 Certificate: cert-expiry-monitoring
> Monitors TLS certificates for expiration

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baseline-security-cert-expiry-monitoring` | Certificate policy identifier |
| Remediation | `inform` | Only inform about expiring certificates |
| Severity | `medium` | Medium severity for expiring certs |

**Duration Requirements:**
| Type | Minimum | Maximum |
| ---- | ------- | ------- |
| Certificate | 720 Alert when certs expire within 30 days (720 hours) | 8760 Flag certs valid longer than 1 year as non-compliant |
| CA Certificate | 2160 Alert when CA certs expire within 90 days | 43800 Flag CA certs valid longer than 5 years |

**SAN Patterns:**
- Allowed Pattern: `^([a-zA-Z0-9-]+\.)*example\.com$` - Regex for allowed Subject Alternative Names
- Disallowed Pattern: `^\*\.` - Disallow wildcard certificates in production

###### 🔐 Certificate: cert-chain-validation
> Validates certificate chains and CA trust

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `baseline-security-cert-chain-validation` | Certificate policy identifier |
| Remediation | `inform` | Remediation action |
| Severity | `high` | Severity level |

**Duration Requirements:**
| Type | Minimum | Maximum |
| ---- | ------- | ------- |
| Certificate | 2160 Minimum 90 days validity | - hours |
| CA Certificate | - hours | - hours |


---

### 📋 Policy: network-security
> Controls network access, segmentation, and traffic policies

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `network-security-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Enable network security controls |
| Severity | `medium` | Medium priority as network is already segmented |
| Remediation | `inform` | Only report violations, don't auto-fix network policies |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | SC System and Communications Protection | Category classifications |
| Controls | SC-7 Boundary Protection, SC-8 Transmission Confidentiality | Control mappings |
| Standards | NIST SP 800-53 | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: require-network-policies
> Ensures all namespaces have NetworkPolicies defined

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `network-security-require-network-policies` | Configuration policy identifier |
| Compliance Type | `musthave` | Resources must exist and match specifications |
| Remediation | `enforce` | Automatically create default policies if missing |
| Severity | `high` | High severity for production namespaces |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/deny-all-ingress.yaml` | musthave | Default deny-all ingress policy for namespace isolation |
| `converters/allow-same-namespace.yaml` | musthave | Allow ingress from same namespace only |
| `converters/controlled-egress.yaml` | musthave | Allow specific egress to external services |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `allowIngressController` | `True` | Allow traffic from ingress controllers |
| `ingressNamespace` | `openshift-ingress` | Specific ingress controller namespace |
| `allowDNS` | `True` | Allow DNS resolution |
| `dnsNamespace` | `openshift-dns` | DNS service namespace |
| `allowedExternalCIDRs` | `['10.0.0.0/8', '172.16.0.0/12']` | List of allowed external endpoints |


---

### 📋 Policy: compliance-scanning
> Manages compliance scanning and audit requirements

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `compliance-scanning-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Enable compliance scanning features |
| Severity | `medium` | Policy severity level |
| Remediation | `inform` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | AU Audit and Accountability, CA Security Assessment | Category classifications |
| Controls | AU-2 Audit Events, CA-2 Security Assessments, CA-7 Continuous Monitoring | Control mappings |
| Standards | NIST SP 800-53, FedRAMP | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: compliance-operator
> Manages OpenShift Compliance Operator for CIS and NIST scanning

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `compliance-scanning-compliance-operator` | Operator policy identifier |
| Namespace | `openshift-compliance` | Target namespace for operator installation |
| Display Name | `Compliance Operator` | Human-friendly display name in ACM |
| Compliance Type | `musthave` | Operator must be present |
| Remediation | `enforce` | Automatically install and configure |
| Severity | `high` | High priority for compliance requirements |
| Upgrade Approval | `Automatic` | Approval strategy for operator updates (Automatic/Manual) |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `compliance-operator` | Operator package name in catalog |
| Channel | `stable` | Update channel (stable/fast/candidate) |
| Source | `redhat-operators` | Catalog source name |
| Source Namespace | `openshift-marketplace` | Namespace containing the catalog |
| Starting CSV | `compliance-operator.v1.5.0` | Initial CSV version to install |

**Approved Versions:**
*Approved versions for controlled operator upgrades*
- `compliance-operator.v1.5.0`
- `compliance-operator.v1.5.1`
- `compliance-operator.v1.5.2`
- `compliance-operator.v1.6.0`

###### 🔧 Operator: monitoring-operator
> Configures Prometheus monitoring and alerting

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `compliance-scanning-monitoring-operator` | Operator policy identifier |
| Namespace | `openshift-monitoring` | Target namespace for operator |
| Display Name | `Cluster Monitoring` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `inform` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Manual` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `cluster-monitoring-operator` | Operator package name |
| Channel | `stable` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: resource-management
> Enforces resource quotas, limits, and capacity management

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `resource-management-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Enable resource quota and limit enforcement |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Enforce quotas to prevent resource exhaustion |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | SC System and Communications Protection | Category classifications |
| Controls | SC-5 Denial of Service Protection, SC-6 Resource Availability | Control mappings |
| Standards | NIST SP 800-53 | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: namespace-resource-quotas
> Enforces resource quotas on application namespaces

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `resource-management-namespace-resource-quotas` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/compute-quota.yaml` | inherited | CPU and memory quotas |
| `converters/storage-quota.yaml` | inherited | Storage quotas for PVCs |
| `converters/object-quota.yaml` | inherited | Object count quotas |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `cpuLimit` | `10` | Maximum CPU cores per namespace |
| `memoryLimit` | `20Gi` | Maximum memory per namespace |
| `storageLimit` | `100Gi` | Maximum storage per namespace |
| `podCount` | `50` | Maximum number of pods |
| `serviceCount` | `10` | Maximum number of services |
| `pvcCount` | `20` | Maximum number of PVCs |


---

## PolicySets

### 📦 PolicySet: security-baseline
> Groups all baseline security policies

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `security-baseline-<release>` | PolicySet identifier |
| Enabled | `True` | Whether this PolicySet is active |

**Included Policies:**
- `baseline-security-<release>`
- `network-security-<release>`

---

### 📦 PolicySet: compliance-audit
> Groups compliance and audit policies

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `compliance-audit-<release>` | PolicySet identifier |
| Enabled | `True` | Whether this PolicySet is active |

**Included Policies:**
- `compliance-scanning-<release>`

---

### 📦 PolicySet: resource-controls
> Groups resource and capacity management policies

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `resource-controls-<release>` | PolicySet identifier |
| Enabled | `True` | Whether this PolicySet is active |

**Included Policies:**
- `resource-management-<release>`

---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 4 |
| Configuration Policies | 5 |
| Operator Policies | 4 |
| Certificate Policies | 2 |
| PolicySets | 3 |
| **Total Resources** | **18** |