{{- /* Rejects values the converters cannot act on. A layer's null removes an entry. */ -}}
{{- define "compliance.validate" -}}
{{- $cfg := .Values.stack.compliance.config -}}
{{- range $rule, $reason := $cfg.waivers -}}
{{- if not (kindIs "invalid" $reason) -}}
{{- if not (and (kindIs "string" $reason) (trim $reason)) }}{{ fail (printf "stack.compliance.config.waivers.%s needs a reason" $rule) }}{{ end -}}
{{- if regexMatch "^(ocp4|rhcos4)-" $rule }}{{ fail (printf "stack.compliance.config.waivers.%s is a result name: use its compliance.openshift.io/rule annotation" $rule) }}{{ end -}}
{{- end -}}
{{- end -}}
{{- $seen := dict -}}
{{- range $name, $apply := $cfg.remediations -}}
{{- if not (kindIs "invalid" $apply) -}}
{{- if not (kindIs "bool" $apply) }}{{ fail (printf "stack.compliance.config.remediations.%s must be true or false" $name) }}{{ end -}}
{{- /* The operator keeps names equal after this replacement in step, so different values would fight it. */ -}}
{{- $base := regexReplaceAll "-[0-9]+" $name "" -}}
{{- if and (hasKey $seen $base) (ne (get $seen $base) $apply) }}{{ fail (printf "stack.compliance.config.remediations.%s conflicts with a name the operator treats as the same remediation" $name) }}{{ end -}}
{{- $_ := set $seen $base $apply -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* ACM list of the rule names in config.waivers. */ -}}
{{- define "compliance.waivedRules" -}}
list
{{- range $rule, $reason := .Values.stack.compliance.config.waivers }}
{{- if $reason }} {{ $rule | quote }}{{ end }}
{{- end }}
{{- end -}}

{{- /* ACM list of the remediations declared true, as the operator's sibling names reduce them. */ -}}
{{- define "compliance.declaredRemediations" -}}
list
{{- range $name, $apply := .Values.stack.compliance.config.remediations }}
{{- if $apply }} {{ regexReplaceAll "-[0-9]+" $name "" | quote }}{{ end }}
{{- end }}
{{- end -}}

{{- /*
ACM statements setting $done: every live scan has finished. The aggregator rewrites remediations
during a scan. Suite status keeps entries of deleted scans, so it is not read.
*/ -}}
{{- define "compliance.scansDone" -}}
{{ "{{" }}- $scans := (lookup "compliance.openshift.io/v1alpha1" "ComplianceScan" "{{ .Parameters.namespace }}" "" "compliance.openshift.io/suite={{ .Parameters.binding }}").items {{ "}}" }}
{{ "{{" }}- $done := $scans {{ "}}" }}
{{ "{{" }}- range $scans {{ "}}" }}
{{ "{{" }}- if ne (dig "status" "phase" "" .) "DONE" {{ "}}" }}
{{ "{{" }}- $done = false {{ "}}" }}
{{ "{{" }}- end {{ "}}" }}
{{ "{{" }}- end {{ "}}" }}
{{- end -}}
