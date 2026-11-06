{{/*
PolicySets grouping the element's Policies by name
*/}}
{{- define "policy-library.renderPolicySets" -}}
{{/* Get component name - either passed in or derived from chart name */}}
{{- $componentName := .component | default (include "policy-library.componentName" .) -}}
{{- $root := . -}}

{{/* Access the component using the key */}}
{{- $component := index .Values.stack $componentName -}}
{{- if $component -}}
{{- if $component.enabled -}}

{{/* Process custom policies */}}
{{- range $component.policySets }}
{{- if and (include "policy-library.enabled" .) .policies }}
{{- $policySetName := .name }}
{{- $policyNamespace := $root.Values.policyNamespace }}
{{- $policyValues := . }}
---
apiVersion: policy.open-cluster-management.io/v1beta1
kind: PolicySet
metadata:
  name: {{ $policySetName }}-{{ $root.Release.Name }}
  namespace: {{ $policyNamespace }}
spec:
  policies:
    {{- range .policies }}
    - {{ . }}-{{ $root.Release.Name }}
    {{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
