{{/*
ACM condition, true on a hosted control plane guest. HyperShift sets the guest's OAuth from the
HostedCluster on the management cluster and overwrites guest-side edits, so every object skips there.
*/}}
{{- define "oauth.hosted" -}}
eq (dig "status" "controlPlaneTopology" "" (lookup "config.openshift.io/v1" "Infrastructure" "" "cluster")) "External"
{{- end -}}

{{/* A CA bundle with CR and trailing whitespace removed from every line. */}}
{{- define "oauth.ca" -}}
{{- regexReplaceAll "[ \t\r]+\n" (. | toString | trim) "\n" -}}
{{- end -}}

{{/* Fails unless .value is a map of .keys to lists of non-empty strings. */}}
{{- define "oauth.lists" -}}
{{- if not (kindIs "map" .value) }}{{ fail (printf "%s must be a map of %s" .path (join ", " .keys)) }}{{ end }}
{{- range $k, $v := .value }}
{{- if not (has $k $.keys) }}{{ fail (printf "%s.%s is not one of %s" $.path $k (join ", " $.keys)) }}{{ end }}
{{- if not (kindIs "slice" $v) }}{{ fail (printf "%s.%s must be a list" $.path $k) }}{{ end }}
{{- range $v }}
{{- if or (not (kindIs "string" .)) (empty .) }}{{ fail (printf "%s.%s: every entry must be a non-empty string" $.path $k) }}{{ end }}
{{- end }}
{{- end }}
{{- end -}}

