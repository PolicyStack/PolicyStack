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
{{- with get ($cfg.bindings | default dict) $cfg.binding }}{{ if and (kindIs "map" .) .profiles }}{{ fail (printf "stack.compliance.config.bindings.%s.profiles: set config.profiles for the default binding" $cfg.binding) }}{{ end }}{{ end -}}
{{- range $name, $b := $cfg.bindings -}}
{{- if not (or (kindIs "invalid" $b) (kindIs "map" $b)) }}{{ fail (printf "stack.compliance.config.bindings.%s must be a map" $name) }}{{ end -}}
{{- end -}}
{{- $owner := dict -}}
{{- range $name, $b := include "compliance.bindings" (dict "Config" $cfg) | fromYaml -}}
{{- $path := printf "stack.compliance.config.bindings.%s" $name -}}
{{- if and (ne $name $cfg.binding) (not $b.profiles) }}{{ fail (printf "%s.profiles is empty" $path) }}{{ end -}}
{{- /* Scans are named after their profile, so two bindings of one profile would share its scans. */ -}}
{{- range $b.profiles -}}
{{- if and (hasKey $owner .) (ne (get $owner .) $name) }}{{ fail (printf "%s.profiles: %s is already in binding %s" $path . (get $owner .)) }}{{ end -}}
{{- $_ := set $owner . $name -}}
{{- end -}}
{{- if not (or (kindIs "invalid" $b.applyAll) (kindIs "bool" $b.applyAll)) }}{{ fail (printf "%s.applyAll must be true or false" $path) }}{{ end -}}
{{- if not (or (kindIs "invalid" $b.exclude) (kindIs "slice" $b.exclude)) }}{{ fail (printf "%s.exclude must be a list of rule names" $path) }}{{ end -}}
{{- range $b.exclude -}}
{{- if not (kindIs "string" .) }}{{ fail (printf "%s.exclude must be a list of rule names" $path) }}{{ end -}}
{{- if regexMatch "^(ocp4|rhcos4)-" . }}{{ fail (printf "%s.exclude: %s is a result name: use its compliance.openshift.io/rule annotation" $path .) }}{{ end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- /*
Every binding by name as YAML, each with profiles, applyAll and exclude: config.binding scans
config.profiles, and config.bindings adds the rest. A layer's null removes an entry.
*/ -}}
{{- define "compliance.bindings" -}}
{{- $out := dict -}}
{{- range $name, $b := .Config.bindings }}{{ if kindIs "map" $b }}{{ $_ := set $out $name $b }}{{ end }}{{ end -}}
{{- $_ := set $out .Config.binding (merge (dict "profiles" .Config.profiles) (get $out .Config.binding | default dict)) -}}
{{- toYaml $out -}}
{{- end -}}

{{- /* Label selector requirement matching the suite of every binding. */ -}}
{{- define "compliance.suites" -}}
{{- $names := keys (include "compliance.bindings" . | fromYaml) | sortAlpha -}}
{{- if eq (len $names) 1 }}compliance.openshift.io/suite={{ first $names }}{{ else }}compliance.openshift.io/suite in ({{ join "," $names }}){{ end -}}
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
{{ "{{" }}- $scans := (lookup "compliance.openshift.io/v1alpha1" "ComplianceScan" "{{ .Config.namespace }}" "" "{{ include "compliance.suites" . }}").items {{ "}}" }}
{{ "{{" }}- $done := $scans {{ "}}" }}
{{ "{{" }}- range $scans {{ "}}" }}
{{ "{{" }}- if ne (dig "status" "phase" "" .) "DONE" {{ "}}" }}
{{ "{{" }}- $done = false {{ "}}" }}
{{ "{{" }}- end {{ "}}" }}
{{ "{{" }}- end {{ "}}" }}
{{- $bindings := include "compliance.bindings" . | fromYaml }}
{{- if gt (len $bindings) 1 }}
{{- /* A binding just added has no scans yet. */}}
{{- range $name, $_ := $bindings }}
{{ "{{" }}- if not (lookup "compliance.openshift.io/v1alpha1" "ComplianceScan" "{{ $.Config.namespace }}" "" "compliance.openshift.io/suite={{ $name }}").items {{ "}}" }}
{{ "{{" }}- $done = false {{ "}}" }}
{{ "{{" }}- end {{ "}}" }}
{{- end }}
{{- end }}
{{- end -}}

{{- /*
ACM statements setting $managed to the remediations applyAll sets: those of a binding with applyAll,
except excluded and waived rules and the families named in config.remediations. A remediation's rule
is on the result of the same name, the -N siblings' too. Renders nothing when no binding sets applyAll.
*/ -}}
{{- define "compliance.managed" -}}
{{- $cfg := .Config -}}
{{- $bindings := include "compliance.bindings" . | fromYaml -}}
{{- $any := false }}{{ range $bindings }}{{ if .applyAll }}{{ $any = true }}{{ end }}{{ end -}}
{{- if $any -}}
{{ "{{" }}- $managed := list {{ "}}" }}
{{ "{{" }}- $named := list
{{- range $name, $apply := $cfg.remediations }}{{ if kindIs "bool" $apply }} {{ regexReplaceAll "-[0-9]+" $name "" | quote }}{{ end }}{{ end }} {{ "}}" }}
{{- range $name, $b := $bindings }}
{{- if $b.applyAll }}
{{ "{{" }}- $skip := {{ include "compliance.waivedRules" $ }}{{ range $b.exclude }} {{ . | quote }}{{ end }} {{ "}}" }}
{{ "{{" }}- $rules := dict {{ "}}" }}
{{ "{{" }}- range (lookup "compliance.openshift.io/v1alpha1" "ComplianceCheckResult" "{{ $cfg.namespace }}" "" "compliance.openshift.io/suite={{ $name }}").items {{ "}}" }}
{{ "{{" }}- $_ := set $rules .metadata.name (dig "metadata" "annotations" "compliance.openshift.io/rule" "" .) {{ "}}" }}
{{ "{{" }}- end {{ "}}" }}
{{ "{{" }}- range (lookup "compliance.openshift.io/v1alpha1" "ComplianceRemediation" "{{ $cfg.namespace }}" "" "compliance.openshift.io/suite={{ $name }}").items {{ "}}" }}
{{ "{{" }}- $rule := get $rules .metadata.name | default (get $rules (regexReplaceAll "-[0-9]+$" .metadata.name "")) {{ "}}" }}
{{ "{{" }}- if not (or (has (regexReplaceAll "-[0-9]+" .metadata.name "") $named) (has $rule $skip)) {{ "}}" }}
{{ "{{" }}- $managed = append $managed . {{ "}}" }}
{{ "{{" }}- end {{ "}}" }}
{{ "{{" }}- end {{ "}}" }}
{{- end }}
{{- end }}
{{- end -}}
{{- end -}}
