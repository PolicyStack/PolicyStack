{{/*
Renders the element's Policies (policy-library.model) with their ConfigurationPolicies and
OperatorPolicies.

remediationAction and severity resolve entry -> policy -> default -> inform/low. Every template takes
its Policy's action, which ACM applies anyway, unless it sets InformOnly.
*/}}
{{- define "policy-library.renderPolicies" -}}
{{- $componentName := .component | default (include "policy-library.componentName" .) -}}
{{- $root := . -}}
{{- $component := index .Values.stack $componentName -}}
{{- if and $component $component.enabled -}}
{{- $default := $component.default | default dict -}}

{{- /*
operatorPolicies is a map keyed by entry name, so layers merge into it. A layer key the element
does not define merges into an entry with no subscription, which would otherwise render nothing.
*/ -}}
{{- range $opName, $op := $component.operatorPolicies -}}
{{- if and (kindIs "map" $op) (not ($op.subscription).name) -}}
{{- fail (printf "policy-library: operatorPolicies.%s has no subscription.name - a values layer may set an operator the element does not define" $opName) -}}
{{- end -}}
{{- end -}}

{{- range $policy := include "policy-library.model" (dict "root" $root "component" $component) | fromJsonArray }}
{{- if or $policy.templates $policy.operators }}
{{- $policyName := $policy.name }}
{{- $pv := get ($component.policies | default dict) $policyName | default dict }}
{{- $action := $pv.remediationAction | default $default.remediationAction | default "inform" }}
{{- $severity := $pv.severity | default $default.severity | default "low" }}
{{- $orderManifests := not (empty $pv.orderManifests) }}
{{- $prevManifest := dict }}
---
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: {{ $policyName }}-{{ $root.Release.Name }}
  namespace: {{ $root.Values.policyNamespace }}
  annotations:
    {{- with $pv.categories | default $default.categories }}
    policy.open-cluster-management.io/categories: {{ join "," . | quote }}
    {{- end }}
    {{- with $pv.controls | default $default.controls }}
    policy.open-cluster-management.io/controls: {{ join "," . | quote }}
    {{- end }}
    {{- with $pv.standards | default $default.standards }}
    policy.open-cluster-management.io/standards: {{ join "," . | quote }}
    {{- end }}
    {{- with $pv.description }}
    description: {{ . | quote }}
    {{- end }}
