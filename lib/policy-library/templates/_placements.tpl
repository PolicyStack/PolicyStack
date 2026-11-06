{{/*
Function to pull in placement resources for Policies or PolicySets
*/}}
{{- define "policy-library.renderPlacements" -}}
{{/* Get component name - either passed in or derived from chart name */}}
{{- $componentName := .component | default (include "policy-library.componentName" .) -}}
{{- $root := . -}}

{{/* Access the component using the key */}}
{{- $component := index .Values.stack $componentName -}}

{{/* Get whether we should use policySets or bind placementbinding directly to the policies */}}
{{- $usePolicySetsPlacements := $component.usePolicySetsPlacements | default false }}
{{- if and $component $component.enabled -}}
{{- $policyNamespace := $root.Values.policyNamespace }}

{{/* Process custom policies */}}
{{- if not $component.disablePlacements | default false }}
{{/*
.Values.placement is the Placement spec. claimSelector/celSelector are ANDed into the predicate built
from .Values.selector, extra predicates are appended (ORed), and every other key passes through.
Tolerating unreachable/unavailable by default keeps policies bound while a cluster is disconnected.
*/}}
{{- $placement := deepCopy ($root.Values.placement | default dict) }}
{{- if kindIs "invalid" $placement.tolerations }}
{{- $_ := set $placement "tolerations" (list (dict "key" "cluster.open-cluster-management.io/unreachable" "operator" "Exists") (dict "key" "cluster.open-cluster-management.io/unavailable" "operator" "Exists")) }}
{{- end }}
{{- $matchExpressions := list }}
{{- range $root.Values.selector.matchExpressions }}
{{- $matchExpressions = append $matchExpressions . }}
{{- end }}
{{- $clusterSelector := set (pick $placement "claimSelector" "celSelector") "labelSelector" (dict "matchExpressions" $matchExpressions) }}
{{- $_ := set $placement "predicates" (prepend ($placement.predicates | default list) (dict "requiredClusterSelector" $clusterSelector)) }}
---
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: {{ $root.Release.Name }}
  namespace: {{ $policyNamespace }}
spec:
  {{- omit $placement "claimSelector" "celSelector" | toYaml | nindent 2 }}
{{- $subjects := list }}
{{- if $usePolicySetsPlacements }}
{{- range $component.policySets }}
{{- if and (include "policy-library.enabled" .) .policies }}
{{- $subjects = append $subjects (dict "name" (printf "%s-%s" .name $root.Release.Name) "kind" "PolicySet" "apiGroup" "policy.open-cluster-management.io") }}
{{- end }}
{{- end }}
{{- else }}
{{- range include "policy-library.model" (dict "root" $root "component" $component) | fromJsonArray }}
{{- if or .templates .operators }}
{{- $subjects = append $subjects (dict "name" (printf "%s-%s" .name $root.Release.Name) "kind" "Policy" "apiGroup" "policy.open-cluster-management.io") }}
{{- end }}
{{- end }}
{{- end }}
{{- with $subjects }}
---
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: {{ $root.Release.Name }}
  namespace: {{ $policyNamespace }}
placementRef:
  name: {{ $root.Release.Name }}
  kind: Placement
  apiGroup: cluster.open-cluster-management.io
subjects:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
{{- end }}
{{- end -}}