{{/*
Fails the render on invalid config. At least one provider must stay enabled: mustonlyhave would
otherwise remove every identity provider, the only way in once kubeadmin is gone. A null or empty
value counts as unset, as it does in the converter, so a layer can clear an inherited key.
*/}}
{{- define "oauth.validate" -}}
{{- $p := "stack.oauth.config" }}
{{- $keys := dict "HTPasswd" (list "secret") "OpenID" (list "secret" "clientID" "issuer" "ca" "claims" "extraScopes" "extraAuthorizeParameters") "LDAP" (list "url" "bindDN" "secret" "insecure" "ca" "attributes") }}
{{- $enabled := 0 }}
{{- range $name, $i := .identityProviders }}
{{- $q := printf "%s.identityProviders.%s" $p $name }}
{{- if not (regexMatch "^[a-z0-9]([-a-z0-9]{0,48}[a-z0-9])?$" $name) }}{{ fail (printf "%s: the name must be 1-50 lowercase letters, digits and hyphens, starting and ending with a letter or digit" $q) }}{{ end }}
{{- if not (kindIs "map" $i) }}{{ fail (printf "%s must be a map. To remove a provider, set enabled: false" $q) }}{{ end }}
{{- if include "policy-library.enabled" $i }}
{{- $enabled = add1 $enabled }}
{{- $type := required (printf "%s.type is required" $q) $i.type }}
{{- if not (hasKey $keys $type) }}{{ fail (printf "%s.type: %q must be one of HTPasswd, OpenID, LDAP" $q $type) }}{{ end }}
{{- range $k, $v := $i }}
{{- if and (not (empty $v)) (not (has $k (concat (list "enabled" "type" "mappingMethod") (get $keys $type)))) }}{{ fail (printf "%s.%s is not a %s setting" $q $k $type) }}{{ end }}
{{- end }}
{{- with $i.mappingMethod }}
{{- if not (has . (list "claim" "lookup" "add")) }}{{ fail (printf "%s.mappingMethod: %q must be one of claim, lookup, add" $q .) }}{{ end }}
{{- end }}
{{- if and $i.secret (not (kindIs "string" $i.secret)) }}{{ fail (printf "%s.secret must be a Secret name" $q) }}{{ end }}
{{- with $i.ca }}
{{- $ca := include "oauth.ca" . }}
{{- if or (not (contains "-----BEGIN CERTIFICATE-----" $ca)) (not (regexMatch "^[\\x20-\\x7e\\n]+$" $ca)) }}{{ fail (printf "%s.ca must be a PEM CA bundle, printable ASCII only" $q) }}{{ end }}
{{- if and (eq $type "LDAP") $i.insecure }}{{ fail (printf "%s: insecure connects without TLS, so it takes no ca" $q) }}{{ end }}
{{- end }}
{{- if eq $type "HTPasswd" }}
{{- $_ := required (printf "%s.secret is required: a hub Secret with key htpasswd" $q) $i.secret }}
{{- else if eq $type "OpenID" }}
{{- $_ := required (printf "%s.secret is required: a hub Secret with key clientSecret" $q) $i.secret }}
{{- $_ = required (printf "%s.clientID is required" $q) $i.clientID }}
{{- if not (kindIs "string" $i.clientID) }}{{ fail (printf "%s.clientID must be a string; quote it" $q) }}{{ end }}
{{- $issuer := toString (required (printf "%s.issuer is required" $q) $i.issuer) }}
{{- if or (not (hasPrefix "https://" $issuer)) (regexMatch "[?#]|/$" $issuer) }}{{ fail (printf "%s.issuer must start with https:// and have no query, fragment or trailing /" $q) }}{{ end }}
{{- with $i.claims }}
{{- include "oauth.lists" (dict "path" (printf "%s.claims" $q) "value" . "keys" (list "preferredUsername" "name" "email" "groups")) }}
{{- end }}
{{- with $i.extraScopes }}
{{- if not (kindIs "slice" .) }}{{ fail (printf "%s.extraScopes must be a list" $q) }}{{ end }}
{{- range . }}
{{- if not (and (kindIs "string" .) (regexMatch "^[!#-\\[\\]-~]+$" .)) }}{{ fail (printf "%s.extraScopes: %v is not a scope; list each scope separately" $q .) }}{{ end }}
{{- end }}
{{- end }}
{{- with $i.extraAuthorizeParameters }}
{{- if not (kindIs "map" .) }}{{ fail (printf "%s.extraAuthorizeParameters must be a map" $q) }}{{ end }}
{{- range $k, $v := . }}
{{- if not (kindIs "string" $v) }}{{ fail (printf "%s.extraAuthorizeParameters.%s must be a string; quote it" $q $k) }}{{ end }}
{{- end }}
{{- end }}
{{- else }}
{{- $url := toString (required (printf "%s.url is required" $q) $i.url) }}
{{- if not (regexMatch "^ldaps?://" $url) }}{{ fail (printf "%s.url must start with ldap:// or ldaps://" $q) }}{{ end }}
{{- if and $i.insecure (hasPrefix "ldaps://" $url) }}{{ fail (printf "%s.insecure: ldaps:// always uses TLS, use ldap:// or drop insecure" $q) }}{{ end }}
{{- if ne (empty $i.bindDN) (empty $i.secret) }}{{ fail (printf "%s: bindDN and secret are set together" $q) }}{{ end }}
{{- if not (and (kindIs "map" $i.attributes) (dig "id" list ($i.attributes | default dict))) }}{{ fail (printf "%s.attributes.id is required, a list such as [dn]" $q) }}{{ end }}
{{- include "oauth.lists" (dict "path" (printf "%s.attributes" $q) "value" $i.attributes "keys" (list "id" "preferredUsername" "name" "email")) }}
{{- end }}
{{- end }}
{{- end }}
{{- if not $enabled }}{{ fail (printf "%s.identityProviders needs at least one enabled provider: the element owns OAuth cluster and would remove every identity provider" $p) }}{{ end }}
{{- $seen := dict }}
{{- range .clusterAdmins }}
{{- if or (not (kindIs "string" .)) (empty .) }}{{ fail (printf "%s.clusterAdmins: every entry must be a user name" $p) }}{{ end }}
{{- if hasKey $seen . }}{{ fail (printf "%s.clusterAdmins: %s appears more than once" $p .) }}{{ end }}
{{- $_ := set $seen . true }}
{{- end }}
{{- with .accessTokenMaxAgeSeconds }}
{{- if or (not (or (kindIs "float64" .) (kindIs "int" .) (kindIs "int64" .))) (lt (float64 .) 0.0) (ne (float64 .) (floor .)) (gt (float64 .) 2147483647.0) }}{{ fail (printf "%s.accessTokenMaxAgeSeconds must be a whole number of seconds" $p) }}{{ end }}
{{- end }}
{{- with .accessTokenInactivityTimeout }}
{{- $q := printf "%s.accessTokenInactivityTimeout" $p }}
{{- if not (regexMatch "^([0-9]+(\\.[0-9]+)?(h|m|s))+$" (toString .)) }}{{ fail (printf "%s: %q must be a duration such as 15m or 1h30m" $q .) }}{{ end }}
{{- $seconds := 0.0 }}
{{- range regexFindAll "[0-9.]+[hms]" . -1 }}
{{- $unit := regexFind "[hms]$" . }}
{{- $seconds = addf $seconds (mulf (trimSuffix $unit .) (get (dict "h" 3600 "m" 60 "s" 1) $unit)) }}
{{- end }}
{{- if lt $seconds 300.0 }}{{ fail (printf "%s: %q is under the 300s minimum" $q .) }}{{ end }}
{{- end }}
{{- end -}}