spec:
  {{- /* global.remediationAction (inform only, per the schema) overrides every template, a dry run for any layer */}}
  remediationAction: {{ ($root.Values.global).remediationAction | default $action }}
  disabled: {{ $pv.disabled | default false }}
  copyPolicyMetadata: false
  {{- with $pv.hubTemplateOptions }}
  hubTemplateOptions: {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with $pv.dependencies }}
  dependencies:
    {{- include "policy-library.dependencyList" (dict "deps" . "root" $root "component" $component "policyRef" $policyName "defaultKind" "Policy") | nindent 4 }}
  {{- end }}
  policy-templates:
  {{- range $policy.templates }}
  {{- $tv := get ($pv.templates | default dict) .name | default dict }}
  {{- $configName := include "policy-library.templateName" (dict "root" $root "component" $component "policyRef" $policyName "name" .name) }}
  {{- /* Converters read .Config: the element's config merged with the template's own */}}
  {{- $context := merge (dict "Config" (mergeOverwrite (deepCopy ($component.config | default dict)) (deepCopy ($tv.config | default dict)))) $root }}
  {{- $contents := list }}
  {{- range .files }}
  {{- $path := printf "converters/%s.yaml" . }}
  {{- $source := $root.Files.Get $path }}
  {{- if not $source }}
  {{- fail (printf "policy-library: %s, a file of %s, is missing or empty" $path $configName) }}
  {{- end }}
  {{- with tpl $source $context | trim }}{{ $contents = append $contents . }}{{ end }}
  {{- end }}
  {{- /*
  The rendered shape picks the field. A list ("- ...", "[]") or an ACM template ("{{ ... }}") goes to
  object-templates-raw, which replaces object-templates and so must be the template's only output.
  Anything else is one object per "---" document. Full-line YAML comments don't count toward the shape,
  and a document of only comments is dropped. Rendering nothing gives an empty raw list, so the Policy
  has nothing to enforce instead of failing to parse.
  */}}
  {{- $documents := list }}
  {{- range $contents }}
  {{- range regexSplit "(?m)^---[ \t]*$" . -1 }}{{ if regexReplaceAll "(?m)^[ \t]*#.*$" . "" | trim }}{{ $documents = append $documents (trim .) }}{{ end }}{{ end }}
  {{- end }}
  {{- $raw := "" }}
  {{- $objects := list }}
  {{- range $documents }}
  {{- $bare := regexReplaceAll "(?m)^[ \t]*#.*$" . "" | trim }}
  {{- if or (hasPrefix "-" $bare) (hasPrefix "[" $bare) (hasPrefix "{{" $bare) }}
  {{- if gt (len $documents) 1 }}
  {{- fail (printf "policy-library: %s renders a raw object list next to other documents - a raw list must be the template's only output" $configName) }}
  {{- end }}
  {{- $raw = . }}
  {{- else }}
  {{- $objects = append $objects . }}
  {{- end }}
  {{- end }}
  {{- if not (or $raw $objects) }}{{ $raw = "[]" }}{{ end }}
  {{- $depsYaml := include "policy-library.subPolicyDependencies" (dict "root" $root "component" $component "policyRef" $policyName "subPolicy" $tv "prev" (ternary $prevManifest dict $orderManifests)) -}}
  {{- $prevManifest = dict "name" $configName "kind" "ConfigurationPolicy" -}}
  {{- /* waitForOperator implies ignorePending: stay compliant while the operator installs */}}
  {{- $ignorePending := ternary (not (empty $tv.waitForOperator)) $tv.ignorePending (kindIs "invalid" $tv.ignorePending) }}
    # Configuration policies - necessary to prevent line issues
    {{- include "policy-library.policyTemplateHeader" (dict "depsYaml" $depsYaml "ignorePending" $ignorePending) | nindent 4 }}
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: {{ $configName }}
          {{- if or $tv.description $tv.disableTemplating }}
          annotations:
            {{- with $tv.description }}
            description: {{ . | quote }}
            {{- end }}
            {{- if $tv.disableTemplating }}
            policy.open-cluster-management.io/disable-templates: "true"
            {{- end }}
          {{- end }}
        spec:
          {{- with $tv.namespaceSelector }}
          namespaceSelector: {{ nindent 12 (toYaml .) }}
          {{- end }}
          {{- with $tv.customMessage }}
          customMessage:
            {{- with .compliant }}
            compliant: {{ . | quote }}
            {{- end }}
            {{- with .noncompliant }}
            noncompliant: {{ . | quote }}
            {{- end }}
          {{- end }}
          {{- with $tv.evaluationInterval }}
          evaluationInterval:
            {{- with .compliant }}
            compliant: {{ . }}
            {{- end }}
            {{- with .noncompliant }}
            noncompliant: {{ . }}
            {{- end }}
          {{- end }}
          {{- with $tv.pruneObjectBehavior }}
          pruneObjectBehavior: {{ . }}
          {{- end }}
          remediationAction: {{ ternary "InformOnly" $action (eq (toString $tv.remediationAction) "InformOnly") }}
          severity: {{ $tv.severity | default $severity }}
          {{- if $raw }}
          object-templates-raw: |{{- $raw | nindent 12 }}
          {{- else }}
          object-templates:
          {{- range $objects }}
            - complianceType: {{ $tv.complianceType | default "musthave" }}
              {{- with $tv.metadataComplianceType }}
              metadataComplianceType: {{ . }}
              {{- end }}
              {{- with $tv.recordDiff }}
              recordDiff: {{ . }}
              {{- end }}
              {{- with $tv.recreateOption }}
              recreateOption: {{ . }}
              {{- end }}
              {{- with $tv.objectSelector }}
              objectSelector: {{ nindent 16 (toYaml .) }}
              {{- end }}
              objectDefinition:{{- nindent 16 . }}
          {{- end }}
          {{- end }}
  {{- end }}
  {{- range $opName := $policy.operators }}
  {{- with set (deepCopy (index $component.operatorPolicies $opName)) "name" $opName }}
  {{- $configName := include "policy-library.templateName" (dict "root" $root "component" $component "policyRef" $policyName "name" .name) }}
  {{- $opSeverity := .severity | default $severity }}
  {{- $complianceType := .complianceType | default "musthave" }}
  {{- $ignorePending := ternary (not (empty .waitForOperator)) .ignorePending (kindIs "invalid" .ignorePending) }}
  {{- /*
  Operator policies render three objects. Under orderManifests they are chained explicitly rather
  than by render position: ns -> OperatorPolicy -> status. Chaining in render position would make
  the OperatorPolicy wait on the CSV status check that only its own installation can satisfy.

  mustnothave uninstalls the operator through the OperatorPolicy's removalBehavior and renders no
  namespace template: deleting the namespace would remove every other operator sharing it.
  */ -}}
  {{- $install := ne $complianceType "mustnothave" }}
  {{- $nsRef := dict "name" (printf "%s-ns" $configName) "kind" "ConfigurationPolicy" }}
  {{- $opRef := dict "name" $configName "kind" "OperatorPolicy" }}
  {{- $statusRef := dict "name" (printf "%s-status" $configName) "kind" "ConfigurationPolicy" }}
  {{- $nsDeps := include "policy-library.subPolicyDependencies" (dict "root" $root "component" $component "policyRef" $policyName "subPolicy" dict "prev" (ternary $prevManifest dict $orderManifests)) }}
  {{- $opDeps := include "policy-library.subPolicyDependencies" (dict "root" $root "component" $component "policyRef" $policyName "subPolicy" . "prev" (ternary (ternary $nsRef $prevManifest $install) dict $orderManifests)) }}
  {{- $statusDeps := include "policy-library.subPolicyDependencies" (dict "root" $root "component" $component "policyRef" $policyName "subPolicy" dict "prev" (ternary $opRef dict $orderManifests)) }}
  {{- $prevManifest = $statusRef }}
    {{- if $install }}
    {{- include "policy-library.policyTemplateHeader" (dict "depsYaml" $nsDeps) | nindent 4 }}
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: {{ $configName }}-ns
        spec:
          remediationAction: {{ $action }}
          severity: {{ $opSeverity }}
          object-templates:
            - complianceType: {{ $complianceType }}
              objectDefinition:
                apiVersion: v1
                kind: Namespace
                metadata:
                  name: {{ .namespace }}
                  {{- if .clusterMonitoring }}
                  labels:
                    openshift.io/cluster-monitoring: "true"
                  {{- end }}
    {{- end }}
    {{- include "policy-library.policyTemplateHeader" (dict "depsYaml" $statusDeps) | nindent 4 }}
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: {{ $configName }}-status
        spec:
          remediationAction: inform
          severity: {{ $opSeverity }}
          object-templates:
            - complianceType: {{ $complianceType }}
              objectDefinition:
                apiVersion: operators.coreos.com/v1alpha1
                kind: ClusterServiceVersion
                metadata:
                  namespace: {{ .namespace }}
                spec:
                  displayName: {{ .displayName | default .subscription.name }}
                status:
                  phase: Succeeded
    {{- include "policy-library.policyTemplateHeader" (dict "depsYaml" $opDeps "ignorePending" $ignorePending) | nindent 4 }}
        apiVersion: policy.open-cluster-management.io/v1beta1
        kind: OperatorPolicy
        metadata:
          name: {{ $configName }}
          {{- with .description }}
          annotations:
            description: {{ . | quote }}
          {{- end }}
        spec:
          {{- with .versions }}
          versions: {{ nindent 12 (toYaml .) }}
          {{- end }}
          remediationAction: {{ $action }}
          severity: {{ $opSeverity }}
          complianceType: {{ $complianceType }}
          {{- with .removalBehavior }}
          removalBehavior: {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- /*
          operatorGroup is optional: omitting it means "name defaults to the subscription name,
          install cluster-scoped". Parenthesised access keeps that working - a bare
          .operatorGroup.name panics when the whole map is absent.
          */}}
          operatorGroup:
            name: {{ (.operatorGroup).name | default .subscription.name }}
            namespace: {{ .namespace }}
            {{- with (.operatorGroup).targetNamespaces }}
            targetNamespaces:{{ nindent 14 (toYaml .) }}
            {{- end }}
          subscription:
            name: {{ .subscription.name }}
            {{- with .namespace }}
            namespace: {{ . }}
            {{- end }}
            {{- with .subscription.channel }}
            channel: {{ . }}
            {{- end }}
            {{- /* global.catalogSources renames a source fleet-wide, e.g. to a mirrored catalog */}}
            {{- $source := .subscription.source | default "redhat-operators" }}
            source: {{ index (($root.Values.global).catalogSources | default dict) $source | default $source }}
            sourceNamespace: {{ .subscription.sourceNamespace | default "openshift-marketplace" }}
            {{- with .subscription.startingCSV }}
            startingCSV: {{ . }}
            {{- end }}
            {{- with .subscription.config }}
            config: {{ nindent 14 (toYaml .) }}
            {{- end }}
          upgradeApproval: {{ .upgradeApproval | default "Automatic" }}
  {{- end }}
  {{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end -}}
