{{/*
Fails the render on an invalid config.forwarders entry. The name is the ClusterLogForwarder, its
DaemonSet and the prefix of ServiceAccount <name>-collector, which the API caps at 64 characters.
An enabled: false entry needs only its name. A null or empty value counts as unset, as it does in
the converters, so a layer can clear an inherited key.
*/}}
{{- define "logging.validateForwarders" -}}
{{- $settings := dict "syslog" (list "rfc") "splunk" (list "index") "http" list }}
{{- $schemes := dict "syslog" "tls|tcp|udp" "splunk" "https|http" "http" "https|http" }}
{{- $auth := dict "syslog" list "splunk" (list "token") "http" (list "token" "username" "password") }}
{{- range $name, $f := . }}
{{- $p := printf "stack.logging.config.forwarders.%s" $name }}
{{- if not (regexMatch "^[a-z][a-z0-9-]{1,52}[a-z0-9]$" $name) }}{{ fail (printf "%s: the name must be 3-54 lowercase letters, digits and hyphens, starting with a letter and ending with a letter or digit" $p) }}{{ end }}
{{- if eq $name "logging" }}{{ fail (printf "%s: logging is the loki element's ClusterLogForwarder, pick another name" $p) }}{{ end }}
{{- if not (kindIs "map" $f) }}{{ fail (printf "%s must be a map. To remove a forwarder, set enabled: false" $p) }}{{ end }}
{{- if include "policy-library.enabled" $f }}
{{- $type := required (printf "%s.type is required" $p) $f.type }}
{{- if not (hasKey $settings $type) }}{{ fail (printf "%s.type: %q must be one of syslog, splunk, http" $p $type) }}{{ end }}
{{- range $k, $v := $f }}
{{- if and (not (empty $v)) (not (has $k (concat (list "enabled" "type" "url" "logTypes" "secret") (get $settings $type)))) }}{{ fail (printf "%s.%s is not a %s setting" $p $k $type) }}{{ end }}
{{- end }}
{{- $url := required (printf "%s.url is required" $p) $f.url }}
{{- if not (regexMatch (printf "^(%s)://" (get $schemes $type)) (toString $url)) }}{{ fail (printf "%s.url: %s needs a %s:// URL" $p $type (replace "|" "://, " (get $schemes $type))) }}{{ end }}
{{- if not (kindIs "slice" $f.logTypes) }}{{ fail (printf "%s.logTypes is required, a list" $p) }}{{ end }}
{{- if not $f.logTypes }}{{ fail (printf "%s.logTypes is required, a list" $p) }}{{ end }}
{{- range $f.logTypes }}
{{- if not (has . (list "application" "infrastructure" "audit")) }}{{ fail (printf "%s.logTypes: %q must be one of application, infrastructure, audit" $p .) }}{{ end }}
{{- end }}
{{- with $f.rfc }}
{{- if not (has . (list "RFC5424" "RFC3164")) }}{{ fail (printf "%s.rfc: %q must be RFC5424 or RFC3164" $p .) }}{{ end }}
{{- end }}
{{- with $f.index }}
{{- if not (kindIs "string" .) }}{{ fail (printf "%s.index must be a string. Quote an operator template: '{.log_type||\"main\"}'" $p) }}{{ end }}
{{- end }}
{{- $s := $f.secret | default dict }}
{{- if not (kindIs "map" $s) }}{{ fail (printf "%s.secret must be a map: {name, ca, certificate, key, token, username, password}" $p) }}{{ end }}
{{- range $k, $v := $s }}
{{- if and (not (empty $v)) (not (has $k (concat (list "name" "ca" "certificate" "key") (get $auth $type)))) }}{{ fail (printf "%s.secret.%s is not used by type %s" $p $k $type) }}{{ end }}
{{- if and (not (empty $v)) (not (kindIs "string" $v)) }}{{ fail (printf "%s.secret.%s must be a string" $p $k) }}{{ end }}
{{- end }}
{{- if and (not $s.name) (or $s.ca $s.certificate $s.key $s.token $s.username $s.password) }}{{ fail (printf "%s.secret.name is required" $p) }}{{ end }}
{{- if ne (empty $s.certificate) (empty $s.key) }}{{ fail (printf "%s.secret: certificate and key are set together" $p) }}{{ end }}
{{- if ne (empty $s.username) (empty $s.password) }}{{ fail (printf "%s.secret: username and password are set together" $p) }}{{ end }}
{{- if and $s.token $s.username }}{{ fail (printf "%s.secret: set token or username and password, not both" $p) }}{{ end }}
{{- if and (eq $type "splunk") (not $s.token) }}{{ fail (printf "%s.secret.token is required: Splunk authenticates with a HEC token" $p) }}{{ end }}
{{- end }}
{{- end }}
{{- end -}}

{{/* A ClusterLogForwarder reference to key .key of the forwarder's copied Secret <.name>-output. */}}
{{- define "logging.secretRef" -}}
key: {{ .key | quote }}
secretName: {{ .name }}-output
{{- end -}}
