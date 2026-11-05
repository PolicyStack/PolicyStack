# Compliance

The [`compliance`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/compliance) element installs the Compliance Operator, runs a scan, and reports the results in Governance. Compliant on `findings-compliance-<cluster>` means the automated checks pass or are waived, on a current scan.

!!! note "Not reported"
    `MANUAL` results are not findings: the operator can't check them, so review them separately (`oc get ccr -n openshift-compliance -l compliance.openshift.io/check-status=MANUAL`).

!!! warning "Hosted control planes"
    Red Hat ships the operator on HCP as Technology Preview, with `PLATFORM=HyperShift` and a worker `nodeSelector` this element doesn't set. A guest that can't scan shows NonCompliant in `findings-scan-state`. Leave the element off there.

## Reading Governance

| Template | NonCompliant means |
|---|---|
| `findings-scan-state` | No results yet, a configured Profile is missing, or a named scan ended `ERROR`, `NOT-AVAILABLE` or `NOT-APPLICABLE` or hasn't started in 48 hours (often a cordoned node). `oc get compliancescan -n openshift-compliance` shows which |
| `findings-high`, `-medium`, `-low` | Results that failed and aren't waived. Includes `ERROR`, and `INCONSISTENT` where a node failed |
| `findings-drift` | A remediation is applied but not declared `true` |
| `findings-waived` | Never. Lists each waived result and its current status |
| `remediate-remediation-state` | A remediation hasn't reached the declared state |

Each findings message starts with the count, then the names.

!!! warning "Names are truncated"
    ACM 2.15+ caps a message at 4096 bytes, ending in `...[truncated]`. Names sort, so `rhcos4-*` is cut first. The count stays right. For the full list, use View details > Related resources on the template.

    For the raw results, which also include waived rules and harmless `INCONSISTENT` ones:

    ```sh
    oc get ccr -n openshift-compliance -l 'compliance.openshift.io/check-status in (FAIL,ERROR,INCONSISTENT)'
    ```

A rescan keeps the last results, so findings stay visible while it runs. After a failed scan they are stale until a good one. `findings-scan-state` polls hourly while Compliant and every 10 minutes while not, so a new failure can take up to an hour to show.

## Benchmarks

`config.profiles` takes any mix. They share one binding, schedule and suite, and result names carry the profile.

```yaml
stack:
  compliance:
    config:
      profiles: [ocp4-cis, ocp4-cis-node, ocp4-stig, ocp4-stig-node, rhcos4-stig]
```

!!! warning "Replacing a profile"
    Dropping a profile deletes its scan, results, remediations and raw-results PVC. Applied MachineConfigs, KubeletConfig settings and created objects stay behind. Set its remediations to `false` and roll out before you drop it; for KubeletConfig see [Reverting](#reverting).

## Pinning

Keep the floating names. Red Hat [supports only the latest](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/compliance-operator#compliance-supported-profiles_compliance-operator-supported-profiles) CIS and STIG, a versioned name like `ocp4-stig-v2r3` doesn't freeze its rules, and operator upgrades remove old versions.

The operator version freezes the content, so pin the CSV in the cluster's layer: `operatorPolicies.compliance.versions` and `subscription.startingCSV`, set to the CSV from `oc get csv -n openshift-compliance` ([Operator versions](../guides/operators.md)).

A bump can change results. A rule dropped from content loses its result and remediation, while an applied MachineConfig stays: find leftovers with `oc get mc | grep ^75-`. An `Outdated` remediation keeps its old payload and shows in `remediate-remediation-state`. To move one to the new payload, [remove its old one](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/compliance-operator#compliance-updating_compliance-remediation). It may roll nodes:

```sh
oc -n openshift-compliance patch complianceremediations <name> --type json -p '[{"op":"remove","path":"/spec/outdated"}]'
```

Don't use the `apply-remediations` annotation: it applies everything in the suite.

## Waivers

The rule keeps being scanned, so the raw `FAIL` stays on the cluster and in the ARF as evidence. The waiver only stops it counting as a finding. `git log` holds who reviewed it.

```yaml
stack:
  compliance:
    config:
      waivers:
        api-server-audit-log-maxsize: "Logs ship off-cluster. SEC-1234" # (1)!
```

1.  The rule name, not the result name Governance shows: `oc get ccr -n openshift-compliance <result> -o jsonpath='{.metadata.annotations.compliance\.openshift\.io/rule}'`. A result name or an empty reason fails the render.

A waiver covers every role and profile on the cluster. To limit it to some clusters, put it in their layer. It takes effect at the next evaluation, with no rescan. A waiver can't hide `ERROR`.

!!! note "Limits"
    - The reason stays in git. Governance doesn't show it.
    - `oc get compliancesuite -n openshift-compliance` still says `NON-COMPLIANT`, since the operator doesn't know about waivers.
    - Waivers don't expire. A waived rule that starts failing again shows only as `=FAIL` in `findings-waived`.

    To stop scanning a rule, Red Hat documents [TailoredProfile](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/compliance-operator#compliance-tailored-profiles_compliance-tailor) `disableRules`. It removes the evidence, and it isn't part of this element.

## Remediations

Off until you turn on policy `remediate`. `remediations` keys are ComplianceRemediation names. Applying one can reboot nodes.

```yaml
stack:
  compliance:
    policies:
      remediate:
        enabled: true
    config:
      remediations:
        ocp4-stig-api-server-encryption-provider-cipher: true # (1)!
        rhcos4-stig-worker-sysctl-kernel-dmesg-restrict: false # (2)!
```

1.  List the base name. The operator keeps `-1`, `-2` siblings in step. Sibling keys with different values fail the render.
2.  Reverts it.

Writes wait for every scan to reach `DONE`. A name that doesn't exist is never created. `remediate-remediation-state` reports one set to `true` unless its check passes. One set to `false` is ignored, so check the spelling.

### Reverting

Removing an entry doesn't revert anything. It stays applied and shows in `findings-drift`. Set it to `false`, and remove the entry once the state is `NotApplied`. Turning `remediate` off stops managing `spec.apply` and reverts nothing.

| Remediation | `false` does |
|---|---|
| MachineConfig | Deletes it and reboots the pool |
| Object the operator created | Deletes it |
| KubeletConfig | Nothing. Status says `NotApplied`, the setting stays, and `remediate-remediation-state` stays NonCompliant until you follow [Red Hat's manual removal](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/compliance-operator#compliance-removing-kubeletconfig_compliance-remediation) and drop the entry. Affected nodes reboot. |
| Object the operator patched | Nothing, and status says `NotApplied`. Nothing here detects it. |

Deleting a ComplianceRemediation doesn't revert it either. Run a rescan afterwards, since results don't change until then.

## Turning the element off

Policies are deleted. The binding, daily scans and applied remediations stay. To stop scanning and keep the results:

```sh
oc patch scansetting default -n openshift-compliance --type merge -p '{"suspend":true}'
```

Deleting the binding (`oc delete ssb <config.binding> -n openshift-compliance`, default `stig`) also deletes the results and raw-results PVCs.
