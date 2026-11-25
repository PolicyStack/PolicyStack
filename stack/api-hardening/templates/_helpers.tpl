{{/*
ACM condition, true on a hosted control plane guest. HyperShift sets the guest's APIServer, OAuth and
kubeadmin Secret from the HostedCluster on the management cluster and overwrites guest-side edits,
so every template skips its objects there.
*/}}
{{- define "api-hardening.hosted" -}}
eq (dig "status" "controlPlaneTopology" "" (lookup "config.openshift.io/v1" "Infrastructure" "" "cluster")) "External"
{{- end -}}

{{/* Fails the render unless .value is one of .allowed. */}}
{{- define "api-hardening.oneOf" -}}
{{- if not (has .value .allowed) }}{{ fail (printf "stack.apiHardening.config.%s: %q must be one of %s" .key .value (join ", " .allowed)) }}{{ end }}
{{- end -}}
